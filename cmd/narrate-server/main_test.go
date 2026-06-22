package main

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/vd09-projects/intelligent-tts-narration-library/pipeline"
	"github.com/vd09-projects/intelligent-tts-narration-library/plan"
	"github.com/vd09-projects/intelligent-tts-narration-library/render"
	"github.com/vd09-projects/intelligent-tts-narration-library/sink"
	"github.com/vd09-projects/intelligent-tts-narration-library/sink/persistent"
)

// ---------------------------------------------------------------------------
// Stubs / seams. NO Kokoro — the stub Narrator populates the capturing sink the
// handler hands to patchBlock, exactly as the real pipeline would.
// ---------------------------------------------------------------------------

// stubNarrator stands in for *pipeline.Pipeline. Its Narrate populates the
// capturer (the sink the production pipeline would have driven) so the handler
// proceeds to patchBlock without spawning a render subprocess.
type stubNarrator struct {
	capturer *capturingSink
	// blockFor lets the stub seed the captured sub-plan's single block.
	block plan.Block
	err   error
	// block until ctx fires (cancel/timeout test).
	blockUntilCtx bool
	gotReq        pipeline.NarrateRequest
	mu            sync.Mutex
}

func (s *stubNarrator) Narrate(ctx context.Context, _ plan.SourceRef, req pipeline.NarrateRequest) (pipeline.NarrateResult, error) {
	s.mu.Lock()
	s.gotReq = req
	s.mu.Unlock()
	if s.blockUntilCtx {
		<-ctx.Done()
		return pipeline.NarrateResult{}, fmt.Errorf("pipeline: %w", ctx.Err())
	}
	if s.err != nil {
		return pipeline.NarrateResult{}, s.err
	}
	// Populate the capturer as the real pipeline's single-block path would.
	if s.capturer != nil {
		s.capturer.plan = plan.NarrationPlan{Blocks: []plan.Block{s.block}}
		s.capturer.result = render.RenderResult{
			Timeline: plan.Timeline{Blocks: []plan.BlockTiming{{BlockID: s.block.ID}}},
		}
		s.capturer.captured = true
	}
	return pipeline.NarrateResult{}, nil
}

// installSeams swaps the package-level seams for a test and returns a restore
// func. Tests that mutate package vars cannot run t.Parallel().
type seams struct {
	narrate  func(outDir string, args serverArgs, capturer *capturingSink) pipeline.Narrator
	patch    func(ctx context.Context, outDir string, p plan.NarrationPlan, res render.RenderResult, blockID string, opts ...persistent.Option) (sink.SinkReceipt, error)
	manifest func(dir string) (persistent.Manifest, error)
	readPlan func(dir string) (plan.NarrationPlan, error)
}

func installSeams(t *testing.T, s seams) {
	t.Helper()
	origN, origP, origM, origR := newPipeline, patchBlock, readManifest, readPlanFile
	if s.narrate != nil {
		newPipeline = s.narrate
	}
	if s.patch != nil {
		patchBlock = s.patch
	}
	if s.manifest != nil {
		readManifest = s.manifest
	}
	if s.readPlan != nil {
		readPlanFile = s.readPlan
	}
	t.Cleanup(func() {
		newPipeline, patchBlock, readManifest, readPlanFile = origN, origP, origM, origR
	})
}

// goodManifest returns a manifest seam that resolves cleanly for blockID.
func goodManifest(blockID string) func(dir string) (persistent.Manifest, error) {
	return func(_ string) (persistent.Manifest, error) {
		return persistent.Manifest{
			SourceKind:  plan.SourceKindFile,
			SourceURI:   "/tmp/doc.md",
			ContentHash: "hash-abc",
			Blocks: []persistent.ManifestBlock{
				{ID: blockID, Class: plan.ClassProse, Level: plan.L3, Status: plan.StatusVoiced, StartMs: 0, EndMs: 1500, AudioRef: "b1.wav"},
			},
		}, nil
	}
}

// planWithBlock returns a readPlan seam producing a plan whose blockID block has
// the given status (+ optional refusal).
func planWithBlock(blk plan.Block) func(dir string) (plan.NarrationPlan, error) {
	return func(_ string) (plan.NarrationPlan, error) {
		return plan.NarrationPlan{Blocks: []plan.Block{blk}}, nil
	}
}

// stubNarrateSeam wires a stub Narrator that captures into the handler's
// capturer and seeds the captured block.
func stubNarrateSeam(stub *stubNarrator) func(string, serverArgs, *capturingSink) pipeline.Narrator {
	return func(_ string, _ serverArgs, capturer *capturingSink) pipeline.Narrator {
		stub.capturer = capturer
		return stub
	}
}

// noopPatch is a patchBlock seam that succeeds.
func noopPatch(_ context.Context, _ string, _ plan.NarrationPlan, _ render.RenderResult, _ string, _ ...persistent.Option) (sink.SinkReceipt, error) {
	return sink.SinkReceipt{}, nil
}

