# mcpsampling cache is server-lifetime with LRU + entry-count-cap eviction

| Field    | Value            |
|----------|------------------|
| Date     | 2026-06-21       |
| Status   | accepted         |
| Category | convention       |
| Tags     | intelligence, mcpsampling, cache, eviction, lru, cache-lifetime, issue-25 |

**Supersedes:** [2026-06-20-mcpsampling-cache-key-includes-full-model-id](./2026-06-20-mcpsampling-cache-key-includes-full-model-id.md) — only the per-call cache lifetime is superseded; the full-model-id cache key + two-phase last-known-model lookup carry forward unchanged.

## Context

CLAUDE.md mandates: *"Intelligence caching: by `(block content hash, level, model)`. Escalation doesn't re-bill."* The original #13 design (decision `2026-06-20-mcpsampling-cache-key-includes-full-model-id`) got the **cache key** right — the full `mcp-sampling@<clientID>/<actualModel>` id with a two-phase last-known-model lookup — but ships the cache with a **per-call lifetime**: `cmd/narrate-mcp`'s speak handler constructed `mcpsampling.New(WithCache(NewInMemoryCache()))` per tool call, so every `runSpeak` invocation got a fresh empty cache.

That per-call lifetime breaks the second half of the CLAUDE.md rule. Escalation in this system is cross-call: the user narrates a block at L1 (tool call 1), then re-requests the same block at L3 (tool call 2). With a per-call cache, call 2 starts cold and re-bills the LLM for content it has already summarized — exactly the re-bill the cache exists to prevent.

#13's own revisit trigger anticipated this: *"If cross-call (per-server) cache lifetime ships, this decision's 'per-call cache' consequence becomes stale — revisit the cache-thread-safety story and the eviction-policy gap together."* Issue #25 is that ship.

## Options considered

### Option A: Server-lifetime cache, LRU + entry-count cap
- **Pros**: Cross-call escalation hits the cache → stops re-billing (honors CLAUDE.md). Bounded memory via a hard entry cap. Cached summaries never go wall-clock stale, so eviction can be purely capacity-driven.
- **Cons**: Cache outlives a single call, so a mid-session model switch opens a small stale-read window (bounded — see Consequences).

### Option B: Server-lifetime cache with TTL eviction
- **Pros**: Familiar pattern; bounds memory by time.
- **Cons**: Cached summaries are **pure functions of `(content_hash, level, full_model)`** — they never go wall-clock stale. A TTL would expire still-valid entries and force needless re-bills. Wrong tool for content that has no time dimension.

### Option C: Unbounded server-lifetime cache
- **Pros**: Simplest; never re-bills.
- **Cons**: No memory bound. A long-running server narrating many distinct documents grows the cache without limit.

## Decision

The mcpsampling cache is **server-lifetime** with **LRU + entry-count-cap eviction** (`DefaultCacheCapacity = 512`).

- Both the LRU summary cache **and** the last-known-model map now live on a single server-lifetime `ServerCache`, allocated **once** in `cmd/narrate-mcp` `serve` / `newServer` and shared across **all** `runSpeak` tool calls. Cross-call escalation therefore reuses cached summaries and stops re-billing.
- The **full-model-id cache key and the two-phase last-known-model lookup are unchanged** — they carry forward from `2026-06-20-mcpsampling-cache-key-includes-full-model-id` exactly. This decision changes *lifetime + eviction*, not *key shape*.
- **LRU + size-cap chosen over TTL** because cached summaries are pure functions of `(content_hash, level, full_model)` and never go wall-clock stale; TTL would only force needless re-bills.
- The **last-known-model map is NOT evicted.** It is bounded by construction: there is one `clientID` per server (`narrate-mcp` sets it via `WithClientID`), so the map holds exactly one entry. No eviction policy is needed for it.

## Consequences

- Cross-call escalation (narrate L1, then re-request the same block at L3 in a later tool call) is a cache hit and does not re-bill. This is the core win and the reason #25 exists.
- Memory is bounded at `DefaultCacheCapacity = 512` summary entries; the LRU evicts the least-recently-used entry past the cap.
- **Benign cross-call stale-read window:** because the cache now spans calls, a mid-session model switch can cause exactly **one** extra re-bill (the first lookup uses the old last-known model, misses, fetches, and updates). It **never returns wrong data** — the key still encodes the full model id, so a hit is always a correct hit. The window costs at most one re-bill, never a correctness violation.
- The last-known-model map's bounding rests on the one-clientID-per-server assumption. If that ever changes, the map becomes unbounded (see Revisit trigger).

## Related decisions

- [2026-06-20-mcpsampling-cache-key-includes-full-model-id](./2026-06-20-mcpsampling-cache-key-includes-full-model-id.md) — superseded by this decision (lifetime only); its cache-key + two-phase lookup carry forward unchanged.
- [2026-06-20-anthropic-cache-single-phase](./2026-06-20-anthropic-cache-single-phase.md) — the anthropic adapter's single-phase cache; its per-call lifetime consequence may now want the same cross-call treatment.

## Revisit trigger

- **If multiple `clientID`s per server are ever introduced**, the last-known-model map's bounding-by-construction assumption breaks. Eviction or per-client scoping for that map must be revisited at that point.

## Source

Inline decision mark from the issue #25 build session, captured in `.claude/handoff/mcpsampling-cache-eviction-25/implementation-build.md` and `.claude/handoff/mcpsampling-cache-eviction-25/planner-task.md`. Implemented as a server-lifetime `ServerCache` allocated in `cmd/narrate-mcp` `serve`/`newServer`.
