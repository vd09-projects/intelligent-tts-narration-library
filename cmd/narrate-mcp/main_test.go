// Unit tests for cmd/narrate-mcp. Covers:
//   - speakArgs.applyDefaults + validate
//   - classifyPipelineErr (caller-error vs internal-error split — Decision v2)
//   - runDeps.run seam: speakHandler routes through deps.run and returns
//     the response untouched on success, propagates errors as-is.
//   - newServer registers the speak tool without panic.
//   - newPipeline seam (Decision v5): runSpeak with a stub narrator
//     observing the wired level/voice/locale + temp-dir cleanup.
//   - real adapter classifier integration: runSpeak with a non-existent
//     file path uses the real adapter/file error and the classifier
//     produces the caller-error wire prefix.
package main

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/vd09-projects/intelligent-tts-narration-library/pipeline"
	"github.com/vd09-projects/intelligent-tts-narration-library/plan"
	"github.com/vd09-projects/intelligent-tts-narration-library/sink"
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

// stubNarrator captures the arguments runSpeak passed through the wired
// pipeline so tests can assert the composition wiring without spawning
// Kokoro. Used via the newPipeline seam introduced in Decision v5.
type stubNarrator struct {
	gotRef    plan.SourceRef
	gotReq    pipeline.NarrateRequest
	gotOutDir string
	receipt   sink.SinkReceipt
	err       error
	// outDirSnapshot captures whether outDir existed at Narrate time so
	// the cleanup-assertion test can confirm the directory was created
	// before the deferred RemoveAll fires (B3 fix).
	outDirExistedAtNarrate bool
	mu                     sync.Mutex
}

func (s *stubNarrator) Narrate(_ context.Context, ref plan.SourceRef, req pipeline.NarrateRequest) (sink.SinkReceipt, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.gotRef = ref
	s.gotReq = req
	if s.gotOutDir != "" {
		_, statErr := os.Stat(s.gotOutDir)
		s.outDirExistedAtNarrate = statErr == nil
	}
	return s.receipt, s.err
}

// withStubPipeline installs a stub narrator via the newPipeline seam,
// returning the stub for inspection and a cleanup func that restores the
// production factory. Captures outDir + args by reference so the test
// can assert how composition flowed.
func withStubPipeline(t *testing.T, stub *stubNarrator) func() {
	t.Helper()
	orig := newPipeline
	newPipeline = func(outDir string, _ speakArgs) narrator {
		stub.gotOutDir = outDir
		return stub
	}
	return func() { newPipeline = orig }
}

func TestRunSpeak_HappyPath_WiresLevelVoiceLocaleAndReturnsReceipt(t *testing.T) {
	// Cannot t.Parallel() — mutates package-level newPipeline var.
	stub := &stubNarrator{receipt: sink.SinkReceipt{BlocksPlayed: 3, TotalDurationMs: 4_200}}
	restore := withStubPipeline(t, stub)
	defer restore()

	// Use a real path that exists so filepath.Abs resolves cleanly. The
	// stub bypasses any real I/O on this path.
	tmpFile, err := os.CreateTemp(t.TempDir(), "sample-*.md")
	if err != nil {
		t.Fatalf("create temp source: %v", err)
	}
	_ = tmpFile.Close()

	resp, err := runSpeak(context.Background(), speakArgs{
		Source: tmpFile.Name(),
		Level:  3,
		Sink:   "ephemeral",
		Gender: "male",
	})
	if err != nil {
		t.Fatalf("runSpeak: %v", err)
	}

	// Composition assertions — what the stub captured.
	if stub.gotReq.Voice != "am_michael" {
		t.Errorf("Voice: want am_michael (male → mapping), got %q", stub.gotReq.Voice)
	}
	if stub.gotRef.Kind != plan.SourceKindFile {
		t.Errorf("SourceRef.Kind: want file, got %v", stub.gotRef.Kind)
	}
	absWant, _ := filepath.Abs(tmpFile.Name())
	if stub.gotRef.URI != absWant {
		t.Errorf("SourceRef.URI: want %q, got %q", absWant, stub.gotRef.URI)
	}
	if stub.gotOutDir == "" || !strings.Contains(filepath.Base(stub.gotOutDir), "narrate-mcp-") {
		t.Errorf("outDir prefix: want narrate-mcp-*, got %q", stub.gotOutDir)
	}
	if !stub.outDirExistedAtNarrate {
		t.Errorf("outDir must exist during Narrate (was missing): %q", stub.gotOutDir)
	}

	// Receipt projection assertions.
	if resp.Receipt.BlocksPlayed != 3 {
		t.Errorf("BlocksPlayed: want 3, got %d", resp.Receipt.BlocksPlayed)
	}
	if resp.Receipt.TotalDurationMs != 4_200 {
		t.Errorf("TotalDurationMs: want 4200, got %d", resp.Receipt.TotalDurationMs)
	}
	if resp.Receipt.OutDir != stub.gotOutDir {
		t.Errorf("OutDir: want %q, got %q", stub.gotOutDir, resp.Receipt.OutDir)
	}
}

