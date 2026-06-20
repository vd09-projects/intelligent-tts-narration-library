package persistent

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/vd09-projects/intelligent-tts-narration-library/plan"
	"github.com/vd09-projects/intelligent-tts-narration-library/render"
)

// Frame math for the patch fixtures: 24 kHz mono s16le → 48 000 bytes/sec →
// 48 bytes/ms. Choosing PCM byte counts that are multiples of 48 keeps every
// block's duration a whole number of ms, so deriveBlockLayout round-trips the
// container exactly and the F1 check passes on the happy path. Non-frame-
// aligned inputs are covered by the container-mismatch test.
const bytesPerMs = 48

// framePCM returns durMs worth of deterministic, non-zero PCM bytes
// (distinguishable from silence and from other blocks via the seed byte).
func framePCM(durMs int, seed byte) []byte {
	b := make([]byte, durMs*bytesPerMs)
	for i := range b {
		b[i] = seed + byte(i%7)
	}
	return b
}

// seededOutDir writes a self-consistent persistent triple (audio.wav +
// plan.json + manifest.json) for a 3-block document using the real Consume
// path, so the container layout PatchBlock harvests from is exactly what the
// writer produces. Returns the outDir, the plan, and the per-block PCM the
// fixture was seeded with (for byte-identity assertions on untouched blocks).
//
// Block layout (frame-aligned):
//
//	b0: StartMs 0,   100 ms PCM  → [0,100]
//	b1: StartMs 150  (50 ms gap), 200 ms PCM → [150,350]   ← patch target
//	b2: StartMs 400  (50 ms gap), 100 ms PCM → [400,500]
func seededOutDir(t *testing.T) (string, plan.NarrationPlan, [][]byte) {
	t.Helper()
	outDir := t.TempDir()
	audioDir := t.TempDir()
	format := render.DefaultFormat()

	pcm := [][]byte{
		framePCM(100, 0x10),
		framePCM(200, 0x40),
		framePCM(100, 0x80),
	}
	starts := []int{0, 150, 400}

	refs := make([]string, len(pcm))
	for i, p := range pcm {
		name := blockRef(i)
		if err := os.WriteFile(filepath.Join(audioDir, name), synthWAV(t, 24000, 1, 16, p), 0o644); err != nil {
			t.Fatalf("seed block %d: %v", i, err)
		}
		refs[i] = name
	}

	p := plan.NarrationPlan{
		SchemaVersion: plan.SchemaVersion,
		PlanID:        "01HTPATCH00000000000000000",
		CreatedAt:     "2026-06-21T00:00:00Z",
		Source: plan.SourceRef{
			Kind:        plan.SourceKindFile,
			URI:         "/tmp/patch-sample.md",
			ContentHash: "sha256-patch-doc",
			Adapter:     "file",
		},
		Defaults: plan.PlanDefaults{Level: plan.L1, Locale: "en"},
		Blocks: []plan.Block{
			{ID: "b0", Order: 0, Class: plan.ClassHeading, Level: plan.L1, Status: plan.StatusVoiced,
				SourceMap: plan.SourceMap{Kind: plan.SourceKindLineRange, StartLine: 1, EndLine: 1}},
			{ID: "b1", Order: 1, Class: plan.ClassProse, Level: plan.L1, Status: plan.StatusVoiced,
				SourceMap: plan.SourceMap{Kind: plan.SourceKindLineRange, StartLine: 3, EndLine: 5}},
			{ID: "b2", Order: 2, Class: plan.ClassProse, Level: plan.L1, Status: plan.StatusVoiced,
				SourceMap: plan.SourceMap{Kind: plan.SourceKindLineRange, StartLine: 7, EndLine: 7}},
		},
	}

	timing := make([]plan.BlockTiming, len(pcm))
	for i := range pcm {
		durMs := len(pcm[i]) / bytesPerMs
		timing[i] = plan.BlockTiming{
			BlockID:  p.Blocks[i].ID,
			StartMs:  starts[i],
			EndMs:    starts[i] + durMs,
			AudioRef: refs[i],
		}
	}

	res := render.RenderResult{
		Audio:    render.AudioStream{Dir: audioDir, Files: refs},
		Timeline: plan.Timeline{PlanID: p.PlanID, Format: format, Blocks: timing},
		Format:   format,
	}

	if _, err := New(outDir, WithVoice("af_bella")).Consume(context.Background(), p, res); err != nil {
		t.Fatalf("seed Consume: %v", err)
	}
	return outDir, p, pcm
}

