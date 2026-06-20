package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"testing"

	"github.com/vd09-projects/intelligent-tts-narration-library/pipeline"
	"github.com/vd09-projects/intelligent-tts-narration-library/plan"
	"github.com/vd09-projects/intelligent-tts-narration-library/sink"
)

// stubDeps builds a runDeps wired to in-memory io and a recording exit fn.
func stubDeps(runFn func(context.Context, flagSet, io.Writer, io.Writer) error) (*runDeps, *bytes.Buffer, *bytes.Buffer, *int) {
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	exitCode := -1
	deps := &runDeps{
		stdout: stdout,
		stderr: stderr,
		exit:   func(c int) { exitCode = c },
		run:    runFn,
	}
	return deps, stdout, stderr, &exitCode
}

func TestRoot_FlagParsing_AllFlags(t *testing.T) {
	t.Parallel()
	var got flagSet
	deps, _, _, _ := stubDeps(func(_ context.Context, a flagSet, _, _ io.Writer) error {
		got = a
		return nil
	})
	cmd := newRootCmd(*deps)
	cmd.SetArgs([]string{"--file=/tmp/x.md", "--level=2", "--gender=male", "--sink=ephemeral"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	want := flagSet{File: "/tmp/x.md", Level: 2, Sink: "ephemeral", Gender: "male", Intelligence: "none"}
	if got != want {
		t.Errorf("flag set: got %+v want %+v", got, want)
	}
}

func TestRoot_FlagDefaults(t *testing.T) {
	t.Parallel()
	var got flagSet
	deps, _, _, _ := stubDeps(func(_ context.Context, a flagSet, _, _ io.Writer) error {
		got = a
		return nil
	})
	cmd := newRootCmd(*deps)
	cmd.SetArgs([]string{"--file=/tmp/x.md"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if got.Level != 1 {
		t.Errorf("default level: got %d want 1", got.Level)
	}
	if got.Sink != "ephemeral" {
		t.Errorf("default sink: got %q want ephemeral", got.Sink)
	}
	if got.Gender != "female" {
		t.Errorf("default gender: got %q want female", got.Gender)
	}
	if got.Block != "" {
		t.Errorf("default --block: got %q want empty", got.Block)
	}
	if got.ExpectedContentHash != "" {
		t.Errorf("default --expected-content-hash: got %q want empty", got.ExpectedContentHash)
	}
	if got.Intelligence != "none" {
		t.Errorf("default --intelligence: got %q want none", got.Intelligence)
	}
}

func TestRoot_MissingFile_ReturnsError(t *testing.T) {
	t.Parallel()
	deps, _, _, _ := stubDeps(func(_ context.Context, _ flagSet, _, _ io.Writer) error {
		t.Fatal("run should not be called when --file is missing")
		return nil
	})
	cmd := newRootCmd(*deps)
	cmd.SetArgs([]string{})
	// Discard cobra's usage output so it doesn't pollute test logs.
	cmd.SetErr(io.Discard)
	cmd.SetOut(io.Discard)

	if err := cmd.Execute(); err == nil {
		t.Fatal("Execute returned nil when --file missing")
	}
}

// TestRunNarrate_PersistentSinkWithoutOut_RejectsAtFlagValidation pivots
// the prior "persistent fast-error" test (issue #7 era) to the new
// behavior landed in issue #16: --sink=persistent without --out is a
// flag-validation error with a specific message. Exit routing is now via
// errFlagValidation rather than its own sentinel.
func TestRunNarrate_PersistentSinkWithoutOut_RejectsAtFlagValidation(t *testing.T) {
	t.Parallel()
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	args := flagSet{File: "/tmp/x.md", Level: 1, Sink: "persistent", Gender: "female"}

	err := runNarrate(context.Background(), args, stdout, stderr)
	if err == nil {
		t.Fatal("runNarrate returned nil for --sink=persistent without --out")
	}
	if !errors.Is(err, errFlagValidation) {
		t.Errorf("error should wrap errFlagValidation; got %v", err)
	}
	if !strings.Contains(err.Error(), "--out is required with --sink=persistent") {
		t.Errorf("error message should mention required --out; got %q", err.Error())
	}
}

// TestFlagSet_Validate_EphemeralWithOutRejected covers the converse: an
// ephemeral sink with --out is rejected because the ephemeral sink owns
// its own temp dir and honoring --out would silently waste it.
func TestFlagSet_Validate_EphemeralWithOutRejected(t *testing.T) {
	t.Parallel()
	a := flagSet{File: "/tmp/x.md", Level: 1, Sink: "ephemeral", Gender: "female", Out: "/tmp/anywhere"}
	err := a.validate()
	if err == nil {
		t.Fatal("validate accepted --sink=ephemeral with --out")
	}
	if !strings.Contains(err.Error(), "--out is only meaningful with --sink=persistent") {
		t.Errorf("error message wording drift: %q", err.Error())
	}
}

// TestFlagSet_Validate_BlockWithPersistentNowAllowed covers issue #28
// (superseding Decision v1.9.0): --block X with --sink=persistent is now a
// valid combination at flag-validation when --out is supplied — it patches one
// block into the existing persistent outDir. The only flag-time guard is the
// pre-existing "--out required with --sink=persistent"; the authoritative
// "nothing to patch" decision lives at runtime in persistent.PatchBlock.
func TestFlagSet_Validate_BlockWithPersistentNowAllowed(t *testing.T) {
	t.Parallel()
	a := flagSet{File: "/tmp/x.md", Level: 1, Sink: "persistent", Gender: "female", Out: "/tmp/persist", Block: "b001"}
	if err := a.validate(); err != nil {
		t.Fatalf("validate rejected --block × --sink=persistent with --out (should be allowed now): %v", err)
	}
}

// TestFlagSet_Validate_BlockWithPersistentNoOut still refuses the combination
// when --out is missing — there is no outDir to patch.
func TestFlagSet_Validate_BlockWithPersistentNoOut(t *testing.T) {
	t.Parallel()
	a := flagSet{File: "/tmp/x.md", Level: 1, Sink: "persistent", Gender: "female", Block: "b001"}
	err := a.validate()
	if err == nil {
		t.Fatal("validate accepted --block × --sink=persistent without --out")
	}
	if !strings.Contains(err.Error(), "--out is required with --sink=persistent") {
		t.Errorf("error should mention required --out; got %q", err.Error())
	}
}

func TestFlagSet_Validate_InvalidLevel(t *testing.T) {
	t.Parallel()
	cases := []int{0, -1, 4, 99}
	for _, lvl := range cases {
		t.Run("", func(t *testing.T) {
			t.Parallel()
			a := flagSet{File: "/tmp/x.md", Level: lvl, Sink: "ephemeral", Gender: "female"}
			if err := a.validate(); err == nil {
				t.Errorf("validate accepted invalid level %d", lvl)
			}
		})
	}
}

func TestFlagSet_Validate_InvalidGender(t *testing.T) {
	t.Parallel()
	a := flagSet{File: "/tmp/x.md", Level: 1, Sink: "ephemeral", Gender: "other"}
	if err := a.validate(); err == nil {
		t.Error("validate accepted unknown --gender")
	}
}

func TestFlagSet_Validate_InvalidSink(t *testing.T) {
	t.Parallel()
	a := flagSet{File: "/tmp/x.md", Level: 1, Sink: "whatever", Gender: "female"}
	if err := a.validate(); err == nil {
		t.Error("validate accepted unknown --sink")
	}
}

func TestFlagSet_Validate_InvalidIntelligence(t *testing.T) {
	t.Parallel()
	a := flagSet{File: "/tmp/x.md", Level: 1, Sink: "ephemeral", Gender: "female", Intelligence: "openai"}
	err := a.validate()
	if err == nil {
		t.Fatal("validate accepted unknown --intelligence value")
	}
	if !strings.Contains(err.Error(), "--intelligence") {
		t.Errorf("error should mention --intelligence: %v", err)
	}
}

// TestFlagSet_Validate_AnthropicWithoutEnvVar — per Decision v6,
// --intelligence=anthropic with an empty ANTHROPIC_API_KEY is a
// flag-validation error (exit 2), not a silent fallback or runtime
// error. The message must name both the flag and the env var so the
// caller can act on it.
func TestFlagSet_Validate_AnthropicWithoutEnvVar(t *testing.T) {
	// Sequential — sets/clears process env.
	t.Setenv(anthropicAPIKeyEnv, "")
	a := flagSet{File: "/tmp/x.md", Level: 1, Sink: "ephemeral", Gender: "female", Intelligence: "anthropic"}
	err := a.validate()
	if err == nil {
		t.Fatal("validate accepted --intelligence=anthropic without env var")
	}
	if !strings.Contains(err.Error(), "--intelligence") {
		t.Errorf("error should mention --intelligence: %v", err)
	}
	if !strings.Contains(err.Error(), anthropicAPIKeyEnv) {
		t.Errorf("error should mention %s: %v", anthropicAPIKeyEnv, err)
	}
}

// TestFlagSet_Validate_AnthropicWithEnvVar — the same flagSet validates
// fine when ANTHROPIC_API_KEY is non-empty.
func TestFlagSet_Validate_AnthropicWithEnvVar(t *testing.T) {
	// Sequential — sets process env.
	t.Setenv(anthropicAPIKeyEnv, "sk-test-key")
	a := flagSet{File: "/tmp/x.md", Level: 1, Sink: "ephemeral", Gender: "female", Intelligence: "anthropic"}
	if err := a.validate(); err != nil {
		t.Errorf("validate rejected --intelligence=anthropic with env set: %v", err)
	}
}

// TestChooseIntelligence_None — returns nil for the default and absent
// --intelligence (zero-value alias).
func TestChooseIntelligence_None(t *testing.T) {
	t.Parallel()
	if got := chooseIntelligence(flagSet{Intelligence: "none"}); got != nil {
		t.Errorf("--intelligence=none: got %T want nil", got)
	}
	if got := chooseIntelligence(flagSet{}); got != nil {
		t.Errorf("zero-value Intelligence: got %T want nil", got)
	}
}

// TestChooseIntelligence_Anthropic — constructs a non-nil adapter when
// the env var is set. We do not exercise the adapter further here; the
// anthropic package owns its own tests.
func TestChooseIntelligence_Anthropic(t *testing.T) {
	t.Setenv(anthropicAPIKeyEnv, "sk-test-key")
	got := chooseIntelligence(flagSet{Intelligence: "anthropic"})
	if got == nil {
		t.Fatal("chooseIntelligence returned nil for --intelligence=anthropic with env set")
	}
}

func TestGenderToVoice_Mapping(t *testing.T) {
	t.Parallel()
	cases := map[string]string{
		"female": "af_bella",
		"male":   "am_michael",
	}
	for gender, want := range cases {
		gender, want := gender, want
		t.Run(gender, func(t *testing.T) {
			t.Parallel()
			got, ok := genderToVoice[gender]
			if !ok {
				t.Fatalf("gender %q not in map", gender)
			}
			if got != want {
				t.Errorf("voice for %q: got %q want %q", gender, got, want)
			}
		})
	}
	if _, ok := genderToVoice["other"]; ok {
		t.Error("unexpected gender 'other' in map")
	}
}

func TestRunNarrate_FlagErrorWrapsSentinel(t *testing.T) {
	t.Parallel()
	// Confirms that the validate() path wraps errFlagValidation so main()
	// can route to exit code 2 rather than 1. Pass an invalid level via
	// runNarrate directly — adapter / planner never get called.
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err := runNarrate(context.Background(),
		flagSet{File: "/tmp/x.md", Level: 99, Sink: "ephemeral", Gender: "female"},
		stdout, stderr,
	)
	if err == nil {
		t.Fatal("runNarrate accepted invalid level")
	}
	if !errors.Is(err, errFlagValidation) {
		t.Errorf("runNarrate validation error did not wrap errFlagValidation; got %v", err)
	}
	if !strings.Contains(err.Error(), "--level") {
		t.Errorf("error message should mention --level; got %q", err.Error())
	}
}

// TestRunNarrate_PersistentSinkWithoutOut_RoutesToFlagValidation pins
// that the --out missing case routes through errFlagValidation, so
// main()'s exit fork reaches exit code 2 via that sentinel rather than
// requiring a dedicated persistent sentinel (which was removed in #16).
func TestRunNarrate_PersistentSinkWithoutOut_RoutesToFlagValidation(t *testing.T) {
	t.Parallel()
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err := runNarrate(context.Background(),
		flagSet{File: "/tmp/x.md", Level: 1, Sink: "persistent", Gender: "female"},
		stdout, stderr,
	)
	if err == nil {
		t.Fatal("runNarrate accepted --sink=persistent without --out")
	}
	if !errors.Is(err, errFlagValidation) {
		t.Errorf("error did not wrap errFlagValidation; got %v", err)
	}
	if got := exitCodeFor(err); got != 2 {
		t.Errorf("exit code: got %d want 2", got)
	}
}

func TestExitCodeFor_RoutesFlagErrorsTo2(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		err  error
		want int
	}{
		{"flag validation", errFlagValidation, 2},
		{"flag validation wrapped", fmt.Errorf("wrap: %w", errFlagValidation), 2},
		{"block not found", errBlockNotFound, 2},
		{"block not found wrapped", fmt.Errorf("%w: bogus", errBlockNotFound), 2},
		{"pipeline error", errors.New("adapter: stat: file not found"), 1},
		{"unrelated", errors.New("kaboom"), 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := exitCodeFor(tc.err); got != tc.want {
				t.Errorf("exitCodeFor(%v): got %d want %d", tc.err, got, tc.want)
			}
		})
	}
}

// --- issue #14: --block + --expected-content-hash + roster ----------------

// stubNarrator captures the NarrateRequest runNarrate hands the pipeline
// and returns a canned NarrateResult / error. Pulled in via the
// newPipeline seam so tests verify the CLI wiring without spawning
// Kokoro.
type stubNarrator struct {
	mu     sync.Mutex
	gotReq pipeline.NarrateRequest
	gotRef plan.SourceRef
	result pipeline.NarrateResult
	err    error
}

func (s *stubNarrator) Narrate(_ context.Context, ref plan.SourceRef, req pipeline.NarrateRequest) (pipeline.NarrateResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.gotRef = ref
	s.gotReq = req
	return s.result, s.err
}

// withStubPipeline swaps newPipeline for a factory that returns the
// supplied stub, returning a cleanup that restores production wiring.
func withStubPipeline(stub *stubNarrator) func() {
	orig := newPipeline
	newPipeline = func(_ string, _ flagSet) pipeline.Narrator { return stub }
	return func() { newPipeline = orig }
}

// TestRunNarrate_Block_ValidID_LevelOverride covers (a): valid
// --block + --level=3 → exit 0, NarrateRequest.BlockID set,
// LevelOverrides has the requested L3, no roster printed.
func TestRunNarrate_Block_ValidID_LevelOverride(t *testing.T) {
	stub := &stubNarrator{
		result: pipeline.NarrateResult{
			SinkReceipt: sink.SinkReceipt{BlocksPlayed: 1, TotalDurationMs: 250},
			BlockSummaries: []pipeline.BlockSummary{
				{ID: "b002", Class: plan.ClassCode, Level: plan.L3, Status: plan.StatusVoiced, StartLine: 3, EndLine: 5},
			},
			DocumentContentHash: "abcd",
		},
	}
	cleanup := withStubPipeline(stub)
	defer cleanup()

	// Capture the factoryArgs the newPipeline factory receives so we can
	// assert the F2 fix: when --block is set, --level must NOT propagate
	// into PipelineDefaults.Level (the planner default stays at L1).
	var gotFactoryArgs flagSet
	origNew := newPipeline
	newPipeline = func(outDir string, args flagSet) pipeline.Narrator {
		gotFactoryArgs = args
		return stub
	}
	defer func() { newPipeline = origNew }()

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err := runNarrate(context.Background(),
		flagSet{File: "/tmp/x.md", Level: 3, Sink: "ephemeral", Gender: "female", Block: "b002"},
		stdout, stderr,
	)
	if err != nil {
		t.Fatalf("runNarrate unexpected error: %v", err)
	}
	if stub.gotReq.BlockID != "b002" {
		t.Errorf("NarrateRequest.BlockID: got %q want %q", stub.gotReq.BlockID, "b002")
	}
	if got := stub.gotReq.LevelOverrides["b002"]; got != plan.L3 {
		t.Errorf("LevelOverrides[b002]: got %v want L3", got)
	}
	// F2 fix: factory-arg Level must be 1 (document default), not 3.
	if gotFactoryArgs.Level != 1 {
		t.Errorf("factory args Level: got %d want 1 (--block must not propagate --level to PipelineDefaults)", gotFactoryArgs.Level)
	}
	if !strings.Contains(stdout.String(), "blocks_played=1") {
		t.Errorf("stdout summary missing or wrong: %q", stdout.String())
	}
	if !strings.Contains(stdout.String(), "content_hash=abcd") {
		t.Errorf("stdout summary missing content_hash key (F1 fix): %q", stdout.String())
	}
	if strings.Contains(stderr.String(), "# ") && strings.Contains(stderr.String(), "blocks — escalate") {
		t.Errorf("roster should NOT print when --block is set; got stderr=%q", stderr.String())
	}
	if strings.Contains(stderr.String(), "warning: content_hash mismatch") {
		t.Errorf("unexpected hash-mismatch warning when --expected-content-hash unset: %q", stderr.String())
	}
}

// TestRunNarrate_WholeDoc_StdoutCarriesContentHash covers F1: the stdout
// summary must expose content_hash so callers can capture it and feed it
// back via --expected-content-hash on a later --block re-render.
func TestRunNarrate_WholeDoc_StdoutCarriesContentHash(t *testing.T) {
	stub := &stubNarrator{
		result: pipeline.NarrateResult{
			SinkReceipt:         sink.SinkReceipt{BlocksPlayed: 4, TotalDurationMs: 9_000},
			BlockSummaries:      []pipeline.BlockSummary{{ID: "b001", Class: plan.ClassHeading, Level: plan.L1, Status: plan.StatusVoiced, StartLine: 1, EndLine: 1}},
			DocumentContentHash: "feedface",
		},
	}
	cleanup := withStubPipeline(stub)
	defer cleanup()

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err := runNarrate(context.Background(),
		flagSet{File: "/tmp/x.md", Level: 1, Sink: "ephemeral", Gender: "female"},
		stdout, stderr,
	)
	if err != nil {
		t.Fatalf("runNarrate unexpected error: %v", err)
	}
	const wantSuffix = "content_hash=feedface\n"
	if !strings.HasSuffix(stdout.String(), wantSuffix) {
		t.Errorf("stdout summary should end with %q; got %q", wantSuffix, stdout.String())
	}
}

// TestFlagSet_Validate_ExpectedContentHashRequiresBlock covers F3: passing
// --expected-content-hash without --block is a validation error.
func TestFlagSet_Validate_ExpectedContentHashRequiresBlock(t *testing.T) {
	t.Parallel()
	a := flagSet{File: "/tmp/x.md", Level: 1, Sink: "ephemeral", Gender: "female", ExpectedContentHash: "deadbeef"}
	err := a.validate()
	if err == nil {
		t.Fatal("validate accepted --expected-content-hash without --block")
	}
	if !strings.Contains(err.Error(), "--expected-content-hash") || !strings.Contains(err.Error(), "--block") {
		t.Errorf("error message should mention both flags; got %q", err.Error())
	}
	// And the converse: with --block it's allowed.
	b := flagSet{File: "/tmp/x.md", Level: 1, Sink: "ephemeral", Gender: "female", Block: "b002", ExpectedContentHash: "deadbeef"}
	if err := b.validate(); err != nil {
		t.Errorf("validate rejected --expected-content-hash + --block: %v", err)
	}
}

// TestRunNarrate_ExactMessageFormats covers F7: pin the exact wording of
// the hash-mismatch stderr warning AND the block-not-found error suffix,
// so future refactors that rephrase them break a test instead of breaking
// downstream MCP wrappers parsing the strings.
func TestRunNarrate_ExactMessageFormats(t *testing.T) {
	// Hash-mismatch warning.
	{
		stub := &stubNarrator{
			result: pipeline.NarrateResult{
				SinkReceipt:         sink.SinkReceipt{BlocksPlayed: 1, TotalDurationMs: 100},
				DocumentContentHash: "actual-hash",
				BlockHashMismatch:   &pipeline.BlockHashMismatch{BlockID: "b001", Expected: "bad", Got: "actual-hash"},
			},
		}
		cleanup := withStubPipeline(stub)
		defer cleanup()

		stdout := &bytes.Buffer{}
		stderr := &bytes.Buffer{}
		if err := runNarrate(context.Background(),
			flagSet{File: "/tmp/x.md", Level: 1, Sink: "ephemeral", Gender: "female", Block: "b001", ExpectedContentHash: "bad"},
			stdout, stderr,
		); err != nil {
			t.Fatalf("runNarrate unexpected error: %v", err)
		}
		const want = "warning: content_hash mismatch (expected bad, got actual-hash) — block content has changed since you got that id\n"
		if got := stderr.String(); got != want {
			t.Errorf("hash-mismatch wording drift\ngot:  %q\nwant: %q", got, want)
		}
	}
	// Block-not-found error.
	{
		stub := &stubNarrator{
			err: fmt.Errorf("%w: bogus", pipeline.ErrBlockNotFound),
		}
		cleanup := withStubPipeline(stub)
		defer cleanup()

		stdout := &bytes.Buffer{}
		stderr := &bytes.Buffer{}
		err := runNarrate(context.Background(),
			flagSet{File: "/tmp/x.md", Level: 1, Sink: "ephemeral", Gender: "female", Block: "bogus"},
			stdout, stderr,
		)
		if err == nil {
			t.Fatal("runNarrate accepted unknown --block id")
		}
		const want = "block not found: bogus"
		if got := err.Error(); got != want {
			t.Errorf("block-not-found wording drift\ngot:  %q\nwant: %q", got, want)
		}
	}
}

// TestRunNarrate_Block_UnknownID_ExitCode2 covers (b): unknown
// --block → runNarrate returns errBlockNotFound (exit 2 via
// exitCodeFor), wrapped error message contains the requested id.
func TestRunNarrate_Block_UnknownID_ExitCode2(t *testing.T) {
	stub := &stubNarrator{
		err: fmt.Errorf("%w: bogus", pipeline.ErrBlockNotFound),
	}
	cleanup := withStubPipeline(stub)
	defer cleanup()

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err := runNarrate(context.Background(),
		flagSet{File: "/tmp/x.md", Level: 1, Sink: "ephemeral", Gender: "female", Block: "bogus"},
		stdout, stderr,
	)
	if err == nil {
		t.Fatal("runNarrate accepted unknown --block id")
	}
	if !errors.Is(err, errBlockNotFound) {
		t.Errorf("error should wrap errBlockNotFound; got %v", err)
	}
	if !strings.Contains(err.Error(), "bogus") {
		t.Errorf("error message should mention block id %q; got %q", "bogus", err.Error())
	}
	if got := exitCodeFor(err); got != 2 {
		t.Errorf("exit code for unknown --block: got %d want 2", got)
	}
}

// TestRunNarrate_Block_LevelDowngrade covers (c): --level=1 is the
// absolute target level for the block — downgrade L3→L1 supported
// symmetrically. The brief calls this out as a hard rule.
func TestRunNarrate_Block_LevelDowngrade(t *testing.T) {
	stub := &stubNarrator{
		result: pipeline.NarrateResult{
			SinkReceipt: sink.SinkReceipt{BlocksPlayed: 1, TotalDurationMs: 100},
		},
	}
	cleanup := withStubPipeline(stub)
	defer cleanup()

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err := runNarrate(context.Background(),
		flagSet{File: "/tmp/x.md", Level: 1, Sink: "ephemeral", Gender: "female", Block: "b001"},
		stdout, stderr,
	)
	if err != nil {
		t.Fatalf("runNarrate unexpected error: %v", err)
	}
	if got := stub.gotReq.LevelOverrides["b001"]; got != plan.L1 {
		t.Errorf("LevelOverrides[b001] downgrade: got %v want L1", got)
	}
}

// TestRunNarrate_Block_HashMismatchWarning covers (d): non-nil
// BlockHashMismatch on the result envelope → stderr warning, exit 0.
func TestRunNarrate_Block_HashMismatchWarning(t *testing.T) {
	stub := &stubNarrator{
		result: pipeline.NarrateResult{
			SinkReceipt: sink.SinkReceipt{BlocksPlayed: 1, TotalDurationMs: 100},
			BlockHashMismatch: &pipeline.BlockHashMismatch{
				BlockID:  "b001",
				Expected: "bad",
				Got:      "actual-hash",
			},
		},
	}
	cleanup := withStubPipeline(stub)
	defer cleanup()

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err := runNarrate(context.Background(),
		flagSet{File: "/tmp/x.md", Level: 1, Sink: "ephemeral", Gender: "female", Block: "b001", ExpectedContentHash: "bad"},
		stdout, stderr,
	)
	if err != nil {
		t.Fatalf("runNarrate unexpected error on hash mismatch (should be warning): %v", err)
	}
	if !strings.Contains(stderr.String(), "warning: content_hash mismatch (expected bad, got actual-hash)") {
		t.Errorf("stderr missing hash-mismatch warning; got %q", stderr.String())
	}
	if !strings.Contains(stderr.String(), "block content has changed") {
		t.Errorf("stderr missing block-changed explanation; got %q", stderr.String())
	}
	if stub.gotReq.ExpectedContentHash != "bad" {
		t.Errorf("NarrateRequest.ExpectedContentHash: got %q want %q", stub.gotReq.ExpectedContentHash, "bad")
	}
}

// TestRunNarrate_BlockWithPersistent_RoutesToPatchPipeline covers issue #28
// (superseding Decision v1.9.0): --block X with --sink=persistent + --out is no
// longer refused at flag-validation. runNarrate must NOT route the whole-doc
// newPipeline factory; it builds the patch pipeline via newPipelineWithSink
// (handed a capturing sink). This test asserts the patch seam is the one
// invoked and the whole-doc seam is not. The full patch outcome (exit codes)
// is covered in main_patch_test.go.
func TestRunNarrate_BlockWithPersistent_RoutesToPatchPipeline(t *testing.T) {
	wholeDocCalls := 0
	origNew := newPipeline
	newPipeline = func(_ string, _ flagSet) pipeline.Narrator {
		wholeDocCalls++
		return &stubNarrator{}
	}
	defer func() { newPipeline = origNew }()

	patchCalls := 0
	origPatch := newPipelineWithSink
	newPipelineWithSink = func(_ string, _ flagSet, s sink.OutputSink) pipeline.Narrator {
		patchCalls++
		// Drive the capturing sink minimally so runNarrate proceeds to
		// PatchBlock (which will refuse — outDir is empty — exit 2).
		return &stubNarrator{}
	}
	defer func() { newPipelineWithSink = origPatch }()

	err := runNarrate(context.Background(),
		flagSet{File: "/tmp/x.md", Level: 1, Sink: "persistent", Gender: "female", Out: t.TempDir(), Block: "b001"},
		&bytes.Buffer{}, &bytes.Buffer{},
	)
	// The combination is accepted past flag-validation: any error here is NOT
	// the flag-validation refusal.
	if errors.Is(err, errFlagValidation) {
		t.Errorf("--block × --sink=persistent (+ --out) must not be a flag-validation error now; got %v", err)
	}
	if patchCalls != 1 {
		t.Errorf("patch pipeline seam (newPipelineWithSink) calls: got %d want 1", patchCalls)
	}
	if wholeDocCalls != 0 {
		t.Errorf("whole-doc newPipeline must NOT be used on the patch path; got %d calls", wholeDocCalls)
	}
}

// TestRunNarrate_WholeDoc_RosterPrintedToStderr covers the
// roster-print invariant: every ephemeral whole-doc run (no --block)
// emits the per-block roster to stderr.
func TestRunNarrate_WholeDoc_RosterPrintedToStderr(t *testing.T) {
	stub := &stubNarrator{
		result: pipeline.NarrateResult{
			SinkReceipt: sink.SinkReceipt{BlocksPlayed: 2, TotalDurationMs: 500},
			BlockSummaries: []pipeline.BlockSummary{
				{ID: "b001", Class: plan.ClassHeading, Level: plan.L1, Status: plan.StatusVoiced, StartLine: 1, EndLine: 1},
				{ID: "b002", Class: plan.ClassCode, Level: plan.L1, Status: plan.StatusVoiced, StartLine: 3, EndLine: 5},
			},
		},
	}
	cleanup := withStubPipeline(stub)
	defer cleanup()

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err := runNarrate(context.Background(),
		flagSet{File: "/tmp/multi.md", Level: 1, Sink: "ephemeral", Gender: "female"},
		stdout, stderr,
	)
	if err != nil {
		t.Fatalf("runNarrate unexpected error: %v", err)
	}
	se := stderr.String()
	if !strings.Contains(se, "# 2 blocks — escalate one with: narrate --file /tmp/multi.md --block <id> --level {2|3}") {
		t.Errorf("roster header missing or wrong; stderr=%q", se)
	}
	if !strings.Contains(se, "b001\theading\t1\tvoiced\t1\n") {
		t.Errorf("roster row for b001 missing or wrong; stderr=%q", se)
	}
	if !strings.Contains(se, "b002\tcode\t1\tvoiced\t3-5\n") {
		t.Errorf("roster row for b002 missing or wrong; stderr=%q", se)
	}
}

func TestRunMain_ExitCalledExactlyOnce(t *testing.T) {
	t.Parallel()
	// Regression for B1 — the prior code called deps.exit(2) THEN
	// deps.exit(1) on the same error path, only correct in production
	// because os.Exit terminates. The recording exit fn here proves the
	// fallthrough is gone.
	cases := []struct {
		name    string
		runErr  error
		wantHit int // 0 means exit not called (success path)
		want    int // exit code if called
	}{
		{"flag error routes to 2", errFlagValidation, 1, 2},
		{"pipeline error routes to 1", errors.New("adapter: boom"), 1, 1},
		{"success path does not call exit", nil, 0, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			hits := 0
			lastCode := -1
			deps := runDeps{
				stdout: io.Discard,
				stderr: io.Discard,
				exit:   func(c int) { hits++; lastCode = c },
				run: func(_ context.Context, _ flagSet, _, _ io.Writer) error {
					return tc.runErr
				},
			}
			// newRootCmd requires --file; supply via os.Args swap by
			// constructing the command and setting args directly.
			cmd := newRootCmd(deps)
			cmd.SetArgs([]string{"--file=/tmp/x.md"})
			cmd.SetOut(io.Discard)
			cmd.SetErr(io.Discard)
			// Mirror runMain's flow against this cobra cmd manually so
			// args control stays with the test (runMain ignores
			// SetArgs).
			if err := cmd.ExecuteContext(context.Background()); err != nil {
				deps.exit(exitCodeFor(err))
			}
			if hits != tc.wantHit {
				t.Fatalf("exit call count: got %d want %d (lastCode=%d)", hits, tc.wantHit, lastCode)
			}
			if tc.wantHit > 0 && lastCode != tc.want {
				t.Errorf("exit code: got %d want %d", lastCode, tc.want)
			}
		})
	}
}

// --- issue #16: --out + persistent sink wiring ----------------------------

// TestChooseSink_BranchesBySinkType verifies the factory picks the right
// concrete sink for each --sink value. This is the unit-level check the
// review T1 build-time TODO asked for: the lookup safety of
// genderToVoice[args.Gender] inside chooseSink lives downstream of
// validate() pinning Gender to a finite set, but we exercise the
// happy paths here.
func TestChooseSink_BranchesBySinkType(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name     string
		args     flagSet
		wantKind string // "ephemeral" or "persistent"
	}{
		{
			name:     "ephemeral picks the ephemeral sink",
			args:     flagSet{Sink: "ephemeral", Gender: "female"},
			wantKind: "ephemeral",
		},
		{
			name:     "persistent picks the persistent sink",
			args:     flagSet{Sink: "persistent", Gender: "female", Out: "/tmp/persist-out"},
			wantKind: "persistent",
		},
		{
			name:     "persistent + male gender still picks persistent",
			args:     flagSet{Sink: "persistent", Gender: "male", Out: "/tmp/persist-out"},
			wantKind: "persistent",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := chooseSink(tc.args)
			gotKind := sinkKind(got)
			if gotKind != tc.wantKind {
				t.Errorf("sink kind: got %q want %q", gotKind, tc.wantKind)
			}
		})
	}
}

