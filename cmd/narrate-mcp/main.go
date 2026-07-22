// Command narrate-mcp — MCP server entry point. Exposes a single `speak`
// tool over stdio.
//
// Wires the four edges (adapter/file + planner with nil intelligence +
// render/sherpa + sink/ephemeral) into pipeline.Pipeline and runs one
// narration per tool call. Sibling composition root to cmd/narrate: same
// pipeline, different surface.
//
// Tool family: narrate.* — currently `speak` is the only registered tool.
// Future tools (e.g. narrate.escalate) belong under the same server.
//
// Exit codes:
//
//	0 — clean shutdown (stdin EOF received, or SIGINT delivered cleanly).
//	1 — transport / server error during Serve.
//
// Decisions baked in (harvested from the build-session plan + review):
//
//	Decision (v1) — convention: accepted, extended by issue #78. The tool
//	  response envelope began receipt-only: {"receipt": {blocks_played,
//	  total_duration_ms, out_dir}}. Issue #78 (ADR #77 Channel 1) adds an
//	  additive `transcript` array sibling to `receipt`: the after-the-fact,
//	  block-granularity spoken transcript (verbatim text + level + status +
//	  per-block duration + refusal reason for refused blocks). The transcript
//	  reaches the client through two channels (ADR #77 D3): structuredContent
//	  (auto-populated from the typed return) and a duplicate serialized-JSON
//	  TextContent block (survives the CLI's one-line collapse). `receipt`
//	  fields are unchanged; no plan.json schema field is added (the transcript
//	  is a receipt-side projection of already-computed plan data).
//	Decision (v2) — convention: accepted. Pipeline errors are classified
//	  caller-error (fs.ErrNotExist, fs.ErrPermission, validation, text-arg,
//	  sink=persistent) vs internal-error (renderer/sink failure). The
//	  classification is encoded as a prefix in the error message returned
//	  inside CallToolResult; the MCP protocol returns the tool error via
//	  IsError=true content, so callers self-correct on the text. The
//	  classifier is testable independently.
//	Decision (v3) — convention: accepted. Tool family is `narrate.*`; the
//	  README config snippet targets Claude Desktop's
//	  claude_desktop_config.json as canonical, with the `mcp` CLI as a
//	  secondary smoke path.
//	Decision (v4) — convention: superseded by Decision (v6) below.
//	  Originally: `errTextNotImplemented` was a transient sentinel so the
//	  `text` arg stayed in the schema for forward-compat while the handler
//	  fast-errored. Issue #17 lands the mcptext adapter and removes the
//	  sentinel; the `text` arg path now resolves end-to-end.
//	Decision (v5) — convention: accepted. Composition seam — `newPipeline`
//	  is a package-level factory hook (var) so tests can substitute a
//	  narrator stub and verify that level/voice/locale wiring threads
//	  through runSpeak without spawning Kokoro. The seam is a deliberate
//	  testability concession; the production var still builds the real
//	  pipeline.Pipeline. Resolves build-review B2. Issue #17 widens the
//	  hook to also take the chosen adapter.InputAdapter so the text-arg
//	  path can substitute a mcptext.Adapter without forking the seam.
//	Decision (v6) — convention: accepted. The `text` arg is implemented
//	  via adapter/mcptext (ticket #17). The composition root constructs
//	  the URI as `mcp://inline/<sha256-hex-of-text>`; the adapter
//	  cross-checks the URI hash against sha256(text) on Read (Decision v3
//	  of the #17 plan). Per Decision v5 of the #17 plan the offset-map
//	  line-walking logic is duplicated between adapter/file and
//	  adapter/mcptext rather than lifted — a shared adapterutil package
//	  is deferred until a third adapter arrives. Supersedes Decision v4
//	  above.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/signal"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/vd09-projects/intelligent-tts-narration-library/adapter"
	"github.com/vd09-projects/intelligent-tts-narration-library/adapter/file"
	"github.com/vd09-projects/intelligent-tts-narration-library/adapter/mcptext"
	"github.com/vd09-projects/intelligent-tts-narration-library/intelligence"
	"github.com/vd09-projects/intelligent-tts-narration-library/intelligence/mcpsampling"
	"github.com/vd09-projects/intelligent-tts-narration-library/internal/errclass"
	"github.com/vd09-projects/intelligent-tts-narration-library/internal/transcript"
	"github.com/vd09-projects/intelligent-tts-narration-library/pipeline"
	"github.com/vd09-projects/intelligent-tts-narration-library/plan"
	"github.com/vd09-projects/intelligent-tts-narration-library/render/rvc"
	"github.com/vd09-projects/intelligent-tts-narration-library/sink"
	"github.com/vd09-projects/intelligent-tts-narration-library/sink/ephemeral"
	"github.com/vd09-projects/intelligent-tts-narration-library/sink/persistent"
)

// genderToVoice — phase-one mapping per CLAUDE.md domain rules + sindri
// patterns A15 amendment. Female default per problem statement. Re-stated
// here rather than imported from cmd/narrate so the two cmd packages stay
// independent — matches the project's existing pattern.
var genderToVoice = map[string]string{
	"female": "af_bella",
	"male":   "am_michael",
}

// Local sentinels. Caller-error sentinels carry the wire-prefix in their
// message so the classifier path and direct-validation path produce
// identical text; tests assert with errors.Is.
//
// Note: errTextNotImplemented was removed by ticket #17 (Decision v6) —
// the `text` arg now resolves end-to-end via adapter/mcptext.
var (
	errMissingSource            = errors.New("caller-error: invalid_argument: must supply either source or text")
	errBothSourceAndText        = errors.New("caller-error: invalid_argument: cannot supply both source and text")
	errPersistentNotImplemented = errors.New("caller-error: invalid_argument: persistent sink not implemented in phase one; use ephemeral")
	errUnknownIntelligence      = errors.New("caller-error: invalid_argument: intelligence must be none or mcpsampling")
)

// speakArgs — JSON-tagged tool arguments. Field defaults are not applied
// by the SDK's schema generator; the handler fills zero values to the
// documented defaults before validating.
//
// jsonschema struct tags drive the SDK's auto-generated input schema.
type speakArgs struct {
	Source       string `json:"source,omitempty"       jsonschema:"file path to a markdown document to narrate (exactly one of source or text)"`
	Text         string `json:"text,omitempty"         jsonschema:"inline markdown text to narrate (exactly one of source or text). Routed through the in-memory mcptext adapter; URI is mcp://inline/<sha256-hex>."`
	Level        int    `json:"level,omitempty"        jsonschema:"leveling depth: 1 (gist) | 2 (summary) | 3 (detail). default 1"`
	Sink         string `json:"sink,omitempty"         jsonschema:"output sink: ephemeral | persistent (persistent not implemented). default ephemeral"`
	Gender       string `json:"gender,omitempty"       jsonschema:"voice gender: female | male. default female"`
	Voice        string `json:"voice,omitempty"        jsonschema:"RVC character voice: cool-jahns | confident-neal (40 kHz; requires the RVC worker). Empty (default) = plain Kokoro. When set it overrides gender (RVC selects the Kokoro source). Additive-compat field — schema_version unchanged; older clients that omit it are unaffected."`
	Intelligence string `json:"intelligence,omitempty" jsonschema:"intelligence backend: none | mcpsampling. default none. Additive-compat field — schema_version unchanged per CLAUDE.md."`
}

// applyDefaults — the SDK does not fill JSON-schema defaults for us.
// Zero-valued fields are normalized here so validate() and runSpeak see
// the documented defaults.
func (a *speakArgs) applyDefaults() {
	if a.Level == 0 {
		a.Level = 1
	}
	if a.Sink == "" {
		a.Sink = "ephemeral"
	}
	if a.Gender == "" {
		a.Gender = "female"
	}
	if a.Intelligence == "" {
		a.Intelligence = "none"
	}
}

