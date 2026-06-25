package ephemeral

import (
	"context"
	"errors"
	"os"
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

// TestSink_Construction — the two construction axes (zero-value defaults and
// functional-option overrides) resolve the binary + per-block timeout the seam
// receives. Folds the former TestSink_ZeroValueUsable + TestSink_OptionsOverride
// into one table so the construction seam is asserted in a single place.
func TestSink_Construction(t *testing.T) {
	cases := []struct {
		name        string
		newSink     func() *Sink // nil → zero-value Sink{}
		wantBinary  string
		wantTimeout time.Duration
	}{
		{
			name:        "zero value uses defaults",
			newSink:     nil,
			wantBinary:  defaultAfplayPath,
			wantTimeout: DefaultPerBlockTimeout,
		},
		{
			name: "options override defaults",
			newSink: func() *Sink {
				return New(
					WithAfplayPath("/opt/homebrew/bin/afplay"),
					WithPerBlockTimeout(5*time.Second),
				)
			},
			wantBinary:  "/opt/homebrew/bin/afplay",
			wantTimeout: 5 * time.Second,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			withStubPlay(t, func(_ context.Context, binary, _ string, timeout time.Duration) error {
				if binary != tc.wantBinary {
					t.Errorf("binary: got %q, want %q", binary, tc.wantBinary)
				}
				if timeout != tc.wantTimeout {
					t.Errorf("timeout: got %v, want %v", timeout, tc.wantTimeout)
				}
				return nil
			})

			var s *Sink
			if tc.newSink != nil {
				s = tc.newSink()
			} else {
				s = &Sink{} // zero value
			}

			res := render.RenderResult{
				Audio: render.AudioStream{Dir: "/tmp/audio"},
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
				t.Errorf("receipt: got %+v, want {TotalDurationMs:500, BlocksPlayed:1}", rec)
			}
		})
	}
}

// TestSink_Consume_EmptyAudioDir — a non-empty AudioRef with no Audio.Dir is a
// mis-wired pipeline; Consume must fail loud before invoking the play seam.
func TestSink_Consume_EmptyAudioDir(t *testing.T) {
	withStubPlay(t, func(context.Context, string, string, time.Duration) error {
		t.Fatal("play should not be called when Audio.Dir is empty")
		return nil
	})

	res := render.RenderResult{
		Audio: render.AudioStream{Dir: ""}, // missing dir
		Timeline: plan.Timeline{
			Blocks: []plan.BlockTiming{
				{BlockID: "b001", StartMs: 0, EndMs: 500, AudioRef: "a.wav"},
			},
		},
	}
	_, err := New().Consume(context.Background(), plan.NarrationPlan{}, res)
	if err == nil {
		t.Fatal("want error for empty Audio.Dir with non-empty AudioRef, got nil")
	}
	if !contains(err.Error(), "empty Audio.Dir") || !contains(err.Error(), "b001") {
		t.Errorf("err should name the cause and block id, got %q", err.Error())
	}
}

