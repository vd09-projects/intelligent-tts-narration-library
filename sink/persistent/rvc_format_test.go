package persistent

// rvc_format_test.go — #146 Phase 2 (test-only): prove the persistent-sink
// family round-trips a 40 kHz RVC container when the composition root supplies
// WithExpectedFormat(rvc.OutputFormat()), and that WithVoice(<slug>) stamps the
// RVC character slug into manifest.voice (Decision D6). No production change and
// no committed .wav — the 40 kHz fixture is synthesized in code, frame-aligned
// to whole milliseconds.

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/vd09-projects/intelligent-tts-narration-library/plan"
	"github.com/vd09-projects/intelligent-tts-narration-library/render"
	"github.com/vd09-projects/intelligent-tts-narration-library/render/rvc"
)

// 40 kHz mono s16le → 40000 * 1 * 16/8 = 80000 bytes/sec → 80 bytes/ms. PCM byte
// counts that are multiples of 80 keep every block's duration a whole number of
// ms, so silenceBytes / deriveBlockLayout round-trip the container exactly at
// 40 kHz (the "offsets stay exact" claim). Distinct from patch_test.go's
// bytesPerMs (48, the 24 kHz constant).
const bytesPerMs40 = 80

// framePCM40 returns durMs of deterministic non-zero 40 kHz PCM.
func framePCM40(durMs int, seed byte) []byte {
	b := make([]byte, durMs*bytesPerMs40)
	for i := range b {
		b[i] = seed + byte(i%7)
	}
	return b
}

