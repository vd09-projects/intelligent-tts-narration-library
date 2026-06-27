// Tests for the listen-path keypress transport controller (issue #83,
// productionized for the in-process oto v3 player in #101).
//
// Build-tag-free and DEVICE-FREE: the transport drives the listenPlayer seam, so
// every test injects a fake player via the listenConfig.newPlayer factory — no
// audio device, no oto context, no tty. The by-ear pause/resume confirmation is
// deferred to /verify (no golden audio). Run under `go test -race ./cmd/narrate`
// (make test) — the single-owner model means a race here is a correctness bug.
//
// What is locked here:
//   - navigation / parse helpers (pure)
//   - nil-factory guard and no-navigable guard
//   - keypress navigation (n/b/g) and boundary no-ops
//   - transition halt+release regression lock: every transition Pause()'s the
//     prior fake player BEFORE constructing the next (bounded retention), and NO
//     Close() exists on the seam (compile-time — the interface has no Close)
//   - true Pause/Resume toggle and paused-cleared-on-transition (no bleed)
//   - end-of-block edge detection (no auto-advance, Err() surfaced) and
//     space-on-ended is a no-op
//   - engine-failure + ready-timeout cleanup at the driveListen boundary
//   - load-error constructs no player and the loop continues
//   - idle interrupt (issue #88) and ctx-cancel teardown
package main

import (
	"context"
	"errors"
	"io"
	"strings"
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

func navTimeline() plan.Timeline {
	return tl(
		bt("a", 0, 100, "a.wav"),
		bt("b", 100, 200, "b.wav"),
		bt("c", 200, 300, "c.wav"),
	)
}

// --- fake player + harness (device-free seam) --------------------------------

// fakePlayer implements listenPlayer with NO audio device. It records the
// ordered Play()/Pause() events so a test can assert the transport's
// halt+release discipline and the Pause/Resume toggle. There is deliberately no
// Close() — the listenPlayer seam has none (oto v3.4 Close() is a no-op).
type fakePlayer struct {
	mu          sync.Mutex
	events      []string // "play" / "pause" in call order
	playing     bool
	err         error
	isPlayingFn func() bool // optional override (e.g. scripted end-of-block)
}

func (f *fakePlayer) Play() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.events = append(f.events, "play")
	f.playing = true
}

func (f *fakePlayer) Pause() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.events = append(f.events, "pause")
	f.playing = false
}

func (f *fakePlayer) IsPlaying() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.isPlayingFn != nil {
		return f.isPlayingFn()
	}
	return f.playing
}

func (f *fakePlayer) Err() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.err
}

func (f *fakePlayer) snapshotEvents() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.events...)
}

func (f *fakePlayer) pauseCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for _, e := range f.events {
		if e == "pause" {
			n++
		}
	}
	return n
}

// fakeHarness wires all listenConfig seams to in-memory fakes and records the
// teardown order, the WAV paths loadPCM was asked for, and every player the
// factory created (so the transition regression lock can assert release-before-
// construct). It is the single device-free substrate for the runListen tests.
type fakeHarness struct {
	mu        sync.Mutex
	order     []string // "restore" / "removeAll" in call order
	loadPaths []string
	created   []*fakePlayer

	loadFn           func(path string) ([]byte, error) // optional load override
	customize        func(idx int, f *fakePlayer)      // optional per-player setup
	releaseViolation bool                              // a transition built the next player before Pause()ing the prior
}

func (h *fakeHarness) restore() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.order = append(h.order, "restore")
}

func (h *fakeHarness) removeAll(string) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.order = append(h.order, "removeAll")
	return nil
}

func (h *fakeHarness) loadPCM(path string) ([]byte, error) {
	h.mu.Lock()
	h.loadPaths = append(h.loadPaths, path)
	fn := h.loadFn
	h.mu.Unlock()
	if fn != nil {
		return fn(path)
	}
	return []byte{0, 0, 0, 0}, nil // 2 int16 samples of silence — content is irrelevant to the fake.
}

func (h *fakeHarness) newPlayer(_ []byte) listenPlayer {
	h.mu.Lock()
	defer h.mu.Unlock()
	// Bounded-retention regression lock: the prior player MUST already have been
	// Pause()'d (released) before the next is constructed — at most one live.
	if n := len(h.created); n > 0 {
		if h.created[n-1].pauseCount() == 0 {
			h.releaseViolation = true
		}
	}
	f := &fakePlayer{}
	if h.customize != nil {
		h.customize(len(h.created), f)
	}
	h.created = append(h.created, f)
	return f
}

