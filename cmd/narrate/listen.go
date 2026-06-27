// Listen-path transport controller for cmd/narrate (issue #83).
//
// This file lives entirely in the cmd/narrate composition root. The planner
// and plan packages stay I/O-free; nothing here is imported by them. The
// listen path is decoupled from the durable sink: cmd/narrate keeps its own
// per-block afplay seam (playBlock) rather than routing through sink/ephemeral,
// so "play then delete temp dir" lifetime and the no-persistence intent stay
// intact.
//
// What it does: after the whole plan is rendered (no streaming — the planner
// needs the whole document), runListen drives afplay one block at a time in
// response to single keypresses, so a listener can step next/back, replay the
// current block, jump to a block, and quit. One signal handler guarantees the
// terminal returns to cooked mode, the afplay child is reaped, and the
// once-per-session temp dir is removed.
//
// Concurrency model (planner-task.md "Invariants Under Concurrency"):
//   - At most one afplay child alive: the loop kills + Wait()s the current
//     child before spawning the next (reap-before-spawn). The loop goroutine
//     is the SOLE owner of the *exec.Cmd handle; the signal handler never
//     touches it — it only REQUESTS shutdown over a channel, and the loop
//     reaps. A per-child "reaped" flag guards against an intra-goroutine
//     double-Wait.
//   - Terminal always ends cooked: enterRaw returns an idempotent (sync.Once)
//     restore closure that is registered BOTH as a deferred call AND inside the
//     shutdown path — first call wins, the other is a no-op.
//   - Temp dir removed exactly once by one owner: the listen path owns the
//     RemoveAll. runNarrate suppresses its own defer-based removal for the
//     listen path so the dir is not double-owned.
//   - Interruptible idle read (issue #88): the blocking stdin read lives in a
//     detached reader goroutine feeding byteCh; the loop selects over
//     {byteCh, shutdown, ctx.Done()} so a kill -TERM delivered while idle is
//     serviced (tty restored, child reaped) without a keypress. The reader
//     reads exactly one byte per grant and parks between reads, so the g-target
//     readLine (read from the loop goroutine) is never racing the same fd.
//
// Honesty rule (CLAUDE.md, non-negotiable): on the DEFAULT (afplay) build the
// on-screen controls are labeled "Stop / Replay block", never "Pause" — afplay
// cannot pause the audio device (SIGSTOP/SIGCONT does not freeze it), so space
// stops the current block and replays it from the top. The build-tagged spike
// seam in listen_oto.go (//go:build oto, issue #100) is the only build that
// labels space "Pause / Resume", because oto v3 delivers true device-level
// pause. The active label is the build-tagged listenControlsLine constant, so
// it can never drift from the active key binding. Block-level only either way:
// the whole per-block WAV is the unit; there is no sub-block or word-level seek.
//
// macOS-only, phase one — same boundary as sink/ephemeral (afplay) and the oto
// purego/CoreAudio path.
package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"path/filepath"
	"sync"

	"github.com/vd09-projects/intelligent-tts-narration-library/plan"
)

// playFn is the per-block playback seam runListen drives. Production passes
// playBlock; tests pass a stub that blocks until ctx cancel (to exercise the
// reap-before-spawn discipline) without touching audio hardware.
//
// The seam takes a *started* contract: it spawns the child, returns the handle
// so the caller (the loop) owns reaping, and blocks until the child exits or
// ctx is cancelled. See playBlock for the production implementation.
//
//nolint:unused // afplay seam — live in the default (!oto) build + listen_test.go; intentionally dormant under -tags oto, which compiles the listen_oto.go seam instead. Removed when #101 collapses the build-tag pair.
type playFn func(ctx context.Context, binary, wavPath string) (*exec.Cmd, error)

// The concrete afplay implementation of playFn (playBlock) and its timeout
// constants live in the build-tagged sibling listen_afplay.go (//go:build
// !oto) so the default/shipped seam is paired with the //go:build oto spike
// seam in listen_oto.go. See those files and printLegend's listenControlsLine.

