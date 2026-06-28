// Command narrate-server — localhost-only HTTP edge for single-block
// escalation. Exposes POST /escalate and GET /healthz, wrapping the existing
// pipeline.Narrate + sink/persistent.PatchBlock path (issue #49).
//
// It is the third composition-root consumer (sibling to cmd/narrate and
// cmd/narrate-mcp): same pipeline, different surface. It re-renders ONE block
// at a new level and patches it into an existing persistent-sink directory,
// leaving every other block byte-identical. It unblocks the React player (#50)
// driving escalation live without copying a CLI command.
//
// # Surface contract (the #50-facing contract — read before changing)
//
//	POST /escalate  {dir, block_id, level} (snake_case JSON)
//	  200 voiced/degraded → {block, timing, audio_ref}
//	  200 refused         → {block, refusal}            (timing + audio_ref ABSENT, not null)
//	  non-2xx             → ErrorResponse{reason, message}  (single envelope, always)
//	GET  /healthz   → 200 {"status":"ok"}
//	GET  /artifact?dir=&name=  → 200 file bytes (audio.wav | manifest.json | plan.json ONLY)
//	  non-2xx             → ErrorResponse{reason, message}  (same envelope as /escalate)
//	GET  /readiness?dir=  → 200 {"status":"rendered"|"rendering"}  (companion render handshake, #76)
//	  404                  → ErrorResponse{reason:"no_out_dir"}    (dir absent — terminal token)
//
// # narrate-server HTTP bridge routes (#109 — the Earshot-UI-facing contract)
//
//	GET  /sessions/{id}/messages → 200 {session_id, messages[]}    (ordered, structurally pre-chunked)
//	  messages[].id is a server StableID (line uuid, else "h:"-hash) — NEVER Turn (#106)
//	  blocks carry source text + coarse class ONLY: no voicing, no refusal at this route (R3)
//	  400 missing_field (bad/empty/".." id) | 404 source_not_found (no jsonl) | 500 internal (glob/parse)
//	POST /narrate  {text, level, gender}  → 200 {audio_url, blocks[], timeline}   (inline TEXT only — R2)
//	  200 refused → {audio_url, blocks[] (refused), timeline}   (refusal is data, HTTP 200 — R-NB1)
//	  400 missing_field (no text / unknown gender) | 400 invalid_level | 408 cancelled | 500 render_failed
//	GET  /audio/{render_id}.wav → 200 audio/wav bytes (Cache-Control: no-store)
//	  400 missing_field (id ≁ ^[0-9a-f]{32}$) | 404 source_not_found (unknown/expired) | 405 method_not_allowed
//
//	Containment (both /artifact and /audio): resolveWithin = EvalSymlinks +
//	filepath.Rel boundary check (never raw string-prefix). The minted-wav
//	lifecycle is a TTL GC reaper (--audio-ttl) + shutdown RemoveAll back-stop.
//	render_failed is the ONLY new reason token this surface adds.
//
// # Static artifact route (#62) — re-fetch the just-patched output over HTTP
//
//	After an in-place escalate the React player must re-read the patched
//	artifacts (audio.wav, manifest.json) so downstream offsets + audio reflect
//	the patched outDir — but in server mode that outDir is an arbitrary path the
//	user typed, NOT the bundled fixture. GET /artifact serves exactly those two
//	files out of exactly that one user-supplied dir so the player can resolve its
//	re-fetch base against the served dir instead of the fixture origin.
//
//	Containment guarantee: the route serves ONLY {manifest.json, audio.wav} and
//	ONLY from within the requested dir. The served path is resolved with
//	filepath.EvalSymlinks and confirmed to stay inside the (also symlink-resolved)
//	dir via a filepath.Rel boundary check — NOT a raw string-prefix check, so a
//	sibling dir sharing a name prefix (…/base vs …/base-evil) cannot bypass it,
//	and a symlink whose target escapes the dir is rejected. No directory listing,
//	no browse root, no arbitrary filename. It inherits the loopback-only bind and
//	the pinned single-origin CORS unchanged (never widens either).
//
// ErrorResponse.reason is drawn from a CLOSED, APPEND-ONLY enum (a token is
// never removed or repurposed — the #50 client switches on these strings; only
// ADD, and add to this table when you do):
//
//	invalid_level           400  caller   level ∉ {1,2,3}
//	missing_field           400  caller   malformed JSON / missing dir or block_id
//	method_not_allowed      405  caller   wrong HTTP method (GET on /escalate, POST on /healthz)
//	source_not_found        400  caller   manifest present-but-unparseable / permission-denied; OR incomplete persistent-sink dir (ErrNothingToPatch, a non-fs.ErrNotExist file fault)
//	source_not_found        404  caller   manifest cleanly absent (fs.ErrNotExist); OR incomplete dir whose missing file surfaces as fs.ErrNotExist
//	stale_patch             409  caller   ErrStalePatch
//	content_hash_mismatch   409  caller   ErrContentHashMismatch
//	unknown_block           409  caller   ErrUnknownBlock
//	container_mismatch      409  caller   ErrContainerMismatch
//	format_mismatch         409  caller   ErrFormatMismatch
//	cancelled               408  cancel   context.Canceled / context.DeadlineExceeded
//	readback_failed         500  internal post-patch plan/manifest read failed
//	no_out_dir              404  caller   GET /readiness ?dir does not exist (typo / wrong path / render never started)
//	render_failed           500  internal POST /narrate hard render/sink fault (NOT a refusal — refusal is 200) (#109)
//	internal                500  internal render/sink hard fault, corrupt manifest, …
//
//	source_not_found is ONE token with a DETERMINISTIC status: 404 iff the
//	underlying file fault is fs.ErrNotExist, else 400. (R7)
//
//	# Idempotence + recovery (B3/B5) — convergence is by RE-RENDER, not a no-op
//	# short-circuit (corrected in build-review B-1)
//
//	persistent.PatchBlock has NO "block already at the requested level"
//	detection: an escalate that lands on the level the block already holds is
//	NOT short-circuited. The block is re-rendered to content-identical bytes and
//	re-patched, and the endpoint returns 200 with the current block (B3 idempotence
//	by content-identical re-render). Likewise, after a readback_failed (500) the
//	three files all exist (the patch committed), so re-issuing the IDENTICAL POST
//	re-renders + re-patches and reads back cleanly → 200 (B5 recovery, also by
//	re-render). Neither path goes through ErrNothingToPatch.
//
//	ErrNothingToPatch fires ONLY for an INCOMPLETE persistent-sink dir (its
//	pre-flight finds manifest.json / audio.wav / plan.json missing). That is a
//	caller-correctable source-resolution failure, so it maps to source_not_found
//	(404 iff the missing file surfaces as fs.ErrNotExist, else 400) — NOT a 200,
//	and NOT readback_failed. There is NO GET/read route by design.
//	Refused-at-new-level is NOT in this table: it is 200 with a refusal payload
//	(refusal-is-data invariant).
//
// # Operational notes
//
//   - Loopback-only, enforced at startup: the server REFUSES to start (non-zero
//     exit) on any --addr host that is not 127.0.0.1 / ::1 / localhost,
//     explicitly including 0.0.0.0, ::, and the empty host (:8080). (B4)
//   - CORS is pinned to the configured --cors-origin literal; the request Origin
//     header is NEVER echoed back, even on a match. (R5)
//   - Per-dir write serialization: concurrent escalates on the same dir are
//     serialized by an in-process per-dir mutex (sync.Map.LoadOrStore + defer
//     Unlock). Different dirs run free. The mutex registry is never evicted —
//     accepted phase one, bounded by distinct dirs per server lifetime. (B1/R10)
//   - Single-writer assumption: this in-process mutex serializes within THIS
//     server only, not across processes or external writers. Local-trust phase
//     one makes that sufficient.
//   - Read-during-write consistency (#62): GET /artifact takes the SAME per-dir
//     mutex /escalate writers hold, so a re-fetch issued mid-patch observes the
//     fully-old or fully-new artifact triple — never a torn cross-file state.
//     persistent.PatchBlock writes both manifest.json and audio.wav via atomic
//     tmp+rename, but commits them as SEQUENTIAL renames (plan, manifest, audio
//     LAST), so without the read-side lock a reader could observe new manifest +
//     old audio. The lock closes that window; the cross-file torn state is not
//     left as accepted debt.
//   - Every request runs under a per-request context.WithTimeout (--request-
//     timeout, default 120s) so a hung Kokoro render cannot hold the per-dir
//     mutex for the server lifetime; a timeout surfaces as 408 cancelled. (R2c)
//
// Exit codes:
//
//	0 — clean shutdown (signal).
//	1 — invalid --addr (non-loopback) or transport/server error.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/vd09-projects/intelligent-tts-narration-library/adapter/file"
	"github.com/vd09-projects/intelligent-tts-narration-library/intelligence"
	"github.com/vd09-projects/intelligent-tts-narration-library/internal/errclass"
	"github.com/vd09-projects/intelligent-tts-narration-library/pipeline"
	"github.com/vd09-projects/intelligent-tts-narration-library/plan"
	"github.com/vd09-projects/intelligent-tts-narration-library/render"
	"github.com/vd09-projects/intelligent-tts-narration-library/render/sherpa"
	"github.com/vd09-projects/intelligent-tts-narration-library/sink"
	"github.com/vd09-projects/intelligent-tts-narration-library/sink/persistent"
)

