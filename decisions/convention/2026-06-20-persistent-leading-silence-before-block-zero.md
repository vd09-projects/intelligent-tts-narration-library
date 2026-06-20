# Persistent sink emits leading silence before block 0 if StartMs > 0

- **Date:** 2026-06-20
- **Status:** accepted
- **Category:** convention
- **Tags:** [sink, persistent, audio-wav, silence, timeline-fidelity, issue-16]
- **Owner:** vd
- **Scope:** issue-16

## Context

`render.RenderResult.Timeline.Blocks[0]` can have a non-zero `StartMs` — e.g. if the planner wants a beat of silence before the first spoken block. The persistent sink concatenates per-block WAVs into a single `audio.wav`; the question is whether the audio file starts at `t=0` (matching the timeline) or at `t=Block[0].StartMs` (skipping the leading silence).

## Options considered

### Option A: Emit leading silence before block 0 if StartMs > 0 (CHOSEN)
- **Pros**: `audio.wav`'s absolute time matches `Timeline.Blocks[i].StartMs` for every i, including i=0. Downstream consumers (the React reference player, future scrub bars) can compute `seek_to(timeline_block.StartMs)` and have it land in the right place without per-block offset arithmetic. Fidelity to planner intent.
- **Cons**: A planner that produces `StartMs=200` for block 0 (intending only an inter-block offset, not a leading pad) now gets 200 ms of silence at the start. Couples sink behavior to the planner's StartMs semantics.

### Option B: Skip leading silence before block 0
- **Pros**: Always-starts-at-zero is intuitive.
- **Cons**: `audio.wav` no longer aligns with the timeline by absolute time. Consumers have to track the leading offset separately. Worse, the sink is silently editing planner output — a soft violation of the plan-is-the-contract invariant.

### Option C: Configurable via Option (default skip)
- **Pros**: Caller picks.
- **Cons**: Speculative configurability. No known caller wants the opposite of "match the timeline".

## Decision

`Consume` emits `Blocks[0].StartMs` milliseconds of silence as the leading prefix of `audio.wav` whenever `StartMs > 0`. The same silence-padding logic that handles inter-block gaps applies to the pre-block-0 region.

This is captured in the per-block walk as a unified `leading = blk.StartMs - cursorMs` calculation (cursor starts at 0; the leading gap before block 0 is `StartMs - 0 = StartMs`). No special-case code for the first block.

## Consequences

- `audio.wav`'s wall-clock at offset `t` ms equals `Timeline.Blocks[i].StartMs == t` for the i-th spoken block. The React reference player can scrub by timeline without offset bookkeeping.
- Planners that don't want leading silence simply set `Blocks[0].StartMs = 0`. The default phase-one planner already does this for prose-only documents.
- A regression test (`TestConsume_LeadingSilenceBeforeFirstBlock`) pins the behavior: a 200 ms `StartMs` on block 0 contributes exactly 9600 zero bytes (200 × 24000 × 2 / 1000) to the combined `data` chunk.

## Related decisions

- [Empty-text blocks zero ms no AudioRef](2026-06-18-empty-text-blocks-zero-ms-no-audioref.md) — inter-block silence semantics this decision extends.
- [Per-block WAVs no concat in renderer](../architecture/2026-06-18-per-block-wavs-no-concat-in-renderer.md) — establishes that concat (with silence math) is the sink's job, not the renderer's.

## Revisit trigger

If a future planner uses non-zero `Blocks[0].StartMs` as inter-block-spacing-only signal (not leading-pad intent), and the React player begins to render its own pre-block silence overlay, revisit whether the sink should skip the leading region to avoid double-padding.