// validate enforces enum + range checks the SDK's schema validator does
// not (defaults applied first by applyDefaults). The XOR / persistent-sink
// checks return sentinel errors so the classifier and tests can match
// them with errors.Is.
//
// Per Decision v6 (#17): the text-arg fast-error is gone — text and
// source both pass validation; runSpeak routes them to different
// adapters.
func (a speakArgs) validate() error {
	if a.Source == "" && a.Text == "" {
		return errMissingSource
	}
	if a.Source != "" && a.Text != "" {
		return errBothSourceAndText
	}
	if a.Level < 1 || a.Level > 3 {
		return fmt.Errorf("caller-error: invalid_argument: level must be 1, 2, or 3 (got %d)", a.Level)
	}
	switch a.Sink {
	case "ephemeral":
		// ok
	case "persistent":
		return errPersistentNotImplemented
	default:
		return fmt.Errorf("caller-error: invalid_argument: sink must be ephemeral or persistent (got %q)", a.Sink)
	}
	if _, ok := genderToVoice[a.Gender]; !ok {
		return fmt.Errorf("caller-error: invalid_argument: gender must be female or male (got %q)", a.Gender)
	}
	// Issue #146 — voice enum. Empty preserves plain Kokoro; a non-empty value
	// must be a known RVC target slug. An unknown voice is an MCP caller-error
	// (classified by the "caller-error: invalid_argument:" prefix), never a
	// silent fallback to Kokoro (honesty rule).
	if a.Voice != "" && !rvc.IsSupportedVoice(a.Voice) {
		return fmt.Errorf("caller-error: invalid_argument: voice must be one of %s (got %q)",
			strings.Join(rvc.SupportedVoices(), ", "), a.Voice)
	}
	switch a.Intelligence {
	case "none", "mcpsampling":
		// ok
	default:
		return fmt.Errorf("%w (got %q)", errUnknownIntelligence, a.Intelligence)
	}
	return nil
}

// speakReceipt — the v1 response envelope (receipt-only, per Decision v1).
//
// out_dir is the renderer's per-call temp directory and is deleted by
// runSpeak's deferred cleanup after the response returns. It is included
// in the response for debugging-window inspection only — clients must not
// rely on the directory existing after the call.
type speakReceipt struct {
	BlocksPlayed    int    `json:"blocks_played"      jsonschema:"number of blocks the sink played"`
	TotalDurationMs int64  `json:"total_duration_ms"  jsonschema:"total planned narration duration in milliseconds"`
	OutDir          string `json:"out_dir"            jsonschema:"renderer's per-call temp dir (deleted on return)"`
}

// transcriptEntry — one block's row in the after-the-fact spoken transcript
// (issue #78, ADR #77 Channel 1). Projects already-computed plan data: the
// verbatim spoken words, per-block level + status, planned per-block duration,
// and — for refused/degraded blocks — the honesty-rule refusal reason+message.
//
// Block-granularity only (CLAUDE.md invariant; ADR #77 D5): no per-word, per-
// segment, or per-char offsets. RefusalReason / RefusalMessage are omitempty —
// absent on voiced/degraded-without-refusal blocks. DurationMs is the planned
// duration (EndMs - StartMs) from the block's timeline row, 0 when the block
// had no row (e.g. refused / not rendered).
type transcriptEntry struct {
	BlockID        string `json:"block_id"                 jsonschema:"stable block id from the narration plan"`
	Order          int    `json:"order"                    jsonschema:"block position in the document (plan order)"`
	Level          int    `json:"level"                    jsonschema:"leveling depth the block was voiced at: 1 (gist) | 2 (summary) | 3 (detail)"`
	Status         string `json:"status"                   jsonschema:"block outcome: voiced | degraded | refused"`
	SpokenText     string `json:"spoken_text"              jsonschema:"verbatim words spoken for this block (single-space join of segment text); empty for a refused block that only spoke a notice"`
	DurationMs     int    `json:"duration_ms"              jsonschema:"planned per-block duration in milliseconds (block-granularity only); 0 when the block had no timeline row"`
	RefusalReason  string `json:"refusal_reason,omitempty"  jsonschema:"why the block was refused (honesty rule); present only on refused blocks"`
	RefusalMessage string `json:"refusal_message,omitempty" jsonschema:"the short spoken refusal notice; present only on refused blocks"`
}

// speakResponse — wraps speakReceipt under the `receipt` key and the per-block
// spoken transcript under `transcript` (sibling, additive — issue #78). The
// `receipt` shape is untouched (counter-metric). This is the MCP result
// envelope, not plan.json — it has no schema_version field (that governs the
// narration plan). The additive / ignore-unknown discipline (CLAUDE.md) applies
// to this envelope by analogy: new fields are omitempty and absent unless they
// carry signal, so consumers that ignore them see byte-identical output.
type speakResponse struct {
	Receipt speakReceipt `json:"receipt"`
	// Transcript is the after-the-fact, block-granularity spoken transcript,
	// in plan order. Present on the success path only (built after Narrate
	// returns a nil error); an error return carries no transcript. Bounded to
	// transcriptMaxEntries entries (issue #86) — see capTranscript.
	Transcript []transcriptEntry `json:"transcript" jsonschema:"after-the-fact per-block spoken transcript in plan order: verbatim text, level, status, duration, and refusal reason for refused blocks; capped at 200 entries on very large documents"`
	// TranscriptTruncated signals that the transcript was capped (issue #86):
	// the document had more blocks than transcriptMaxEntries and the tail was
	// dropped. omitempty → absent (byte-identical) on the common under-cap case.
	TranscriptTruncated bool `json:"transcript_truncated,omitempty" jsonschema:"true when the transcript was capped because the document exceeded the per-response entry limit; absent otherwise"`
	// TranscriptOmittedCount is how many trailing entries were dropped by the
	// cap (issue #86). omitempty → absent (and byte-identical) when nothing was
	// dropped. The dropped blocks are still counted in receipt.blocks_played.
	TranscriptOmittedCount int `json:"transcript_omitted_count,omitempty" jsonschema:"number of trailing transcript entries dropped by the cap; absent when none were dropped"`
	// TranscriptOmittedRefusedCount is how many of the dropped trailing entries
	// (issue #98) were refusals — i.e. entries in the dropped tail whose status
	// equals string(plan.StatusRefused). A truncated transcript is not an
	// exhaustive refusal ledger; this count lets a consumer reading the response
	// alone tell that refused blocks fell off the tail. The dropped refused
	// blocks were still spoken and counted in receipt.blocks_played — only their
	// transcript rows were dropped, no text is surfaced (count only). omitempty →
	// absent (byte-identical) when none were dropped or no refusals fell in the
	// dropped tail. Wording is pinned to the speak tool Description sentence to
	// prevent drift.
	TranscriptOmittedRefusedCount int `json:"transcript_omitted_refused_count,omitempty" jsonschema:"number of refused blocks dropped from the trailing transcript tail by the cap; absent when none were dropped or no refusals fell in the dropped tail. The dropped refused blocks were still spoken and counted in receipt.blocks_played."`
}

// runDeps — IO + behavior seams injected by tests. Production wires
// runSpeak (the real composition root) and stderr to os.Stderr. The run
// seam lets unit tests exercise the handler wiring without spawning
// Kokoro; the manual smoke test wires the real runSpeak.
//
// cache is the single server-lifetime mcpsampling cache handle (issue
// #25). It is allocated ONCE in newServer and threaded into every tool
// call's buildIntelligence so the bounded LRU and the shared last-known
// map outlive a single call — that lifetime is what makes cross-call
// escalation stop re-billing. A nil cache (e.g. tests wiring runSpeak
// directly) means mcpsampling caching is off for that call.
type runDeps struct {
	stderr io.Writer
	run    func(ctx context.Context, args speakArgs) (speakResponse, error)
	cache  *mcpsampling.ServerCache
}

// The minimal surface runSpeak needs from a wired pipeline is
// pipeline.Narrator (issue #27 — formerly a private narrator interface
// duplicated here and in cmd/narrate, per Decision v5). The newPipeline seam
// returns it so tests can substitute the pipeline without spawning Kokoro;
// production wires *pipeline.Pipeline.

