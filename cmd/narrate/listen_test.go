//go:build !oto

// Tests for the listen-path keypress transport controller (issue #83).
//
// These exercise the pure navigation/parse helpers plus the load-bearing
// concurrency discipline: childHandle.reap idempotency, playBlock's
// cancel-path (done-chan + WaitDelay backstop), and the runListen keypress
// loop driven through stubbed seams (no real tty, no real afplay). Run under
// `go test -race ./cmd/narrate/` (make test) — the single-owner model means a
// race here is a correctness bug, not a flake.
//
// Build-tagged !oto: childHandle, playBlock, and runListen are the afplay seam,
// which only exists in the default (!oto) build. The //go:build oto spike seam
// (listen_oto.go, issue #100) is verified by ear via `make spike-oto-listen`,
// not by these tests. The build-tag-free pcmReader has its own tag-free test
// (pcm_wav_test.go) that runs under both configs.
package main

import (
	"context"
	"errors"
	"io"
	"os/exec"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/vd09-projects/intelligent-tts-narration-library/plan"
)

func tl(blocks ...plan.BlockTiming) plan.Timeline {
	return plan.Timeline{Blocks: blocks}
}

func bt(id string, start, end int, ref string) plan.BlockTiming {
	return plan.BlockTiming{BlockID: id, StartMs: start, EndMs: end, AudioRef: ref}
}

func TestNavigableBlocks(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		timeline plan.Timeline
		want     []int
	}{
		{"empty", tl(), nil},
		{
			"all navigable",
			tl(bt("a", 0, 100, "a.wav"), bt("b", 100, 200, "b.wav")),
			[]int{0, 1},
		},
		{
			"skip empty audioref",
			tl(bt("a", 0, 100, "a.wav"), bt("b", 100, 100, ""), bt("c", 100, 200, "c.wav")),
			[]int{0, 2},
		},
		{
			"skip zero duration",
			tl(bt("a", 0, 0, "a.wav"), bt("b", 0, 200, "b.wav")),
			[]int{1},
		},
		{
			"skip negative duration",
			tl(bt("a", 200, 100, "a.wav"), bt("b", 0, 200, "b.wav")),
			[]int{1},
		},
		{
			"none navigable",
			tl(bt("a", 0, 0, "a.wav"), bt("b", 100, 100, "")),
			nil,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := navigableBlocks(tc.timeline)
			if len(got) != len(tc.want) {
				t.Fatalf("navigableBlocks() = %v, want %v", got, tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Fatalf("navigableBlocks() = %v, want %v", got, tc.want)
				}
			}
		})
	}
}

func TestNearestNavigable(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		nav    []int
		target int
		want   int // index into nav
	}{
		{"exact hit", []int{0, 3, 7}, 3, 1},
		{"below first snaps to first", []int{2, 5, 9}, 0, 0},
		{"above last snaps to last", []int{2, 5, 9}, 20, 2},
		{"between snaps nearer", []int{0, 10}, 3, 0},
		{"tie breaks toward later", []int{0, 10}, 5, 1},
		{"single", []int{4}, 99, 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := nearestNavigable(tc.nav, tc.target); got != tc.want {
				t.Fatalf("nearestNavigable(%v, %d) = %d, want %d", tc.nav, tc.target, got, tc.want)
			}
		})
	}
}

func TestParseBlockIndex(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		line    string
		n       int
		want    int
		wantErr bool
	}{
		{"plain", "3", 10, 3, false},
		{"trailing CR", "3\r", 10, 3, false},
		{"surrounding space", "  2  \r\n", 10, 2, false},
		{"zero", "0", 10, 0, false},
		{"empty", "", 10, 0, true},
		{"whitespace only", "   \r\n", 10, 0, true},
		{"non numeric", "abc", 10, 0, true},
		{"mixed", "1a", 10, 0, true},
		{"out of range", "10", 10, 0, true},
		{"way out of range", "999", 5, 0, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseBlockIndex(tc.line, tc.n)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("parseBlockIndex(%q, %d) = %d, want error", tc.line, tc.n, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseBlockIndex(%q, %d) unexpected error: %v", tc.line, tc.n, err)
			}
			if got != tc.want {
				t.Fatalf("parseBlockIndex(%q, %d) = %d, want %d", tc.line, tc.n, got, tc.want)
			}
		})
	}
}

