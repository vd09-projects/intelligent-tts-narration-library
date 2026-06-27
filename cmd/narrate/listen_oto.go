//go:build oto

// True-pause listen-path spike: in-process oto v3 player (issue #100).
//
// This file is the //go:build oto half of the build-tagged seam pair (its
// //go:build !oto counterpart is listen_afplay.go). It is a THROWAWAY-GRADE
// SPIKE — it proves, by ear, that github.com/ebitengine/oto/v3 can sit in the
// cmd/narrate listen path, consume real Kokoro 24 kHz mono int16 PCM through an
// io.Reader, and deliver true device-level Pause/Resume (audio freezes at the
// sample where space was pressed and resumes from that exact sample). #101
// productionizes (collapses the two build paths, adds io.Seeker block seeking,
// in-memory piping). Build/run it with `-tags oto` (see Makefile spike-oto-listen).
//
// Why a separate loop (runListenOto) rather than reusing runListen: the afplay
// seam is BLOCK-TO-COMPLETION — playBlock parks the loop goroutine for the whole
// block, so keypresses are serviced only at block boundaries (verified in
// source, plan review blocker 1). True pause needs the loop to read a space
// keypress WHILE audio is still playing, which is structurally impossible under
// that seam. So oto gets a Play()-then-watch-IsPlaying() lifecycle: Play()
// returns immediately (oto pulls the reader on its own goroutine) and the loop
// selects over {byteCh, shutdown, ctx.Done(), ticker} — the ticker polls
// IsPlaying() only as an end-of-block edge signal; the AUTHORITATIVE pause state
// is the loop-owned `paused` flag, never inferred from oto.
//
// The shared transport scaffolding (navigation math, raw mode, signal handling,
// legend/now-playing printers, parseBlockIndex, the listenConfig seam) is reused
// unchanged from the build-tag-free listen.go.
//
// macOS phase one — purego/CoreAudio path, no CGo.
package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/ebitengine/oto/v3"

	"github.com/vd09-projects/intelligent-tts-narration-library/plan"
)

// listenControlsLine is the honest control legend for the oto build: space is
// "Pause / Resume" because oto delivers true device-level pause (decision
// 2026-06-27-true-pause-via-oto-v3: device-confirmed pause delta 0 bytes). The
// afplay build's sibling constant says "Stop / Replay block" instead.
const listenControlsLine = "n next   b back   space Pause / Resume   g go-to   q quit"

// oto context audio format — Kokoro native: 24 kHz mono int16, NO resampling at
// our layer (CLAUDE.md). oto handles device resampling internally; the by-ear
// /verify gate confirms that path sounds acceptable (acceptance criterion 3).
const (
	otoSampleRate   = 24000
	otoChannelCount = 1
)

// otoWatchTick is how often the loop polls player.IsPlaying() to detect a
// natural end-of-block. ~50 ms keeps pause feel immediate without busy-spinning
// (the exact interval is not load-bearing — plan open question).
const otoWatchTick = 50 * time.Millisecond

// listenEnginePreflight is a no-op for the oto build: there is no external
// binary to look up (afplay's sibling preflight does that). The engine's real
// failure mode — no audio device / context construction failure — surfaces from
// oto.NewContext in driveListen, where it is checked and surfaced honestly.
func listenEnginePreflight() error { return nil }

// driveListen is the oto engine entry the build-tag-free runListenMode calls. It
// constructs the single process-wide oto.Context (oto permits exactly one),
// surfaces a construction error instead of nil-dereferencing a failed context,
// waits on the ready channel, then hands off to the watch loop.
func driveListen(ctx context.Context, cfg listenConfig, timeline plan.Timeline) error {
	otoCtx, ready, err := oto.NewContext(&oto.NewContextOptions{
		SampleRate:   otoSampleRate,
		ChannelCount: otoChannelCount,
		Format:       oto.FormatSignedInt16LE,
	})
	if err != nil {
		// Honest failure: tear down the tty + temp dir the same way the loop's
		// cleanup would, then surface the error up the pipeline (it stops; this
		// is an engine error, not a refusal).
		cfg.restore()
		_ = cfg.removeAll(cfg.tempDir)
		return fmt.Errorf("listen(oto): new audio context: %w", err)
	}
	<-ready // block until the device is ready for the first Play().
	return runListenOto(ctx, otoCtx, cfg, timeline)
}

// otoBlock is the loop's private handle to the current block's player + the
// backing WAV file. The loop goroutine is its sole owner (mirrors the afplay
// single-owner model: the signal handler only requests shutdown over a channel).
type otoBlock struct {
	player *oto.Player
	file   *os.File
	ended  bool // natural end-of-block already observed (don't re-report).
	closed bool // idempotency guard for close().
}

