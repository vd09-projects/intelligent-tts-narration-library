package main

// narrate.go — POST /narrate (#109 Step 4). Inline TEXT only this phase (R2: no
// server-side `source` path — a browser-reachable arbitrary-file-read on
// loopback). Decode {text, level, gender} → render the whole input through a
// fresh pipeline with a capturingSink (no write) → mint a render_id + write the
// single combined wav (WAVFileSink) into the server temp root → respond
// {audio_url, blocks, timeline}.
//
// Refusal is HTTP 200 (R-NB1, CLAUDE.md honesty rule): an oversized-prose text
// with nil intelligence yields refused blocks whose refusal-notice is itself
// rendered to the wav — that is data, not an error. Only a hard render/sink
// fault is render_failed/500.

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"

	"github.com/vd09-projects/intelligent-tts-narration-library/adapter/mcptext"
	"github.com/vd09-projects/intelligent-tts-narration-library/intelligence"
	"github.com/vd09-projects/intelligent-tts-narration-library/internal/errclass"
	"github.com/vd09-projects/intelligent-tts-narration-library/pipeline"
	"github.com/vd09-projects/intelligent-tts-narration-library/plan"
	"github.com/vd09-projects/intelligent-tts-narration-library/render"
	"github.com/vd09-projects/intelligent-tts-narration-library/render/sherpa"
	"github.com/vd09-projects/intelligent-tts-narration-library/sink/persistent"
)

// narrateRequest — POST /narrate body. snake_case, DisallowUnknownFields. NO
// `source` field this phase (R2). gender is optional (absent → female).
type narrateRequest struct {
	Text   string `json:"text"`
	Level  int    `json:"level"`
	Gender string `json:"gender"`
}

// narrateResponse — the 200 body. blocks is the planner's roster (carrying
// refused blocks as data); timeline is the renderer's block-level sync; audio_url
// points at the minted wav. No word-level timing (block-level sync only).
type narrateResponse struct {
	AudioURL string        `json:"audio_url"`
	Blocks   []plan.Block  `json:"blocks"`
	Timeline plan.Timeline `json:"timeline"`
}

// newNarratePipeline — package-level factory seam (mirrors newPipeline). Wires
// the inline-text adapter (mcptext.New — Step 0 Q3) + nil intelligence (phase
// one; the planner's deterministic + degraded + refuse path is the honesty
// backbone) + the sherpa renderer + the caller's capturingSink. outDir is a
// per-request scratch dir the renderer writes its per-block wavs into; the
// WAVFileSink later concatenates those into the single served blob. Tests swap
// this var for a stub narrator (no Kokoro).
var newNarratePipeline = func(text, voice string, level plan.Level, outDir string, capturer *capturingSink) pipeline.Narrator {
	return pipeline.New(
		mcptext.New(text),
		intelligence.IntelligenceAdapter(nil),
		sherpa.New(sherpa.EngineConfig{}),
		capturer,
		pipeline.PipelineDefaults{
			Level:  level,
			OutDir: outDir,
			Locale: "en",
		},
	)
}

// writeRenderWAV — seam over the single-wav sink so tests can write deterministic
// bytes without a real render. Production writes the combined 24 kHz mono s16le
// wav via persistent.NewWAVFile.
//
// R-NB4: this is the SINGLE-WAV variant (one combined audio.wav, no plan.json /
// manifest.json sidecars), NOT the 3-file persistent sink and NOT a tee off the
// speak/ephemeral path. A consumer wanting durable artifacts uses a SEPARATE
// `cmd/narrate --sink persistent` run (standing decoupling order).
var writeRenderWAV = func(ctx context.Context, path string, p plan.NarrationPlan, res render.RenderResult) error {
	_, err := persistent.NewWAVFile(path).Consume(ctx, p, res)
	return err
}

