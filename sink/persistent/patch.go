package persistent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/vd09-projects/intelligent-tts-narration-library/plan"
	"github.com/vd09-projects/intelligent-tts-narration-library/render"
	"github.com/vd09-projects/intelligent-tts-narration-library/sink"
)

// PatchBlock re-writes a single block's audio into an existing persistent
// outDir, leaving every other block byte-identical (issue #28).
//
// It is a PACKAGE func, not a method on the OutputSink interface, mirroring
// the CheckStale precedent (Decision v1.6.0): a patch is a read-then-rewrite
// operation over an existing dir, not the whole-document Consume write path.
// Consume stays single-purpose; this is the composition root's escalation
// tool.
//
// Source of truth (Route A — Decision: manifest-is-block-INDEX,
// byte-ranges-DERIVED):
//
//	The existing on-disk manifest.json is the authoritative block INDEX. Each
//	block's byte range inside audio.wav is DERIVED from manifest timing
//	(StartMs/EndMs) + AudioFormat via the same silenceBytes() math
//	writeCombinedWAV used to write it — never stored. A stored byte offset
//	would be a second source of truth that could drift into an undetectable
//	corruption class the honesty rule forbids; a derived offset cannot drift,
//	it can only DISAGREE with a tampered container, which the F1 container-vs-
//	manifest check (deriveDataLen vs the on-disk data length) turns into an
//	explicit ErrContainerMismatch refusal rather than a silent mis-slice.
//
// Flow (planner-task.md Patch Algorithm):
//  1. Pre-flight: outDir exists and holds readable manifest.json + audio.wav
//     + plan.json. Missing any → errNothingToPatch (refusal, exit 2).
//  2. Stale check (CheckStale). stale → refusal; corrupt manifest → hard error.
//  3. Content-hash gate: manifest.ContentHash must equal the re-render's
//     document hash (p.Source.ContentHash). Mismatch → ErrContentHashMismatch
//     refusal — patching a different document is corruption.
//  4. Locate target block in the manifest. Absent → ErrUnknownBlock refusal.
//  5. F1 container-vs-manifest validation: derive the expected data-chunk
//     length from the manifest and compare to the actual on-disk data length.
//     Mismatch → ErrContainerMismatch refusal. Unparseable WAV → hard error.
//  6. Format-mismatch gate: the re-rendered block's format must match the
//     container's. Mismatch → formatMismatchError BEFORE any tmp write.
//  7. Reconstruct the segment list: harvest every non-target block's PCM from
//     the existing container by its derived byte range; take the target
//     block's PCM fresh from res.
//  8. Recompute downstream timing via the same leading = StartMs - cursor walk
//     Consume uses (no special-case code).
//  9. Commit per the Cross-File Write Ordering Invariant (stage all three tmp
//     files, then rename plan.json, manifest.json, audio.wav LAST).
// 10. Return a SinkReceipt identical in shape to Consume's (S1).
//
// F1 (input-guard) and F2 (write-ordering) are INDEPENDENT guarantees, NOT a
// detect-and-recover pair: F1 guards the container PatchBlock harvests
// non-target PCM FROM; crash-consistency of the OUTPUT is carried by the
// write-ordering invariant PLUS the fact that a patch re-run always
// reconstructs and rewrites all three files, so any half-committed state is
// overwritten, not detected (planner-task.md Recovery Invariant, F3).
func PatchBlock(ctx context.Context, outDir string, p plan.NarrationPlan, res render.RenderResult, blockID string, opts ...Option) (sink.SinkReceipt, error) {
	var receipt sink.SinkReceipt

	if err := ctx.Err(); err != nil {
		return receipt, err
	}
	if outDir == "" {
		return receipt, ErrNoOutDir
	}

	// expectedFormat — honor WithExpectedFormat if supplied, else the
	// documented render.DefaultFormat() default (same resolution Consume uses).
	cfg := &Sink{ExpectedFormat: defaultExpectedFormat()}
	for _, opt := range opts {
		opt(cfg)
	}
	expectedFormat := cfg.ExpectedFormat
	if expectedFormat == (plan.AudioFormat{}) {
		expectedFormat = defaultExpectedFormat()
	}

	audioPath := filepath.Join(outDir, audioFilename)
	planPath := filepath.Join(outDir, planFilename)
	manifestPath := filepath.Join(outDir, manifestFilename)

	// Step 1 — pre-flight existence. All three canonical files must be present
	// and readable. A missing file means "nothing to patch": inventing an
	// outDir or a manifest would violate the honesty rule (refuse, don't
	// fabricate). errNothingToPatch routes to exit 2 (caller-correctable).
	for _, pth := range []string{manifestPath, audioPath, planPath} {
		if _, err := os.Stat(pth); err != nil {
			if errors.Is(err, os.ErrNotExist) {
				return receipt, fmt.Errorf("%w: %s", ErrNothingToPatch, pth)
			}
			return receipt, fmt.Errorf("sink/persistent: stat %s: %w", pth, err)
		}
	}

	// Step 2 — stale check first. A stale outDir must not be patched: the
	// source has drifted from what produced the bytes on disk, so harvesting
	// neighbors would splice stale audio. Corrupt manifest (err != nil) is a
	// hard error — we cannot tell, so we do not patch.
	stale, reason, err := CheckStale(outDir, p)
	if err != nil {
		return receipt, fmt.Errorf("sink/persistent: patch stale-check: %w", err)
	}
	if stale {
		return receipt, fmt.Errorf("%w: %s", ErrStalePatch, reason)
	}

	// Read the manifest as the authoritative block INDEX. (CheckStale already
	// parsed it; re-read once here for the block layout — patch is interactive,
	// not a hot path, so the extra read is acceptable.)
	m, err := readManifest(manifestPath)
	if err != nil {
		return receipt, fmt.Errorf("sink/persistent: patch read manifest: %w", err)
	}

	// Step 3 — content-hash gate. CheckStale already refuses a manifest whose
	// ContentHash differs from the plan's, so this is belt-and-suspenders: an
	// explicit, named refusal for the cross-document case even if a future
	// CheckStale change relaxes its comparison. The --expected-content-hash
	// flag is OPTIONAL (Open Question #3 — resolved optional): this manifest
	// gate already refuses cross-document patches without it.
	if m.ContentHash != p.Source.ContentHash {
		return receipt, fmt.Errorf("%w: manifest=%s plan=%s",
			ErrContentHashMismatch, m.ContentHash, p.Source.ContentHash)
	}

	// Step 4 — locate the target block in the manifest index.
	targetIdx := -1
	for i := range m.Blocks {
		if m.Blocks[i].ID == blockID {
			targetIdx = i
			break
		}
	}
	if targetIdx < 0 {
		return receipt, fmt.Errorf("%w: %s", ErrUnknownBlock, blockID)
	}

	// Step 5 — F1 container-vs-manifest validation. Derive each block's byte
	// range from the manifest timing + format; the sum is the expected data
	// length. Read the actual on-disk audio.wav data chunk. A structurally
	// unreadable / parse-failing audio.wav is a HARD error (exit 1) — we
	// cannot trust bytes we cannot parse. A parseable container whose data
	// length disagrees with the manifest-derived length is a REFUSAL
	// (ErrContainerMismatch, exit 2) — the container does not match its index;
	// do not slice.
	layout, derivedDataLen := deriveBlockLayout(m.Blocks, expectedFormat)
	_, containerPCM, err := readWAV(audioPath, expectedFormat)
	if err != nil {
		// Format mismatch on the EXISTING container is also a hard error here:
		// the on-disk file is not the format we index it against, so we cannot
		// safely harvest. (The re-rendered block's format is checked at step 6.)
		return receipt, fmt.Errorf("sink/persistent: patch read container: %w", err)
	}
	if derivedDataLen != len(containerPCM) {
		return receipt, fmt.Errorf("%w: manifest-derived data length %d != on-disk data length %d",
			ErrContainerMismatch, derivedDataLen, len(containerPCM))
	}

	// Step 6 — format-mismatch gate on the RE-RENDERED block. Read the fresh
	// target audio from res; a format divergence refuses BEFORE any tmp write
	// (S3) so the existing audio.wav stays untouched. An empty AudioRef on the
	// re-render means a zero-length target block (all-pause re-render); that is
	// valid — the target contributes no PCM.
	var targetPCM []byte
	targetTiming, ok := timingForBlock(res.Timeline.Blocks, blockID)
	if !ok {
		// The re-render did not produce a timing row for the requested block.
		// Treat as an internal contract breach rather than a refusal — the
		// caller handed us a RenderResult that does not describe the block it
		// asked us to patch.
		return receipt, fmt.Errorf("sink/persistent: patch: re-render result has no timing for block %s", blockID)
	}
	if targetTiming.AudioRef != "" {
		freshPath := filepath.Join(res.Audio.Dir, targetTiming.AudioRef)
		_, pcm, rerr := readWAV(freshPath, expectedFormat)
		if rerr != nil {
			// A format mismatch on the re-render is a refusal (exit 2) surfaced
			// via formatMismatchError; a structurally invalid re-render WAV is
			// a hard error. readWAV returns formatMismatchError for the former
			// and ErrInvalidWAV for the latter — both flow up unwrapped-enough
			// for exitCodeFor to classify (formatMismatchError → 2).
			return receipt, fmt.Errorf("sink/persistent: patch read re-rendered block %s: %w", blockID, rerr)
		}
		targetPCM = pcm
	}

	// Step 7 — reconstruct the segment list. For every block other than the
	// target, slice its PCM from the existing container by its derived byte
	// range (Route A). For the target, use the freshly rendered PCM. The
	// leading silence for each segment is recomputed in step 8 from the
	// patched timing so a target length delta shifts neighbors correctly.
	pcmByIndex := make([][]byte, len(m.Blocks))
	for i, bl := range layout {
		if i == targetIdx {
			pcmByIndex[i] = targetPCM
			continue
		}
		// bl.pcmStart/bl.pcmEnd are offsets into the container data chunk.
		pcmByIndex[i] = containerPCM[bl.pcmStart:bl.pcmEnd]
	}

	// B1 fix — preserve the multi-block plan.json. `p` is the RE-RENDER plan: on
	// the CLI patch path it is a ONE-block sub-plan (the pipeline's single-block
	// branch trims Blocks to the target). Writing it verbatim would clobber
	// plan.json down to a single block, destroying the plan contract for every
	// other block. Instead read the EXISTING on-disk plan.json as the
	// authoritative document plan and SPLICE only the target block's plan entry
	// from `p`, preserving every other block verbatim (the manifest already
	// does this for timing). Source/Defaults/Diagnostics stay as on disk.
	// `target` is the single source of truth for the patched block's
	// Class/Level/Status — used for BOTH plan.json (already spliced) and the
	// manifest block below (B2), so the two files never disagree (S5).
	patchedPlan, target, err := mergeTargetBlockPlan(planPath, p, blockID)
	if err != nil {
		return receipt, err
	}

	// Step 8 — recompute downstream timing + build segments via the same
	// leading = StartMs - cursor walk Consume uses. The patched block's audio
	// length (in ms) is derived from its PCM length; every subsequent block's
	// StartMs/EndMs shifts by the delta. cursorMs walks by each block's new
	// EndMs so the leading-silence math is identical to the write path,
	// including the StartMs>0 prefix before block zero (Decision:
	// persistent-leading-silence-before-block-zero).
	segments := make([]wavSegment, 0, len(m.Blocks))
	patchedBlocks := make([]ManifestBlock, len(m.Blocks))
	cursorMs := 0
	for i := range m.Blocks {
		orig := m.Blocks[i]
		// Leading silence = original planned gap before this block. The gap is
		// a property of the plan (inter-block spacing), preserved as-authored;
		// only the block's own duration changes when the target is re-rendered.
		leading := max(orig.StartMs-origCursor(m.Blocks, i), 0)

		durMs := pcmDurationMs(len(pcmByIndex[i]), expectedFormat)
		startMs := cursorMs + leading
		endMs := startMs + durMs

		segments = append(segments, wavSegment{
			pcm:              pcmByIndex[i],
			leadingSilenceMs: leading,
		})

		pb := orig
		pb.StartMs = startMs
		pb.EndMs = endMs
		if i == targetIdx {
			// B2 — sync the patched block's classification with plan.json. The
			// re-render may have re-classified the block (e.g. an escalation
			// L1→L3, or a refused→voiced enrich); the manifest index must mirror
			// the plan or a manifest-first consumer reads a stale verdict.
			pb.Class = target.Class
			pb.Level = target.Level
			pb.Status = target.Status
		}
		patchedBlocks[i] = pb

		receipt.TotalDurationMs += int64(endMs - startMs)
		if len(pcmByIndex[i]) > 0 {
			receipt.BlocksPlayed++
		}
		cursorMs = endMs
	}

	// Build the patched manifest. The manifest carries the recomputed per-block
	// timing + the synced target classification; everything else (schema
	// version, source, format, voice) is preserved from the existing manifest —
	// a patch changes audio, not provenance. Voice is preserved from disk unless
	// a WithVoice option overrides it (composition root supplies the current
	// voice; voice is document-global, not per-block — S2).
	patchedManifest := m
	patchedManifest.Blocks = patchedBlocks
	patchedManifest.AudioFormat = expectedFormat
	if cfg.Voice != "" {
		patchedManifest.Voice = cfg.Voice
	}
	patchedManifest.Stale = false
	patchedManifest.StaleReason = ""

	// Step 9 — commit per the Cross-File Write Ordering & Recovery Invariant.
	if err := commitPatch(outDir, expectedFormat, segments, patchedPlan, patchedManifest); err != nil {
		return receipt, err
	}

	// Step 10 — return the receipt (Consume-shaped).
	return receipt, nil
}