// TestChildHandleReap_Idempotent reaps a real, still-running child twice and
// asserts the second reap is a no-op and the process is gone — the
// intra-goroutine double-Wait guard (reaped flag).
func TestChildHandleReap_Idempotent(t *testing.T) {
	t.Parallel()
	cmd := exec.Command("sleep", "30")
	if err := cmd.Start(); err != nil {
		t.Skipf("cannot start sleep (env without /bin/sleep): %v", err)
	}
	h := &childHandle{cmd: cmd}

	h.reap()
	if !h.reaped {
		t.Fatal("reap() did not set reaped flag")
	}
	// Second reap must be a no-op (no panic, no double-Wait error surfacing).
	h.reap()

	// nil handle / nil cmd / already-reaped handle are all safe no-ops.
	var nilH *childHandle
	nilH.reap()
	(&childHandle{}).reap()
}

// TestPlayBlock_CancelReaps drives the production playBlock against a real
// long-running process and cancels the context. The done-chan + WaitDelay
// backstop must return the ctx cause and leave the child reaped — exercised
// under -race so the wait goroutine is checked for data races.
func TestPlayBlock_CancelReaps(t *testing.T) {
	t.Parallel()
	if _, err := exec.LookPath("sleep"); err != nil {
		t.Skipf("no sleep binary: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	var cmd *exec.Cmd
	var err error
	go func() {
		cmd, err = playBlock(ctx, "sleep", "30")
		close(done)
	}()
	// Give the child a moment to start, then cancel.
	time.Sleep(50 * time.Millisecond)
	cancel()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("playBlock did not return within 5s of cancel (WaitDelay backstop failed)")
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("playBlock err = %v, want context.Canceled", err)
	}
	if cmd == nil {
		t.Fatal("playBlock returned nil cmd on cancel; caller cannot reap")
	}
}

// scriptedBytes returns a readByte seam that yields the given bytes in order,
// then returns io.EOF. Used to drive the keypress loop deterministically.
func scriptedBytes(keys string) func() (byte, error) {
	var i int32 = -1
	b := []byte(keys)
	return func() (byte, error) {
		idx := atomic.AddInt32(&i, 1)
		if int(idx) >= len(b) {
			return 0, io.EOF
		}
		return b[int(idx)], nil
	}
}

// recordingCleanup tracks restore + removeAll invocations and their order so a
// test can assert the load-bearing teardown sequence ran.
type recordingCleanup struct {
	mu       sync.Mutex
	order    []string
	playN    int32
	playArgs []string
}

func (r *recordingCleanup) restore() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.order = append(r.order, "restore")
}

func (r *recordingCleanup) removeAll(string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.order = append(r.order, "removeAll")
	return nil
}

// stubPlay honors the playBlock contract: clean self-exit, already reaped by
// the time it returns (nil cmd, nil err) so the loop's spawn marks the child
// reaped and the next reap-before-spawn is a no-op. Counts invocations.
func (r *recordingCleanup) stubPlay(_ context.Context, _, wavPath string) (*exec.Cmd, error) {
	atomic.AddInt32(&r.playN, 1)
	r.mu.Lock()
	r.playArgs = append(r.playArgs, wavPath)
	r.mu.Unlock()
	return nil, nil
}

func navTimeline() plan.Timeline {
	return tl(
		bt("a", 0, 100, "a.wav"),
		bt("b", 100, 200, "b.wav"),
		bt("c", 200, 300, "c.wav"),
	)
}

func TestRunListen_KeypressSequence(t *testing.T) {
	t.Parallel()
	rec := &recordingCleanup{}
	cfg := listenConfig{
		binary:    "afplay",
		audioDir:  "/tmp/audio",
		play:      rec.stubPlay,
		restore:   rec.restore,
		removeAll: rec.removeAll,
		tempDir:   "/tmp/narrate-xyz",
		readByte:  scriptedBytes("nnb q"), // next, next, back, replay(space), quit
		readLine:  func() (string, error) { return "", nil },
		shutdown:  make(chan shutdownRequest),
		out:       io.Discard,
	}
	if err := runListen(context.Background(), cfg, navTimeline()); err != nil {
		t.Fatalf("runListen returned error: %v", err)
	}
	// Plays: initial spawn(0) + n + n + b + space = 5. 'q' does not spawn.
	if got := atomic.LoadInt32(&rec.playN); got != 5 {
		t.Fatalf("play invoked %d times, want 5", got)
	}
	assertCleanupRan(t, rec)
}

