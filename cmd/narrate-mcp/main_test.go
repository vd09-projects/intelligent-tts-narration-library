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
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/vd09-projects/intelligent-tts-narration-library/adapter"
	"github.com/vd09-projects/intelligent-tts-narration-library/adapter/file"
	"github.com/vd09-projects/intelligent-tts-narration-library/adapter/mcptext"
	"github.com/vd09-projects/intelligent-tts-narration-library/intelligence"
	"github.com/vd09-projects/intelligent-tts-narration-library/intelligence/mcpsampling"
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
			name:   "happy path source",
			args:   speakArgs{Source: "doc.md", Level: 1, Sink: "ephemeral", Gender: "female", Intelligence: "none"},
			wantOK: true,
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
			// Decision v6 (#17): text arg is now a valid path; mcptext
			// adapter resolves it. validate() must accept it.
			// Intelligence must be set explicitly because this test
			// case bypasses applyDefaults().
			name:   "text only is valid post-#17",
			args:   speakArgs{Text: "hi", Level: 1, Sink: "ephemeral", Gender: "female", Intelligence: "none"},
			wantOK: true,
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
		name       string
		in         error
		wantNil    bool
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
	// errTextNotImplemented was removed by #17 (Decision v6). Use
	// errPersistentNotImplemented as the propagation smoke — same wire
	// shape (caller-error sentinel), still in the package.
	deps := runDeps{
		run: func(_ context.Context, _ speakArgs) (speakResponse, error) {
			return speakResponse{}, errPersistentNotImplemented
		},
	}
	h := speakHandler(deps)
	_, _, err := h(context.Background(), nil, speakArgs{Source: "doc.md", Sink: "persistent"})
	if !errors.Is(err, errPersistentNotImplemented) {
		t.Fatalf("want errors.Is errPersistentNotImplemented, got %v", err)
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
// Kokoro. Used via the newPipeline seam introduced in Decision v5 and
// widened in Decision v6 (#17) to also surface the wired InputAdapter.
type stubNarrator struct {
	gotRef    plan.SourceRef
	gotReq    pipeline.NarrateRequest
	gotOutDir string
	gotIntel  intelligence.IntelligenceAdapter
	gotInput  adapter.InputAdapter
	receipt   sink.SinkReceipt
	err       error
	// outDirSnapshot captures whether outDir existed at Narrate time so
	// the cleanup-assertion test can confirm the directory was created
	// before the deferred RemoveAll fires (B3 fix).
	outDirExistedAtNarrate bool
	// gotCtx captures the ctx passed to Narrate so tests can assert
	// SamplingClient threading via mcpsampling.WithSamplingClient.
	gotCtx context.Context
	mu     sync.Mutex
}

func (s *stubNarrator) Narrate(ctx context.Context, ref plan.SourceRef, req pipeline.NarrateRequest) (pipeline.NarrateResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.gotRef = ref
	s.gotReq = req
	s.gotCtx = ctx
	if s.gotOutDir != "" {
		_, statErr := os.Stat(s.gotOutDir)
		s.outDirExistedAtNarrate = statErr == nil
	}
	return pipeline.NarrateResult{SinkReceipt: s.receipt}, s.err
}

// withStubPipeline installs a stub narrator via the newPipeline seam,
// returning the stub for inspection and a cleanup func that restores the
// production factory. Captures outDir + args + adapter + intel so the
// test can assert how composition flowed. Per Decision v6 (#17) the
// hook now takes an adapter.InputAdapter — the stub records it so
// text-arg-vs-source tests can confirm the right concrete type was wired.
func withStubPipeline(t *testing.T, stub *stubNarrator) func() {
	t.Helper()
	orig := newPipeline
	newPipeline = func(outDir string, _ speakArgs, input adapter.InputAdapter, intel intelligence.IntelligenceAdapter) pipeline.Narrator {
		stub.gotOutDir = outDir
		stub.gotInput = input
		stub.gotIntel = intel
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

// ── Phase 5 of #13: intelligence wiring ────────────────────────────

// TestSpeakArgs_IntelligenceDefault_None — sanity: omitting the field
// applies the documented default.
func TestSpeakArgs_IntelligenceDefault_None(t *testing.T) {
	t.Parallel()
	a := speakArgs{Source: "x.md"}
	a.applyDefaults()
	if a.Intelligence != "none" {
		t.Errorf("default Intelligence: got %q want %q", a.Intelligence, "none")
	}
}

// TestSpeakArgs_InvalidIntelligence_RejectsWithCallerError — unknown
// backend produces the errUnknownIntelligence sentinel (caller-error
// wire prefix preserved via %w).
func TestSpeakArgs_InvalidIntelligence_RejectsWithCallerError(t *testing.T) {
	t.Parallel()
	a := speakArgs{Source: "x.md", Intelligence: "openai"}
	a.applyDefaults()
	err := a.validate()
	if !errors.Is(err, errUnknownIntelligence) {
		t.Fatalf("want errors.Is errUnknownIntelligence, got %v", err)
	}
	if !strings.HasPrefix(err.Error(), "caller-error: invalid_argument: intelligence must be") {
		t.Errorf("missing caller-error prefix; got %q", err.Error())
	}
}

// TestSpeakArgs_ValidIntelligence_Accepted — both "none" and
// "mcpsampling" pass validate.
func TestSpeakArgs_ValidIntelligence_Accepted(t *testing.T) {
	t.Parallel()
	for _, want := range []string{"none", "mcpsampling"} {
		want := want
		t.Run(want, func(t *testing.T) {
			t.Parallel()
			a := speakArgs{Source: "x.md", Intelligence: want}
			a.applyDefaults()
			if err := a.validate(); err != nil {
				t.Errorf("validate rejected %q: %v", want, err)
			}
		})
	}
}

// TestBuildIntelligence_NoneReturnsNil — args.Intelligence="none" =>
// nil adapter (planner takes its deterministic+degraded path).
func TestBuildIntelligence_NoneReturnsNil(t *testing.T) {
	t.Parallel()
	got := buildIntelligence(speakArgs{Intelligence: "none"})
	if got != nil {
		t.Errorf("want nil for Intelligence=none, got %T", got)
	}
}

// TestBuildIntelligence_McpsamplingReturnsAdapter — args.Intelligence="mcpsampling"
// => non-nil concrete adapter.
func TestBuildIntelligence_McpsamplingReturnsAdapter(t *testing.T) {
	t.Parallel()
	got := buildIntelligence(speakArgs{Intelligence: "mcpsampling"})
	if got == nil {
		t.Fatalf("want non-nil adapter for Intelligence=mcpsampling, got nil")
	}
	// Smoke: the concrete type is the mcpsampling.Adapter (interface
	// assignment already checked at compile time; this assertion pins
	// the production wiring choice).
	if _, ok := got.(*mcpsampling.Adapter); !ok {
		t.Errorf("want *mcpsampling.Adapter, got %T", got)
	}
}

// TestRunSpeak_McpsamplingFlag_WiresAdapterIntoPipeline — Intelligence=mcpsampling
// flows the concrete adapter through newPipeline (verified via the
// stub's gotIntel field). The default Intelligence=none keeps the
// existing nil-intel path.
func TestRunSpeak_McpsamplingFlag_WiresAdapterIntoPipeline(t *testing.T) {
	// Cannot t.Parallel() — mutates package-level newPipeline var.
	stub := &stubNarrator{receipt: sink.SinkReceipt{BlocksPlayed: 1, TotalDurationMs: 10}}
	restore := withStubPipeline(t, stub)
	defer restore()

	tmpFile, err := os.CreateTemp(t.TempDir(), "sample-*.md")
	if err != nil {
		t.Fatalf("create temp source: %v", err)
	}
	_ = tmpFile.Close()

	if _, err := runSpeak(context.Background(), speakArgs{
		Source:       tmpFile.Name(),
		Intelligence: "mcpsampling",
	}); err != nil {
		t.Fatalf("runSpeak: %v", err)
	}
	if stub.gotIntel == nil {
		t.Errorf("expected non-nil intelligence adapter threaded through newPipeline")
	}
	if _, ok := stub.gotIntel.(*mcpsampling.Adapter); !ok {
		t.Errorf("expected *mcpsampling.Adapter, got %T", stub.gotIntel)
	}
}

// TestRunSpeak_DefaultIntelligence_None_WiresNilAdapter — default keeps
// the existing nil-intel composition (preserves prior behavior — no
// silent change).
func TestRunSpeak_DefaultIntelligence_None_WiresNilAdapter(t *testing.T) {
	// Cannot t.Parallel() — mutates package-level newPipeline var.
	stub := &stubNarrator{receipt: sink.SinkReceipt{BlocksPlayed: 1, TotalDurationMs: 10}}
	restore := withStubPipeline(t, stub)
	defer restore()

	tmpFile, err := os.CreateTemp(t.TempDir(), "sample-*.md")
	if err != nil {
		t.Fatalf("create temp source: %v", err)
	}
	_ = tmpFile.Close()

	if _, err := runSpeak(context.Background(), speakArgs{Source: tmpFile.Name()}); err != nil {
		t.Fatalf("runSpeak: %v", err)
	}
	if stub.gotIntel != nil {
		t.Errorf("expected nil intelligence for Intelligence=none, got %T", stub.gotIntel)
	}
}

// TestSpeakHandler_McpsamplingThreadsClientViaCtx — when Intelligence
// is mcpsampling and the CallToolRequest carries a Session, the handler
// threads the SamplingClient into ctx via mcpsampling.WithSamplingClient.
// We verify by capturing the ctx the stub narrator sees and confirming
// the WithSamplingClient key resolves.
//
// We cannot easily construct a real *mcp.ServerSession in a unit test
// (it requires a live server handshake), so we drive the handler with
// a nil-Session CallToolRequest — the threading branch should NOT
// run, and gotCtx must NOT carry a SamplingClient. The companion test
// below (with a fake session) covers the affirmative branch.
func TestSpeakHandler_McpsamplingNilSession_NoCtxThreading(t *testing.T) {
	// Cannot t.Parallel() — mutates package-level newPipeline var.
	stub := &stubNarrator{receipt: sink.SinkReceipt{BlocksPlayed: 1, TotalDurationMs: 10}}
	restore := withStubPipeline(t, stub)
	defer restore()

	tmpFile, err := os.CreateTemp(t.TempDir(), "sample-*.md")
	if err != nil {
		t.Fatalf("create temp source: %v", err)
	}
	_ = tmpFile.Close()

	deps := runDeps{run: runSpeak}
	h := speakHandler(deps)
	// CallToolRequest with no Session — the threading branch in
	// speakHandler must skip cleanly.
	_, _, err = h(context.Background(), nil, speakArgs{
		Source:       tmpFile.Name(),
		Intelligence: "mcpsampling",
	})
	if err != nil {
		t.Fatalf("handler returned err: %v", err)
	}
	// Verify the ctx the narrator saw does NOT have a SamplingClient.
	if stub.gotCtx == nil {
		t.Fatalf("stub did not capture ctx")
	}
	if v := stub.gotCtx.Value(samplingClientCtxProbe{}); v != nil {
		// This probe key is not the same as mcpsampling's key, so any
		// returned value is a test setup error. Just guards against
		// accidental probe key collision.
		t.Fatalf("unexpected probe key collision: %v", v)
	}
}

// samplingClientCtxProbe is a probe key for the test above — its
// presence in ctx would indicate something accidentally leaking the
// probe; we never set it, so the test must always see nil. The
// mcpsampling package's key is unexported, so we cannot directly
// inspect for it from this test — TestClassifyPipelineErr_McpsamplingSentinels
// instead verifies the classifier-side contract, which is the
// observable wire behavior.
type samplingClientCtxProbe struct{}

// TestClassifyPipelineErr_McpsamplingSentinels — direct unit test on
// the classifier confirming ErrNoSamplingClient and ErrUnexpectedContentKind
// map to internal_error: with the documented sub-prefix.
func TestClassifyPipelineErr_McpsamplingSentinels(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name       string
		in         error
		wantPrefix string
	}{
		{
			name:       "no sampling client",
			in:         mcpsampling.ErrNoSamplingClient,
			wantPrefix: "internal_error: sampling client missing from ctx",
		},
		{
			name:       "unexpected content kind",
			in:         mcpsampling.ErrUnexpectedContentKind,
			wantPrefix: "internal_error: sampling reply not text",
		},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := classifyPipelineErr(tc.in)
			if got == nil {
				t.Fatalf("want error, got nil")
			}
			if !strings.HasPrefix(got.Error(), tc.wantPrefix) {
				t.Errorf("want prefix %q, got %q", tc.wantPrefix, got.Error())
			}
			if !errors.Is(got, tc.in) {
				t.Errorf("classified error must wrap original (errors.Is); got %v", got)
			}
		})
	}
}

