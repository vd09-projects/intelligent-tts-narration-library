// Unit tests for cmd/narrate-mcp. Covers:
//   - speakArgs.applyDefaults + validate
//   - classifyPipelineErr (caller-error vs internal-error split — Decision v2)
//   - runDeps.run seam: speakHandler routes through deps.run and returns
//     the response untouched on success, propagates errors as-is.
//   - newServer registers the speak tool without panic.
package main

import (
	"context"
	"errors"
	"io/fs"
	"strings"
	"testing"
)

func TestSpeakArgs_ApplyDefaults(t *testing.T) {
	t.Parallel()
	a := speakArgs{Source: "doc.md"}
	a.applyDefaults()
	if a.Level != 1 {
		t.Errorf("Level default: want 1, got %d", a.Level)
	}
	if a.Sink != "ephemeral" {
		t.Errorf("Sink default: want ephemeral, got %q", a.Sink)
	}
	if a.Gender != "female" {
		t.Errorf("Gender default: want female, got %q", a.Gender)
	}
}

func TestSpeakArgs_ApplyDefaults_PreservesNonZero(t *testing.T) {
	t.Parallel()
	a := speakArgs{Source: "doc.md", Level: 3, Sink: "ephemeral", Gender: "male"}
	a.applyDefaults()
	if a.Level != 3 || a.Sink != "ephemeral" || a.Gender != "male" {
		t.Errorf("applyDefaults stomped on caller values: %+v", a)
	}
}

func TestSpeakArgs_Validate(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		args    speakArgs
		wantErr error  // matched via errors.Is when non-nil sentinel
		wantSub string // substring match when wantErr is nil and we still expect failure
		wantOK  bool
	}{
		{
			name:    "happy path source",
			args:    speakArgs{Source: "doc.md", Level: 1, Sink: "ephemeral", Gender: "female"},
			wantOK:  true,
		},
		{
			name:    "missing both source and text",
			args:    speakArgs{Level: 1, Sink: "ephemeral", Gender: "female"},
			wantErr: errMissingSource,
		},
		{
			name:    "both source and text set",
			args:    speakArgs{Source: "doc.md", Text: "hi", Level: 1, Sink: "ephemeral", Gender: "female"},
			wantErr: errBothSourceAndText,
		},
		{
			name:    "text only triggers fast-error",
			args:    speakArgs{Text: "hi", Level: 1, Sink: "ephemeral", Gender: "female"},
			wantErr: errTextNotImplemented,
		},
		{
			name:    "sink=persistent triggers fast-error",
			args:    speakArgs{Source: "doc.md", Level: 1, Sink: "persistent", Gender: "female"},
			wantErr: errPersistentNotImplemented,
		},
		{
			name:    "level out of range low",
			args:    speakArgs{Source: "doc.md", Level: 0, Sink: "ephemeral", Gender: "female"},
			wantSub: "level must be 1, 2, or 3",
		},
		{
			name:    "level out of range high",
			args:    speakArgs{Source: "doc.md", Level: 4, Sink: "ephemeral", Gender: "female"},
			wantSub: "level must be 1, 2, or 3",
		},
		{
			name:    "unknown sink",
			args:    speakArgs{Source: "doc.md", Level: 1, Sink: "wat", Gender: "female"},
			wantSub: "sink must be ephemeral or persistent",
		},
		{
			name:    "unknown gender",
			args:    speakArgs{Source: "doc.md", Level: 1, Sink: "ephemeral", Gender: "robot"},
			wantSub: "gender must be female or male",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := tc.args.validate()
			switch {
			case tc.wantOK:
				if err != nil {
					t.Fatalf("want nil, got %v", err)
				}
			case tc.wantErr != nil:
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("want errors.Is %v, got %v", tc.wantErr, err)
				}
			case tc.wantSub != "":
				if err == nil || !strings.Contains(err.Error(), tc.wantSub) {
					t.Fatalf("want substring %q, got %v", tc.wantSub, err)
				}
			}
		})
	}
}