// seed40kOutDir writes a self-consistent persistent triple for a 3-block doc
// through the real Consume path at 40 kHz, using WithExpectedFormat(40k) +
// WithVoice(voice). Returns the outDir + plan. Block layout (frame-aligned):
//
//	b0: StartMs 0,   100 ms → [0,100]
//	b1: StartMs 150  (50 ms gap), 200 ms → [150,350]   ← patch target
//	b2: StartMs 400  (50 ms gap), 100 ms → [400,500]
func seed40kOutDir(t *testing.T, voice string) (string, plan.NarrationPlan) {
	t.Helper()
	outDir := t.TempDir()
	audioDir := t.TempDir()
	format := rvc.OutputFormat()

	pcm := [][]byte{
		framePCM40(100, 0x11),
		framePCM40(200, 0x44),
		framePCM40(100, 0x88),
	}
	starts := []int{0, 150, 400}

	refs := make([]string, len(pcm))
	for i, pc := range pcm {
		name := blockRef(i)
		if err := os.WriteFile(filepath.Join(audioDir, name), synthWAV(t, 40000, 1, 16, pc), 0o644); err != nil {
			t.Fatalf("seed block %d: %v", i, err)
		}
		refs[i] = name
	}

	p := plan.NarrationPlan{
		SchemaVersion: plan.SchemaVersion,
		PlanID:        "01HTRVC0000000000000000000",
		CreatedAt:     "2026-07-22T00:00:00Z",
		Source: plan.SourceRef{
			Kind:        plan.SourceKindFile,
			URI:         "/tmp/rvc-sample.md",
			ContentHash: "sha256-rvc-doc",
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
		durMs := len(pcm[i]) / bytesPerMs40
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

	if _, err := New(outDir, WithExpectedFormat(format), WithVoice(voice)).Consume(context.Background(), p, res); err != nil {
		t.Fatalf("seed 40k Consume: %v", err)
	}
	return outDir, p
}

// rerender40 builds a one-block 40 kHz RenderResult for blockID, mirroring the
// single-block sub-result the pipeline hands a sink on the escalation path.
func rerender40(t *testing.T, blockID string, freshPCM []byte) render.RenderResult {
	t.Helper()
	dir := t.TempDir()
	format := rvc.OutputFormat()
	ref := blockID + "-patch.wav"
	if err := os.WriteFile(filepath.Join(dir, ref), synthWAV(t, 40000, 1, 16, freshPCM), 0o644); err != nil {
		t.Fatalf("rerender40 write: %v", err)
	}
	durMs := len(freshPCM) / bytesPerMs40
	return render.RenderResult{
		Audio:    render.AudioStream{Dir: dir, Files: []string{ref}},
		Format:   format,
		Timeline: plan.Timeline{Blocks: []plan.BlockTiming{{BlockID: blockID, StartMs: 0, EndMs: durMs, AudioRef: ref}}},
	}
}

// Consume with WithExpectedFormat(40k) accepts a 40 kHz container, records
// 40 kHz in the manifest, keeps the ms<->byte offsets exact, and stamps the RVC
// character slug into manifest.voice (D6).
func TestConsume_40kRoundTrip_RecordsRVCVoice(t *testing.T) {
	outDir, _ := seed40kOutDir(t, "cool-jahns")

	m, err := readManifest(filepath.Join(outDir, manifestFilename))
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}

	// (i) manifest records the 40 kHz format, not Kokoro's 24 kHz.
	if m.AudioFormat != rvc.OutputFormat() {
		t.Errorf("manifest AudioFormat = %+v, want %+v", m.AudioFormat, rvc.OutputFormat())
	}

	// (ii) D6 — the RVC character slug the user asked to hear, not af_bella.
	if m.Voice != "cool-jahns" {
		t.Errorf("manifest Voice = %q, want %q (D6: RVC slug, not the Kokoro source)", m.Voice, "cool-jahns")
	}

	// Offsets stay exact at 40 kHz: the manifest-derived byte layout must equal
	// the on-disk container length (deriveBlockLayout uses SampleRate=40000).
	_, derived := deriveBlockLayout(m.Blocks, rvc.OutputFormat())
	_, containerPCM, err := readWAV(filepath.Join(outDir, audioFilename), rvc.OutputFormat())
	if err != nil {
		t.Fatalf("read container at 40k: %v", err)
	}
	if derived != len(containerPCM) {
		t.Errorf("40k layout/container drift: manifest-derived %d != on-disk %d", derived, len(containerPCM))
	}

	// Whole-ms block spans survive the round-trip.
	wantSpans := [][2]int{{0, 100}, {150, 350}, {400, 500}}
	if len(m.Blocks) != len(wantSpans) {
		t.Fatalf("manifest has %d blocks, want %d", len(m.Blocks), len(wantSpans))
	}
	for i, w := range wantSpans {
		if m.Blocks[i].StartMs != w[0] || m.Blocks[i].EndMs != w[1] {
			t.Errorf("block %d span = [%d,%d], want [%d,%d]", i, m.Blocks[i].StartMs, m.Blocks[i].EndMs, w[0], w[1])
		}
	}
}

// PatchBlock with WithExpectedFormat(40k) + WithVoice(slug) accepts a 40 kHz
// re-render, recomputes downstream timing at 40 kHz, and PRESERVES the RVC slug
// in manifest.voice (D6 patch-path lock).
func TestPatchBlock_40k_PreservesRVCVoice(t *testing.T) {
	outDir, p := seed40kOutDir(t, "cool-jahns")

	// Re-render b1 longer (240 ms) so downstream b2 must shift.
	res := rerender40(t, "b1", framePCM40(240, 0x55))
	if _, err := PatchBlock(context.Background(), outDir, p, res, "b1",
		WithExpectedFormat(rvc.OutputFormat()), WithVoice("cool-jahns")); err != nil {
		t.Fatalf("PatchBlock 40k: %v", err)
	}

	m, err := readManifest(filepath.Join(outDir, manifestFilename))
	if err != nil {
		t.Fatalf("read manifest post-patch: %v", err)
	}

	if m.AudioFormat != rvc.OutputFormat() {
		t.Errorf("post-patch AudioFormat = %+v, want %+v", m.AudioFormat, rvc.OutputFormat())
	}
	if m.Voice != "cool-jahns" {
		t.Errorf("post-patch Voice = %q, want %q (D6: slug preserved across patch)", m.Voice, "cool-jahns")
	}

	// b1 grew to 240 ms → [150,390]; b2 shifts by +40 ms → [440,540]; b0 unchanged.
	wantSpans := [][2]int{{0, 100}, {150, 390}, {440, 540}}
	for i, w := range wantSpans {
		if m.Blocks[i].StartMs != w[0] || m.Blocks[i].EndMs != w[1] {
			t.Errorf("post-patch block %d span = [%d,%d], want [%d,%d]", i, m.Blocks[i].StartMs, m.Blocks[i].EndMs, w[0], w[1])
		}
	}

	// Container still round-trips exactly at 40 kHz after the patch.
	_, derived := deriveBlockLayout(m.Blocks, rvc.OutputFormat())
	_, containerPCM, err := readWAV(filepath.Join(outDir, audioFilename), rvc.OutputFormat())
	if err != nil {
		t.Fatalf("read patched container at 40k: %v", err)
	}
	if derived != len(containerPCM) {
		t.Errorf("post-patch 40k layout/container drift: derived %d != on-disk %d", derived, len(containerPCM))
	}
}

// Negative guard: a 40 kHz container against the DEFAULT (24 kHz) expected
// format must still be rejected with ErrFormatMismatch. This locks in that the
// sink's strict format-validation contract is intact (D1 rejected alternative —
// the sink does not silently accept whatever rate the renderer produces).
func TestConsume_40kAgainstDefaultFormat_Rejected(t *testing.T) {
	outDir := t.TempDir()
	audioDir := t.TempDir()

	if err := os.WriteFile(filepath.Join(audioDir, "b0.wav"), synthWAV(t, 40000, 1, 16, framePCM40(100, 0x11)), 0o644); err != nil {
		t.Fatalf("write 40k block: %v", err)
	}

	p := plan.NarrationPlan{
		SchemaVersion: plan.SchemaVersion,
		PlanID:        "01HTRVC0000000000000000001",
		CreatedAt:     "2026-07-22T00:00:00Z",
		Source:        plan.SourceRef{Kind: plan.SourceKindFile, URI: "/tmp/x.md", ContentHash: "h", Adapter: "file"},
		Defaults:      plan.PlanDefaults{Level: plan.L1, Locale: "en"},
		Blocks: []plan.Block{
			{ID: "b0", Order: 0, Class: plan.ClassProse, Level: plan.L1, Status: plan.StatusVoiced,
				SourceMap: plan.SourceMap{Kind: plan.SourceKindLineRange, StartLine: 1, EndLine: 1}},
		},
	}
	res := render.RenderResult{
		Audio:    render.AudioStream{Dir: audioDir, Files: []string{"b0.wav"}},
		Timeline: plan.Timeline{PlanID: p.PlanID, Format: rvc.OutputFormat(), Blocks: []plan.BlockTiming{{BlockID: "b0", StartMs: 0, EndMs: 100, AudioRef: "b0.wav"}}},
		Format:   rvc.OutputFormat(),
	}

	// New(outDir) with NO WithExpectedFormat → default 24 kHz validation.
	_, err := New(outDir).Consume(context.Background(), p, res)
	if !errors.Is(err, ErrFormatMismatch) {
		t.Fatalf("want ErrFormatMismatch when a 40 kHz WAV meets the default 24 kHz expected format, got %v", err)
	}
}