// narratorFunc adapts a func to the pipeline.Narrator interface so the real-
// PatchBlock tests can supply a bespoke capturer-populating Narrate without a
// full stubNarrator. The capturer is closed over by the narrate seam.
type narratorFunc func(context.Context, plan.SourceRef, pipeline.NarrateRequest) (pipeline.NarrateResult, error)

func (f narratorFunc) Narrate(ctx context.Context, ref plan.SourceRef, req pipeline.NarrateRequest) (pipeline.NarrateResult, error) {
	return f(ctx, ref, req)
}

// serverBytesPerMs — 24 kHz mono s16le = 48 bytes/ms (render.DefaultFormat()).
const serverBytesPerMs = 48

// serverFramePCM builds durMs of deterministic frame-aligned PCM for a real
// PatchBlock fixture (mirrors sink/persistent's framePCM, kept local so the cmd
// test stays independent of the persistent package's _test helpers).
func serverFramePCM(durMs int, seed byte) []byte {
	b := make([]byte, durMs*serverBytesPerMs)
	for i := range b {
		b[i] = seed
	}
	return b
}

// serverSynthWAV wraps PCM in a minimal 24 kHz mono s16le WAV container — the
// format render.DefaultFormat() / persistent expect.
func serverSynthWAV(pcm []byte) []byte {
	const sampleRate, channels, bitsPerSample = 24000, 1, 16
	blockAlign := uint16(channels * bitsPerSample / 8)
	byteRate := uint32(sampleRate * channels * bitsPerSample / 8)
	dataSize := uint32(len(pcm))
	buf := make([]byte, 0, 44+len(pcm))
	buf = append(buf, 'R', 'I', 'F', 'F')
	buf = binary.LittleEndian.AppendUint32(buf, 36+dataSize)
	buf = append(buf, 'W', 'A', 'V', 'E')
	buf = append(buf, 'f', 'm', 't', ' ')
	buf = binary.LittleEndian.AppendUint32(buf, 16)
	buf = binary.LittleEndian.AppendUint16(buf, 1) // PCM
	buf = binary.LittleEndian.AppendUint16(buf, uint16(channels))
	buf = binary.LittleEndian.AppendUint32(buf, uint32(sampleRate))
	buf = binary.LittleEndian.AppendUint32(buf, byteRate)
	buf = binary.LittleEndian.AppendUint16(buf, blockAlign)
	buf = binary.LittleEndian.AppendUint16(buf, uint16(bitsPerSample))
	buf = append(buf, 'd', 'a', 't', 'a')
	buf = binary.LittleEndian.AppendUint32(buf, dataSize)
	return append(buf, pcm...)
}

