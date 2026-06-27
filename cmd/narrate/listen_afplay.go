//go:build !oto

// Default (shipped) listen-path seam: afplay, block-to-completion (issue #83).
//
// This file is the //go:build !oto half of the build-tagged seam pair. It owns
// the per-block afplay playback (playBlock), the honest "Stop / Replay block"
// control legend (afplay cannot pause the device), and the driveListen entry
// the build-tag-free runListenMode (listen_run.go) calls. Its //go:build oto
// counterpart is listen_oto.go (the true-pause spike, issue #100). Exactly one
// of the two compiles per build config, so listenControlsLine, driveListen, and
// listenEnginePreflight have a single definition each.
//
// Nothing here changes the shipped behavior: the symbols below were factored
// out of listen.go verbatim so the default build (and listen_test.go, which is
// build-tag-free and so compiles against this !oto path) is byte-for-byte
// equivalent.
package main

import (
	"context"
	"fmt"
	"os/exec"
	"time"

	"github.com/vd09-projects/intelligent-tts-narration-library/plan"
)

// listenControlsLine is the honest control legend for the afplay build. afplay
// has no device pause (SIGSTOP/SIGCONT does not freeze CoreAudio), so space is
// "Stop / Replay block", never "Pause" (CLAUDE.md honesty rule).
const listenControlsLine = "n next   b back   space Stop / Replay block   g go-to   q quit"

// listenPerBlockTimeout caps a single afplay invocation on the listen path.
// Mirrors ephemeral.DefaultPerBlockTimeout intent — a single per-block clip is
// short, so a generous ceiling only matters as a backstop against a wedged
// afplay. A keypress (n/b/space/g/q) interrupts well before this elapses in
// normal use; the timeout exists for the pathological "afplay never returns"
// case.
const listenPerBlockTimeout = 60 * time.Second

// listenWaitDelay is the grace window handed to exec.Cmd.WaitDelay. After the
// call context is cancelled (keypress interrupt, timeout, or shutdown), the
// os/exec machinery sends its own interrupt; if the child has not exited within
// WaitDelay it is hard-killed (SIGKILL) and the I/O pipes are force-closed so
// cmd.Wait() can never block forever. This replaces the hand-rolled
// kill-grace timer + drain-the-done-chan dance in sink/ephemeral
// (decision 2026-06-25-listen-transport-keypress-loop-not-tui mandates
// exec.Cmd.WaitDelay over hand-rolled grace). 200ms is comfortably longer than
// afplay's observed teardown.
const listenWaitDelay = 200 * time.Millisecond

// listenEnginePreflight is the afplay engine's pre-loop fail-fast: afplay must
// exist on PATH (macOS phase one). A missing binary is a clean pre-loop refusal,
// not a mid-playback surprise. The oto build's preflight is a no-op (its engine
// failure surfaces from oto.NewContext instead).
func listenEnginePreflight() error {
	if _, err := exec.LookPath(listenAfplayBinary); err != nil {
		return fmt.Errorf("listen: %q not found on PATH (macOS afplay required, phase one): %w", listenAfplayBinary, err)
	}
	return nil
}

// driveListen is the engine-specific entry the build-tag-free runListenMode
// calls after render + raw-mode + signal wiring. The afplay seam wires playBlock
// into the shared runListen transport loop (block-to-completion: the loop parks
// in playBlock for the whole block, keypresses serviced at block boundaries).
func driveListen(ctx context.Context, cfg listenConfig, timeline plan.Timeline) error {
	cfg.play = playBlock
	return runListen(ctx, cfg, timeline)
}

// playBlock spawns `afplay <wavPath>` and waits for it under ctx, mirroring the
// reaping discipline of sink/ephemeral.playWithAfplay but using
// exec.Cmd.WaitDelay instead of a hand-rolled kill-grace timer + done-chan
// drain.
//
// Returns the started *exec.Cmd so the caller owns the handle for an explicit
// kill+Wait on the next transition (reap-before-spawn). The returned cmd has
// already been Wait()ed by the time this function returns nil/err in the
// non-cancelled case; on ctx cancel the returned cmd may still be terminating,
// and the caller's kill+Wait is the authoritative reap.
//
// Behavior:
//   - ctx cancelled before/while playing → returns ctx.Err() (wrapped). The
//     os/exec WaitDelay backstop guarantees the child is SIGKILLed and reaped.
//   - afplay exits non-zero on its own → wrapped exec error.
//   - clean exit → nil.
func playBlock(ctx context.Context, binary, wavPath string) (*exec.Cmd, error) {
	callCtx, cancel := context.WithTimeout(ctx, listenPerBlockTimeout)
	defer cancel()

	cmd := exec.CommandContext(callCtx, binary, wavPath)
	// WaitDelay: once callCtx is cancelled, exec sends its interrupt; if the
	// child has not exited within listenWaitDelay, exec force-kills it and
	// closes the I/O so cmd.Wait() returns. This is the decision-mandated
	// replacement for the ephemeral hand-rolled grace timer.
	cmd.WaitDelay = listenWaitDelay

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start afplay: %w", err)
	}

	// Buffered so the wait goroutine cannot leak if we return on ctx.Done()
	// before Wait() returns. The WaitDelay backstop bounds how long Wait()
	// can block after cancellation.
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()

	select {
	case err := <-done:
		if cerr := callCtx.Err(); cerr != nil {
			// Cancelled/timed out: surface the ctx cause, not the
			// "signal: killed" exec wrapper.
			return cmd, cerr
		}
		if err != nil {
			return cmd, fmt.Errorf("afplay: %w", err)
		}
		return cmd, nil
	case <-callCtx.Done():
		// Cancellation requested. WaitDelay guarantees the child is reaped;
		// drain done so the wait goroutine exits, then surface the ctx cause.
		<-done
		return cmd, callCtx.Err()
	}
}