// genderToVoice — phase-one mapping per CLAUDE.md domain rules + sindri
// patterns A15 amendment. Re-stated here (not imported from cmd/narrate) so the
// cmd packages stay independent — matches the project's existing pattern.
var genderToVoice = map[string]string{
	"female": "af_bella",
	"male":   "am_michael",
}

// loopbackHosts is the closed set of host literals the server will bind to.
// Anything else (0.0.0.0, ::, a LAN IP, a hostname, or the empty host in
// ":8080") is refused at startup (B4). This is a startup constraint, not a
// runtime request filter.
var loopbackHosts = map[string]bool{
	"127.0.0.1": true,
	"::1":       true,
	"localhost": true,
}

// reason tokens — the closed, append-only enum. Declared as named constants so
// the classifier and the tests share one source of truth and a typo cannot
// silently invent a new token.
//
// CLOSED + APPEND-ONLY (restated; see the surface-contract header): a token is
// NEVER removed or repurposed once shipped — the #50 client switches on these
// machine-stable strings, so changing a token's meaning is a silent contract
// break. Only ADD new tokens (and add them to the header table too).
const (
	reasonInvalidLevel        = "invalid_level"
	reasonMissingField        = "missing_field"
	reasonMethodNotAllowed    = "method_not_allowed"
	reasonSourceNotFound      = "source_not_found"
	reasonStalePatch          = "stale_patch"
	reasonContentHashMismatch = "content_hash_mismatch"
	reasonUnknownBlock        = "unknown_block"
	reasonContainerMismatch   = "container_mismatch"
	reasonFormatMismatch      = "format_mismatch"
	reasonCancelled           = "cancelled"
	reasonReadbackFailed      = "readback_failed"
	// reasonNoOutDir is the readiness-handshake terminal token (#76): GET
	// /readiness was asked about a dir that does not exist. Appended to the
	// closed enum (never repurposing source_not_found — that token stays scoped
	// to the escalate/artifact source-resolution path; the #50 client switches
	// on these strings). The player converts a no_out_dir into its defined
	// render_failed terminal state immediately.
	reasonNoOutDir = "no_out_dir"
	reasonInternal = "internal"
	// reasonRenderFailed is the SOLE new token added by #109 (the narrate-server
	// HTTP bridge). POST /narrate returns it on a hard render/sink fault (Kokoro
	// fault, wav-write failure) — distinct from a refusal, which is HTTP 200 with
	// refused blocks (honesty rule), not an error token. Appended to the closed
	// enum (never repurposing an existing token); the Earshot UI switches on it.
	reasonRenderFailed = "render_failed"
)