func (h *fakeHarness) lastLoadPath() string {
	h.mu.Lock()
	defer h.mu.Unlock()
	if len(h.loadPaths) == 0 {
		return ""
	}
	return h.loadPaths[len(h.loadPaths)-1]
}

func (h *fakeHarness) players() []*fakePlayer {
	h.mu.Lock()
	defer h.mu.Unlock()
	return append([]*fakePlayer(nil), h.created...)
}

// cfg builds a listenConfig wired to this harness. out defaults to io.Discard;
// callers that assert printed output replace cfg.out afterward.
func (h *fakeHarness) cfg(readByte func() (byte, error), readLine func() (string, error), shutdown <-chan shutdownRequest) listenConfig {
	return listenConfig{
		audioDir:  "/tmp/audio",
		newPlayer: h.newPlayer,
		loadPCM:   h.loadPCM,
		restore:   h.restore,
		removeAll: h.removeAll,
		tempDir:   "/tmp/narrate-xyz",
		readByte:  readByte,
		readLine:  readLine,
		shutdown:  shutdown,
		out:       io.Discard,
	}
}

// scriptedBytes returns a readByte seam that yields the given bytes in order,
// then returns io.EOF.
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

// delayedBytes delays ONLY the first byte by d (giving the end-of-block ticker
// time to fire while the loop is idle on the read), then yields keys in order
// and finally io.EOF.
func delayedBytes(d time.Duration, keys string) func() (byte, error) {
	var i int32 = -1
	b := []byte(keys)
	return func() (byte, error) {
		idx := atomic.AddInt32(&i, 1)
		if idx == 0 {
			time.Sleep(d)
		}
		if int(idx) >= len(b) {
			return 0, io.EOF
		}
		return b[int(idx)], nil
	}
}

func noLine() (string, error) { return "", nil }

func assertCleanupRan(t *testing.T, h *fakeHarness) {
	t.Helper()
	h.mu.Lock()
	defer h.mu.Unlock()
	restoreAt, removeAt := -1, -1
	for i, step := range h.order {
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

// --- pure helpers ------------------------------------------------------------

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

// --- guards ------------------------------------------------------------------

// TestRunListen_NilFactory pins the nil-guard (suggestion 5): a config with no
// player factory is a wiring error returned honestly, not a panic, and cleanup
// still runs.
func TestRunListen_NilFactory(t *testing.T) {
	t.Parallel()
	h := &fakeHarness{}
	cfg := h.cfg(scriptedBytes("q"), noLine, make(chan shutdownRequest))
	cfg.newPlayer = nil // the wiring error under test
	err := runListen(context.Background(), cfg, navTimeline())
	if err == nil || !strings.Contains(err.Error(), "listen player factory not configured") {
		t.Fatalf("runListen err = %v, want \"listen player factory not configured\"", err)
	}
	assertCleanupRan(t, h)
}

func TestRunListen_NoNavigable(t *testing.T) {
	t.Parallel()
	h := &fakeHarness{}
	cfg := h.cfg(func() (byte, error) { return 0, io.EOF }, noLine, make(chan shutdownRequest))
	// All zero-duration → no navigable blocks → guarded error + cleanup.
	empty := tl(bt("a", 0, 0, "a.wav"))
	if err := runListen(context.Background(), cfg, empty); err == nil {
		t.Fatal("runListen with no navigable blocks should return an error")
	}
	assertCleanupRan(t, h)
}

// --- navigation --------------------------------------------------------------

func TestRunListen_KeypressNavigation(t *testing.T) {
	t.Parallel()
	h := &fakeHarness{}
	// nnbq: play a(0) → n b(1) → n c(2) → b b(1) → q.
	cfg := h.cfg(scriptedBytes("nnbq"), noLine, make(chan shutdownRequest))
	if err := runListen(context.Background(), cfg, navTimeline()); err != nil {
		t.Fatalf("runListen returned error: %v", err)
	}
	if got := len(h.players()); got != 4 {
		t.Fatalf("players created = %d, want 4 (initial + n + n + b)", got)
	}
	if last := h.lastLoadPath(); last != "/tmp/audio/b.wav" {
		t.Fatalf("last loaded block = %q, want /tmp/audio/b.wav", last)
	}
	if h.releaseViolation {
		t.Fatal("a transition constructed the next player before Pause()ing the prior")
	}
	assertCleanupRan(t, h)
}

// TestRunListen_NavigationBoundaries pins the two navigation guards that make a
// keypress a silent no-op at a boundary: 'n' at the last navigable block and 'b'
// at position 0. A boundary no-op constructs no new player and leaves the last
// loaded block unchanged.
func TestRunListen_NavigationBoundaries(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name        string
		script      string
		wantPlayers int
		wantLastWav string
	}{
		{"n at last navigable is no-op", "nnn", 3, "/tmp/audio/c.wav"},
		{"b at position zero is no-op", "b", 1, "/tmp/audio/a.wav"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			h := &fakeHarness{}
			cfg := h.cfg(scriptedBytes(tc.script), noLine, make(chan shutdownRequest))
			if err := runListen(context.Background(), cfg, navTimeline()); err != nil {
				t.Fatalf("runListen returned error: %v", err)
			}
			if got := len(h.players()); got != tc.wantPlayers {
				t.Fatalf("players created = %d, want %d", got, tc.wantPlayers)
			}
			if last := h.lastLoadPath(); last != tc.wantLastWav {
				t.Fatalf("last loaded block = %q, want %q", last, tc.wantLastWav)
			}
			if h.releaseViolation {
				t.Fatal("a transition constructed the next player before Pause()ing the prior")
			}
		})
	}
}

