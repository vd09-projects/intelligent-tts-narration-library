// Command narrate — CLI entry point for the vertical-slice demo.
//
// Wires the four edges (adapter/file + planner with nil intelligence +
// render/sherpa + sink/ephemeral) into pipeline.Pipeline and runs one
// narration over the file at --file.
//
// Exit codes:
//
//	0 — success (including plans containing refused blocks — refusal is data).
//	1 — adapter / planner / renderer / sink error.
//	2 — flag / argument error (cobra) OR --sink=persistent (not implemented).
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

// errPersistentNotImplemented is exported (lower-case but stable string)
// for test inspection via stderr capture. Phase-2 work removes this.
const persistentNotImplementedMsg = "persistent sink not implemented in vertical slice (issue #7)"

// flagSet — parsed flag values, populated by cobra. Pulled out so tests
// can drive the command without os.Exit.
type flagSet struct {
	File   string
	Level  int
	Sink   string
	Gender string
}

// runDeps — exit-fn + IO seams injected by tests. Production uses
// os.Exit + os.Stdout + os.Stderr.
type runDeps struct {
	stdout io.Writer
	stderr io.Writer
	exit   func(int)
	// run executes the wired pipeline. Default is runNarrate; tests
	// stub it to verify wiring without spawning subprocesses.
	run func(ctx context.Context, args flagSet, stdout io.Writer) error
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
			return deps.run(c.Context(), args, deps.stdout)
		},
	}

	cmd.Flags().StringVar(&args.File, "file", "", "path to the markdown document to narrate (required)")
	cmd.Flags().IntVar(&args.Level, "level", 1, "leveling depth: 1 (gist) | 2 (summary) | 3 (detail)")
	cmd.Flags().StringVar(&args.Sink, "sink", "ephemeral", "output sink: ephemeral | persistent (persistent not implemented in vertical slice)")
	cmd.Flags().StringVar(&args.Gender, "gender", "female", "voice gender: female | male")

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
func runNarrate(ctx context.Context, args flagSet, stdout io.Writer) error {
	if err := args.validate(); err != nil {
		return fmt.Errorf("%w: %w", errFlag, err)
	}
	if args.Sink == "persistent" {
		return errors.New(persistentNotImplementedMsg)
	}

	absPath, err := filepath.Abs(args.File)
	if err != nil {
		return fmt.Errorf("resolve --file: %w", err)
	}

	// Renderer writes per-block WAVs to a fresh temp dir; ephemeral sink
	// reads them and plays via afplay (real or stubbed depending on build
	// tag). Persistent sink (phase 2) would write somewhere stable.
	outDir, err := os.MkdirTemp("", "narrate-")
	if err != nil {
		return fmt.Errorf("create out dir: %w", err)
	}

	pl := pipeline.New(
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

	receipt, err := pl.Narrate(ctx, plan.SourceRef{
		Kind: plan.SourceKindFile,
		URI:  absPath,
	}, pipeline.NarrateRequest{
		Voice: genderToVoice[args.Gender],
	})
	if err != nil {
		return err
	}

	if _, err := fmt.Fprintf(stdout, "blocks_played=%d total_duration_ms=%d out_dir=%s\n",
		receipt.BlocksPlayed, receipt.TotalDurationMs, outDir); err != nil {
		return fmt.Errorf("write summary: %w", err)
	}
	return nil
}

func main() {
	deps := runDeps{
		stdout: os.Stdout,
		stderr: os.Stderr,
		exit:   os.Exit,
		run:    runNarrate,
	}
	cmd := newRootCmd(deps)
	cmd.SetOut(deps.stdout)
	cmd.SetErr(deps.stderr)
	if err := cmd.ExecuteContext(context.Background()); err != nil {
		_, _ = fmt.Fprintln(deps.stderr, "narrate: "+err.Error())
		// Distinguish flag errors (cobra returns *flagError) from runtime
		// errors at the exit code level. Persistent-sink rejection counts
		// as a flag-style error per Decision v3.
		if errors.Is(err, errFlag) || err.Error() == persistentNotImplementedMsg {
			deps.exit(2)
		}
		deps.exit(1)
	}
}

// errFlag — sentinel that runNarrate wraps validation errors with so the
// exit-code switch can distinguish flag errors from pipeline errors. Cobra
// flag-package errors come through RunE without this sentinel; we add it
// for our own validate() path. Keep matching exit-2 semantics consistent.
var errFlag = errors.New("flag error")