// writeTimeoutCushion is the fixed headroom added to the per-request render
// timeout to derive the http.Server WriteTimeout. WriteTimeout must always be
// strictly greater than the request timeout so the handler still has an open
// connection to write the 408 body after the per-request context fires; this
// named const keeps that relationship explicit so a future edit cannot set
// WriteTimeout below RequestTimeout and silently truncate the 408 (S-4).
const writeTimeoutCushion = 30 * time.Second

// serverArgs — parsed flags. Pulled out so newPipeline and the handler read
// the same shape and tests can construct it directly.
type serverArgs struct {
	Addr           string
	CORSOrigin     string
	RequestTimeout time.Duration
	Gender         string
	// AudioTTL bounds how long a /narrate-minted wav lives in the server temp
	// store before the GC reaper removes it (#109 Step 7, the AC5 "settle").
	AudioTTL time.Duration
}

// escalateRequest — POST /escalate body. snake_case JSON tags (the #50
// contract). Level is the absolute target level for the one block.
type escalateRequest struct {
	Dir     string `json:"dir"`
	BlockID string `json:"block_id"`
	Level   int    `json:"level"`
}

// escalateResponse — the 200 success body. block ALWAYS carries status so the
// player distinguishes degraded from voiced (honesty rule covers degradation,
// not just refusal). On a refusal, Timing + AudioRef are OMITTED (omitempty /
// absent, not null) and Refusal is populated; on voiced/degraded, Timing +
// AudioRef are present and Refusal is omitted (Step 5 nullability rule).
type escalateResponse struct {
	Block    plan.Block        `json:"block"`
	Timing   *plan.BlockTiming `json:"timing,omitempty"`
	AudioRef string            `json:"audio_ref,omitempty"`
	Refusal  *plan.Refusal     `json:"refusal,omitempty"`
}

// readiness status tokens — the GET /readiness 200 body values (#76). Named
// consts (not inline literals) to match the reason* token discipline above: the
// #76 companion client switches on these machine-stable strings, so a typo in a
// bare literal would be a silent contract drift the compiler cannot catch.
const (
	statusRendered  = "rendered"
	statusRendering = "rendering"
)

// readinessResponse — the GET /readiness 200 body (#76). Status is statusRendered
// (plan.json + manifest.json + audio.wav all present & non-empty) or statusRendering
// (dir exists but the triple is incomplete / mid-write). A missing dir is NOT a
// readiness state — it is the no_out_dir terminal token on a 404 ErrorResponse,
// so the player can distinguish "still rendering" from "wrong path / failed".
type readinessResponse struct {
	Status string `json:"status"`
}

// ErrorResponse — the single non-2xx envelope. Reason is a machine-stable token
// from the closed enum; Message is human-actionable.
type ErrorResponse struct {
	Reason  string `json:"reason"`
	Message string `json:"message"`
}

// capturingSink is a non-writing OutputSink — same role as cmd/narrate's
// (issue #28). The pipeline re-renders the target block and calls Consume with
// the one-block sub-plan + sub-result; capturingSink records them (never
// touching the filesystem) so the handler can hand them to persistent.PatchBlock.
// The returned receipt reports the single re-rendered block so NarrateResult is
// well-formed; the authoritative patch receipt comes from PatchBlock.
type capturingSink struct {
	plan     plan.NarrationPlan
	result   render.RenderResult
	captured bool
}

func (c *capturingSink) Consume(_ context.Context, p plan.NarrationPlan, res render.RenderResult) (sink.SinkReceipt, error) {
	c.plan = p
	c.result = res
	c.captured = true
	var rec sink.SinkReceipt
	for _, bt := range res.Timeline.Blocks {
		rec.TotalDurationMs += int64(bt.EndMs - bt.StartMs)
		if bt.AudioRef != "" {
			rec.BlocksPlayed++
		}
	}
	return rec, nil
}

// newPipeline — package-level factory hook (mirrors cmd/narrate's seam). The
// capturing sink is wired in so the pipeline re-renders the block WITHOUT
// writing; the handler then calls patchBlock explicitly to splice it into the
// existing outDir (exactly cmd/narrate's --block × --sink=persistent path,
// main.go:228). Production builds the real pipeline.Pipeline; tests swap this
// var to inject a stub narrator that populates the same capturer — so no Kokoro
// subprocess spawns in unit tests.
//
// Intelligence is nil phase one (the escalate edge has no intelligence flag);
// the planner's deterministic + degraded + refuse path is the honesty backbone.
var newPipeline = func(outDir string, args serverArgs, capturer *capturingSink) pipeline.Narrator {
	return pipeline.New(
		file.New(),
		intelligence.IntelligenceAdapter(nil),
		sherpa.New(sherpa.EngineConfig{}),
		capturer,
		pipeline.PipelineDefaults{
			// F2 (mirrors cmd/narrate): the document default level stays L1 on a
			// single-block re-render so the planner does NOT re-plan every
			// untargeted block at the escalation level. The target block's level
			// is funneled through LevelOverrides in the handler.
			Level:  plan.L1,
			OutDir: outDir,
			Locale: "en",
		},
	)
}

// patchBlock — seam over persistent.PatchBlock so tests can inject the patch
// sentinels (ErrNothingToPatch, ErrStalePatch, …) and the readback scenarios
// without driving a real sink. Production is the real func.
var patchBlock = persistent.PatchBlock

// readManifest / readPlanFile — readback seams. readManifest defaults to the
// additive persistent.ReadManifest (R1); readPlanFile reads plan.json into a
// plan.NarrationPlan locally (plan.json is plain plan-schema JSON, so no
// persistent helper is needed and plan/ stays untouched).
var (
	readManifest = persistent.ReadManifest
	readPlanFile = func(dir string) (plan.NarrationPlan, error) {
		raw, err := os.ReadFile(filepath.Join(dir, "plan.json"))
		if err != nil {
			return plan.NarrationPlan{}, fmt.Errorf("read plan.json: %w", err)
		}
		var p plan.NarrationPlan
		if err := json.Unmarshal(raw, &p); err != nil {
			return plan.NarrationPlan{}, fmt.Errorf("parse plan.json: %w", err)
		}
		return p, nil
	}
)

