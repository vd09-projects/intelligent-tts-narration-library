// Command narrate — CLI entry point for the vertical-slice demo.
//
// Wires the four edges (adapter/file + planner with nil intelligence +
// render/sherpa + sink/ephemeral or sink/persistent) into pipeline.Pipeline
// and runs one narration over the file at --file.
//
// Exit codes:
//
//	0 — success (including plans containing refused blocks — refusal is data;
//	    AND including --expected-content-hash mismatch warnings).
//	1 — adapter / planner / renderer / sink error.
//	2 — flag / argument error (cobra), or --block id not found in the plan.
//
// Decisions baked in:
//
//	Decision (v2) — convention: accepted. Named flags only, no positional args.
//	  --file (required), --level {1|2|3} default 1, --sink {ephemeral|persistent}
//	  default ephemeral, --gender {female|male} default female.
//	Decision (v3) — tradeoff: superseded by issue #16. The persistent sink is
//	  wired in #16; --sink=persistent now runs the real implementation. --out
//	  is required when --sink=persistent (validated at flag time).
//	Decision v1.9.0 (issue #16) — convention: experimental. --block X with
//	  --sink=persistent is rejected at flag-validation. Block-level patch into
//	  an existing persistent outDir is a follow-up; refusing the combination
//	  preserves the honesty rule (don't silently overwrite audio.wav with a
//	  single-block output).
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
	"github.com/vd09-projects/intelligent-tts-narration-library/sink"
	"github.com/vd09-projects/intelligent-tts-narration-library/sink/ephemeral"
	"github.com/vd09-projects/intelligent-tts-narration-library/sink/persistent"
)

// genderToVoice — phase-one mapping per CLAUDE.md domain rules + sindri
// patterns A15 amendment. Female default per problem statement.
var genderToVoice = map[string]string{
	"female": "af_bella",
	"male":   "am_michael",
}

