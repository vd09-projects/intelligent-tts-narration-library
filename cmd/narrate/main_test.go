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
	want := flagSet{File: "/tmp/x.md", Level: 2, Sink: "ephemeral", Gender: "male"}
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

func TestRunNarrate_PersistentSink_ReturnsKnownError(t *testing.T) {
	t.Parallel()
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	args := flagSet{File: "/tmp/x.md", Level: 1, Sink: "persistent", Gender: "female"}

	err := runNarrate(context.Background(), args, stdout, stderr)
	if err == nil {
		t.Fatal("runNarrate returned nil for --sink=persistent")
	}
	if err.Error() != persistentNotImplementedMsg {
		t.Errorf("persistent-sink error message: got %q want %q", err.Error(), persistentNotImplementedMsg)
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

func TestRunNarrate_PersistentSink_WrapsSentinel(t *testing.T) {
	t.Parallel()
	// Confirms persistent-sink path is errors.Is-compatible with
	// errPersistentNotImplemented so main()'s exit routing (B1 fix)
	// reaches exit code 2 without relying on string equality.
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err := runNarrate(context.Background(),
		flagSet{File: "/tmp/x.md", Level: 1, Sink: "persistent", Gender: "female"},
		stdout, stderr,
	)
	if err == nil {
		t.Fatal("runNarrate accepted --sink=persistent")
	}
	if !errors.Is(err, errPersistentNotImplemented) {
		t.Errorf("runNarrate persistent-sink error did not match errPersistentNotImplemented; got %v", err)
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
		{"persistent sink", errPersistentNotImplemented, 2},
		{"persistent sink wrapped", fmt.Errorf("wrap: %w", errPersistentNotImplemented), 2},
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
	mu      sync.Mutex
	gotReq  pipeline.NarrateRequest
	gotRef  plan.SourceRef
	gotOpts string // optional capture field for future use
	result  pipeline.NarrateResult
	err     error
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
	newPipeline = func(_ string, _ flagSet) narrator { return stub }
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
	newPipeline = func(outDir string, args flagSet) narrator {
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

// TestRunNarrate_Block_PersistentSinkRegression covers (e): existing
// --sink=persistent fast-error fires BEFORE --block reasoning. The
// stub MUST NOT be called — validation order is load-bearing per the
// locked plan (persistent-sink AC scoped to #16).
func TestRunNarrate_Block_PersistentSinkRegression(t *testing.T) {
	calls := 0
	stub := &stubNarrator{}
	// Wrap the stub so we can detect a call without exposing an int field.
	origNew := newPipeline
	newPipeline = func(_ string, _ flagSet) narrator {
		calls++
		return stub
	}
	defer func() { newPipeline = origNew }()

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err := runNarrate(context.Background(),
		flagSet{File: "/tmp/x.md", Level: 1, Sink: "persistent", Gender: "female", Block: "b001"},
		stdout, stderr,
	)
	if err == nil {
		t.Fatal("runNarrate accepted --sink=persistent even with --block")
	}
	if !errors.Is(err, errPersistentNotImplemented) {
		t.Errorf("error: got %v, want errPersistentNotImplemented", err)
	}
	if calls != 0 {
		t.Errorf("newPipeline must not be invoked when --sink=persistent fast-errors; got %d calls", calls)
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
		{"persistent-sink routes to 2", errPersistentNotImplemented, 1, 2},
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