// dirLocks is the per-dir mutex registry (B1). Lazily populated via
// LoadOrStore so two first-touchers cannot install competing mutexes (R2b).
// Never evicted — accepted phase one, bounded by distinct dirs touched per
// server lifetime (R10).
var dirLocks sync.Map // map[absDirPath]*sync.Mutex

func main() {
	args, err := parseFlags(os.Args[1:], os.Stderr)
	if err != nil {
		// Flag parse error already printed by the flag package.
		os.Exit(2)
	}

	// Loopback enforcement — refuse to start on any non-loopback host (B4).
	if err := validateAddr(args.Addr); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, "narrate-server: "+err.Error())
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if err := serve(ctx, args, os.Stderr); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, "narrate-server: "+err.Error())
		os.Exit(1)
	}
}

// parseFlags parses the server flags from argv. Pulled out so tests can drive
// it without os.Args. errOut receives the flag package's usage on error.
func parseFlags(argv []string, errOut io.Writer) (serverArgs, error) {
	fs := flag.NewFlagSet("narrate-server", flag.ContinueOnError)
	fs.SetOutput(errOut)
	var a serverArgs
	fs.StringVar(&a.Addr, "addr", "127.0.0.1:8080", "loopback host:port to bind (host must be 127.0.0.1, ::1, or localhost)")
	fs.StringVar(&a.CORSOrigin, "cors-origin", "http://localhost:5173", "the single allowed CORS origin (emitted literally; the request Origin is never echoed)")
	fs.DurationVar(&a.RequestTimeout, "request-timeout", 120*time.Second, "per-request render timeout (bounds a hung Kokoro render)")
	fs.DurationVar(&a.AudioTTL, "audio-ttl", 30*time.Minute, "how long a /narrate-minted wav lives before the GC reaper removes it")
	fs.StringVar(&a.Gender, "gender", "female", "voice gender for the re-render: female | male")
	if err := fs.Parse(argv); err != nil {
		return serverArgs{}, err
	}
	if _, ok := genderToVoice[a.Gender]; !ok {
		_, _ = fmt.Fprintf(errOut, "narrate-server: --gender must be female or male (got %q)\n", a.Gender)
		return serverArgs{}, fmt.Errorf("invalid --gender %q", a.Gender)
	}
	return a, nil
}

// validateAddr enforces the loopback-only bind (B4). The host portion of addr
// must be a loopback literal; a non-loopback host (incl. 0.0.0.0, ::, and the
// empty host in ":8080") is refused. Returns an error to the caller, which
// refuses to start. Tested directly — unit tests do NOT bind a port.
func validateAddr(addr string) error {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return fmt.Errorf("invalid --addr %q: %w", addr, err)
	}
	if !loopbackHosts[host] {
		return fmt.Errorf(
			"refusing to bind non-loopback host %q (from --addr %q): only 127.0.0.1, ::1, or localhost are allowed; %q binds beyond loopback",
			host, addr, addr)
	}
	return nil
}

// serve builds the mux and runs the http.Server until ctx is cancelled. The
// addr is assumed already validated by validateAddr.
func serve(ctx context.Context, args serverArgs, logOut io.Writer) error {
	// Server-owned audio temp root for /narrate-minted wavs (#109). A fresh
	// MkdirTemp per server lifetime; RemoveAll on shutdown is the back-stop GC.
	tempRoot, err := os.MkdirTemp("", "narrate-server-audio-*")
	if err != nil {
		return fmt.Errorf("create audio temp root: %w", err)
	}
	defer func() { _ = os.RemoveAll(tempRoot) }()
	store := newRenderStore(tempRoot)
	// TTL reaper: snapshots expired entries under the store write-lock then
	// deletes lock-free (R1). Stops on ctx cancel.
	go store.runReaper(ctx, defaultReaperInterval, args.AudioTTL)

	srv := &http.Server{
		Addr:    args.Addr,
		Handler: newMux(args, store),
		// Belt-and-suspenders ceiling (R2c): the per-request context.WithTimeout
		// in the handler is the primary render bound; WriteTimeout ≥ it backs it.
		ReadHeaderTimeout: 10 * time.Second,
		WriteTimeout:      args.RequestTimeout + writeTimeoutCushion,
	}

	ln, err := net.Listen("tcp", args.Addr)
	if err != nil {
		return fmt.Errorf("bind %s: %w", args.Addr, err)
	}
	_, _ = fmt.Fprintf(logOut, "narrate-server: listening on http://%s (cors-origin %s, request-timeout %s)\n",
		ln.Addr(), args.CORSOrigin, args.RequestTimeout)

	errc := make(chan error, 1)
	go func() {
		err := srv.Serve(ln)
		if errors.Is(err, http.ErrServerClosed) {
			err = nil
		}
		errc <- err
	}()

	select {
	case <-ctx.Done():
		shutCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutCtx)
		return nil
	case err := <-errc:
		return err
	}
}

// newMux wires the routes with the CORS middleware. Exposed for tests so they
// drive the handler in-process via httptest without binding a port.
func newMux(args serverArgs, store *renderStore) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", healthzHandler)
	mux.Handle("/escalate", escalateHandler(args))
	mux.HandleFunc("/artifact", artifactHandler)
	mux.HandleFunc("/readiness", readinessHandler)
	// #109 narrate-server HTTP bridge — three new routes. Go 1.22 method+wildcard
	// patterns: {id}/{file} are whole-segment wildcards (so /audio uses {file}
	// and strips the ".wav" suffix inside, since a wildcard cannot be a partial
	// segment). Method is checked inside each handler so a wrong method yields the
	// shared ErrorResponse envelope (not the mux's default 405 body).
	mux.HandleFunc("/sessions/{id}/messages", messagesHandler)
	mux.Handle("/narrate", narrateHandler(args, store))
	mux.HandleFunc("/audio/{file}", store.serveAudio)
	return withCORS(args.CORSOrigin, mux)
}