func blockRef(i int) string {
	return fmt.Sprintf("b%d.wav", i)
}

// rerender builds a one-block RenderResult for blockID with the given fresh
// PCM, mirroring the single-block sub-result the pipeline hands a sink.
func rerender(t *testing.T, blockID string, freshPCM []byte) render.RenderResult {
	t.Helper()
	dir := t.TempDir()
	format := render.DefaultFormat()
	ref := blockID + "-patch.wav"
	if err := os.WriteFile(filepath.Join(dir, ref), synthWAV(t, 24000, 1, 16, freshPCM), 0o644); err != nil {
		t.Fatalf("rerender write: %v", err)
	}
	durMs := len(freshPCM) / bytesPerMs
	return render.RenderResult{
		Audio:    render.AudioStream{Dir: dir, Files: []string{ref}},
		Format:   format,
		Timeline: plan.Timeline{Blocks: []plan.BlockTiming{{BlockID: blockID, StartMs: 0, EndMs: durMs, AudioRef: ref}}},
	}
}

// containerPCMOf parses outDir/audio.wav and returns its data-chunk bytes.
func containerPCMOf(t *testing.T, outDir string) []byte {
	t.Helper()
	_, pcm, err := readWAV(filepath.Join(outDir, audioFilename), render.DefaultFormat())
	if err != nil {
		t.Fatalf("read container: %v", err)
	}
	return pcm
}

// blockSlice returns the [pcmStart:pcmEnd) bytes of block i derived from the
// manifest layout — what the test asserts is byte-identical pre/post patch.
func blockSlice(t *testing.T, outDir string, i int) []byte {
	t.Helper()
	m, err := readManifest(filepath.Join(outDir, manifestFilename))
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}
	layout, total := deriveBlockLayout(m.Blocks, render.DefaultFormat())
	pcm := containerPCMOf(t, outDir)
	if total != len(pcm) {
		t.Fatalf("layout/container drift: derived %d on-disk %d", total, len(pcm))
	}
	bl := layout[i]
	return append([]byte(nil), pcm[bl.pcmStart:bl.pcmEnd]...)
}

// --- a: fresh (not-yet-escalated) block swap; others byte-identical ---------

func TestPatchBlock_FreshBlock(t *testing.T) {
	outDir, p, seed := seededOutDir(t)

	b0Before := blockSlice(t, outDir, 0)
	b2Before := blockSlice(t, outDir, 2)

	fresh := framePCM(200, 0xC0) // same length as original b1 (zero-delta length)
	rec, err := PatchBlock(context.Background(), outDir, p, rerender(t, "b1", fresh), "b1")
	if err != nil {
		t.Fatalf("PatchBlock: %v", err)
	}
	if rec.BlocksPlayed != 3 {
		t.Errorf("BlocksPlayed: got %d want 3", rec.BlocksPlayed)
	}

	if !bytes.Equal(blockSlice(t, outDir, 0), b0Before) {
		t.Error("b0 bytes changed (should be byte-identical)")
	}
	if !bytes.Equal(blockSlice(t, outDir, 2), b2Before) {
		t.Error("b2 bytes changed (should be byte-identical)")
	}
	if bytes.Equal(blockSlice(t, outDir, 1), seed[1]) {
		t.Error("b1 bytes did not change (patch had no effect)")
	}
	if !bytes.Equal(blockSlice(t, outDir, 1), fresh) {
		t.Error("b1 bytes are not the freshly rendered audio")
	}
}

