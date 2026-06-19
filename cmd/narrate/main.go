// Command narrate — CLI entry point for the vertical-slice demo.
//
// Wires the four edges (adapter/file + planner with nil intelligence +
// render/sherpa + sink/ephemeral) into pipeline.Pipeline and runs one
// narration over the file at --file.
//
// Exit codes:
//
//	0 — success (including plans containing refused blocks — refusal is data;
//	    AND including --expected-content-hash mismatch warnings).
//	1 — adapter / planner / renderer / sink error.
//	2 — flag / argument error (cobra), --sink=persistent (not implemented),
//	    or --block id not found in the plan.
//
// Decisions baked in:
//
//	Decision (v2) — convention: accepted. Named flags only, no positional args.
//	  --file (required), --level {1|2|3} default 1, --sink {ephemeral|persistent}
//	  default ephemeral, --gender {female|male} default female.
//	Decision (v3) — tradeoff: accepted. --sink=persistent errors fast with a
//	  clear message rather than silently falling back to ephemeral.
package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/vd09-projects/intelligent-tts-narration-library/adapter/file"
	"github.com/vd09-projects/intelligent-tts-narration-library/pipeline"
	"github.com/vd09-projects/intelligent-tts-narration-library/plan"
	"github.com/vd09-projects/intelligent-tts-narration-library/render/sherpa"
	"github.com/vd09-projects/intelligent-tts-narration-library/sink/ephemeral"
)

// genderToVoice — phase-one mapping per CLAUDE.md domain rules + sindri
// patterns A15 amendment. Female default per problem statement.
var genderToVoice = map[string]string{
	"female": "af_bella",
	"male":   "am_michael",
}

// persistentNotImplementedMsg is the stable message returned when the
// caller asks for the persistent sink in the vertical slice. Surfaced via
// stderr and used by main()'s exit-code routing; promoted to a sentinel
// (errPersistentNotImplemented) below so `errors.Is` works at the caller.
// Phase-2 work removes both.
const persistentNotImplementedMsg = "persistent sink not implemented in vertical slice (issue #7)"

// errFlagValidation — sentinel that runNarrate wraps validation errors
// with so the exit-code switch can distinguish flag errors from pipeline
// errors. Cobra flag-package errors come through RunE without this
// sentinel; we add it for our own validate() path. Keep matching exit-2
// semantics consistent.
//
// errPersistentNotImplemented routes --sink=persistent to the same exit-2
// path. Both sentinels are package-level so tests can errors.Is against
// them without reaching into runNarrate's internals.
//
// errBlockNotFound is a thin CLI-side sentinel runNarrate wraps the
// pipeline.ErrBlockNotFound with, so exitCodeFor routes "unknown
// --block id" to exit code 2 (caller-correctable input) rather than 1
// (pipeline failure). Per issue #14.
var (
	errFlagValidation           = errors.New("flag error")
	errPersistentNotImplemented = errors.New(persistentNotImplementedMsg)
	errBlockNotFound            = errors.New("block not found")
)

// flagSet — parsed flag values, populated by cobra. Pulled out so tests
// can drive the command without os.Exit.
//
// Block + ExpectedContentHash are the issue-#14 single-block re-render
// knobs. Empty Block preserves the whole-document flow; non-empty Block
// routes through the pipeline.NarrateRequest.BlockID branch.
type flagSet struct {
	File                string
	Level               int
	Sink                string
	Gender              string
	Block               string
	ExpectedContentHash string
}

// narrator — the minimal pipeline surface runNarrate needs. Pulled out
// as an interface so the newPipeline seam (and tests) can substitute a
// stub without spawning Kokoro. Production wires *pipeline.Pipeline
// (which satisfies this via its Narrate method). Matches cmd/narrate-mcp's
// Decision v5 seam pattern.
type narrator interface {
	Narrate(ctx context.Context, ref plan.SourceRef, req pipeline.NarrateRequest) (pipeline.NarrateResult, error)
}

// newPipeline — package-level factory hook. Production builds the real
// pipeline.Pipeline; tests swap this var to inject a stub narrator. The
// returned narrator owns its own OutDir; runNarrate creates the temp
// dir and passes it through so callers can swap the factory without
// re-implementing cleanup.
var newPipeline = func(outDir string, args flagSet) narrator {
	return pipeline.New(
		file.New(),
		nil, // no intelligence adapter — phase one deterministic + degraded path.
		sherpa.New(sherpa.EngineConfig{}),
		ephemeral.New(),
		pipeline.PipelineDefaults{
			Level:  plan.Level(args.Level),
			OutDir: outDir,
			Locale: "en",
		},
	)
}

