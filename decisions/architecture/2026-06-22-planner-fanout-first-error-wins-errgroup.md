# First-error-wins via errgroup, not errors.Join, in planner fan-out

| Field    | Value            |
|----------|------------------|
| Date     | 2026-06-22       |
| Status   | accepted         |
| Category | architecture     |
| Tags     | planner, concurrency, errgroup, error-handling, stop-semantics, issue-46 |

## Context

Issue #46 parallelizes per-block intelligence Voice calls in the planner. With concurrent
workers, multiple blocks could fail at once. The pipeline's error/refusal boundary
(CLAUDE.md) says adapter I/O failure is an *error returned up the pipeline that stops it* —
distinct from a `Refusal`, which is data and never stops anything. The serial planner
returned the first error encountered and stopped. The parallel version must preserve those
same stop semantics: an error should halt the run with one error, not accumulate.

## Options considered

### Option A: errgroup first-error-wins
- **Pros**: `errgroup.Group` returns the first non-nil error and cancels the shared context,
  signalling other in-flight workers to stop. This mirrors the original sequential stop
  semantics exactly — one error, run stops. Standard idiom.
- **Cons**: Sibling errors that happened concurrently are not surfaced.

### Option B: errors.Join all worker errors
- **Pros**: Surfaces every concurrent failure together.
- **Cons**: Changes the error contract from "first error stops the run" to "aggregate of
  whatever raced to fail," which is non-deterministic in content/order and inconsistent with
  the serial planner's behavior. The pipeline only needs to know it stopped, not the full
  set.

## Decision

Use `errgroup` first-error-wins for the planner Voice fan-out. The first worker to return a
non-nil error wins; its error propagates up and the shared context is cancelled so the
remaining workers wind down. We deliberately do **not** `errors.Join` the worker errors,
because the planner's contract is sequential stop-on-first-error and aggregating concurrent
failures would change that contract for no consumer benefit (the pipeline stops either way).
Refusals remain data inside the plan and are unaffected — only true errors trip this path.

## Consequences

- Stop semantics are identical to the pre-parallelization serial planner: one error stops
  the run.
- Which specific error wins under simultaneous failures is scheduling-dependent, but that is
  acceptable — any one error is sufficient to stop, and this matches errgroup's documented
  contract.
- Diagnostics produced before the error are still slotted positionally (see related), but a
  returned error stops the pipeline before the plan is consumed.

## Related decisions

- [Diagnostics positionally slotted per-block, flattened after g.Wait()](2026-06-22-planner-fanout-positional-diagnostics-slotting.md) — same fan-out; determinism of the non-error output path.

## Revisit trigger

If callers ever need to see all concurrent failures at once (e.g. a batch validation mode),
revisit and consider an aggregating wrapper — but keep the default stop-on-first-error path.
