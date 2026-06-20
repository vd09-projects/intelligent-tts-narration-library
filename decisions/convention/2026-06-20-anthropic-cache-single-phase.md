# Anthropic cache is single-phase (no last-known-actual-model lookup)

- **Date:** 2026-06-20
- **Status:** experimental
- **Category:** convention
- **Tags:** [intelligence, anthropic, cache, model-id, simplification, issue-15]
- **Owner:** vd
- **Scope:** issue-15

## Context

mcpsampling's cache is two-phase because the MCP client picks the actual model id and the adapter only learns it after the first CreateMessage reply. The pre-call key cannot include the actual model until a previous call has populated a per-clientID `lastKnownByCID` map.

The Anthropic Messages API is the opposite: the adapter chooses `a.model` at construction. The pre-call key is fully knowable.

## Options considered

### Option A: Single-phase, key uses configured `a.model` on Get and Put (CHOSEN)
- **Pros**: Simpler. No lookup-state map. Cache hits return immediately without a per-clientID indirection. First call hits cache on the second call (no first-call-always-misses-for-this-clientID property).
- **Cons**: If Anthropic silently moves the resolved model under a stable alias (e.g. `claude-haiku-4-5` → newer dated version), cached entries become stale until the adapter is reconstructed. Acceptable: production wiring constructs a fresh per-call cache, so staleness is bounded by one pipeline run.

### Option B: Mirror mcpsampling's two-phase with a single-entry degenerate map
- **Pros**: Identical shape to mcpsampling makes a future lift cleaner.
- **Cons**: All the ceremony of a per-clientID map for one entry. First call always misses (no last-known yet). Dead complexity.

### Option C: Cache also stores actualModel; cache hit returns the actualModel
- **Pros**: Cache hit Model field exactly matches first-call Model field.
- **Cons**: Cache `Value` type grows. The Model field on cache hit using configured `a.model` is documented and acceptable — the actual model id is folded into the cache key, so a hit by definition matched.

## Decision

intelligence/anthropic/cache.go is single-phase:
- `Cache` interface: `Get(CacheKey) (string, bool)` / `Put(CacheKey, string)`. Value type is plain string.
- `CacheKey{ContentHash, Level plan.Level, Model string}`.
- `cacheLookupState{ContentHash, Level plan.Level}` — carries hash + level across to cachePut so we do not rehash.
- `cacheGet` / `cachePut` both use `fullModelString(a.model)` for the `Model` component.
- Cache hit Voice() returns `IntelligenceResult{Text: hit, Model: fullModelString(a.model)}`; first-call Voice() returns `IntelligenceResult{Text: text, Model: fullModelString(parsed.Model)}` (actual model from API response). Divergence is intentional.

## Consequences

- intelligence/anthropic/cache.go is shorter than mcpsampling/cache.go by ~40 LOC.
- Test `TestVoice_FirstCallMisses_SecondCallHits` asserts `second.Model == fullModelString(a.model)`, NOT `first.Model == second.Model`. The Model-field divergence is contract.
- Future lift to internal/cache/ would have to handle both shapes (two-phase MCP, single-phase Anthropic). Per Decision v2 the lift is deferred until a 3rd adapter forces it.
- Server-side alias updates produce stale entries until the adapter is reconstructed — bounded by per-call cache lifetime in the production wiring.

## Related decisions

- [Cache machinery duplicate-not-lift between mcpsampling and anthropic](2026-06-20-cache-machinery-duplicate-not-lift.md) — broader principle.
- [Anthropic intelligence caching key shape](https://CLAUDE.md/#per-block-leveling) — the `(content_hash, level, model)` key rule from CLAUDE.md.

## Revisit trigger

If the production wiring switches to a long-lived (cross-call) cache, the stale-entry-on-alias-update case becomes real. Either pin the resolved actual model in the key, or add explicit cache invalidation.