func writeFileT(t *testing.T, path string, b []byte) {
	t.Helper()
	if err := os.WriteFile(path, b, 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

// seedPersistentTriple writes a COMPLETE, self-consistent persistent triple
// (audio.wav + plan.json + manifest.json) for a 2-block document via the REAL
// persistent.Consume, so the real PatchBlock has a genuine fixture dir to splice
// into. Returns outDir and the document content hash (the patch's Step 3 gate).
// Block b1 (the target) is 200 ms at L1; b0 is 100 ms heading at L1.
func seedPersistentTriple(t *testing.T) (outDir, contentHash, sourceURI string) {
	t.Helper()
	outDir = t.TempDir()
	audioDir := t.TempDir()
	format := render.DefaultFormat()
	contentHash = "sha256-server-patch-doc"
	sourceURI = "/tmp/srv-patch.md"

	pcm := [][]byte{serverFramePCM(100, 0x10), serverFramePCM(200, 0x40)}
	starts := []int{0, 150}
	refs := []string{"b0.wav", "b1.wav"}
	for i, p := range pcm {
		writeFileT(t, filepath.Join(audioDir, refs[i]), serverSynthWAV(p))
	}

	p := plan.NarrationPlan{
		SchemaVersion: plan.SchemaVersion,
		PlanID:        "01HTSRVPATCH0000000000000",
		CreatedAt:     "2026-06-21T00:00:00Z",
		Source:        plan.SourceRef{Kind: plan.SourceKindFile, URI: sourceURI, ContentHash: contentHash, Adapter: "file"},
		Defaults:      plan.PlanDefaults{Level: plan.L1, Locale: "en"},
		Blocks: []plan.Block{
			{ID: "b0", Order: 0, Class: plan.ClassHeading, Level: plan.L1, Status: plan.StatusVoiced,
				SourceMap: plan.SourceMap{Kind: plan.SourceKindLineRange, StartLine: 1, EndLine: 1}},
			{ID: "b1", Order: 1, Class: plan.ClassProse, Level: plan.L1, Status: plan.StatusVoiced,
				SourceMap: plan.SourceMap{Kind: plan.SourceKindLineRange, StartLine: 3, EndLine: 5}},
		},
	}

	timing := make([]plan.BlockTiming, len(pcm))
	for i := range pcm {
		durMs := len(pcm[i]) / serverBytesPerMs
		timing[i] = plan.BlockTiming{BlockID: p.Blocks[i].ID, StartMs: starts[i], EndMs: starts[i] + durMs, AudioRef: refs[i]}
	}
	res := render.RenderResult{
		Audio:    render.AudioStream{Dir: audioDir, Files: refs},
		Timeline: plan.Timeline{PlanID: p.PlanID, Format: format, Blocks: timing},
		Format:   format,
	}
	if _, err := persistent.New(outDir, persistent.WithVoice("af_bella")).Consume(context.Background(), p, res); err != nil {
		t.Fatalf("seed Consume: %v", err)
	}
	return outDir, contentHash, sourceURI
}

// doPost drives one POST /escalate through the full mux and returns the recorder.
func doPost(t *testing.T, args serverArgs, body string, origin string) *httptest.ResponseRecorder {
	t.Helper()
	mux := newMux(args)
	r := httptest.NewRequest(http.MethodPost, "/escalate", strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	if origin != "" {
		r.Header.Set("Origin", origin)
	}
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, r)
	return w
}

func defaultArgs() serverArgs {
	return serverArgs{
		Addr:           "127.0.0.1:8080",
		CORSOrigin:     "http://localhost:5173",
		RequestTimeout: 5 * time.Second,
		Gender:         "female",
	}
}

func decodeErr(t *testing.T, w *httptest.ResponseRecorder) ErrorResponse {
	t.Helper()
	var e ErrorResponse
	if err := json.Unmarshal(w.Body.Bytes(), &e); err != nil {
		t.Fatalf("decode ErrorResponse: %v (body %q)", err, w.Body.String())
	}
	return e
}

func decodeOK(t *testing.T, w *httptest.ResponseRecorder) escalateResponse {
	t.Helper()
	var r escalateResponse
	if err := json.Unmarshal(w.Body.Bytes(), &r); err != nil {
		t.Fatalf("decode escalateResponse: %v (body %q)", err, w.Body.String())
	}
	return r
}

// ---------------------------------------------------------------------------
// Happy path — voiced.
// ---------------------------------------------------------------------------

func TestEscalate_HappyPath_Voiced(t *testing.T) {
	const blockID = "b001"
	stub := &stubNarrator{block: plan.Block{ID: blockID, Status: plan.StatusVoiced, Level: plan.L3}}
	var patchedBlockID string
	installSeams(t, seams{
		narrate: stubNarrateSeam(stub),
		patch: func(_ context.Context, _ string, _ plan.NarrationPlan, _ render.RenderResult, id string, _ ...persistent.Option) (sink.SinkReceipt, error) {
			patchedBlockID = id // spy: assert PatchBlock got exactly the target id.
			return sink.SinkReceipt{}, nil
		},
		manifest: goodManifest(blockID),
		readPlan: planWithBlock(plan.Block{ID: blockID, Status: plan.StatusVoiced, Level: plan.L3}),
	})

	w := doPost(t, defaultArgs(), `{"dir":"/tmp/x","block_id":"b001","level":3}`, "http://localhost:5173")

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body %q)", w.Code, w.Body.String())
	}
	resp := decodeOK(t, w)
	if resp.Block.Status != plan.StatusVoiced {
		t.Errorf("block.status = %q, want voiced", resp.Block.Status)
	}
	if resp.Timing == nil || resp.Timing.AudioRef != "b1.wav" {
		t.Errorf("timing = %+v, want populated with audio_ref b1.wav", resp.Timing)
	}
	if resp.AudioRef != "b1.wav" {
		t.Errorf("audio_ref = %q, want b1.wav", resp.AudioRef)
	}
	if resp.Refusal != nil {
		t.Errorf("refusal should be omitted on voiced, got %+v", resp.Refusal)
	}
	if patchedBlockID != blockID {
		t.Errorf("PatchBlock got blockID %q, want %q", patchedBlockID, blockID)
	}
	if stub.gotReq.LevelOverrides[blockID] != plan.L3 {
		t.Errorf("LevelOverrides[%s] = %v, want L3", blockID, stub.gotReq.LevelOverrides[blockID])
	}
}

// ---------------------------------------------------------------------------
// Degraded vs voiced — player must distinguish.
// ---------------------------------------------------------------------------

func TestEscalate_Degraded(t *testing.T) {
	const blockID = "b001"
	stub := &stubNarrator{block: plan.Block{ID: blockID, Status: plan.StatusDegraded}}
	installSeams(t, seams{
		narrate:  stubNarrateSeam(stub),
		patch:    noopPatch,
		manifest: goodManifest(blockID),
		readPlan: planWithBlock(plan.Block{ID: blockID, Status: plan.StatusDegraded}),
	})

	w := doPost(t, defaultArgs(), `{"dir":"/tmp/x","block_id":"b001","level":3}`, "")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	resp := decodeOK(t, w)
	if resp.Block.Status != plan.StatusDegraded {
		t.Errorf("block.status = %q, want degraded", resp.Block.Status)
	}
	if resp.Timing == nil || resp.AudioRef == "" {
		t.Errorf("degraded block must carry timing + audio_ref, got timing=%+v audio_ref=%q", resp.Timing, resp.AudioRef)
	}
}

