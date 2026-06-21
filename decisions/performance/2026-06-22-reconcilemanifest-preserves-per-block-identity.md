# reconcileManifest preserves per-block object identity for React.memo short-circuit

| Field    | Value            |
|----------|------------------|
| Date     | 2026-06-22       |
| Status   | accepted         |
| Category | performance      |
| Tags     | player, escalate, reconcileManifest, react-memo, BlockRow, rerender, identity, issue-50 |

## Context

Task #50, React player. On an escalate patch, the manifest is reconciled into new state. A requirement was "zero re-render on other rows" — patching one block must not re-render the sibling `BlockRow`s. `BlockRow` is wrapped in `React.memo`, which short-circuits a re-render only when its props are referentially equal to the previous render.

## Options considered

### Option A: Wholesale manifest overwrite
- **Pros**: Simplest reconcile — replace the whole manifest with the freshly fetched one.
- **Cons**: Every per-block object gets a new reference, so `React.memo(BlockRow)` sees changed props for all rows and re-renders the entire list — violates "zero re-render on other rows".

### Option B: reconcileManifest preserves per-block object identity
- **Pros**: For each block, return the prior object reference when the new block is deep-equal to it; only the genuinely-changed (patched) block gets a new reference. `React.memo(BlockRow)` then short-circuits every unchanged sibling — only the patched row re-renders.
- **Cons**: Reconcile must deep-equal each block and hold prior references; more logic than a blind overwrite.

## Decision

Chose Option B. `reconcileManifest` preserves per-block object identity — it returns the prior reference when a block is deep-equal — so `React.memo(BlockRow)` short-circuits unchanged siblings and only the patched row re-renders. Chosen over wholesale manifest overwrite to satisfy "zero re-render on other rows".

## Consequences

- Patching one block re-renders exactly one `BlockRow`; siblings short-circuit.
- reconcileManifest deliberately changes the TOP-LEVEL manifest object identity each patch (so React reconciles the root) while preserving per-block identities — this top-level churn is why tracking-reset must NOT key on top-level identity (see related).
- Reconcile carries deep-equal cost per block; negligible at document scale, but a cost nonetheless.

## Related decisions

- [usePlayback tracking-reset keyed on sorted block-id signature, not top-level manifest identity](../architecture/2026-06-22-useplayback-reset-on-block-id-signature-not-manifest-identity.md) — consequence of this reconcile changing top-level identity per patch.
