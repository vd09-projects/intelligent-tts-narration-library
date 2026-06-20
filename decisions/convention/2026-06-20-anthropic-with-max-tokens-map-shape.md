# WithMaxTokens uses a map shape (partial-override) on the anthropic adapter

- **Date:** 2026-06-20
- **Status:** accepted
- **Category:** convention
- **Tags:** [intelligence, anthropic, options, max-tokens, issue-15]
- **Owner:** vd
- **Scope:** issue-15

## Context

mcpsampling's `WithMaxTokens(l1, l2, l3 int64)` takes three positional ints — callers tuning a single level have to re-state the other two. anthropic's option shape was up for redesign.

## Options considered

### Option A: `WithMaxTokens(m map[plan.Level]int)` — partial override (CHOSEN)
- **Pros**: Caller supplies only the level(s) they want to change. Unspecified levels keep their defaults. Aligns with `WithPromptTemplates(map[plan.Class]intelligencetmpl.PromptTemplate)` map-shape sibling. Non-positive values are silently ignored (defensive).
- **Cons**: Map iteration order matters nowhere, but the type is heavier than three ints at the call site.

### Option B: `WithMaxTokens(l1, l2, l3 int)` — positional, mirrors mcpsampling
- **Pros**: Symmetric with mcpsampling.
- **Cons**: Forces callers tuning one level to re-state the other two — bug-prone.

### Option C: Three separate options `WithMaxTokensL1(int)`, etc.
- **Pros**: Maximally explicit per-level.
- **Cons**: Three new symbols. Verbose for the common "tune all three" case.

## Decision

`WithMaxTokens(m map[plan.Level]int)`. Iterates the supplied map; for each entry with `n > 0` writes `a.maxTokens[level] = n`. Unspecified levels keep their defaults (L1 80, L2 240, L3 600).

## Consequences

- intelligence/anthropic/options.go's WithMaxTokens differs in shape from mcpsampling's. Documented in the godoc.
- The asymmetry is acceptable because the two adapters have different ergonomic stories — mcpsampling is constructed once per call (so re-stating all three is cheap); anthropic is constructed once and tuning a single level is the realistic case.
- If a future lift consolidates the two adapters' options, the map shape generalizes (the positional shape doesn't).

## Related decisions

- (none directly — sibling to other Phase 2 Options)

## Revisit trigger

If mcpsampling.WithMaxTokens migrates to the map shape, the asymmetry resolves. Until then, the two adapters' WithMaxTokens shapes are intentionally different.