// runDeps — exit-fn + IO seams injected by tests. Production uses
// os.Exit + os.Stdout + os.Stderr.
//
// stderr is threaded into the run-fn (as of issue #14) so runNarrate
// can print the block roster + hash-mismatch warning without reaching
// for the package-level os.Stderr.
type runDeps struct {
	stdout io.Writer
	stderr io.Writer
	exit   func(int)
	// run executes the wired pipeline. Default is runNarrate; tests
	// stub it to verify wiring without spawning subprocesses.
	run func(ctx context.Context, args flagSet, stdout, stderr io.Writer) error
}

// newRootCmd builds the cobra command tree. Exposed for tests.
func newRootCmd(deps runDeps) *cobra.Command {
	var args flagSet
	cmd := &cobra.Command{
		Use:   "narrate",
		Short: "Narrate a markdown document via TTS (vertical-slice demo).",
		Long: "narrate runs the file → planner → render/sherpa → sink/ephemeral " +
			"pipeline. With no intelligence adapter wired (phase one), the " +
			"planner uses the deterministic + degraded path; prose under " +
			"~120 words is read verbatim, larger prose without intelligence " +
			"is refused honestly.",
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(c *cobra.Command, _ []string) error {
			return deps.run(c.Context(), args, deps.stdout, deps.stderr)
		},
	}

	cmd.Flags().StringVar(&args.File, "file", "", "path to the markdown document to narrate (required)")
	cmd.Flags().IntVar(&args.Level, "level", 1, "leveling depth: 1 (gist) | 2 (summary) | 3 (detail)")
	cmd.Flags().StringVar(&args.Sink, "sink", "ephemeral", "output sink: ephemeral | persistent (persistent not implemented in vertical slice)")
	cmd.Flags().StringVar(&args.Gender, "gender", "female", "voice gender: female | male")
	cmd.Flags().StringVar(&args.Block, "block", "", "re-render a single block by id (from the roster); --level is the target level for that block")
	cmd.Flags().StringVar(&args.ExpectedContentHash, "expected-content-hash", "", "warn on stderr if the document content_hash differs from this value (only meaningful with --block)")

	_ = cmd.MarkFlagRequired("file")
	return cmd
}

// validate enforces enum + range checks cobra's flag types do not cover.
func (a flagSet) validate() error {
	if a.Level < 1 || a.Level > 3 {
		return fmt.Errorf("--level must be 1, 2, or 3 (got %d)", a.Level)
	}
	switch a.Sink {
	case "ephemeral", "persistent":
		// ok (persistent rejected at execution time, not flag-validation time)
	default:
		return fmt.Errorf("--sink must be ephemeral or persistent (got %q)", a.Sink)
	}
	if _, ok := genderToVoice[a.Gender]; !ok {
		return fmt.Errorf("--gender must be female or male (got %q)", a.Gender)
	}
	return nil
}

