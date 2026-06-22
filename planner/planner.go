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

	"golang.org/x/sync/errgroup"

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
//  4. Structural pass — single-threaded walk assigning every block its
//     ID and Order, classifying + leveling, and deciding needsVoice. No
//     intel.Voice calls here. IDs and Order are assigned before any
//     goroutine starts.
//  5. Voice pass — bounded-concurrent fan-out of the needsVoice blocks
//     through intel.Voice, each goroutine writing only its own index-
//     disjoint result slot. Non-Voice blocks were materialized inline in
//     step 4. Diagnostics are flattened from the per-slot results only
//     after the group's Wait, in document order.
//  6. Assemble NarrationPlan with stable header fields.
//
// ctx is checked once at the top and (via the per-call group context)
// before/inside every intel.Voice call. Pure CPU work in the structural
// pass does not poll ctx — the Voice calls themselves are the
// cancellation points.
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
	limit := cfg.concurrencyLimit
	if limit <= 0 {
		limit = defaultIntelligenceConcurrency
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

	// --- Structural pass ---------------------------------------------
	// Single-threaded walk: classify + level every rawBlock, assign each
	// resulting block its ID and Order, and decide needsVoice. No
	// intel.Voice call happens here. idx stays a plain local int confined
	// to this pass — the pointer-threaded counter never crosses a
	// goroutine boundary. IDs and Order are assigned before any goroutine
	// starts.
	idx := 1
	var planned []plannedBlock
	for _, rb := range rawBlocks {
		planned = structuralPass(planned, rb, &idx, doc, req, lex)
	}

	// One result slot per planned block, pre-sized. Each slot is written
	// by exactly one writer: the structural/materialization pass for
	// non-Voice blocks below, or the owning Voice-pass goroutine for
	// needsVoice blocks. No goroutine ever touches out.Diagnostics — the
	// flatten happens-after Wait, in index order.
	results := make([]blockResult, len(planned))

	// Materialize the inline (non-Voice) blocks now via the pure degrade
	// path. A block is voiced by the adapter only when it needs voicing
	// AND an adapter is plugged in; otherwise the degrade path handles it
	// (verbatim prose, refusal, structured downshift — the honesty rule's
	// last line). Only those adapter-bound slots are left zero-valued for
	// the Voice pass to fill.
	dispatchVoice := intel != nil
	anyVoice := false
	for i := range planned {
		pb := planned[i]
		if pb.needsVoice && dispatchVoice {
			anyVoice = true
			continue
		}
		blk, diag := degrade(pb.rb, pb.cls, pb.target, pb.levelOut, pb.sm, lex)
		results[i] = materialize(pb, blk, diag)
	}

	// --- Voice pass ----------------------------------------------------
	// Short-circuit: with no adapter or no needsVoice block, skip
	// errgroup entirely — no group, no goroutines. The nil-adapter /
	// no-intel path stays byte-identical to a fully sequential run.
	if dispatchVoice && anyVoice {
		g, gctx := errgroup.WithContext(ctx)
		g.SetLimit(limit) // before the first g.Go — SetLimit-after-Go is a bug.
		for i := range planned {
			pb := planned[i]
			if !pb.needsVoice {
				continue
			}
			i := i // capture: each goroutine owns exactly one slot.
			// g.Go is issued from the single Plan goroutine in this loop,
			// never concurrently. gctx (not the parent ctx) is forwarded
			// into intel.Voice so first-error cancellation reaches all
			// in-flight calls.
			g.Go(func() error {
				blk, diag, err := callIntelligence(gctx, pb.rb, pb.cls, pb.target, pb.sm, pb.levelOut, intel)
				if err != nil {
					return err
				}
				results[i] = materialize(pb, blk, diag)
				return nil // refusals are data, not errors — never trip cancellation.
			})
		}
		// First-error-wins is deliberate: errgroup.Wait returns only the
		// first non-nil error and discards siblings. Do NOT errors.Join —
		// merging sibling errors would change the stop semantics.
		if err := g.Wait(); err != nil {
			return plan.NarrationPlan{}, err
		}
	}

	// Flatten results in index (document) order, happens-after Wait. This
	// is the load-bearing determinism + race invariant: out.Diagnostics
	// ordering is document order, never completion order, and no goroutine
	// ever appended to it.
	for i := range results {
		out.Blocks = append(out.Blocks, results[i].block)
		out.Diagnostics = append(out.Diagnostics, results[i].diags...)
	}
	return out, nil
}

// plannedBlock — the output of the structural pass for one leaf block:
// everything decided single-threaded (ID, Order, classification, level
// result, needsVoice) before any goroutine starts. The Voice pass reads
// these read-only; it never mutates a plannedBlock.
type plannedBlock struct {
	id         string
	order      int
	cls        plan.Class
	target     plan.Level
	sm         plan.SourceMap
	levelOut   levelResult
	needsVoice bool
	rb         rawBlock
}

