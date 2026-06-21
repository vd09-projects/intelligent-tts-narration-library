# usePlayback tracking-reset keyed on sorted block-id signature, not top-level manifest identity

| Field    | Value            |
|----------|------------------|
| Date     | 2026-06-22       |
| Status   | accepted         |
| Category | architecture     |
| Tags     | player, escalate, usePlayback, react, reconcileManifest, identity, highlight, issue-50 |

## Context

Task #50, React player. `usePlayback` resets its active-block tracking when the loaded directory changes (a genuinely new document means the old highlight/playhead are meaningless). The naive reset signal is the identity of the top-level manifest object.

But escalation introduced `reconcileManifest`, which deliberately changes the top-level manifest object identity on EVERY patch (so React sees a new root and reconciles). If the tracking-reset keyed on bare top-level object identity, every patch would look like a directory swap and wipe the paused-block highlight — failing requirement R7 (preserve the paused highlight across a patch).

## Options considered

### Option A: Reset on bare top-level manifest object identity
- **Pros**: Trivially simple.
- **Cons**: reconcileManifest changes top-level identity on every patch by design, so this fires on every escalation and wipes the R7 paused highlight. Conflates "patched same document" with "loaded a new document".

### Option B: Reset on a sorted block-id signature (directory-swap signal)
- **Pros**: A patch reuses the same set of block ids, so the signature is stable across patches — highlight survives (R7). A true directory swap changes the block-id set, so the signature changes and the reset fires correctly.
- **Cons**: Slightly more work than an identity check; assumes the block-id set is the right swap discriminator.

## Decision

Chose Option B. `usePlayback` keys its tracking-reset on a sorted block-id signature — the real "this is a different document" signal — NOT on top-level manifest object identity. This survives the deliberate per-patch identity churn from reconcileManifest and preserves the paused highlight (R7). Rejected the bare object-identity reset.

## Consequences

- R7 satisfied: paused highlight survives escalation patches.
- Reset still fires correctly on a real directory swap (different block-id set).
- Couples the reset signal to the assumption that a patch never changes the block-id set (true for in-place escalation; splitting/merging blocks would change it — out of scope phase one).

## Related decisions

- [reconcileManifest preserves per-block object identity for React.memo short-circuit](../performance/2026-06-22-reconcilemanifest-preserves-per-block-identity.md) — same reconcile path; this entry is why the top-level identity is NOT a safe reset key.
- [Player audio sync via requestAnimationFrame; React state writes only on block transition](../convention/2026-06-21-player-raf-audio-sync-transition-only-rerender.md) — the tracking state being reset lives here.

## Revisit trigger

If a future patch operation can add or remove block ids (e.g. block splitting/merging on escalation), the sorted-block-id signature would mis-fire as a directory swap — revisit the reset discriminator then.
