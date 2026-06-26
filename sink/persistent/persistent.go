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

// Output filenames inside the configured OutDir. Names are stable —
// downstream consumers (the React reference player, future MCP tooling)
// resolve these by convention rather than reading them out of a config.
const (
	audioFilename    = "audio.wav"
	planFilename     = "plan.json"
	manifestFilename = "manifest.json"
)

// Sink is the persistent OutputSink. Construct via New; the zero value is
// not usable because OutDir is mandatory (Decision v1.3.0 — positional
// argument so the omission is a compile error rather than a runtime check).
//
// Field semantics:
//   - OutDir — destination directory. Created with 0o755 if missing.
//   - Voice — engine voice id used for this render (e.g. "af_bella"). Flows
//     in via WithVoice from the composition root (Decision v1.8.0). Empty
//     is a valid manifest — the sink does not invent a voice id.
//   - ExpectedFormat — the AudioFormat readWAV validates per-block WAVs
//     against. Defaults to render.DefaultFormat() (Decision v1.0.0).
type Sink struct {
	OutDir         string
	Voice          string
	ExpectedFormat plan.AudioFormat
}

// Compile-time assertion: Sink implements sink.OutputSink. Matches the
// pattern used by sink/ephemeral so a future interface drift fails at
// build time.
var _ sink.OutputSink = (*Sink)(nil)

// Option — functional config for New.
type Option func(*Sink)

// WithVoice records the engine voice id used for this render so the
// manifest can carry it forward (Decision v1.8.0). The sink itself never
// looks at the value — it only writes it.
func WithVoice(voiceID string) Option {
	return func(s *Sink) { s.Voice = voiceID }
}

// WithExpectedFormat overrides the AudioFormat readWAV validates per-block
// WAVs against. The default render.DefaultFormat() suits the phase-one
// Kokoro renderer; a future engine would supply its own.
func WithExpectedFormat(f plan.AudioFormat) Option {
	return func(s *Sink) { s.ExpectedFormat = f }
}

// New constructs a persistent Sink with outDir applied and the provided
// options layered on. outDir is positional because it is mandatory —
// making it an option would let callers silently omit it (Decision v1.3.0).
//
// ExpectedFormat defaults to render.DefaultFormat() when no
// WithExpectedFormat option is supplied; the option still overrides.
func New(outDir string, opts ...Option) *Sink {
	s := &Sink{
		OutDir:         outDir,
		ExpectedFormat: defaultExpectedFormat(),
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// ErrNoOutDir signals that Consume was called on a Sink with an empty
// OutDir. New rejects this at construction-time when used through the
// canonical path, but the zero-value Sink path is preserved so this
// sentinel exists as a final guard.
var ErrNoOutDir = errors.New("sink/persistent: OutDir is empty")

// ErrMissingBlockAudio signals that a block's BlockTiming.AudioRef
// pointed to a file that does not exist on disk. The wrapped error
// carries the block id + the resolved absolute path so the caller can
// debug why the renderer left a gap.
var ErrMissingBlockAudio = errors.New("sink/persistent: block audio file missing")

// Consume writes audio.wav, plan.json, and manifest.json into s.OutDir.
//
// Flow (planner-task.md step 3):
//  1. Validate OutDir + MkdirAll.
//  2. Walk res.Timeline.Blocks in plan order:
//     - Pre-block ctx.Err() short-circuit (partial receipt + ctx.Err()).
//     - Empty AudioRef → skip read; planned duration still counts toward
//     receipt; BlocksPlayed does NOT increment.
//     - Otherwise → readWAV the per-block file; format mismatch / missing
//     file → wrapped error with block id; partial receipt; output files
//     NOT written (partial-write guard).
//     - Track cursor + planned span for leading silence math.
//  3. Atomically write audio.wav, plan.json, manifest.json via tmp+rename
//     (Decision v1.4.0 — partial state on disk would violate the honesty
//     rule).
//
// Refusal handling: refused blocks already carry audio of Refusal.Message
// produced by the renderer; the sink treats them identical to voiced
// blocks. The manifest preserves Block.Status so downstream consumers can
// surface "this is a refusal".
func (s *Sink) Consume(ctx context.Context, p plan.NarrationPlan, res render.RenderResult) (sink.SinkReceipt, error) {
	var receipt sink.SinkReceipt

	if s.OutDir == "" {
		return receipt, ErrNoOutDir
	}
	if err := os.MkdirAll(s.OutDir, 0o755); err != nil {
		return receipt, fmt.Errorf("sink/persistent: mkdir %s: %w", s.OutDir, err)
	}

	// expectedFormat — honor the option if set, otherwise the documented
	// render.DefaultFormat() default. The zero-value Sink path (no New)
	// would land here with an empty AudioFormat; treat that as "use default"
	// so the read side doesn't blow up on a zero-value misconfiguration.
	expectedFormat := s.ExpectedFormat
	if expectedFormat == (plan.AudioFormat{}) {
		expectedFormat = defaultExpectedFormat()
	}

	// segments accumulates per-block PCM + leading-silence pairs that
	// writeCombinedWAV will concatenate into a single audio.wav. We collect
	// the whole list before writing so a per-block error leaves no partial
	// audio.wav behind.
	segments := make([]wavSegment, 0, len(res.Timeline.Blocks))
	cursorMs := 0
	for _, blk := range res.Timeline.Blocks {
		if err := ctx.Err(); err != nil {
			return receipt, err
		}

		planned := int64(blk.EndMs - blk.StartMs)
		// Leading silence = the gap between the previous block's end (cursor)
		// and this block's planned start. Negative gaps (overlapping blocks)
		// clamp to zero inside silenceBytes — overlap is a planner bug, not
		// a corrupting condition the sink should propagate.
		leading := max(blk.StartMs-cursorMs, 0)

		if blk.AudioRef == "" {
			// Empty AudioRef = no on-disk WAV for this block (all-pause or
			// no-speech). Planned duration still flows into the receipt so
			// the receipt mirrors plan truth, but BlocksPlayed does NOT
			// increment (matches ephemeral sink invariant). The leading
			// silence still emits per Decision v1.5.0 — fidelity to the
			// planned timeline.
			segments = append(segments, wavSegment{pcm: nil, leadingSilenceMs: leading})
			receipt.TotalDurationMs += planned
			cursorMs = blk.EndMs
			continue
		}

		absPath := filepath.Join(res.Audio.Dir, blk.AudioRef)
		_, pcm, err := readWAV(absPath, expectedFormat)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				return receipt, fmt.Errorf("%w: block %s at %s", ErrMissingBlockAudio, blk.BlockID, absPath)
			}
			return receipt, fmt.Errorf("sink/persistent: read block %s: %w", blk.BlockID, err)
		}
		segments = append(segments, wavSegment{pcm: pcm, leadingSilenceMs: leading})
		receipt.TotalDurationMs += planned
		receipt.BlocksPlayed++
		cursorMs = blk.EndMs
	}

	// Atomic write of audio.wav. writeCombinedWAV streams to an io.Writer;
	// we wrap a tmp file so a ctx-cancel mid-write leaves the existing
	// audio.wav untouched. Same pattern manifest.go uses for JSON.
	if err := writeAudio(filepath.Join(s.OutDir, audioFilename), expectedFormat, segments); err != nil {
		return receipt, fmt.Errorf("sink/persistent: write %s: %w", audioFilename, err)
	}

	// plan.json — verbatim from the input NarrationPlan. The sink never
	// rewrites, sanitizes, or re-derives plan fields (CLAUDE.md "plan is
	// the contract"). MarshalIndent gives deterministic, human-readable
	// output; a trailing newline matches manifest.json's cat-friendly
	// convention.
	planRaw, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return receipt, fmt.Errorf("sink/persistent: marshal plan: %w", err)
	}
	planRaw = append(planRaw, '\n')
	if err := atomicWriteFile(filepath.Join(s.OutDir, planFilename), planRaw, 0o644); err != nil {
		return receipt, fmt.Errorf("sink/persistent: write %s: %w", planFilename, err)
	}

	// Manifest — built from the input plan + render result. Stale is false
	// on a fresh write; CheckStale is the read-side query that flips it
	// (Decision v1.6.0). The sink never writes Stale=true: auto-
	// regeneration is forbidden by the honesty rule.
	m := buildManifest(p, res, s.Voice)
	if err := writeManifest(filepath.Join(s.OutDir, manifestFilename), m); err != nil {
		return receipt, fmt.Errorf("sink/persistent: write %s: %w", manifestFilename, err)
	}

	return receipt, nil
}