// withCORS pins Access-Control-Allow-Origin to the configured literal and
// answers OPTIONS preflight (R5). It NEVER echoes the request Origin header,
// even on a match — emitting the configured literal is the whole point.
func withCORS(origin string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Pin the configured literal only. The request Origin is intentionally
		// ignored: a disallowed origin simply does not receive the allow header
		// (the browser blocks it); a spoofed Origin is never reflected.
		w.Header().Set("Access-Control-Allow-Origin", origin)
		w.Header().Set("Vary", "Origin")
		if r.Method == http.MethodOptions {
			// GET added for the /artifact static-artifact route (#62); POST stays
			// for /escalate. One pinned method list across the shared middleware.
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
			w.Header().Set("Access-Control-Max-Age", "600")
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func healthzHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, reasonMethodNotAllowed, "GET only")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// escalateHandler returns the POST /escalate handler bound to args.
func escalateHandler(args serverArgs) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeError(w, http.StatusMethodNotAllowed, reasonMethodNotAllowed, "POST only")
			return
		}

		// Step 3 — decode + validate.
		var req escalateRequest
		dec := json.NewDecoder(r.Body)
		dec.DisallowUnknownFields()
		if err := dec.Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, reasonMissingField, "malformed JSON body: "+err.Error())
			return
		}
		if req.Dir == "" {
			writeError(w, http.StatusBadRequest, reasonMissingField, "dir is required")
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

		resp, status, errResp := runEscalate(r.Context(), args, req)
		if errResp != nil {
			writeError(w, status, errResp.Reason, errResp.Message)
			return
		}
		writeJSON(w, status, resp)
	})
}

// artifactAllowlist is the closed set of filenames GET /artifact will serve.
// Exact-match only — a request for anything else (incl. plan.json, source.md, a
// path containing a separator, or "..") is a missing_field rejection. This is
// the first containment gate, before any filesystem touch.
var artifactAllowlist = map[string]string{
	"audio.wav":     "audio/wav",
	"manifest.json": "application/json",
	// plan.json is allowlisted (#76) so the companion player can HTTP-load the
	// INITIAL triple over the existing /artifact route (it polls one origin; the
	// disk picker cannot be auto-polled). The containment machinery
	// (resolveArtifactPath: EvalSymlinks + filepath.Rel) is unchanged and now
	// covers plan.json for free.
	"plan.json": "application/json",
}

// artifactHandler serves GET /artifact?dir=&name= — the static re-fetch route
// (#62). It serves ONLY {manifest.json, audio.wav} and ONLY from inside the
// requested dir, resolving symlinks and confirming containment via a
// filepath.Rel boundary check (not a raw string prefix). Errors map onto the
// existing closed reason enum only — no new tokens. It takes the per-dir mutex
// READ side (the same mutex /escalate writers hold) so a re-fetch mid-patch sees
// a consistent artifact triple, never a torn cross-file state.
func artifactHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, reasonMethodNotAllowed, "GET only")
		return
	}

	dir := r.URL.Query().Get("dir")
	name := r.URL.Query().Get("name")
	if dir == "" {
		writeError(w, http.StatusBadRequest, reasonMissingField, "dir is required")
		return
	}
	if name == "" {
		writeError(w, http.StatusBadRequest, reasonMissingField, "name is required")
		return
	}

	// Allowlist gate FIRST (before any fs touch). Exact-match against the closed
	// set; a name carrying a path separator or ".." is never in the map and is
	// rejected here as missing_field — a malformed artifact name, not a 404.
	contentType, ok := artifactAllowlist[name]
	if !ok {
		writeError(w, http.StatusBadRequest, reasonMissingField,
			fmt.Sprintf("name must be one of audio.wav, manifest.json, or plan.json (got %q)", name))
		return
	}
	// Defence in depth: even though the allowlist is exact-match, reject any name
	// that contains a separator or "..". A future allowlist edit cannot smuggle a
	// traversal segment past this guard.
	if strings.ContainsRune(name, '/') || strings.ContainsRune(name, os.PathSeparator) || strings.Contains(name, "..") {
		writeError(w, http.StatusBadRequest, reasonMissingField,
			fmt.Sprintf("name must not contain a path separator or %q (got %q)", "..", name))
		return
	}

	// Resolve the requested dir to an absolute path — this is also the per-dir
	// mutex key, the SAME key /escalate uses (filepath.Abs of req.Dir), so a
	// concurrent escalate on the same dir serializes against this read.
	absDir, err := filepath.Abs(dir)
	if err != nil {
		writeError(w, http.StatusBadRequest, reasonMissingField, "resolve dir: "+err.Error())
		return
	}

	// Take the per-dir lock READ side for the duration of resolve+open+serve so a
	// mid-patch reader observes the fully-old or fully-new triple. Shares the
	// /escalate write-side mutex registry (sync.Mutex has no separate read lock;
	// a full Lock is correct and the read path is short).
	muIface, _ := dirLocks.LoadOrStore(absDir, &sync.Mutex{})
	mu := muIface.(*sync.Mutex)
	mu.Lock()
	defer mu.Unlock()

	served, status, errResp := resolveWithin(absDir, name)
	if errResp != nil {
		writeError(w, status, errResp.Reason, errResp.Message)
		return
	}

	f, err := os.Open(served) //nolint:gosec // path is containment-checked above
	if err != nil {
		// A file that EvalSymlinks resolved cleanly but then fails to open is the
		// only genuinely unexpected fault here → internal (500). A not-exist at
		// open time (raced delete) is still caller-correctable → source_not_found.
		if errors.Is(err, fs.ErrNotExist) {
			writeError(w, http.StatusNotFound, reasonSourceNotFound, "artifact not found: "+name)
			return
		}
		writeError(w, http.StatusInternalServerError, reasonInternal, "open artifact: "+err.Error())
		return
	}
	defer func() { _ = f.Close() }()

	info, err := f.Stat()
	if err != nil {
		writeError(w, http.StatusInternalServerError, reasonInternal, "stat artifact: "+err.Error())
		return
	}

	// Pin the Content-Type from the allowlist (do NOT let http sniff it) and
	// forbid caching so a re-fetch always sees the just-patched bytes.
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Cache-Control", "no-store")
	// http.ServeContent (not http.FileServer/http.Dir) — no directory listing, no
	// implicit index, just these bytes. It honors Range + sets Content-Length.
	http.ServeContent(w, r, name, info.ModTime(), f)
}

