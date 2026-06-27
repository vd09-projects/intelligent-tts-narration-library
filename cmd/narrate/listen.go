// Listen-path transport controller for cmd/narrate (issue #83, productionized
// for the in-process oto v3 player in #101).
//
// This file lives entirely in the cmd/narrate composition root. The planner
// and plan packages stay I/O-free; nothing here is imported by them. The
// listen path is decoupled from the durable sink: cmd/narrate reads each
// per-block WAV straight from the render temp dir, so "play then delete temp
// dir" lifetime and the no-persistence intent stay intact.
//
// What it does: after the whole plan is rendered (no streaming — the planner
// needs the whole document), runListen drives an in-process oto v3 player one
// block at a time in response to single keypresses, so a listener can step
// next/back, replay the current block, jump to a block, true-Pause/Resume the
// current block, and quit. One signal handler guarantees the terminal returns
// to cooked mode and the once-per-session temp dir is removed.
//
// Engine: the in-process oto v3 player is the DEFAULT and ONLY listen engine
// (#101 collapsed the #100 build-tag spike). It consumes raw 24 kHz mono int16
// PCM through an io.Reader and delivers true device-level Pause/Resume — the
// capability afplay+SIGSTOP structurally cannot give. The listen path no longer
// shells out to afplay; sink/ephemeral's afplay (the speak/MCP path) is a
// separate concern and is untouched. The player lives behind the listenPlayer
// seam so this loop is unit-testable without an audio device; the only place
// that imports oto is driveListen's real factory (listen_oto.go).
//
// Concurrency model (planner-task.md "Invariants Under Concurrency"):
//   - Single owner of the player handle: only the loop goroutine calls
//     Play/Pause/IsPlaying or drops the reference. The signal handler never
//     touches it — it only REQUESTS shutdown over a channel, and the loop acts.
//   - Bounded retention: at most one live player + one PCM buffer is referenced.
//     On every n/b/g/replay transition (and cleanup) the prior player is
//     Pause()'d — removing it from oto's active mux set so it stops pulling its
//     source — THEN its Go reference is dropped before the next is constructed.
//     A dropped, paused player becomes GC-reclaimable via oto's runtime
//     finalizer; the in-memory PCM buffer has no fd, so the finalizer reclaims
//     only memory. This is NOT a process-lifetime leak. Player.Close() is NOT
//     called: in oto v3.4 it is a documented no-op that does not stop the read
//     pull and trips staticcheck SA1019, so it must be avoided outright (not
//     //nolint-suppressed). Release is Pause()+drop-the-reference; the finalizer
//     does the real teardown.
//   - paused is authoritative: the loop-owned paused flag is the sole source of
//     play/pause truth — never inferred from oto, because a paused player and a
//     finished player both report !IsPlaying(). The end-of-block edge is gated
//     on !paused so a pause is never misread as block end.
//   - Terminal always ends cooked: enterRaw returns an idempotent (sync.Once)
//     restore closure registered BOTH as a deferred call AND inside the shutdown
//     path — first call wins, the other is a no-op.
//   - Temp dir removed exactly once by one owner: the listen path owns the
//     RemoveAll. runNarrate suppresses its own defer-based removal for the
//     listen path so the dir is not double-owned.
//   - Interruptible idle read (issue #88): the blocking stdin read lives in a
//     detached reader goroutine feeding byteCh; the loop selects over
//     {byteCh, shutdown, ctx.Done(), ticker} so a kill -TERM delivered while
//     idle is serviced (tty restored, temp dir removed) without a keypress. The
//     reader reads exactly one byte per grant and parks between reads, so the
//     g-target readLine (read from the loop goroutine) is never racing the fd.
//
// Honesty rule (CLAUDE.md, non-negotiable): the on-screen controls say
// "Pause / Resume" only because oto genuinely pauses the audio device. An engine
// or device-open failure is surfaced as an honest error UP the pipeline (it
// stops), never as a Refusal — refusals are for readable-but-unvoiceable
// content, not a missing audio device. Block-level only: the whole per-block WAV
// is the unit; there is no sub-block or word-level seek.
//
// macOS-only, phase one — same boundary as sink/ephemeral and the oto
// purego/CoreAudio path.
package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"sync"
	"time"

	"github.com/vd09-projects/intelligent-tts-narration-library/plan"
)