// newPipeline — package-level factory hook. Production builds the real
// pipeline.Pipeline; tests swap this var to inject a stub narrator.
// Per Decision v5 (build-review B2 fix): the seam lets unit tests verify
// the level/voice/locale wiring without spawning Kokoro.
//
// Phase 5 of #13 added the intel arg: the intelligence adapter the caller
// selected via args.Intelligence. nil = current deterministic+degraded
// behavior. Constructed by runSpeak after applyDefaults+validate so the
// factory stays a pure composition step.
//
// Issue #17 (Decision v6) adds the input arg: the concrete input adapter
// runSpeak chose for this call (file.New() for source path, mcptext.New
// for text path). Threading it through the seam lets unit tests assert
// which adapter was wired, and keeps the production composition single-
// sourced inside this hook.
// listenCodeMinLevel — the code-class level FLOOR every speak call requests
// (issue #73, listen-mode). A Claude Code user listening to a large
// assistant response wants code voiced as a structural gist (L2), not the
// bare L1 line count. Set as a floor on PipelineDefaults: code lifts to L2,
// but an explicit L3 listen request survives (max(L3, L2) == L3); prose and
// other classes are untouched. This is composition-root configuration of a
// planner-read declarative field — NOT a new speakArgs field. applyDefaults
// / validate keep their document-wide Level semantics unchanged.
const listenCodeMinLevel = plan.L2

var newPipeline = func(outDir string, args speakArgs, input adapter.InputAdapter, intel intelligence.IntelligenceAdapter, observer ephemeral.BlockObserver) pipeline.Narrator {
	renderer, _, err := pipeline.BuildRenderer(args.Voice)
	if err != nil {
		// validate() pins args.Voice to a known slug (or empty) before this seam
		// runs, so BuildRenderer's only reachable error — ErrUnsupportedVoice —
		// cannot fire here. Any error is a composition-root bug; panic loudly
		// rather than silently degrade to Kokoro. The ephemeral sink plays via
		// afplay, which handles a 40 kHz WAV natively — no WithExpectedFormat.
		panic(fmt.Sprintf("buildRenderer failed after validation: %v", err))
	}
	return pipeline.New(
		input,
		intel,
		renderer,
		ephemeral.New(ephemeral.WithBlockObserver(observer)),
		pipeline.PipelineDefaults{
			Level:        plan.Level(args.Level),
			OutDir:       outDir,
			Locale:       "en",
			CodeMinLevel: listenCodeMinLevel,
		},
	)
}

// runSpeak is the production wiring — composition root for one tool call.
// Same shape as cmd/narrate's runNarrate; the differences are arg parsing,
// the XOR source/text validation, and that we return the receipt rather
// than printing to stdout.
//
// Phase 5 of #13 wires the optional intelligence adapter. When
// args.Intelligence == "none" (the default), nil flows into newPipeline
// and the planner takes its current deterministic+degraded path. When
// "mcpsampling", a per-call mcpsampling.Adapter is constructed with a
// per-call in-memory cache. The speak handler threads the live
// *mcp.ServerSession into ctx via mcpsampling.WithSamplingClient — see
// speakHandler — so the adapter can call CreateMessage without crossing
// the pipeline.New layer boundary (plan Decision v3).
func runSpeak(ctx context.Context, args speakArgs) (speakResponse, error) {
	// runSpeak with no server cache: mcpsampling caching is off for this
	// call. The production path goes through runSpeakWithCache (wired in
	// newServer) so it threads the single server-lifetime cache handle.
	return runSpeakWithCache(ctx, args, nil)
}

// runSpeakWithCache is the real composition body. cache is the single
// server-lifetime mcpsampling.ServerCache allocated in newServer, or nil
// to disable mcpsampling caching for this call. Hoisting this ONE
// allocation out of the per-call path (it used to be a fresh per-call
// cache constructed here, in buildIntelligence) is the cross-call fix:
// the bounded LRU and the shared last-known map now outlive a single
// tool call (issue #25). The per-call temp-dir + ephemeral-sink
// lifecycle below is unchanged.
func runSpeakWithCache(ctx context.Context, args speakArgs, cache *mcpsampling.ServerCache) (speakResponse, error) {
	args.applyDefaults()
	if err := args.validate(); err != nil {
		return speakResponse{}, err
	}

	input, ref, err := inputAdapterAndRef(args)
	if err != nil {
		return speakResponse{}, err
	}

	// Per-call temp dir. Ephemeral sink plays then we wipe; afplay
	// completes before sink.Consume returns, so the defer is safe.
	outDir, err := os.MkdirTemp("", "narrate-mcp-")
	if err != nil {
		return speakResponse{}, fmt.Errorf("internal_error: create out dir: %w", err)
	}
	defer func() {
		_ = os.RemoveAll(outDir)
	}()

	// Channel-2 live observer (issue #81). Off unless opted in via env; a nil
	// observer leaves the sink path byte-identical. The scratch file is NOT
	// under outDir — it is ephemeral but user-facing, reaped by the OS, never
	// teed into a durable sink. STDERR (not the speak response) is the only
	// place a scratch failure ever surfaces, so the response stays decoupled.
	observer, closeObserve := newBlockObserver(os.Stderr)
	defer closeObserve()

	intel := buildIntelligence(args, cache)
	pl := newPipeline(outDir, args, input, intel, observer)

	receipt, err := pl.Narrate(ctx, ref, pipeline.NarrateRequest{
		Voice: genderToVoice[args.Gender],
	})
	if err != nil {
		// Error path: propagate unchanged, attach NO transcript. The
		// transcript is success-path-only (issue #78) — it is built solely
		// from a plan that rendered end-to-end. Errors stop the pipeline;
		// refusals are data and arrive on the success path below.
		return speakResponse{}, classifyPipelineErr(err)
	}

	// Success path only: project the rendered plan's already-computed roster
	// into the after-the-fact transcript. Refusals are data here, not errors.
	// Then bound the roster (issue #86): a very large document ships its
	// transcript twice (structuredContent + the duplicate serialized TextContent,
	// ADR #77 D3), so an unbounded array doubles a huge payload. capTranscript
	// keeps the head and drops the tail. NOTE: the dropped tail may include
	// refused entries — those blocks are still counted in receipt.blocks_played,
	// so a truncated transcript is NOT an exhaustive refusal ledger.
	transcript, truncated, omitted, omittedRefused := capTranscript(transcriptFromResult(receipt), transcriptMaxEntries)
	return speakResponse{
		Receipt:                       receiptFromSink(receipt.SinkReceipt, outDir),
		Transcript:                    transcript,
		TranscriptTruncated:           truncated,
		TranscriptOmittedCount:        omitted,
		TranscriptOmittedRefusedCount: omittedRefused,
	}, nil
}

// inputAdapterAndRef picks the InputAdapter + SourceRef for one call.
//
// Per Decision v6 (#17): args.Text != "" routes through adapter/mcptext
// with the canonical URI "mcp://inline/<sha256-hex-of-text>"; the adapter
// cross-checks the URI hash against sha256(args.Text) on Read (Decision
// v3 of the #17 plan). args.Source != "" keeps the existing adapter/file
// path with an absolute path URI. The XOR + missing checks happened in
// validate(), so exactly one branch fires here; the trailing error is
// defensive — the validate contract should make it unreachable.
func inputAdapterAndRef(args speakArgs) (adapter.InputAdapter, plan.SourceRef, error) {
	switch {
	case args.Text != "":
		return mcptext.New(args.Text), plan.SourceRef{
			Kind: plan.SourceKindMCPText,
			URI:  mcptext.URIFor(args.Text),
		}, nil
	case args.Source != "":
		absPath, err := filepath.Abs(args.Source)
		if err != nil {
			return nil, plan.SourceRef{}, fmt.Errorf("caller-error: invalid_argument: resolve source: %w", err)
		}
		return file.New(), plan.SourceRef{
			Kind: plan.SourceKindFile,
			URI:  absPath,
		}, nil
	default:
		// validate() already rejected this. Defensive — if validate
		// changes, surface a clear error rather than a nil panic.
		return nil, plan.SourceRef{}, errMissingSource
	}
}

