package main

import (
	"context"
	"encoding/json"
	"errors"
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

// installNarrateSeams swaps the /narrate factory + dir-writer seams for a test.
func installNarrateSeams(t *testing.T,
	narrate func(voice string, level plan.Level, outDir string, capturer *capturingSink) pipeline.Narrator,
	write func(ctx context.Context, dir, voice string, format plan.AudioFormat, p plan.NarrationPlan, res render.RenderResult) error,
) {
	t.Helper()
	origN, origW := newNarratePipeline, writeRenderDir
	if narrate != nil {
		newNarratePipeline = narrate
	}
	if write != nil {
		writeRenderDir = write
	}
	t.Cleanup(func() { newNarratePipeline, writeRenderDir = origN, origW })
}

// narrateStub returns a /narrate factory seam producing a narrator that
// populates the capturer with the given blocks (whole-doc path), plus a
// matching writeRenderDir that writes payload to {dir}/audio.wav (the served
// artifact in the new 3-file render_id layout).
func narrateStub(blocks []plan.Block, narrateErr error, payload []byte) (
	func(string, plan.Level, string, *capturingSink) pipeline.Narrator,
	func(context.Context, string, string, plan.AudioFormat, plan.NarrationPlan, render.RenderResult) error,
) {
	narrate := func(_ string, _ plan.Level, _ string, capturer *capturingSink) pipeline.Narrator {
		return narratorFunc(func(_ context.Context, _ plan.SourceRef, _ pipeline.NarrateRequest) (pipeline.NarrateResult, error) {
			if narrateErr != nil {
				return pipeline.NarrateResult{}, narrateErr
			}
			timing := make([]plan.BlockTiming, len(blocks))
			for i := range blocks {
				timing[i] = plan.BlockTiming{BlockID: blocks[i].ID}
			}
			capturer.plan = plan.NarrationPlan{Blocks: blocks}
			capturer.result = render.RenderResult{Timeline: plan.Timeline{Blocks: timing}}
			capturer.captured = true
			return pipeline.NarrateResult{}, nil
		})
	}
	write := func(_ context.Context, dir, _ string, _ plan.AudioFormat, _ plan.NarrationPlan, _ render.RenderResult) error {
		return os.WriteFile(filepath.Join(dir, "audio.wav"), payload, 0o600)
	}
	return narrate, write
}

// postNarrate drives one POST /narrate through narrateHandler bound to store.
func postNarrate(t *testing.T, store *renderStore, body string) *httptest.ResponseRecorder {
	t.Helper()
	h := narrateHandler(defaultArgs(), store)
	r := httptest.NewRequest(http.MethodPost, "/narrate", strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

func decodeNarrate(t *testing.T, w *httptest.ResponseRecorder) narrateResponse {
	t.Helper()
	var n narrateResponse
	if err := json.Unmarshal(w.Body.Bytes(), &n); err != nil {
		t.Fatalf("decode narrateResponse: %v (body %q)", err, w.Body.String())
	}
	return n
}

// ---------------------------------------------------------------------------
// Request validation — the pinned Reason Token Contract per row.
// ---------------------------------------------------------------------------

func TestNarrate_RequestValidation(t *testing.T) {
	tests := []struct {
		name       string
		method     string
		body       string
		wantStatus int
		wantReason string
	}{
		{"method_get", http.MethodGet, `{"text":"x","level":1}`, http.StatusMethodNotAllowed, reasonMethodNotAllowed},
		{"malformed_json", http.MethodPost, `{`, http.StatusBadRequest, reasonMissingField},
		{"unknown_field", http.MethodPost, `{"text":"x","level":1,"source":"/etc/passwd"}`, http.StatusBadRequest, reasonMissingField},
		{"missing_text", http.MethodPost, `{"level":1}`, http.StatusBadRequest, reasonMissingField},
		{"empty_text", http.MethodPost, `{"text":"","level":1}`, http.StatusBadRequest, reasonMissingField},
		{"level_zero", http.MethodPost, `{"text":"x","level":0}`, http.StatusBadRequest, reasonInvalidLevel},
		{"level_four", http.MethodPost, `{"text":"x","level":4}`, http.StatusBadRequest, reasonInvalidLevel},
		{"bad_gender", http.MethodPost, `{"text":"x","level":1,"gender":"robot"}`, http.StatusBadRequest, reasonMissingField},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			store := newRenderStore(t.TempDir())
			h := narrateHandler(defaultArgs(), store)
			r := httptest.NewRequest(tc.method, "/narrate", strings.NewReader(tc.body))
			r.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)
			if w.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d (body %q)", w.Code, tc.wantStatus, w.Body.String())
			}
			if got := decodeErr(t, w).Reason; got != tc.wantReason {
				t.Fatalf("reason = %q, want %q", got, tc.wantReason)
			}
			if len(store.entries) != 0 {
				t.Fatalf("validation failure minted %d store entries, want 0", len(store.entries))
			}
		})
	}
}

