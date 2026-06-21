# AI semantic gist for code at L2 only (keep L1 free/instant/deterministic)

| Field    | Value            |
|----------|------------------|
| Date     | 2026-06-22       |
| Status   | accepted         |
| Category | tradeoff         |
| Tags     | planner, levelCode, intelligence, leveling, cost-model, deterministic-l1, core-invariant, honesty-rule, issue-48 |

## Context

`planner/level.go` `levelCode`:
- **L1 (default gist):** `A 15-line Go code block.` — line count only.
- **L2:** + `Declares foo, bar, baz.` — deterministic top-level decl enumeration.
- **L3:** `needsIntelligence=true` → AI explains line-by-line meaning (only with an adapter; else downshifts to L2).

A real "what this code does" gist ("retries the request with exponential backoff") only appears at L3 and only with intelligence. At the default L1, a listener hears just a line count — not enough to decide whether to deep-dive.

This blocked issue #48. The proposal — let code blocks get an AI one-line meaning-gist at lower levels — is a deliberate change to a core invariant (CLAUDE.md domain rule): *"Deterministic L1 for structured classes. code / config / table / … voice at all levels with no intelligence adapter. Only prose truly needs the adapter; intelligence enriches L2/L3 for structured classes but never blocks voicing."* Adding AI to code **L1** trades the free / instant / zero-token L1 property — the whole cost-control story of the leveling system — for tokens + latency on every code block in every document.

## Options considered

### Option 1: Enrich at L2 only, keep L1 free (chosen)
- **Pros**: Smallest departure; consistent with "enriches L2/L3". L1 stays free/instant/zero-token — core invariant intact. Honesty fallback to deterministic count/decls when no adapter.
- **Cons**: Default-level listener still gets only a line count; meaning gist requires escalating to L2.

### Option 2: Enrich at L1 behind an opt-in flag (default off)
- **Pros**: Preserves the free L1 default; power users opt into richer L1.
- **Cons**: Bigger code surface (flag plumbing, two L1 behaviors to test); meaning gap remains for the default path.

### Option 3: Full L1 AI gist by default
- **Pros**: Biggest UX win — meaning at the default level.
- **Cons**: Biggest cost-model change; bills tokens + latency on every code block in every doc by default. Directly breaks the deterministic-L1 invariant. Needs explicit sign-off.

## Decision

**Option 1 — enrich at L2 only.** L1 keeps today's deterministic count. L2 sets `needsIntelligence=true` and gets an AI one-line meaning gist; honesty rule preserved by falling back to deterministic count/decls when no adapter is wired. Possibly size-gated (skip the AI call for very large blocks, ~200–300+ lines, and keep the deterministic structural gist there).

Rationale: the deterministic-L1 invariant is load-bearing — it is the free/instant/zero-token property that makes the whole leveling system's cost story work. Option 1 delivers the meaning-gist win one escalation away while leaving that invariant fully intact, and is consistent with the existing "enriches L2/L3" framing (same shape as the #47 table decision). Option 3 was explicitly rejected as breaking the invariant by default; Option 2 was rejected as more surface for no L1 benefit the user wanted.

## Consequences

- Core invariant "Deterministic L1 for structured classes" stays true — confirmed, not changed.
- Code L2 moves from deterministic decl-enumeration to token-billed gist (with deterministic fallback). L1 unchanged.
- Caching by `(block content hash, level, model)` applies; escalation doesn't re-bill.
- New scope to build: `levelCode` L2 change + `degrade.go` fallback + `internal/intelligencetmpl` code-gist prompt + optional size-gate + golden fixtures.

## Related decisions

- [Wire intelligence into table meaning-summary at L2/L3](2026-06-22-table-meaning-summary-via-intelligence-l2-l3.md) — sibling leveling/cost-model decision; same "enrich L2/L3, keep L1 deterministic, degrade without adapter" shape.

## Revisit trigger

If user demand for meaning-at-default-level grows, revisit Option 2 (L1 opt-in flag) or Option 3 (L1 default with explicit sign-off on the cost-model change).
