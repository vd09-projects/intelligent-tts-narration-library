# Earshot resume persists block identity only (blockId + block-signature), never an ms offset

| Field    | Value            |
|----------|------------------|
| Date     | 2026-06-29       |
| Status   | accepted         |
| Category | convention       |
| Tags     | issue-112, earshot, resume, localstorage, block-level-sync, block-identity, block-signature, schema-version, self-healing, no-word-timing, no-ms-offset, playback-engine |

## Context

#112's resume feature must remember where a listener stopped so it can restore on next load. The tempting representation is a millisecond offset into the audio. But a stored ms offset is a word/time-level position — it contradicts the block-level-sync-only invariant (sync is keyed by `block_id`; spoken text ≠ source text under gist mode, so sub-block timing is forbidden), and it goes stale the moment the block is re-rendered (escalation reflows downstream offsets).

## Decision

The localStorage resume entry stores block IDENTITY only:

- `blockId`
- `blockOrder`
- `blockSignature`
- `schemaVersion`

`startMs` is re-derived LIVE from the current timeline on restore — it is never persisted. On a `blockSignature` or `schemaVersion` mismatch the entry is dropped (self-healing): a stale or schema-incompatible resume entry silently discards rather than restoring to a wrong position.

This upholds the block-level-sync-only invariant for the persistence layer — the saved position is a block reference, not a time offset.

## Consequences

- Resume survives re-render / escalation reflow: the offset is re-derived from whatever the current timeline says for that block.
- Signature/schema mismatch self-heals by dropping the entry rather than restoring to a fabricated or stale offset.
- Resume granularity is block-level only — a listener resumes at the start of the saved block, never mid-block. Consistent with the no-word-timing invariant.

## Related decisions

- [Earshot restore gate clears only on a landed seek](../architecture/2026-06-29-restore-gate-clears-only-on-landed-seek.md) — the restore path that consumes this saved identity and re-derives startMs.
- [usePlayback reset keyed on block id + signature, not manifest identity](../architecture/2026-06-22-useplayback-reset-on-block-id-signature-not-manifest-identity.md) — same block-id + signature keying applied to reset.
- [Earshot parallel per-entry narration model](../architecture/2026-06-29-earshot-parallel-per-entry-narration-model.md) — #112 resume persistence is the localStorage superset of that in-memory per-entry key.

## Revisit trigger

If product ever demands sub-block resume (resume mid-block), this convention and the block-level-sync-only invariant both have to be revisited together.