// mergeTargetBlockPlan reads the existing on-disk plan.json (the authoritative
// multi-block document plan) and returns a copy with ONLY the target block's
// plan entry replaced by the re-render's version (matched by id). Every other
// block — and Source / Defaults / Diagnostics — is preserved verbatim. This is
// the B1 fix: a single-block re-render must not truncate plan.json.
//
// rerender is the plan PatchBlock was handed (the sub-plan on the CLI path, the
// full plan in direct callers/tests). Its block matching blockID supplies the
// updated Class/Level/Status/Provenance/Segments for the patched block. If the
// re-render plan does not contain blockID, the on-disk plan is returned
// unchanged for that block (the audio/manifest still patch; the plan entry was
// not re-described).
//
// If the on-disk plan.json is missing the target block id, that is an
// inconsistency between plan.json and manifest.json — returned as an error
// (the pre-flight + F1 checks already validated the manifest/container, so a
// plan/manifest divergence here is a genuine corruption we refuse on).
func mergeTargetBlockPlan(planPath string, rerender plan.NarrationPlan, blockID string) (plan.NarrationPlan, plan.Block, error) {
	onDisk, err := readPlan(planPath)
	if err != nil {
		return plan.NarrationPlan{}, plan.Block{}, fmt.Errorf("sink/persistent: patch read plan.json: %w", err)
	}

	var rerendered *plan.Block
	for i := range rerender.Blocks {
		if rerender.Blocks[i].ID == blockID {
			rerendered = &rerender.Blocks[i]
			break
		}
	}

	var target plan.Block
	found := false
	for i := range onDisk.Blocks {
		if onDisk.Blocks[i].ID == blockID {
			found = true
			if rerendered != nil {
				// Preserve Order from the on-disk plan (document position is a
				// property of the document, not the re-render).
				order := onDisk.Blocks[i].Order
				onDisk.Blocks[i] = *rerendered
				onDisk.Blocks[i].Order = order
			}
			target = onDisk.Blocks[i]
			break
		}
	}
	if !found {
		return plan.NarrationPlan{}, plan.Block{}, fmt.Errorf("%w: %s present in manifest but absent from plan.json", ErrContainerMismatch, blockID)
	}
	// target is the single source of truth for the patched block's
	// Class/Level/Status — used BOTH for plan.json (already spliced into onDisk)
	// AND for the manifest block (B2), so the two files never disagree (S5).
	return onDisk, target, nil
}