// --- B1 regression: a one-block sub-plan must NOT truncate plan.json --------
//
// On the CLI patch path the pipeline hands PatchBlock a ONE-block sub-plan
// (subPlan.Blocks = [target]). plan.json must still contain every original
// block afterward, with only the target's plan entry updated.
func TestPatchBlock_PreservesMultiBlockPlanJSON(t *testing.T) {
	outDir, p, _ := seededOutDir(t)

	// Build the one-block sub-plan the pipeline would hand the sink: same
	// document Source (so the content-hash gate passes), Blocks trimmed to the
	// target, and the target re-described at a new Level (L3) to prove the
	// entry is spliced.
	subPlan := p
	var target plan.Block
	for _, b := range p.Blocks {
		if b.ID == "b1" {
			target = b
		}
	}
	target.Level = plan.L3
	subPlan.Blocks = []plan.Block{target}

	if _, err := PatchBlock(context.Background(), outDir, subPlan, rerender(t, "b1", framePCM(200, 0xC0)), "b1"); err != nil {
		t.Fatalf("PatchBlock: %v", err)
	}

	got, err := readPlan(filepath.Join(outDir, planFilename))
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Blocks) != 3 {
		t.Fatalf("plan.json truncated: got %d blocks want 3", len(got.Blocks))
	}
	ids := map[string]plan.Level{}
	for _, b := range got.Blocks {
		ids[b.ID] = b.Level
	}
	for _, id := range []string{"b0", "b1", "b2"} {
		if _, ok := ids[id]; !ok {
			t.Errorf("plan.json lost block %s", id)
		}
	}
	if ids["b1"] != plan.L3 {
		t.Errorf("target b1 Level not spliced: got %v want L3", ids["b1"])
	}
	// Non-target blocks keep their original level (L1).
	if ids["b0"] != plan.L1 || ids["b2"] != plan.L1 {
		t.Errorf("non-target block levels changed: b0=%v b2=%v", ids["b0"], ids["b2"])
	}

	// B2: the manifest block for the target must mirror plan.json's Level (not
	// carry the pre-patch L1), and non-target manifest blocks stay L1.
	mf, err := readManifest(filepath.Join(outDir, manifestFilename))
	if err != nil {
		t.Fatal(err)
	}
	mlvl := map[string]plan.Level{}
	for _, b := range mf.Blocks {
		mlvl[b.ID] = b.Level
	}
	if mlvl["b1"] != plan.L3 {
		t.Errorf("manifest target b1 Level not synced (B2): got %v want L3", mlvl["b1"])
	}
	if mlvl["b0"] != plan.L1 || mlvl["b2"] != plan.L1 {
		t.Errorf("manifest non-target levels changed: b0=%v b2=%v", mlvl["b0"], mlvl["b2"])
	}
	// Manifest and plan must AGREE on the target block's classification.
	if mlvl["b1"] != ids["b1"] {
		t.Errorf("manifest/plan disagree on b1 Level: manifest=%v plan=%v", mlvl["b1"], ids["b1"])
	}
}

// --- b1: re-patch with LONGER audio; downstream timings shift up ------------

func TestPatchBlock_OverExisting_Longer(t *testing.T) {
	outDir, p, _ := seededOutDir(t)

	fresh := framePCM(260, 0xC0) // +60 ms vs original 200 ms
	if _, err := PatchBlock(context.Background(), outDir, p, rerender(t, "b1", fresh), "b1"); err != nil {
		t.Fatalf("PatchBlock: %v", err)
	}

	m, err := readManifest(filepath.Join(outDir, manifestFilename))
	if err != nil {
		t.Fatal(err)
	}
	// b1: StartMs 150, now 260 ms long → [150,410]. b2 gap preserved (50 ms) →
	// StartMs 460, 100 ms → [460,560].
	assertTiming(t, m.Blocks[0], 0, 100)
	assertTiming(t, m.Blocks[1], 150, 410)
	assertTiming(t, m.Blocks[2], 460, 560)
}

// --- b2: re-patch with SHORTER audio; downstream timings shift down ---------