// TestRunListen_NavigationBoundaries pins the two runListen navigation guards
// that make a keypress a silent no-op at a boundary: 'n' at the last navigable
// block (listen.go: if pos < len(nav)-1) and 'b' at position 0 (if pos > 0).
//
// pos is a runListen local, not observable from a test. The no-op is asserted
// indirectly: spawn count (rec.playN) is 1 (the opening spawn(0)) plus one per
// navigation that actually moved, and the last wav played (rec.playArgs)
// pins which block pos landed on. A boundary no-op adds zero spawns and leaves
// the last wav unchanged.
//
// Failure-mode note: deleting either guard does NOT cleanly bump the count by
// one. 'n' at the last block would do pos++ then spawn(pos) with pos == len(nav),
// indexing nav[len(nav)] — out of range; 'b' at 0 would index nav[-1]. Both
// panic. Detection still holds (a panic fails the test loudly); the
// "count bumps by one" framing is just imprecise.
func TestRunListen_NavigationBoundaries(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name        string
		script      string
		wantPlayN   int32
		wantLastWav string
	}{
		{"n at last navigable is no-op", "nnn", 3, "/tmp/audio/c.wav"},
		{"b at position zero is no-op", "b", 1, "/tmp/audio/a.wav"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			rec := &recordingCleanup{}
			cfg := listenConfig{
				binary:    "afplay",
				audioDir:  "/tmp/audio",
				play:      rec.stubPlay,
				restore:   rec.restore,
				removeAll: rec.removeAll,
				tempDir:   "/tmp/narrate-xyz",
				readByte:  scriptedBytes(tc.script),
				readLine:  func() (string, error) { return "", nil },
				shutdown:  make(chan shutdownRequest),
				out:       io.Discard,
			}
			if err := runListen(context.Background(), cfg, navTimeline()); err != nil {
				t.Fatalf("runListen returned error: %v", err)
			}
			if got := atomic.LoadInt32(&rec.playN); got != tc.wantPlayN {
				t.Fatalf("play invoked %d times, want %d", got, tc.wantPlayN)
			}
			rec.mu.Lock()
			last := rec.playArgs[len(rec.playArgs)-1]
			rec.mu.Unlock()
			if last != tc.wantLastWav {
				t.Fatalf("last wav played = %q, want %q", last, tc.wantLastWav)
			}
			assertCleanupRan(t, rec)
		})
	}
}

func TestRunListen_GoTo(t *testing.T) {
	t.Parallel()
	rec := &recordingCleanup{}
	cfg := listenConfig{
		binary:    "afplay",
		audioDir:  "/tmp/audio",
		play:      rec.stubPlay,
		restore:   rec.restore,
		removeAll: rec.removeAll,
		tempDir:   "/tmp/narrate-xyz",
		readByte:  scriptedBytes("gq"),
		readLine:  func() (string, error) { return "2\r", nil },
		shutdown:  make(chan shutdownRequest),
		out:       io.Discard,
	}
	if err := runListen(context.Background(), cfg, navTimeline()); err != nil {
		t.Fatalf("runListen returned error: %v", err)
	}
	// initial spawn(0) + g jump to nearest of block 2 = 2 plays.
	if got := atomic.LoadInt32(&rec.playN); got != 2 {
		t.Fatalf("play invoked %d times, want 2", got)
	}
	rec.mu.Lock()
	last := rec.playArgs[len(rec.playArgs)-1]
	rec.mu.Unlock()
	if last != "/tmp/audio/c.wav" {
		t.Fatalf("g jumped to %q, want /tmp/audio/c.wav", last)
	}
}