// navigableBlocks returns the positions (indices into timeline.Blocks) of the
// blocks a listener can navigate to with n/b/g: those with a non-empty AudioRef
// AND a non-zero duration (EndMs > StartMs).
//
// Refused or zero-duration blocks are intentionally retained in the display
// roster (the listener still SEES them in the legend/roster) but are skipped
// for navigation — there is nothing to play, so landing on one would be a
// silent dead key. This honors the honesty rule's spirit (refusals stay
// visible) while keeping navigation meaningful.
//
// A refused block whose renderer produced a spoken refusal notice WILL have a
// non-empty AudioRef and a non-zero duration, so it remains navigable — the
// listener hears the spoken refusal. Only blocks with no audio at all (empty
// ref) or a degenerate zero-length clip are skipped.
func navigableBlocks(timeline plan.Timeline) []int {
	var nav []int
	for i, bt := range timeline.Blocks {
		if bt.AudioRef == "" {
			continue
		}
		if bt.EndMs-bt.StartMs <= 0 {
			continue
		}
		nav = append(nav, i)
	}
	return nav
}

// nearestNavigable resolves an arbitrary timeline position to the nearest
// navigable position, used by the g (go-to) jump so a target that landed on a
// refused/zero-duration block snaps to the closest playable block rather than
// becoming a dead jump. Ties break toward the later block. Returns the
// position within nav (an index into the nav slice), not the timeline index.
//
// Precondition: nav is non-empty (callers guard with len(nav) == 0 up front,
// since a plan with zero navigable blocks cannot enter the listen loop).
func nearestNavigable(nav []int, target int) int {
	best := 0
	bestDist := abs(nav[0] - target)
	for i := 1; i < len(nav); i++ {
		d := abs(nav[i] - target)
		if d <= bestDist {
			bestDist = d
			best = i
		}
	}
	return best
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

// enterRaw puts the terminal at fd into raw mode and returns an idempotent
// restore closure. The restore is guarded by sync.Once so it is safe to call
// from BOTH the deferred cleanup AND the shutdown path — the first call wins,
// the rest are no-ops. This is the load-bearing "terminal always ends cooked"
// invariant: whichever path fires first restores the tty, and a second call
// (defer firing after the shutdown path, or vice versa) does nothing.
//
// rawState is the seam over golang.org/x/term so the package compiles and is
// unit-testable without a real tty. Production wires termMakeRaw / termRestore
// to x/term; tests swap them.
func enterRaw(fd int) (restore func(), err error) {
	st, err := termMakeRaw(fd)
	if err != nil {
		return nil, fmt.Errorf("enter raw mode: %w", err)
	}
	var once sync.Once
	return func() {
		once.Do(func() {
			// Best-effort restore: if it fails there is nothing useful to do
			// (the process is on its way out), and surfacing it from a deferred
			// no-arg closure is awkward. The manual verify (5× Ctrl-C → cooked
			// tty) is the real gate here.
			_ = termRestore(fd, st)
		})
	}, nil
}

// term seams — overridable in tests. Production points them at x/term.
// rawRestoreState is whatever the term backend needs to undo MakeRaw.
type rawRestoreState = any

var (
	termMakeRaw = func(fd int) (rawRestoreState, error) { return defaultMakeRaw(fd) }
	termRestore = func(fd int, st rawRestoreState) error { return defaultRestore(fd, st) }
)

// shutdownRequest is what the single signal handler sends to the loop. It
// carries no data — its presence on the channel is the request. The loop, the
// SOLE owner of the child handle, performs the actual reap + cleanup. This is
// the "request-not-reap" split: the handler never touches the *exec.Cmd, the
// terminal, or the temp dir directly; it asks, the loop acts.
type shutdownRequest struct{}

// listenConfig bundles the seams runListen needs so the keypress loop is fully
// unit-testable without a real tty, real afplay, or a real signal. Production
// wires these in runNarrate; tests pass stubs.
type listenConfig struct {
	binary   string // afplay binary path.
	audioDir string // render temp dir holding per-block WAVs.
	//nolint:unused // afplay seam (playBlock in prod); set only by the !oto driver, dormant under -tags oto.
	play      playFn
	restore   func()                 // idempotent tty restore (from enterRaw).
	removeAll func(string) error     // temp-dir removal seam (os.RemoveAll in prod).
	tempDir   string                 // session temp dir the loop owns removal of.
	readByte  func() (byte, error)   // one raw stdin byte (blocking) — prod reads fd.
	readLine  func() (string, error) // raw-mode line read for the g target.
	shutdown  <-chan shutdownRequest // single signal handler → loop.
	out       io.Writer              // legend + per-block static display.
}

// childHandle is the loop's private record of the current afplay child plus a
// reaped flag. The flag is the intra-goroutine double-Wait guard
// (planner-task.md): the loop reaps a child exactly once even though the reap
// is reachable from several branches (next transition, replay, quit, shutdown).
//
//nolint:unused // afplay seam — dormant under -tags oto (see playFn).
type childHandle struct {
	cmd    *exec.Cmd
	reaped bool
}

// reap kills the current child and Wait()s it exactly once. Calling reap on a
// nil handle, an already-reaped handle, or a handle whose cmd already exited is
// a safe no-op. Only the loop goroutine calls this, so no locking is needed —
// the single-owner model is the synchronization.
//
//nolint:unused // afplay seam — dormant under -tags oto (see playFn).
func (h *childHandle) reap() {
	if h == nil || h.cmd == nil || h.reaped {
		return
	}
	h.reaped = true
	if h.cmd.Process != nil {
		// Best-effort kill; the process may already have exited. WaitDelay on
		// the cmd backstops a slow death.
		_ = h.cmd.Process.Kill()
	}
	// Wait reaps the zombie. playBlock's own wait goroutine may have already
	// called Wait(); a second Wait returns an error we intentionally discard —
	// the reaped flag means we only reach here once per child from the loop.
	_ = h.cmd.Wait()
}

// runListen is the keypress transport loop. It renders nothing — by the time it
// is called the whole plan has been rendered and timeline.Blocks + audioDir
// point at the per-block WAVs. It reads one byte at a time from raw stdin and
// dispatches:
//
//	n      — next navigable block, play it
//	b      — back to the previous navigable block, play it
//	space  — Stop / Replay block: stop the current child, replay the same block
//	g       — go-to: read a block index, jump to the nearest navigable, play
//	q      — quit: stop the child, restore tty, remove temp dir, return
//
// Before every spawn the current child is killed + Wait()ed (reap-before-spawn)
// so at most one afplay child is ever alive. The loop is the sole owner of the
// child handle; the signal handler only sends shutdownRequest, and the loop
// performs the cleanup in order: restore → reap → RemoveAll.
//
// It returns nil on a clean q/shutdown exit, or a wrapped error if a stdin read
// fails irrecoverably (a play error does NOT stop the loop — a failed block
// surfaces and the listener can space-replay or move on, per the
// no-retry/manual-replay stance).
//
//nolint:unused // afplay seam — driven by the !oto driveListen + listen_test.go; the -tags oto build uses runListenOto (listen_oto.go) instead. Both collapse into one loop in #101.
func runListen(ctx context.Context, cfg listenConfig, timeline plan.Timeline) error {
	nav := navigableBlocks(timeline)
	if len(nav) == 0 {
		// Nothing to play. Caller (runNarrate) fail-fasts before reaching here,
		// but guard so the loop never indexes an empty nav slice.
		cfg.restore()
		_ = cfg.removeAll(cfg.tempDir)
		return errors.New("listen: no navigable blocks (all empty/refused/zero-duration)")
	}

	child := &childHandle{}

	// cleanup runs the load-bearing teardown order exactly once: restore tty →
	// reap child → RemoveAll temp dir. restore is idempotent (sync.Once);
	// child.reap is idempotent (reaped flag); removeAll on an already-removed
	// dir is a harmless no-op. Deferred so a panic in the loop still ends with
	// a cooked tty and no zombie.
	var cleanupOnce sync.Once
	cleanup := func() {
		cleanupOnce.Do(func() {
			cfg.restore()
			child.reap()
			_ = cfg.removeAll(cfg.tempDir)
		})
	}
	defer cleanup()

	printLegend(cfg.out, timeline, nav)

	// spawn reaps the current child (reap-before-spawn) then starts the block
	// at nav[pos]. A play error is printed, not returned — the loop continues.
	spawn := func(pos int) {
		child.reap()
		child = &childHandle{}
		blkIdx := nav[pos]
		bt := timeline.Blocks[blkIdx]
		printNowPlaying(cfg.out, blkIdx, bt)
		wavPath := filepath.Join(cfg.audioDir, bt.AudioRef)
		cmd, err := cfg.play(ctx, cfg.binary, wavPath)
		if cmd != nil {
			child.cmd = cmd
			// play() already Wait()ed the child on the clean / self-exit path;
			// reflect that so a later reap is a no-op rather than a double-kill.
			if err == nil || (!errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded)) {
				child.reaped = true
			}
		}
		if err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
			_, _ = fmt.Fprintf(cfg.out, "  ! playback failed: %v — press space to replay or n/b to move\r\n", err)
		}
	}

	// Interruptible read (issue #88). A blocking cfg.readByte() is parked on a
	// stdin syscall and cannot observe a shutdownRequest, so an external
	// kill -TERM while the listener is idle at a keypress was not serviced until
	// the next byte — leaving the tty raw and the afplay child unreaped. The fix
	// moves the read into a detached goroutine that feeds bytes on byteCh; the
	// loop selects over {byteCh, shutdown, ctx.Done()} so an idle shutdown wins
	// immediately and runs cleanup without a keypress.
	//
	// Single-reader-of-stdin invariant: the reader reads exactly one byte per
	// grant (a token on wantByte) and PARKS on wantByte between reads — it is
	// never blocked on a stdin read unless the loop granted it. The g-target
	// path (cfg.readLine) reads stdin directly from the loop goroutine; because
	// the loop withholds the grant across that read, the reader is parked and
	// not racing the same fd. The loop maintains exactly one outstanding grant.
	//
	// The reader is detached: cleanup never joins it (planner-task.md). On exit
	// the loop's defer cancels readerCtx; a reader parked on wantByte returns at
	// once, and a reader blocked on an in-flight cfg.readByte() returns as soon
	// as that read yields a byte/EOF (the byteCh send then loses to
	// readerCtx.Done()). The process is on its way out either way.
	type byteRead struct {
		b   byte
		err error
	}
	byteCh := make(chan byteRead, 1)
	wantByte := make(chan struct{}, 1)
	readerCtx, stopReader := context.WithCancel(ctx)
	defer stopReader()
	go func() {
		for {
			select {
			case <-wantByte:
			case <-readerCtx.Done():
				return
			}
			b, err := cfg.readByte()
			select {
			case byteCh <- byteRead{b: b, err: err}:
			case <-readerCtx.Done():
				return
			}
			if err != nil {
				return
			}
		}
	}()
	// grantRead asks the reader for exactly one byte. Non-blocking send over a
	// buffered(1) channel: with the one-outstanding-grant discipline the buffer
	// is always empty here, and the default is pure belt-and-suspenders against
	// an accidental double-grant (which would let the reader read ahead).
	grantRead := func() {
		select {
		case wantByte <- struct{}{}:
		default:
		}
	}

	pos := 0
	spawn(pos)
	grantRead()

	for {
		select {
		case <-cfg.shutdown:
			cleanup()
			return nil
		case <-ctx.Done():
			cleanup()
			return nil
		case r := <-byteCh:
			if r.err != nil {
				// A read error after a shutdown signal closed stdin is expected;
				// distinguish a genuine read failure from a clean shutdown.
				select {
				case <-cfg.shutdown:
					cleanup()
					return nil
				default:
				}
				cleanup()
				if errors.Is(r.err, io.EOF) {
					return nil
				}
				return fmt.Errorf("listen: read stdin: %w", r.err)
			}

			switch r.b {
			case 'n':
				if pos < len(nav)-1 {
					pos++
					spawn(pos)
				}
				grantRead()
			case 'b':
				if pos > 0 {
					pos--
					spawn(pos)
				}
				grantRead()
			case ' ':
				// Stop / Replay block (honesty rule: never "Pause"). Stop the
				// current child and replay the SAME block from the top.
				spawn(pos)
				grantRead()
			case 'g':
				// Read the target line directly here. The reader is parked on
				// wantByte (no grant outstanding across this read), so this is
				// the sole stdin reader — the single-reader invariant holds.
				_, _ = fmt.Fprint(cfg.out, "  go to block index (0-based): \r\n")
				line, lerr := cfg.readLine()
				if lerr != nil {
					_, _ = fmt.Fprintf(cfg.out, "  ! could not read target: %v\r\n", lerr)
					grantRead()
					continue
				}
				target, perr := parseBlockIndex(line, len(timeline.Blocks))
				if perr != nil {
					_, _ = fmt.Fprintf(cfg.out, "  ! %v\r\n", perr)
					grantRead()
					continue
				}
				pos = nearestNavigable(nav, target)
				spawn(pos)
				grantRead()
			case 'q':
				cleanup()
				return nil
			default:
				// Unknown key — ignore. Keeps the loop forgiving in raw mode
				// where arrow keys etc. arrive as escape sequences.
				grantRead()
			}
		}
	}
}