// ---------------------------------------------------------------------------
// Refusal-is-data + nullability — 200, populated refusal, timing/audio_ref ABSENT.
// ---------------------------------------------------------------------------

func TestEscalate_Refusal_Is200_WithNullability(t *testing.T) {
	const blockID = "b001"
	refusal := &plan.Refusal{
		Reason:  plan.RefuseNoIntelligence,
		Message: "no intelligence adapter; cannot summarize at L3",
		Spoken:  true,
	}
	stub := &stubNarrator{block: plan.Block{ID: blockID, Status: plan.StatusRefused, Refusal: refusal}}
	installSeams(t, seams{
		narrate:  stubNarrateSeam(stub),
		patch:    noopPatch,
		manifest: goodManifest(blockID),
		readPlan: planWithBlock(plan.Block{ID: blockID, Status: plan.StatusRefused, Refusal: refusal}),
	})

	w := doPost(t, defaultArgs(), `{"dir":"/tmp/x","block_id":"b001","level":3}`, "")
	if w.Code != http.StatusOK {
		t.Fatalf("refusal must be 200 (never 5xx), got %d (body %q)", w.Code, w.Body.String())
	}
	resp := decodeOK(t, w)
	if resp.Block.Status != plan.StatusRefused {
		t.Errorf("block.status = %q, want refused", resp.Block.Status)
	}
	if resp.Refusal == nil || resp.Refusal.Reason != plan.RefuseNoIntelligence {
		t.Errorf("refusal payload missing/wrong: %+v", resp.Refusal)
	}
	// Nullability: timing + audio_ref ABSENT (not present-but-null) in the raw body.
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(w.Body.Bytes(), &raw); err != nil {
		t.Fatalf("raw decode: %v", err)
	}
	if _, ok := raw["timing"]; ok {
		t.Errorf("timing key must be absent on refusal, body=%q", w.Body.String())
	}
	if _, ok := raw["audio_ref"]; ok {
		t.Errorf("audio_ref key must be absent on refusal, body=%q", w.Body.String())
	}
}

// ---------------------------------------------------------------------------
// ErrNothingToPatch — driven through the REAL persistent.PatchBlock (build-
// review B-1). The earlier build injected ErrNothingToPatch through the patch
// seam and asserted a 200 "already-at-level" no-op, but the real PatchBlock has
// NO already-at-level detection: it returns ErrNothingToPatch ONLY for an
// INCOMPLETE persistent-sink dir. These tests drive the genuine dependency.
// ---------------------------------------------------------------------------

// TestEscalate_IncompleteDir_RealPatchBlock_Is4xxSourceNotFound points the
// endpoint at an EMPTY temp dir and lets the default (real) patchBlock seam run.
// The real PatchBlock pre-flight finds manifest.json / audio.wav / plan.json
// missing → ErrNothingToPatch, which the handler maps to a source_not_found 4xx
// (404 here: the missing files surface as fs.ErrNotExist), NOT the old 200→
// readback→500 trap. We seed a manifest seam so the handler gets past the
// manifest read (the real failure we want to exercise is in PatchBlock, not the
// manifest read), but everything from patchBlock onward is the REAL dependency.
func TestEscalate_IncompleteDir_RealPatchBlock_Is4xxSourceNotFound(t *testing.T) {
	const blockID = "b001"
	emptyDir := t.TempDir() // exists but holds none of the three canonical files
	stub := &stubNarrator{block: plan.Block{ID: blockID, Status: plan.StatusVoiced}}
	installSeams(t, seams{
		narrate:  stubNarrateSeam(stub),
		manifest: goodManifest(blockID),
		// NO patch seam → the real persistent.PatchBlock runs.
		// NO readPlan seam → the real readPlanFile runs (it must never be reached:
		// the 4xx returns before read-back).
	})

	body := fmt.Sprintf(`{"dir":%q,"block_id":%q,"level":3}`, emptyDir, blockID)
	w := doPost(t, defaultArgs(), body, "")
	// 4xx, not the old 200→readback→500 trap. The real PatchBlock wraps
	// ErrNothingToPatch without re-wrapping fs.ErrNotExist, so the handler's
	// fs.ErrNotExist split lands on 400 (still source_not_found-class, still a
	// clean caller-correctable error — the point of B-1).
	if w.Code != http.StatusBadRequest {
		t.Fatalf("incomplete dir must be 4xx source_not_found (NOT 200/500), got %d (body %q)", w.Code, w.Body.String())
	}
	e := decodeErr(t, w)
	if e.Reason != reasonSourceNotFound {
		t.Errorf("incomplete dir reason = %q, want %q", e.Reason, reasonSourceNotFound)
	}
	if !strings.Contains(e.Message, "incomplete persistent-sink dir") {
		t.Errorf("message should name the incomplete-dir cause, got %q", e.Message)
	}
}