// buildIntelligence selects an IntelligenceAdapter per args.Intelligence.
// Returns nil for "none" so the planner takes its deterministic+degraded
// path (intelligence == "none" still yields a nil adapter).
//
// For "mcpsampling" it wires the SHARED, server-lifetime ServerCache
// passed in by the caller via WithServerCache. Issue #25: this handle is
// allocated once in newServer and the same instance threads through every
// tool call, so the bounded LRU and the shared last-known map outlive a
// single call and cross-call escalation stops re-billing. A nil cache
// (tests calling runSpeak directly) wires caching off — WithServerCache
// treats nil as a no-op.
func buildIntelligence(args speakArgs, cache *mcpsampling.ServerCache) intelligence.IntelligenceAdapter {
	if args.Intelligence != "mcpsampling" {
		return nil
	}
	return mcpsampling.New(
		mcpsampling.WithClientID("narrate-mcp"),
		mcpsampling.WithServerCache(cache),
	)
}

// receiptFromSink projects the sink-side SinkReceipt into our wire shape.
// Pulled out so the classifier and the response builder are independently
// testable.
func receiptFromSink(r sink.SinkReceipt, outDir string) speakReceipt {
	return speakReceipt{
		BlocksPlayed:    r.BlocksPlayed,
		TotalDurationMs: r.TotalDurationMs,
		OutDir:          outDir,
	}
}

// transcriptFromResult projects the pipeline's per-block roster
// (NarrateResult.BlockSummaries — populated in plan order, with SpokenText /
// RefusalReason / RefusalMessage / DurationMs already filled by the pipeline)
// into the MCP transcript wire shape. Sibling to receiptFromSink and
// independently testable. Refusal fields are carried only when present
// (refusal-is-data); voiced/degraded blocks leave them empty so the omitempty
// JSON tags drop them. Caller invokes this on the success path only.
func transcriptFromResult(r pipeline.NarrateResult) []transcriptEntry {
	if len(r.BlockSummaries) == 0 {
		return nil
	}
	out := make([]transcriptEntry, 0, len(r.BlockSummaries))
	for _, b := range r.BlockSummaries {
		out = append(out, transcriptEntry{
			BlockID:        b.ID,
			Order:          b.Order,
			Level:          int(b.Level),
			Status:         string(b.Status),
			SpokenText:     b.SpokenText,
			DurationMs:     b.DurationMs,
			RefusalReason:  string(b.RefusalReason),
			RefusalMessage: b.RefusalMessage,
		})
	}
	return out
}

// transcriptMaxEntries bounds how many per-block transcript entries one speak
// response carries (issue #86). The transcript ships twice in a single response
// (structuredContent + the duplicate serialized-JSON TextContent, ADR #77 D3),
// so an unbounded array doubles the cost on very large documents. 200 entries is
// a generous bound for a "very large" document while keeping the duplicated
// payload sane; it is a single named const so re-tuning is a one-line change,
// and transcript_truncated / transcript_omitted_count let a consumer detect when
// the bound bites. The cap drops the document tail (head-keep), never narration
// content — the blocks were still voiced and counted in the receipt.
const transcriptMaxEntries = 200

// capTranscript bounds a transcript roster to limit entries, head-keep /
// tail-truncate by entry count. Pure: no I/O, no logging, no global state.
//
// Under the limit (the common case) it returns the input slice UNCHANGED with
// truncated=false, omitted=0, omittedRefused=0 — no copy, no allocation — so the
// small-case response serializes byte-identically (the omitempty signal fields
// stay absent and the slice header is the same backing array). Over the limit it
// returns the first limit entries (plan order preserved), truncated=true, the
// number of dropped trailing entries, and how many of those dropped trailing
// entries were refusals (issue #98).
//
// omittedRefused counts entries in the dropped tail (entries[limit:]) whose
// status equals string(plan.StatusRefused) — enum-sourced, never a bare "refused"
// literal, so the count cannot silently decouple from the plan enum. It is scoped
// to the dropped tail only: refusals that survive in the kept head are not
// counted. One bounded loop over the dropped tail keeps the helper pure/total.
//
// A limit <= 0 is treated as "no cap" (a defensive contract, not a request to
// drop everything): the input is returned unchanged with truncated=false,
// omitted=0, omittedRefused=0. The production caller always passes
// transcriptMaxEntries (200).
func capTranscript(entries []transcriptEntry, limit int) (kept []transcriptEntry, truncated bool, omitted, omittedRefused int) {
	if limit <= 0 || len(entries) <= limit {
		return entries, false, 0, 0
	}
	for _, e := range entries[limit:] {
		if e.Status == string(plan.StatusRefused) {
			omittedRefused++
		}
	}
	return entries[:limit], true, len(entries) - limit, omittedRefused
}

// ----- speak_last: narrate Claude's most recent assistant turn ---------------
//
// Self-contained, on-demand counterpart to `speak`. Where `speak` is handed the
// text to voice, `speak_last` LOCATES it: the server reads Claude Code's own
// session transcript off disk, extracts the most recent completed assistant turn,
// and routes that text through the very same runSpeak composition root (via the
// mcptext adapter). This is the "handle it internally" path — no external Stop
// hook, no shell parse script. The MCP server still cannot SELF-TRIGGER (it acts
// only when Claude calls a tool), so this is on-demand, not automatic.
//
// Reading the transcript file is composition-root glue (cmd/), analogous to
// cmd/narrate reading a --file flag — it does not put I/O in planner/ or plan/.

// speakLastToolName is the registered tool name. Referenced both at registration
// and inside lastAssistantText (to exclude the invoking turn), so the two cannot
// drift apart.
const speakLastToolName = "speak_last"

// speakLastArgs — args for speak_last. Level / Gender / Intelligence mirror
// speak's semantics and defaults (applied downstream by speakArgs.applyDefaults
// once the resolved text is handed to runSpeak). TranscriptPath is an optional
// override: empty means "locate the newest transcript under the projects root".
// It exists mainly for tests and power users; normal callers omit everything.
type speakLastArgs struct {
	Level          int    `json:"level,omitempty"           jsonschema:"leveling depth: 1 (gist) | 2 (summary) | 3 (detail). default 1"`
	Gender         string `json:"gender,omitempty"          jsonschema:"voice gender: female | male. default female"`
	Voice          string `json:"voice,omitempty"           jsonschema:"RVC character voice: cool-jahns | confident-neal (40 kHz; requires the RVC worker). Empty (default) = plain Kokoro; when set it overrides gender. Additive-compat field."`
	Intelligence   string `json:"intelligence,omitempty"    jsonschema:"intelligence backend: none | mcpsampling. default none"`
	TranscriptPath string `json:"transcript_path,omitempty" jsonschema:"optional override: absolute path to a Claude Code transcript .jsonl; default = newest under the projects root"`
}

var (
	errNoTranscript    = errors.New("internal_error: no Claude Code transcript found under the projects root")
	errNoAssistantText = errors.New("caller-error: failed_precondition: no prior assistant message with spoken text found in the transcript")
)

// transcriptProjectsRoot resolves where Claude Code writes per-project session
// transcripts. NARRATE_TRANSCRIPT_ROOT overrides it (tests + unusual installs);
// otherwise ~/.claude/projects.
func transcriptProjectsRoot() string {
	if v := os.Getenv("NARRATE_TRANSCRIPT_ROOT"); v != "" {
		return v
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(".claude", "projects")
	}
	return filepath.Join(home, ".claude", "projects")
}

