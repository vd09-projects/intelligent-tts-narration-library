# Left/Right arrows do ±10s time seek on the wav playhead, not block-step

| Field    | Value            |
|----------|------------------|
| Date     | 2026-06-29       |
| Status   | accepted         |
| Category | tradeoff         |
| Tags     | issue-134, earshot, keyboard, transport, playback, seek, block-level-sync, media-convention |

## Context

Issue #134 layers YouTube-style media keyboard shortcuts onto the shipped #112 Earshot transport. The open design question: what should Left/Right arrows do at the global (document) scope — a ±10s time seek on the combined wav playhead, or a previous/next-block step? The #112 transport already binds ←/→ locally in two places: the role=toolbar roving button group (move between buttons) and the role=slider block scrubber (block-step prev/next). Block-level-sync is a load-bearing invariant: plan/timeline stays block-keyed, no sub-block/word timing.

## Options considered

### Option A: ←/→ = ±10s time seek on the combined wav playhead
- **Pros**: matches universal media convention (YouTube, video players); a free-moving audio playhead is transport/display only and plan↔text-mapping-neutral — it persists nothing sub-block, so it does not violate block-level-sync; block-step remains available on the slider's native role=slider arrow keys.
- **Cons**: introduces a sub-block playhead position that has no representation in the plan (acceptable — it is display-only and never persisted).

### Option B: ←/→ = previous/next block, ±10s remapped to ,/. or J/L
- **Pros**: keeps arrows on a block-quantized action consistent with the slider.
- **Cons**: less familiar; fights media convention; pushes the common ±10s action onto secondary keys.

## Decision

Left/Right arrows perform a **±10-second time seek** on the combined wav playhead, NOT a block-step. A free-moving audio playhead is transport/display only and plan↔text-mapping-neutral — it persists nothing sub-block, so it does not violate the block-level-sync invariant. Block-step stays on the slider's native role=slider arrow keys. Option (b) was rejected as less familiar and against media convention. This resolves the design decision posed in issue #134.

## Consequences

- A new relative `skipSeconds(deltaSec)` method is added to the playback command surface; it no-ops when the audio element or duration metadata is absent, and clamps both ends.
- Two arrow-key owners coexist by scope: global document scope = ±10s time seek; slider widget scope = block-step. The focus-guard allow-list keeps them from clashing.
- No plan/timeline schema change; no sub-block timing persisted.

## Related decisions

- [Earshot transport deck exposes two keyboard tab stops, not one](../architecture/2026-06-29-earshot-transport-two-tab-stops-not-one.md) — defines the toolbar/slider arrow-key ownership this seek scope coexists with.

## Revisit trigger

Revisit if Earshot ever needs frame-accurate sub-block scrubbing, or if user testing shows the ±10s step size is wrong for narration content.