// writeAudio is a thin wrapper that opens a tmp file in the same directory,
// calls writeCombinedWAV, closes, and renames atomically. Pulled out from
// Consume to keep the per-block walk readable and to centralize the
// tmp+rename ceremony alongside atomicWriteFile.
//
// On any failure during write/close/rename, the tmp file is removed
// best-effort. The original audio.wav (if any) stays untouched.
func writeAudio(path string, format plan.AudioFormat, segments []wavSegment) (retErr error) {
	dir := filepathDir(path)
	tmp, err := openTempFile(dir, ".persistent-audio-*.wav")
	if err != nil {
		return fmt.Errorf("create tmp audio in %s: %w", dir, err)
	}
	tmpPath := tmp.Name()
	defer func() {
		if retErr != nil {
			_ = os.Remove(tmpPath)
		}
	}()
	if err := writeCombinedWAV(tmp, format, segments); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close tmp audio %s: %w", tmpPath, err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("rename %s -> %s: %w", tmpPath, path, err)
	}
	return nil
}

// buildManifest joins per-block timing data (BlockTiming) with the source-
// of-truth plan blocks (Block) so the manifest carries class/level/status
// alongside start/end/audio_ref. The plan is the source of truth for
// classification + leveling; BlockTiming is the source of truth for ms
// offsets + AudioRef.
//
// If a BlockTiming.BlockID has no matching plan.Block (would only happen
// on a malformed render result), the manifest block still gets recorded
// with empty class/level/status rather than dropping it — the manifest is
// a receipt, not a validator.
func buildManifest(p plan.NarrationPlan, res render.RenderResult, voice string) Manifest {
	planByID := make(map[string]plan.Block, len(p.Blocks))
	for _, b := range p.Blocks {
		planByID[b.ID] = b
	}

	blocks := make([]ManifestBlock, 0, len(res.Timeline.Blocks))
	for _, bt := range res.Timeline.Blocks {
		mb := ManifestBlock{
			ID:       bt.BlockID,
			StartMs:  bt.StartMs,
			EndMs:    bt.EndMs,
			AudioRef: bt.AudioRef,
		}
		if pb, ok := planByID[bt.BlockID]; ok {
			mb.Class = pb.Class
			mb.Level = pb.Level
			mb.Status = pb.Status
		}
		blocks = append(blocks, mb)
	}

	return Manifest{
		SchemaVersion:     ManifestSchemaVersion,
		PlanSchemaVersion: p.SchemaVersion,
		SourceKind:        p.Source.Kind,
		SourceURI:         p.Source.URI,
		ContentHash:       p.Source.ContentHash,
		AudioFormat:       res.Format,
		Voice:             voice,
		Blocks:            blocks,
		Stale:             false,
		StaleReason:       "",
	}
}
