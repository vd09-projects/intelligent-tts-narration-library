# Read-side per-dir mutex in /artifact, keyed the same as the escalate writer, over accepting a torn-read window

| Field    | Value            |
|----------|------------------|
| Date     | 2026-06-22       |
| Status   | accepted         |
| Category | concurrency      |
| Tags     | persistent-sink, commitPatch, atomic-rename, torn-read, per-dir-mutex, filepath-abs, artifact-route, escalate, issue-62 |

## Context

Issue #62 added the read-only `GET /artifact` route that serves `{manifest.json, audio.wav}` from an escalated dir. On the write side, `internal/sink/persistent`'s `commitPatch` writes `plan.json`, `manifest.json`, and `audio.wav` each via atomic tmp+rename — so each individual file is never observed half-written. BUT the three renames happen sequentially, with `audio.wav` renamed LAST. This opens a cross-file window: a concurrent reader can observe a torn state where the new `manifest.json` (with updated offsets) is already in place while `audio.wav` is still the old blob. Per-file atomicity does not give cross-file atomicity.

## Options considered

### Option A: Hold a read-side per-dir mutex in the /artifact handler, keyed on the same key the writer holds
- **Pros**: Guarantees a reader observes a consistent triple — it cannot interleave with an in-flight `commitPatch`. The key (`filepath.Abs(dir)`) is exactly the one `/escalate`'s writer already holds, so reader and writer serialize correctly per directory.
- **Cons**: A read can briefly block behind an in-flight patch of the same dir. Negligible for a local single-user tool.

### Option B: Accept the cross-file observation window as known debt
- **Pros**: No read-side locking; reads never block.
- **Cons**: A re-fetch landing inside the rename sequence can pull a new manifest against old audio — exactly the bug the re-fetch exists to avoid. Silent, intermittent, and hard to reproduce.

## Decision

Chose Option A. The `/artifact` handler takes a read-side per-dir mutex keyed on `filepath.Abs(dir)` — the SAME key `/escalate`'s writer holds — rather than accepting the cross-file observation window as debt. Because reader and writer contend on the same per-dir lock, a reader is guaranteed to see a consistent `{plan.json, manifest.json, audio.wav}` triple: it observes the state either fully before or fully after a `commitPatch`, never mid-rename-sequence. The alternative (fixing atomicity on the write side, e.g. staging all three and swapping under one rename of a dir) was heavier than gating the read; the read-side lock closes the window with the writer's existing key.

## Consequences

- Readers see a consistent cross-file triple; the new-manifest/old-audio torn read is eliminated.
- Reads of a dir block while that dir's `commitPatch` is in flight — acceptable for a local single-user hobby tool.
- The correctness guarantee depends on the read key and the write key being identical (`filepath.Abs(dir)`); if either side ever changes its keying, the guarantee breaks silently.
- The underlying sequential-rename non-atomicity in `commitPatch` remains; the read-side lock compensates for it rather than removing it.

## Related decisions

- [Read-only GET /artifact route serves the live escalated dir](../architecture/2026-06-22-artifact-route-serves-live-dir-resolves-refetch.md) — the route this mutex guards.

## Revisit trigger

If `commitPatch` is ever made cross-file atomic (single-rename / staging-dir swap), the read-side lock could be relaxed. If the server ever serves multiple concurrent users or the artifact set grows, re-examine lock granularity.