// narrateHandler returns the POST /narrate handler bound to args + the store.
func narrateHandler(args serverArgs, store *renderStore) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeError(w, http.StatusMethodNotAllowed, reasonMethodNotAllowed, "POST only")
			return
		}

		var req narrateRequest
		dec := json.NewDecoder(r.Body)
		dec.DisallowUnknownFields()
		if err := dec.Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, reasonMissingField, "malformed JSON body: "+err.Error())
			return
		}
		if req.Text == "" {
			writeError(w, http.StatusBadRequest, reasonMissingField, "text is required")
			return
		}
		if req.Level < 1 || req.Level > 3 {
			writeError(w, http.StatusBadRequest, reasonInvalidLevel,
				fmt.Sprintf("level must be 1, 2, or 3 (got %d)", req.Level))
			return
		}
		// gender: absent → female (no error); a present-but-unknown value is a
		// caller mistake → missing_field (per the pinned Reason Token Contract).
		gender := req.Gender
		if gender == "" {
			gender = "female"
		}
		voice, ok := genderToVoice[gender]
		if !ok {
			writeError(w, http.StatusBadRequest, reasonMissingField,
				fmt.Sprintf("gender must be female or male (got %q)", req.Gender))
			return
		}

		resp, status, errResp := runNarrate(r.Context(), args, store, req.Text, plan.Level(req.Level), voice)
		if errResp != nil {
			writeError(w, status, errResp.Reason, errResp.Message)
			return
		}
		writeJSON(w, status, resp)
	})
}

// runNarrate is the handler core (pulled out so tests assert the mapping
// directly). Returns either a success response (errResp nil, 200 — INCLUDING the
// refusal case) or an ErrorResponse + status.
func runNarrate(reqCtx context.Context, args serverArgs, store *renderStore, text string, level plan.Level, voice string) (narrateResponse, int, *ErrorResponse) {
	// Per-request timeout bounds a hung Kokoro render (mirrors /escalate).
	ctx, cancel := context.WithTimeout(reqCtx, args.RequestTimeout)
	defer cancel()

	// Per-request scratch dir for the renderer's per-block wavs. Removed after
	// the combined wav is written; only the final {id}.wav under tempRoot
	// survives.
	renderDir, err := os.MkdirTemp("", "narrate-server-render-*")
	if err != nil {
		return narrateResponse{}, http.StatusInternalServerError,
			&ErrorResponse{Reason: reasonInternal, Message: "create render scratch dir: " + err.Error()}
	}
	defer func() { _ = os.RemoveAll(renderDir) }()

	capturer := &capturingSink{}
	narrator := newNarratePipeline(text, voice, level, renderDir, capturer)
	ref := plan.SourceRef{Kind: plan.SourceKindMCPText, URI: mcptext.URIFor(text)}
	if _, err := narrator.Narrate(ctx, ref, pipeline.NarrateRequest{Voice: voice}); err != nil {
		// A timeout/cancel mirrors /escalate's 408 path; any other pipeline fault
		// (Kokoro hard fault, sink error) is the one new render_failed/500 token.
		if errclass.Classify(err) == errclass.ClassCancelled {
			return narrateResponse{}, http.StatusRequestTimeout,
				&ErrorResponse{Reason: reasonCancelled, Message: "request cancelled or timed out: " + err.Error()}
		}
		return narrateResponse{}, http.StatusInternalServerError,
			&ErrorResponse{Reason: reasonRenderFailed, Message: "render failed: " + err.Error()}
	}
	if !capturer.captured {
		return narrateResponse{}, http.StatusInternalServerError,
			&ErrorResponse{Reason: reasonInternal, Message: "render produced no captured result"}
	}

	// Mint id + path FIRST, write the wav, THEN commit (write-before-commit, so a
	// write fault leaves no map entry pointing at a missing file).
	id, wavPath, err := store.reserve()
	if err != nil {
		return narrateResponse{}, http.StatusInternalServerError,
			&ErrorResponse{Reason: reasonInternal, Message: err.Error()}
	}
	if err := writeRenderWAV(ctx, wavPath, capturer.plan, capturer.result); err != nil {
		// No commit, no map entry → nothing leaked; the orphan scan mops up any
		// partial file by mtime.
		return narrateResponse{}, http.StatusInternalServerError,
			&ErrorResponse{Reason: reasonRenderFailed, Message: "write render wav: " + err.Error()}
	}
	store.commit(id)

	return narrateResponse{
		AudioURL: "/audio/" + id + ".wav",
		Blocks:   capturer.plan.Blocks,
		Timeline: capturer.result.Timeline,
	}, http.StatusOK, nil
}
