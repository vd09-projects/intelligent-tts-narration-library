# Wire intelligence into table meaning-summary at L2/L3 (L1 stays deterministic)

| Field    | Value            |
|----------|------------------|
| Date     | 2026-06-22       |
| Status   | accepted         |
| Category | tradeoff         |
| Tags     | planner, levelTable, intelligence, leveling, cost-model, structured-classes, honesty-rule, issue-47 |

## Context

`planner/level.go` `levelTable` is fully deterministic at every level — never calls intelligence:
- L1: `A 3-column, 5-row table.` (shape only)
- L2: headers + first row + last row
- L3: every row read raw — `Row: a, b, c.`

A table is never *interpreted* — only its shape and raw cells are read aloud. Contrast `levelDiagram`, which already sets `needsIntelligence=true` at L2/L3 and gets an AI explanation. Result: "what does this table mean / what is it showing" is never produced. A pricing table, comparison matrix, or config grid is spoken as a raw cell sequence — hard to follow by ear.

This blocked issue #47. The open question was whether moving tables to token-billed intelligence at L2/L3 is a new cost model (needs sign-off) or a consistency fix.

## Options considered

### Option A: Intelligence at L2/L3 + deterministic degrade (chosen)
- **Pros**: Mirrors `levelDiagram` exactly — closes a real inconsistency, not a new cost model. L1 stays free/instant/deterministic. Honesty rule preserved via degrade. Best meaning-by-ear win for tables.
- **Cons**: Bills tokens at L2/L3 for a class that was previously zero-cost at all levels.

### Option B: Intelligence at L3 only
- **Pros**: Smallest token footprint; L2 stays deterministic header/row reading.
- **Cons**: Diverges from `levelDiagram` (which enriches at L2 too) — leaves a smaller version of the same inconsistency. L2 listener still gets no meaning.

### Option C: Reject — keep fully deterministic
- **Pros**: Zero cost, no change.
- **Cons**: Tables remain the one structured class with no interpretation path; diagrams stay privileged for no principled reason.

## Decision

**Option A.** L1 stays deterministic (shape only). L2/L3 set `needsIntelligence=true` and pass table facts (cols, rows, headers) via `IntelligenceRequest.Facts`; adapter returns a meaning summary (e.g. "a 3-tier pricing comparison; enterprise adds SSO and audit logs"). With no intelligence adapter wired, degrade to the current deterministic header/row reading — never fabricate.

Rationale: this aligns with the existing CLAUDE.md rule *"intelligence enriches L2/L3 for structured classes but never blocks voicing."* Diagrams already do exactly this. Tables are therefore an inconsistency to fix, not a new cost model to justify — which is why this clears as `accepted` rather than needing the same level of sign-off as #48.

## Consequences

- Tables move from zero-cost-at-all-levels to token-billed at L2/L3 (L1 still free).
- Caching by `(block content hash, level, model)` applies — escalation doesn't re-bill.
- New scope to build: `levelTable` change + `degrade.go` fallback + `internal/intelligencetmpl` table prompt + golden fixtures with and without intelligence.
- L1 deterministic property of the leveling system is untouched.

## Related decisions

<!-- levelDiagram's existing L2/L3 intelligence wiring is the precedent this mirrors. -->

## Revisit trigger

If the L1-deterministic property is ever extended to L2 (i.e. a future decision pushes free tiers higher), re-examine whether table L2 should drop back to deterministic.