func TestRunListen_GoTo(t *testing.T) {
	t.Parallel()
	h := &fakeHarness{}
	cfg := h.cfg(scriptedBytes("gq"), func() (string, error) { return "2\r", nil }, make(chan shutdownRequest))
	if err := runListen(context.Background(), cfg, navTimeline()); err != nil {
		t.Fatalf("runListen returned error: %v", err)
	}
	if got := len(h.players()); got != 2 {
		t.Fatalf("players created = %d, want 2 (initial + g jump)", got)
	}
	if last := h.lastLoadPath(); last != "/tmp/audio/c.wav" {
		t.Fatalf("g jumped to %q, want /tmp/audio/c.wav", last)
	}
	if h.releaseViolation {
		t.Fatal("g transition constructed the next player before Pause()ing the prior")
	}
}

// --- transition halt+release regression lock (BLOCKING plan item 2) ----------

// TestRunListen_TransitionHaltsAndReleasesPrior is the required regression lock:
// for each transition (n/b/g, and g-to-current as a replay) the PRIOR player is
// Pause()'d BEFORE the next is constructed (bounded retention, at most one live),
// and the final player is Pause()'d by cleanup. The seam has no Close() —
// release is Pause()+drop-the-reference, asserted device-free; leak-freedom is
// by-construction (in-memory buffer has no fd) and confirmed by-ear at /verify.
func TestRunListen_TransitionHaltsAndReleasesPrior(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name        string
		script      string
		line        string
		wantPlayers int
	}{
		{"next", "nq", "", 2},
		{"back", "nbq", "", 3},
		{"go-to", "gq", "2", 2},
		{"replay via go-to current block", "gq", "0", 2},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			h := &fakeHarness{}
			line := func() (string, error) { return tc.line + "\r", nil }
			cfg := h.cfg(scriptedBytes(tc.script), line, make(chan shutdownRequest))
			if err := runListen(context.Background(), cfg, navTimeline()); err != nil {
				t.Fatalf("runListen returned error: %v", err)
			}
			players := h.players()
			if len(players) != tc.wantPlayers {
				t.Fatalf("players created = %d, want %d", len(players), tc.wantPlayers)
			}
			if h.releaseViolation {
				t.Fatal("transition constructed the next player before Pause()ing the prior (bounded retention broken)")
			}
			// Every player — including the final one (Pause()'d by cleanup) — was
			// released exactly through Pause(), proving the loop never retains more
			// than one live player.
			for i, p := range players {
				if p.pauseCount() == 0 {
					t.Fatalf("player[%d] was never Pause()'d (reference not released)", i)
				}
			}
		})
	}
}

// --- true Pause/Resume (AC3) -------------------------------------------------

// TestRunListen_PauseResumeToggle pins the space toggle: space while playing →
// one Pause(); space while paused → one Play() (resume). The recorded event
// order on the single block's player is the lock.
func TestRunListen_PauseResumeToggle(t *testing.T) {
	t.Parallel()
	h := &fakeHarness{}
	// space (pause), space (resume), q.
	cfg := h.cfg(scriptedBytes("  q"), noLine, make(chan shutdownRequest))
	if err := runListen(context.Background(), cfg, navTimeline()); err != nil {
		t.Fatalf("runListen returned error: %v", err)
	}
	players := h.players()
	if len(players) != 1 {
		t.Fatalf("players created = %d, want 1 (space never constructs a player)", len(players))
	}
	// initial Play, space→Pause, space→Play (resume), q→cleanup Pause.
	got := players[0].snapshotEvents()
	want := []string{"play", "pause", "play", "pause"}
	if !equalStrs(got, want) {
		t.Fatalf("player events = %v, want %v", got, want)
	}
}