// readPlan loads a NarrationPlan from path. An absent file is wrapped so
// callers can errors.Is(os.ErrNotExist); a corrupt payload is a hard error.
func readPlan(path string) (plan.NarrationPlan, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return plan.NarrationPlan{}, fmt.Errorf("read plan %s: %w", path, err)
	}
	var p plan.NarrationPlan
	if err := json.Unmarshal(raw, &p); err != nil {
		return plan.NarrationPlan{}, fmt.Errorf("parse plan %s: %w", path, err)
	}
	return p, nil
}

// blockLayout is one block's derived byte range inside the container's data
// chunk. pcmStart/pcmEnd bound the block's PCM payload (after its leading
// silence); the slice [pcmStart:pcmEnd] is exactly that block's audio.
type blockLayout struct {
	pcmStart int
	pcmEnd   int
}

// deriveBlockLayout walks the manifest blocks computing each block's PCM byte
// range inside the container data chunk, using the SAME silenceBytes() math
// writeCombinedWAV used to write it (Route A). Returns the per-block layout
// and the total derived data-chunk length (Σ leading silence + Σ PCM) for the
// F1 container-vs-manifest check.
//
// Layout per block i: silenceBytes(leading_i) of zeros, then pcmLen_i bytes of
// PCM. leading_i = max(StartMs_i - cursorMs, 0); pcmLen_i =
// silenceBytes(EndMs_i - StartMs_i) — the block's own span converted to bytes.
// cursorMs walks by EndMs_i. This is the exact inverse of the write walk when
// block durations are frame-aligned (the renderer derives EndMs from PCM byte
// count, so the round-trip is exact for renderer-produced audio); any
// non-frame-aligned drift surfaces as a derivedDataLen mismatch the F1 check
// refuses on, rather than a silent mis-slice.
func deriveBlockLayout(blocks []ManifestBlock, format plan.AudioFormat) ([]blockLayout, int) {
	layout := make([]blockLayout, len(blocks))
	offset := 0
	cursorMs := 0
	for i, b := range blocks {
		leading := max(b.StartMs-cursorMs, 0)
		offset += silenceBytes(leading, format)
		pcmLen := silenceBytes(b.EndMs-b.StartMs, format)
		layout[i] = blockLayout{pcmStart: offset, pcmEnd: offset + pcmLen}
		offset += pcmLen
		cursorMs = b.EndMs
	}
	return layout, offset
}