// TestEscalate_SameLevelReissue_RealPatchBlock_Is200 seeds a COMPLETE persistent
// triple via the real persistent.Consume, then issues the escalate to the level
// the block already holds. There is no ErrNothingToPatch short-circuit: the real
// PatchBlock re-renders + re-splices content-identical bytes and the endpoint
// returns 200 with the current block (B3/B5 convergence by re-render). This is
// the real-dependency replacement for the deleted seam-injected no-op test.
func TestEscalate_SameLevelReissue_RealPatchBlock_Is200(t *testing.T) {
	const blockID = "b1"
	outDir, contentHash, sourceURI := seedPersistentTriple(t)

	// narrate seam: produce the one-block sub-plan + sub-result the production
	// pipeline would hand the sink — matching content hash (Step 3 gate) and a
	// real fresh WAV for the target block so the REAL PatchBlock splices.
	freshDir := t.TempDir()
	freshRef := blockID + "-rerender.wav"
	freshPCM := serverFramePCM(200, 0x40) // same 200ms target duration as seeded
	writeFileT(t, filepath.Join(freshDir, freshRef), serverSynthWAV(freshPCM))

	narrate := func(_ string, _ serverArgs, capturer *capturingSink) pipeline.Narrator {
		return narratorFunc(func(_ context.Context, _ plan.SourceRef, req pipeline.NarrateRequest) (pipeline.NarrateResult, error) {
			capturer.plan = plan.NarrationPlan{
				SchemaVersion: plan.SchemaVersion,
				// Source must match the seeded manifest (Kind+URI+ContentHash) or
				// CheckStale refuses with stale_patch before the splice.
				Source: plan.SourceRef{Kind: plan.SourceKindFile, URI: sourceURI, ContentHash: contentHash},
				Blocks: []plan.Block{{ID: blockID, Class: plan.ClassProse, Level: plan.Level(req.LevelOverrides[blockID]), Status: plan.StatusVoiced}},
			}
			capturer.result = render.RenderResult{
				Audio:    render.AudioStream{Dir: freshDir, Files: []string{freshRef}},
				Format:   render.DefaultFormat(),
				Timeline: plan.Timeline{Blocks: []plan.BlockTiming{{BlockID: blockID, StartMs: 0, EndMs: 200, AudioRef: freshRef}}},
			}
			capturer.captured = true
			return pipeline.NarrateResult{}, nil
		})
	}

	installSeams(t, seams{narrate: narrate})
	// NO patch / manifest / readPlan seams → real PatchBlock + real readback.

	body := fmt.Sprintf(`{"dir":%q,"block_id":%q,"level":1}`, outDir, blockID) // level it already holds
	w := doPost(t, defaultArgs(), body, "")
	if w.Code != http.StatusOK {
		t.Fatalf("same-level re-issue must converge to 200 by re-render, got %d (body %q)", w.Code, w.Body.String())
	}
	resp := decodeOK(t, w)
	if resp.Block.ID != blockID {
		t.Errorf("200 must return the current block, got %+v", resp.Block)
	}
	if resp.Timing == nil || resp.AudioRef == "" {
		t.Errorf("voiced 200 must carry timing + audio_ref, got %+v / %q", resp.Timing, resp.AudioRef)
	}
}

// ---------------------------------------------------------------------------
// Cancel / timeout class — 408 cancelled, NOT 500; mutex released afterward.
// ---------------------------------------------------------------------------