// newestTranscript returns the most-recently-modified *.jsonl under root/*/ — the
// active session's transcript, regardless of which repo it belongs to. This
// sidesteps reconstructing Claude Code's path-mangled project dir from a cwd (the
// launcher pins cwd, so cwd is unreliable here). Concurrency caveat: with two
// live sessions the newest-modified file wins; pin one with transcript_path.
func newestTranscript(root string) (string, error) {
	matches, err := filepath.Glob(filepath.Join(root, "*", "*.jsonl"))
	if err != nil {
		return "", fmt.Errorf("internal_error: glob transcripts: %w", err)
	}
	var newest string
	var newestMod time.Time
	for _, m := range matches {
		fi, statErr := os.Stat(m)
		if statErr != nil {
			continue
		}
		if newest == "" || fi.ModTime().After(newestMod) {
			newest, newestMod = m, fi.ModTime()
		}
	}
	if newest == "" {
		return "", errNoTranscript
	}
	return newest, nil
}

// lastAssistantText extracts the verbatim text of the most recent COMPLETED
// assistant turn from a Claude Code transcript .jsonl. It returns the last
// non-empty assistant join, EXCLUDING any assistant turn that itself invoked
// speak_last — that is the current turn, so excluding it makes the tool speak
// the PRIOR response rather than its own preamble. Assumes one assistant
// message per turn (the common case); a turn split into a separate text line +
// tool line would still voice the text line.
//
// The file scan, the 16 MiB line buffer, and the tolerance (non-JSON /
// schema-variant / unrecognised-content lines skipped, never errored) now live
// in the shared internal/transcript parser. This function is the speak_last
// POLICY layer over the full ordered message list: the speak_last self-skip is
// applied caller-side via ToolNames so the parser stays tool-agnostic.
func lastAssistantText(path string) (string, error) {
	msgs, err := transcript.ParseMessages(path)
	if err != nil {
		return "", err
	}
	var last string
	for _, m := range msgs {
		if m.Role != "assistant" {
			continue
		}
		// Exclude the invoking speak_last turn (the current one) — drop the whole
		// turn, its text included, exactly as the pre-refactor break-on-speak_last
		// path did.
		if slices.Contains(m.ToolNames, speakLastToolName) {
			continue
		}
		// Skip turns with no spoken text (e.g. tool-only turns). Keep the
		// untrimmed join, matching the historical behaviour.
		if strings.TrimSpace(m.Text) != "" {
			last = m.Text
		}
	}
	return last, nil
}

// runSpeakLast resolves the last assistant turn's text and delegates to the same
// composition root as speak: the text rides the mcptext adapter exactly as an
// inline `text` arg would, so defaults, validation, leveling, refusal, and the
// receipt+transcript envelope are all single-sourced from runSpeakWithCache.
func runSpeakLast(ctx context.Context, args speakLastArgs, cache *mcpsampling.ServerCache) (speakResponse, error) {
	path := args.TranscriptPath
	if path == "" {
		p, err := newestTranscript(transcriptProjectsRoot())
		if err != nil {
			return speakResponse{}, err
		}
		path = p
	}
	text, err := lastAssistantText(path)
	if err != nil {
		return speakResponse{}, err
	}
	if strings.TrimSpace(text) == "" {
		return speakResponse{}, errNoAssistantText
	}
	return runSpeakWithCache(ctx, speakArgs{
		Text:         text,
		Level:        args.Level,
		Gender:       args.Gender,
		Voice:        args.Voice,
		Intelligence: args.Intelligence,
	}, cache)
}

// speakLastHandler bridges the SDK's typed handler signature to runSpeakLast.
// Mirrors speakHandler's dual-channel success result (manual TextContent blob +
// auto-populated StructuredContent) and the mcpsampling session threading, so a
// speak_last response is byte-shaped identically to a speak response. It closes
// over deps.cache directly rather than the speakArgs-typed deps.run seam, since
// speak_last has its own pre-pipeline resolution step; runSpeakLast and its
// helpers stay unit-testable without the seam.
func speakLastHandler(deps runDeps) func(context.Context, *mcp.CallToolRequest, speakLastArgs) (*mcp.CallToolResult, speakResponse, error) {
	return func(ctx context.Context, req *mcp.CallToolRequest, args speakLastArgs) (*mcp.CallToolResult, speakResponse, error) {
		if args.Intelligence == "mcpsampling" && req != nil && req.Session != nil {
			ctx = mcpsampling.WithSamplingClient(ctx, req.Session)
		}
		resp, err := runSpeakLast(ctx, args, deps.cache)
		if err != nil {
			return nil, speakResponse{}, err
		}
		blob, mErr := json.Marshal(resp)
		if mErr != nil {
			return nil, speakResponse{}, fmt.Errorf("internal_error: marshal speak response: %w", mErr)
		}
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: string(blob)}},
		}, resp, nil
	}
}

// ----- speak_to_file: render to a single .wav at a caller-given path ---------
//
// The third tool (issue #105, Earshot use case 3). Renders inline text OR a
// text/.md file into ONE combined .wav written at output_path; with no path it
// falls back to ephemeral play (speak behavior). It is a NEW THIRD pipeline
// path — it wires EITHER its own single-wav sink (persistent.WAVFileSink) OR
// the ephemeral sink, never both in one pipeline, and never tees off the speak
// path. The tool returns ONE uniform response shape (speakToFileResponse) for
// both branches: output_path:"" is the sentinel for "played, not written".

// speakToFileArgs — speak_to_file tool arguments. Declares its own fields
// (rather than embedding speakArgs) so the JSON schema advertises EXACTLY the
// args this tool accepts: text/source XOR, level, gender, intelligence, plus
// the optional OutputPath. The sink is NOT a caller arg here — speak_to_file
// always renders through the ephemeral-style internals and writes its own wav
// via output_path, so exposing a sink would be misleading (and sink:persistent
// would mis-error). toSpeakArgs forces sink=ephemeral for the shared internals.
// An empty/omitted output_path means "play ephemerally, write no file".
type speakToFileArgs struct {
	Source       string `json:"source,omitempty"       jsonschema:"file path to a markdown document to narrate (exactly one of source or text)"`
	Text         string `json:"text,omitempty"         jsonschema:"inline markdown text to narrate (exactly one of source or text). Routed through the in-memory mcptext adapter; URI is mcp://inline/<sha256-hex>."`
	Level        int    `json:"level,omitempty"        jsonschema:"leveling depth: 1 (gist) | 2 (summary) | 3 (detail). default 1"`
	Gender       string `json:"gender,omitempty"       jsonschema:"voice gender: female | male. default female"`
	Voice        string `json:"voice,omitempty"        jsonschema:"RVC character voice: cool-jahns | confident-neal (40 kHz; requires the RVC worker). Empty (default) = plain Kokoro; when set it overrides gender. The written .wav is 40 kHz. Additive-compat field."`
	Intelligence string `json:"intelligence,omitempty" jsonschema:"intelligence backend: none | mcpsampling. default none. Additive-compat field — schema_version unchanged per CLAUDE.md."`
	OutputPath   string `json:"output_path,omitempty"  jsonschema:"destination for the rendered .wav. A file path is written (a missing .wav extension is appended); an existing directory gets a derived filename inside it. Omit/empty to play the audio ephemerally and write no file."`
}

// toSpeakArgs projects speak_to_file's args into the shared speakArgs the
// internals (runSpeakWithCache, inputAdapterAndRef, buildIntelligence) consume.
// Sink is pinned to ephemeral: speak_to_file selects its sink via output_path,
// never via a caller-supplied sink, so there is no persistent path to mis-route.
func (a speakToFileArgs) toSpeakArgs() speakArgs {
	return speakArgs{
		Source:       a.Source,
		Text:         a.Text,
		Level:        a.Level,
		Gender:       a.Gender,
		Voice:        a.Voice,
		Intelligence: a.Intelligence,
		Sink:         "ephemeral",
	}
}

