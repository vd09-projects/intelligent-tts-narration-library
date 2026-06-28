# Seed a per-(blockId,level) snapshot cache with paired timeline; de-escalate-to-seen-level is a no-model-rebill swap

| Field    | Value            |
|----------|------------------|
| Date     | 2026-06-29       |
| Status   | accepted         |
| Category | architecture     |
| Tags     | issue-113, earshot, state-management, useNarrationSession, snapshot-cache, no-model-rebill, timeline-snapshot, de-escalation, in-memory-cache, f1-offset-bug |

## Context

Issue #113 lets a listener return to a previously-seen level without re-paying for intelligence. The cache must let a de-escalate (or return-to-original) be a client-side swap. But after the full-timeline decision, restoring a cached block against the *current* (post-escalate) timeline would reintroduce stale offsets (the F1 offset bug): a block cached at the original level paired with a later, shifted timeline puts sibling seeks on the wrong audio. The v1 cache keyed only `{block, timing}` — no timeline — so a no-network de-escalate would restore against the wrong offsets.

## Options considered

### Option A: cache {block, timing} only
- **Pros**: smaller.
- **Cons**: a no-network de-escalate restores a block against the post-escalate timeline → reintroduces F1 (stale sibling offsets on the offline path).

### Option B (chosen): cache {block, timing, timelineSnapshot}, seeded for every block on every transcript set
- **Pros**: each cached level is paired with the timeline authoritative *at that level*; return-to-any-seen-level (including the original) is a pure client swap with correct offsets and zero model re-bill.
- **Cons**: holds a timeline snapshot per seen level (in-memory, session-scoped — acceptable).

## Decision

Seed `cache[(blockId, block.level)] = {block, timing, timelineSnapshot}` for **every** block whenever a transcript is set (initial load **and** after each escalate), pairing each cached level with the timeline that was authoritative at that level. On a **cache hit**, restore the cached `{block, timing}` **and replace the entry's whole `transcript.timeline` with the cached `timelineSnapshot`** (so the offline de-escalate cannot reintroduce stale offsets — F1), bump the reload nonce, and `reload()` — with **no `postNarrateBlock` call and no model re-bill**. **"No re-bill" is clarified to mean no model/intelligence spend:** under the single-shared-`<audio>` model a de-escalate *may* still POST so the server rewrites the combined wav, but the server's `(block hash, level, model)` cache returns it **without billing the model**; the client snapshot cache additionally enables a zero-network swap for return-to-seen-level. Rejected: caching block+timing without the timeline snapshot — reintroduces F1 on the offline path.

## Consequences

- The `(blockId, level)` snapshot cache lives in `useNarrationSession` (the single server-cache owner) alongside `entries`; not duplicated into any new store; lost on reload by design (in-memory, session-scoped).
- A review test (`cache-hit timeline-snapshot`) asserts a cache-hit de-escalate restores the cached timeline snapshot, not the post-escalate timeline.

## Related decisions

- [/narrate/block returns the full post-patch timeline; client replaces wholesale](2026-06-29-narrate-block-returns-full-post-patch-timeline.md) — the timeline-replace path this cache must pair correctly per level (F1).
- [Earshot parallel per-entry narration model with in-memory client-side dedup](2026-06-29-earshot-parallel-per-entry-narration-model.md) — the `entries` Map this snapshot cache sits beside; both in-memory, session-scoped.
