// Package render defines the Renderer interface — phase one of the four edges
// (design doc §3.3). Concrete engine backends live in subpackages (sherpa).
//
// Invariants (CLAUDE.md):
//   - render/ imports plan/ + its own subpackages only. NOT planner/.
//   - Refusal is data, not an error. Refused blocks still get audio of their
//     Refusal.Message; only adapter-style I/O failures return Go errors.
//   - Block-level sync only. No word-level / per-segment timing.
//   - Plan stays engine-neutral and pre-render — Timeline (with offsets) is
//     the renderer's output, not part of the plan it consumed.
//
// Phase one's default backend is render/sherpa, which shells out to the
// scripts/kokoro wrapper (kokoro-onnx 0.5.0). In-process CGo bindings
// (sherpa-onnx-go) are deferred.
package render

import (
	"context"
	"time"

	"github.com/vd09-projects/intelligent-tts-narration-library/plan"
)

// Renderer turns a NarrationPlan into block-level audio + a Timeline.
//
// Render produces one audio file per Block (refused blocks included) and a
// Timeline whose BlockTiming rows align 1:1 with plan.Blocks in plan order.
//
// RenderBlock re-renders a single block — the escalation path. The caller
// patches the new BlockTiming + audio file into an existing RenderResult;
// other blocks stay untouched.
//
// Errors stop the pipeline (missing binary, subprocess crash, timeout,
// malformed plan). Refusals do not — they are data carried through as audio
// of Refusal.Message plus a BlockTiming.
type Renderer interface {
	Render(ctx context.Context, p plan.NarrationPlan, opts RenderOptions) (RenderResult, error)
	RenderBlock(ctx context.Context, p plan.NarrationPlan, blockID string, opts RenderOptions) (BlockRender, error)
}

// RenderOptions — caller-tunable knobs for a Render or RenderBlock call.
//
// Voice resolution order (the renderer applies this; callers do not):
//  1. RenderOptions.Voice (engine voice id, e.g. "af_bella")
//  2. plan.NarrationPlan.Defaults.Voice (engine-neutral hint)
//  3. backend default ("af_bella" for the sherpa/Kokoro backend)
//
// Unknown PlanDefaults.Voice strings fall back to (3) silently — never error.
//
// Timeouts are derived ctx deadlines, not absolute clocks. Zero values mean
// "use backend defaults": phase-one sherpa uses PerBlockTimeout=60 s,
// WallClockTimeout=10 min. WallClockTimeout is ignored by RenderBlock.
type RenderOptions struct {
	// OutDir is the directory where per-block WAV files are written.
	// Required for Render. Required for RenderBlock unless the backend
	// documents an in-memory single-block path (sherpa requires it).
	OutDir string

	// Voice optionally overrides plan.Defaults.Voice. Empty means use the
	// fallback chain (PlanDefaults → backend default).
	Voice string

	// PerBlockTimeout caps a single subprocess invocation. Zero → backend default.
	PerBlockTimeout time.Duration

	// WallClockTimeout caps the whole Render call. Zero → backend default.
	// Ignored by RenderBlock.
	WallClockTimeout time.Duration
}

// RenderResult — what Render returns to the pipeline / sink.
//
// Audio is an on-disk handle; the sink copies, streams, or concatenates as
// it sees fit. The renderer does not hold WAV bytes in memory.
//
// Timeline.Blocks aligns 1:1 with plan.Blocks in plan order; refused blocks
// included.
//
// Format describes the audio the timeline points into — phase-one Kokoro is
// 24 kHz mono PCM s16le WAV (see DefaultFormat).
type RenderResult struct {
	Audio    AudioStream      `json:"audio"`
	Timeline plan.Timeline    `json:"timeline"`
	Format   plan.AudioFormat `json:"format"`
}

// BlockRender — what RenderBlock returns. The caller patches Timing into the
// existing Timeline (recomputing downstream block offsets if needed) and
// swaps Audio.Files[0] for the old WAV.
type BlockRender struct {
	BlockID string           `json:"block_id"`
	Audio   AudioStream      `json:"audio"`
	Timing  plan.BlockTiming `json:"timing"`
	Format  plan.AudioFormat `json:"format"`
}

// AudioStream — opaque-ish handle to the on-disk WAVs the renderer produced.
//
// Dir is an absolute path. Files are filenames (not paths) relative to Dir,
// ordered to match Timeline.Blocks. ManifestPath, when set, is the sink-
// readable list of (order, block_id, relpath) the renderer wrote; sinks
// that prefer to read the directory directly may ignore it.
//
// The renderer never reads back the bytes it just wrote — bytes belong to
// the sink.
type AudioStream struct {
	Dir          string   `json:"dir"`
	Files        []string `json:"files"`
	ManifestPath string   `json:"manifest_path,omitempty"`
}

// DefaultFormat is the phase-one Kokoro format: 24 kHz mono PCM s16le WAV.
// Backends should populate RenderResult.Format / BlockRender.Format with
// this (or their actual format if they ever diverge).
func DefaultFormat() plan.AudioFormat {
	return plan.AudioFormat{
		SampleRate: 24000,
		Channels:   1,
		Encoding:   "pcm_s16le",
	}
}
