// Package ephemeral is the phase-one local-playback OutputSink.
//
// It implements sink.OutputSink by shelling out to `afplay` (the macOS
// system audio player) once per non-empty block in res.Timeline.Blocks.
// Phase one is macOS-only by design — cross-platform playback is deferred
// (CLAUDE.md "Out of scope phase one").
//
// Invariants:
//   - Imports plan/ + render/ + sink/ + stdlib only. No planner/, no
//     adapter/, no intelligence/.
//   - Refused blocks are played the same as voiced blocks — the renderer
//     has already produced WAV audio of Refusal.Message; this sink does
//     not inspect Block.Status.
//   - Blocks with empty AudioRef are skipped (no subprocess, no file read);
//     their planned duration (zero in practice for all-pause blocks) still
//     contributes to SinkReceipt.TotalDurationMs.
//   - TotalDurationMs is summed from BlockTiming.EndMs - StartMs, NOT from
//     afplay wall-clock time.
//   - Context cancellation propagates: an in-flight afplay call is killed
//     and Consume returns ctx.Err() with a partial receipt.
package ephemeral

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/vd09-projects/intelligent-tts-narration-library/plan"
	"github.com/vd09-projects/intelligent-tts-narration-library/render"
	"github.com/vd09-projects/intelligent-tts-narration-library/sink"
)

// defaultAfplayPath is the binary resolved from PATH when AfplayPath is empty.
const defaultAfplayPath = "afplay"

// DefaultPerBlockTimeout caps a single afplay invocation when a Sink is
// constructed with a zero or negative PerBlockTimeout. Exported so callers
// tuning their own ephemeral sinks (longer clips, slower devices) can derive
// from the package default instead of hard-coding 60s.
const DefaultPerBlockTimeout = 60 * time.Second

// killGrace is how long playWithAfplay waits, after the context is cancelled,
// for cmd.Wait() to return before issuing a backstop SIGKILL.
//
// Why a grace window at all: exec.CommandContext already wires the context to
// the process — when callCtx is cancelled it sends its OWN SIGKILL and reaps
// the child asynchronously. killGrace is the time we give that built-in kill
// to land before we redundantly Kill() the process ourselves. In the common
// case the child is reaped within the window and the backstop never fires;
// the explicit Kill() exists only for the pathological case where the
// built-in kill is slow or the process ignores it. Either way we then drain
// the wait channel, so the wait goroutine is guaranteed to exit (no leak).
// 200ms is comfortably longer than afplay's observed teardown.
const killGrace = 200 * time.Millisecond

// Sink is the concrete sink.OutputSink that plays each block's WAV
// through afplay. Zero value is usable — fields take effect only when
// set. Use New(opts...) for the recommended construction path.
type Sink struct {
	// AfplayPath overrides the executable name resolved from PATH.
	// Empty → "afplay".
	AfplayPath string

	// PerBlockTimeout caps a single afplay invocation. Zero → 60 s.
	PerBlockTimeout time.Duration
}

// Compile-time assertion: Sink implements sink.OutputSink.
var _ sink.OutputSink = (*Sink)(nil)

// Option — functional config for New.
type Option func(*Sink)

// WithAfplayPath overrides the afplay binary location (useful for tests
// or non-standard installs).
func WithAfplayPath(path string) Option {
	return func(s *Sink) { s.AfplayPath = path }
}

// WithPerBlockTimeout sets the per-block subprocess timeout. Zero or
// negative values are treated as "use default".
func WithPerBlockTimeout(d time.Duration) Option {
	return func(s *Sink) { s.PerBlockTimeout = d }
}