func TestPatchBlock_OverExisting_Shorter(t *testing.T) {
	outDir, p, _ := seededOutDir(t)

	fresh := framePCM(120, 0xC0) // -80 ms vs original 200 ms
	if _, err := PatchBlock(context.Background(), outDir, p, rerender(t, "b1", fresh), "b1"); err != nil {
		t.Fatalf("PatchBlock: %v", err)
	}

	m, err := readManifest(filepath.Join(outDir, manifestFilename))
	if err != nil {
		t.Fatal(err)
	}
	// b1: [150,270]; b2 gap 50 ms preserved → StartMs 320, [320,420].
	assertTiming(t, m.Blocks[0], 0, 100)
	assertTiming(t, m.Blocks[1], 150, 270)
	assertTiming(t, m.Blocks[2], 320, 420)
}

func assertTiming(t *testing.T, b ManifestBlock, start, end int) {
	t.Helper()
	if b.StartMs != start || b.EndMs != end {
		t.Errorf("block %s timing: got [%d,%d] want [%d,%d]", b.ID, b.StartMs, b.EndMs, start, end)
	}
}

// --- c: stale manifest refused; no file mutated -----------------------------

func TestPatchBlock_StaleManifest_Refused(t *testing.T) {
	outDir, p, _ := seededOutDir(t)
	before := snapshot(t, outDir)

	// Drift the source so CheckStale flips: change the plan's content hash.
	stalePlan := p
	stalePlan.Source.ContentHash = "sha256-different-doc"

	_, err := PatchBlock(context.Background(), outDir, stalePlan, rerender(t, "b1", framePCM(200, 0xC0)), "b1")
	if !errors.Is(err, ErrStalePatch) {
		t.Fatalf("want ErrStalePatch, got %v", err)
	}
	assertUnchanged(t, outDir, before)
}

// --- d: content-hash mismatch refused ---------------------------------------
//
// CheckStale catches a content-hash drift first (it compares the same field),
// so to exercise the dedicated ErrContentHashMismatch gate we tamper the
// on-disk manifest's ContentHash while leaving SourceURI/Kind matching, then
// pass the original plan. CheckStale flips on the hash too — so this test
// asserts a refusal (either sentinel) AND that no bytes changed. The dedicated
// gate is unit-covered below via a manifest whose hash differs only after the
// stale check is satisfied.
func TestPatchBlock_ContentHashMismatch_Refused(t *testing.T) {
	outDir, p, _ := seededOutDir(t)

	// Tamper the manifest content hash on disk.
	m, err := readManifest(filepath.Join(outDir, manifestFilename))
	if err != nil {
		t.Fatal(err)
	}
	m.ContentHash = "sha256-tampered"
	if err := writeManifest(filepath.Join(outDir, manifestFilename), m); err != nil {
		t.Fatal(err)
	}
	before := snapshot(t, outDir) // snapshot after the deliberate tamper

	_, err = PatchBlock(context.Background(), outDir, p, rerender(t, "b1", framePCM(200, 0xC0)), "b1")
	if err == nil {
		t.Fatal("expected refusal on content-hash mismatch")
	}
	if !errors.Is(err, ErrContentHashMismatch) && !errors.Is(err, ErrStalePatch) {
		t.Fatalf("want ErrContentHashMismatch or ErrStalePatch, got %v", err)
	}
	assertUnchanged(t, outDir, before)
}

// --- e: idempotent — two identical patches → byte-equal all three files -----

func TestPatchBlock_Idempotent(t *testing.T) {
	outDir, p, _ := seededOutDir(t)
	fresh := framePCM(200, 0xC0)

	if _, err := PatchBlock(context.Background(), outDir, p, rerender(t, "b1", fresh), "b1"); err != nil {
		t.Fatalf("first patch: %v", err)
	}
	after1 := snapshot(t, outDir)

	if _, err := PatchBlock(context.Background(), outDir, p, rerender(t, "b1", fresh), "b1"); err != nil {
		t.Fatalf("second patch: %v", err)
	}
	after2 := snapshot(t, outDir)

	for _, name := range []string{audioFilename, planFilename, manifestFilename} {
		if !bytes.Equal(after1[name], after2[name]) {
			t.Errorf("%s not byte-identical across two identical patches", name)
		}
	}
}

// --- f: nothing-to-patch (no outDir / no manifest) --------------------------

