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
	"errors"
	"fmt"
	"strings"

	"github.com/vd09-projects/intelligent-tts-narration-library/adapter"
	"github.com/vd09-projects/intelligent-tts-narration-library/intelligence"
	"github.com/vd09-projects/intelligent-tts-narration-library/plan"
	"github.com/vd09-projects/intelligent-tts-narration-library/planner"
	"github.com/vd09-projects/intelligent-tts-narration-library/render"
	"github.com/vd09-projects/intelligent-tts-narration-library/sink"
)

// ErrBlockNotFound is returned by Narrate when NarrateRequest.BlockID is
// non-empty and the planner-produced plan has no Block with that ID. The
// wrapped error message always includes the requested ID so callers can
// surface it without re-deriving.
//
// Sentinel error: callers compare with errors.Is(err, pipeline.ErrBlockNotFound).
var ErrBlockNotFound = errors.New("pipeline: block not found")

// BlockSummary is one row of the block roster returned in NarrateResult.
// Populated on every successful Narrate call (both whole-doc and single-block
// paths) so callers can render a "narrate --block <id> --level 2|3" roster
// without re-planning.
//
// StartLine / EndLine come from the block's SourceMap when SourceKindLineRange
// applies; zero when the source map has no line range (e.g. screenshot OCR).
//
// SpokenText is the verbatim words this block speaks — the single-space join
// of every non-empty Segment.Text in segment order (pause / empty segments
// contribute nothing). It mirrors what the renderer voices, so a listener can
// read back what was said. Populated by summarizeBlocks (pre-render; pure).
//
// RefusalReason / RefusalMessage are non-empty only when the block was refused
// (Block.Refusal != nil). They carry the honesty-rule refusal as data onto the
// roster so a consumer can surface "skipped X because Y" without re-walking the
// plan. Empty on voiced / degraded blocks.
//
// DurationMs is the per-block planned duration (EndMs - StartMs from the
// matching BlockTiming, by BlockID). It is NOT set by summarizeBlocks — that
// pass runs before the renderer produces a Timeline. It is filled in a separate
// post-render pass (joinTimelineDurations) inside Narrate, after Render returns.
// Blocks with no timing row (e.g. refused / not rendered) stay 0.
type BlockSummary struct {
	ID             string
	Order          int
	Class          plan.Class
	Level          plan.Level
	Status         plan.Status
	StartLine      int
	EndLine        int
	SpokenText     string
	RefusalReason  plan.RefusalReason
	RefusalMessage string
	DurationMs     int
}

// BlockHashMismatch is non-nil on NarrateResult only when
// NarrateRequest.ExpectedContentHash was set AND it did not match the actual
// document content_hash the planner produced. Carried as a warning, NOT an
// error — the block content has changed since the caller obtained its ID,
// but the re-render still proceeds.
//
// Decision (v1) — pipeline-block-rerender: accepted. ExpectedContentHash is
// compared against the document-level Source.ContentHash (the only hash the
// plan carries). Block-level hashes are not part of the schema — the planner
// regenerates blocks deterministically from the document, so a document hash
// mismatch is the correct "your block id is stale" signal.
type BlockHashMismatch struct {
	BlockID  string
	Expected string
	Got      string
}

// NarrateResult is the envelope Narrate returns. Embeds the sink.SinkReceipt
// (preserving everything callers used to read directly) and adds the
// per-block roster + the optional hash-mismatch warning + the document
// content hash callers need to pass back on a later --expected-content-hash
// guard.
//
// Additive within the pipeline API: callers reading Receipt fields can do so
// via the embedded SinkReceipt without any field renames.
//
// Error contract: on a non-nil error return from Narrate, NarrateResult
// fields may be partially populated (BlockSummaries when the planner ran,
// SinkReceipt when the sink ran with a partial receipt, BlockHashMismatch
// when the hash was checked before a downstream error). Callers should
// generally honor the error first and consult result fields only when
// surfacing diagnostic context.
type NarrateResult struct {
	sink.SinkReceipt

	// BlockSummaries lists every block from the latest plan, in plan order.
	// Always populated on success.
	BlockSummaries []BlockSummary

	// DocumentContentHash is the planner-produced document content_hash
	// (mirror of NarrationPlan.Source.ContentHash). Populated on every
	// successful Narrate call — callers persist it alongside a chosen
	// BlockID and pass it back via NarrateRequest.ExpectedContentHash on
	// a later --block re-render to detect staleness.
	DocumentContentHash string

	// BlockHashMismatch is non-nil iff NarrateRequest.ExpectedContentHash was
	// set and did not match the document's content_hash. Non-fatal — see
	// the error contract on NarrateResult.
	BlockHashMismatch *BlockHashMismatch
}