func TestClassifyPipelineErr(t *testing.T) {
	t.Parallel()

	// Helper for "renderer-ish" errors — generic wrapped error that the
	// classifier should treat as internal_error.
	rendererBoom := errors.New("kokoro subprocess crashed")

	cases := []struct {
		name      string
		in        error
		wantNil   bool
		wantPrefix string
	}{
		{
			name:    "nil passes through",
			in:      nil,
			wantNil: true,
		},
		{
			name:       "fs.ErrNotExist → caller-error",
			in:         fs.ErrNotExist,
			wantPrefix: "caller-error: invalid_argument: source not found",
		},
		{
			name:       "wrapped fs.ErrNotExist → caller-error",
			in:         &fakePathError{Op: "open", Path: "/no/such", Err: fs.ErrNotExist},
			wantPrefix: "caller-error: invalid_argument: source not found",
		},
		{
			name:       "fs.ErrPermission → caller-error",
			in:         fs.ErrPermission,
			wantPrefix: "caller-error: invalid_argument: source permission denied",
		},
		{
			name:       "wrapped fs.ErrPermission → caller-error",
			in:         &fakePathError{Op: "open", Path: "/locked", Err: fs.ErrPermission},
			wantPrefix: "caller-error: invalid_argument: source permission denied",
		},
		{
			name:       "context.Canceled → cancelled",
			in:         context.Canceled,
			wantPrefix: "cancelled:",
		},
		{
			name:       "context.DeadlineExceeded → cancelled",
			in:         context.DeadlineExceeded,
			wantPrefix: "cancelled:",
		},
		{
			name:       "renderer-ish error → internal_error",
			in:         rendererBoom,
			wantPrefix: "internal_error: pipeline failure",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := classifyPipelineErr(tc.in)
			if tc.wantNil {
				if got != nil {
					t.Fatalf("want nil, got %v", got)
				}
				return
			}
			if got == nil {
				t.Fatalf("want error prefixed %q, got nil", tc.wantPrefix)
			}
			if !strings.HasPrefix(got.Error(), tc.wantPrefix) {
				t.Fatalf("want prefix %q, got %q", tc.wantPrefix, got.Error())
			}
			// Wrapping invariant: the original error must remain reachable via errors.Is.
			if tc.in != nil && !errors.Is(got, tc.in) {
				t.Fatalf("classified error must wrap original (errors.Is); got %v", got)
			}
		})
	}
}

func TestSpeakHandler_HappyPath(t *testing.T) {
	t.Parallel()
	want := speakResponse{Receipt: speakReceipt{
		BlocksPlayed:    7,
		TotalDurationMs: 12_345,
		OutDir:          "/tmp/narrate-mcp-fake",
	}}
	deps := runDeps{
		run: func(_ context.Context, _ speakArgs) (speakResponse, error) {
			return want, nil
		},
	}
	h := speakHandler(deps)
	_, got, err := h(context.Background(), nil, speakArgs{Source: "doc.md"})
	if err != nil {
		t.Fatalf("want nil err, got %v", err)
	}
	if got != want {
		t.Fatalf("want %+v, got %+v", want, got)
	}
}

func TestSpeakHandler_PropagatesError(t *testing.T) {
	t.Parallel()
	deps := runDeps{
		run: func(_ context.Context, _ speakArgs) (speakResponse, error) {
			return speakResponse{}, errTextNotImplemented
		},
	}
	h := speakHandler(deps)
	_, _, err := h(context.Background(), nil, speakArgs{Text: "hi"})
	if !errors.Is(err, errTextNotImplemented) {
		t.Fatalf("want errors.Is errTextNotImplemented, got %v", err)
	}
}

func TestNewServer_RegistersSpeakTool(t *testing.T) {
	t.Parallel()
	// We only assert construction does not panic and the returned server
	// is non-nil. The SDK's tool registration validates schema shape at
	// AddTool time (panics on misconfigured tools), so a successful
	// construction is a non-trivial assertion.
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("newServer panicked: %v", r)
		}
	}()
	deps := runDeps{
		run: func(_ context.Context, _ speakArgs) (speakResponse, error) {
			return speakResponse{}, nil
		},
	}
	s := newServer(deps)
	if s == nil {
		t.Fatal("want non-nil server")
	}
}

// fakePathError mirrors os.PathError shape closely enough that errors.Is
// against fs.ErrNotExist / fs.ErrPermission resolves correctly. Used in
// classifier tests so we exercise the wrapping path rather than the bare
// sentinel — that's the realistic shape adapter/file returns.
type fakePathError struct {
	Op   string
	Path string
	Err  error
}

func (e *fakePathError) Error() string { return e.Op + " " + e.Path + ": " + e.Err.Error() }
func (e *fakePathError) Unwrap() error { return e.Err }