// origCursor returns the cumulative end (ms) of all blocks before index i in
// the ORIGINAL manifest — the cursor position the leading-silence gap before
// block i is measured against. Pulled out so the step-8 recompute walk reads
// the original inter-block gaps (StartMs_i - prevEnd) without mutating state
// mid-loop.
func origCursor(blocks []ManifestBlock, i int) int {
	if i == 0 {
		return 0
	}
	return blocks[i-1].EndMs
}

// pcmDurationMs converts a PCM byte count to milliseconds at the given format,
// the inverse of silenceBytes for whole-frame inputs. Used to recompute the
// patched block's duration (and thus downstream StartMs/EndMs) from its actual
// PCM length. Rounds to nearest ms, matching render/sherpa's wavDurationMs.
func pcmDurationMs(pcmBytes int, format plan.AudioFormat) int {
	if pcmBytes <= 0 {
		return 0
	}
	bitDepth, err := bitDepthFromEncoding(format.Encoding)
	if err != nil {
		return 0
	}
	bytesPerSec := int64(format.SampleRate) * int64(format.Channels) * int64(bitDepth) / 8
	if bytesPerSec == 0 {
		return 0
	}
	return int((int64(pcmBytes)*1000 + bytesPerSec/2) / bytesPerSec)
}

// timingForBlock finds the BlockTiming row for blockID in the re-render
// result. The re-render hands a one-row Timeline (the single-block path), but
// we look up by id rather than assuming index 0 so the func is robust to a
// future multi-row RenderResult.
func timingForBlock(blocks []plan.BlockTiming, blockID string) (plan.BlockTiming, bool) {
	for _, bt := range blocks {
		if bt.BlockID == blockID {
			return bt, true
		}
	}
	return plan.BlockTiming{}, false
}

