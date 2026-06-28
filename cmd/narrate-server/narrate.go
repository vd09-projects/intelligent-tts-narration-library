package main

// narrate.go — POST /narrate (#109 Step 4, #125/#110 patchable-dir conversion).
// Inline TEXT only this phase (R2: no server-side `source` path in the REQUEST —
// a browser-reachable arbitrary-file-read on loopback). Decode {text, level,
// gender} → mint a render_id + its server-owned output dir → write the inline
// text to a server-MINTED {id}/source.txt (never a user path — R2 holds) →
// render through a fresh file-adapter pipeline with a capturingSink (no write) →
// write the 3-file persistent-sink dir ({id}/{audio.wav, plan.json,
// manifest.json}) → respond {audio_url, blocks, timeline}.
//
// Why a file source over source.txt (build-time correction to #109's R-NB4
// single-wav choice, recorded in decision-journal): the dir must be re-renderable
// by render_id so POST /narrate/block can escalate ONE block through the existing
// sink/persistent.PatchBlock core. PatchBlock re-reads the document source via
// the file adapter; the mcptext adapter holds text in-memory only and its URI is
// a one-way hash, so an mcp_text dir is NOT recoverable from disk. Persisting the
// text to a server-minted source.txt and sourcing it as a FILE makes each
// render_id dir a normal file-sourced persistent dir the escalate core reuses
// verbatim — no adapter change, no new patch logic, R2 intact (path is
// server-minted, not user-supplied).
//
// Refusal is HTTP 200 (R-NB1, CLAUDE.md honesty rule): an oversized-prose text
// with nil intelligence yields refused blocks whose refusal-notice is itself
// rendered to the wav — that is data, not an error. Only a hard render/sink
// fault is render_failed/500.

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"

	"github.com/vd09-projects/intelligent-tts-narration-library/adapter/file"
	"github.com/vd09-projects/intelligent-tts-narration-library/intelligence"
	"github.com/vd09-projects/intelligent-tts-narration-library/internal/errclass"
	"github.com/vd09-projects/intelligent-tts-narration-library/pipeline"
	"github.com/vd09-projects/intelligent-tts-narration-library/plan"
	"github.com/vd09-projects/intelligent-tts-narration-library/render"
	"github.com/vd09-projects/intelligent-tts-narration-library/render/sherpa"
	"github.com/vd09-projects/intelligent-tts-narration-library/sink/persistent"
)

