# Map a real ErrNothingToPatch to a 4xx (Option A); do NOT add already-at-level detection to persistent.PatchBlock (Option B rejected)

| Field    | Value            |
|----------|------------------|
| Date     | 2026-06-21       |
| Status   | accepted         |
| Category | error-handling   |
| Tags     | cmd/narrate-server, http-escalate, errnothingtopatch, persistent-sink, patchblock, idempotency, dependency-contract, issue-49 |

## Context

Issue #49 adds an HTTP escalate endpoint (`cmd/narrate-server`) that re-renders one block at a higher level into an existing persistent-sink output directory, reusing `persistent.PatchBlock`.

The implementation plan's B3/B5 steps assumed an *idempotency premise*: that escalating a block to a level it is already at would surface as `ErrNothingToPatch`, which the handler could treat as a benign no-op. Build review found that premise was **false against the live `persistent.PatchBlock` contract**. In reality `PatchBlock` returns `ErrNothingToPatch` only when the target persistent-sink directory is **incomplete** (a missing/partial output dir), **not** when a block is already at the requested level. A same-level escalate does not hit `ErrNothingToPatch` at all — it flows through the normal patch path and re-renders content-identical bytes.

This forced a decision: how should the endpoint treat an actual `ErrNothingToPatch`, and should `persistent.PatchBlock` be changed to add already-at-level detection so the original idempotency premise becomes true?

## Options considered

### Option A: Map a real ErrNothingToPatch to a 4xx source_not_found-class error; leave PatchBlock unchanged
- **Pros**: `ErrNothingToPatch` genuinely means "there is no complete prior output to patch into" — a caller/precondition problem, correctly a 4xx (source_not_found class). Same-level-escalate convergence is documented as happening via a content-identical re-render through the happy path (no special case needed). Does not touch a dependency's contract. Smallest, most honest change.
- **Cons**: Same-level escalate does a real re-render (a wasted render pass) instead of being short-circuited. The endpoint relies on a documented-but-implicit convergence behavior rather than an explicit no-op.

### Option B: Add already-at-level detection to persistent.PatchBlock so it returns ErrNothingToPatch for a same-level request
- **Pros**: Would make the original B3/B5 idempotency premise true; same-level escalate becomes a cheap explicit no-op.
- **Cons**: Changes the contract of a shared dependency (`persistent.PatchBlock`) — `ErrNothingToPatch` would take on a second, overloaded meaning. The sink would need to know "current level" semantics it currently doesn't carry. Marginal benefit (avoiding one re-render) on a local-only hobby tool. Risks rippling into the existing `--block` offline patch path (issue #28) that also uses `PatchBlock`.

## Decision

Adopt **Option A**. Map a real `ErrNothingToPatch` from `persistent.PatchBlock` to a **4xx source_not_found-class** HTTP error in the escalate handler, since that sentinel actually signals an incomplete/absent prior output (a precondition failure), not an idempotent no-op. Document that **same-level escalate convergence happens via a content-identical re-render through the happy path** — no special-casing in either the handler or the sink.

Explicitly **reject Option B**: do not modify `persistent.PatchBlock` to add already-at-level detection. Overloading a dependency's error contract to recover a marginal re-render saving is the wrong trade for a local hobby tool, and it would risk the existing offline `--block` patch path.

The load-bearing reasoning: keep the dependency contract honest and single-meaning; let the wasted same-level re-render stand as an acceptable cost rather than complicate the sink.

## Consequences

- A same-level escalate request performs a full (content-identical) re-render rather than short-circuiting. Acceptable cost on a local tool.
- `ErrNothingToPatch` retains its single meaning ("no complete prior output to patch into") across both the HTTP escalate path and the offline `--block` path.
- The endpoint depends on the documented convergence behavior; a future contributor must not "optimize" same-level escalate by reintroducing the false idempotency premise.

## Related decisions

- [--block patch into a persistent outDir: manifest is the INDEX, byte ranges are DERIVED](../convention/2026-06-21-persistent-block-patch-manifest-index-derived-ranges.md) — defines the `persistent.PatchBlock` / Route A behavior this endpoint reuses.

## Revisit trigger

Revisit if `persistent.PatchBlock` gains a real notion of a block's current level for another reason (then an explicit same-level no-op becomes nearly free), or if same-level re-render cost ever becomes observable in practice.