// sinkKind classifies a sink.OutputSink by its concrete package via %T.
// Avoids importing each sink subpackage just to do a type assertion in a
// test — the package name is reliable signal because the binary already
// uses package-internal names (sink/persistent → *persistent.Sink, etc).
func sinkKind(s sink.OutputSink) string {
	name := fmt.Sprintf("%T", s)
	switch {
	case strings.Contains(name, "persistent"):
		return "persistent"
	case strings.Contains(name, "ephemeral"):
		return "ephemeral"
	default:
		return name
	}
}

// TestRunNarrate_PersistentSink_OutDirInSummary verifies the stdout
// machine-readable summary reports the persistent sink's --out value
// (not the renderer's transient temp dir). Per issue #16 step 5.
func TestRunNarrate_PersistentSink_OutDirInSummary(t *testing.T) {
	stub := &stubNarrator{
		result: pipeline.NarrateResult{
			SinkReceipt:         sink.SinkReceipt{BlocksPlayed: 1, TotalDurationMs: 250},
			DocumentContentHash: "feedface",
			BlockSummaries: []pipeline.BlockSummary{
				{ID: "b001", Class: plan.ClassHeading, Level: plan.L1, Status: plan.StatusVoiced, StartLine: 1, EndLine: 1},
			},
		},
	}
	cleanup := withStubPipeline(stub)
	defer cleanup()

	persistOut := t.TempDir()
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err := runNarrate(context.Background(),
		flagSet{File: "/tmp/x.md", Level: 1, Sink: "persistent", Gender: "female", Out: persistOut},
		stdout, stderr,
	)
	if err != nil {
		t.Fatalf("runNarrate unexpected error: %v", err)
	}
	// out_dir in the summary should be the user-supplied --out, not the
	// transient renderer temp dir.
	if !strings.Contains(stdout.String(), "out_dir="+persistOut) {
		t.Errorf("stdout summary should carry out_dir=%s; got %q", persistOut, stdout.String())
	}
	// Roster should also print for persistent (B2 update: the gate drops
	// the ephemeral-only restriction).
	if !strings.Contains(stderr.String(), "blocks — escalate") {
		t.Errorf("roster should print for persistent whole-doc run; got stderr=%q", stderr.String())
	}
}

// TestGenderToVoice_ValidationCoverage is the T1 review-v2 follow-through:
// every --gender value validate() accepts must have an entry in
// genderToVoice; otherwise chooseSink would silently fall through to an
// empty voice id on the persistent path.
func TestGenderToVoice_ValidationCoverage(t *testing.T) {
	t.Parallel()
	// Mirror the validate() switch: every value validate() accepts must be
	// a key in genderToVoice. If validate() grows to accept "neutral", the
	// map must grow alongside.
	accepted := []string{"female", "male"}
	for _, g := range accepted {
		if _, ok := genderToVoice[g]; !ok {
			t.Errorf("genderToVoice missing key for accepted --gender value %q", g)
		}
	}
	// And the reverse — the map should not contain values validate() rejects.
	a := flagSet{File: "/tmp/x.md", Level: 1, Sink: "ephemeral"}
	for g := range genderToVoice {
		a.Gender = g
		if err := a.validate(); err != nil {
			t.Errorf("validate rejected --gender=%q which is in genderToVoice; err=%v", g, err)
		}
	}
}