// sourceLeaf is the server-minted inline-text file written inside each render_id
// dir. It is sourced as a FILE (not mcp_text) so POST /narrate/block can
// re-render any block by re-reading it through the file adapter. The path is
// always {tempRoot}/{id}/source.txt — server-minted, never user-supplied (R2).
const sourceLeaf = "source.txt"

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
// the file adapter (reading the server-minted {id}/source.txt — see file header)
// + nil intelligence (phase one; the planner's deterministic + degraded + refuse
// path is the honesty backbone) + the sherpa renderer + the caller's
// capturingSink. outDir is the render_id dir: the renderer writes its per-block
// wavs there and the persistent sink concatenates them into {outDir}/audio.wav
// alongside plan.json + manifest.json. Tests swap this var for a stub narrator
// (no Kokoro).
var newNarratePipeline = func(voice string, level plan.Level, outDir string, capturer *capturingSink) pipeline.Narrator {
	return pipeline.New(
		file.New(),
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

// writeRenderDir — seam over the 3-file persistent sink so tests can write a
// deterministic dir without a real render. Production writes the combined 24 kHz
// mono s16le audio.wav + plan.json + manifest.json into the render_id dir via
// persistent.New(dir).Consume, making the dir patchable by render_id (#125/#110).
// This SUPERSEDES #109's single-wav WAVFileSink for the SERVER /narrate path
// only; persistent.NewWAVFile (no sidecars) stays for speak_to_file / MCP.
var writeRenderDir = func(ctx context.Context, dir, voice string, p plan.NarrationPlan, res render.RenderResult) error {
	_, err := persistent.New(dir, persistent.WithVoice(voice)).Consume(ctx, p, res)
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

	// Mint id + its output dir FIRST; render + write the 3-file dir; THEN commit
	// (write-before-commit, so a fault before commit leaves at most an untracked
	// dir reaped by the orphan scan, never a map entry pointing at nothing). The
	// dir is the renderer's OutDir AND the persistent-sink target AND the served
	// artifact root — it is NOT removed on success (#125): it must survive so
	// POST /narrate/block can patch it by render_id.
	id, outDir, err := store.reserve()
	if err != nil {
		return narrateResponse{}, http.StatusInternalServerError,
			&ErrorResponse{Reason: reasonInternal, Message: err.Error()}
	}
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return narrateResponse{}, http.StatusInternalServerError,
			&ErrorResponse{Reason: reasonInternal, Message: "create render dir: " + err.Error()}
	}

	// Persist the inline text to a server-MINTED source.txt and source it as a
	// FILE so the dir is re-renderable by render_id (see file header). The path
	// is server-owned — never a user-supplied path — so R2 holds.
	srcPath := filepath.Join(outDir, sourceLeaf)
	if err := os.WriteFile(srcPath, []byte(text), 0o644); err != nil {
		_ = os.RemoveAll(outDir) // no commit → nothing leaked
		return narrateResponse{}, http.StatusInternalServerError,
			&ErrorResponse{Reason: reasonInternal, Message: "write source: " + err.Error()}
	}

	capturer := &capturingSink{}
	narrator := newNarratePipeline(voice, level, outDir, capturer)
	ref := plan.SourceRef{
		Kind:        plan.SourceKindFile,
		URI:         srcPath,
		ContentHash: contentHashOf(text),
	}
	if _, err := narrator.Narrate(ctx, ref, pipeline.NarrateRequest{Voice: voice}); err != nil {
		_ = os.RemoveAll(outDir)
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
		_ = os.RemoveAll(outDir)
		return narrateResponse{}, http.StatusInternalServerError,
			&ErrorResponse{Reason: reasonInternal, Message: "render produced no captured result"}
	}

	if err := writeRenderDir(ctx, outDir, voice, capturer.plan, capturer.result); err != nil {
		// No commit, no map entry → nothing leaked; the orphan scan mops up any
		// partial dir by mtime.
		_ = os.RemoveAll(outDir)
		return narrateResponse{}, http.StatusInternalServerError,
			&ErrorResponse{Reason: reasonRenderFailed, Message: "write render dir: " + err.Error()}
	}
	store.commit(id)

	return narrateResponse{
		AudioURL: "/audio/" + id + ".wav",
		Blocks:   capturer.plan.Blocks,
		Timeline: capturer.result.Timeline,
	}, http.StatusOK, nil
}

// contentHashOf computes the document content hash the file adapter derives from
// source bytes (sha256 hex). Stamped onto the SourceRef so the persisted
// manifest's ContentHash matches what a later /narrate/block patch re-derives —
// keeping PatchBlock's content-hash gate green across the round-trip.
func contentHashOf(text string) string {
	sum := sha256.Sum256([]byte(text))
	return hex.EncodeToString(sum[:])
}

// narrateBlockRequest — POST /narrate/block body. snake_case, DisallowUnknownFields.
// render_id keys the server-internal output dir a prior POST /narrate minted;
// block_id + level select the one block to re-render. There is deliberately NO
// `dir` field — the dir is resolved INTERNALLY from render_id (store.dirFor), so
// the #109 arbitrary-file-read vector stays closed.
type narrateBlockRequest struct {
	RenderID string `json:"render_id"`
	BlockID  string `json:"block_id"`
	Level    int    `json:"level"`
}

// narrateBlockResponse — the 200 body. It embeds /escalate's field set {block,
// timing?, refusal?} but SUBSTITUTES audio_url (the stable /audio/{render_id}.wav,
// served Cache-Control: no-store by GET /audio) for /escalate's audio_ref +
// /artifact?dir= pair: the whole-file playback unit is unchanged (audio.wav is
// rewritten in place at the same render_id), so Earshot re-points only the
// playing block to the same URL. /escalate's own escalateResponse stays FROZEN.
// On a refusal, timing is omitted and refusal is populated (same nullability).
type narrateBlockResponse struct {
	Block    plan.Block        `json:"block"`
	Timing   *plan.BlockTiming `json:"timing,omitempty"`
	Refusal  *plan.Refusal     `json:"refusal,omitempty"`
	AudioURL string            `json:"audio_url"`
}

// narrateBlockHandler returns the POST /narrate/block handler bound to args + the
// store. It validates the request, resolves render_id → server-internal dir, and
// funnels through the shared escalateInDir core (same single-block patch path as
// /escalate), then shapes the narrate-flavored response.
func narrateBlockHandler(args serverArgs, store *renderStore) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeError(w, http.StatusMethodNotAllowed, reasonMethodNotAllowed, "POST only")
			return
		}

		var req narrateBlockRequest
		dec := json.NewDecoder(r.Body)
		dec.DisallowUnknownFields()
		if err := dec.Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, reasonMissingField, "malformed JSON body: "+err.Error())
			return
		}
		if req.RenderID == "" {
			writeError(w, http.StatusBadRequest, reasonMissingField, "render_id is required")
			return
		}
		if !renderIDPattern.MatchString(req.RenderID) {
			writeError(w, http.StatusBadRequest, reasonMissingField,
				"render_id must be 32 hex chars: "+req.RenderID)
			return
		}
		if req.BlockID == "" {
			writeError(w, http.StatusBadRequest, reasonMissingField, "block_id is required")
			return
		}
		if req.Level < 1 || req.Level > 3 {
			writeError(w, http.StatusBadRequest, reasonInvalidLevel,
				fmt.Sprintf("level must be 1, 2, or 3 (got %d)", req.Level))
			return
		}

		resp, status, errResp := runNarrateBlock(r.Context(), args, store, req)
		if errResp != nil {
			writeError(w, status, errResp.Reason, errResp.Message)
			return
		}
		writeJSON(w, status, resp)
	})
}

// runNarrateBlock is the handler core (pulled out so tests assert the mapping
// directly). It resolves render_id → server-internal dir (unknown/expired → 404
// source_not_found), runs the shared escalateInDir core, and substitutes the
// stable audio_url for /escalate's audio_ref. The patched audio.wav is rewritten
// in place at the same render_id, so the URL is unchanged across escalations.
func runNarrateBlock(reqCtx context.Context, args serverArgs, store *renderStore, req narrateBlockRequest) (narrateBlockResponse, int, *ErrorResponse) {
	// Per-request timeout bounds a hung Kokoro re-render (mirrors /escalate).
	ctx, cancel := context.WithTimeout(reqCtx, args.RequestTimeout)
	defer cancel()

	absDir, status, errResp := store.dirFor(req.RenderID)
	if errResp != nil {
		return narrateBlockResponse{}, status, errResp
	}

	esc, status, errResp := escalateInDir(ctx, args, absDir, req.BlockID, plan.Level(req.Level))
	if errResp != nil {
		return narrateBlockResponse{}, status, errResp
	}

	return narrateBlockResponse{
		Block:    esc.Block,
		Timing:   esc.Timing,
		Refusal:  esc.Refusal,
		AudioURL: "/audio/" + req.RenderID + ".wav",
	}, http.StatusOK, nil
}