func TestEscalate_Timeout_Is408Cancelled_AndReleasesLock(t *testing.T) {
	const blockID = "b001"
	stub := &stubNarrator{blockUntilCtx: true}
	installSeams(t, seams{
		narrate:  stubNarrateSeam(stub),
		patch:    noopPatch,
		manifest: goodManifest(blockID),
		readPlan: planWithBlock(plan.Block{ID: blockID, Status: plan.StatusVoiced}),
	})

	args := defaultArgs()
	args.RequestTimeout = 50 * time.Millisecond

	w := doPost(t, args, `{"dir":"/tmp/cancel-dir","block_id":"b001","level":3}`, "")
	if w.Code != http.StatusRequestTimeout {
		t.Fatalf("timeout must be 408, got %d (body %q)", w.Code, w.Body.String())
	}
	if e := decodeErr(t, w); e.Reason != reasonCancelled {
		t.Errorf("reason = %q, want cancelled (NOT internal)", e.Reason)
	}

	// Lock-release-on-cancel (R2b/R2c): a subsequent same-dir request must
	// acquire the per-dir mutex without hanging. Swap to a fast stub and assert
	// it completes promptly.
	fastStub := &stubNarrator{block: plan.Block{ID: blockID, Status: plan.StatusVoiced}}
	installSeams(t, seams{
		narrate:  stubNarrateSeam(fastStub),
		patch:    noopPatch,
		manifest: goodManifest(blockID),
		readPlan: planWithBlock(plan.Block{ID: blockID, Status: plan.StatusVoiced}),
	})
	done := make(chan int, 1)
	go func() {
		w2 := doPost(t, defaultArgs(), `{"dir":"/tmp/cancel-dir","block_id":"b001","level":3}`, "")
		done <- w2.Code
	}()
	select {
	case code := <-done:
		if code != http.StatusOK {
			t.Errorf("post-cancel same-dir request status = %d, want 200", code)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("post-cancel same-dir request hung — per-dir mutex was leaked on the cancel path")
	}
}

// ---------------------------------------------------------------------------
// Error-status mapping — full closed enum (R7).
// ---------------------------------------------------------------------------

func TestEscalate_ErrorStatusMapping(t *testing.T) {
	const blockID = "b001"
	cases := []struct {
		name       string
		patchErr   error // injected via patch seam (nil = no patch error)
		manifest   func(dir string) (persistent.Manifest, error)
		wantStatus int
		wantReason string
	}{
		{"stale", fmt.Errorf("%w", persistent.ErrStalePatch), nil, http.StatusConflict, reasonStalePatch},
		{"content_hash", fmt.Errorf("%w", persistent.ErrContentHashMismatch), nil, http.StatusConflict, reasonContentHashMismatch},
		{"unknown_block", fmt.Errorf("%w", persistent.ErrUnknownBlock), nil, http.StatusConflict, reasonUnknownBlock},
		{"container", fmt.Errorf("%w", persistent.ErrContainerMismatch), nil, http.StatusConflict, reasonContainerMismatch},
		{"format", fmt.Errorf("%w", persistent.ErrFormatMismatch), nil, http.StatusConflict, reasonFormatMismatch},
		{"internal_patch", errors.New("disk on fire"), nil, http.StatusInternalServerError, reasonInternal},
		// caller_patch_*: fs.ErrNotExist/fs.ErrPermission wrapped through the patch
		// seam are errclass.ClassCaller, yet the server patch path deliberately routes
		// ClassCaller to the default 500 alongside ClassInternal (statusForErr, main.go
		// #51 note) — NOT the MCP root's caller→4xx. These rows are what PROVE that
		// ClassCaller specifically lands on 500: flipping statusForErr's default to a
		// 4xx would redden them. (It would also redden the existing genuinely-internal
		// rows like internal_patch — those aren't ClassCaller, so only these two pin
		// the ClassCaller→500 wire mapping.) The wrap text avoids the ErrNothingToPatch
		// sentinel so the handler reaches statusForErr instead of the 4xx source_not_found
		// pre-branch (main.go:539).
		{"caller_patch_notexist", fmt.Errorf("patch read: %w", fs.ErrNotExist), nil, http.StatusInternalServerError, reasonInternal},
		{"caller_patch_permission", fmt.Errorf("patch read: %w", fs.ErrPermission), nil, http.StatusInternalServerError, reasonInternal},
		{
			"manifest_absent_404", nil,
			func(_ string) (persistent.Manifest, error) {
				return persistent.Manifest{}, fmt.Errorf("read manifest: %w", fs.ErrNotExist)
			},
			http.StatusNotFound, reasonSourceNotFound,
		},
		{
			"manifest_permission_400", nil,
			func(_ string) (persistent.Manifest, error) {
				return persistent.Manifest{}, fmt.Errorf("read manifest: %w", fs.ErrPermission)
			},
			http.StatusBadRequest, reasonSourceNotFound,
		},
		{
			"manifest_unparseable_400", nil,
			func(_ string) (persistent.Manifest, error) {
				return persistent.Manifest{}, errors.New("invalid character 'x' looking for value")
			},
			http.StatusBadRequest, reasonSourceNotFound,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			stub := &stubNarrator{block: plan.Block{ID: blockID, Status: plan.StatusVoiced}}
			mf := tc.manifest
			if mf == nil {
				mf = goodManifest(blockID)
			}
			patchErr := tc.patchErr
			installSeams(t, seams{
				narrate: stubNarrateSeam(stub),
				patch: func(_ context.Context, _ string, _ plan.NarrationPlan, _ render.RenderResult, _ string, _ ...persistent.Option) (sink.SinkReceipt, error) {
					return sink.SinkReceipt{}, patchErr
				},
				manifest: mf,
				readPlan: planWithBlock(plan.Block{ID: blockID, Status: plan.StatusVoiced}),
			})

			w := doPost(t, defaultArgs(), `{"dir":"/tmp/x","block_id":"b001","level":3}`, "")
			if w.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d (body %q)", w.Code, tc.wantStatus, w.Body.String())
			}
			e := decodeErr(t, w)
			if e.Reason != tc.wantReason {
				t.Errorf("reason = %q, want %q", e.Reason, tc.wantReason)
			}
			if e.Message == "" {
				t.Errorf("ErrorResponse.message must be populated")
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Render error (Narrate hard fault) → 500 internal (errors stop the pipeline).
// ---------------------------------------------------------------------------

func TestEscalate_RenderError_Is500Internal(t *testing.T) {
	const blockID = "b001"
	stub := &stubNarrator{err: errors.New("pipeline: renderer: kokoro boom")}
	installSeams(t, seams{
		narrate:  stubNarrateSeam(stub),
		patch:    noopPatch,
		manifest: goodManifest(blockID),
	})
	w := doPost(t, defaultArgs(), `{"dir":"/tmp/x","block_id":"b001","level":3}`, "")
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("render error must be 500, got %d", w.Code)
	}
	if e := decodeErr(t, w); e.Reason != reasonInternal {
		t.Errorf("reason = %q, want internal", e.Reason)
	}
}

// ---------------------------------------------------------------------------
// Request validation — 400 missing_field / invalid_level.
// ---------------------------------------------------------------------------

func TestEscalate_RequestValidation(t *testing.T) {
	cases := []struct {
		name       string
		body       string
		wantReason string
	}{
		{"malformed_json", `{not json`, reasonMissingField},
		{"missing_dir", `{"block_id":"b1","level":2}`, reasonMissingField},
		{"missing_block_id", `{"dir":"/tmp/x","level":2}`, reasonMissingField},
		{"empty_dir", `{"dir":"","block_id":"b1","level":2}`, reasonMissingField},
		{"level_zero", `{"dir":"/tmp/x","block_id":"b1","level":0}`, reasonInvalidLevel},
		{"level_four", `{"dir":"/tmp/x","block_id":"b1","level":4}`, reasonInvalidLevel},
		{"unknown_field", `{"dir":"/tmp/x","block_id":"b1","level":2,"bogus":1}`, reasonMissingField},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// No seams needed — validation precedes any pipeline call. But install a
			// manifest seam so an accidental fall-through would not hit disk.
			installSeams(t, seams{manifest: goodManifest("b1")})
			w := doPost(t, defaultArgs(), tc.body, "")
			if w.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400 (body %q)", w.Code, w.Body.String())
			}
			if e := decodeErr(t, w); e.Reason != tc.wantReason {
				t.Errorf("reason = %q, want %q", e.Reason, tc.wantReason)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// CORS / preflight (R5) + healthz.
// ---------------------------------------------------------------------------

func TestCORS_PinnedLiteral_NeverEchoesOrigin(t *testing.T) {
	args := defaultArgs() // cors-origin = http://localhost:5173
	mux := newMux(args)

	t.Run("preflight_from_configured_origin", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodOptions, "/escalate", nil)
		r.Header.Set("Origin", "http://localhost:5173")
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, r)
		if w.Code != http.StatusNoContent {
			t.Errorf("preflight status = %d, want 204", w.Code)
		}
		if got := w.Header().Get("Access-Control-Allow-Origin"); got != "http://localhost:5173" {
			t.Errorf("ACAO = %q, want configured literal", got)
		}
	})

	t.Run("spoofed_origin_never_reflected", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodOptions, "/escalate", nil)
		r.Header.Set("Origin", "http://evil.example.com")
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, r)
		got := w.Header().Get("Access-Control-Allow-Origin")
		if got == "http://evil.example.com" {
			t.Fatalf("spoofed Origin was reflected back — security regression")
		}
		if got != "http://localhost:5173" {
			t.Errorf("ACAO = %q, want pinned literal regardless of request Origin", got)
		}
	})

	t.Run("healthz", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodGet, "/healthz", nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, r)
		if w.Code != http.StatusOK {
			t.Errorf("healthz status = %d, want 200", w.Code)
		}
		var body map[string]string
		_ = json.Unmarshal(w.Body.Bytes(), &body)
		if body["status"] != "ok" {
			t.Errorf("healthz body = %q, want status:ok", w.Body.String())
		}
	})
}