// close halts playback FIRST, THEN closes the underlying os.File — so oto can
// never read a closed descriptor. Idempotent; only the loop goroutine calls it.
// At most one player + one open file is alive at a time.
//
// SPIKE FINDING (deviates from the plan's stated teardown): the plan said
// "player.Close() FIRST (stops oto's goroutine pulling the reader), THEN
// file.Close()". But oto v3.4 turned Player.Close() into a DOCUMENTED NO-OP —
// "Close does nothing and always returns nil" — because teardown is now
// finalizer/GC-driven (mux.Player.Close runs from a runtime finalizer). So
// Close() does NOT stop the read-pull, and calling it is deprecated (SA1019).
// Pause() is the v3.4-correct way to deterministically halt device consumption
// before we drop our reference and close the fd. A paused player does not pull,
// so closing the file here is safe by ear for the spike.
//
// TODO(#101-followup): redesign teardown around oto v3.4's finalizer lifecycle —
// the old player only fully stops when GC runs its finalizer, so the production
// player must either hand oto an io.ReadCloser it owns (close-on-finalize) or
// retain the reader until the player is collected, instead of closing the fd
// out from under a not-yet-finalized player. What/where/why: the fixed
// "Close-then-close-fd" ordering no longer guarantees the fd outlives the
// reader; this is a productionization (#101) concern, out of scope for the spike.
func (b *otoBlock) close() {
	if b == nil || b.closed {
		return
	}
	b.closed = true
	if b.player != nil {
		// Deterministically stop playback (v3.4: Close is a no-op; Pause is the
		// real halt). Dropping block here makes the player GC-eligible; its
		// finalizer performs the underlying mux teardown.
		b.player.Pause()
	}
	if b.file != nil {
		_ = b.file.Close()
	}
}

// runListenOto is the oto keypress transport loop — the Play()-then-watch
// counterpart of runListen. Same navigation semantics (n/b/g/q), same
// single-reader-of-stdin model, same teardown ordering, but space toggles true
// device Pause/Resume instead of Stop/Replay, and a ticker watches IsPlaying()
// so the loop stays responsive mid-block.
func runListenOto(ctx context.Context, otoCtx *oto.Context, cfg listenConfig, timeline plan.Timeline) error {
	nav := navigableBlocks(timeline)
	if len(nav) == 0 {
		cfg.restore()
		_ = cfg.removeAll(cfg.tempDir)
		return errors.New("listen: no navigable blocks (all empty/refused/zero-duration)")
	}

	block := &otoBlock{}
	// paused is the AUTHORITATIVE play/pause state, owned solely by this loop
	// goroutine. Never inferred from oto (a paused player and a finished player
	// both report !IsPlaying()).
	paused := false

	var cleanupOnce sync.Once
	cleanup := func() {
		cleanupOnce.Do(func() {
			cfg.restore()
			block.close()
			_ = cfg.removeAll(cfg.tempDir)
		})
	}
	defer cleanup()

	printLegend(cfg.out, timeline, nav)

	// play tears down the current block (Pause then file.Close — see close) and
	// starts nav[pos]. It CLEARS paused so a pause state can never bleed across
	// blocks (review blocker 2). A play error is printed, not returned — the
	// loop continues (no-retry / manual-replay stance, same as the afplay seam).
	play := func(pos int) {
		block.close()
		block = &otoBlock{}
		paused = false
		blkIdx := nav[pos]
		bt := timeline.Blocks[blkIdx]
		printNowPlaying(cfg.out, blkIdx, bt)
		wavPath := filepath.Join(cfg.audioDir, bt.AudioRef)
		f, oerr := os.Open(wavPath) //nolint:gosec // spike: path is a renderer-produced temp WAV.
		if oerr != nil {
			_, _ = fmt.Fprintf(cfg.out, "  ! open block: %v — press space/n/b\r\n", oerr)
			return
		}
		r, perr := newPCMReader(f)
		if perr != nil {
			_, _ = fmt.Fprintf(cfg.out, "  ! parse wav: %v — press n/b\r\n", perr)
			_ = f.Close()
			return
		}
		p := otoCtx.NewPlayer(r)
		p.Play() // returns immediately; oto pulls the reader on its own goroutine.
		block.player = p
		block.file = f
	}

	// Reader goroutine — identical single-reader-of-stdin model to runListen
	// (issue #88): one byte per grant, parks on wantByte between reads, detached
	// (cleanup never joins it). The g-target readLine is read from this loop
	// goroutine while no grant is outstanding, so it never races the reader.
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
	grantRead := func() {
		select {
		case wantByte <- struct{}{}:
		default:
		}
	}

	// Watch ticker: the loop polls IsPlaying() to detect natural end-of-block so
	// it can stay responsive to a mid-block keypress (the afplay seam blocks to
	// completion instead — review blocker 1). Stopped on exit.
	ticker := time.NewTicker(otoWatchTick)
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
			// End-of-block edge detection. A PAUSED player also reports
			// !IsPlaying(), so the end-of-block branch is gated on !paused — a
			// pause must never be misread as block end (plan risk: mis-gated
			// watch loop). IsPlaying() is used ONLY as an edge signal here.
			if !paused && block.player != nil && !block.ended && !block.player.IsPlaying() {
				block.ended = true
				if perr := block.player.Err(); perr != nil {
					_, _ = fmt.Fprintf(cfg.out, "  ! playback error: %v\r\n", perr)
				}
				// Natural end: like the afplay seam, do NOT auto-advance — wait
				// for the next key (block-level transport).
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
				// True Pause / Resume. Toggle the loop-owned paused flag and
				// drive oto: Pause() freezes the device read position (decision
				// device-confirmed delta 0 bytes); Play() resumes from the frozen
				// sample. A finished block (ended) ignores space — there is
				// nothing to resume.
				if block.player != nil && !block.ended {
					if !paused {
						block.player.Pause()
						paused = true
						_, _ = fmt.Fprint(cfg.out, "  || paused\r\n")
					} else {
						block.player.Play()
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
				// Unknown key — ignore (forgiving in raw mode where arrow keys
				// arrive as escape sequences).
				grantRead()
			}
		}
	}
}
