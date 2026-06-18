package ephemeral

import (
	"context"
	"errors"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/vd09-projects/intelligent-tts-narration-library/plan"
	"github.com/vd09-projects/intelligent-tts-narration-library/render"
	"github.com/vd09-projects/intelligent-tts-narration-library/sink"
)

// Compile-time conformance: *Sink satisfies sink.OutputSink.
var _ sink.OutputSink = (*Sink)(nil)

// trivialRenderResult — three-block timeline matching the happy-path case:
// two non-empty AudioRefs sandwiching one empty (pause) block. AudioRef
// values are relative WAV names matching what render/sherpa emits.
func threeBlockResult() render.RenderResult {
	return render.RenderResult{
		Audio: render.AudioStream{
			Dir:   "/tmp/audio",
			Files: []string{"b001.wav", "b003.wav"},
		},
		Timeline: plan.Timeline{
			PlanID: "01HTEST00000000000000000000",
			Format: render.DefaultFormat(),
			Blocks: []plan.BlockTiming{
				{BlockID: "b001", StartMs: 0, EndMs: 1000, AudioRef: "b001.wav"},
				{BlockID: "b002", StartMs: 1000, EndMs: 1000, AudioRef: ""}, // pause
				{BlockID: "b003", StartMs: 1000, EndMs: 2500, AudioRef: "b003.wav"},
			},
		},
		Format: render.DefaultFormat(),
	}
}

// emptyOnlyResult — single block with empty AudioRef.
func emptyOnlyResult() render.RenderResult {
	return render.RenderResult{
		Audio: render.AudioStream{Dir: "/tmp/audio"},
		Timeline: plan.Timeline{
			PlanID: "01HTEST00000000000000000001",
			Format: render.DefaultFormat(),
			Blocks: []plan.BlockTiming{
				{BlockID: "b001", StartMs: 0, EndMs: 0, AudioRef: ""},
			},
		},
		Format: render.DefaultFormat(),
	}
}

// stubPlayer captures the per-block invocations a Consume run produces
// and lets each test decide what each call returns. recorded order is
// preserved so tests can assert plan-order traversal.
type stubPlayer struct {
	mu       sync.Mutex
	calls    []string                                                              // resolved paths, in invocation order
	respond  func(ctx context.Context, binary, path string, timeout time.Duration) error
}

func (sp *stubPlayer) fn() playFunc {
	return func(ctx context.Context, binary, path string, timeout time.Duration) error {
		sp.mu.Lock()
		sp.calls = append(sp.calls, path)
		sp.mu.Unlock()
		return sp.respond(ctx, binary, path, timeout)
	}
}

func (sp *stubPlayer) callPaths() []string {
	sp.mu.Lock()
	defer sp.mu.Unlock()
	out := make([]string, len(sp.calls))
	copy(out, sp.calls)
	return out
}

// withStubPlay swaps the package-level seam for the duration of a test
// and restores it afterwards. Tests must not run in parallel because the
// seam is package-level (documented constraint).
func withStubPlay(t *testing.T, p playFunc) {
	t.Helper()
	orig := play
	play = p
	t.Cleanup(func() { play = orig })
}

func TestSink_Consume_TableDriven(t *testing.T) {
	// errBoom is the sentinel returned by case (c).
	errBoom := errors.New("boom")

	cases := []struct {
		name        string
		res         render.RenderResult
		respond     func(ctx context.Context, binary, path string, timeout time.Duration) error
		ctx         func() (context.Context, context.CancelFunc)
		wantPlayed  int
		wantDurMs   int64
		wantCalls   []string
		wantErrIs   error // errors.Is target; nil means expect no error
		wantErrText string // substring to find in err.Error() (when wantErrIs is nil but err expected)
	}{
		{
			name:       "happy 3-block (2 non-empty + 1 empty)",
			res:        threeBlockResult(),
			respond:    func(_ context.Context, _, _ string, _ time.Duration) error { return nil },
			ctx:        func() (context.Context, context.CancelFunc) { return context.Background(), func() {} },
			wantPlayed: 2,
			// Block durations: 1000 + 0 + 1500 = 2500.
			wantDurMs: 2500,
			wantCalls: []string{
				filepath.Join("/tmp/audio", "b001.wav"),
				filepath.Join("/tmp/audio", "b003.wav"),
			},
			wantErrIs: nil,
		},
		{
			name:       "skip empty AudioRef only",
			res:        emptyOnlyResult(),
			respond:    func(_ context.Context, _, _ string, _ time.Duration) error { t.Fatal("play should not be called for empty AudioRef"); return nil },
			ctx:        func() (context.Context, context.CancelFunc) { return context.Background(), func() {} },
			wantPlayed: 0,
			wantDurMs:  0,
			wantCalls:  nil,
			wantErrIs:  nil,
		},
		{
			name: "subprocess error mid-stream",
			res:  threeBlockResult(),
			respond: func() func(context.Context, string, string, time.Duration) error {
				var n int
				return func(_ context.Context, _, _ string, _ time.Duration) error {
					n++
					if n == 1 {
						return nil
					}
					return errBoom
				}
			}(),
			ctx:         func() (context.Context, context.CancelFunc) { return context.Background(), func() {} },
			wantPlayed:  1,    // only the first non-empty block succeeded
			wantDurMs:   1000, // block 1 succeeded, empty pause (+0) NOT yet reached
			wantCalls:   []string{filepath.Join("/tmp/audio", "b001.wav"), filepath.Join("/tmp/audio", "b003.wav")},
			wantErrIs:   errBoom,
			wantErrText: "sink/ephemeral: afplay block b003:",
		},
		{
			name: "ctx cancel mid-stream",
			res:  threeBlockResult(),
			respond: func() func(context.Context, string, string, time.Duration) error {
				// Cancel parent ctx on the first call; the second call's
				// pre-check should observe ctx.Err() and return it.
				return func(ctx context.Context, _, _ string, _ time.Duration) error {
					if c, ok := ctx.Value(cancelKey{}).(context.CancelFunc); ok && c != nil {
						c()
					}
					return nil // first call "succeeds" before cancel takes effect
				}
			}(),
			ctx: func() (context.Context, context.CancelFunc) {
				ctx, cancel := context.WithCancel(context.Background())
				ctx = context.WithValue(ctx, cancelKey{}, cancel)
				return ctx, cancel
			},
			wantPlayed: 1,
			wantDurMs:  1000,
			wantCalls:  []string{filepath.Join("/tmp/audio", "b001.wav")},
			wantErrIs:  context.Canceled,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			sp := &stubPlayer{respond: tc.respond}
			withStubPlay(t, sp.fn())

			ctx, cancel := tc.ctx()
			defer cancel()

			s := New(WithAfplayPath("/usr/bin/afplay"))
			rec, err := s.Consume(ctx, plan.NarrationPlan{}, tc.res)

			// Error assertions.
			switch {
			case tc.wantErrIs != nil:
				if err == nil {
					t.Fatalf("want err matching %v, got nil", tc.wantErrIs)
				}
				if !errors.Is(err, tc.wantErrIs) {
					t.Fatalf("want errors.Is %v, got %v", tc.wantErrIs, err)
				}
				if tc.wantErrText != "" && !contains(err.Error(), tc.wantErrText) {
					t.Fatalf("want err text to contain %q, got %q", tc.wantErrText, err.Error())
				}
			default:
				if err != nil {
					t.Fatalf("unexpected err: %v", err)
				}
			}

			// Receipt assertions.
			if rec.BlocksPlayed != tc.wantPlayed {
				t.Errorf("BlocksPlayed: got %d, want %d", rec.BlocksPlayed, tc.wantPlayed)
			}
			if rec.TotalDurationMs != tc.wantDurMs {
				t.Errorf("TotalDurationMs: got %d, want %d", rec.TotalDurationMs, tc.wantDurMs)
			}

			// Call-trace assertion (plan-order traversal).
			got := sp.callPaths()
			if !equalStrings(got, tc.wantCalls) {
				t.Errorf("calls: got %v, want %v", got, tc.wantCalls)
			}
		})
	}
}