// listenControlsLine is the honest control legend: space is "Pause / Resume"
// because the in-process oto v3 player delivers true device-level pause (decision
// 2026-06-27-true-pause-via-oto-v3: device-confirmed pause delta 0 bytes). It is
// a single non-tagged constant now that oto is the sole listen engine — the
// on-screen label can never drift from the active key binding.
const listenControlsLine = "n next   b back   space Pause / Resume   g go-to   q quit"

// endOfBlockPollInterval is how often the loop polls player.IsPlaying() to detect
// a natural end-of-block. ~50 ms keeps the pause feel immediate without
// busy-spinning (the exact interval is not load-bearing — plan open question,
// tune by ear at /verify).
const endOfBlockPollInterval = 50 * time.Millisecond

// listenReadyTimeout bounds how long driveListen waits for oto's ready channel
// before giving up with an honest error. A device that never reports ready must
// not hang the listen session forever. Package-level so the engine-failure tests
// can shrink it (the production value is generous — device init is fast on
// macOS).
var listenReadyTimeout = 10 * time.Second

// listenPlayer is the per-block playback seam runListen drives. It is the whole
// surface the transport loop needs, so the loop is unit-testable with a fake
// player and NO audio device. driveListen's real factory wraps an *oto.Player
// (listen_oto.go); runListen never imports oto.
//
// There is deliberately NO Close(): in oto v3.4 Player.Close() is a documented
// no-op (teardown is finalizer/GC-driven) that trips SA1019, so teardown here is
// Pause()+drop-the-reference, not an explicit release.
type listenPlayer interface {
	// Play starts (or resumes) playback. It returns immediately; the engine
	// pulls the source on its own goroutine.
	Play()
	// Pause halts playback by removing the player from oto's active mux set so it
	// stops pulling its source. A paused player becomes GC-reclaimable once the
	// last Go reference to it is dropped (oto's runtime finalizer does the real
	// teardown); there is no explicit release call.
	Pause()
	// IsPlaying reports whether the engine is actively pulling the source. It is
	// used ONLY as an end-of-block edge signal — a paused player and a finished
	// player both report false, so play/pause truth comes from the loop-owned
	// paused flag, never from this.
	IsPlaying() bool
	// Err reports a non-nil error on construction failure, a device
	// underrun/error mid-play, or an after-drain device error; it is nil during
	// normal play and after a clean end-of-block.
	Err() error
}

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
// SOLE owner of the player handle, performs the actual teardown. This is the
// "request-not-reap" split: the handler never touches the player, the terminal,
// or the temp dir directly; it asks, the loop acts.
type shutdownRequest struct{}

// listenConfig bundles the seams runListen needs so the keypress loop is fully
// unit-testable without a real tty, a real audio device, or a real signal.
// Production wires these in runListenMode + driveListen; tests pass stubs.
type listenConfig struct {
	audioDir string // render temp dir holding per-block WAVs.
	// newPlayer constructs the per-block player from in-memory raw PCM. The real
	// factory (wrapping otoCtx.NewPlayer over a *bytes.Reader) is built only in
	// driveListen; runListen calls this and never imports oto. A nil factory is a
	// wiring error runListen rejects up front.
	newPlayer func(pcm []byte) listenPlayer
	// loadPCM reads a per-block WAV fully into memory and strips the header to
	// raw PCM. Production wires loadBlockPCM (os.Open + stripWAVToPCM); tests
	// inject canned bytes so no real file or device is touched.
	loadPCM   func(wavPath string) ([]byte, error)
	restore   func()                 // idempotent tty restore (from enterRaw).
	removeAll func(string) error     // temp-dir removal seam (os.RemoveAll in prod).
	tempDir   string                 // session temp dir the loop owns removal of.
	readByte  func() (byte, error)   // one raw stdin byte (blocking) — prod reads fd.
	readLine  func() (string, error) // raw-mode line read for the g target.
	shutdown  <-chan shutdownRequest // single signal handler → loop.
	out       io.Writer              // legend + per-block static display.
}

