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

	// observer is the optional per-block emit seam (issue #81, ADR #77 D5
	// Channel 2). Nil = off (no-op). When set, Consume calls it once per
	// timeline block, in plan order, BEFORE that block's blocking play()
	// (and once with Playing:false for an empty-AudioRef block) — so a
	// decoupled user-launched observer can show live, during-playback
	// progress the after-the-fact Channel-1 receipt structurally cannot.
	// Set via WithBlockObserver.
	observer BlockObserver
}

// BlockObserver is the shape of the per-block progress emit seam (issue
// #81). Named (mirroring the playFunc idiom) so the Option signature reads
// clearly and nil-checks have a name. Implementations must be cheap and
// non-blocking: Consume calls it inline on the playback path, fire-and-
// forget — it never returns an error and never alters control flow.
type BlockObserver func(BlockProgress)

// BlockProgress is the per-block event the sink hands a BlockObserver. It
// carries ONLY what Consume holds in its loop: the current BlockTiming, the
// block's Level + Status (read from the plan.NarrationPlan Consume already
// receives — correlated 1:1 by BlockID, since render emits one BlockTiming
// per top-level Block in plan order; see render.go), the 1-based Order and
// Total, and Playing.
//
// Playing means strictly "this block has rendered audio being sent to
// afplay" = Timing.AudioRef != "". It is INDEPENDENT of Status and derived
// only from the timeline, so it reports what the renderer actually produced:
//   - voiced / degraded block → has a WAV → Playing:true.
//   - refused block → the renderer voiced its Refusal.Message into a WAV
//     (see render/sherpa spokenTextFor), so a refused block normally
//     Playing:true too — Status (refused) carries the honesty signal, not
//     Playing.
//   - all-pause / empty-voiced block → no WAV (AudioRef "") → Playing:false.
//
// Carries NO source or spoken text BY DESIGN (issue #81 / CLAUDE.md
// "local-only means secrets get read aloud") — only structural metadata, so
// the Channel-2 side channel never widens the secret-leak surface.
type BlockProgress struct {
	Timing  plan.BlockTiming
	Level   plan.Level
	Status  plan.Status
	Order   int // 1-based position in the timeline.
	Total   int // len(timeline.Blocks).
	Playing bool
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

// WithBlockObserver installs the per-block progress emit seam (issue #81,
// ADR #77 D5 Channel 2). A nil observer leaves the feature off (the Consume
// path is byte-for-byte unchanged), mirroring the play-seam idiom. The
// concrete observer — JSONL marshaling, scratch-file lifecycle — lives at
// the composition root (cmd/narrate-mcp), not here: the sink stays import-
// clean (plan + render + sink + stdlib) and knows nothing of Channel 2.
func WithBlockObserver(o BlockObserver) Option {
	return func(s *Sink) { s.observer = o }
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
func (s *Sink) Consume(ctx context.Context, p plan.NarrationPlan, res render.RenderResult) (sink.SinkReceipt, error) {
	binary := s.AfplayPath
	if binary == "" {
		binary = defaultAfplayPath
	}
	perBlock := s.PerBlockTimeout
	if perBlock <= 0 {
		perBlock = DefaultPerBlockTimeout
	}

	// Level/Status lookup for the observer seam — built only when an
	// observer is wired (off-path stays allocation-free and byte-identical).
	// Keyed by Block.ID, which BlockTiming.BlockID matches 1:1 in plan order
	// (render emits one timing row per top-level Block; see render.go). The
	// plan param was historically discarded here; the observer is the first
	// consumer that needs its Level/Status.
	var meta map[string]plan.Block
	if s.observer != nil {
		meta = make(map[string]plan.Block, len(p.Blocks))
		for _, b := range p.Blocks {
			meta[b.ID] = b
		}
	}
	total := len(res.Timeline.Blocks)

	var receipt sink.SinkReceipt
	for i, blk := range res.Timeline.Blocks {
		// Short-circuit on already-cancelled context before doing any
		// per-block work; honors the "ctx cancel propagates" invariant
		// for the edge case where the caller cancelled between blocks.
		if err := ctx.Err(); err != nil {
			return receipt, err
		}

		planned := int64(blk.EndMs - blk.StartMs)

		// Live Channel-2 emit (issue #81): once per block, in plan order,
		// BEFORE the blocking play()/skip below — fire-and-forget, nil-safe,
		// never alters the receipt or control flow. A missing meta entry
		// (BlockTiming with no matching plan Block — should not happen given
		// the 1:1 invariant) emits zero Level/Status rather than fabricating.
		if s.observer != nil {
			b := meta[blk.BlockID]
			s.observer(BlockProgress{
				Timing:  blk,
				Level:   b.Level,
				Status:  b.Status,
				Order:   i + 1,
				Total:   total,
				Playing: blk.AudioRef != "",
			})
		}

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