// TestSink_ZeroValueUsable — Sink{} must work with defaults.
func TestSink_ZeroValueUsable(t *testing.T) {
	withStubPlay(t, func(_ context.Context, binary, _ string, timeout time.Duration) error {
		if binary != defaultAfplayPath {
			t.Errorf("binary: got %q, want default %q", binary, defaultAfplayPath)
		}
		if timeout != defaultPerBlockTimeout {
			t.Errorf("timeout: got %v, want default %v", timeout, defaultPerBlockTimeout)
		}
		return nil
	})

	var s Sink // zero value
	res := render.RenderResult{
		Timeline: plan.Timeline{
			Blocks: []plan.BlockTiming{
				{BlockID: "b001", StartMs: 0, EndMs: 500, AudioRef: "a.wav"},
			},
		},
	}
	rec, err := s.Consume(context.Background(), plan.NarrationPlan{}, res)
	if err != nil {
		t.Fatalf("Consume: %v", err)
	}
	if rec.BlocksPlayed != 1 || rec.TotalDurationMs != 500 {
		t.Errorf("receipt: got %+v, want {500, 1}", rec)
	}
}

// TestSink_OptionsOverride — functional options propagate.
func TestSink_OptionsOverride(t *testing.T) {
	wantBin := "/opt/homebrew/bin/afplay"
	wantTimeout := 5 * time.Second

	withStubPlay(t, func(_ context.Context, binary, _ string, timeout time.Duration) error {
		if binary != wantBin {
			t.Errorf("binary: got %q, want %q", binary, wantBin)
		}
		if timeout != wantTimeout {
			t.Errorf("timeout: got %v, want %v", timeout, wantTimeout)
		}
		return nil
	})

	s := New(
		WithAfplayPath(wantBin),
		WithPerBlockTimeout(wantTimeout),
	)
	res := render.RenderResult{
		Timeline: plan.Timeline{
			Blocks: []plan.BlockTiming{
				{BlockID: "b001", StartMs: 0, EndMs: 100, AudioRef: "x.wav"},
			},
		},
	}
	if _, err := s.Consume(context.Background(), plan.NarrationPlan{}, res); err != nil {
		t.Fatalf("Consume: %v", err)
	}
}

// TestPlayWithAfplay_MissingBinary — the real seam (not stubbed) should
// return a wrapped "start afplay" error when the binary doesn't exist.
// Runs the production playWithAfplay directly without going through the
// package-level seam, so it works regardless of swap state.
func TestPlayWithAfplay_MissingBinary(t *testing.T) {
	err := playWithAfplay(context.Background(), "/nonexistent/definitely-not-afplay", "/tmp/whatever.wav", time.Second)
	if err == nil {
		t.Fatal("want error from missing binary, got nil")
	}
	if !contains(err.Error(), "start afplay") {
		t.Errorf("want error to mention 'start afplay', got %q", err.Error())
	}
}

// cancelKey is the context key the ctx-cancel table case uses to fish
// the cancel func out and trigger cancellation mid-call.
type cancelKey struct{}

// contains is a tiny strings.Contains shim kept local so the test file's
// imports stay minimal.
func contains(haystack, needle string) bool {
	return len(needle) == 0 || (len(haystack) >= len(needle) && indexOf(haystack, needle) >= 0)
}

func indexOf(haystack, needle string) int {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return i
		}
	}
	return -1
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