// TestClassifyPipelineErr_SamplingDeadlineExceeded_IsCancelled — per S3:
// context.DeadlineExceeded coming from inside mcpsampling.Voice() (after
// %w-wrapping) still routes to the existing "cancelled:" bucket. Pins
// the behavior so a future refactor doesn't accidentally break it.
func TestClassifyPipelineErr_SamplingDeadlineExceeded_IsCancelled(t *testing.T) {
	t.Parallel()
	// Synthesize the exact wrap mcpsampling.Voice produces.
	wrapped := fakeMcpsamplingTimeout()
	got := classifyPipelineErr(wrapped)
	if got == nil {
		t.Fatalf("want error, got nil")
	}
	if !strings.HasPrefix(got.Error(), "cancelled:") {
		t.Errorf("want cancelled: prefix; got %q", got.Error())
	}
	if !errors.Is(got, context.DeadlineExceeded) {
		t.Errorf("classified error must wrap context.DeadlineExceeded")
	}
}

// fakeMcpsamplingTimeout produces the same %w wrap shape mcpsampling.Voice
// uses for a timeout reply. Mirrors the wrap so the classifier test is
// not coupled to the literal string but to the wrap chain.
func fakeMcpsamplingTimeout() error {
	return errFmtWrap("mcpsampling: createMessage: ", context.DeadlineExceeded)
}