// speakToFileResponse — the UNIFORM response envelope for speak_to_file,
// returned for BOTH the with-path and no-path branches (BLOCKING-1: the tool
// never emits two different structuredContent schemas).
//
//   - OutputPath — the resolved absolute .wav path that was written, or "" when
//     the audio was played ephemerally (no file written).
//   - BlocksWritten / TotalDurationMs — block-level accounting, same meaning as
//     speak's receipt fields (BlocksWritten mirrors SinkReceipt.BlocksPlayed).
//   - Transcript (+ the truncation signal fields) — the same after-the-fact,
//     block-granularity spoken transcript speak returns, capped identically
//     (issue #86 / #98), so honest refusal/degraded blocks are surfaced here too.
type speakToFileResponse struct {
	OutputPath      string            `json:"output_path"       jsonschema:"resolved absolute path of the written .wav, or empty string when the audio was played ephemerally and no file was written"`
	BlocksWritten   int               `json:"blocks_written"    jsonschema:"number of blocks concatenated into the output (mirrors the sink's blocks_played)"`
	TotalDurationMs int64             `json:"total_duration_ms" jsonschema:"total planned narration duration in milliseconds"`
	Transcript      []transcriptEntry `json:"transcript"        jsonschema:"after-the-fact per-block spoken transcript in plan order: verbatim text, level, status, duration, and refusal reason for refused blocks; capped at 200 entries on very large documents"`

	TranscriptTruncated           bool `json:"transcript_truncated,omitempty"             jsonschema:"true when the transcript was capped because the document exceeded the per-response entry limit; absent otherwise"`
	TranscriptOmittedCount        int  `json:"transcript_omitted_count,omitempty"         jsonschema:"number of trailing transcript entries dropped by the cap; absent when none were dropped"`
	TranscriptOmittedRefusedCount int  `json:"transcript_omitted_refused_count,omitempty" jsonschema:"number of refused blocks dropped from the trailing transcript tail by the cap; absent when none were dropped or no refusals fell in the dropped tail. The dropped refused blocks were still spoken and counted in blocks_written."`
}

// newFilePipeline — package-level factory hook for the WITH-PATH branch,
// mirroring newPipeline (the ephemeral seam). Production wires the real
// pipeline with persistent.WAVFileSink as the sink so the combined wav lands at
// wavPath; tests swap this var to inject a stub narrator (or a real sink behind
// a stub renderer) without spawning Kokoro. newPipeline is left UNTOUCHED — the
// two paths share no sink (decoupling-regression contract).
//
// renderDir is the per-block render scratch dir (deleted by runSpeakToFile's
// defer); wavPath is the resolved destination OUTSIDE that scratch, so the
// scratch RemoveAll never deletes the output.
var newFilePipeline = func(renderDir, wavPath string, args speakArgs, input adapter.InputAdapter, intel intelligence.IntelligenceAdapter) pipeline.Narrator {
	renderer, _, err := pipeline.BuildRenderer(args.Voice)
	if err != nil {
		// Unreachable post-validate() (see newPipeline); panic on a genuine bug
		// rather than degrade silently.
		panic(fmt.Sprintf("buildRenderer failed after validation (file): %v", err))
	}
	// Under a voice the render is 40 kHz — the WAVFile sink must validate the
	// per-block containers at 40 kHz, else readWAV rejects them against the
	// 24 kHz default. NewWAVFile writes no manifest.json, so there is no
	// manifest.voice to set (WithVoice is a documented no-op here) — D6 scope.
	wavOpts := []persistent.Option{}
	if args.Voice != "" {
		wavOpts = append(wavOpts, persistent.WithExpectedFormat(rvc.OutputFormat()))
	}
	return pipeline.New(
		input,
		intel,
		renderer,
		persistent.NewWAVFile(wavPath, wavOpts...),
		pipeline.PipelineDefaults{
			Level:        plan.Level(args.Level),
			OutDir:       renderDir,
			Locale:       "en",
			CodeMinLevel: listenCodeMinLevel,
		},
	)
}

// hasWAVSuffix reports whether p already ends in a .wav extension,
// case-insensitively (so "foo.WAV" is NOT double-appended to "foo.WAV.wav").
func hasWAVSuffix(p string) bool {
	return strings.EqualFold(filepath.Ext(p), ".wav")
}

// derivedWAVName builds the filename to use when output_path is a directory.
// File source → the source base name with its extension swapped to .wav
// (notes.md → notes.wav). Inline text → narration-<first-8-hex-of-hash>.wav,
// reading the sha256 hex back out of the mcptext URI. Any other / unknown
// source kind → narration.wav.
func derivedWAVName(src plan.SourceRef) string {
	switch src.Kind {
	case plan.SourceKindFile:
		base := filepath.Base(src.URI)
		stem := strings.TrimSuffix(base, filepath.Ext(base))
		if stem == "" || stem == "." {
			stem = "narration"
		}
		return stem + ".wav"
	case plan.SourceKindMCPText:
		short, ok := mcptext.InlineHash(src.URI)
		if !ok || short == "" {
			short = "inline"
		} else if len(short) > 8 {
			short = short[:8]
		}
		return "narration-" + short + ".wav"
	default:
		return "narration.wav"
	}
}

// resolveOutputPath turns a non-empty caller output_path into a concrete,
// absolute .wav file path and MkdirAll's its parent (0o755).
//
// Rule (settled in the planner-task Decisions):
//  1. Trailing path separator, ".", "..", OR an existing directory → treat as a
//     directory; place derivedWAVName(src) inside it.
//  2. Otherwise → treat as a target file; append ".wav" when missing (case-
//     insensitive; never double-append on "foo.WAV").
//  3. Always return a fully-resolved, Cleaned absolute path. An existing target
//     is overwritten in place by the sink (deliberate; local-only trust model).
//
// The no-path branch never calls this (empty output_path is handled upstream);
// an empty argument here is a programmer error and returns an internal error.
func resolveOutputPath(outputPath string, src plan.SourceRef) (string, error) {
	if outputPath == "" {
		return "", fmt.Errorf("internal_error: resolveOutputPath called with empty output_path")
	}

	isDir := strings.HasSuffix(outputPath, string(os.PathSeparator)) ||
		outputPath == "." || outputPath == ".."
	if !isDir {
		if fi, statErr := os.Stat(outputPath); statErr == nil && fi.IsDir() {
			isDir = true
		}
	}

	target := outputPath
	if isDir {
		target = filepath.Join(outputPath, derivedWAVName(src))
	} else if !hasWAVSuffix(target) {
		target += ".wav"
	}

	abs, err := filepath.Abs(target)
	if err != nil {
		return "", fmt.Errorf("caller-error: invalid_argument: resolve output_path: %w", err)
	}
	abs = filepath.Clean(abs)

	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		return "", fmt.Errorf("caller-error: invalid_argument: create output_path parent: %w", err)
	}
	return abs, nil
}

