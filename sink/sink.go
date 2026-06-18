// Package sink defines the OutputSink interface — phase one of the four
// edges (design doc §3.4). Concrete sinks live in subpackages (ephemeral,
// persistent). The pipeline composition root picks one and passes a
// finished RenderResult through it; the sink decides how the bytes leave
// the process (speaker, filesystem, both).
//
// Invariants (CLAUDE.md):
//   - sink/ imports plan/ + render/ + stdlib only. NOT planner/, NOT
//     intelligence/, NOT pipeline/. The render/ import is intentional —
//     RenderResult and AudioStream are the sink's input shape (both edges
//     already depend on plan/, so a sink-local mirror would be structural
//     duplication).
//   - Errors stop the pipeline (missing audio file, subprocess crash,
//     timeout, ctx cancel). Refusals do NOT — refused blocks have already
//     been rendered by render/ as audio of Refusal.Message and arrive here
//     looking identical to voiced blocks. The sink treats them the same.
//   - Empty AudioRef blocks (all-pause / no-speech, set by the renderer per
//     review-findings B2) are skipped — no subprocess call, no file read —
//     but their planned duration (EndMs - StartMs) still counts toward
//     SinkReceipt.TotalDurationMs so the receipt mirrors plan truth.
//   - Context cancellation must propagate. Sinks return ctx.Err() with a
//     partial SinkReceipt populated up to the cancellation point.
//   - SinkReceipt.TotalDurationMs is derived from timeline planned duration,
//     NOT wall-clock playback time. Reproducible from the plan + render
//     output alone; decouples the sink's truth from device decode jitter.
package sink

import (
	"context"

	"github.com/vd09-projects/intelligent-tts-narration-library/plan"
	"github.com/vd09-projects/intelligent-tts-narration-library/render"
)

// OutputSink consumes a finished plan + render output and emits the audio.
//
// Consume iterates res.Timeline.Blocks in plan order, emitting each block's
// audio (resolved against res.Audio.Dir + BlockTiming.AudioRef) to whatever
// destination the concrete sink chose. Blocks with an empty AudioRef are
// skipped without error — their planned duration still contributes to the
// returned SinkReceipt.TotalDurationMs.
//
// On any per-block I/O or subprocess failure, Consume returns a wrapped
// error and a partial SinkReceipt reporting state up to the failure point.
// Context cancellation propagates the same way: ctx.Err() plus a partial
// receipt. Refusals are not errors — refused blocks are pre-rendered audio
// of Refusal.Message and are played through the normal path.
type OutputSink interface {
	Consume(ctx context.Context, p plan.NarrationPlan, res render.RenderResult) (SinkReceipt, error)
}

// SinkReceipt — block-level accounting of a Consume call.
//
// TotalDurationMs is the sum of (EndMs - StartMs) over every block that
// was processed before Consume returned — including skipped empty-AudioRef
// blocks (their planned duration is zero in practice, but the field is
// authoritative). It is NOT measured wall-clock playback time; the sink's
// truth comes from the plan, not from the audio device.
//
// BlocksPlayed counts blocks that actually produced audio output (non-empty
// AudioRef with a successful play). Skipped empty blocks do not increment
// it. On error, BlocksPlayed reflects the count up to (and not including)
// the failed block.
type SinkReceipt struct {
	TotalDurationMs int64 `json:"total_duration_ms"`
	BlocksPlayed    int   `json:"blocks_played"`
}