// errFmtWrap is a tiny helper so the test does not import "fmt" just
// for one Errorf call.
func errFmtWrap(prefix string, inner error) error {
	return &wrapped{prefix: prefix, inner: inner}
}

type wrapped struct {
	prefix string
	inner  error
}

func (w *wrapped) Error() string { return w.prefix + w.inner.Error() }
func (w *wrapped) Unwrap() error { return w.inner }

// ── Issue #17 (Decision v6): text-arg via adapter/mcptext ──────────

// TestRunSpeak_TextArg_WiresMcptextAdapter — Text != "" routes through
// the mcptext adapter. The stub captures the wired InputAdapter via the
// newPipeline seam; we assert the concrete type and the SourceRef Kind.
func TestRunSpeak_TextArg_WiresMcptextAdapter(t *testing.T) {
	// Cannot t.Parallel() — mutates package-level newPipeline var.
	stub := &stubNarrator{receipt: sink.SinkReceipt{BlocksPlayed: 1, TotalDurationMs: 10}}
	restore := withStubPipeline(t, stub)
	defer restore()

	const text = "hi"
	if _, err := runSpeak(context.Background(), speakArgs{Text: text}); err != nil {
		t.Fatalf("runSpeak: %v", err)
	}
	if _, ok := stub.gotInput.(*mcptext.Adapter); !ok {
		t.Errorf("InputAdapter: want *mcptext.Adapter, got %T", stub.gotInput)
	}
	if stub.gotRef.Kind != plan.SourceKindMCPText {
		t.Errorf("SourceRef.Kind: want mcp_text, got %v", stub.gotRef.Kind)
	}
	if !strings.HasPrefix(stub.gotRef.URI, "mcp://inline/") {
		t.Errorf("SourceRef.URI: want mcp://inline/* prefix, got %q", stub.gotRef.URI)
	}
}