// runNarrate is the production wiring — composition root proper.
//
// Validation order (load-bearing, per issue #14):
//  1. flag validate() (--level, --sink, --gender)
//  2. --sink=persistent fast-error — fires BEFORE any --block reasoning
//     so the persistent-sink AC stays scoped to issue #16.
//  3. then everything else.
func runNarrate(ctx context.Context, args flagSet, stdout, stderr io.Writer) error {
	if err := args.validate(); err != nil {
		return fmt.Errorf("%w: %w", errFlagValidation, err)
	}
	if args.Sink == "persistent" {
		return errPersistentNotImplemented
	}

	absPath, err := filepath.Abs(args.File)
	if err != nil {
		return fmt.Errorf("resolve --file: %w", err)
	}

	// Renderer writes per-block WAVs to a fresh temp dir; ephemeral sink
	// reads them and plays via afplay (real or stubbed depending on build
	// tag). The dir is removed when this function returns — ephemeral
	// sink owns "play and forget". Persistent sink (phase 2) will own its
	// own retention policy and won't go through this path.
	outDir, err := os.MkdirTemp("", "narrate-")
	if err != nil {
		return fmt.Errorf("create out dir: %w", err)
	}
	defer func() {
		// Best-effort cleanup; a leftover dir is annoying, not fatal, so
		// we do not propagate the error. The stdout summary printed below
		// runs before this defer fires (LIFO at function return), so
		// curious users can copy the out_dir= value before the dir
		// disappears — though the dir is gone by the time the process
		// exits.
		_ = os.RemoveAll(outDir)
	}()

	pl := newPipeline(outDir, args)

	req := pipeline.NarrateRequest{
		Voice:               genderToVoice[args.Gender],
		BlockID:             args.Block,
		ExpectedContentHash: args.ExpectedContentHash,
	}
	if args.Block != "" {
		// --level on a --block invocation is the absolute target level for
		// that one block (supports downgrade L3→L1 symmetrically). Funnel
		// it through LevelOverrides so the planner re-classifies that block
		// at the requested level without disturbing the document default.
		req.LevelOverrides = map[string]plan.Level{
			args.Block: plan.Level(args.Level),
		}
	}

	result, err := pl.Narrate(ctx, plan.SourceRef{
		Kind: plan.SourceKindFile,
		URI:  absPath,
	}, req)
	if err != nil {
		if errors.Is(err, pipeline.ErrBlockNotFound) {
			return fmt.Errorf("%w: %s", errBlockNotFound, args.Block)
		}
		return err
	}

	if result.BlockHashMismatch != nil {
		// Non-fatal: the re-render still ran. The block content has
		// changed since the caller obtained the id, so the audio they
		// just heard may not match their stale roster.
		_, _ = fmt.Fprintf(stderr,
			"warning: content_hash mismatch (expected %s, got %s) — block content has changed since you got that id\n",
			result.BlockHashMismatch.Expected,
			result.BlockHashMismatch.Got,
		)
	}

	if _, err := fmt.Fprintf(stdout, "blocks_played=%d total_duration_ms=%d out_dir=%s\n",
		result.BlocksPlayed, result.TotalDurationMs, outDir); err != nil {
		return fmt.Errorf("write summary: %w", err)
	}

	// Block roster on stderr after every ephemeral whole-doc run (skip
	// when --block is set — the caller already knows which block they
	// targeted). Roster goes to stderr so it never mixes with the
	// machine-readable stdout summary; format is tab-separated so a
	// shell pipeline can `cut -f1` to grab ids.
	if args.Block == "" && args.Sink == "ephemeral" {
		printRoster(stderr, args.File, result.BlockSummaries)
	}
	return nil
}

// printRoster writes the block roster to w. One header line + one row
// per block; columns are tab-separated. lines is "start-end" when the
// span is multi-line, "start" when start==end (or when end is zero).
func printRoster(w io.Writer, file string, summaries []pipeline.BlockSummary) {
	_, _ = fmt.Fprintf(w, "# %d blocks — escalate one with: narrate --file %s --block <id> --level {2|3}\n",
		len(summaries), file)
	for _, s := range summaries {
		lines := fmt.Sprintf("%d", s.StartLine)
		if s.EndLine > 0 && s.EndLine != s.StartLine {
			lines = fmt.Sprintf("%d-%d", s.StartLine, s.EndLine)
		}
		_, _ = fmt.Fprintf(w, "%s\t%s\t%d\t%s\t%s\n",
			s.ID, s.Class, int(s.Level), s.Status, lines)
	}
}

func main() {
	deps := runDeps{
		stdout: os.Stdout,
		stderr: os.Stderr,
		exit:   os.Exit,
		run:    runNarrate,
	}
	runMain(deps)
}

// runMain executes the cobra tree using the supplied deps and routes the
// result to deps.exit exactly once. Pulled out of main() so tests can
// drive the full exit-code path through a recording exit fn without
// reaching os.Exit (which would terminate the test binary).
//
// Exit code policy (re-stated from the package doc):
//
//	0 — success path: cmd.ExecuteContext returned nil.
//	1 — adapter / planner / renderer / sink error.
//	2 — flag / validation error, --sink=persistent, or unknown --block id.
func runMain(deps runDeps) {
	cmd := newRootCmd(deps)
	cmd.SetOut(deps.stdout)
	cmd.SetErr(deps.stderr)
	err := cmd.ExecuteContext(context.Background())
	if err == nil {
		// Success path — leave the exit fn alone; production os.Exit(0)
		// is implicit at end of main(), and tests can verify by seeing
		// no recorded exit call.
		return
	}
	_, _ = fmt.Fprintln(deps.stderr, "narrate: "+err.Error())
	deps.exit(exitCodeFor(err))
}

// exitCodeFor classifies a top-level error into the slice's exit code
// taxonomy. Flag validation, --sink=persistent, and unknown --block id
// all route to 2 (caller-correctable input); any other error routes to
// 1 (pipeline / system failure). Pulled out for testability.
func exitCodeFor(err error) int {
	if errors.Is(err, errFlagValidation) ||
		errors.Is(err, errPersistentNotImplemented) ||
		errors.Is(err, errBlockNotFound) {
		return 2
	}
	return 1
}
