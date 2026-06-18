package planner

import (
	"fmt"
	"strings"

	"github.com/vd09-projects/intelligent-tts-narration-library/plan"
)

// proseVerbatimMaxWords — the threshold below which a prose block is
// read verbatim when no intelligence adapter is plugged in (A4 working
// default). At or below the threshold → Status=degraded + Diagnostic.
// Above → Status=refused + RefuseNoIntelligence.
const proseVerbatimMaxWords = 120

// degrade — assemble a final plan.Block when the orchestrator's
// "intel is nil OR not needed" path applies. The caller has already
// decided no IntelligenceAdapter call is happening for this block.
//
// Branches:
//   - levelOut.refuseHint != "" → refuse (image / un-voiceable)
//   - levelOut.segments populated → block is voiced deterministically
//     (no degradation needed)
//   - levelOut.needsIntelligence is true → the (class, level) cell
//     required comprehension. Behavior depends on class:
//       prose ≤120 words → degraded verbatim
//       prose >120 words → refused RefuseNoIntelligence
//       structured (code/diagram) → degrade to L1 deterministic gist
//
// Returns the block plus a non-nil diagnostic when downgrade information
// is useful to surface. Block IDs are assigned by the orchestrator.
func degrade(
	rb rawBlock,
	cls plan.Class,
	target plan.Level,
	levelOut levelResult,
	sm plan.SourceMap,
	lex *compiledLex,
) (plan.Block, *plan.Diagnostic) {
	// Image / un-voiceable hint — honesty rule, refuse with source map.
	if levelOut.refuseHint != "" {
		return buildRefusal(rb, cls, target, levelOut.refuseHint, sm), nil
	}

	// Block was voiced deterministically by level() — no degradation.
	if len(levelOut.segments) > 0 {
		return plan.Block{
			Class:      cls,
			Level:      target,
			Status:     plan.StatusVoiced,
			SourceMap:  sm,
			Segments:   levelOut.segments,
			Provenance: provenanceWith(levelOut.provenance, "planner", true, target),
		}, nil
	}

	// Needs intelligence; none plugged in.
	if cls == plan.ClassProse {
		return degradeProse(rb, target, sm, lex)
	}

	// Structured class needing intelligence (code L3, diagram L2/L3).
	// Downshift to L1 deterministic gist and mark Status=degraded.
	return degradeStructuredDownshift(rb, cls, target, sm, lex)
}

// buildRefusal — assemble a Status=refused block. Honesty rule: every
// refusal carries Spoken=true and a populated SourceMap.
func buildRefusal(rb rawBlock, cls plan.Class, target plan.Level, reason plan.RefusalReason, sm plan.SourceMap) plan.Block {
	msg := refusalMessage(reason, rb, sm)
	return plan.Block{
		Class:     cls,
		Level:     target,
		Status:    plan.StatusRefused,
		SourceMap: sm,
		Refusal: &plan.Refusal{
			Reason:    reason,
			Message:   msg,
			Spoken:    true,
			SourceMap: sm,
		},
		Provenance: plan.Provenance{VoicedBy: "planner", Deterministic: true, LevelAsked: target},
	}
}

func refusalMessage(reason plan.RefusalReason, rb rawBlock, sm plan.SourceMap) string {
	switch reason {
	case plan.RefuseBareImage:
		return fmt.Sprintf("There's an image here that isn't voiced — check the source, line %d.", sm.StartLine)
	case plan.RefuseNoIntelligence:
		return fmt.Sprintf("A long prose section I can't summarize without an intelligence adapter — check the source, lines %d to %d.", sm.StartLine, sm.EndLine)
	case plan.RefuseTooLarge:
		return fmt.Sprintf("This block is too large to faithfully voice at the requested level — check the source, lines %d to %d.", sm.StartLine, sm.EndLine)
	case plan.RefuseTooRaw:
		return fmt.Sprintf("This block is too raw to voice — check the source, lines %d to %d.", sm.StartLine, sm.EndLine)
	case plan.RefuseUnsupported:
		return fmt.Sprintf("This content type isn't supported in phase one — check the source, lines %d to %d.", sm.StartLine, sm.EndLine)
	default:
		_ = rb
		return fmt.Sprintf("This block isn't voiced — check the source, lines %d to %d.", sm.StartLine, sm.EndLine)
	}
}

// degradeProse — verbatim under threshold, refuse over.
func degradeProse(rb rawBlock, target plan.Level, sm plan.SourceMap, lex *compiledLex) (plan.Block, *plan.Diagnostic) {
	words := len(strings.Fields(rb.text))
	if words > proseVerbatimMaxWords {
		blk := buildRefusal(rb, plan.ClassProse, target, plan.RefuseNoIntelligence, sm)
		return blk, &plan.Diagnostic{
			Code:     "intelligence_unavailable",
			Severity: "warning",
			Message:  fmt.Sprintf("prose block over %d words refused without an intelligence adapter", proseVerbatimMaxWords),
		}
	}
	spoken := voice(strings.TrimSpace(rb.text), lex)
	return plan.Block{
			Class:     plan.ClassProse,
			Level:     target,
			Status:    plan.StatusDegraded,
			SourceMap: sm,
			Segments: []plan.Segment{
				{ID: "s1", Kind: plan.SegmentKindSpeech, Text: spoken},
			},
			Provenance: plan.Provenance{
				VoicedBy:      "verbatim",
				Deterministic: true,
				LevelAsked:    target,
			},
		}, &plan.Diagnostic{
			Code:     "intelligence_unavailable",
			Severity: "info",
			Message:  fmt.Sprintf("prose block under %d words read verbatim without an intelligence adapter", proseVerbatimMaxWords),
		}
}

// degradeStructuredDownshift — re-level a structured class at L1 (which
// is always deterministic) and mark the result Status=degraded so a
// downstream consumer sees the requested-vs-delivered gap.
func degradeStructuredDownshift(rb rawBlock, cls plan.Class, target plan.Level, sm plan.SourceMap, lex *compiledLex) (plan.Block, *plan.Diagnostic) {
	l1 := level(rb, cls, plan.L1, lex)
	if len(l1.segments) == 0 {
		// Should not happen — L1 is deterministic for every structured
		// class. Refuse as a safety net rather than emit an empty block.
		blk := buildRefusal(rb, cls, target, plan.RefuseTooLarge, sm)
		return blk, &plan.Diagnostic{
			Code:     "intelligence_unavailable",
			Severity: "warning",
			Message:  fmt.Sprintf("%s block could not be downshifted to L1", cls),
		}
	}
	return plan.Block{
			Class:     cls,
			Level:     target,
			Status:    plan.StatusDegraded,
			SourceMap: sm,
			Segments:  l1.segments,
			Provenance: plan.Provenance{
				VoicedBy:      "planner",
				Deterministic: true,
				LevelAsked:    target,
			},
		}, &plan.Diagnostic{
			Code:     "intelligence_unavailable",
			Severity: "info",
			Message:  fmt.Sprintf("%s block downshifted to L1 without an intelligence adapter", cls),
		}
}

// provenanceWith — fill in the run-time fields the level() result left
// blank (VoicedBy, Deterministic, LevelAsked).
func provenanceWith(p plan.Provenance, voicedBy string, deterministic bool, asked plan.Level) plan.Provenance {
	if p.VoicedBy == "" {
		p.VoicedBy = voicedBy
	}
	if !p.Deterministic {
		p.Deterministic = deterministic
	}
	if p.LevelAsked == 0 {
		p.LevelAsked = asked
	}
	return p
}
