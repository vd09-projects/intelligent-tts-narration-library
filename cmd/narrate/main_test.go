package main

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"
)

// stubDeps builds a runDeps wired to in-memory io and a recording exit fn.
func stubDeps(runFn func(context.Context, flagSet, io.Writer) error) (*runDeps, *bytes.Buffer, *bytes.Buffer, *int) {
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
	deps, _, _, _ := stubDeps(func(_ context.Context, a flagSet, _ io.Writer) error {
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
	deps, _, _, _ := stubDeps(func(_ context.Context, a flagSet, _ io.Writer) error {
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
}

func TestRoot_MissingFile_ReturnsError(t *testing.T) {
	t.Parallel()
	deps, _, _, _ := stubDeps(func(_ context.Context, _ flagSet, _ io.Writer) error {
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
	args := flagSet{File: "/tmp/x.md", Level: 1, Sink: "persistent", Gender: "female"}

	err := runNarrate(context.Background(), args, stdout)
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
		lvl := lvl
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
	// Confirms that the validate() path wraps errFlag so main() can route
	// to exit code 2 rather than 1. Pass an invalid level via runNarrate
	// directly — adapter / planner never get called.
	stdout := &bytes.Buffer{}
	err := runNarrate(context.Background(),
		flagSet{File: "/tmp/x.md", Level: 99, Sink: "ephemeral", Gender: "female"},
		stdout,
	)
	if err == nil {
		t.Fatal("runNarrate accepted invalid level")
	}
	if !errors.Is(err, errFlag) {
		t.Errorf("runNarrate validation error did not wrap errFlag; got %v", err)
	}
	if !strings.Contains(err.Error(), "--level") {
		t.Errorf("error message should mention --level; got %q", err.Error())
	}
}