// listenEnginePreflight is the pre-loop engine check. It is a no-op now that oto
// is the sole listen engine: there is no external binary to look up (the old
// afplay seam did that), and the engine's real failure mode — no audio device /
// context construction failure — surfaces honestly from oto.NewContext in
// driveListen. Retained as a no-op so runListenMode's fail-fast structure is
// unchanged.
func listenEnginePreflight() error { return nil }

// listenCleanup runs the device-independent teardown — restore the tty, remove
// the session temp dir — exactly the work the engine-failure path needs before
// any player exists. It is the single extracted helper used by driveListen's
// NewContext-failure and ready-wait-timeout branches and by runListen's
// before-the-loop guards, so that teardown ordering lives in one place.
func listenCleanup(cfg listenConfig) {
	cfg.restore()
	_ = cfg.removeAll(cfg.tempDir)
}

// otoContextOpener opens the process-wide audio context and returns a player
// factory bound to it, the context's ready channel, and any construction error.
// The real opener (openOtoContext, listen_oto.go) is the only thing that imports
// oto; tests inject a fake opener so driveListen is exercised without a device.
type otoContextOpener func() (newPlayer func(pcm []byte) listenPlayer, ready <-chan struct{}, err error)

// driveListen is the composition-root engine entry runListenMode calls. It opens
// the process-wide oto context via the real opener, then delegates to
// driveListenWith. Kept as a thin wrapper so the device-free seam
// (driveListenWith) is the unit-tested surface.
func driveListen(ctx context.Context, cfg listenConfig, timeline plan.Timeline) error {
	return driveListenWith(ctx, cfg, timeline, openOtoContext, listenReadyTimeout)
}

// driveListenWith is the device-free core of the engine wiring. It checks the
// opener's construction error AND waits on the ready channel with a bounded
// timeout / ctx.Done() before the first play. On EITHER failure it routes
// through the single listenCleanup helper (restore tty + remove temp dir) and
// returns a wrapped, ACTIONABLE error UP the pipeline — an engine/device-open
// failure is an honest error, never a Refusal. On success it injects the player
// factory and hands off to runListen.
func driveListenWith(ctx context.Context, cfg listenConfig, timeline plan.Timeline, open otoContextOpener, readyTimeout time.Duration) error {
	factory, ready, err := open()
	if err != nil {
		listenCleanup(cfg)
		return fmt.Errorf("listen: cannot open audio device: %w "+
			"(listen mode needs an openable audio device; the non-interactive speak/MCP path is unaffected)", err)
	}

	// Bounded ready-wait: a device that never signals ready must not hang the
	// session. ctx cancellation and the timeout both route through the SAME
	// cleanup + actionable-error path as a NewContext failure.
	select {
	case <-ready:
	case <-ctx.Done():
		listenCleanup(cfg)
		return fmt.Errorf("listen: audio device not ready: %w "+
			"(listen mode needs an openable audio device; the non-interactive speak/MCP path is unaffected)", ctx.Err())
	case <-time.After(readyTimeout):
		listenCleanup(cfg)
		return fmt.Errorf("listen: audio device did not become ready within %s "+
			"(listen mode needs an openable audio device; the non-interactive speak/MCP path is unaffected)", readyTimeout)
	}

	cfg.newPlayer = factory
	return runListen(ctx, cfg, timeline)
}