// TestRunListen_PauseClearedOnTransition proves paused never bleeds across a
// transition: pause block A, advance to B with n, then press space. If paused
// had bled true into B, space would RESUME (Play); because the transition clears
// paused, space PAUSES B instead. The second event on B's player is therefore
// "pause", not "play".
func TestRunListen_PauseClearedOnTransition(t *testing.T) {
	t.Parallel()
	h := &fakeHarness{}
	// space (pause A), n (→B, clears paused), space (must pause B, not resume), q.
	cfg := h.cfg(scriptedBytes(" n q"), noLine, make(chan shutdownRequest))
	if err := runListen(context.Background(), cfg, navTimeline()); err != nil {
		t.Fatalf("runListen returned error: %v", err)
	}
	players := h.players()
	if len(players) != 2 {
		t.Fatalf("players created = %d, want 2", len(players))
	}
	if h.releaseViolation {
		t.Fatal("n transition constructed B before Pause()ing A")
	}
	// B: initial Play, then space must PAUSE (paused was cleared) — not resume.
	bEvents := players[1].snapshotEvents()
	if len(bEvents) < 2 || bEvents[0] != "play" || bEvents[1] != "pause" {
		t.Fatalf("block B events = %v, want play then pause (paused must have been cleared on transition)", bEvents)
	}
}

// --- end-of-block edge detection ---------------------------------------------

// TestRunListen_EndOfBlockNoAutoAdvance scripts the fake's IsPlaying() to report
// false (block finished) and sets a player error. The ticker must mark the block
// ended, surface Err(), and NOT auto-advance (no second player constructed).
func TestRunListen_EndOfBlockNoAutoAdvance(t *testing.T) {
	t.Parallel()
	h := &fakeHarness{
		customize: func(_ int, f *fakePlayer) {
			f.isPlayingFn = func() bool { return false } // finished immediately
			f.err = errors.New("device underrun")
		},
	}
	var out strings.Builder
	// Delay the first (and only, before q) key so ≥1 ticker fires while idle.
	cfg := h.cfg(delayedBytes(150*time.Millisecond, "q"), noLine, make(chan shutdownRequest))
	cfg.out = &out
	if err := runListen(context.Background(), cfg, navTimeline()); err != nil {
		t.Fatalf("runListen returned error: %v", err)
	}
	if got := len(h.players()); got != 1 {
		t.Fatalf("players created = %d, want 1 (end-of-block must NOT auto-advance)", got)
	}
	if !strings.Contains(out.String(), "device underrun") {
		t.Fatalf("end-of-block did not surface player.Err(); out = %q", out.String())
	}
}

// TestRunListen_SpaceOnEndedBlockNoOp proves space is a no-op once a block has
// ended (nothing to resume): the ticker marks the block ended first (delayed
// keypress), then space must NOT drive the player. The only Pause() on the
// player is cleanup's, so the player records exactly one pause and no resume.
func TestRunListen_SpaceOnEndedBlockNoOp(t *testing.T) {
	t.Parallel()
	h := &fakeHarness{
		customize: func(_ int, f *fakePlayer) {
			f.isPlayingFn = func() bool { return false } // finished immediately
		},
	}
	// Delay so the block is marked ended before space arrives; then space, then q.
	cfg := h.cfg(delayedBytes(150*time.Millisecond, " q"), noLine, make(chan shutdownRequest))
	if err := runListen(context.Background(), cfg, navTimeline()); err != nil {
		t.Fatalf("runListen returned error: %v", err)
	}
	players := h.players()
	if len(players) != 1 {
		t.Fatalf("players created = %d, want 1", len(players))
	}
	got := players[0].snapshotEvents()
	// Expect: initial play, then ONLY cleanup's pause. A space acting on the ended
	// block would add an extra pause/play.
	want := []string{"play", "pause"}
	if !equalStrs(got, want) {
		t.Fatalf("ended-block player events = %v, want %v (space must be a no-op on an ended block)", got, want)
	}
}

// --- load error --------------------------------------------------------------

// TestRunListen_LoadErrorConstructsNoPlayer pins the step-2 acceptance: a
// mid-read WAV-load error constructs NO player, prints, and the loop continues
// (awaits the next key) rather than crashing.
func TestRunListen_LoadErrorConstructsNoPlayer(t *testing.T) {
	t.Parallel()
	h := &fakeHarness{
		loadFn: func(string) ([]byte, error) { return nil, errors.New("parse wav: boom") },
	}
	var out strings.Builder
	cfg := h.cfg(scriptedBytes("q"), noLine, make(chan shutdownRequest))
	cfg.out = &out
	if err := runListen(context.Background(), cfg, navTimeline()); err != nil {
		t.Fatalf("runListen returned error: %v", err)
	}
	if got := len(h.players()); got != 0 {
		t.Fatalf("players created = %d, want 0 (load error must construct no player)", got)
	}
	if !strings.Contains(out.String(), "load block") {
		t.Fatalf("load error not surfaced; out = %q", out.String())
	}
	assertCleanupRan(t, h)
}

