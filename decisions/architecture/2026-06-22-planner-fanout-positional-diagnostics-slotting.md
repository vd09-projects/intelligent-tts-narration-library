# Diagnostics positionally slotted per-block, flattened after g.Wait() in index order

| Field    | Value            |
|----------|------------------|
| Date     | 2026-06-22       |
| Status   | accepted         |
| Category | architecture     |
| Tags     | planner, concurrency, errgroup, determinism, race-freedom, diagnostics, issue-46 |

## Context

Issue #46 parallelizes the per-block intelligence Voice calls in the planner so that
N blocks needing L2/L3 enrichment fan out concurrently instead of running serially.
The planner is a pure, no-I/O package whose output (the narration plan, including its
`Diagnostics`) must be deterministic: the same input must always produce byte-identical
output regardless of goroutine scheduling. Concurrent workers each produce zero or more
diagnostics for their block, and those need to land in the plan in a stable, reproducible
order.

## Options considered

### Option A: Each goroutine appends its diagnostics directly to the shared `out.Diagnostics` slice
- **Pros**: Less plumbing; no per-block result struct.
- **Cons**: Concurrent append to a shared slice is a data race (requires a mutex), and
  even with a mutex the resulting order is scheduling-dependent and non-deterministic.
  Breaks the planner's determinism guarantee and the golden `plan.json` fixtures.

### Option B: Positionally slot results per-block, flatten after the barrier
- **Pros**: Each worker writes only to its own index in a pre-sized `[]blockResult`
  (`blockResult{block, diags}`), so there is no shared mutable state and no race.
  After `g.Wait()` the main goroutine flattens results in index order, producing a
  deterministic diagnostics sequence identical to the old serial order.
- **Cons**: Requires a small result-carrier struct and a flatten pass.

## Decision

Each fan-out worker writes its block and its diagnostics into a positionally-owned
`blockResult{block, diags}` entry indexed by the block's position. Workers never touch
`out.Diagnostics`. Only after the `errgroup` barrier (`g.Wait()`) does the main goroutine
flatten the `[]blockResult` into `out.Diagnostics` in index order.

This keeps the planner race-free (each goroutine owns a disjoint slice index — no shared
write target) and deterministic (final ordering is index order, not completion order),
preserving both the no-I/O/purity invariant and the golden-fixture contract.

## Consequences

- Diagnostics ordering is stable and matches the pre-parallelization serial order, so
  golden `plan.json` fixtures remain valid without re-baselining for order.
- The `blockResult` carrier is the canonical pattern for any future per-block fan-out in
  the planner; new concurrent passes should slot positionally rather than append to shared
  output.
- `go test -race` on the planner stays clean.

## Related decisions

- [First-error-wins via errgroup, not errors.Join, in planner fan-out](2026-06-22-planner-fanout-first-error-wins-errgroup.md) — same fan-out mechanism; complementary error-handling decision.

## Revisit trigger

If the planner ever needs to emit diagnostics in completion/streaming order (e.g. a
progress feed), revisit — but that would also reopen the determinism invariant.