// commitPatch stages all three outputs to sibling tmp files, then commits by
// rename in the order plan.json, manifest.json, audio.wav LAST (the
// Cross-File Write Ordering Invariant, F2). The container the manifest indexes
// is the last thing to land, so a crash never leaves a manifest pointing at
// audio that does not exist; a manifest-first consumer is never handed an
// index to a missing container.
//
// All bytes are staged first; only renames remain in the commit window, so the
// reachable crash states are exactly (i) the old consistent triple or (ii)
// new plan/manifest + old audio — the latter is self-corrected by the next
// patch re-run (which reconstructs + rewrites everything).
func commitPatch(outDir string, format plan.AudioFormat, segments []wavSegment, p plan.NarrationPlan, m Manifest) (retErr error) {
	// Stage plan.json bytes.
	planRaw, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return fmt.Errorf("sink/persistent: patch marshal plan: %w", err)
	}
	planRaw = append(planRaw, '\n')

	// Stage manifest.json bytes.
	manifestRaw, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return fmt.Errorf("sink/persistent: patch marshal manifest: %w", err)
	}
	manifestRaw = append(manifestRaw, '\n')

	// Stage all three tmp files (audio via writeCombinedWAV) BEFORE any rename.
	// Each staged file is tracked so a failure mid-commit removes only the
	// tmps that were not yet renamed into place (committed entries are marked
	// so the deferred cleanup skips them).
	planTmp, err := stageTmp(outDir, ".persistent-plan-*.tmp", planRaw)
	if err != nil {
		return err
	}
	manifestTmp, err := stageTmp(outDir, ".persistent-manifest-*.tmp", manifestRaw)
	if err != nil {
		_ = os.Remove(planTmp)
		return err
	}
	audioTmp, err := stageAudioTmp(outDir, format, segments)
	if err != nil {
		_ = os.Remove(planTmp)
		_ = os.Remove(manifestTmp)
		return err
	}

	staged := []*stagedTmp{{path: planTmp}, {path: manifestTmp}, {path: audioTmp}}
	defer func() {
		if retErr == nil {
			return
		}
		for _, s := range staged {
			if !s.committed {
				_ = os.Remove(s.path)
			}
		}
	}()

	// Commit by rename — plan.json, then manifest.json, then audio.wav LAST.
	// renameFn is the injectable seam tests use to fail mid-sequence.
	targets := []string{planFilename, manifestFilename, audioFilename}
	for i, s := range staged {
		if err := renameFn(s.path, filepath.Join(outDir, targets[i])); err != nil {
			return fmt.Errorf("sink/persistent: patch rename %s: %w", targets[i], err)
		}
		s.committed = true
	}
	return nil
}

