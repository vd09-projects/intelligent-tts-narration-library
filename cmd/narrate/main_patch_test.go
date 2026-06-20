package main

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/vd09-projects/intelligent-tts-narration-library/pipeline"
	"github.com/vd09-projects/intelligent-tts-narration-library/plan"
	"github.com/vd09-projects/intelligent-tts-narration-library/render"
	"github.com/vd09-projects/intelligent-tts-narration-library/sink"
	"github.com/vd09-projects/intelligent-tts-narration-library/sink/persistent"
)

// --- issue #28 Phase 3: cmd/narrate exit-code tests (test row k) ------------
//
// These exercise the --block × --sink=persistent patch path through runNarrate
// end-to-end against a REAL on-disk persistent outDir (seeded via the
// persistent sink), with the render/planner stage stubbed so no Kokoro spawns.
// The stub drives the capturing sink exactly as the pipeline single-block path
// would (one-block sub-plan + sub-result), so runNarrate reaches the real
// persistent.PatchBlock and its refusal taxonomy.

const patchBytesPerMs = 48 // 24 kHz mono s16le

func patchFramePCM(durMs int, seed byte) []byte {
	b := make([]byte, durMs*patchBytesPerMs)
	for i := range b {
		b[i] = seed + byte(i%5)
	}
	return b
}

func synthWAVBytes(sampleRate, channels, bits int, pcm []byte) []byte {
	blockAlign := uint16(channels * bits / 8)
	byteRate := uint32(sampleRate * channels * bits / 8)
	dataSize := uint32(len(pcm))
	buf := make([]byte, 0, 44+len(pcm))
	buf = append(buf, 'R', 'I', 'F', 'F')
	buf = binary.LittleEndian.AppendUint32(buf, 36+dataSize)
	buf = append(buf, 'W', 'A', 'V', 'E', 'f', 'm', 't', ' ')
	buf = binary.LittleEndian.AppendUint32(buf, 16)
	buf = binary.LittleEndian.AppendUint16(buf, 0x0001)
	buf = binary.LittleEndian.AppendUint16(buf, uint16(channels))
	buf = binary.LittleEndian.AppendUint32(buf, uint32(sampleRate))
	buf = binary.LittleEndian.AppendUint32(buf, byteRate)
	buf = binary.LittleEndian.AppendUint16(buf, blockAlign)
	buf = binary.LittleEndian.AppendUint16(buf, uint16(bits))
	buf = append(buf, 'd', 'a', 't', 'a')
	buf = binary.LittleEndian.AppendUint32(buf, dataSize)
	return append(buf, pcm...)
}