// resolveWithin resolves name inside base to a fully symlink-resolved leaf and
// confirms it stays inside the symlink-resolved base via a filepath.Rel boundary
// check. It returns the path to serve, or an ErrorResponse + HTTP status drawn
// ONLY from the closed enum. A non-existent dir/leaf or any containment failure
// is source_not_found (404) — never a 500. internal (500) is reserved for
// unexpected faults at open time (handled by the caller), not for resolution
// failures here.
//
// Generalised from the original resolveArtifactPath (#62) so BOTH GET /artifact
// (base = the user-supplied dir, name = an allowlisted filename) and GET /audio
// (base = the server temp root, name = {render_id}.wav) share one containment
// proof (#109 R-NB3). The existing /artifact symlink-escape tests re-run against
// this helper unchanged.
func resolveWithin(base, name string) (string, int, *ErrorResponse) {
	// Build the FULL joined leaf, then EvalSymlinks the WHOLE thing — so a
	// symlinked leaf whose target escapes the base is caught by the boundary check
	// below (resolution happens before, not after, the check).
	leaf := filepath.Join(base, name)
	resolvedLeaf, err := filepath.EvalSymlinks(leaf)
	if err != nil {
		// EvalSymlinks failure is a CLEAN reject, never a 500: ErrNotExist (file
		// absent / dangling symlink) and any other resolve failure both map to
		// source_not_found (404). We cannot serve a path we cannot resolve.
		return "", http.StatusNotFound, &ErrorResponse{
			Reason:  reasonSourceNotFound,
			Message: "artifact not found: " + name,
		}
	}

	// Independently resolve the base dir's symlinks so the boundary check
	// compares fully-resolved against fully-resolved (otherwise a symlinked dir
	// would spuriously fail containment).
	resolvedBase, err := filepath.EvalSymlinks(base)
	if err != nil {
		return "", http.StatusNotFound, &ErrorResponse{
			Reason:  reasonSourceNotFound,
			Message: "artifact dir not found",
		}
	}

	// Boundary check via filepath.Rel — NOT raw strings.HasPrefix, which would let
	// a sibling sharing a name prefix (…/base vs …/base-evil) slip through.
	// Containment holds ONLY if rel is computable, is not "..", does not start
	// with "../", and is not absolute.
	rel, err := filepath.Rel(resolvedBase, resolvedLeaf)
	if err != nil ||
		rel == ".." ||
		strings.HasPrefix(rel, ".."+string(os.PathSeparator)) ||
		filepath.IsAbs(rel) {
		return "", http.StatusNotFound, &ErrorResponse{
			Reason:  reasonSourceNotFound,
			Message: "artifact not found: " + name,
		}
	}

	return resolvedLeaf, http.StatusOK, nil
}

// readinessLeaves are the three artifacts that must ALL be present and non-empty
// for a dir to read as "rendered" — the same triple loadFromServer fetches and
// loadDirectory requires. A missing or zero-byte leaf (mid-write / torn) keeps
// the dir at "rendering" so the player re-polls instead of loading a half-write.
var readinessLeaves = []string{"plan.json", "manifest.json", "audio.wav"}

// readinessHandler serves GET /readiness?dir= — the companion-player render
// handshake (#76). The render is a SEPARATE cmd/narrate --sink persistent
// process, so the server reports only the on-disk truth it can see; it never
// authors "the render crashed" (the player owns that give-up bound). Mapping:
//
//	dir absent / unreadable                       → 404 no_out_dir (terminal token)
//	dir present, triple all present & non-empty   → 200 {"status":"rendered"}
//	dir present, ≥1 leaf missing or empty         → 200 {"status":"rendering"}
//
// It takes the SAME per-dir mutex /escalate writers + /artifact readers hold, so
// a poll issued mid-patch observes the fully-old or fully-new triple, never a
// torn cross-file state (R3). rendered-with-refusal is still "rendered": refusal
// is data, plan.json renders it; the readiness check is purely file-existence.
func readinessHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, reasonMethodNotAllowed, "GET only")
		return
	}

	dir := r.URL.Query().Get("dir")
	if dir == "" {
		writeError(w, http.StatusBadRequest, reasonMissingField, "dir is required")
		return
	}

	absDir, err := filepath.Abs(dir)
	if err != nil {
		writeError(w, http.StatusBadRequest, reasonMissingField, "resolve dir: "+err.Error())
		return
	}

	// Exclusive per-dir lock (shared with the /escalate writers + /artifact
	// readers — sync.Mutex has no shared read mode, so this is a full Lock, not a
	// read lock) so a poll that races a mid-patch escalate observes a consistent
	// triple, never a torn one. The read path here is short, so serializing polls
	// against each other costs nothing in practice.
	muIface, _ := dirLocks.LoadOrStore(absDir, &sync.Mutex{})
	mu := muIface.(*sync.Mutex)
	mu.Lock()
	defer mu.Unlock()

	// The dir itself must exist and be a directory. Absence (typo / wrong path /
	// render never started) — or any stat fault that means we cannot see a valid
	// outDir — is the no_out_dir terminal token, NOT a readiness state and NOT a
	// 500. The player converts it to render_failed immediately.
	info, err := os.Stat(absDir)
	if err != nil || !info.IsDir() {
		writeError(w, http.StatusNotFound, reasonNoOutDir,
			fmt.Sprintf("output dir does not exist: %q", dir))
		return
	}

	// All three leaves present AND non-empty → rendered; else still rendering.
	//
	// COUPLING (do NOT weaken without revisiting the sink): the size>0 test is a
	// sufficient "this leaf is complete" signal ONLY because the persistent sink
	// publishes each leaf atomically — it streams to a tmp file then os.Rename()s
	// it into place (sink/persistent atomicWriteFile), so a non-zero file at the
	// final path is never a partial write. If a leaf were ever streamed in place
	// (written directly to its final path), this size>0 check could read a
	// truncated file as "rendered" and the player would fetch a partial audio.wav.
	// The atomic-publish guarantee is what makes file-existence a safe readiness
	// proxy across the SEPARATE render process. (Doc only — never edit the sink here.)
	for _, leaf := range readinessLeaves {
		li, lerr := os.Stat(filepath.Join(absDir, leaf))
		if lerr != nil || li.IsDir() || li.Size() == 0 {
			writeJSON(w, http.StatusOK, readinessResponse{Status: statusRendering})
			return
		}
	}
	writeJSON(w, http.StatusOK, readinessResponse{Status: statusRendered})
}

