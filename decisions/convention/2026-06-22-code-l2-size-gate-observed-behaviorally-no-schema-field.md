# Code L2 size-gate is observed behaviorally (no `size_gated` plan field)

| Field    | Value            |
|----------|------------------|
| Date     | 2026-06-22       |
| Status   | accepted         |
| Category | convention       |
| Tags     | planner, levelCode, size-gate, plan-schema, additive-compatible, no-io-invariant, honesty-rule, observability, issue-48 |

## Context

Issue #48 added an AI semantic gist for code blocks at L2 (recorded in
[the L2-only tradeoff](../tradeoff/2026-06-22-code-semantic-gist-l2-only.md)).
That decision included a **size-gate**: very large code blocks (over
`codeGistMaxLines`, ~250 lines) skip the LLM call at L2 and voice the
deterministic count+decls gist directly, so the system never bills tokens /
latency on an oversized block.

The implementation question was: **how does the rest of the system (and a
reader of the plan) know a block was size-gated rather than AI-gisted?**
The obvious move is a new `plan.Block`/facts field — e.g. `size_gated: true`
or a Facts/Diagnostic entry — so the gate is explicit in the artifact.

Two constraints push back on that:

- **Additive-schema rule.** `plan/` stays additive-compatible within a major
  `schema_version`; every new field is permanent surface that all consumers
  must tolerate.
- **`planner/` purity / no-I/O invariant.** `levelCode` is pure and emits a
  `levelResult`; adding a diagnostic field threads new state through the
  planner for a condition that has no consumer asking for it.

## Options considered

### Option A: Observe the gate behaviorally (chosen)
A size-gated block is indistinguishable in the plan from any other
deterministically-voiced block: `Status = voiced`, deterministic segment text
(the shared count+decls gist), and simply **no AI reply** because no
intelligence request was emitted. The gate lives entirely in `levelCode`
control flow (`nLines > codeGistMaxLines` returns a fully-voiced deterministic
`levelResult` instead of `needsIntelligence=true`). No `size_gated` fact is
ever emitted — and a test asserts that absence.

- **Pros**: Zero new schema surface; additive rule untouched. `planner/`
  stays pure — no diagnostic state threaded through. The gate is an internal
  optimization, not a contract; consumers already handle "voiced +
  deterministic + no AI" for every other deterministic path. Honest: the
  block really is voiced at the deterministic gist, which is exactly what the
  plan says.
- **Cons**: An operator cannot read off the plan "this block hit the gate vs.
  was gisted by the adapter" — they'd infer it from size + deterministic text.
  Observability is behavioral, not declared.

### Option B: Add a `size_gated` Facts/Diagnostic schema field
- **Pros**: Explicit, machine-readable "this block was gated" signal.
- **Cons**: Permanent additive-schema surface for an internal optimization
  with no consumer requesting it; threads diagnostic state through the pure
  planner; invites every consumer to branch on a field that means nothing to
  the listener. Speculative.

## Decision

**Option A — the size-gate is observed behaviorally**, never via a schema
field. Over the gate, `levelCode` returns `Status = voiced` with the
byte-identical deterministic count+decls gist and emits **no** intelligence
request; under the gate it sets `needsIntelligence=true`. A test
(`TestLevel_CodeL2Gate`) pins the boundary at exactly `codeGistMaxLines` AND
asserts that no `size_gated` fact ever leaks into `res.facts`.

Rationale: the gate is an internal cost optimization, not part of the
narration contract. Two load-bearing invariants — the additive-schema rule and
the planner's purity / no-I/O property — both argue against minting a field
nobody consumes. The behavioral signature (voiced + deterministic + absent AI
reply) is already a first-class, well-handled state in the system, so the gate
costs zero new surface.

## Consequences

- `plan/` schema unchanged; no `size_gated` field exists, by design.
- `planner/` purity preserved — the gate is pure control flow in `levelCode`.
- Anyone debugging "why didn't this big code block get an AI gist?" must know
  the gate exists and is behavioral; the doc comment in `level.go` and the
  test comment in `level_test.go` carry that knowledge so the absence reads as
  intentional, not a bug.
- If a real consumer (e.g. an operator dashboard) ever needs to *declare*
  gated blocks, an additive fact can be introduced then — with a consumer to
  justify it.

## Related decisions

- [AI semantic gist for code at L2 only](../tradeoff/2026-06-22-code-semantic-gist-l2-only.md) — the parent decision that introduced the size-gate; this entry records how the gate is surfaced (or deliberately not).
- [Code L2 deterministic fallback shared by degrade and size-gate paths](2026-06-22-code-l2-deterministic-gist-shared-helper.md) — the shared helper whose byte-identical output makes the gated block indistinguishable from the no-adapter degrade path.

## Revisit trigger

If a real consumer needs to distinguish size-gated blocks from
adapter-gisted blocks in the artifact (operator tooling, metrics), revisit
and add an additive Facts entry — justified by that consumer, not speculative.