// Narrator is the minimal narration surface the binaries depend on — one
// method, matching (*Pipeline).Narrate. Extracted here (issue #27) so the
// composition roots (cmd/narrate, cmd/narrate-mcp) share a single interface
// declaration instead of each maintaining a private copy that must track
// Narrate's signature in lock-step. *Pipeline is the only production
// implementation; tests inject stubs through their newPipeline seams.
//
// Lives in pipeline/ (the composition root) rather than plan/ — plan/ imports
// nothing from this project (CLAUDE.md invariant), and NarrateRequest /
// NarrateResult are pipeline-owned types anyway.
type Narrator interface {
	Narrate(ctx context.Context, ref plan.SourceRef, req NarrateRequest) (NarrateResult, error)
}

// Compile-time assertion: *Pipeline satisfies Narrator. Keeps the interface
// and the concrete method from drifting apart.
var _ Narrator = (*Pipeline)(nil)

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
//
// CodeMinLevel is a pass-through floor (issue #73, listen-mode) the
// pipeline forwards verbatim into planner.Request.CodeMinLevel. When set
// to a valid level, code blocks resolve to at least that level (a floor,
// not a set — an explicit L3 request survives). Zero/invalid means "no
// floor", preserving today's behavior for every caller that does not set
// it. The composition root (cmd/narrate-mcp) sets it; the pipeline only
// relays it — it is engine-neutral and never persisted to plan.PlanDefaults.
type PipelineDefaults struct {
	Level        plan.Level
	OutDir       string
	Locale       string
	CodeMinLevel plan.Level
}