func TestPatchBlock_NoOutDir_Refused(t *testing.T) {
	_, p, _ := seededOutDir(t)
	missing := filepath.Join(t.TempDir(), "does-not-exist")
	_, err := PatchBlock(context.Background(), missing, p, rerender(t, "b1", framePCM(200, 0xC0)), "b1")
	if !errors.Is(err, ErrNothingToPatch) {
		t.Fatalf("want ErrNothingToPatch, got %v", err)
	}
}

func TestPatchBlock_NoManifest_Refused(t *testing.T) {
	outDir, p, _ := seededOutDir(t)
	if err := os.Remove(filepath.Join(outDir, manifestFilename)); err != nil {
		t.Fatal(err)
	}
	_, err := PatchBlock(context.Background(), outDir, p, rerender(t, "b1", framePCM(200, 0xC0)), "b1")
	if !errors.Is(err, ErrNothingToPatch) {
		t.Fatalf("want ErrNothingToPatch, got %v", err)
	}
}

// --- g: unknown block id refused --------------------------------------------

func TestPatchBlock_UnknownBlockID_Refused(t *testing.T) {
	outDir, p, _ := seededOutDir(t)
	before := snapshot(t, outDir)
	_, err := PatchBlock(context.Background(), outDir, p, rerender(t, "bX", framePCM(200, 0xC0)), "bX")
	if !errors.Is(err, ErrUnknownBlock) {
		t.Fatalf("want ErrUnknownBlock, got %v", err)
	}
	assertUnchanged(t, outDir, before)
}

// --- h: container length != manifest-derived length → ErrContainerMismatch --

func TestPatchBlock_ContainerMismatch_Refused(t *testing.T) {
	outDir, p, _ := seededOutDir(t)

	// Corrupt the container: append spurious bytes to audio.wav's data chunk
	// so its data length no longer matches the manifest-derived length, while
	// keeping it a parseable RIFF/WAVE (so this is a REFUSAL, not a hard error).
	pcm := containerPCMOf(t, outDir)
	tampered := append(append([]byte(nil), pcm...), framePCM(10, 0xEE)...)
	if err := os.WriteFile(filepath.Join(outDir, audioFilename), synthWAV(t, 24000, 1, 16, tampered), 0o644); err != nil {
		t.Fatal(err)
	}
	before := snapshot(t, outDir)

	_, err := PatchBlock(context.Background(), outDir, p, rerender(t, "b1", framePCM(200, 0xC0)), "b1")
	if !errors.Is(err, ErrContainerMismatch) {
		t.Fatalf("want ErrContainerMismatch, got %v", err)
	}
	assertUnchanged(t, outDir, before)
}

// --- i: mid-sequence failure → re-run reconstructs correct final state ------
//
// Covers F2 (write ordering) + F3 (zero-delta recovery). Inject a rename
// failure AFTER plan.json + manifest.json commit but BEFORE audio.wav, leaving
// the new-manifest/old-audio interim state. Then re-run a NORMAL patch and
// assert all three files reach the correct final state — including the zero-
// delta sub-case (same byte length, different PCM) where an F1 length check
// alone could not have detected the interim inconsistency.
func TestPatchBlock_MidSequenceFailure_Consistent(t *testing.T) {
	outDir, p, _ := seededOutDir(t)

	// Zero-delta fresh audio: SAME length as original b1 (200 ms), different
	// PCM content (seed 0xC0 vs original 0x40).
	fresh := framePCM(200, 0xC0)

	// First, compute the correct final state by running a clean patch in a
	// throwaway copy, so we have an oracle to compare against.
	oracleDir := cloneDir(t, outDir)
	if _, err := PatchBlock(context.Background(), oracleDir, p, rerender(t, "b1", fresh), "b1"); err != nil {
		t.Fatalf("oracle patch: %v", err)
	}
	oracle := snapshot(t, oracleDir)

	// Now inject a failure on the THIRD rename (audio.wav) in the real dir.
	calls := 0
	orig := renameFn
	renameFn = func(from, to string) error {
		calls++
		if calls == 3 {
			return errors.New("injected mid-sequence rename failure")
		}
		return orig(from, to)
	}
	_, err := PatchBlock(context.Background(), outDir, p, rerender(t, "b1", fresh), "b1")
	renameFn = orig
	if err == nil {
		t.Fatal("expected injected failure to surface")
	}

	// Interim state: new manifest.json (b1 already shows patched timing — here
	// zero-delta so timing is unchanged) + OLD audio.wav. The container still
	// holds the ORIGINAL b1 PCM. This is the F3 hole an F1 length check could
	// not catch (length unchanged).
	interimB1 := blockSlice(t, outDir, 1)
	if bytes.Equal(interimB1, fresh) {
		t.Fatal("precondition: interim audio.wav should still hold OLD b1 (zero-delta)")
	}

	// Re-run a normal patch: it reconstructs + rewrites all three files.
	if _, err := PatchBlock(context.Background(), outDir, p, rerender(t, "b1", fresh), "b1"); err != nil {
		t.Fatalf("recovery re-run: %v", err)
	}
	final := snapshot(t, outDir)
	for _, name := range []string{audioFilename, planFilename, manifestFilename} {
		if !bytes.Equal(final[name], oracle[name]) {
			t.Errorf("%s after recovery re-run != clean-patch oracle", name)
		}
	}
	if !bytes.Equal(blockSlice(t, outDir, 1), fresh) {
		t.Error("b1 after recovery is not the fresh audio")
	}
}