// ---------------------------------------------------------------------------
// Loopback binding — assert the NEGATIVE (B4). Test validateAddr directly.
// ---------------------------------------------------------------------------

func TestValidateAddr_RefusesNonLoopback(t *testing.T) {
	cases := []struct {
		addr    string
		wantErr bool
	}{
		{"127.0.0.1:8080", false},
		{"localhost:8080", false},
		{"[::1]:8080", false},
		{"0.0.0.0:8080", true}, // binds all interfaces — must refuse.
		{":8080", true},        // empty host == all interfaces — must refuse.
		{"[::]:8080", true},    // unspecified IPv6 — must refuse.
		{"192.168.1.50:8080", true},
		{"example.com:8080", true},
		{"garbage", true}, // missing port.
	}
	for _, tc := range cases {
		t.Run(tc.addr, func(t *testing.T) {
			err := validateAddr(tc.addr)
			if tc.wantErr && err == nil {
				t.Errorf("validateAddr(%q) = nil, want refusal", tc.addr)
			}
			if !tc.wantErr && err != nil {
				t.Errorf("validateAddr(%q) = %v, want accept", tc.addr, err)
			}
		})
	}
}

func TestParseFlags_RejectsBadGender(t *testing.T) {
	var buf bytes.Buffer
	if _, err := parseFlags([]string{"--gender", "robot"}, &buf); err == nil {
		t.Error("parseFlags accepted --gender robot, want error")
	}
}