// blockResult — one index-disjoint result slot. Each slot is written by
// a single writer (its owning goroutine, or the inline materialization
// for non-Voice blocks); diagnostics live here, never in a shared slice.
type blockResult struct {
	block plan.Block
	diags []plan.Diagnostic
}

// materialize — stamp a freshly built plan.Block with its structural-pass
// ID and Order and attach any single diagnostic (also ID-tagged). Used by
// both the inline non-Voice path and the Voice-pass goroutines so the
// ID/Order stamping is identical on both paths.
func materialize(pb plannedBlock, blk plan.Block, diag *plan.Diagnostic) blockResult {
	blk.ID = pb.id
	blk.Order = pb.order // carried verbatim from the structural pass — never re-derived.
	res := blockResult{block: blk}
	if diag != nil {
		d := *diag
		d.BlockID = pb.id
		res.diags = []plan.Diagnostic{d}
	}
	return res
}

// structuralPass — single-threaded walk of one rawBlock (recursing into
// oversized-split sub-blocks), appending a plannedBlock per leaf in
// document order. Assigns IDs/Order via the *idx counter exactly as the
// old sequential code did. Does classification + leveling + the
// needsVoice decision, but NEVER calls intel.Voice — that is the Voice
// pass's job. idx is threaded by pointer here and only here; it never
// crosses a goroutine boundary.
func structuralPass(
	planned []plannedBlock,
	rb rawBlock,
	idx *int,
	doc adapter.RawDocument,
	req Request,
	lex *compiledLex,
) []plannedBlock {
	cls := classify(rb)
	sm := rawBlockSourceMap(rb, doc)
	target := effectiveLevel(req, fmt.Sprintf("b%03d", *idx))

	levelOut := level(rb, cls, target, lex)

	// Oversized-split: recurse into each sub-block, still single-threaded.
	if len(levelOut.subBlocks) > 0 {
		for _, sub := range levelOut.subBlocks {
			planned = structuralPass(planned, sub, idx, doc, req, lex)
		}
		return planned
	}

	id := fmt.Sprintf("b%03d", *idx)
	order := *idx
	*idx++

	return append(planned, plannedBlock{
		id:         id,
		order:      order,
		cls:        cls,
		target:     target,
		sm:         sm,
		levelOut:   levelOut,
		needsVoice: intelNeedsVoice(levelOut),
		rb:         rb,
	})
}

// intelNeedsVoice — whether this leaf block, given its level result,
// would dispatch to the intelligence adapter (vs the pure degrade path).
// Mirrors the old planBlockTree branch condition; the intel != nil check
// lives at the call site (a nil adapter short-circuits the whole Voice
// pass).
func intelNeedsVoice(levelOut levelResult) bool {
	return levelOut.needsIntelligence
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
		return intelRefusalBlock(cls, target, sm, refusalMessageFromAdapter(res.RefusalNote, sm), res.Model), nil, nil
	}

	// Code-L2 cap enforcement — the single choke point where an adapter
	// reply becomes Segment.Text. The CodeUserL2 prompt advertises "one
	// sentence, at most 30 words"; that instruction is advisory until
	// enforced here. Guarded to ClassCode && L2 only, so every other class
	// and every other level passes through byte-identical. Both the
	// anthropic and mcpsampling adapters route through this point, so both
	// get the enforcement for free — no adapter code touched.
	//
	// Honesty rule: we either speak the adapter's own words (trimmed at a
	// clean sentence seam) or we refuse — never truncate mid-sentence
	// (fabrication). An over-cap reply is a too-large-for-level condition,
	// hence RefuseTooLarge (no new sentinel).
	voicedText := res.Text
	if cls == plan.ClassCode && target == plan.L2 {
		capped, ok := firstSentenceWithinCap(res.Text, codeL2MaxWords)
		if !ok {
			return intelRefusalBlock(cls, target, sm, refusalMessageFromAdapter(res.RefusalNote, sm), res.Model), nil, nil
		}
		voicedText = capped
	}

	blk := plan.Block{
		Class:     cls,
		Level:     target,
		Status:    plan.StatusVoiced,
		SourceMap: sm,
		Segments: []plan.Segment{
			{ID: "s1", Kind: plan.SegmentKindSpeech, Text: voicedText},
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

// intelRefusalBlock — build the honesty-rule refusal block for an
// intelligence-path block (adapter refused, or code-L2 reply exceeded the
// one-sentence/word cap). Both refusal sites share this so Status, the
// RefuseTooLarge reason, Spoken=true, and the populated SourceMap stay
// identical on both paths.
func intelRefusalBlock(cls plan.Class, target plan.Level, sm plan.SourceMap, message, model string) plan.Block {
	return plan.Block{
		Class:     cls,
		Level:     target,
		Status:    plan.StatusRefused,
		SourceMap: sm,
		Refusal: &plan.Refusal{
			Reason:    plan.RefuseTooLarge,
			Message:   message,
			Spoken:    true,
			SourceMap: sm,
		},
		Provenance: plan.Provenance{
			VoicedBy:      "intelligence",
			Deterministic: false,
			Model:         model,
			LevelAsked:    target,
		},
	}
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