// runSpeakToFile is the composition root for one speak_to_file call. Both
// branches converge on a single speakToFileResponse.
//
// No-path branch (empty OutputPath): reuse runSpeak's ephemeral internals via
// runSpeakWithCache (so defaults/validation/leveling/refusal/transcript are
// single-sourced), then RE-WRAP the speakResponse into a speakToFileResponse
// with output_path:"" — NOT returning runSpeak's own envelope (BLOCKING-1).
//
// With-path branch: validate, pick the input adapter, resolve output_path to an
// absolute .wav OUTSIDE the render scratch, build the wav-file pipeline, narrate
// (the sink writes the one combined wav), then build the response. The render
// scratch is removed on return; the output wav survives.
func runSpeakToFile(ctx context.Context, args speakToFileArgs, cache *mcpsampling.ServerCache) (speakToFileResponse, error) {
	if args.OutputPath == "" {
		resp, err := runSpeakWithCache(ctx, args.toSpeakArgs(), cache)
		if err != nil {
			return speakToFileResponse{}, err
		}
		return speakToFileResponseFromSpeak("", resp), nil
	}

	sa := args.toSpeakArgs()
	sa.applyDefaults()
	if err := sa.validate(); err != nil {
		return speakToFileResponse{}, err
	}

	input, ref, err := inputAdapterAndRef(sa)
	if err != nil {
		return speakToFileResponse{}, err
	}

	wavPath, err := resolveOutputPath(args.OutputPath, ref)
	if err != nil {
		return speakToFileResponse{}, err
	}

	// Per-call render scratch for the per-block WAVs. The combined output wav
	// lives at wavPath (outside this dir), so the deferred RemoveAll wipes only
	// the scratch, never the caller's output.
	renderDir, err := os.MkdirTemp("", "narrate-mcp-file-")
	if err != nil {
		return speakToFileResponse{}, fmt.Errorf("internal_error: create render dir: %w", err)
	}
	defer func() {
		_ = os.RemoveAll(renderDir)
	}()

	intel := buildIntelligence(sa, cache)
	pl := newFilePipeline(renderDir, wavPath, sa, input, intel)

	receipt, err := pl.Narrate(ctx, ref, pipeline.NarrateRequest{
		Voice: genderToVoice[sa.Gender],
	})
	if err != nil {
		// Errors stop the pipeline; refusals are data and arrive on success.
		return speakToFileResponse{}, classifyPipelineErr(err)
	}

	transcript, truncated, omitted, omittedRefused := capTranscript(transcriptFromResult(receipt), transcriptMaxEntries)
	return speakToFileResponse{
		OutputPath:                    wavPath,
		BlocksWritten:                 receipt.BlocksPlayed,
		TotalDurationMs:               receipt.TotalDurationMs,
		Transcript:                    transcript,
		TranscriptTruncated:           truncated,
		TranscriptOmittedCount:        omitted,
		TranscriptOmittedRefusedCount: omittedRefused,
	}, nil
}

// speakToFileResponseFromSpeak re-wraps a speakResponse (from the ephemeral
// no-path branch) into the uniform speakToFileResponse, carrying the chosen
// output_path ("" for the no-path case). This is the BLOCKING-1 fix in code:
// the no-path branch reuses speak's internals but never returns speak's own
// envelope shape.
func speakToFileResponseFromSpeak(outputPath string, r speakResponse) speakToFileResponse {
	return speakToFileResponse{
		OutputPath:                    outputPath,
		BlocksWritten:                 r.Receipt.BlocksPlayed,
		TotalDurationMs:               r.Receipt.TotalDurationMs,
		Transcript:                    r.Transcript,
		TranscriptTruncated:           r.TranscriptTruncated,
		TranscriptOmittedCount:        r.TranscriptOmittedCount,
		TranscriptOmittedRefusedCount: r.TranscriptOmittedRefusedCount,
	}
}

// speakToFileHandler bridges the SDK's typed handler signature to
// runSpeakToFile. Mirrors speakHandler's dual-channel success result (manual
// serialized-JSON TextContent + auto-populated StructuredContent) and the
// mcpsampling session threading. It closes over deps.cache directly (like
// speakLastHandler) rather than the speakArgs-typed deps.run seam, since
// speak_to_file has its own sink-selection composition; runSpeakToFile and its
// helpers stay unit-testable via the newFilePipeline / newPipeline seams.
func speakToFileHandler(deps runDeps) func(context.Context, *mcp.CallToolRequest, speakToFileArgs) (*mcp.CallToolResult, speakToFileResponse, error) {
	return func(ctx context.Context, req *mcp.CallToolRequest, args speakToFileArgs) (*mcp.CallToolResult, speakToFileResponse, error) {
		if args.Intelligence == "mcpsampling" && req != nil && req.Session != nil {
			ctx = mcpsampling.WithSamplingClient(ctx, req.Session)
		}
		resp, err := runSpeakToFile(ctx, args, deps.cache)
		if err != nil {
			return nil, speakToFileResponse{}, err
		}
		blob, mErr := json.Marshal(resp)
		if mErr != nil {
			return nil, speakToFileResponse{}, fmt.Errorf("internal_error: marshal speak_to_file response: %w", mErr)
		}
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: string(blob)}},
		}, resp, nil
	}
}

// classifyPipelineErr — caller-error vs internal-error split, per
// Decision v2. The split rule: anything the caller could fix by changing
// the request maps to caller-error (and the wire message begins
// "caller-error: invalid_argument:"); anything the server cannot fulfill
// regardless of how the caller asks maps to internal-error (wire prefix
// "internal_error:").
//
// Context cancellation is its own class: the MCP layer treats it as
// `cancelled` already; we still tag the wire message so the classifier
// remains the single source of truth.
//
// The CATEGORY decision (caller/internal/cancel) is delegated to the shared
// errclass.Classify (#51). This root owns only the per-root wire mapping:
// the text-prefix contract, the local errors.Is message-pickers, and the %w
// wrapping of the ORIGINAL err. The message-selection errors.Is checks below
// are deliberate wire-format choices (distinct text per sentinel), NOT
// leftover dedup — they stay per-root.
func classifyPipelineErr(err error) error {
	if err == nil {
		return nil
	}
	switch errclass.Classify(err) {
	case errclass.ClassCancelled:
		return fmt.Errorf("cancelled: %w", err)
	case errclass.ClassCaller:
		// Wire-format text choice: pick the caller message per fs sentinel.
		if errors.Is(err, fs.ErrPermission) {
			return fmt.Errorf("caller-error: invalid_argument: source permission denied: %w", err)
		}
		// fs.ErrNotExist (and any future caller sentinel) -> not-found text.
		return fmt.Errorf("caller-error: invalid_argument: source not found: %w", err)
	default: // errclass.ClassInternal
		// Phase 5 of #13: mcpsampling sentinels. ErrNoSamplingClient means the
		// server failed to thread the session — operator bug, not caller bug.
		// ErrUnexpectedContentKind means the client returned non-text content
		// from a sampling request — outside the caller's control. These are
		// wire-format text choices within the internal category.
		if errors.Is(err, mcpsampling.ErrNoSamplingClient) {
			return fmt.Errorf("internal_error: sampling client missing from ctx: %w", err)
		}
		if errors.Is(err, mcpsampling.ErrUnexpectedContentKind) {
			return fmt.Errorf("internal_error: sampling reply not text: %w", err)
		}
		return fmt.Errorf("internal_error: pipeline failure: %w", err)
	}
}

// speakHandler bridges the SDK's typed handler signature to runSpeak.
// All handler errors are returned via the `error` return — the SDK
// surfaces them as IsError=true content per the SDK's documented design.
// The classification text inside the error message is the wire contract.
//
// Phase 5 of #13: when args.Intelligence == "mcpsampling", the live
// *mcp.ServerSession from the CallToolRequest is threaded into ctx via
// mcpsampling.WithSamplingClient so the adapter (constructed inside
// runSpeak via buildIntelligence) can reach the client without
// pipeline.New knowing about MCP sessions. Threading happens before
// deps.run so the runSpeak → newPipeline → Narrate path sees the
// SamplingClient on the ctx.Value chain.
//
// Issue #78 (ADR #77 D3) — dual-channel result. On the success path the
// handler returns the same speakResponse (receipt + transcript) through TWO
// independent SDK mechanisms:
//
//   - StructuredContent — auto-populated by the go-sdk from the typed `Out`
//     return value (`resp`). We do NOT set it ourselves; returning the typed
//     resp is enough. This is the machine-readable channel.
//   - Content — set MANUALLY to a single TextContent carrying the serialized
//     speakResponse JSON. Because we now return a non-nil *CallToolResult with
//     Content already set, the SDK's auto-population of Content from `Out` is
//     bypassed (it only fires when Content is unset). This duplicate blob
//     survives the Claude Code CLI's one-line tool-result collapse and is
//     readable on ctrl+o.
//
// Both channels carry the FULL speakResponse so the human-readable blob and the
// structured value never diverge in substance. Attached on the success path
// only — an error return still propagates with a nil result and no transcript.
func speakHandler(deps runDeps) func(context.Context, *mcp.CallToolRequest, speakArgs) (*mcp.CallToolResult, speakResponse, error) {
	return func(ctx context.Context, req *mcp.CallToolRequest, args speakArgs) (*mcp.CallToolResult, speakResponse, error) {
		if args.Intelligence == "mcpsampling" && req != nil && req.Session != nil {
			ctx = mcpsampling.WithSamplingClient(ctx, req.Session)
		}
		resp, err := deps.run(ctx, args)
		if err != nil {
			// Error path: nil result, zero resp, error propagated. No
			// transcript attached (success-path-only, issue #78). The SDK
			// surfaces the error as IsError=true content.
			return nil, speakResponse{}, err
		}

		// Success path: build the duplicate serialized-JSON Content blob
		// (Channel-1 D3). Marshal the FULL speakResponse so the TextContent and
		// the auto-populated StructuredContent stay substance-equal.
		blob, mErr := json.Marshal(resp)
		if mErr != nil {
			// Marshaling our own well-typed response should never fail; if it
			// does it is an internal fault, surfaced as a tool error.
			return nil, speakResponse{}, fmt.Errorf("internal_error: marshal speak response: %w", mErr)
		}
		result := &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: string(blob)},
			},
		}
		// Return the non-nil result (manual Content) AND the typed resp as Out
		// (StructuredContent auto-populated from it). Two channels, one source.
		return result, resp, nil
	}
}