// stagedTmp tracks one staged tmp file through the commit window. committed
// flips true once the file is renamed into place, so the deferred cleanup
// removes only the tmps a mid-commit failure left behind.
type stagedTmp struct {
	path      string
	committed bool
}

// renameFn is the rename seam commitPatch uses. Production is os.Rename; tests
// swap it to inject a mid-sequence failure between renames (planner-task.md
// test row i — the #29 openTempFile seam is a SOFT dependency, so we keep a
// package-local var rather than taking a hard dependency on it).
var renameFn = os.Rename

// stageTmp writes data to a fresh tmp file in outDir and returns its path
// (closed, not yet renamed). The caller renames it into place during the
// commit window.
func stageTmp(outDir, pattern string, data []byte) (string, error) {
	tmp, err := os.CreateTemp(outDir, pattern)
	if err != nil {
		return "", fmt.Errorf("sink/persistent: patch create tmp in %s: %w", outDir, err)
	}
	tmpPath := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
		return "", fmt.Errorf("sink/persistent: patch write tmp %s: %w", tmpPath, err)
	}
	if err := tmp.Chmod(0o644); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
		return "", fmt.Errorf("sink/persistent: patch chmod tmp %s: %w", tmpPath, err)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return "", fmt.Errorf("sink/persistent: patch close tmp %s: %w", tmpPath, err)
	}
	return tmpPath, nil
}

// stageAudioTmp streams the combined WAV to a fresh tmp file in outDir and
// returns its path (closed, not yet renamed).
func stageAudioTmp(outDir string, format plan.AudioFormat, segments []wavSegment) (string, error) {
	tmp, err := os.CreateTemp(outDir, ".persistent-audio-*.wav")
	if err != nil {
		return "", fmt.Errorf("sink/persistent: patch create tmp audio in %s: %w", outDir, err)
	}
	tmpPath := tmp.Name()
	if err := writeCombinedWAV(tmp, format, segments); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
		return "", err
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return "", fmt.Errorf("sink/persistent: patch close tmp audio %s: %w", tmpPath, err)
	}
	return tmpPath, nil
}

