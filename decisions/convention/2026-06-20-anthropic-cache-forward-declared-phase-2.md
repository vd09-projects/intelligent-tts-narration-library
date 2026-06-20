# Anthropic Cache forward-declared in Phase 2, narrowed in Phase 4

- **Date:** 2026-06-20
- **Status:** experimental
- **Category:** convention
- **Tags:** [intelligence, anthropic, cache, interface, phased-build, issue-15]
- **Owner:** vd
- **Scope:** issue-15

## Context

The #15 build is split across 7 phases. Phase 2 scaffolds the `Adapter` struct + `Option` closures; Phase 4 introduces the real `Cache` interface + concrete `inMemoryCache`. The `WithCache(c Cache)` Option in Phase 2 needs a `Cache` type to refer to, but the real interface (with `CacheKey` arg) does not yet exist.

## Options considered

### Option A: Forward-declare `Cache` as `interface { Get(any) (string, bool); Put(any, string) }` in Phase 2's anthropic.go (CHOSEN)
- **Pros**: Phase 2 scaffold compiles cleanly. Named type at the `WithCache(c Cache)` call site is self-documenting. Phase 4 narrows the interface in cache.go and removes the forward-declaration in a single edit.
- **Cons**: The forward-declared interface is briefly wider than the final one (uses `any` instead of `CacheKey`). The narrowing in Phase 4 is a breaking change to any code that imported the wide form between Phase 2 and Phase 4 — acceptable because the only consumer is the same package.

### Option B: Type the field as `any` in Phase 2, name it properly in Phase 4
- **Pros**: No forward-declaration to delete.
- **Cons**: `WithCache(c any) Option` is information-poor at the call site. Loses the IDE / godoc benefit of a named type. The narrowing in Phase 4 is no smaller.

### Option C: Bundle Phase 2 + Phase 4 into one commit
- **Pros**: No bridge type at all.
- **Cons**: Per-phase commits is the user's explicit feedback rule. Violating it for a one-line forward-declaration cost is wrong tradeoff.

## Decision

Phase 2 forward-declares `Cache` as a wide interface in anthropic.go. Phase 4 removes the forward-declaration when cache.go lands with the narrow interface.

## Consequences

- intelligence/anthropic/anthropic.go briefly carried a `Cache interface { Get(any) (string, bool); Put(any, string) }` declaration between commits ab29da8 (Phase 2) and 0d57d0b (Phase 4).
- Phase 4's commit explicitly removes the forward-declaration as part of the cache.go landing, so the package always has exactly one `Cache` definition at any point in history.
- The narrowing changed the interface's method signatures — caught immediately by the build (no external consumers existed).

## Related decisions

- (none — this is a one-off phased-build pattern)

## Revisit trigger

If the project formalizes a phased-build pattern across more PRs, codify the forward-declare/narrow approach as a convention. Otherwise, treat as one-off and let it age out.
