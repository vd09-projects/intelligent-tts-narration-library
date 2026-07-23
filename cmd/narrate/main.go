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
//	Decision v1.9.0 (issue #16) — SUPERSEDED by issue #28. --block X with
//	  --sink=persistent was rejected at flag-validation as a placeholder; the
//	  block-level patch into an existing persistent outDir now ships
//	  (sink/persistent.PatchBlock). The combination is allowed when --out is
//	  supplied; the honesty rule is preserved by patching (every other block
//	  byte-preserved) instead of overwriting audio.wav with a single-block
//	  output. The "nothing to patch" / stale / container-mismatch refusals are
//	  enforced at runtime in PatchBlock (all exit 2).
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
	"github.com/vd09-projects/intelligent-tts-narration-library/intelligence"
	"github.com/vd09-projects/intelligent-tts-narration-library/intelligence/anthropic"
	"github.com/vd09-projects/intelligent-tts-narration-library/pipeline"
	"github.com/vd09-projects/intelligent-tts-narration-library/plan"
	"github.com/vd09-projects/intelligent-tts-narration-library/render"
	"github.com/vd09-projects/intelligent-tts-narration-library/sink"
	"github.com/vd09-projects/intelligent-tts-narration-library/sink/ephemeral"
	"github.com/vd09-projects/intelligent-tts-narration-library/sink/persistent"
)

// anthropicAPIKeyEnv is the environment variable the anthropic adapter
// reads its API key from. Named here so validate() and chooseIntelligence
// agree, and so tests can document the binding without grepping for the
// string literal.
const anthropicAPIKeyEnv = "ANTHROPIC_API_KEY"

// Voice selection is unified over pipeline's named-voice roster (#156):
// --voice is the primary selector over af-bella / am-michael (Kokoro) +
// cool-jahns / confident-neal (RVC); --gender is a deprecated alias resolved via
// pipeline.SlugForGender (female→af-bella, male→am-michael). The precedence
// funnel lives in flagSet.effectiveVoice.

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
	File   string
	Level  int
	Sink   string
	Gender string
	// Voice is the optional RVC character voice slug (cool-jahns |
	// confident-neal). Empty (the default) preserves the plain-Kokoro
	// 24 kHz path byte-identically. When set, the renderer is the render/rvc
	// decorator (40 kHz), the persistent sink validates against 40 kHz, and
	// manifest.voice records this slug (Decision D6). --voice overrides
	// --gender (the decorator picks the Kokoro source); the two together emit
	// a non-fatal stderr notice (Decision D4'). Rejected with --listen for
	// #146 (Decision D4). Per issue #146.
	Voice               string
	Block               string
	ExpectedContentHash string
	// Out is the destination directory for --sink=persistent. Required when
	// --sink=persistent; rejected when --sink=ephemeral (the ephemeral sink
	// owns its own temp-dir lifecycle). Per issue #16.
	Out string
	// Intelligence is the optional adapter that enriches prose past the
	// deterministic / verbatim threshold. "none" (the default) keeps the
	// existing nil-adapter degraded-or-refused path; "anthropic" wires
	// the direct-API adapter from intelligence/anthropic. Per issue #15.
	Intelligence string
	// Listen routes the run into the raw-mode keypress transport controller
	// (issue #83): render the whole plan, then drive afplay one block at a
	// time in response to single keys (n/b/space/g/q). Listen mode is
	// inherently ephemeral and interactive — validate() rejects it combined
	// with --block (single-block re-render) or --sink=persistent (durable
	// write). Per issue #83.
	Listen bool
}

// The minimal pipeline surface runNarrate needs is pipeline.Narrator
// (issue #27 — formerly a private narrator interface duplicated here and in
// cmd/narrate-mcp). The newPipeline seam returns it so tests can substitute a
// stub without spawning Kokoro; production wires *pipeline.Pipeline.

// newPipeline — package-level factory hook. Production builds the real
// pipeline.Pipeline; tests swap this var to inject a stub narrator. The
// returned narrator owns its own OutDir; runNarrate creates the temp
// dir and passes it through so callers can swap the factory without
// re-implementing cleanup.
//
// Sink selection branches on args.Sink (issue #16):
//   - "ephemeral" → sink/ephemeral (afplay; speaker output).
//   - "persistent" → sink/persistent (audio.wav + plan.json + manifest.json
//     into args.Out). The manifest voice + expected format are resolved at
//     composition time from the roster via args.effectiveVoice() (#156).
var newPipeline = func(outDir string, args flagSet) pipeline.Narrator {
	return newPipelineWithSink(outDir, args, chooseSink(args))
}