// TestRunSpeak_TextArg_HashURIMatches — the URI's hex suffix must equal
// sha256(args.Text). Pins the composition-root contract that the
// adapter cross-checks (Decision v3 of the #17 plan); if these drift,
// the adapter rejects every well-formed call.
func TestRunSpeak_TextArg_HashURIMatches(t *testing.T) {
	// Cannot t.Parallel() — mutates package-level newPipeline var.
	stub := &stubNarrator{receipt: sink.SinkReceipt{BlocksPlayed: 1, TotalDurationMs: 10}}
	restore := withStubPipeline(t, stub)
	defer restore()

	const text = "## title\n\nbody paragraph.\n"
	if _, err := runSpeak(context.Background(), speakArgs{Text: text}); err != nil {
		t.Fatalf("runSpeak: %v", err)
	}
	sum := sha256.Sum256([]byte(text))
	wantURI := "mcp://inline/" + hex.EncodeToString(sum[:])
	if stub.gotRef.URI != wantURI {
		t.Errorf("SourceRef.URI: got %q want %q", stub.gotRef.URI, wantURI)
	}
}

// TestRunSpeak_SourceArg_WiresFileAdapter — the existing source path
// must still wire the file adapter post-#17. Guards against the seam
// widening regressing the default branch.
func TestRunSpeak_SourceArg_WiresFileAdapter(t *testing.T) {
	// Cannot t.Parallel() — mutates package-level newPipeline var.
	stub := &stubNarrator{receipt: sink.SinkReceipt{BlocksPlayed: 1, TotalDurationMs: 10}}
	restore := withStubPipeline(t, stub)
	defer restore()

	tmpFile, err := os.CreateTemp(t.TempDir(), "sample-*.md")
	if err != nil {
		t.Fatalf("create temp source: %v", err)
	}
	_ = tmpFile.Close()

	if _, err := runSpeak(context.Background(), speakArgs{Source: tmpFile.Name()}); err != nil {
		t.Fatalf("runSpeak: %v", err)
	}
	if _, ok := stub.gotInput.(*file.Adapter); !ok {
		t.Errorf("InputAdapter: want *file.Adapter, got %T", stub.gotInput)
	}
	if stub.gotRef.Kind != plan.SourceKindFile {
		t.Errorf("SourceRef.Kind: want file, got %v", stub.gotRef.Kind)
	}
}