// TestRunListen_InterruptibleRead proves the idle read is interruptible
// (issue #88): with readByte GENUINELY blocked (never returns), an interrupt —
// a shutdownRequest or a ctx-cancel — delivered AFTER the loop is parked on the
// read must still run the full cleanup and return nil WITHOUT any keypress.
//
// This supersedes the former TestRunListen_ShutdownDuringRead, which pre-loaded
// the shutdown request AND returned io.EOF immediately, so it never exercised a
// genuinely-blocked read and would pass even against the pre-#88 blocking-read
// loop. Here readByte blocks on a never-fed channel; the test fires the
// interrupt only after observing the read is reached, so a non-interruptible
// loop would hang and fail on the timeout.
func TestRunListen_InterruptibleRead(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		trigger func(shutdown chan shutdownRequest, cancel context.CancelFunc)
	}{
		{
			"shutdown while blocked on read",
			func(shutdown chan shutdownRequest, _ context.CancelFunc) { shutdown <- shutdownRequest{} },
		},
		{
			"ctx cancel while blocked on read",
			func(_ chan shutdownRequest, cancel context.CancelFunc) { cancel() },
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			rec := &recordingCleanup{}
			shutdown := make(chan shutdownRequest, 1)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			// readByte parks forever on `block` after signalling `reading`, so
			// the loop is genuinely idle on the read — no byte will ever arrive.
			reading := make(chan struct{})
			block := make(chan struct{})
			var once sync.Once
			cfg := listenConfig{
				binary:    "afplay",
				audioDir:  "/tmp/audio",
				play:      rec.stubPlay,
				restore:   rec.restore,
				removeAll: rec.removeAll,
				tempDir:   "/tmp/narrate-xyz",
				readByte: func() (byte, error) {
					once.Do(func() { close(reading) })
					<-block // never fed — the read is genuinely blocked
					return 0, io.EOF
				},
				readLine: func() (string, error) { return "", nil },
				shutdown: shutdown,
				out:      io.Discard,
			}

			done := make(chan error, 1)
			go func() { done <- runListen(ctx, cfg, navTimeline()) }()

			// Fire the interrupt only after the read is reached — this is what
			// makes the test genuine rather than racing the opening spawn.
			select {
			case <-reading:
			case <-time.After(2 * time.Second):
				t.Fatal("readByte was never reached")
			}
			tc.trigger(shutdown, cancel)

			select {
			case err := <-done:
				if err != nil {
					t.Fatalf("runListen returned error on idle interrupt: %v", err)
				}
			case <-time.After(2 * time.Second):
				t.Fatal("runListen did not return after idle interrupt — read not interruptible")
			}
			close(block) // release the detached reader goroutine
			assertCleanupRan(t, rec)
		})
	}
}

func TestRunListen_ContextCancel(t *testing.T) {
	t.Parallel()
	rec := &recordingCleanup{}
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancelled before loop starts
	cfg := listenConfig{
		binary:    "afplay",
		audioDir:  "/tmp/audio",
		play:      rec.stubPlay,
		restore:   rec.restore,
		removeAll: rec.removeAll,
		tempDir:   "/tmp/narrate-xyz",
		readByte:  func() (byte, error) { return 'n', nil },
		readLine:  func() (string, error) { return "", nil },
		shutdown:  make(chan shutdownRequest),
		out:       io.Discard,
	}
	if err := runListen(ctx, cfg, navTimeline()); err != nil {
		t.Fatalf("runListen returned error on ctx cancel: %v", err)
	}
	assertCleanupRan(t, rec)
}

func TestRunListen_NoNavigable(t *testing.T) {
	t.Parallel()
	rec := &recordingCleanup{}
	cfg := listenConfig{
		binary:    "afplay",
		play:      rec.stubPlay,
		restore:   rec.restore,
		removeAll: rec.removeAll,
		tempDir:   "/tmp/narrate-xyz",
		readByte:  func() (byte, error) { return 0, io.EOF },
		readLine:  func() (string, error) { return "", nil },
		shutdown:  make(chan shutdownRequest),
		out:       io.Discard,
	}
	// All zero-duration → no navigable blocks → guarded error + cleanup.
	empty := tl(bt("a", 0, 0, "a.wav"))
	if err := runListen(context.Background(), cfg, empty); err == nil {
		t.Fatal("runListen with no navigable blocks should return an error")
	}
	assertCleanupRan(t, rec)
}

func assertCleanupRan(t *testing.T, rec *recordingCleanup) {
	t.Helper()
	rec.mu.Lock()
	defer rec.mu.Unlock()
	var restoreAt, removeAt = -1, -1
	for i, step := range rec.order {
		switch step {
		case "restore":
			if restoreAt == -1 {
				restoreAt = i
			}
		case "removeAll":
			if removeAt == -1 {
				removeAt = i
			}
		}
	}
	if restoreAt == -1 {
		t.Fatal("cleanup never restored the tty")
	}
	if removeAt == -1 {
		t.Fatal("cleanup never removed the temp dir")
	}
	if restoreAt > removeAt {
		t.Fatalf("cleanup order wrong: restore@%d after removeAll@%d (want restore → removeAll)", restoreAt, removeAt)
	}
}
