# Observer seam placement: the sink reads Level/Status from the plan param it already receives, not a cmd-side BlockSummary closure

| Field    | Value            |
|----------|------------------|
| Date     | 2026-06-26       |
| Status   | accepted         |
| Category | architecture     |
| Tags     | observer-seam, sink-ephemeral, composition-root, blocksummary, liveness, import-wall, sub-blocks, issue-81, issue-77, plan-v2-deviation |

## Context

The #81 Channel-2 observer renders a per-block line that needs each block's **Level** and
**Status**. The approved mimir plan (v2) specified that `sink/ephemeral` stay roster-free
and emit only `BlockTiming`, while a `cmd/narrate-mcp` closure enriched Level/Status from
the flattened `pipeline.BlockSummary` slice — justified by a claim that `BlockSummary`
flattens oversized-split sub-blocks whereas the sink's `plan.Blocks` does not.

Step 0 verification against the live code falsified the plan's premise on two counts:
1. **The closure is unbuildable as specified.** `BlockSummaries` are returned by `Narrate`
   only *after* it runs — but the observer must be wired into the sink *before* `Narrate`
   is called. A pre-Narrate closure has no roster to capture.
2. **The sub-block justification does not apply phase one.** No producer populates
   `Block.SubBlocks` (it is a schema field with zero writers), and `render/sherpa` emits
   exactly one `BlockTiming` per top-level `Block` in plan order (render.go invariant), so
   `Timeline.Blocks` ↔ top-level `plan.Blocks` are 1:1 co-ordered. There is nothing to
   flatten.

## Options considered

### Option A: sink reads Level/Status from its `plan.NarrationPlan` param (chosen)
- **Pros**: live (emit before each blocking `play()`); buildable; correct 1:1 BlockID
  correlation; the sink already *receives* the plan param (historically discarded), and
  `plan/` is already imported, so the import wall is unchanged.
- **Cons**: the sink gains a small `BlockID→Block` lookup — minimal "roster knowledge",
  but bounded to reading two fields off a param it was already handed.

### Option B: cmd-side closure enriching from `pipeline.BlockSummary` (the plan v2 design)
- **Cons**: unbuildable (roster exists only post-Narrate, observer wires pre-Narrate); its
  sub-block rationale is moot phase one. Rejected.

## Decision

The emit seam splits as: `sink/ephemeral` owns the import-clean `BlockObserver` /
`BlockProgress` / `WithBlockObserver` types and reads Level/Status from the plan param it
already receives (correlated 1:1 by BlockID, zero-Level/Status fallback on a miss — never
fabricated); `cmd/narrate-mcp/observe.go` owns the concrete JSONL marshaling and scratch-
file lifecycle. This keeps liveness AND import-cleanliness AND honesty. The sink import set
(`plan` + `render` + `sink` + stdlib) is unchanged and now guarded by a new
`sink/ephemeral/deps_test.go`.

This deviates from the approved plan v2; the deviation was surfaced during Step 0 and is
recorded here so the "why" survives.

## Consequences

- `Sink.Consume`'s plan param is no longer discarded; a `BlockID→Block` map is built only
  when an observer is wired (off-path stays allocation-free and byte-identical).
- `Playing` is derived purely from `AudioRef != ""`, so it reports what the renderer
  produced — a refused block (whose `Refusal.Message` IS voiced into a WAV) is normally
  `Playing:true`; `Status:refused` carries the honesty signal, not `Playing`. (Caught and
  corrected in review — the plan's "refused ⇒ Playing:false" truth table was wrong.)
- If a future change populates `Block.SubBlocks` AND the renderer emits sub-block timing
  rows, the BlockID-keyed lookup still resolves correctly; only an order-only correlation
  would have broken.

## Related decisions

- [ADR: Playback observability & control model (issue #77)](2026-06-24-playback-observability-control-model-issue-77.md)
- [Channel-2 mechanism: JSONL + tail over MCP progress](2026-06-26-channel2-mechanism-jsonl-tail-over-mcp-progress.md) — sibling decision from the same build.

## Revisit trigger

If oversized-block splitting lands (a producer populates `Block.SubBlocks` and the renderer
times sub-blocks), re-verify the Timeline ↔ plan correlation and the observer's BlockID
lookup against the new flattening.