// ---------------------------------------------------------------------------
// R2 regression guard — a {text, source} body MUST be rejected with HTTP 400.
// The /narrate request shape deliberately carries NO source field; combined with
// DisallowUnknownFields a "source" becomes a hard 400, closing the
// browser-reachable arbitrary-file-read on loopback. Pinned explicitly (beyond
// the generic unknown_field row) so a future struct-tag edit re-adding `source`
// cannot silently reopen the read.
// ---------------------------------------------------------------------------

func TestNarrate_SourceFieldRejected(t *testing.T) {
	store := newRenderStore(t.TempDir())
	w := postNarrate(t, store, `{"text":"hello","level":1,"source":"/etc/passwd"}`)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 (a source field must be rejected)", w.Code)
	}
	if got := decodeErr(t, w).Reason; got != reasonMissingField {
		t.Fatalf("reason = %q, want %q", got, reasonMissingField)
	}
	if len(store.entries) != 0 {
		t.Fatalf("rejected source request minted %d store entries, want 0", len(store.entries))
	}
}

// ---------------------------------------------------------------------------
// Happy path — {audio_url, blocks, timeline}, a wav lands in the store, and the
// audio_url id matches the minted id. Default + explicit gender both 200.
// ---------------------------------------------------------------------------

func TestNarrate_HappyPath(t *testing.T) {
	for _, body := range []string{
		`{"text":"hello","level":1}`, // gender absent → female
		`{"text":"hello","level":2,"gender":"male"}`,
	} {
		t.Run(body, func(t *testing.T) {
			blocks := []plan.Block{{ID: "b1", Status: plan.StatusVoiced, Level: plan.L1}}
			payload := []byte("RIFFfake-wav-bytes")
			narrate, write := narrateStub(blocks, nil, payload)
			installNarrateSeams(t, narrate, write)

			store := newRenderStore(t.TempDir())
			w := postNarrate(t, store, body)
			if w.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200 (body %q)", w.Code, w.Body.String())
			}
			resp := decodeNarrate(t, w)
			if len(resp.Blocks) != 1 || resp.Blocks[0].ID != "b1" {
				t.Fatalf("blocks = %+v, want one block b1", resp.Blocks)
			}
			if len(resp.Timeline.Blocks) != 1 {
				t.Fatalf("timeline blocks = %d, want 1", len(resp.Timeline.Blocks))
			}
			id, ok := strings.CutSuffix(strings.TrimPrefix(resp.AudioURL, "/audio/"), ".wav")
			if !ok || !renderIDPattern.MatchString(id) {
				t.Fatalf("audio_url = %q, want /audio/{32-hex}.wav", resp.AudioURL)
			}
			// #113 — render_id is the bare id, exposed additively so a client need
			// not parse it out of the opaque audio_url. It must equal that id.
			if resp.RenderID != id {
				t.Fatalf("render_id = %q, want %q (must match the audio_url id)", resp.RenderID, id)
			}
			if _, tracked := store.entries[id]; !tracked {
				t.Fatalf("minted id %q not registered in store", id)
			}
			gotBytes, err := os.ReadFile(filepath.Join(store.tempRoot, id, "audio.wav"))
			if err != nil {
				t.Fatalf("read minted wav: %v", err)
			}
			if string(gotBytes) != string(payload) {
				t.Fatalf("wav bytes = %q, want %q", gotBytes, payload)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Refusal is HTTP 200 (R-NB1) — refused blocks + a refusal-notice audio_url,
// NOT a fabricated gist and NOT an error status.
// ---------------------------------------------------------------------------

func TestNarrate_RefusalIsHTTP200(t *testing.T) {
	refusal := &plan.Refusal{Reason: plan.RefuseNoIntelligence, Message: "this block was not voiced"}
	blocks := []plan.Block{{ID: "r1", Status: plan.StatusRefused, Refusal: refusal}}
	narrate, write := narrateStub(blocks, nil, []byte("refusal-notice-wav"))
	installNarrateSeams(t, narrate, write)

	store := newRenderStore(t.TempDir())
	w := postNarrate(t, store, `{"text":"a very long prose body","level":1}`)
	if w.Code != http.StatusOK {
		t.Fatalf("refusal status = %d, want 200 (refusal is data, not an error)", w.Code)
	}
	resp := decodeNarrate(t, w)
	if len(resp.Blocks) != 1 || resp.Blocks[0].Status != plan.StatusRefused {
		t.Fatalf("blocks = %+v, want one refused block", resp.Blocks)
	}
	if resp.AudioURL == "" {
		t.Fatal("refusal must still carry a refusal-notice audio_url")
	}
	// The refusal-notice wav resolves.
	id, _ := strings.CutSuffix(strings.TrimPrefix(resp.AudioURL, "/audio/"), ".wav")
	if _, tracked := store.entries[id]; !tracked {
		t.Fatalf("refusal audio_url id %q not in store", id)
	}
}

// ---------------------------------------------------------------------------
// #156 launch --gender is INERT for /narrate — the PER-REQUEST gender drives the
// render voice. Driven end-to-end through narrateHandler (the `gender` body field
// → resolved render-voice hint), with launch Gender="male" and NO launch --voice,
// so the launch gender is provably inert: a per-request gender=female must NOT be
// overridden by launch Gender="male". This locks the byte-identical pre-#156
// behavior where launch --gender never routed /narrate; per-request gender wins.
// (Distinct from voice_test.go's TestNarrate_NoLaunchVoice_RequestGenderDrives,
// which calls runNarrate directly with a pre-resolved requestVoice under a female
// launch gender — this one exercises the HTTP gender→voice resolution and a
// non-matching launch gender.) An explicit launch --voice is the separate
// authoritative path (see TestNarrate_LaunchVoice_* + the route-contract header).
// ---------------------------------------------------------------------------

func TestNarrate_LaunchGenderInert_PerRequestGenderWins(t *testing.T) {
	tests := []struct {
		name      string
		reqGender string // per-request `gender` body fragment ("" → absent → female)
		wantVoice string // expected render-voice hint AND manifest voice
	}{
		{"per_request_female_overrides_launch_male", `"gender":"female",`, "af_bella"},
		{"per_request_male_matches_launch_male", `"gender":"male",`, "am_michael"},
		{"per_request_absent_defaults_female", "", "af_bella"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var gotRenderVoice, gotManifestVoice string
			narrate := func(_ string, _ plan.Level, _ string, capturer *capturingSink) pipeline.Narrator {
				return narratorFunc(func(_ context.Context, _ plan.SourceRef, req pipeline.NarrateRequest) (pipeline.NarrateResult, error) {
					gotRenderVoice = req.Voice // the render-voice hint that reaches the renderer
					capturer.plan = plan.NarrationPlan{Blocks: []plan.Block{{ID: "b1", Status: plan.StatusVoiced}}}
					capturer.result = render.RenderResult{Timeline: plan.Timeline{Blocks: []plan.BlockTiming{{BlockID: "b1"}}}}
					capturer.captured = true
					return pipeline.NarrateResult{}, nil
				})
			}
			write := func(_ context.Context, dir, voice string, _ plan.AudioFormat, _ plan.NarrationPlan, _ render.RenderResult) error {
				gotManifestVoice = voice
				return os.WriteFile(filepath.Join(dir, "audio.wav"), []byte("wav"), 0o600)
			}
			installNarrateSeams(t, narrate, write)

			// Launch --gender=male, NO launch --voice (Voice==""): --gender is inert
			// for /narrate, so it must not leak into the render voice.
			args := defaultArgs()
			args.Gender = "male"
			args.Voice = ""

			store := newRenderStore(t.TempDir())
			h := narrateHandler(args, store)
			body := `{"text":"hello",` + tc.reqGender + `"level":1}`
			r := httptest.NewRequest(http.MethodPost, "/narrate", strings.NewReader(body))
			r.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)

			if w.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200 (body %q)", w.Code, w.Body.String())
			}
			if gotRenderVoice != tc.wantVoice {
				t.Fatalf("render voice hint = %q, want %q (per-request gender must win; launch --gender=male is inert)", gotRenderVoice, tc.wantVoice)
			}
			if gotManifestVoice != tc.wantVoice {
				t.Fatalf("manifest voice = %q, want %q (per-request gender must win; launch --gender=male is inert)", gotManifestVoice, tc.wantVoice)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Hard render fault → render_failed/500 with NO wav left in the store.
// ---------------------------------------------------------------------------

func TestNarrate_RenderError(t *testing.T) {
	t.Run("pipeline_error", func(t *testing.T) {
		narrate, write := narrateStub(nil, errors.New("pipeline: renderer: kokoro boom"), nil)
		installNarrateSeams(t, narrate, write)
		store := newRenderStore(t.TempDir())
		w := postNarrate(t, store, `{"text":"x","level":1}`)
		if w.Code != http.StatusInternalServerError {
			t.Fatalf("status = %d, want 500", w.Code)
		}
		if got := decodeErr(t, w).Reason; got != reasonRenderFailed {
			t.Fatalf("reason = %q, want %q", got, reasonRenderFailed)
		}
		if len(store.entries) != 0 {
			t.Fatalf("render error left %d store entries, want 0 (no leak)", len(store.entries))
		}
	})

	t.Run("wav_write_error", func(t *testing.T) {
		blocks := []plan.Block{{ID: "b1", Status: plan.StatusVoiced}}
		narrate, _ := narrateStub(blocks, nil, nil)
		writeErr := func(_ context.Context, _, _ string, _ plan.AudioFormat, _ plan.NarrationPlan, _ render.RenderResult) error {
			return errors.New("disk full")
		}
		installNarrateSeams(t, narrate, writeErr)
		store := newRenderStore(t.TempDir())
		w := postNarrate(t, store, `{"text":"x","level":1}`)
		if w.Code != http.StatusInternalServerError {
			t.Fatalf("status = %d, want 500", w.Code)
		}
		if got := decodeErr(t, w).Reason; got != reasonRenderFailed {
			t.Fatalf("reason = %q, want %q", got, reasonRenderFailed)
		}
		if len(store.entries) != 0 {
			t.Fatalf("wav-write error left %d store entries, want 0 (write-before-commit)", len(store.entries))
		}
	})
}