// runEscalate is the handler core (Step 4). Returns either a success response
// (errResp == nil, status 200) or an ErrorResponse + HTTP status. Pulled out of
// the http.HandlerFunc so tests can assert the mapping directly and so the
// per-request-context + per-dir-lock discipline is one readable unit.
func runEscalate(reqCtx context.Context, args serverArgs, req escalateRequest) (escalateResponse, int, *ErrorResponse) {
	// (a) Per-request timeout context FIRST (R2c). Threaded into Narrate +
	// PatchBlock so a hung render is cancellable and cannot hold the per-dir
	// mutex for the server lifetime.
	ctx, cancel := context.WithTimeout(reqCtx, args.RequestTimeout)
	defer cancel()

	// (b) Resolve dir to an absolute, cleaned path — the mutex key (keying
	// hygiene, not a sandbox; local-trust model, R4) — then acquire the per-dir
	// lock with defer Unlock BEFORE any fallible call (R2b: no lock leak on
	// panic/early return; no deadlock for the dir's server lifetime).
	absDir, err := filepath.Abs(req.Dir)
	if err != nil {
		return escalateResponse{}, http.StatusBadRequest,
			&ErrorResponse{Reason: reasonSourceNotFound, Message: "resolve dir: " + err.Error()}
	}
	muIface, _ := dirLocks.LoadOrStore(absDir, &sync.Mutex{})
	mu := muIface.(*sync.Mutex)
	mu.Lock()
	defer mu.Unlock()

	// (c) Read the manifest as the source-of-truth index. Any read error is a
	// caller-correctable source-resolution failure → source_not_found, with the
	// R7 deterministic status split inlined: 404 iff the file is cleanly absent
	// (fs.ErrNotExist), else 400 (present-but-unparseable / permission-denied).
	// A corrupt manifest must NOT silently fall through with a zero manifest, so
	// there is deliberately no "unrecognized error" pass-through here.
	manifest, err := readManifest(absDir)
	if err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, fs.ErrNotExist) {
			status = http.StatusNotFound
		}
		return escalateResponse{}, status, classifySourceErr(err)
	}

	ref := plan.SourceRef{
		Kind:        manifest.SourceKind,
		URI:         manifest.SourceURI,
		ContentHash: manifest.ContentHash,
	}

	// (d) Re-render the one block via the capturing sink (no write), then commit
	// via patchBlock — exactly cmd/narrate's capturingSink path.
	capturer := &capturingSink{}
	pl := newPipeline(absDir, args, capturer)
	_, err = pl.Narrate(ctx, ref, pipeline.NarrateRequest{
		Voice:               genderToVoice[args.Gender],
		BlockID:             req.BlockID,
		LevelOverrides:      map[string]plan.Level{req.BlockID: plan.Level(req.Level)},
		ExpectedContentHash: manifest.ContentHash,
	})
	if err != nil {
		status, errResp := statusForErr(err)
		return escalateResponse{}, status, errResp
	}
	if !capturer.captured {
		return escalateResponse{}, http.StatusInternalServerError,
			&ErrorResponse{Reason: reasonInternal, Message: "render produced no captured result for block " + req.BlockID}
	}

	_, perr := patchBlock(ctx, absDir, capturer.plan, capturer.result, req.BlockID,
		persistent.WithVoice(genderToVoice[args.Gender]))
	// (e) ErrNothingToPatch → 4xx source_not_found, NOT a 200 no-op.
	//
	// CORRECTION (build-review B-1): the real persistent.PatchBlock returns
	// ErrNothingToPatch ONLY when the outDir is an INCOMPLETE persistent-sink
	// directory (manifest.json / audio.wav / plan.json missing) — its Step 1
	// pre-flight. It has NO "block already at the requested level" detection, so
	// ErrNothingToPatch is never the "already-at-level repeat" signal the earlier
	// build assumed. An incomplete dir is a caller-correctable source-resolution
	// failure (the dir is not a complete persistent-sink output), so it maps to
	// the source_not_found class: 404 iff the missing file surfaces as
	// fs.ErrNotExist, else 400. Mapping it to a 200 here was the confusing trap —
	// it then ran readBack against the missing plan.json and surfaced 500
	// readback_failed. (B3/B5 convergence is documented at the head of this file:
	// it happens by content-identical RE-RENDER through the happy path below, not
	// via this branch.) Any other patch error maps via statusForErr.
	if perr != nil {
		if errors.Is(perr, persistent.ErrNothingToPatch) {
			status := http.StatusBadRequest
			if errors.Is(perr, fs.ErrNotExist) {
				status = http.StatusNotFound
			}
			return escalateResponse{}, status,
				&ErrorResponse{Reason: reasonSourceNotFound, Message: "incomplete persistent-sink dir (missing manifest.json, audio.wav, or plan.json): " + perr.Error()}
		}
		status, errResp := statusForErr(perr)
		return escalateResponse{}, status, errResp
	}

	// (f) Post-patch read-back from on-disk truth (R1). NarrateResult does not
	// expose the updated Block / BlockTiming / audio_ref, so we re-read
	// plan.json + manifest.json. Read-back failure → 500 readback_failed; the
	// patch already committed (all three files exist), so the caller's recovery
	// is to re-issue the IDENTICAL POST, which re-renders the same block to
	// content-identical bytes and re-patches, then reads back → 200 (B5
	// convergence by re-render, NOT by an ErrNothingToPatch short-circuit).
	resp, rerr := readBack(absDir, req.BlockID)
	if rerr != nil {
		return escalateResponse{}, http.StatusInternalServerError,
			&ErrorResponse{Reason: reasonReadbackFailed, Message: "post-patch read-back failed: " + rerr.Error()}
	}
	return resp, http.StatusOK, nil
}

