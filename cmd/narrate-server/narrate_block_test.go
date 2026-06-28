package main

// narrate_block_test.go — POST /narrate/block single-block escalation (#110).
// Covers: route non-shadowing with /narrate; request validation (reuses the
// pinned reason-token contract); the L1→L2 round-trip through POST /narrate →
// POST /narrate/block (AC1 re-render at new level, AC2 sibling bytes preserved,
// AC3 round-trip passes); and the error table (unknown render_id, unknown
// block_id, incomplete dir). No Kokoro — the narrate + escalate re-renders are
// stubbed, but the persistent write + the REAL PatchBlock + readBack run so the
// byte-preservation invariant is genuinely exercised.

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vd09-projects/intelligent-tts-narration-library/pipeline"
	"github.com/vd09-projects/intelligent-tts-narration-library/plan"
	"github.com/vd09-projects/intelligent-tts-narration-library/render"
)

// postNarrateBlock drives one POST /narrate/block through narrateBlockHandler.
func postNarrateBlock(t *testing.T, store *renderStore, body string) *httptest.ResponseRecorder {
	t.Helper()
	h := narrateBlockHandler(defaultArgs(), store)
	r := httptest.NewRequest(http.MethodPost, "/narrate/block", strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

func decodeNarrateBlock(t *testing.T, w *httptest.ResponseRecorder) narrateBlockResponse {
	t.Helper()
	var n narrateBlockResponse
	if err := json.Unmarshal(w.Body.Bytes(), &n); err != nil {
		t.Fatalf("decode narrateBlockResponse: %v (body %q)", err, w.Body.String())
	}
	return n
}

// roundTripText is a 2-block document used by the round-trip + error tests.
const roundTripText = "Heading line\n\nSome prose body for block one.\n"

// b1StartByte — b1 begins at 100 ms; 24 kHz mono s16le = 48 bytes/ms, plus the
// 44-byte WAV header. Everything before this offset is the header + block b0,
// which the patch must leave byte-identical (AC2).
const b1StartByte = 44 + 100*serverBytesPerMs

// seedRenderDir drives a real POST /narrate (stub narrate writing two real
// per-block wavs into the render_id dir; REAL writeRenderDir persisting the
// 3-file triple) and returns the minted render_id + its on-disk outDir. The dir
// is a genuine file-sourced persistent triple, patchable by render_id.
func seedRenderDir(t *testing.T, store *renderStore) (renderID, outDir string) {
	t.Helper()
	hash := contentHashOf(roundTripText)
	b0pcm := serverFramePCM(100, 0x11)
	b1pcm := serverFramePCM(200, 0x22)

	narrate := func(_ string, _ plan.Level, dir string, capturer *capturingSink) pipeline.Narrator {
		return narratorFunc(func(_ context.Context, _ plan.SourceRef, _ pipeline.NarrateRequest) (pipeline.NarrateResult, error) {
			writeFileT(t, filepath.Join(dir, "b0.wav"), serverSynthWAV(b0pcm))
			writeFileT(t, filepath.Join(dir, "b1.wav"), serverSynthWAV(b1pcm))
			capturer.plan = plan.NarrationPlan{
				SchemaVersion: plan.SchemaVersion,
				PlanID:        "01HTSRVNARR0000000000000",
				CreatedAt:     "2026-06-28T00:00:00Z",
				Source:        plan.SourceRef{Kind: plan.SourceKindFile, URI: filepath.Join(dir, sourceLeaf), ContentHash: hash},
				Defaults:      plan.PlanDefaults{Level: plan.L1, Locale: "en"},
				Blocks: []plan.Block{
					{ID: "b0", Order: 0, Class: plan.ClassHeading, Level: plan.L1, Status: plan.StatusVoiced,
						SourceMap: plan.SourceMap{Kind: plan.SourceKindLineRange, StartLine: 1, EndLine: 1}},
					{ID: "b1", Order: 1, Class: plan.ClassProse, Level: plan.L1, Status: plan.StatusVoiced,
						SourceMap: plan.SourceMap{Kind: plan.SourceKindLineRange, StartLine: 3, EndLine: 3}},
				},
			}
			capturer.result = render.RenderResult{
				Audio:  render.AudioStream{Dir: dir, Files: []string{"b0.wav", "b1.wav"}},
				Format: render.DefaultFormat(),
				Timeline: plan.Timeline{Blocks: []plan.BlockTiming{
					{BlockID: "b0", StartMs: 0, EndMs: 100, AudioRef: "b0.wav"},
					{BlockID: "b1", StartMs: 100, EndMs: 300, AudioRef: "b1.wav"},
				}},
			}
			capturer.captured = true
			return pipeline.NarrateResult{}, nil
		})
	}
	installNarrateSeams(t, narrate, nil) // nil write → REAL writeRenderDir (persistent.Consume)

	w := postNarrate(t, store, fmt.Sprintf(`{"text":%q,"level":1}`, roundTripText))
	if w.Code != http.StatusOK {
		t.Fatalf("seed POST /narrate status = %d, want 200 (body %q)", w.Code, w.Body.String())
	}
	nr := decodeNarrate(t, w)
	id, ok := strings.CutSuffix(strings.TrimPrefix(nr.AudioURL, "/audio/"), ".wav")
	if !ok || !renderIDPattern.MatchString(id) {
		t.Fatalf("seed audio_url = %q, want /audio/{32-hex}.wav", nr.AudioURL)
	}
	return id, filepath.Join(store.tempRoot, id)
}

// installEscalateRerender stubs the escalate re-render of blockID at the
// requested level, producing a fresh per-block wav (different bytes, same 200 ms
// duration so b0's region is byte-identical). REAL PatchBlock + readBack run.
func installEscalateRerender(t *testing.T, outDir, blockID string, seed byte) {
	t.Helper()
	hash := contentHashOf(roundTripText)
	freshDir := t.TempDir()
	freshRef := blockID + "-rerender.wav"
	writeFileT(t, filepath.Join(freshDir, freshRef), serverSynthWAV(serverFramePCM(200, seed)))

	narrate := func(_ string, _ serverArgs, capturer *capturingSink) pipeline.Narrator {
		return narratorFunc(func(_ context.Context, _ plan.SourceRef, req pipeline.NarrateRequest) (pipeline.NarrateResult, error) {
			capturer.plan = plan.NarrationPlan{
				SchemaVersion: plan.SchemaVersion,
				Source:        plan.SourceRef{Kind: plan.SourceKindFile, URI: filepath.Join(outDir, sourceLeaf), ContentHash: hash},
				Blocks: []plan.Block{
					{ID: blockID, Class: plan.ClassProse, Level: req.LevelOverrides[blockID], Status: plan.StatusVoiced},
				},
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
	installSeams(t, seams{narrate: narrate}) // NO patch/manifest/readPlan seams → real PatchBlock + readback
}

func mustRead(t *testing.T, path string) []byte {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return b
}

// ---------------------------------------------------------------------------
// Route non-shadowing — /narrate and /narrate/block are distinct Go 1.22
// patterns; each must reach its own handler (the public-API guard).
// ---------------------------------------------------------------------------

func TestMux_NarrateRoutesDoNotShadow(t *testing.T) {
	// Stub /narrate so it does not spawn Kokoro; assert routing, not rendering.
	blocks := []plan.Block{{ID: "b1", Status: plan.StatusVoiced, Level: plan.L1}}
	narrate, write := narrateStub(blocks, nil, []byte("RIFFnarrate"))
	installNarrateSeams(t, narrate, write)

	mux := newMux(defaultArgs(), newRenderStore(t.TempDir()))

	t.Run("POST /narrate reaches narrate handler", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodPost, "/narrate", strings.NewReader(`{"text":"x","level":1}`))
		r.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, r)
		// narrateResponse carries blocks[]; narrateBlockResponse never does.
		if w.Code != http.StatusOK {
			t.Fatalf("POST /narrate status = %d, want 200 (body %q)", w.Code, w.Body.String())
		}
		if !strings.Contains(w.Body.String(), `"blocks"`) {
			t.Fatalf("POST /narrate did not reach the narrate handler (body %q)", w.Body.String())
		}
	})

	t.Run("POST /narrate/block reaches block handler", func(t *testing.T) {
		// An unknown render_id is the block handler's 404; the narrate handler
		// would instead 400 on the missing `text` field. The 404 proves routing.
		r := httptest.NewRequest(http.MethodPost, "/narrate/block",
			strings.NewReader(fmt.Sprintf(`{"render_id":%q,"block_id":"b1","level":2}`, strings.Repeat("0", 32))))
		r.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, r)
		if w.Code != http.StatusNotFound {
			t.Fatalf("POST /narrate/block status = %d, want 404 (body %q)", w.Code, w.Body.String())
		}
		if got := decodeErr(t, w).Reason; got != reasonSourceNotFound {
			t.Fatalf("POST /narrate/block reason = %q, want %q (wrong handler?)", got, reasonSourceNotFound)
		}
	})
}

// ---------------------------------------------------------------------------
// Request validation — the pinned Reason Token Contract per row.
// ---------------------------------------------------------------------------

func TestNarrateBlock_RequestValidation(t *testing.T) {
	id := strings.Repeat("a", 32)
	tests := []struct {
		name       string
		method     string
		body       string
		wantStatus int
		wantReason string
	}{
		{"method_get", http.MethodGet, fmt.Sprintf(`{"render_id":%q,"block_id":"b1","level":1}`, id), http.StatusMethodNotAllowed, reasonMethodNotAllowed},
		{"malformed_json", http.MethodPost, `{`, http.StatusBadRequest, reasonMissingField},
		{"unknown_field", http.MethodPost, fmt.Sprintf(`{"render_id":%q,"block_id":"b1","level":1,"dir":"/etc"}`, id), http.StatusBadRequest, reasonMissingField},
		{"missing_render_id", http.MethodPost, `{"block_id":"b1","level":1}`, http.StatusBadRequest, reasonMissingField},
		{"bad_render_id", http.MethodPost, `{"render_id":"not-hex","block_id":"b1","level":1}`, http.StatusBadRequest, reasonMissingField},
		{"missing_block_id", http.MethodPost, fmt.Sprintf(`{"render_id":%q,"level":1}`, id), http.StatusBadRequest, reasonMissingField},
		{"level_zero", http.MethodPost, fmt.Sprintf(`{"render_id":%q,"block_id":"b1","level":0}`, id), http.StatusBadRequest, reasonInvalidLevel},
		{"level_four", http.MethodPost, fmt.Sprintf(`{"render_id":%q,"block_id":"b1","level":4}`, id), http.StatusBadRequest, reasonInvalidLevel},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			store := newRenderStore(t.TempDir())
			h := narrateBlockHandler(defaultArgs(), store)
			r := httptest.NewRequest(tc.method, "/narrate/block", strings.NewReader(tc.body))
			r.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)
			if w.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d (body %q)", w.Code, tc.wantStatus, w.Body.String())
			}
			if got := decodeErr(t, w).Reason; got != tc.wantReason {
				t.Fatalf("reason = %q, want %q", got, tc.wantReason)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Unknown / expired render_id → 404 source_not_found (a well-formed but
// untracked id; dirFor never touches the fs for an untracked id).
// ---------------------------------------------------------------------------

func TestNarrateBlock_UnknownRenderID_Is404(t *testing.T) {
	store := newRenderStore(t.TempDir())
	w := postNarrateBlock(t, store, fmt.Sprintf(`{"render_id":%q,"block_id":"b1","level":2}`, strings.Repeat("f", 32)))
	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404 (body %q)", w.Code, w.Body.String())
	}
	if got := decodeErr(t, w).Reason; got != reasonSourceNotFound {
		t.Fatalf("reason = %q, want %q", got, reasonSourceNotFound)
	}
}

// ---------------------------------------------------------------------------
// AC1 + AC2 + AC3 — the L1→L2 round-trip. POST /narrate mints a patchable dir;
// POST /narrate/block re-renders b1 at L2 via the real PatchBlock; b1 reflects
// L2, sibling b0's audio bytes are byte-identical, audio_url is unchanged.
// ---------------------------------------------------------------------------

func TestNarrateBlock_RoundTrip_L1ToL2(t *testing.T) {
	store := newRenderStore(t.TempDir())
	renderID, outDir := seedRenderDir(t, store)

	beforeAudio := mustRead(t, filepath.Join(outDir, "audio.wav"))
	if len(beforeAudio) <= b1StartByte {
		t.Fatalf("seeded audio.wav too short (%d bytes) for the b1 offset %d", len(beforeAudio), b1StartByte)
	}

	installEscalateRerender(t, outDir, "b1", 0x77)

	w := postNarrateBlock(t, store, fmt.Sprintf(`{"render_id":%q,"block_id":"b1","level":2}`, renderID))
	if w.Code != http.StatusOK {
		t.Fatalf("POST /narrate/block status = %d, want 200 (body %q)", w.Code, w.Body.String())
	}
	resp := decodeNarrateBlock(t, w)

	// AC1 + AC3 — b1 re-rendered at L2.
	if resp.Block.ID != "b1" {
		t.Fatalf("response block = %q, want b1", resp.Block.ID)
	}
	if resp.Block.Level != plan.L2 {
		t.Fatalf("response block level = %v, want L2 (re-render did not take)", resp.Block.Level)
	}
	if resp.Timing == nil || resp.Timing.BlockID != "b1" {
		t.Fatalf("voiced 200 must carry timing for b1, got %+v", resp.Timing)
	}
	// audio_url is the stable /audio/{render_id}.wav (whole-file playback unit).
	if resp.AudioURL != "/audio/"+renderID+".wav" {
		t.Fatalf("audio_url = %q, want /audio/%s.wav", resp.AudioURL, renderID)
	}

	// AC2 — sibling b0 byte-preserved: the header + b0 region (everything before
	// b1's start) must be byte-identical before vs after the patch.
	afterAudio := mustRead(t, filepath.Join(outDir, "audio.wav"))
	if !bytes.Equal(beforeAudio[:b1StartByte], afterAudio[:b1StartByte]) {
		t.Fatal("sibling b0 audio bytes changed across the patch — AC2 (byte-preserved siblings) broken")
	}
	// And the patch actually changed b1's region (the escalation took effect).
	if bytes.Equal(beforeAudio[b1StartByte:], afterAudio[b1StartByte:]) {
		t.Fatal("b1 audio region unchanged — the L2 re-render was not spliced in")
	}
}

// ---------------------------------------------------------------------------
// #113 — the /narrate/block response carries the FULL post-patch timeline, with
// downstream sibling offsets SHIFTED by the patch. Escalating b0 (the first
// block) to a longer render (100 ms → 200 ms) must move sibling b1's start
// 100 → 200 in the returned timeline. This is the server oracle the client's
// downstream-sibling-seek correctness depends on (reinterpreted AC6).
// ---------------------------------------------------------------------------

func TestNarrateBlock_ResponseCarriesPostPatchTimeline(t *testing.T) {
	store := newRenderStore(t.TempDir())
	renderID, outDir := seedRenderDir(t, store)    // b0 0–100, b1 100–300
	installEscalateRerender(t, outDir, "b0", 0x33) // re-render b0 at 200 ms (grows +100)

	w := postNarrateBlock(t, store, fmt.Sprintf(`{"render_id":%q,"block_id":"b0","level":2}`, renderID))
	if w.Code != http.StatusOK {
		t.Fatalf("POST /narrate/block status = %d, want 200 (body %q)", w.Code, w.Body.String())
	}
	resp := decodeNarrateBlock(t, w)

	if len(resp.Timeline.Blocks) != 2 {
		t.Fatalf("timeline must carry ALL blocks, got %d: %+v", len(resp.Timeline.Blocks), resp.Timeline.Blocks)
	}
	if resp.Timeline.PlanID == "" {
		t.Fatal("post-patch timeline must carry plan_id (read back from plan.json)")
	}
	byID := map[string]plan.BlockTiming{}
	for _, b := range resp.Timeline.Blocks {
		byID[b.BlockID] = b
	}
	if b0 := byID["b0"]; b0.StartMs != 0 || b0.EndMs != 200 {
		t.Fatalf("b0 timing = %+v, want {start 0, end 200} (the grown re-render)", b0)
	}
	// The load-bearing assertion: the DOWNSTREAM sibling b1 shifted by the patch.
	if b1 := byID["b1"]; b1.StartMs != 200 {
		t.Fatalf("downstream sibling b1 start = %d, want 200 (shifted by the longer b0) — sibling reflow missing", b1.StartMs)
	}
}

// ---------------------------------------------------------------------------
// Error table — unknown block_id (real PatchBlock → ErrUnknownBlock → 409) and
// an incomplete render_id dir (manifest absent → 404 source_not_found, the
// ErrNothingToPatch-class read-path failure).
// ---------------------------------------------------------------------------

func TestNarrateBlock_UnknownBlockID_Is409(t *testing.T) {
	store := newRenderStore(t.TempDir())
	renderID, outDir := seedRenderDir(t, store)
	installEscalateRerender(t, outDir, "bZ", 0x55) // bZ is not in the manifest

	w := postNarrateBlock(t, store, fmt.Sprintf(`{"render_id":%q,"block_id":"bZ","level":2}`, renderID))
	if w.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409 (body %q)", w.Code, w.Body.String())
	}
	if got := decodeErr(t, w).Reason; got != reasonUnknownBlock {
		t.Fatalf("reason = %q, want %q", got, reasonUnknownBlock)
	}
}

func TestNarrateBlock_IncompleteDir_Is404SourceNotFound(t *testing.T) {
	store := newRenderStore(t.TempDir())
	// Reserve + commit a render_id whose dir holds only source.txt — no triple,
	// so escalateInDir's readManifest fails with fs.ErrNotExist → 404.
	id, dir, err := store.reserve()
	if err != nil {
		t.Fatalf("reserve: %v", err)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	writeFileT(t, filepath.Join(dir, sourceLeaf), []byte("incomplete"))
	store.commit(id)

	w := postNarrateBlock(t, store, fmt.Sprintf(`{"render_id":%q,"block_id":"b1","level":2}`, id))
	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404 (body %q)", w.Code, w.Body.String())
	}
	if got := decodeErr(t, w).Reason; got != reasonSourceNotFound {
		t.Fatalf("reason = %q, want %q", got, reasonSourceNotFound)
	}
}