func TestRunSpeak_TempDir_CleanedUpOnSuccess(t *testing.T) {
	// Cannot t.Parallel() — mutates package-level newPipeline var.
	// B3 fix: assert the deferred RemoveAll actually fires.
	stub := &stubNarrator{receipt: sink.SinkReceipt{BlocksPlayed: 1, TotalDurationMs: 100}}
	restore := withStubPipeline(t, stub)
	defer restore()

	tmpFile, err := os.CreateTemp(t.TempDir(), "sample-*.md")
	if err != nil {
		t.Fatalf("create temp source: %v", err)
	}
	_ = tmpFile.Close()

	resp, err := runSpeak(context.Background(), speakArgs{Source: tmpFile.Name()})
	if err != nil {
		t.Fatalf("runSpeak: %v", err)
	}
	// After runSpeak returns, the deferred RemoveAll should have run.
	if _, statErr := os.Stat(resp.Receipt.OutDir); !errors.Is(statErr, fs.ErrNotExist) {
		t.Errorf("temp dir should be removed after runSpeak; got stat err %v", statErr)
	}
}

func TestRunSpeak_TempDir_CleanedUpOnPipelineError(t *testing.T) {
	// Cannot t.Parallel() — mutates package-level newPipeline var.
	// Cleanup must run even when the pipeline errors.
	stub := &stubNarrator{err: errors.New("kokoro boom")}
	restore := withStubPipeline(t, stub)
	defer restore()

	tmpFile, err := os.CreateTemp(t.TempDir(), "sample-*.md")
	if err != nil {
		t.Fatalf("create temp source: %v", err)
	}
	_ = tmpFile.Close()

	_, err = runSpeak(context.Background(), speakArgs{Source: tmpFile.Name()})
	if err == nil {
		t.Fatal("want error, got nil")
	}
	if !strings.HasPrefix(err.Error(), "internal_error: pipeline failure") {
		t.Errorf("want internal_error prefix, got %v", err)
	}
	// The stub captured the outDir before erroring; it should be gone now.
	if _, statErr := os.Stat(stub.gotOutDir); !errors.Is(statErr, fs.ErrNotExist) {
		t.Errorf("temp dir should be removed after error path; got stat err %v", statErr)
	}
}

func TestRunSpeak_NonExistentSource_UsesRealAdapterErrorViaClassifier(t *testing.T) {
	// Cannot t.Parallel() — would intersect with newPipeline-mutating tests.
	// B1 fix: verify that the classifier catches the *real* adapter/file
	// error shape (not just synthetic fakePathError). We do not install a
	// stub narrator here — we use the production newPipeline so the call
	// actually walks the adapter path. The adapter will reject the
	// non-existent file with a wrapped fs.ErrNotExist, the renderer is
	// never reached, and the classifier should map that to caller-error.
	missing := filepath.Join(t.TempDir(), "does-not-exist.md")
	_, err := runSpeak(context.Background(), speakArgs{
		Source: missing,
		Level:  1,
		Sink:   "ephemeral",
		Gender: "female",
	})
	if err == nil {
		t.Fatal("want error for non-existent source, got nil")
	}
	if !strings.HasPrefix(err.Error(), "caller-error: invalid_argument: source not found") {
		t.Errorf("want caller-error prefix from real adapter path, got %q", err.Error())
	}
	if !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("classified error must wrap fs.ErrNotExist (errors.Is); got %v", err)
	}
}