// newServer constructs the MCP server with the `speak` tool registered.
// Exposed for tests so they can drive the handler in-process.
//
// Issue #25: this is where the ONE server-lifetime mcpsampling cache is
// allocated. When deps does not already carry a cache, newServer creates
// a single ServerCache(DefaultCacheCapacity); when deps.run is the
// default (unset), it wires the production run closure that threads that
// shared handle into every tool call via runSpeakWithCache. Tests that
// inject their own deps.run keep their stub untouched.
func newServer(deps runDeps) *mcp.Server {
	if deps.cache == nil {
		deps.cache = mcpsampling.NewServerCache(mcpsampling.DefaultCacheCapacity)
	}
	if deps.run == nil {
		cache := deps.cache
		deps.run = func(ctx context.Context, args speakArgs) (speakResponse, error) {
			return runSpeakWithCache(ctx, args, cache)
		}
	}
	server := mcp.NewServer(&mcp.Implementation{
		Name:    "narrate-mcp",
		Version: "0.1.0",
	}, nil)
	// Whole-response buffering convention (issue #73, listen-mode): the
	// MCP host MUST buffer the ENTIRE assistant response and hand it to
	// speak as a single `text` (or `source`) input. The planner needs the
	// whole document to decide voicing (no streaming — see CLAUDE.md
	// non-goals); partial / streamed / chunk-per-call invocations are out
	// of contract and will be planned as independent fragments, not one
	// narration. "As it arrives" is reconciled by buffering to completion,
	// then calling speak once. Stated in the Description so MCP clients see
	// it in the tool list, and mirrored in the README.
	mcp.AddTool(server, &mcp.Tool{
		Name:        "speak",
		Description: "Narrate a markdown document via TTS using the intelligent-tts-narration-library pipeline. Buffer the COMPLETE response and pass it as one `text` (or `source`) input — the planner needs the whole document and does not stream; partial/chunked calls are out of contract. Code blocks are voiced at a Level-2 floor (structural gist) for listen-mode. Returns a `receipt` (planned duration + temp dir the renderer used) AND an after-the-fact per-block `transcript`: the verbatim spoken text, level, status, per-block duration, and — for refused blocks — the refusal reason. The transcript is block-granularity only (no word-level timing) and is present both in structuredContent and as a duplicate serialized-JSON text block (expand with ctrl+o). On very large documents the transcript is capped at 200 entries (head-keep): `transcript_truncated` is then true and `transcript_omitted_count` reports how many trailing entries were dropped (the dropped blocks are still counted in the receipt, so a truncated transcript is not an exhaustive refusal ledger), and `transcript_omitted_refused_count` reports how many of those dropped tail blocks were refusals (still spoken and counted in the receipt). Refusals stay inside the plan (honesty rule) — the call still succeeds with refused blocks listed in the transcript.",
	}, speakHandler(deps))

	// speak_last — narrate Claude's most recent COMPLETED assistant turn. The
	// server reads its own Claude Code session transcript off disk (newest
	// .jsonl under the projects root), extracts the last assistant turn's text
	// EXCLUDING this speak_last invocation, and voices it through the same
	// pipeline as speak. No external Stop hook or parse script. Works from any
	// repo (user-scope registration + transcript located by mtime, not cwd).
	// On-demand only — MCP servers cannot self-trigger, so this is not the
	// "automatic every turn" path (that still needs a Stop hook). Same
	// receipt+transcript envelope as speak; takes level/gender/intelligence.
	mcp.AddTool(server, &mcp.Tool{
		Name:        speakLastToolName,
		Description: "Narrate Claude's most recent completed assistant response via TTS, without being handed the text. The server reads its own Claude Code session transcript (newest .jsonl under the projects root) and voices the last assistant turn — excluding this speak_last call itself, so it speaks the PRIOR response, not its own preamble. Use it when the user asks to \"speak/read your last response\" out loud. Works from any repo. Args mirror speak: level (1 gist | 2 summary | 3 detail, default 1), gender (female | male, default female), intelligence (none | mcpsampling, default none); optional transcript_path pins a specific transcript instead of the newest. Returns the same receipt + per-block transcript envelope as speak. Caveats: locates the newest session by file mtime (with two live sessions, pass transcript_path); assumes one assistant message per turn.",
	}, speakLastHandler(deps))

	// speak_to_file — render inline text or a text/.md file into a single .wav
	// at a caller-given output_path. The third tool (issue #105, Earshot use
	// case 3). One uniform response shape for both outcomes: output_path is the
	// written path, or "" when the audio was played ephemerally and no file was
	// written. No plan.json / manifest.json sidecars are ever produced.
	mcp.AddTool(server, &mcp.Tool{
		Name:        "speak_to_file",
		Description: "Render a markdown document (inline `text` or a `source` file path — exactly one) to a single .wav file via the intelligent-tts-narration-library pipeline. Pass `output_path` to write the audio: (a) omit or leave it empty to instead PLAY the audio ephemerally and write no file (the response then has output_path:\"\"); (b) a file path missing a .wav extension gets one appended; (c) an explicit path is overwritten in place and may sit anywhere you can write; (d) a directory path gets a derived filename inside it (the source's base name for a file, narration-<hash>.wav for inline text); (e) NO plan.json/manifest.json sidecars are written — only the one .wav. Buffer the COMPLETE response and pass it as one input — the planner needs the whole document and does not stream. Returns a single uniform shape for every outcome: the resolved output_path (or \"\"), blocks_written, total_duration_ms, and the same after-the-fact per-block transcript as speak (verbatim text, level, status, duration, and refusal reason for refused blocks; capped at 200 entries on very large documents). Refusals stay inside the plan (honesty rule) — the call still succeeds with refused blocks listed in the transcript.",
	}, speakToFileHandler(deps))

	return server
}

func main() {
	// Leave deps.run unset: newServer allocates the single server-lifetime
	// mcpsampling cache and wires the production run closure that threads
	// it through every tool call (issue #25).
	deps := runDeps{
		stderr: os.Stderr,
	}
	if err := serve(deps); err != nil {
		_, _ = fmt.Fprintln(deps.stderr, "narrate-mcp: "+err.Error())
		os.Exit(1)
	}
}

// serve runs the stdio MCP server until stdin EOF or SIGINT. Clean
// shutdown returns nil and main() exits 0; transport / server errors
// surface here and main() exits 1.
func serve(deps runDeps) error {
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()
	server := newServer(deps)
	err := server.Run(ctx, &mcp.StdioTransport{})
	// signal.NotifyContext-triggered cancellation surfaces as ctx.Err();
	// the SDK propagates that as an error. Treat it as clean shutdown.
	if errors.Is(err, context.Canceled) {
		return nil
	}
	return err
}
