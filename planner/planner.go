// Package planner is the intelligence-light core. Pure, no I/O. Imports
// only plan/, adapter/ (for the RawDocument value type), and
// intelligence/ (interface only).
//
// Invariants (CLAUDE.md):
//   - No I/O. No os/net/io/ioutil/syscall imports. No concrete adapter,
//     render, sink, or intelligence implementation package. Enforced by
//     planner/deps_test.go.
//   - Voicing happens here. Segment.Text is the literal words to speak.
//   - Refusal is data, not an error. Block.Status=refused carries a
//     Refusal with Spoken=true and a populated SourceMap.
package planner

import (
	"context"
	"fmt"
	"time"

	"github.com/vd09-projects/intelligent-tts-narration-library/adapter"
	"github.com/vd09-projects/intelligent-tts-narration-library/intelligence"
	"github.com/vd09-projects/intelligent-tts-narration-library/plan"
)

// SchemaVersion — the plan schema major.minor this planner emits.
const SchemaVersion = "1.0"

// Request — caller-supplied planning parameters.
//
// Level is the document-wide default. Overrides maps a block id (as
// emitted by Plan: "b001", "b002", …) to a per-block level — used by
// the escalation flow (design doc §4). A nil/empty map means "use
// Level for every block".
type Request struct {
	Level     plan.Level
	Overrides map[string]plan.Level
}

// Clock + plan-id seams are no longer package globals. They are threaded
// per-call through VoiceOption (withClock / withPlanID, test-only) and
// resolved to locals inside Plan(), defaulting to wall-clock + real ULID.
// Eliminating the globals removes the only shared mutable state in the
// planner, so parallel tests can install distinct seams without a data
// race.

// Plan — convert a RawDocument into a NarrationPlan.
//
// Pipeline (design doc §3.5):
//  1. Validate req.Level; default to L1 if unset.
//  2. Compile lexicon from DefaultLexicon + caller overrides.
//  3. Segment the document.
//  4. For each rawBlock: classify → level → (intel call OR degrade) →
//     assemble plan.Block.
//  5. Assemble NarrationPlan with stable header fields.
//
// ctx is checked once at the top and before every intel.Voice call.
// Pure CPU work between intel calls does not poll ctx — the calls
// themselves are the cancellation points.
//
// Error policy: returns an error only on IntelligenceAdapter transport
// failures (network down, deadline exceeded, etc.). Validation issues
// (empty document, unknown level) produce a plan with zero blocks plus
// a warning Diagnostic — never an error.
func Plan(
	ctx context.Context,
	doc adapter.RawDocument,
	req Request,
	intel intelligence.IntelligenceAdapter,
	opts ...VoiceOption,
) (plan.NarrationPlan, error) {
	if err := ctx.Err(); err != nil {
		return plan.NarrationPlan{}, fmt.Errorf("planner: %w", err)
	}

	if !req.Level.IsValid() {
		req.Level = plan.L1
	}

	// Parse opts once: surface the compiled lexicon AND the clock/planID
	// seams from the same parse. Defaults live here so a nil seam func can
	// never reach the read path below.
	cfg := resolveVoiceOptions(opts...)
	lex := compileLexiconCfg(cfg)
	now := cfg.clock
	if now == nil {
		now = func() time.Time { return time.Now().UTC() }
	}
	newPlanID := cfg.planID
	if newPlanID == nil {
		newPlanID = plan.NewPlanID
	}

	rawBlocks := segment(doc)

	out := plan.NarrationPlan{
		SchemaVersion: SchemaVersion,
		PlanID:        newPlanID(),
		CreatedAt:     now().Format(time.RFC3339),
		Source:        doc.Source,
		Defaults: plan.PlanDefaults{
			Level:  req.Level,
			Locale: "en",
		},
	}

	if len(rawBlocks) == 0 {
		out.Diagnostics = append(out.Diagnostics, plan.Diagnostic{
			Code:     "empty_document",
			Severity: "warning",
			Message:  "document contained no narratable blocks",
		})
		return out, nil
	}

	idx := 1
	for _, rb := range rawBlocks {
		blocks, diags, err := planBlockTree(ctx, rb, &idx, doc, req, intel, lex)
		if err != nil {
			return plan.NarrationPlan{}, err
		}
		out.Blocks = append(out.Blocks, blocks...)
		out.Diagnostics = append(out.Diagnostics, diags...)
	}

	// Set Order field after final flattening.
	for i := range out.Blocks {
		out.Blocks[i].Order = i + 1
	}
	return out, nil
}

