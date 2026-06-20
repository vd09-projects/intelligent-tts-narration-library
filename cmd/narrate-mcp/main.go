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
//	Decision (v1) — convention: accepted. Tool response envelope is
//	  receipt-only for v1: {"receipt": {blocks_played, total_duration_ms,
//	  out_dir}}. A `plan` envelope can be added additively later under the
//	  CLAUDE.md schema_version rule.
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
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/vd09-projects/intelligent-tts-narration-library/adapter"
	"github.com/vd09-projects/intelligent-tts-narration-library/adapter/file"
	"github.com/vd09-projects/intelligent-tts-narration-library/adapter/mcptext"
	"github.com/vd09-projects/intelligent-tts-narration-library/intelligence"
	"github.com/vd09-projects/intelligent-tts-narration-library/intelligence/mcpsampling"
	"github.com/vd09-projects/intelligent-tts-narration-library/pipeline"
	"github.com/vd09-projects/intelligent-tts-narration-library/plan"
	"github.com/vd09-projects/intelligent-tts-narration-library/render/sherpa"
	"github.com/vd09-projects/intelligent-tts-narration-library/sink"
	"github.com/vd09-projects/intelligent-tts-narration-library/sink/ephemeral"
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

// speakResponse — wraps speakReceipt under the `receipt` key, leaving room
// for additive future fields (e.g. `plan`) under the schema_version rule.
type speakResponse struct {
	Receipt speakReceipt `json:"receipt"`
}

// runDeps — IO + behavior seams injected by tests. Production wires
// runSpeak (the real composition root) and stderr to os.Stderr. The run
// seam lets unit tests exercise the handler wiring without spawning
// Kokoro; the manual smoke test wires the real runSpeak.
type runDeps struct {
	stderr io.Writer
	run    func(ctx context.Context, args speakArgs) (speakResponse, error)
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
var newPipeline = func(outDir string, args speakArgs, input adapter.InputAdapter, intel intelligence.IntelligenceAdapter) pipeline.Narrator {
	return pipeline.New(
		input,
		intel,
		sherpa.New(sherpa.EngineConfig{}),
		ephemeral.New(),
		pipeline.PipelineDefaults{
			Level:  plan.Level(args.Level),
			OutDir: outDir,
			Locale: "en",
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

	intel := buildIntelligence(args)
	pl := newPipeline(outDir, args, input, intel)

	receipt, err := pl.Narrate(ctx, ref, pipeline.NarrateRequest{
		Voice: genderToVoice[args.Gender],
	})
	if err != nil {
		return speakResponse{}, classifyPipelineErr(err)
	}

	return speakResponse{
		Receipt: receiptFromSink(receipt.SinkReceipt, outDir),
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
// path. For "mcpsampling":
//
// Per-call cache: scoped to one MCP tool call. Catches intra-call
// escalation (L1 → L2 re-narration of the same block) without persisting
// across calls. Promoting to per-server lifetime is a deliberate future
// change requiring eviction policy + cross-call thread-safety review;
// out of scope for #13. (Per S4.)
func buildIntelligence(args speakArgs) intelligence.IntelligenceAdapter {
	if args.Intelligence != "mcpsampling" {
		return nil
	}
	return mcpsampling.New(
		mcpsampling.WithClientID("narrate-mcp"),
		mcpsampling.WithCache(mcpsampling.NewInMemoryCache()),
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
func classifyPipelineErr(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return fmt.Errorf("cancelled: %w", err)
	}
	if errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("caller-error: invalid_argument: source not found: %w", err)
	}
	if errors.Is(err, fs.ErrPermission) {
		return fmt.Errorf("caller-error: invalid_argument: source permission denied: %w", err)
	}
	// Phase 5 of #13: mcpsampling sentinels. ErrNoSamplingClient means the
	// server failed to thread the session — operator bug, not caller bug.
	// ErrUnexpectedContentKind means the client returned non-text content
	// from a sampling request — outside the caller's control.
	if errors.Is(err, mcpsampling.ErrNoSamplingClient) {
		return fmt.Errorf("internal_error: sampling client missing from ctx: %w", err)
	}
	if errors.Is(err, mcpsampling.ErrUnexpectedContentKind) {
		return fmt.Errorf("internal_error: sampling reply not text: %w", err)
	}
	return fmt.Errorf("internal_error: pipeline failure: %w", err)
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
func speakHandler(deps runDeps) func(context.Context, *mcp.CallToolRequest, speakArgs) (*mcp.CallToolResult, speakResponse, error) {
	return func(ctx context.Context, req *mcp.CallToolRequest, args speakArgs) (*mcp.CallToolResult, speakResponse, error) {
		if args.Intelligence == "mcpsampling" && req != nil && req.Session != nil {
			ctx = mcpsampling.WithSamplingClient(ctx, req.Session)
		}
		resp, err := deps.run(ctx, args)
		if err != nil {
			return nil, speakResponse{}, err
		}
		return nil, resp, nil
	}
}

// newServer constructs the MCP server with the `speak` tool registered.
// Exposed for tests so they can drive the handler in-process.
func newServer(deps runDeps) *mcp.Server {
	server := mcp.NewServer(&mcp.Implementation{
		Name:    "narrate-mcp",
		Version: "0.1.0",
	}, nil)
	mcp.AddTool(server, &mcp.Tool{
		Name:        "speak",
		Description: "Narrate a markdown document via TTS using the intelligent-tts-narration-library pipeline. Returns a SinkReceipt with planned duration and the temp dir the renderer used. Refusals stay inside the plan (honesty rule) — the call still succeeds.",
	}, speakHandler(deps))
	return server
}

func main() {
	deps := runDeps{
		stderr: os.Stderr,
		run:    runSpeak,
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
