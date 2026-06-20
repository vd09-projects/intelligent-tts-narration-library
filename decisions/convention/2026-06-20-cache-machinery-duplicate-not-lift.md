# Cache machinery duplicate-not-lift between mcpsampling and anthropic

- **Date:** 2026-06-20
- **Status:** experimental
- **Category:** convention
- **Tags:** [intelligence, anthropic, mcpsampling, cache, code-reuse, issue-15]
- **Owner:** vd
- **Scope:** issue-15

## Context

mcpsampling's `Cache` interface, `CacheKey`, `inMemoryCache`, `cacheLookupState`, `hashContent`, `fullModelString`, two-phase `cacheGet`/`cachePut` are reusable in shape. anthropic ships its own near-mirror.

The key shape differs: mcpsampling's Model field is `"mcp-sampling@<clientID>/<actualModel>"` (clientID-scoped); anthropic's is `"anthropic@<model>"` (no clientID). Lifting forces either a `FullModelFormat` slot per adapter or pulls clientID semantics into a shared package that does not need them.

## Options considered

### Option A: Duplicate the cache (CHOSEN)
- **Pros**: Each adapter owns its model-string format. No coordination tax on the shared package. ~80 LOC of duplication is bounded and grep-auditable.
- **Cons**: Drift risk over time; eviction policy when added has to land twice.

### Option B: Lift `Cache` + `CacheKey` + helpers to `internal/cache/` (or similar)
- **Pros**: Single source of truth.
- **Cons**: Either a `FullModelFormat string` slot on each adapter or `cacheKey.Model` becomes adapter-formatted-on-the-way-in — both add coupling for what is essentially formatting. Phase 1 lift was already a stretch.

### Option C: Skip cache entirely on anthropic
- **Pros**: Simplest.
- **Cons**: Violates CLAUDE.md's intelligence-caching rule ("escalation must not re-bill") for the CLI intelligence path. Non-starter.

## Decision

anthropic gets its own `cache.go` mirroring mcpsampling. Lift when a 3rd adapter materializes — same "two consumers before lift" principle.

## Consequences

- intelligence/anthropic/cache.go is a near-mirror of mcpsampling/cache.go.
- Eviction-policy work has to land in both packages when it ships.
- The single-phase simplification (see related decision) makes the two implementations not identical — lifting would have to handle both the two-phase MCP shape and the single-phase Anthropic shape.

## Related decisions

- [Refusal-parser duplicate-not-lift between mcpsampling and anthropic](2026-06-20-refusal-parser-duplicate-not-lift.md) — same principle.
- [Anthropic cache is single-phase](2026-06-20-anthropic-cache-single-phase.md) — simplification that further argues against premature lift.

## Revisit trigger

When a 3rd intelligence adapter is proposed.