// newPipelineWithSink builds a pipeline with an explicit sink. Pulled out so
// the issue-#28 patch path can hold a reference to the capturing sink (which
// chooseSink returns on --block × --sink=persistent) and read the captured
// sub-plan + sub-result back after Narrate runs. Tests that stub newPipeline
// keep working; tests that exercise the patch path can stub this seam.
var newPipelineWithSink = func(outDir string, args flagSet, s sink.OutputSink) pipeline.Narrator {
	renderer, _, err := pipeline.BuildRenderer(args.effectiveVoice())
	if err != nil {
		// validate() pins the effective voice to a known roster slug before this
		// seam runs, so BuildRenderer's only reachable error — ErrUnknownVoice —
		// cannot fire here. Any error is therefore a composition-root bug
		// (validate() drifted from its contract), the same unreachable-guard
		// precedent chooseIntelligence uses. The seam returns no error (R1 — keep
		// factory-seam signatures stable), so a genuine bug panics loudly rather
		// than silently degrading to Kokoro.
		panic(fmt.Sprintf("buildRenderer failed after validation: %v", err))
	}
	return pipeline.New(
		file.New(),
		chooseIntelligence(args),
		renderer,
		s,
		pipeline.PipelineDefaults{
			Level:  plan.Level(args.Level),
			OutDir: outDir,
			Locale: "en",
		},
	)
}

// chooseIntelligence picks the intelligence.IntelligenceAdapter per
// args.Intelligence. "none" → nil (preserves the existing degraded /
// refused path for prose past the verbatim threshold). "anthropic" →
// intelligence/anthropic adapter constructed from the ANTHROPIC_API_KEY
// env var. Validate() rejects "anthropic" with an empty env, so by the
// time chooseIntelligence runs the env is present; the construction
// error is wrapped in a panic for visibility — the only path that would
// trigger it is an inconsistent validate(), which is a programmer bug.
//
// Per issue #15: cmd/narrate-mcp is untouched (mcpsampling already
// gives MCP clients intelligence for free).
//
// Per issue #32: ANTHROPIC_API_KEY accepts either an Anthropic Console
// key (sk-ant-api03-) or a `claude setup-token` subscription OAuth token
// (sk-ant-oat01-). cmd stays thin — the adapter auto-detects the
// sk-ant-oat01 prefix and switches to Authorization: Bearer auth itself
// (see anthropic.New and WithBearerAuth, including the ToS caveat). No
// extra flag or option is passed here.
func chooseIntelligence(args flagSet) intelligence.IntelligenceAdapter {
	if args.Intelligence == "anthropic" {
		a, err := anthropic.New(
			anthropic.WithAPIKey(os.Getenv(anthropicAPIKeyEnv)),
			anthropic.WithCache(anthropic.NewInMemoryCache()),
		)
		if err != nil {
			// Validate() guarantees the env var is set before this point —
			// a non-nil error here means validate() drifted from its
			// contract, which is a composition-root bug.
			panic(fmt.Sprintf("anthropic adapter construction failed after validation: %v", err))
		}
		return a
	}
	return nil
}

// chooseSink picks the OutputSink implementation per args.Sink. Pulled out
// of newPipeline so the wiring is testable without spawning a real
// pipeline.Pipeline.
func chooseSink(args flagSet) sink.OutputSink {
	if args.Sink == "persistent" {
		// Issue #28 — a --block patch into a persistent outDir must NOT route
		// the persistent sink through the pipeline: the single-block path hands
		// the sink a one-block sub-plan + sub-result, and persistent.Consume
		// would faithfully concatenate a ONE-block audio.wav, overwriting every
		// other block. Instead we capture the re-rendered block (sub-plan +
		// sub-result) without writing, and runNarrate calls persistent.PatchBlock
		// to splice it into the existing outDir. The composition root owns the
		// branch; the sink owns the bytes (planner-task.md "Where it lives").
		if args.Block != "" {
			return &capturingSink{}
		}
		// #156 — the manifest voice + expected format come from the roster
		// resolution of the effective voice (D-D/D-G): manifest.voice is the
		// engine-native id — the Kokoro engine id for a Kokoro voice
		// (byte-identical to the old gender-derived value) or the RVC character
		// slug for an RVC voice — and WithExpectedFormat is applied ONLY for the
		// 40 kHz RVC case (info.Format != the 24 kHz default), so a Kokoro voice
		// keeps the sink's 24 kHz default (F5 counter-metric). The format comes
		// from the same roster resolution as BuildRenderer's returned format, so
		// sink-expected-format and renderer-format cannot drift (D1/D-G).
		info, err := pipeline.ResolveVoice(args.effectiveVoice())
		if err != nil {
			// Unreachable — validate() pins the effective voice to the roster.
			// Empty-voice sink (loud rather than silent) if it ever fires.
			return persistent.New(args.Out)
		}
		opts := []persistent.Option{persistent.WithVoice(info.ManifestVoice)}
		if info.Format != render.DefaultFormat() {
			opts = append(opts, persistent.WithExpectedFormat(info.Format))
		}
		return persistent.New(args.Out, opts...)
	}
	return ephemeral.New()
}