// seedPersistentOutDir writes a 2-block persistent triple via the real sink.
func seedPersistentOutDir(t *testing.T) (outDir string, p plan.NarrationPlan) {
	t.Helper()
	outDir = t.TempDir()
	audioDir := t.TempDir()
	format := render.DefaultFormat()

	pcm := [][]byte{patchFramePCM(100, 0x10), patchFramePCM(100, 0x50)}
	starts := []int{0, 150}
	refs := make([]string, 2)
	for i := range pcm {
		refs[i] = fmt.Sprintf("blk%d.wav", i)
		if err := os.WriteFile(filepath.Join(audioDir, refs[i]), synthWAVBytes(24000, 1, 16, pcm[i]), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	p = plan.NarrationPlan{
		SchemaVersion: plan.SchemaVersion,
		PlanID:        "01HTCMDPATCH0000000000000",
		CreatedAt:     "2026-06-21T00:00:00Z",
		Source: plan.SourceRef{
			Kind: plan.SourceKindFile, URI: "/tmp/cmd-patch.md",
			ContentHash: "sha256-cmd-doc", Adapter: "file",
		},
		Defaults: plan.PlanDefaults{Level: plan.L1, Locale: "en"},
		Blocks: []plan.Block{
			{ID: "b0", Order: 0, Class: plan.ClassHeading, Level: plan.L1, Status: plan.StatusVoiced,
				SourceMap: plan.SourceMap{Kind: plan.SourceKindLineRange, StartLine: 1, EndLine: 1}},
			{ID: "b1", Order: 1, Class: plan.ClassProse, Level: plan.L1, Status: plan.StatusVoiced,
				SourceMap: plan.SourceMap{Kind: plan.SourceKindLineRange, StartLine: 3, EndLine: 3}},
		},
	}
	timing := make([]plan.BlockTiming, 2)
	for i := range pcm {
		timing[i] = plan.BlockTiming{BlockID: p.Blocks[i].ID, StartMs: starts[i],
			EndMs: starts[i] + len(pcm[i])/patchBytesPerMs, AudioRef: refs[i]}
	}
	res := render.RenderResult{
		Audio:    render.AudioStream{Dir: audioDir, Files: refs},
		Timeline: plan.Timeline{PlanID: p.PlanID, Format: format, Blocks: timing},
		Format:   format,
	}
	if _, err := persistent.New(outDir, persistent.WithVoice("af_bella")).Consume(context.Background(), p, res); err != nil {
		t.Fatalf("seed: %v", err)
	}
	return outDir, p
}

// withPatchPipeline swaps newPipelineWithSink for a stub that drives the
// supplied capturing sink with a one-block sub-plan/sub-result for blockID,
// using freshPCM as the re-rendered audio. Mirrors the pipeline single-block
// path so runNarrate reaches the real PatchBlock.
func withPatchPipeline(t *testing.T, p plan.NarrationPlan, blockID string, freshPCM []byte, contentHash string) func() {
	t.Helper()
	orig := newPipelineWithSink
	newPipelineWithSink = func(_ string, _ flagSet, s sink.OutputSink) narrator {
		return &patchStub{t: t, sink: s, plan: p, blockID: blockID, freshPCM: freshPCM, docHash: contentHash}
	}
	return func() { newPipelineWithSink = orig }
}

type patchStub struct {
	t        *testing.T
	sink     sink.OutputSink
	plan     plan.NarrationPlan
	blockID  string
	freshPCM []byte
	docHash  string
}

func (ps *patchStub) Narrate(ctx context.Context, _ plan.SourceRef, _ pipeline.NarrateRequest) (pipeline.NarrateResult, error) {
	// Build the one-block sub-plan + sub-result the pipeline would hand a sink.
	dir := ps.t.TempDir()
	ref := ps.blockID + "-fresh.wav"
	if err := os.WriteFile(filepath.Join(dir, ref), synthWAVBytes(24000, 1, 16, ps.freshPCM), 0o644); err != nil {
		ps.t.Fatal(err)
	}
	subPlan := ps.plan
	for _, b := range ps.plan.Blocks {
		if b.ID == ps.blockID {
			subPlan.Blocks = []plan.Block{b}
		}
	}
	subRes := render.RenderResult{
		Audio:  render.AudioStream{Dir: dir, Files: []string{ref}},
		Format: render.DefaultFormat(),
		Timeline: plan.Timeline{Blocks: []plan.BlockTiming{
			{BlockID: ps.blockID, StartMs: 0, EndMs: len(ps.freshPCM) / patchBytesPerMs, AudioRef: ref},
		}},
	}
	rec, err := ps.sink.Consume(ctx, subPlan, subRes)
	return pipeline.NarrateResult{SinkReceipt: rec, DocumentContentHash: ps.docHash}, err
}

// k.0 — success: patch one block → exit 0, all three files present + valid.
func TestRunNarrate_Patch_Success_Exit0(t *testing.T) {
	outDir, p := seedPersistentOutDir(t)
	cleanup := withPatchPipeline(t, p, "b1", patchFramePCM(160, 0xC0), p.Source.ContentHash)
	defer cleanup()

	args := flagSet{File: "/tmp/cmd-patch.md", Level: 2, Sink: "persistent", Gender: "female", Out: outDir, Block: "b1"}
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	if err := runNarrate(context.Background(), args, stdout, stderr); err != nil {
		t.Fatalf("patch should succeed, got %v (exit %d)", err, exitCodeFor(err))
	}
	// manifest now indexes b1 with the patched (160 ms) duration.
	m := mustReadManifest(t, outDir)
	if m.Blocks[1].EndMs-m.Blocks[1].StartMs != 160 {
		t.Errorf("b1 duration not patched: got %d ms", m.Blocks[1].EndMs-m.Blocks[1].StartMs)
	}
	// B1 regression: plan.json must still carry BOTH blocks (the pipeline hands
	// a one-block sub-plan; PatchBlock must not truncate plan.json).
	if len(m.Blocks) != 2 {
		t.Errorf("manifest lost a block: got %d want 2", len(m.Blocks))
	}
	planRaw, err := os.ReadFile(filepath.Join(outDir, "plan.json"))
	if err != nil {
		t.Fatal(err)
	}
	var gotPlan plan.NarrationPlan
	if err := json.Unmarshal(planRaw, &gotPlan); err != nil {
		t.Fatalf("plan.json unmarshal: %v", err)
	}
	if len(gotPlan.Blocks) != 2 {
		t.Errorf("plan.json truncated to %d blocks (B1): want 2 (b0,b1)", len(gotPlan.Blocks))
	}
}

// k.1 — no --out → flag validation, exit 2.
func TestRunNarrate_Patch_NoOut_Exit2(t *testing.T) {
	args := flagSet{File: "/tmp/cmd-patch.md", Level: 2, Sink: "persistent", Gender: "female", Block: "b1"}
	err := runNarrate(context.Background(), args, &bytes.Buffer{}, &bytes.Buffer{})
	if got := exitCodeFor(err); got != 2 {
		t.Fatalf("no --out: exit %d want 2 (err=%v)", got, err)
	}
}

// k.2 — outDir empty / nothing to patch → exit 2.
func TestRunNarrate_Patch_NothingToPatch_Exit2(t *testing.T) {
	empty := t.TempDir() // exists but holds no manifest/audio/plan
	_, p := seedPersistentOutDir(t)
	cleanup := withPatchPipeline(t, p, "b1", patchFramePCM(160, 0xC0), p.Source.ContentHash)
	defer cleanup()

	args := flagSet{File: "/tmp/cmd-patch.md", Level: 2, Sink: "persistent", Gender: "female", Out: empty, Block: "b1"}
	err := runNarrate(context.Background(), args, &bytes.Buffer{}, &bytes.Buffer{})
	if got := exitCodeFor(err); got != 2 {
		t.Fatalf("nothing-to-patch: exit %d want 2 (err=%v)", got, err)
	}
}

// k.3 — unknown block id → exit 2.
func TestRunNarrate_Patch_UnknownBlock_Exit2(t *testing.T) {
	outDir, p := seedPersistentOutDir(t)
	cleanup := withPatchPipeline(t, p, "bZ", patchFramePCM(160, 0xC0), p.Source.ContentHash)
	defer cleanup()

	args := flagSet{File: "/tmp/cmd-patch.md", Level: 2, Sink: "persistent", Gender: "female", Out: outDir, Block: "bZ"}
	err := runNarrate(context.Background(), args, &bytes.Buffer{}, &bytes.Buffer{})
	if got := exitCodeFor(err); got != 2 {
		t.Fatalf("unknown-block: exit %d want 2 (err=%v)", got, err)
	}
}

// k.4 — container/manifest mismatch → exit 2 (refusal, not hard error).
func TestRunNarrate_Patch_ContainerMismatch_Exit2(t *testing.T) {
	outDir, p := seedPersistentOutDir(t)
	// Corrupt audio.wav length while keeping it a parseable WAV.
	corrupt := synthWAVBytes(24000, 1, 16, append(patchFramePCM(250, 0x10), patchFramePCM(5, 0x99)...))
	if err := os.WriteFile(filepath.Join(outDir, "audio.wav"), corrupt, 0o644); err != nil {
		t.Fatal(err)
	}
	cleanup := withPatchPipeline(t, p, "b1", patchFramePCM(160, 0xC0), p.Source.ContentHash)
	defer cleanup()

	args := flagSet{File: "/tmp/cmd-patch.md", Level: 2, Sink: "persistent", Gender: "female", Out: outDir, Block: "b1"}
	err := runNarrate(context.Background(), args, &bytes.Buffer{}, &bytes.Buffer{})
	if got := exitCodeFor(err); got != 2 {
		t.Fatalf("container-mismatch: exit %d want 2 (err=%v)", got, err)
	}
}

// k.5 — corrupt manifest (unreadable) → exit 1 (hard error, not a refusal).
func TestRunNarrate_Patch_CorruptManifest_Exit1(t *testing.T) {
	outDir, p := seedPersistentOutDir(t)
	if err := os.WriteFile(filepath.Join(outDir, "manifest.json"), []byte("{ this is not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	cleanup := withPatchPipeline(t, p, "b1", patchFramePCM(160, 0xC0), p.Source.ContentHash)
	defer cleanup()

	args := flagSet{File: "/tmp/cmd-patch.md", Level: 2, Sink: "persistent", Gender: "female", Out: outDir, Block: "b1"}
	err := runNarrate(context.Background(), args, &bytes.Buffer{}, &bytes.Buffer{})
	if got := exitCodeFor(err); got != 1 {
		t.Fatalf("corrupt-manifest: exit %d want 1 (err=%v)", got, err)
	}
}

func mustReadManifest(t *testing.T, outDir string) persistent.Manifest {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(outDir, "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	var m persistent.Manifest
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("manifest unmarshal: %v", err)
	}
	return m
}