// errFlagValidation — sentinel that runNarrate wraps validation errors
// with so the exit-code switch can distinguish flag errors from pipeline
// errors. Cobra flag-package errors come through RunE without this
// sentinel; we add it for our own validate() path. Keep matching exit-2
// semantics consistent.
//
// errBlockNotFound is a thin CLI-side sentinel runNarrate wraps the
// pipeline.ErrBlockNotFound with, so exitCodeFor routes "unknown
// --block id" to exit code 2 (caller-correctable input) rather than 1
// (pipeline failure). Per issue #14.
//
// The prior errPersistentNotImplemented sentinel + persistentNotImplementedMsg
// constant were removed in issue #16 when the real persistent sink landed.
// Tests that pinned the old fast-error path were pivoted to the new flag-
// validation rejection ("--out is required with --sink=persistent").
var (
	errFlagValidation = errors.New("flag error")
	errBlockNotFound  = errors.New("block not found")
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
	// Out is the destination directory for --sink=persistent. Required when
	// --sink=persistent; rejected when --sink=ephemeral (the ephemeral sink
	// owns its own temp-dir lifecycle). Per issue #16.
	Out string
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
//
// Sink selection branches on args.Sink (issue #16):
//   - "ephemeral" → sink/ephemeral (afplay; speaker output).
//   - "persistent" → sink/persistent (audio.wav + plan.json + manifest.json
//     into args.Out). The engine voice id is resolved at composition time
//     via genderToVoice[args.Gender]; validate() already pins args.Gender
//     to {female, male} so the lookup is total.
var newPipeline = func(outDir string, args flagSet) narrator {
	return pipeline.New(
		file.New(),
		nil, // no intelligence adapter — phase one deterministic + degraded path.
		sherpa.New(sherpa.EngineConfig{}),
		chooseSink(args),
		pipeline.PipelineDefaults{
			Level:  plan.Level(args.Level),
			OutDir: outDir,
			Locale: "en",
		},
	)
}

// chooseSink picks the OutputSink implementation per args.Sink. Pulled out
// of newPipeline so the wiring is testable without spawning a real
// pipeline.Pipeline.
//
// T1 (review v2): args.Gender is validated upstream to {female, male};
// genderToVoice has both keys, so the lookup is total. The defensive
// lookup-with-ok form below is documentation rather than a real check.
// A future Gender expansion that adds a value without extending
// genderToVoice would trip the test (and surface this branch).
func chooseSink(args flagSet) sink.OutputSink {
	if args.Sink == "persistent" {
		voice, ok := genderToVoice[args.Gender]
		if !ok {
			// Should be unreachable — validate() pins Gender. Sentinel:
			// pass through with empty voice (manifest.Voice will be empty,
			// loud rather than silent). Tests assert genderToVoice covers
			// every Gender value validate() accepts.
			voice = ""
		}
		return persistent.New(args.Out, persistent.WithVoice(voice))
	}
	return ephemeral.New()
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
	cmd.Flags().StringVar(&args.Sink, "sink", "ephemeral", "output sink: ephemeral | persistent")
	cmd.Flags().StringVar(&args.Gender, "gender", "female", "voice gender: female | male")
	cmd.Flags().StringVar(&args.Block, "block", "", "re-render a single block by id (from the roster); --level is the target level for that block")
	cmd.Flags().StringVar(&args.ExpectedContentHash, "expected-content-hash", "", "warn on stderr if the document content_hash differs from this value (only meaningful with --block)")
	cmd.Flags().StringVar(&args.Out, "out", "", "output directory (required when --sink=persistent; rejected with --sink=ephemeral)")

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
	// F3 (#14 review): --expected-content-hash is only meaningful with
	// --block — without --block the pipeline takes the whole-doc path and
	// never compares hashes, so a non-empty value would be a silent no-op.
	// Reject it at flag-time so the caller knows their guard is ineffective.
	if a.ExpectedContentHash != "" && a.Block == "" {
		return fmt.Errorf("--expected-content-hash is only meaningful with --block")
	}
	// Issue #16 — wire --out paired with --sink:
	//   --sink=persistent requires --out (no sensible default; honesty rule
	//     means we refuse to guess).
	//   --sink=ephemeral rejects --out (the ephemeral sink owns its own
	//     temp-dir lifecycle; honoring --out would silently waste it).
	if a.Sink == "persistent" && a.Out == "" {
		return fmt.Errorf("--out is required with --sink=persistent")
	}
	if a.Sink == "ephemeral" && a.Out != "" {
		return fmt.Errorf("--out is only meaningful with --sink=persistent")
	}
	// Decision v1.9.0 (issue #16): --block X with --sink=persistent is
	// rejected. The block-level patch into an existing persistent outDir
	// is tracked as issue #28; allowing the combination today would let
	// pipeline.Narrate return a single-block RenderResult that the
	// persistent sink would faithfully concatenate into a one-block
	// audio.wav, silently overwriting the multi-block output. Honesty rule:
	// refuse, don't corrupt.
	if a.Block != "" && a.Sink == "persistent" {
		return fmt.Errorf("--block and --sink=persistent are not yet supported together (see issue #28 for block-level patch into a persistent outDir)")
	}
	return nil
}

// runNarrate is the production wiring — composition root proper.
//
// Validation order (load-bearing, per issue #14 + #16):
//  1. flag validate() (--level, --sink, --gender, --out pairing, --block × sink).
//  2. Then pipeline wiring + invocation.
//
// Per-block WAVs always land in a renderer temp dir (used by ephemeral
// for afplay, by persistent as the source it reads to build audio.wav).
// The temp dir is cleaned up on function return regardless of sink — the
// persistent sink has already copied / concatenated what it needs into
// args.Out by then.
func runNarrate(ctx context.Context, args flagSet, stdout, stderr io.Writer) error {
	if err := args.validate(); err != nil {
		return fmt.Errorf("%w: %w", errFlagValidation, err)
	}

	absPath, err := filepath.Abs(args.File)
	if err != nil {
		return fmt.Errorf("resolve --file: %w", err)
	}

	// Renderer writes per-block WAVs to a fresh temp dir. ephemeral reads
	// + plays via afplay; persistent reads + concatenates into args.Out
	// (then the temp dir is disposable). The dir is removed when this
	// function returns either way.
	outDir, err := os.MkdirTemp("", "narrate-")
	if err != nil {
		return fmt.Errorf("create out dir: %w", err)
	}
	defer func() {
		// Best-effort cleanup; a leftover dir is annoying, not fatal, so
		// we do not propagate the error.
		_ = os.RemoveAll(outDir)
	}()

	// Build the pipeline. F2 fix: on a --block invocation, the document
	// default level must stay at L1 so the planner does NOT re-plan every
	// untargeted block at the block's escalation level (which would waste
	// planner work AND make the returned BlockSummaries roster misrepresent
	// the document's default-level shape). Pass a copy of args with Level
	// forced back to 1 into the factory; the original args.Level only
	// drives LevelOverrides[args.Block] below.
	factoryArgs := args
	if args.Block != "" {
		factoryArgs.Level = 1
	}
	pl := newPipeline(outDir, factoryArgs)

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

	// Summary on stdout — machine-readable key=value line.
	// content_hash is exposed so callers can capture it and pass it back via
	// --expected-content-hash on a later --block re-render (F1 from #14
	// review). Trailing keys are additive — older parsers must tolerate
	// unknown trailing key=value pairs.
	//
	// out_dir reports the persistent sink's --out when in persistent mode,
	// or the renderer's transient temp dir when in ephemeral mode (the
	// latter is gone by the time the process exits — useful only for
	// in-flight inspection).
	reportedOutDir := outDir
	if args.Sink == "persistent" {
		reportedOutDir = args.Out
	}
	if _, err := fmt.Fprintf(stdout, "blocks_played=%d total_duration_ms=%d out_dir=%s content_hash=%s\n",
		result.BlocksPlayed, result.TotalDurationMs, reportedOutDir, result.DocumentContentHash); err != nil {
		return fmt.Errorf("write summary: %w", err)
	}

	// Block roster on stderr after every whole-doc run (skip when --block
	// is set — the caller already knows which block they targeted). Roster
	// prints on both ephemeral and persistent (#16 — persistent users
	// benefit just as much from seeing the block index, and the manifest
	// also carries it). Roster goes to stderr so it never mixes with the
	// machine-readable stdout summary; format is tab-separated so a shell
	// pipeline can `cut -f1` to grab ids.
	if args.Block == "" {
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
//	2 — flag / validation error or unknown --block id.
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
// taxonomy. Flag validation and unknown --block id both route to 2
// (caller-correctable input); any other error routes to 1 (pipeline /
// system failure). Pulled out for testability.
//
// Per issue #16, --sink=persistent without --out now routes through
// errFlagValidation (flag-time check) rather than its own sentinel.
func exitCodeFor(err error) int {
	if errors.Is(err, errFlagValidation) ||
		errors.Is(err, errBlockNotFound) {
		return 2
	}
	return 1
}