// capturingSink is a non-writing OutputSink used on the --block ×
// --sink=persistent patch path (issue #28). The pipeline re-renders the
// target block and calls Consume with the one-block sub-plan + sub-result;
// capturingSink records them (and never touches the filesystem) so runNarrate
// can hand them to persistent.PatchBlock. The returned receipt reports the
// single re-rendered block so the pipeline's NarrateResult is well-formed; the
// authoritative patch receipt comes from PatchBlock.
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
			// #156 D-C — --gender is deprecated. Gated on cobra.Changed("gender")
			// (an EXPLICIT --gender), emitted here (not validate()) because this is
			// the only layer with both Changed() and a stderr writer:
			//   - --gender alone       → NoticeGenderDeprecated (steer to --voice)
			//   - --gender AND --voice  → NoticeGenderIgnored (--voice wins)
			// Pinned constants (S7) so exact-string tests assert the constant.
			if c.Flags().Changed("gender") {
				msg := pipeline.NoticeGenderDeprecated
				if c.Flags().Changed("voice") {
					msg = pipeline.NoticeGenderIgnored
				}
				_, _ = fmt.Fprintln(deps.stderr, "notice: "+msg)
			}
			return deps.run(c.Context(), args, deps.stdout, deps.stderr)
		},
	}

	cmd.Flags().StringVar(&args.File, "file", "", "path to the markdown document to narrate (required)")
	cmd.Flags().IntVar(&args.Level, "level", 1, "leveling depth: 1 (gist) | 2 (summary) | 3 (detail)")
	cmd.Flags().StringVar(&args.Sink, "sink", "ephemeral", "output sink: ephemeral | persistent")
	cmd.Flags().StringVar(&args.Gender, "gender", "female", "(DEPRECATED — use --voice) voice gender: female | male (female → af-bella, male → am-michael)")
	cmd.Flags().StringVar(&args.Voice, "voice", "", "voice (primary selector): "+pipeline.VoiceHelp()+" — default af-bella; RVC voices need the worker and are rejected with --listen")
	cmd.Flags().StringVar(&args.Block, "block", "", "re-render a single block by id (from the roster); --level is the target level for that block")
	cmd.Flags().StringVar(&args.ExpectedContentHash, "expected-content-hash", "", "warn on stderr if the document content_hash differs from this value (only meaningful with --block)")
	cmd.Flags().StringVar(&args.Out, "out", "", "output directory (required when --sink=persistent; rejected with --sink=ephemeral)")
	cmd.Flags().StringVar(&args.Intelligence, "intelligence", "none", "intelligence adapter: none | anthropic (anthropic requires "+anthropicAPIKeyEnv+")")
	cmd.Flags().BoolVar(&args.Listen, "listen", false, "interactive raw-mode transport: render the whole doc, then step blocks with single keys (n next, b back, space Stop/Replay block, g go-to, q quit). Ephemeral only; rejected with --block or --sink=persistent")

	_ = cmd.MarkFlagRequired("file")
	return cmd
}