// New constructs a Sink with the supplied options applied over the
// zero value. Both New() and Sink{} produce a functioning sink; the
// zero-value path is preserved deliberately for callers wiring through
// struct literals.
func New(opts ...Option) *Sink {
	s := &Sink{}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// playFunc is the shape of the per-block playback seam. The package-level
// var `play` is swapped by tests to avoid touching real audio hardware.
type playFunc func(ctx context.Context, binary, path string, timeout time.Duration) error

// play is the package-level seam: production calls playWithAfplay; tests
// overwrite this var to inject deterministic behavior (success, error,
// blocking-until-cancel). The seam is package-level (not a Sink field)
// to keep the public Sink struct minimal and the zero-value path clean.
//
// Concurrency: this seam is NOT safe for parallel use. Consume invokes it
// sequentially (one block at a time), and because the seam is a package-level
// var, a test that swaps it races any other goroutine running Consume. Tests
// that swap play must not run in parallel (see withStubPlay), and callers must
// not run two Consume calls concurrently across a swap. Production playback is
// inherently serial — afplay drives the single local audio device — so this
// constraint costs nothing at runtime.
var play playFunc = playWithAfplay

// Consume plays each block in res.Timeline.Blocks in plan order through
// afplay. Empty-AudioRef blocks are skipped (the renderer marks all-pause
// blocks that way per review-findings B2); their planned duration still
// contributes to the receipt.
//
// On afplay error the receipt reports state up to (and not including) the
// failed block, and the wrapped error names the offending block id. On
// ctx cancel the receipt similarly reports the partial state.
func (s *Sink) Consume(ctx context.Context, _ plan.NarrationPlan, res render.RenderResult) (sink.SinkReceipt, error) {
	binary := s.AfplayPath
	if binary == "" {
		binary = defaultAfplayPath
	}
	perBlock := s.PerBlockTimeout
	if perBlock <= 0 {
		perBlock = DefaultPerBlockTimeout
	}

	var receipt sink.SinkReceipt
	for _, blk := range res.Timeline.Blocks {
		// Short-circuit on already-cancelled context before doing any
		// per-block work; honors the "ctx cancel propagates" invariant
		// for the edge case where the caller cancelled between blocks.
		if err := ctx.Err(); err != nil {
			return receipt, err
		}

		planned := int64(blk.EndMs - blk.StartMs)

		if blk.AudioRef == "" {
			// All-pause / no-speech block. No subprocess. Plan duration
			// (zero in practice) still counts toward the receipt so the
			// receipt mirrors plan truth.
			receipt.TotalDurationMs += planned
			continue
		}

		// Defensive guard: a non-empty AudioRef with no Audio.Dir means the
		// renderer's "Dir holds the per-block WAVs" invariant was violated
		// upstream (mis-wired pipeline). filepath.Join would silently produce
		// a relative path and afplay would fail with an opaque error; fail
		// loud and early instead. This is an error, not a refusal — refusals
		// are for readable-but-unvoiceable source, not backend mis-wiring.
		if res.Audio.Dir == "" {
			return receipt, fmt.Errorf("sink/ephemeral: empty Audio.Dir with AudioRef %q for block %s", blk.AudioRef, blk.BlockID)
		}

		path := filepath.Join(res.Audio.Dir, blk.AudioRef)
		if err := play(ctx, binary, path, perBlock); err != nil {
			// Receipt reflects partial state — callers can inspect how
			// far playback got before the failure / cancellation.
			if ctxErr := ctx.Err(); ctxErr != nil {
				return receipt, ctxErr
			}
			return receipt, fmt.Errorf("sink/ephemeral: afplay block %s: %w", blk.BlockID, err)
		}
		receipt.TotalDurationMs += planned
		receipt.BlocksPlayed++
	}
	return receipt, nil
}

// playWithAfplay runs `afplay <path>` under a per-block deadline. It
// returns nil on a clean exit, ctx.Err() if the parent context cancels,
// or a wrapped exec error otherwise.
//
// Ctx handling pattern (mirrors render/sherpa intent):
//  1. Wrap parent ctx with the per-block timeout — the timeout binds
//     the subprocess but does NOT shadow parent cancellation, because
//     callCtx inherits Done from parent.
//  2. Spawn cmd via exec.CommandContext(callCtx, ...). Start in-process
//     so we own the Process handle for the fallback kill.
//  3. cmd.Wait in a goroutine, result onto a buffered chan (size 1) so
//     the goroutine cannot leak even if we return early on ctx.Done.
//  4. On ctx.Done before wait completes, give cmd up to killGrace to
//     exit on its own (exec.CommandContext's own SIGKILL), then send
//     a hard Kill() as a backstop.
func playWithAfplay(ctx context.Context, binary, path string, timeout time.Duration) error {
	callCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(callCtx, binary, path)
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start afplay: %w", err)
	}

	// Buffered channel (size 1) prevents the wait goroutine from leaking
	// if we return on ctx.Done() before cmd.Wait() returns.
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()

	select {
	case err := <-done:
		// Distinguish "parent cancelled / timed out" from "afplay exited
		// non-zero on its own". exec.CommandContext wraps the killed
		// process's error; surfacing ctx.Err() is more useful to callers.
		if cerr := callCtx.Err(); cerr != nil {
			return cerr
		}
		if err != nil {
			return fmt.Errorf("afplay: %w", err)
		}
		return nil
	case <-callCtx.Done():
		// Give exec.CommandContext's own SIGKILL a moment to land; if it
		// hasn't been reaped by killGrace, kill explicitly. Either way,
		// drain done so the goroutine exits cleanly, capturing the wait
		// error so we can join it onto the ctx error below.
		var waitErr error
		select {
		case waitErr = <-done:
			// Already reaped.
		case <-time.After(killGrace):
			if cmd.Process != nil {
				_ = cmd.Process.Kill()
			}
			waitErr = <-done
		}
		// Join the cancellation cause with the process's own exit error
		// (typically "signal: killed") instead of discarding the latter, so
		// callers can see BOTH why we stopped (ctx) and how the child died.
		// errors.Join keeps errors.Is(err, context.Canceled / DeadlineExceeded)
		// matching, and drops waitErr when it is nil (clean reap).
		return errors.Join(callCtx.Err(), waitErr)
	}
}