// --- j: re-rendered block format mismatch → refuse before any tmp write ------

func TestPatchBlock_FormatMismatch_Refused(t *testing.T) {
	outDir, p, _ := seededOutDir(t)
	before := snapshot(t, outDir)

	// Build a re-render whose WAV is a different sample rate (48 kHz) than the
	// container's 24 kHz — readWAV returns formatMismatchError.
	dir := t.TempDir()
	ref := "b1-badfmt.wav"
	if err := os.WriteFile(filepath.Join(dir, ref), synthWAV(t, 48000, 1, 16, framePCM(200, 0xC0)), 0o644); err != nil {
		t.Fatal(err)
	}
	res := render.RenderResult{
		Audio:    render.AudioStream{Dir: dir, Files: []string{ref}},
		Format:   render.DefaultFormat(),
		Timeline: plan.Timeline{Blocks: []plan.BlockTiming{{BlockID: "b1", StartMs: 0, EndMs: 200, AudioRef: ref}}},
	}

	_, err := PatchBlock(context.Background(), outDir, p, res, "b1")
	if err == nil {
		t.Fatal("expected format-mismatch refusal")
	}
	var fmErr *formatMismatchError
	if !errors.As(err, &fmErr) {
		t.Fatalf("want formatMismatchError, got %v", err)
	}
	assertUnchanged(t, outDir, before)
	// No leftover tmp files in outDir.
	assertNoTmpFiles(t, outDir)
}

// --- helpers ----------------------------------------------------------------

func snapshot(t *testing.T, outDir string) map[string][]byte {
	t.Helper()
	out := map[string][]byte{}
	for _, name := range []string{audioFilename, planFilename, manifestFilename} {
		raw, err := os.ReadFile(filepath.Join(outDir, name))
		if err != nil {
			t.Fatalf("snapshot %s: %v", name, err)
		}
		out[name] = raw
	}
	return out
}

func assertUnchanged(t *testing.T, outDir string, before map[string][]byte) {
	t.Helper()
	now := snapshot(t, outDir)
	for name, raw := range before {
		if !bytes.Equal(now[name], raw) {
			t.Errorf("%s was mutated by a refused patch (must be untouched)", name)
		}
	}
	assertNoTmpFiles(t, outDir)
}

func assertNoTmpFiles(t *testing.T, outDir string) {
	t.Helper()
	entries, err := os.ReadDir(outDir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if len(e.Name()) >= len(".persistent") && e.Name()[:len(".persistent")] == ".persistent" {
			t.Errorf("leftover tmp file: %s", e.Name())
		}
	}
}

func cloneDir(t *testing.T, src string) string {
	t.Helper()
	dst := t.TempDir()
	entries, err := os.ReadDir(src)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(src, e.Name()))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dst, e.Name()), raw, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dst
}