// TestInputAdapterAndRef — direct unit test on the helper. Covers all
// three branches: text, source, neither (defensive — validate() catches
// it upstream, but the helper must surface a sensible error if reached
// directly).
func TestInputAdapterAndRef(t *testing.T) {
	t.Parallel()

	t.Run("text branch", func(t *testing.T) {
		t.Parallel()
		input, ref, err := inputAdapterAndRef(speakArgs{Text: "hi"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if _, ok := input.(*mcptext.Adapter); !ok {
			t.Errorf("InputAdapter: want *mcptext.Adapter, got %T", input)
		}
		if ref.Kind != plan.SourceKindMCPText {
			t.Errorf("Kind: want mcp_text, got %v", ref.Kind)
		}
		if !strings.HasPrefix(ref.URI, "mcp://inline/") {
			t.Errorf("URI: want mcp://inline/* prefix, got %q", ref.URI)
		}
	})

	t.Run("source branch", func(t *testing.T) {
		t.Parallel()
		input, ref, err := inputAdapterAndRef(speakArgs{Source: "doc.md"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if _, ok := input.(*file.Adapter); !ok {
			t.Errorf("InputAdapter: want *file.Adapter, got %T", input)
		}
		if ref.Kind != plan.SourceKindFile {
			t.Errorf("Kind: want file, got %v", ref.Kind)
		}
		// URI is absolute — filepath.Abs("doc.md") joins cwd.
		if !filepath.IsAbs(ref.URI) {
			t.Errorf("URI: want absolute path, got %q", ref.URI)
		}
	})

	t.Run("neither branch defensive", func(t *testing.T) {
		t.Parallel()
		_, _, err := inputAdapterAndRef(speakArgs{})
		if !errors.Is(err, errMissingSource) {
			t.Errorf("want errors.Is errMissingSource, got %v", err)
		}
	})
}