// parseBlockIndex parses the g-target line into a 0-based block index in
// [0, n). A simple numeric line-read is the chosen g UX (planner-task.md open
// question): the listener types a block index and presses enter. Out-of-range
// or non-numeric input is rejected with an honest message rather than silently
// clamped, so the listener knows their input was ignored.
func parseBlockIndex(line string, n int) (int, error) {
	trimmed := ""
	for _, r := range line {
		if r == '\r' || r == '\n' || r == ' ' || r == '\t' {
			continue
		}
		trimmed += string(r)
	}
	if trimmed == "" {
		return 0, errors.New("empty target — give a block index")
	}
	v := 0
	for _, r := range trimmed {
		if r < '0' || r > '9' {
			return 0, fmt.Errorf("not a block index: %q", trimmed)
		}
		v = v*10 + int(r-'0')
	}
	if v >= n {
		return 0, fmt.Errorf("block index %d out of range (have %d blocks)", v, n)
	}
	return v, nil
}

// printLegend writes the honest control legend + the static per-block roster
// once at loop start. This is a STATIC display (printed once), not a live
// progress bar — the two-channel model puts any live progress in a separate
// observer (Channel 2), a different ticket. Controls are labeled
// "Stop / Replay block", never "Pause".
func printLegend(w io.Writer, timeline plan.Timeline, nav []int) {
	navSet := make(map[int]bool, len(nav))
	for _, p := range nav {
		navSet[p] = true
	}
	_, _ = fmt.Fprint(w, "\r\n")
	_, _ = fmt.Fprint(w, "Listen mode — single-key transport (block-level)\r\n")
	// listenControlsLine is build-tagged (listen_afplay.go vs listen_oto.go) so
	// the on-screen label can never drift from the active key binding: the
	// default afplay build prints "Stop / Replay block" (honesty rule — no true
	// pause), the oto spike build prints "Pause / Resume" (true device pause).
	_, _ = fmt.Fprintf(w, "  %s\r\n", listenControlsLine)
	_, _ = fmt.Fprintf(w, "  %d blocks, %d navigable (empty/refused/zero-duration shown but skipped)\r\n", len(timeline.Blocks), len(nav))
	_, _ = fmt.Fprint(w, "\r\n")
	for i, bt := range timeline.Blocks {
		marker := "  -"
		if navSet[i] {
			marker = "  *"
		}
		// StartMs/EndMs are display-only (CLAUDE.md: offsets are sync data,
		// never a seek target). Block-level only.
		_, _ = fmt.Fprintf(w, "%s [%d] %s  %d-%dms\r\n", marker, i, bt.BlockID, bt.StartMs, bt.EndMs)
	}
	_, _ = fmt.Fprint(w, "\r\n")
}

// printNowPlaying writes a one-line static "now playing" notice for the block
// at timeline index blkIdx. Static, not a progress bar.
func printNowPlaying(w io.Writer, blkIdx int, bt plan.BlockTiming) {
	_, _ = fmt.Fprintf(w, "> playing [%d] %s (%d-%dms)\r\n", blkIdx, bt.BlockID, bt.StartMs, bt.EndMs)
}