// readBack re-reads plan.json + manifest.json and shapes the 200 response for
// blockID. On a refused block, Timing + AudioRef are left nil/empty (omitted
// from the body); Refusal is populated. On voiced/degraded, Timing + AudioRef
// are populated and Refusal stays nil (Step 5 nullability).
func readBack(dir, blockID string) (escalateResponse, error) {
	p, err := readPlanFile(dir)
	if err != nil {
		return escalateResponse{}, err
	}
	var blk *plan.Block
	for i := range p.Blocks {
		if p.Blocks[i].ID == blockID {
			blk = &p.Blocks[i]
			break
		}
	}
	if blk == nil {
		return escalateResponse{}, fmt.Errorf("block %s not found in plan.json after patch", blockID)
	}

	resp := escalateResponse{Block: *blk}

	if blk.Status == plan.StatusRefused {
		// Refusal-is-data: surface the refusal payload; no audio was produced,
		// so timing + audio_ref are omitted (absent, not null).
		resp.Refusal = blk.Refusal
		return resp, nil
	}

	// Voiced / degraded: pull timing + audio_ref from the manifest index.
	manifest, err := readManifest(dir)
	if err != nil {
		return escalateResponse{}, err
	}
	for i := range manifest.Blocks {
		if manifest.Blocks[i].ID == blockID {
			mb := manifest.Blocks[i]
			resp.Timing = &plan.BlockTiming{
				BlockID:  mb.ID,
				StartMs:  mb.StartMs,
				EndMs:    mb.EndMs,
				AudioRef: mb.AudioRef,
			}
			resp.AudioRef = mb.AudioRef
			return resp, nil
		}
	}
	return escalateResponse{}, fmt.Errorf("block %s not found in manifest.json after patch", blockID)
}

// classifySourceErr maps a manifest-read error to its source_not_found
// ErrorResponse. It ALWAYS returns a non-nil response — every manifest-read
// failure is a caller-correctable source-resolution failure (absent, permission,
// or present-but-unparseable), never a silent fall-through. The HTTP status
// (404 vs 400) is decided by the caller per R7 (404 iff fs.ErrNotExist).
func classifySourceErr(err error) *ErrorResponse {
	if errors.Is(err, fs.ErrNotExist) {
		return &ErrorResponse{Reason: reasonSourceNotFound, Message: "manifest.json not found in dir"}
	}
	if errors.Is(err, fs.ErrPermission) {
		return &ErrorResponse{Reason: reasonSourceNotFound, Message: "manifest.json permission denied"}
	}
	// Present-but-unparseable (corrupt JSON) is also a caller-correctable
	// source resolution failure → 400 source_not_found.
	return &ErrorResponse{Reason: reasonSourceNotFound, Message: "manifest.json unreadable: " + err.Error()}
}

// statusForErr classifies a pipeline/patch error into (HTTP status,
// ErrorResponse). The caller/internal/cancel CATEGORY decision is delegated to
// the shared errclass.Classify (#51); this root owns the HTTP enum mapping
// (status + reason token) and flattens the error via err.Error() into
// ErrorResponse.Message (NO %w wrap — the server never propagated a wrapped
// error and must not start, or ErrorResponse.Message would change).
//
// Branch order is load-bearing and preserved verbatim: the cancel check stays
// FIRST, then the five persistent.Err* -> 409 branches, then the default 500.
// Only the cancel category is delegated to errclass; the persistent ladder is
// NOT handed to errclass (it is server-specific patch-state, not pipeline
// classification).
//
// Note (#51 [Domain Logic]): errclass also distinguishes ClassCaller (fs
// not-found / permission), but on the server PATCH path a caller-class error
// maps to 500 reasonInternal today and MUST continue to — so this function adds
// NO caller case; ClassCaller falls through to the default 500 alongside
// ClassInternal. (Source-resolution 400/404 on the READ path is owned by
// classifySourceErr, untouched.)
func statusForErr(err error) (int, *ErrorResponse) {
	switch {
	case errclass.Classify(err) == errclass.ClassCancelled:
		return http.StatusRequestTimeout, &ErrorResponse{Reason: reasonCancelled, Message: "request cancelled or timed out: " + err.Error()}
	case errors.Is(err, persistent.ErrStalePatch):
		return http.StatusConflict, &ErrorResponse{Reason: reasonStalePatch, Message: err.Error()}
	case errors.Is(err, persistent.ErrContentHashMismatch):
		return http.StatusConflict, &ErrorResponse{Reason: reasonContentHashMismatch, Message: err.Error()}
	case errors.Is(err, persistent.ErrUnknownBlock):
		return http.StatusConflict, &ErrorResponse{Reason: reasonUnknownBlock, Message: err.Error()}
	case errors.Is(err, persistent.ErrContainerMismatch):
		return http.StatusConflict, &ErrorResponse{Reason: reasonContainerMismatch, Message: err.Error()}
	case errors.Is(err, persistent.ErrFormatMismatch):
		return http.StatusConflict, &ErrorResponse{Reason: reasonFormatMismatch, Message: err.Error()}
	default:
		// Render/sink hard fault, corrupt manifest mid-patch, unreadable WAV,
		// ErrNoOutDir, or any other internal fault → 500 internal.
		return http.StatusInternalServerError, &ErrorResponse{Reason: reasonInternal, Message: err.Error()}
	}
}

// writeJSON encodes v as JSON with the given status. Pulled out so success and
// error bodies share one encoder.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// writeError emits the single ErrorResponse envelope.
func writeError(w http.ResponseWriter, status int, reason, message string) {
	writeJSON(w, status, ErrorResponse{Reason: reason, Message: message})
}