// TestPlayWithAfplay_CtxCancel_Joins — direct test of playWithAfplay's real
// cancel branch (not the stub seam). Uses /bin/sleep as a controllable slow
// "player": canceling the parent ctx mid-run must return an error that both
// reports the cancellation (errors.Is context.Canceled) AND carries the killed
// process's own exit error (the errors.Join from AC#5). Skipped where
// /bin/sleep is absent so non-unix dev boxes do not false-fail.
func TestPlayWithAfplay_CtxCancel_Joins(t *testing.T) {
	const sleepBin = "/bin/sleep"
	if _, err := os.Stat(sleepBin); err != nil {
		t.Skipf("%s not available (%v); skipping real-cancel test", sleepBin, err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	// Cancel shortly after the child starts so we exercise the
	// callCtx.Done() branch rather than a clean exit.
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	// binary=/bin/sleep, path="60" → execs `/bin/sleep 60`, blocking well past
	// the cancel. Generous per-block timeout so the timeout path doesn't fire
	// first — we want the parent-cancel path.
	err := playWithAfplay(ctx, sleepBin, "60", 30*time.Second)
	if err == nil {
		t.Fatal("want error from cancelled sleep, got nil")
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("want errors.Is(err, context.Canceled), got %v", err)
	}
	// The join must surface the process's own death too (e.g. "signal: killed").
	// errors.Join with a non-nil second arg wraps multiple errors; assert the
	// joined error is more than the bare context error.
	joined, ok := err.(interface{ Unwrap() []error })
	if !ok {
		t.Fatalf("want a joined (multi) error carrying the wait error, got single error %v", err)
	}
	errs := joined.Unwrap()
	if len(errs) < 2 {
		t.Fatalf("want joined error to carry both ctx and wait errors, got %d: %v", len(errs), err)
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

// TestSink_Consume_BlockObserver — the issue #81 Channel-2 emit seam.
// Asserts: one emit per timeline block in plan order; the Playing truth
// table (AudioRef != ""); Level/Status enriched from the plan param, keyed
// 1:1 by BlockID; refused + pause blocks emit Playing:false carrying their
// real Status; and emit-count == block-count (no double-emit on the empty-
// AudioRef branch).
func TestSink_Consume_BlockObserver(t *testing.T) {
	withStubPlay(t, func(context.Context, string, string, time.Duration) error { return nil })

	// Four blocks spanning the truth table. Playing is derived purely from
	// AudioRef, so it mirrors what the renderer emits — NOT Status:
	//   b1 voiced+audio   → Playing:true
	//   b2 degraded+audio → Playing:true
	//   b3 refused+audio  → Playing:true (the renderer voiced Refusal.Message
	//                       into a WAV; Status:refused is the honesty signal,
	//                       not Playing — see render/sherpa spokenTextFor)
	//   b4 voiced+no-audio (all-pause/empty) → Playing:false
	res := render.RenderResult{
		Audio: render.AudioStream{Dir: "/tmp/audio"},
		Timeline: plan.Timeline{
			Blocks: []plan.BlockTiming{
				{BlockID: "b1", StartMs: 0, EndMs: 1000, AudioRef: "b1.wav"},
				{BlockID: "b2", StartMs: 1000, EndMs: 2200, AudioRef: "b2.wav"},
				{BlockID: "b3", StartMs: 2200, EndMs: 3300, AudioRef: "b3.wav"},
				{BlockID: "b4", StartMs: 3300, EndMs: 3300, AudioRef: ""},
			},
		},
		Format: render.DefaultFormat(),
	}
	np := plan.NarrationPlan{
		Blocks: []plan.Block{
			{ID: "b1", Level: plan.L2, Status: plan.StatusVoiced},
			{ID: "b2", Level: plan.L3, Status: plan.StatusDegraded},
			{ID: "b3", Level: plan.L1, Status: plan.StatusRefused},
			{ID: "b4", Level: plan.L1, Status: plan.StatusVoiced},
		},
	}

	var got []BlockProgress
	s := New(WithBlockObserver(func(p BlockProgress) { got = append(got, p) }))
	if _, err := s.Consume(context.Background(), np, res); err != nil {
		t.Fatalf("Consume: %v", err)
	}

	want := []BlockProgress{
		{Timing: res.Timeline.Blocks[0], Level: plan.L2, Status: plan.StatusVoiced, Order: 1, Total: 4, Playing: true},
		{Timing: res.Timeline.Blocks[1], Level: plan.L3, Status: plan.StatusDegraded, Order: 2, Total: 4, Playing: true},
		{Timing: res.Timeline.Blocks[2], Level: plan.L1, Status: plan.StatusRefused, Order: 3, Total: 4, Playing: true},
		{Timing: res.Timeline.Blocks[3], Level: plan.L1, Status: plan.StatusVoiced, Order: 4, Total: 4, Playing: false},
	}
	if len(got) != len(want) {
		t.Fatalf("emit count: got %d, want %d (one per block, no double-emit)", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("emit[%d]: got %+v, want %+v", i, got[i], want[i])
		}
	}
}

// TestSink_Consume_NoObserver_IsNoOp — without WithBlockObserver the sink is
// the unmodified player: the off-path must not panic and must produce the
// same receipt it always did (the byte-identity counter-metric is asserted
// end-to-end in cmd/narrate-mcp; this is the sink-local guard).
func TestSink_Consume_NoObserver_IsNoOp(t *testing.T) {
	withStubPlay(t, func(context.Context, string, string, time.Duration) error { return nil })

	s := New() // no observer Option
	rec, err := s.Consume(context.Background(), plan.NarrationPlan{}, threeBlockResult())
	if err != nil {
		t.Fatalf("Consume: %v", err)
	}
	if rec.BlocksPlayed != 2 || rec.TotalDurationMs != 2500 {
		t.Errorf("off-path receipt drifted: got %+v, want {BlocksPlayed:2, TotalDurationMs:2500}", rec)
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