// planBlockTree — assemble plan.Blocks for one rawBlock. When the
// oversized-split has produced sub-blocks, this recurses into each one
// and returns them flat (no wrapper SubBlocks today — phase one emits a
// flat block list; SubBlocks is reserved for the design doc's tree
// shape and remains empty unless leveling produces children).
func planBlockTree(
	ctx context.Context,
	rb rawBlock,
	idx *int,
	doc adapter.RawDocument,
	req Request,
	intel intelligence.IntelligenceAdapter,
	lex *compiledLex,
) ([]plan.Block, []plan.Diagnostic, error) {
	if err := ctx.Err(); err != nil {
		return nil, nil, fmt.Errorf("planner: %w", err)
	}

	cls := classify(rb)
	sm := rawBlockSourceMap(rb, doc)
	target := effectiveLevel(req, fmt.Sprintf("b%03d", *idx))

	levelOut := level(rb, cls, target, lex)

	// Oversized-split: re-plan each sub-block.
	if len(levelOut.subBlocks) > 0 {
		var blocks []plan.Block
		var diags []plan.Diagnostic
		for _, sub := range levelOut.subBlocks {
			subBlks, subDiags, err := planBlockTree(ctx, sub, idx, doc, req, intel, lex)
			if err != nil {
				return nil, nil, err
			}
			blocks = append(blocks, subBlks...)
			diags = append(diags, subDiags...)
		}
		return blocks, diags, nil
	}

	id := fmt.Sprintf("b%03d", *idx)
	*idx++

	// Intelligence path — only when an adapter is plugged in AND the
	// level result asked for comprehension.
	if intel != nil && levelOut.needsIntelligence {
		if err := ctx.Err(); err != nil {
			return nil, nil, fmt.Errorf("planner: %w", err)
		}
		blk, diag, err := callIntelligence(ctx, rb, cls, target, sm, levelOut, intel)
		if err != nil {
			return nil, nil, err
		}
		blk.ID = id
		blocks := []plan.Block{blk}
		var diags []plan.Diagnostic
		if diag != nil {
			d := *diag
			d.BlockID = id
			diags = append(diags, d)
		}
		return blocks, diags, nil
	}

	// No intelligence call → degrade path (handles refusal hints,
	// verbatim prose, and structured downshift).
	blk, diag := degrade(rb, cls, target, levelOut, sm, lex)
	blk.ID = id
	blocks := []plan.Block{blk}
	var diags []plan.Diagnostic
	if diag != nil {
		d := *diag
		d.BlockID = id
		diags = append(diags, d)
	}
	return blocks, diags, nil
}

// callIntelligence — invoke the adapter and translate its result into a
// plan.Block. Refused → Status=refused with RefuseTooLarge (honesty
// rule). Transport error → propagate.
func callIntelligence(
	ctx context.Context,
	rb rawBlock,
	cls plan.Class,
	target plan.Level,
	sm plan.SourceMap,
	levelOut levelResult,
	intel intelligence.IntelligenceAdapter,
) (plan.Block, *plan.Diagnostic, error) {
	res, err := intel.Voice(ctx, intelligence.IntelligenceRequest{
		BlockText: rb.text,
		Class:     cls,
		Facts:     levelOut.facts,
		Level:     target,
		Locale:    "en",
	})
	if err != nil {
		return plan.Block{}, nil, fmt.Errorf("planner: intelligence at line %d: %w", rb.startLine, err)
	}
	if res.Refused {
		blk := plan.Block{
			Class:     cls,
			Level:     target,
			Status:    plan.StatusRefused,
			SourceMap: sm,
			Refusal: &plan.Refusal{
				Reason:    plan.RefuseTooLarge,
				Message:   refusalMessageFromAdapter(res.RefusalNote, sm),
				Spoken:    true,
				SourceMap: sm,
			},
			Provenance: plan.Provenance{
				VoicedBy:      "intelligence",
				Deterministic: false,
				Model:         res.Model,
				LevelAsked:    target,
			},
		}
		return blk, nil, nil
	}
	blk := plan.Block{
		Class:     cls,
		Level:     target,
		Status:    plan.StatusVoiced,
		SourceMap: sm,
		Segments: []plan.Segment{
			{ID: "s1", Kind: plan.SegmentKindSpeech, Text: res.Text},
		},
		Provenance: plan.Provenance{
			VoicedBy:      "intelligence",
			Deterministic: false,
			Model:         res.Model,
			LevelAsked:    target,
		},
	}
	return blk, nil, nil
}

func refusalMessageFromAdapter(note string, sm plan.SourceMap) string {
	if note != "" {
		return note
	}
	return fmt.Sprintf("The intelligence adapter declined to summarize this block — check the source, lines %d to %d.", sm.StartLine, sm.EndLine)
}

func effectiveLevel(req Request, blockID string) plan.Level {
	if req.Overrides != nil {
		if lvl, ok := req.Overrides[blockID]; ok && lvl.IsValid() {
			return lvl
		}
	}
	return req.Level
}