// runListen is the keypress transport loop. It renders nothing — by the time it
// is called the whole plan has been rendered and timeline.Blocks + audioDir
// point at the per-block WAVs. It runs a Play()-then-watch-IsPlaying() lifecycle:
// the player's Play() returns immediately (the engine pulls the PCM on its own
// goroutine) and the loop selects over {byteCh, shutdown, ctx.Done(), ticker} so
// a mid-block keypress is serviced while audio is still playing. It dispatches:
//
//	n      — next navigable block, play it
//	b      — back to the previous navigable block, play it
//	space  — Pause / Resume the current block (true device pause)
//	g      — go-to: read a block index, jump to the nearest navigable, play
//	q      — quit: restore tty, halt+release the player, remove temp dir, return
//
// Bounded retention: every transition (and cleanup) Pause()'s the prior player
// then drops its reference before constructing the next, so at most one live
// player + PCM buffer is referenced; the dropped paused player is
// GC-reclaimable (oto finalizer). Player.Close() is never called (v3.4 no-op).
//
// It returns nil on a clean q/shutdown exit, or a wrapped error if a stdin read
// fails irrecoverably (a play/load error does NOT stop the loop — a failed block
// surfaces and the listener can space-replay or move on, per the
// no-retry/manual-replay stance).
func runListen(ctx context.Context, cfg listenConfig, timeline plan.Timeline) error {
	if cfg.newPlayer == nil {
		// Wiring error, not a runtime device failure: the factory must be
		// injected (by driveListen) before the loop runs.
		listenCleanup(cfg)
		return errors.New("listen player factory not configured")
	}

	nav := navigableBlocks(timeline)
	if len(nav) == 0 {
		// Nothing to play. Caller (runNarrate) fail-fasts before reaching here,
		// but guard so the loop never indexes an empty nav slice.
		listenCleanup(cfg)
		return errors.New("listen: no navigable blocks (all empty/refused/zero-duration)")
	}

	// Current-block state, owned solely by this loop goroutine.
	//   player — the live player handle (nil before the first play / after drop).
	//   paused — AUTHORITATIVE play/pause state, never inferred from oto.
	//   ended  — natural end-of-block already observed (don't re-report).
	var player listenPlayer
	paused := false
	ended := false

	// cleanup runs the load-bearing teardown order exactly once: restore tty →
	// halt+release the player (Pause() then drop the reference — NOT Close()) →
	// RemoveAll temp dir. restore is idempotent (sync.Once inside enterRaw);
	// removeAll on an already-removed dir is a harmless no-op. Deferred so a panic
	// in the loop still ends with a cooked tty.
	var cleanupOnce sync.Once
	cleanup := func() {
		cleanupOnce.Do(func() {
			cfg.restore()
			if player != nil {
				// Pause() removes the player from oto's active mux set so it stops
				// pulling; dropping the reference makes it GC-reclaimable via the
				// finalizer. Close() is a v3.4 no-op and is deliberately not called.
				player.Pause()
				player = nil
			}
			_ = cfg.removeAll(cfg.tempDir)
		})
	}
	defer cleanup()

	printLegend(cfg.out, timeline, nav)

	// play halts+releases the current player (Pause() then drop the reference —
	// bounded retention) and starts nav[pos]. It CLEARS paused and ended so a
	// pause state can never bleed across blocks. The full PCM buffer is loaded
	// BEFORE the player is constructed — no player is ever built over a
	// partially-read source. A load error is printed, not returned, and
	// constructs NO player; the loop continues (no-retry / manual-replay stance).
	play := func(pos int) {
		if player != nil {
			player.Pause()
			player = nil
		}
		paused = false
		ended = false
		blkIdx := nav[pos]
		bt := timeline.Blocks[blkIdx]
		printNowPlaying(cfg.out, blkIdx, bt)
		wavPath := filepath.Join(cfg.audioDir, bt.AudioRef)
		pcm, lerr := cfg.loadPCM(wavPath)
		if lerr != nil {
			_, _ = fmt.Fprintf(cfg.out, "  ! load block: %v — press space to replay or n/b to move\r\n", lerr)
			return
		}
		p := cfg.newPlayer(pcm)
		p.Play() // returns immediately; the engine pulls the PCM on its own goroutine.
		player = p
	}

	// Interruptible read (issue #88). A blocking cfg.readByte() is parked on a
	// stdin syscall and cannot observe a shutdownRequest, so an external
	// kill -TERM while the listener is idle at a keypress was not serviced until
	// the next byte — leaving the tty raw. The fix moves the read into a detached
	// goroutine that feeds bytes on byteCh; the loop selects over
	// {byteCh, shutdown, ctx.Done(), ticker} so an idle shutdown wins immediately
	// and runs cleanup without a keypress.
	//
	// Single-reader-of-stdin invariant: the reader reads exactly one byte per
	// grant (a token on wantByte) and PARKS on wantByte between reads — it is
	// never blocked on a stdin read unless the loop granted it. The g-target path
	// (cfg.readLine) reads stdin directly from the loop goroutine; because the
	// loop withholds the grant across that read, the reader is parked and not
	// racing the same fd. The loop maintains exactly one outstanding grant.
	//
	// The reader is detached: cleanup never joins it. On exit the loop's defer
	// cancels readerCtx; a reader parked on wantByte returns at once, and a reader
	// blocked on an in-flight cfg.readByte() returns as soon as that read yields a
	// byte/EOF (the byteCh send then loses to readerCtx.Done()).
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

	// Watch ticker: poll IsPlaying() as an end-of-block edge signal so the loop
	// stays responsive to a mid-block keypress. Stopped on exit.
	ticker := time.NewTicker(endOfBlockPollInterval)
	defer ticker.Stop()

	pos := 0
	play(pos)
	grantRead()

	for {
		select {
		case <-cfg.shutdown:
			cleanup()
			return nil
		case <-ctx.Done():
			cleanup()
			return nil
		case <-ticker.C:
			// End-of-block edge detection. paused is AUTHORITATIVE: a PAUSED
			// player also reports !IsPlaying(), so the end-of-block branch is gated
			// on !paused — a pause must NEVER be misread as block end. IsPlaying()
			// is trusted ONLY here, ONLY as a finished-edge signal, and ONLY when
			// !paused. (paused and finished are indistinguishable from oto alone.)
			if !paused && player != nil && !ended && !player.IsPlaying() {
				ended = true
				// Surface a device underrun / mid-play error if oto recorded one.
				if perr := player.Err(); perr != nil {
					_, _ = fmt.Fprintf(cfg.out, "  ! playback error: %v\r\n", perr)
				}
				// Natural end: do NOT auto-advance — wait for the next key
				// (block-level transport).
			}
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
					play(pos)
				}
				grantRead()
			case 'b':
				if pos > 0 {
					pos--
					play(pos)
				}
				grantRead()
			case ' ':
				// True Pause / Resume. Toggle the loop-owned paused flag and drive
				// oto: Pause() freezes the device read position; Play() resumes from
				// the frozen sample. A finished block (ended) or a block with no
				// live player ignores space — there is nothing to resume.
				if player != nil && !ended {
					if !paused {
						player.Pause()
						paused = true
						_, _ = fmt.Fprint(cfg.out, "  || paused\r\n")
					} else {
						player.Play()
						paused = false
						_, _ = fmt.Fprint(cfg.out, "  > resumed\r\n")
					}
				}
				grantRead()
			case 'g':
				// Read the target line directly here. The reader is parked on
				// wantByte (no grant outstanding across this read), so this is the
				// sole stdin reader — the single-reader invariant holds.
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
				play(pos)
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
// "Pause / Resume" because oto delivers true device pause.
func printLegend(w io.Writer, timeline plan.Timeline, nav []int) {
	navSet := make(map[int]bool, len(nav))
	for _, p := range nav {
		navSet[p] = true
	}
	_, _ = fmt.Fprint(w, "\r\n")
	_, _ = fmt.Fprint(w, "Listen mode — single-key transport (block-level)\r\n")
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
