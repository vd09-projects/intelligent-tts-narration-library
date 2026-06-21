# mcpsampling cache key includes the full chosen-model id

- **Date:** 2026-06-20
- **Status:** superseded
- **Superseded by:** [2026-06-21-mcpsampling-cache-server-lifetime-lru-eviction](./2026-06-21-mcpsampling-cache-server-lifetime-lru-eviction.md) (lifetime only — this decision's cache-key + two-phase lookup carry forward unchanged)
- **Category:** convention
- **Tags:** intelligence, mcpsampling, cache, cache-key, escalation, claude-md-rule, issue-13

## Context

CLAUDE.md mandates: *"Intelligence caching: by `(block content hash, level, model)`. Escalation doesn't re-bill."* The mcpsampling adapter calls the MCP client LLM, which CHOOSES the model (the server requests; the client decides). The adapter only learns the actual model id from `CreateMessageResult.Model` after the call returns. That presents a sequencing problem for cache lookup.

Naive options:

- **A. Cache key uses clientID only** (`"mcp-sampling@<clientID>"`). Loses cross-model invalidation. Two model versions for the same clientID would collide. Plan-review round 1 caught this as B1 (blocking).
- **B. Skip lookup before the call; cache only AFTER.** Defeats caching for the common case (cache hit avoids any LLM round-trip — that IS the value).
- **C. Wildcard match in the cache.** Complicates the cache interface; opens up surprising semantics; not actually a known model id.

## Decision

`CacheKey.Model` is the FULL `IntelligenceResult.Model` string: `"mcp-sampling@<clientID>/<actualModel>"`. Lookup happens via a **two-phase scheme**:

1. The adapter holds a per-instance `map[clientID]actualModel` (`lastKnownByCID`), guarded by `sync.RWMutex`. The map records the most-recent actualModel observed from a CreateMessage reply.
2. On `Voice()`, BEFORE the call: if the map has an entry for `clientID`, build the full CacheKey using `lastKnown` and Get from Cache. On hit, return cached; on miss, fall through.
3. After the call returns: build the full CacheKey using `result.Model`, Put the result, and update `lastKnownByCID[clientID] = result.Model`.

First call per `clientID` always misses (no last-known yet). Model switches invalidate the cache for that clientID (lookup uses OLD id, misses, fetches, updates to NEW id). Refusals are never cached.

## Justification

- **Honors CLAUDE.md literally.** The cache key includes the chosen model id, so two model versions (e.g., the client upgrades from Haiku to Sonnet) get separate cache entries — escalation across model upgrade re-bills the new model exactly once per `(hash, level)` pair.
- **Cross-client safe.** Two clientIDs sharing the same actualModel (e.g., two separate MCP servers both running Claude) cannot leak cache entries — the clientID prefix isolates them.
- **B1 fix.** Plan-review round 1 caught the clientID-only key as a bug. The full-model-id key is the fix; `TestCache_KeyIncludesChosenModel` pins the boundary.
- **Single-clientID common case is fast.** Production wiring (`cmd/narrate-mcp` Phase 5) constructs a per-call adapter with one clientID; the lookup is one map read + one cache Get.

## Rejected alternatives

- **clientID-only key** (the original B1-flagged design). Loses model-version isolation. Rejected.
- **Provisional-key indirection layer.** Cache stores `(provisional_key → set of full keys; full_key → response)`. Get on provisional, then Get on full. More complex than the last-known-model map, with no additional safety. Rejected as over-engineering.
- **No lookup before call** (Option B above). Defeats the value of caching. Rejected.

## Consequences

- `cache.go` carries `cacheLookupState` with a `sync.RWMutex`-protected `map[string]string`. RWMutex chosen because reads (lookup) are common; writes (after each LLM call) are rare.
- The map is theoretically unbounded — one entry per distinct clientID. In practice the production path constructs a per-call adapter with a single clientID, so the map holds exactly one entry for the lifetime of that call. Documented in the cache.go doc comment; revisit if a future production path reuses adapters across many clientIDs.
- First call per clientID always misses. `TestCache_FirstCallPerClientAlwaysMisses` pins this as documented behavior — not a bug.
- Model switch invalidates the cache for that clientID. The OLD entries remain in the underlying `sync.Map` (no eviction in #13) but are unreachable via the lookup; effectively dead. Cleanup is a follow-up if memory pressure shows up.
- Refusals are not cached — `Voice()` skips the `cachePut` branch when the result is `Refused`. A transient refusal does not poison subsequent attempts.
- Per-call cache lifetime in production: the cmd/narrate-mcp speak handler constructs `mcpsampling.New(WithCache(NewInMemoryCache()))` per tool call, so the cache is per-call. Cross-call persistence (per-server cache) is a separate ticket if needed.

## Related decisions

- [2026-06-21-mcpsampling-cache-server-lifetime-lru-eviction](./2026-06-21-mcpsampling-cache-server-lifetime-lru-eviction.md) — **supersedes this decision (lifetime only).** Moves the cache from per-call to server-lifetime with LRU + entry-count-cap eviction; the full-model-id cache key + two-phase last-known-model lookup recorded here carry forward unchanged.
- [2026-06-20-mcpsampling-prompt-templates-stay-in-package-for-13](./2026-06-20-mcpsampling-prompt-templates-stay-in-package-for-13.md) — same project tolerance for "good enough for phase one" — eviction policy out of scope for #13.
- The Severity 2-value decision (2026-06-20) — diagnostics are not the path for cache-miss reporting; cache-miss is internal accounting (no `Diagnostic` emitted).

## Revisit trigger

- If a future production path reuses an Adapter across many clientIDs, the `lastKnownByCID` map becomes unboundedly large. Add an eviction policy or move to a `sync.Map` and accept eventual consistency on the lookup.
- If cross-call (per-server) cache lifetime ships, this decision's "per-call cache" consequence becomes stale — revisit the cache-thread-safety story and the eviction-policy gap together.
- If a future intelligence adapter wants a different cache-key shape (e.g., adding `req.Facts` hash), generalize `CacheKey` rather than fork.

## Source

Inline mark `**Decision (v4) — convention: experimental.**` in `planner-task.md v2` for scope `intelligence-mcpsampling-issue-13`. Plan-review round 1 finding B1 corrected the original clientID-only design; this decision records the post-B1 shape. Implemented in commit `5112441` (Phase 4 — `cache.go` two-phase lookup).