// ---------------------------------------------------------------------------
// Per-dir serialization (B1) under -race. Proves SERIALIZATION (the per-dir
// mutex), NOT byte-level atomic-rename-under-contention — the stub Narrator
// never drives the real sink rename (R9). Atomic-rename stays covered by
// sink/persistent tests + manual /verify.
// ---------------------------------------------------------------------------

func TestEscalate_SameDirSerialized_DifferentDirNotBlocked(t *testing.T) {
	const blockID = "b001"

	// concurrentInCritical tracks how many requests are inside the critical
	// section at once; maxConcurrent must stay 1 for a given dir to prove
	// serialization. The narrate seam increments on entry and decrements on
	// exit while holding the (handler-acquired) per-dir mutex.
	var inCritical atomic.Int32
	var maxObserved atomic.Int32
	narrateSeam := func(_ string, _ serverArgs, capturer *capturingSink) pipeline.Narrator {
		return &serialSpyNarrator{capturer: capturer, block: plan.Block{ID: blockID, Status: plan.StatusVoiced},
			inCritical: &inCritical, maxObserved: &maxObserved}
	}
	installSeams(t, seams{
		narrate:  narrateSeam,
		patch:    noopPatch,
		manifest: goodManifest(blockID),
		readPlan: planWithBlock(plan.Block{ID: blockID, Status: plan.StatusVoiced}),
	})

	// Fire N concurrent escalates at the SAME dir.
	const n = 8
	args := defaultArgs()
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			w := doPost(t, args, `{"dir":"/tmp/same-dir","block_id":"b001","level":3}`, "")
			if w.Code != http.StatusOK {
				t.Errorf("concurrent same-dir status = %d, want 200", w.Code)
			}
		}()
	}
	wg.Wait()

	if got := maxObserved.Load(); got > 1 {
		t.Errorf("max concurrent critical-section occupancy = %d, want 1 (per-dir mutex not serializing)", got)
	}

	// Different-dir requests must NOT be mutually blocked (per-path mutex). Fire
	// two distinct dirs concurrently and confirm both complete quickly. We do
	// not assert overlap (timing-flaky); we assert neither hangs.
	done := make(chan struct{}, 2)
	for _, d := range []string{"/tmp/dir-a", "/tmp/dir-b"} {
		go func(dir string) {
			w := doPost(t, args, fmt.Sprintf(`{"dir":%q,"block_id":"b001","level":3}`, dir), "")
			if w.Code != http.StatusOK {
				t.Errorf("different-dir status = %d, want 200", w.Code)
			}
			done <- struct{}{}
		}(d)
	}
	for i := 0; i < 2; i++ {
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Fatal("different-dir request hung — mutex is global, not per-path")
		}
	}
}

// serialSpyNarrator observes critical-section occupancy. The handler holds the
// per-dir mutex across the entire Narrate call, so a counter incremented at
// Narrate entry and decremented at exit reflects how many handlers share the
// critical section for that dir at once.
type serialSpyNarrator struct {
	capturer    *capturingSink
	block       plan.Block
	inCritical  *atomic.Int32
	maxObserved *atomic.Int32
}

func (s *serialSpyNarrator) Narrate(_ context.Context, _ plan.SourceRef, _ pipeline.NarrateRequest) (pipeline.NarrateResult, error) {
	cur := s.inCritical.Add(1)
	for {
		m := s.maxObserved.Load()
		if cur <= m || s.maxObserved.CompareAndSwap(m, cur) {
			break
		}
	}
	time.Sleep(2 * time.Millisecond) // widen the window so overlap would be observable.
	if s.capturer != nil {
		s.capturer.plan = plan.NarrationPlan{Blocks: []plan.Block{s.block}}
		s.capturer.result = render.RenderResult{Timeline: plan.Timeline{Blocks: []plan.BlockTiming{{BlockID: s.block.ID}}}}
		s.capturer.captured = true
	}
	s.inCritical.Add(-1)
	return pipeline.NarrateResult{}, nil
}

// ---------------------------------------------------------------------------
// ReadManifest additive reader (R1) round-trips a real manifest dir.
// ---------------------------------------------------------------------------

func TestPersistentReadManifest_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	m := persistent.Manifest{
		SchemaVersion: persistent.ManifestSchemaVersion,
		SourceKind:    plan.SourceKindFile,
		SourceURI:     "/tmp/doc.md",
		ContentHash:   "hash-xyz",
		Blocks:        []persistent.ManifestBlock{{ID: "b001", StartMs: 0, EndMs: 100, AudioRef: "b1.wav"}},
	}
	raw, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "manifest.json"), raw, 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := persistent.ReadManifest(dir)
	if err != nil {
		t.Fatalf("ReadManifest: %v", err)
	}
	if got.ContentHash != "hash-xyz" || len(got.Blocks) != 1 || got.Blocks[0].ID != "b001" {
		t.Errorf("round-trip mismatch: %+v", got)
	}

	// Absent manifest → wrapped fs.ErrNotExist (the 404 source_not_found path).
	if _, err := persistent.ReadManifest(t.TempDir()); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("absent manifest err = %v, want fs.ErrNotExist", err)
	}
}
