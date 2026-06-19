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
//	Decision (v4) — convention: accepted. `errTextNotImplemented` is a
//	  known transient sentinel. Ticket #17 (mcptext in-memory adapter)
//	  replaces this path with a real implementation. Until then, the `text`
//	  arg path stays in the schema (for forward-compat) but the handler
//	  fast-errors so the contract is honest.
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

	"github.com/vd09-projects/intelligent-tts-narration-library/adapter/file"
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
var (
	errMissingSource            = errors.New("caller-error: invalid_argument: must supply either source or text")
	errBothSourceAndText        = errors.New("caller-error: invalid_argument: cannot supply both source and text")
	errTextNotImplemented       = errors.New("caller-error: invalid_argument: text arg not implemented in issue #12 (see mcptext adapter ticket #17); use source")
	errPersistentNotImplemented = errors.New("caller-error: invalid_argument: persistent sink not implemented in phase one; use ephemeral")
)

// speakArgs — JSON-tagged tool arguments. Field defaults are not applied
// by the SDK's schema generator; the handler fills zero values to the
// documented defaults before validating.
//
// jsonschema struct tags drive the SDK's auto-generated input schema.
type speakArgs struct {
	Source string `json:"source,omitempty" jsonschema:"file path to a markdown document to narrate (exactly one of source or text)"`
	Text   string `json:"text,omitempty"   jsonschema:"inline markdown text to narrate (not implemented in this release; use source)"`
	Level  int    `json:"level,omitempty"  jsonschema:"leveling depth: 1 (gist) | 2 (summary) | 3 (detail). default 1"`
	Sink   string `json:"sink,omitempty"   jsonschema:"output sink: ephemeral | persistent (persistent not implemented). default ephemeral"`
	Gender string `json:"gender,omitempty" jsonschema:"voice gender: female | male. default female"`
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
}

// validate enforces enum + range checks the SDK's schema validator does
// not (defaults applied first by applyDefaults). The XOR / text-arg /
// persistent-sink checks all return sentinel errors so the classifier and
// tests can match them with errors.Is.
func (a speakArgs) validate() error {
	if a.Source == "" && a.Text == "" {
		return errMissingSource
	}
	if a.Source != "" && a.Text != "" {
		return errBothSourceAndText
	}
	if a.Text != "" {
		return errTextNotImplemented
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

// runSpeak is the production wiring — composition root for one tool call.
// Same shape as cmd/narrate's runNarrate; the differences are arg parsing,
// the XOR source/text validation, and that we return the receipt rather
// than printing to stdout.
func runSpeak(ctx context.Context, args speakArgs) (speakResponse, error) {
	args.applyDefaults()
	if err := args.validate(); err != nil {
		return speakResponse{}, err
	}

	absPath, err := filepath.Abs(args.Source)
	if err != nil {
		return speakResponse{}, fmt.Errorf("caller-error: invalid_argument: resolve source: %w", err)
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

	pl := pipeline.New(
		file.New(),
		nil, // nil intelligence — phase one deterministic + degraded path.
		sherpa.New(sherpa.EngineConfig{}),
		ephemeral.New(),
		pipeline.PipelineDefaults{
			Level:  plan.Level(args.Level),
			OutDir: outDir,
			Locale: "en",
		},
	)

	receipt, err := pl.Narrate(ctx, plan.SourceRef{
		Kind: plan.SourceKindFile,
		URI:  absPath,
	}, pipeline.NarrateRequest{
		Voice: genderToVoice[args.Gender],
	})
	if err != nil {
		return speakResponse{}, classifyPipelineErr(err)
	}

	return speakResponse{
		Receipt: receiptFromSink(receipt, outDir),
	}, nil
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
	return fmt.Errorf("internal_error: pipeline failure: %w", err)
}

// speakHandler bridges the SDK's typed handler signature to runSpeak.
// All handler errors are returned via the `error` return — the SDK
// surfaces them as IsError=true content per the SDK's documented design.
// The classification text inside the error message is the wire contract.
func speakHandler(deps runDeps) func(context.Context, *mcp.CallToolRequest, speakArgs) (*mcp.CallToolResult, speakResponse, error) {
	return func(ctx context.Context, _ *mcp.CallToolRequest, args speakArgs) (*mcp.CallToolResult, speakResponse, error) {
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