// effectiveVoice resolves the roster slug this invocation selects (#156): an
// explicit --voice wins; otherwise the deprecated --gender alias (female→af-bella,
// male→am-michael); an empty result falls to the roster default (af-bella) inside
// ResolveVoice. Precedence keys on --voice being non-empty (its documented default
// is ""); the --gender deprecation/ignored NOTICE is emitted separately in RunE,
// where cobra.Changed is available (D-C).
func (a flagSet) effectiveVoice() string {
	if a.Voice != "" {
		return a.Voice
	}
	if slug, ok := pipeline.SlugForGender(a.Gender); ok {
		return slug
	}
	return a.Voice
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
	if _, ok := pipeline.SlugForGender(a.Gender); !ok {
		return fmt.Errorf("--gender must be female or male (got %q)", a.Gender)
	}
	// #156 — --voice enum over the full roster. Empty falls to the --gender
	// alias / roster default. Validated EAGERLY here (exit 2) so an unknown voice
	// is a caller-error BEFORE any temp-dir or render work — never a silent
	// fallback to Kokoro (honesty rule, F1 error class 1); pipeline.ErrUnknownVoice
	// from BuildRenderer is the backstop.
	if a.Voice != "" && !pipeline.IsVoice(a.Voice) {
		return fmt.Errorf("--voice must be one of: %s (got %q)", pipeline.VoiceHelp(), a.Voice)
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
	// Issue #28 — --block X with --sink=persistent is now SUPPORTED: it
	// patches one block's audio into an existing persistent outDir
	// (sink/persistent.PatchBlock), every other block byte-preserved. The
	// Decision v1.9.0 blanket flag-time refusal (and its old error string)
	// were deleted here, not bypassed. The narrowed flag-time guard is only
	// "--out must be supplied" — already enforced above by the
	// --sink=persistent requires --out check. The authoritative
	// "nothing to patch" decision (dir missing / no manifest) lives at
	// runtime in persistent.PatchBlock (ErrNothingToPatch), so a present-but-
	// empty --out refuses with a precise reason rather than a blanket guess.
	// Issue #15 — wire --intelligence:
	//   "none" (the default) keeps the existing nil-adapter degraded path.
	//   "anthropic" requires ANTHROPIC_API_KEY. Per Decision v6, a missing
	//   env var with the user opting in is a flag-validation error (exit 2)
	//   rather than a silent fallback to "none" or a runtime error. Same
	//   precedent as --block × --sink=persistent: caller-correctable input
	//   that should not silently morph into something else.
	//   Per issue #32, ANTHROPIC_API_KEY may hold either a Console key
	//   (sk-ant-api03-) or a `claude setup-token` subscription OAuth token
	//   (sk-ant-oat01-); both are valid — the adapter auto-detects the
	//   oat prefix and uses Bearer auth for it.
	switch a.Intelligence {
	case "", "none", "anthropic":
		// "" is the zero-value alias for "none" — keeps unit tests that
		// construct flagSet directly without setting every flag working,
		// and matches the user-visible "no intelligence adapter" intent.
	default:
		return fmt.Errorf("--intelligence must be none or anthropic (got %q)", a.Intelligence)
	}
	if a.Intelligence == "anthropic" && os.Getenv(anthropicAPIKeyEnv) == "" {
		return fmt.Errorf("--intelligence=anthropic requires %s to be set", anthropicAPIKeyEnv)
	}
	// Issue #83 — wire --listen mutual exclusions:
	//   --listen is an interactive, ephemeral, whole-document transport. It is
	//   incompatible with --block (a single-block re-render — there is nothing
	//   to step through) and with --sink=persistent (a durable write — the
	//   listen path is decoupled from the durable sink by design). Both are
	//   caller-correctable input, so they are flag-validation errors (exit 2),
	//   same precedent as the other --sink/--block guards above.
	if a.Listen && a.Block != "" {
		return fmt.Errorf("--listen cannot be combined with --block (listen steps the whole document; --block re-renders one block)")
	}
	if a.Listen && a.Sink == "persistent" {
		return fmt.Errorf("--listen cannot be combined with --sink=persistent (listen is ephemeral and decoupled from the durable sink)")
	}
	// #156 (Phase 2b, re-keys #146 D4) — the --listen transport plays through a
	// fixed 24 kHz oto context (listen_oto.go: otoSampleRate = 24000). A 24 kHz
	// Kokoro --voice is now compatible with --listen; only a worker-backed 40 kHz
	// RVC voice is rejected — re-keyed off RequiresWorker, NOT "--voice != ''".
	// Mirrors the existing --listen×--block / --listen×--sink=persistent guards
	// (exit 2). Full 40 kHz listen (dynamic oto.NewContext sample rate in
	// cmd/narrate/listen_oto.go) is deferred to #154.
	if a.Listen {
		if info, rerr := pipeline.ResolveVoice(a.effectiveVoice()); rerr == nil && info.RequiresWorker {
			return fmt.Errorf("--listen cannot be combined with --voice=%s (RVC is 40 kHz; listen plays through a fixed 24 kHz context — pending dynamic oto rate, #154)", a.effectiveVoice())
		}
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
		//
		// Issue #83 — the listen path transfers temp-dir removal ownership to
		// the listen loop (which removes it exactly once in its cleanup, after
		// reaping the afplay child). Suppress this defer for that path so the
		// dir is not double-owned.
		if args.Listen {
			return
		}
		_ = os.RemoveAll(outDir)
	}()

	// Issue #83 — listen-path branch. Renders the whole plan into a capturing
	// sink (decoupled from the durable sink), installs the single SIGINT/SIGTERM
	// handler, enters raw mode, and drives the keypress transport loop. It owns
	// the temp-dir removal (suppressed above). validate() has already rejected
	// --listen combined with --block or --sink=persistent.
	if args.Listen {
		return runListenMode(ctx, args, outDir, stdout, stderr)
	}

	// #156 — resolve the effective voice once (roster). validate() already pinned
	// it to a known slug, so the error is an unreachable guard; treat any error as
	// a flag error (exit 2) rather than a silent fallback.
	voiceInfo, verr := pipeline.ResolveVoice(args.effectiveVoice())
	if verr != nil {
		return fmt.Errorf("%w: %w", errFlagValidation, verr)
	}

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

	// Issue #28 — patch path detection. On --block × --sink=persistent we wire
	// a capturing sink we keep a handle to, so after Narrate re-renders the
	// block we can splice it into the existing outDir via persistent.PatchBlock.
	// On every other path the sink is built inside newPipeline as before.
	patchEligible := args.Block != "" && args.Sink == "persistent"
	var capturer *capturingSink
	var pl pipeline.Narrator
	if patchEligible {
		capturer = &capturingSink{}
		pl = newPipelineWithSink(outDir, factoryArgs, capturer)
	} else {
		pl = newPipeline(outDir, factoryArgs)
	}

	req := pipeline.NarrateRequest{
		Voice:               voiceInfo.RenderVoiceID,
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

	// Issue #28 — patch commit. Narrate re-rendered the target block into the
	// capturing sink (no bytes written). Splice it into the existing persistent
	// outDir: every other block byte-preserved, all three files written
	// atomically (PatchBlock owns the bytes + the refusal taxonomy). The
	// patch receipt supersedes the pipeline receipt for the stdout summary.
	if patchEligible {
		if !capturer.captured {
			return fmt.Errorf("internal: patch path produced no captured render result for block %s", args.Block)
		}
		// #156 — the patch records the roster manifest voice and applies 40 kHz
		// only for an RVC voice (D-D/D-G), matching chooseSink. patch.go overrides
		// manifest.Voice from WithVoice when non-empty, so only the passed value
		// changes. Kokoro voices keep the sink's 24 kHz default (F5).
		patchOpts := []persistent.Option{persistent.WithVoice(voiceInfo.ManifestVoice)}
		if voiceInfo.Format != render.DefaultFormat() {
			patchOpts = append(patchOpts, persistent.WithExpectedFormat(voiceInfo.Format))
		}
		rec, perr := persistent.PatchBlock(
			ctx, args.Out, capturer.plan, capturer.result, args.Block,
			patchOpts...,
		)
		if perr != nil {
			// PatchBlock refusals (ErrNothingToPatch / ErrStalePatch /
			// ErrContentHashMismatch / ErrUnknownBlock / ErrContainerMismatch /
			// format mismatch) carry their own sentinels; exitCodeFor maps them
			// to exit 2. A corrupt manifest / unreadable WAV surfaces as a plain
			// error → exit 1. Return as-is so the classifier sees the sentinel.
			return perr
		}
		result.SinkReceipt = rec
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
//
// Per issue #28, the persistent --block patch refusals all route to exit 2
// (caller-correctable: nothing to patch, stale outDir, cross-document hash,
// unknown block id, container/manifest mismatch, or re-render format mismatch).
// A corrupt manifest or structurally unreadable WAV inside PatchBlock surfaces
// as a plain error and falls through to exit 1 (system/pipeline failure) — it
// carries none of these sentinels.
func exitCodeFor(err error) int {
	if errors.Is(err, errFlagValidation) ||
		errors.Is(err, errBlockNotFound) ||
		errors.Is(err, persistent.ErrNothingToPatch) ||
		errors.Is(err, persistent.ErrStalePatch) ||
		errors.Is(err, persistent.ErrContentHashMismatch) ||
		errors.Is(err, persistent.ErrUnknownBlock) ||
		errors.Is(err, persistent.ErrContainerMismatch) ||
		errors.Is(err, persistent.ErrFormatMismatch) {
		return 2
	}
	return 1
}