// NarrateRequest — per-call knobs.
//
// Voice is an optional engine voice id (e.g. "af_bella", "am_michael") that
// overrides PlanDefaults.Voice. Empty means "use the renderer's resolution
// chain": opts.Voice → plan.Defaults.Voice → backend default.
//
// LevelOverrides is the block-level escalation map (design doc §4) — empty
// in the phase-one vertical slice.
//
// BlockID switches Narrate into single-block re-render mode. Empty (the
// zero value) preserves the whole-document flow. When non-empty, Narrate
// runs adapter + planner over the whole document (planner stays I/O-free
// and pure — see CLAUDE.md), locates the block by ID, then calls
// Renderer.RenderBlock and sinks a one-block plan. ErrBlockNotFound when
// the ID is absent from the resulting plan.
//
// ExpectedContentHash is the caller-supplied document content hash they
// got the BlockID alongside. When set together with BlockID, Narrate
// compares it against the planner-produced Source.ContentHash; on
// mismatch, NarrateResult.BlockHashMismatch is populated and the
// re-render still proceeds (warning, not error). Empty disables the
// check.
type NarrateRequest struct {
	Voice               string
	LevelOverrides      map[string]plan.Level
	BlockID             string
	ExpectedContentHash string
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
// Whole-document path (NarrateRequest.BlockID empty — design doc §3.5):
//  1. adapter.Read(ctx, ref) → RawDocument. Errors stop.
//  2. planner.Plan(ctx, doc, Request{Level, Overrides}, intel) → NarrationPlan.
//     Errors stop. Refused blocks land inside the plan as data.
//  3. renderer.Render(ctx, plan, RenderOptions{OutDir, Voice}) → RenderResult.
//     Errors stop. Refused blocks are pre-rendered audio of Refusal.Message.
//  4. sink.Consume(ctx, plan, renderResult) → SinkReceipt. Errors stop with
//     a partial receipt.
//
// Single-block re-render path (NarrateRequest.BlockID non-empty — issue #14):
//  1. adapter.Read — same as whole-doc.
//  2. planner.Plan — same. Whole document re-planned; the planner is pure
//     and sub-100ms, so trimming the source would risk segmenter
//     reclassification with no real win (and the planner cannot do I/O
//     anyway — CLAUDE.md invariant).
//  3. Index by BlockID. Missing → ErrBlockNotFound wrapping the ID.
//  4. If ExpectedContentHash is set, compare it against the document's
//     Source.ContentHash. Mismatch is surfaced as BlockHashMismatch on the
//     result, NOT as an error — the block content has changed since the
//     caller obtained the ID, but the re-render still runs.
//  5. renderer.RenderBlock(ctx, plan, BlockID, …) → BlockRender.
//  6. Build a one-block sub-plan (defaults + the targeted block) so the
//     sink plays only that block; sink.Consume runs against the sub-plan
//     and a one-row RenderResult.
//
// Returns (NarrateResult, nil) on the honest path — including plans containing
// refused blocks. NarrateResult embeds sink.SinkReceipt so callers that read
// receipt fields directly continue to compile. Errors propagate from any edge.
//
// Error contract for NarrateResult fields: see the NarrateResult docstring.
// In short — when err is non-nil, prefer surfacing the error; the result
// envelope may carry partial fields (e.g. BlockSummaries from a successful
// planner pass, or a BlockHashMismatch warning that fired before the
// downstream error) but those are diagnostic context, not the primary
// signal.
func (p *Pipeline) Narrate(ctx context.Context, ref plan.SourceRef, req NarrateRequest) (NarrateResult, error) {
	if err := ctx.Err(); err != nil {
		return NarrateResult{}, fmt.Errorf("pipeline: %w", err)
	}

	doc, err := p.Adapter.Read(ctx, ref)
	if err != nil {
		return NarrateResult{}, fmt.Errorf("pipeline: adapter: %w", err)
	}

	narrationPlan, err := planner.Plan(ctx, doc, planner.Request{
		Level:        p.Defaults.Level,
		Overrides:    req.LevelOverrides,
		CodeMinLevel: p.Defaults.CodeMinLevel,
	}, p.Intelligence)
	if err != nil {
		return NarrateResult{}, fmt.Errorf("pipeline: planner: %w", err)
	}

	summaries := summarizeBlocks(narrationPlan.Blocks)
	docHash := narrationPlan.Source.ContentHash

	if req.BlockID == "" {
		// Whole-document path — unchanged behavior plus block roster.
		result, err := p.Renderer.Render(ctx, narrationPlan, render.RenderOptions{
			OutDir: p.Defaults.OutDir,
			Voice:  req.Voice,
		})
		if err != nil {
			return NarrateResult{}, fmt.Errorf("pipeline: renderer: %w", err)
		}

		// Post-render projection (BLOCKING-2): the timeline only exists now,
		// after Render. Join per-block durations onto the roster summarizeBlocks
		// produced before render. Refused / unrendered blocks keep DurationMs 0.
		joinTimelineDurations(summaries, result.Timeline)

		receipt, err := p.Sink.Consume(ctx, narrationPlan, result)
		out := NarrateResult{
			SinkReceipt:         receipt,
			BlockSummaries:      summaries,
			DocumentContentHash: docHash,
		}
		if err != nil {
			return out, fmt.Errorf("pipeline: sink: %w", err)
		}
		return out, nil
	}

	// Single-block re-render path.
	targetIdx := -1
	for i := range narrationPlan.Blocks {
		if narrationPlan.Blocks[i].ID == req.BlockID {
			targetIdx = i
			break
		}
	}
	if targetIdx < 0 {
		return NarrateResult{
			BlockSummaries:      summaries,
			DocumentContentHash: docHash,
		}, fmt.Errorf("%w: %s", ErrBlockNotFound, req.BlockID)
	}

	out := NarrateResult{
		BlockSummaries:      summaries,
		DocumentContentHash: docHash,
	}
	if req.ExpectedContentHash != "" && req.ExpectedContentHash != docHash {
		out.BlockHashMismatch = &BlockHashMismatch{
			BlockID:  req.BlockID,
			Expected: req.ExpectedContentHash,
			Got:      docHash,
		}
	}

	br, err := p.Renderer.RenderBlock(ctx, narrationPlan, req.BlockID, render.RenderOptions{
		OutDir: p.Defaults.OutDir,
		Voice:  req.Voice,
	})
	if err != nil {
		return out, fmt.Errorf("pipeline: renderer: %w", err)
	}

	// Build a one-block sub-plan. F4 fix: also trim Diagnostics to those
	// scoped to the targeted block (or document-level, BlockID==""); a sink
	// or future consumer that walks Diagnostics expects every BlockID to
	// resolve in plan.Blocks.
	subPlan := narrationPlan
	subPlan.Blocks = []plan.Block{narrationPlan.Blocks[targetIdx]}
	subPlan.Diagnostics = filterDiagnosticsForBlock(narrationPlan.Diagnostics, req.BlockID)

	subResult := render.RenderResult{
		Audio:    br.Audio,
		Format:   br.Format,
		Timeline: plan.Timeline{Blocks: []plan.BlockTiming{br.Timing}},
	}

	// Post-render projection (BLOCKING-2): join the single re-rendered block's
	// duration onto the full roster. Only the targeted block has a timing row,
	// so the rest stay 0 — the re-render does not re-time the other blocks.
	joinTimelineDurations(out.BlockSummaries, subResult.Timeline)

	receipt, err := p.Sink.Consume(ctx, subPlan, subResult)
	out.SinkReceipt = receipt
	if err != nil {
		return out, fmt.Errorf("pipeline: sink: %w", err)
	}
	return out, nil
}

// filterDiagnosticsForBlock keeps only diagnostics scoped to blockID or to
// the document level (empty BlockID). Returns nil when there is nothing to
// keep (skips an empty allocation).
func filterDiagnosticsForBlock(in []plan.Diagnostic, blockID string) []plan.Diagnostic {
	if len(in) == 0 {
		return nil
	}
	out := make([]plan.Diagnostic, 0, len(in))
	for _, d := range in {
		if d.BlockID == "" || d.BlockID == blockID {
			out = append(out, d)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// summarizeBlocks builds the per-block roster from the latest plan. Pure;
// safe to call before or after rendering. Enumerates TOP-LEVEL blocks only —
// it does NOT recurse into Block.SubBlocks (the transcript projection mirrors
// this exactly). DurationMs is left zero here; it is a post-render concern
// filled by joinTimelineDurations (the timeline does not exist yet at the call
// site, ~pipeline Narrate before Render).
func summarizeBlocks(blocks []plan.Block) []BlockSummary {
	if len(blocks) == 0 {
		return nil
	}
	out := make([]BlockSummary, 0, len(blocks))
	for _, b := range blocks {
		out = append(out, BlockSummary{
			ID:             b.ID,
			Order:          b.Order,
			Class:          b.Class,
			Level:          b.Level,
			Status:         b.Status,
			StartLine:      b.SourceMap.StartLine,
			EndLine:        b.SourceMap.EndLine,
			SpokenText:     spokenTextOf(b),
			RefusalReason:  refusalReasonOf(b),
			RefusalMessage: refusalMessageOf(b),
		})
	}
	return out
}

// spokenTextOf joins a block's spoken words: the single-space join of every
// non-empty Segment.Text in segment order. Pause / empty-text segments
// contribute nothing (they have no words). Mirrors what the renderer voices.
func spokenTextOf(b plan.Block) string {
	parts := make([]string, 0, len(b.Segments))
	for _, s := range b.Segments {
		if s.Text == "" {
			continue
		}
		parts = append(parts, s.Text)
	}
	return strings.Join(parts, " ")
}

// refusalReasonOf returns the block's refusal reason, empty unless the block
// carries a Refusal (Status == StatusRefused per the schema invariant).
func refusalReasonOf(b plan.Block) plan.RefusalReason {
	if b.Refusal == nil {
		return ""
	}
	return b.Refusal.Reason
}

// refusalMessageOf returns the block's refusal message, empty unless the block
// carries a Refusal.
func refusalMessageOf(b plan.Block) string {
	if b.Refusal == nil {
		return ""
	}
	return b.Refusal.Message
}

// joinTimelineDurations is the SEPARATE post-render projection pass (NOT part
// of summarizeBlocks, which runs before a Timeline exists). It walks the
// already-built roster and sets each BlockSummary.DurationMs to EndMs - StartMs
// from the matching BlockTiming, keyed by BlockID. Blocks with no timing row
// (e.g. refused / not rendered) keep DurationMs == 0. Mutates summaries in
// place; safe on a nil/empty timeline (every duration stays 0).
func joinTimelineDurations(summaries []BlockSummary, tl plan.Timeline) {
	if len(summaries) == 0 || len(tl.Blocks) == 0 {
		return
	}
	durByID := make(map[string]int, len(tl.Blocks))
	for _, bt := range tl.Blocks {
		durByID[bt.BlockID] = bt.EndMs - bt.StartMs
	}
	for i := range summaries {
		if d, ok := durByID[summaries[i].ID]; ok {
			summaries[i].DurationMs = d
		}
	}
}
