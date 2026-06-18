// Package pipeline is the composition root — the only place that knows
// concrete adapter / intelligence / renderer / sink implementations.
//
// Invariants (CLAUDE.md):
//   - Only pipeline/ and cmd/ import concrete edges. planner/ stays I/O-free.
//   - Refusal is data, not error. Adapter I/O failure IS an error and stops
//     the pipeline. Refused blocks arrive at the sink looking just like
//     voiced blocks (the renderer already produced audio of Refusal.Message);
//     Narrate returns nil error with a valid SinkReceipt.
//   - One narration-plan format everywhere. Pipeline forwards plan unchanged
//     adapter → planner → renderer → sink.
//   - Block-level sync only.
//
// Decision (v1) — architecture: accepted. Pipeline is the only struct that
// holds concrete edge instances. Narrate is the only public method. The
// ctor takes interfaces, not concrete types, so cmd/narrate (issue #7) and
// the future MCP server (phase 4) reuse this struct without duplication.
package pipeline

import (
	"context"
	"fmt"

	"github.com/vd09-projects/intelligent-tts-narration-library/adapter"
	"github.com/vd09-projects/intelligent-tts-narration-library/intelligence"
	"github.com/vd09-projects/intelligent-tts-narration-library/plan"
	"github.com/vd09-projects/intelligent-tts-narration-library/planner"
	"github.com/vd09-projects/intelligent-tts-narration-library/render"
	"github.com/vd09-projects/intelligent-tts-narration-library/sink"
)

// Pipeline wires the four edges into a single Narrate call.
//
// Intelligence may be nil — the planner takes the deterministic + degraded
// path in that case (phase one default).
//
// Defaults supplies the planner Request and the engine-neutral PlanDefaults
// the renderer reads as a voice hint. Voice overrides flow through
// NarrateRequest.Voice (engine voice id, e.g. "af_bella").
type Pipeline struct {
	Adapter      adapter.InputAdapter
	Intelligence intelligence.IntelligenceAdapter
	Renderer     render.Renderer
	Sink         sink.OutputSink
	Defaults     PipelineDefaults
}

// PipelineDefaults — the document-wide defaults the pipeline owns.
//
// Level is the planner's per-block target unless overridden in
// NarrateRequest.LevelOverrides.
// OutDir is required — the renderer writes per-block WAVs there.
// Locale is recorded into plan.PlanDefaults.Locale; phase one is always "en".
type PipelineDefaults struct {
	Level  plan.Level
	OutDir string
	Locale string
}

// NarrateRequest — per-call knobs.
//
// Voice is an optional engine voice id (e.g. "af_bella", "am_michael") that
// overrides PlanDefaults.Voice. Empty means "use the renderer's resolution
// chain": opts.Voice → plan.Defaults.Voice → backend default.
//
// LevelOverrides is the block-level escalation map (design doc §4) — empty
// in the phase-one vertical slice.
type NarrateRequest struct {
	Voice          string
	LevelOverrides map[string]plan.Level
}

// New constructs a Pipeline. All four edges are required; Intelligence may
// be nil. New panics on a nil required edge — composition-root bugs are
// programmer errors, not runtime conditions.
func New(
	a adapter.InputAdapter,
	intel intelligence.IntelligenceAdapter,
	r render.Renderer,
	s sink.OutputSink,
	defaults PipelineDefaults,
) *Pipeline {
	if a == nil {
		panic("pipeline.New: nil adapter")
	}
	if r == nil {
		panic("pipeline.New: nil renderer")
	}
	if s == nil {
		panic("pipeline.New: nil sink")
	}
	if defaults.Locale == "" {
		defaults.Locale = "en"
	}
	if !defaults.Level.IsValid() {
		defaults.Level = plan.L1
	}
	return &Pipeline{
		Adapter:      a,
		Intelligence: intel,
		Renderer:     r,
		Sink:         s,
		Defaults:     defaults,
	}
}

// Narrate runs the full pipeline once for one source ref.
//
// Pipeline (design doc §3.5):
//  1. adapter.Read(ctx, ref) → RawDocument. Errors stop.
//  2. planner.Plan(ctx, doc, Request{Level, Overrides}, intel) → NarrationPlan.
//     Errors stop. Refused blocks land inside the plan as data.
//  3. renderer.Render(ctx, plan, RenderOptions{OutDir, Voice}) → RenderResult.
//     Errors stop. Refused blocks are pre-rendered audio of Refusal.Message.
//  4. sink.Consume(ctx, plan, renderResult) → SinkReceipt. Errors stop with
//     a partial receipt.
//
// Returns (SinkReceipt, nil) on the honest path — including plans containing
// refused blocks. Errors propagate from any edge.
func (p *Pipeline) Narrate(ctx context.Context, ref plan.SourceRef, req NarrateRequest) (sink.SinkReceipt, error) {
	if err := ctx.Err(); err != nil {
		return sink.SinkReceipt{}, fmt.Errorf("pipeline: %w", err)
	}

	doc, err := p.Adapter.Read(ctx, ref)
	if err != nil {
		return sink.SinkReceipt{}, fmt.Errorf("pipeline: adapter: %w", err)
	}

	narrationPlan, err := planner.Plan(ctx, doc, planner.Request{
		Level:     p.Defaults.Level,
		Overrides: req.LevelOverrides,
	}, p.Intelligence)
	if err != nil {
		return sink.SinkReceipt{}, fmt.Errorf("pipeline: planner: %w", err)
	}

	result, err := p.Renderer.Render(ctx, narrationPlan, render.RenderOptions{
		OutDir: p.Defaults.OutDir,
		Voice:  req.Voice,
	})
	if err != nil {
		return sink.SinkReceipt{}, fmt.Errorf("pipeline: renderer: %w", err)
	}

	receipt, err := p.Sink.Consume(ctx, narrationPlan, result)
	if err != nil {
		return receipt, fmt.Errorf("pipeline: sink: %w", err)
	}
	return receipt, nil
}