// --- idle interrupt (issue #88) + ctx cancel ---------------------------------

// TestRunListen_InterruptibleRead proves the idle read is interruptible
// (issue #88): with readByte GENUINELY blocked (never returns), an interrupt — a
// shutdownRequest or a ctx-cancel — delivered AFTER the loop is parked on the
// read must still run the full cleanup and return nil WITHOUT any keypress.
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
			h := &fakeHarness{}
			shutdown := make(chan shutdownRequest, 1)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			reading := make(chan struct{})
			block := make(chan struct{})
			var once sync.Once
			readByte := func() (byte, error) {
				once.Do(func() { close(reading) })
				<-block // never fed — the read is genuinely blocked
				return 0, io.EOF
			}
			cfg := h.cfg(readByte, noLine, shutdown)

			done := make(chan error, 1)
			go func() { done <- runListen(ctx, cfg, navTimeline()) }()

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
			assertCleanupRan(t, h)
		})
	}
}

func TestRunListen_ContextCancel(t *testing.T) {
	t.Parallel()
	h := &fakeHarness{}
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancelled before loop starts
	cfg := h.cfg(func() (byte, error) { return 'n', nil }, noLine, make(chan shutdownRequest))
	if err := runListen(ctx, cfg, navTimeline()); err != nil {
		t.Fatalf("runListen returned error on ctx cancel: %v", err)
	}
	assertCleanupRan(t, h)
}

// --- engine-failure cleanup at the driveListen boundary (suggestion 4) -------

// TestDriveListen_EngineFailureCleanup injects a failing context opener and a
// never-ready opener at the driveListenWith boundary. Both must route through
// listenCleanup (restore tty + remove temp dir) and return a wrapped, ACTIONABLE
// error UP the pipeline — an engine/device-open failure is an honest error, NOT
// a Refusal, and never a nil-deref.
func TestDriveListen_EngineFailureCleanup(t *testing.T) {
	t.Parallel()

	okFactory := func(_ []byte) listenPlayer { return &fakePlayer{} }

	tests := []struct {
		name         string
		open         otoContextOpener
		readyTimeout time.Duration
		ctxCancelled bool
		wantSubstr   string
	}{
		{
			name: "NewContext fails",
			open: func() (func([]byte) listenPlayer, <-chan struct{}, error) {
				return nil, nil, errors.New("no audio device")
			},
			readyTimeout: time.Second,
			wantSubstr:   "cannot open audio device",
		},
		{
			name: "ready never fires (timeout)",
			open: func() (func([]byte) listenPlayer, <-chan struct{}, error) {
				return okFactory, make(chan struct{}), nil // never closed
			},
			readyTimeout: 20 * time.Millisecond,
			wantSubstr:   "did not become ready",
		},
		{
			name: "ctx cancelled during ready-wait",
			open: func() (func([]byte) listenPlayer, <-chan struct{}, error) {
				return okFactory, make(chan struct{}), nil // never closed
			},
			readyTimeout: time.Second,
			ctxCancelled: true,
			wantSubstr:   "not ready",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			h := &fakeHarness{}
			cfg := h.cfg(scriptedBytes("q"), noLine, make(chan shutdownRequest))
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if tc.ctxCancelled {
				cancel()
			}
			err := driveListenWith(ctx, cfg, navTimeline(), tc.open, tc.readyTimeout)
			if err == nil {
				t.Fatal("driveListenWith should return an error on engine failure")
			}
			if !strings.Contains(err.Error(), tc.wantSubstr) {
				t.Fatalf("error = %q, want substring %q", err.Error(), tc.wantSubstr)
			}
			// Honesty rule: the failure is a plain error UP the pipeline, never a
			// Refusal — and the actionable message names the speak/MCP path.
			if !strings.Contains(err.Error(), "speak/MCP path is unaffected") {
				t.Fatalf("error = %q, want the actionable speak/MCP note", err.Error())
			}
			// No player should ever have been constructed on the failure path.
			if got := len(h.players()); got != 0 {
				t.Fatalf("players created = %d, want 0 on engine failure", got)
			}
			assertCleanupRan(t, h)
		})
	}
}

func equalStrs(a, b []string) bool {
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
