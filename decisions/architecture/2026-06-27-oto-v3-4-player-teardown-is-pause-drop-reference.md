# oto v3.4 player teardown is Pause()+drop-reference (GC finalizer), not Close()

| Field    | Value            |
|----------|------------------|
| Date     | 2026-06-27       |
| Status   | accepted         |
| Category | architecture     |
| Tags     | listen-path, oto, ebitengine, oto-v3.4, player-teardown, pause, finalizer, gc, no-close, sa1019, listenPlayer-seam, bytes-reader, bounded-retention, issue-101, issue-100 |

## Context

Issue #101 productionizes the single-path oto v3 listen player, applying the existing finding `2026-06-27-oto-v3-4-player-close-no-op-finalizer-teardown` to the shipped code. In `github.com/ebitengine/oto/v3` v3.4.0, `Player.Close()` is documented as "does nothing and always returns nil" — teardown moved to a runtime GC finalizer. So `Close()` does not stop oto's read-pull, calling it trips SA1019, and the deprecated API must be handled in the shipped per-block transition and cleanup paths (n/b/g/replay), not just the spike.

## Options considered

### Option A: Pause() then drop the reference (let the finalizer reclaim)
- **Pros**: `Pause()` removes the player from oto's active mux set so it stops pulling its source — a deterministic, v3.4-correct halt; dropping the reference lets the runtime finalizer do the real teardown; no SA1019; no `//nolint`.
- **Cons**: Reclaim is not synchronous/deterministic — it is GC-timed.

### Option B: call Player.Close() (optionally //nolint:staticcheck)
- **Pros**: Reads like conventional resource cleanup.
- **Cons**: It is a no-op in v3.4 (returns nil, does not stop the read-pull), so it gives a false sense of teardown; trips SA1019; suppressing with `//nolint` would institutionalize a misleading call. Rejected.

## Decision

On every `n`/`b`/`g`/`replay` transition and on cleanup, the prior player is `Pause()`'d — which removes it from oto's active mux set so it stops pulling its source — and then its Go reference is dropped. Real teardown is the runtime GC finalizer.

`Player.Close()` is **NOT** called and is **NOT** `//nolint:staticcheck`-suppressed: in oto v3.4 it is a no-op (returns nil, does not stop the read-pull) and trips SA1019, so it must be **avoided outright, not suppressed**. The `listenPlayer` seam interface deliberately has **no `Close()` method** — the seam encodes the teardown contract so the no-op can't be called by mistake.

The PCM source is an in-memory `*bytes.Reader` (no fd), so the finalizer reclaims only memory — there is no file descriptor lifetime hazard on this path.

**Honest invariant — bounded retention:** the prior player is `Pause()`'d and dereferenced on transition, becoming GC-reclaimable via oto's finalizer. This is **NOT** a deterministic synchronous free and **NOT** a process-lifetime leak — retention is bounded to the next GC after the reference is dropped.

## Consequences

- The `listenPlayer` seam has no `Close()`, so callers physically cannot invoke the deprecated no-op.
- Teardown is GC-timed, not synchronous — acceptable because the source is an in-memory `*bytes.Reader` (memory only, no fd).
- golangci-lint stays clean (no SA1019, no `//nolint` suppression to audit later).

## Related decisions

- [oto v3.4 Player.Close() is a no-op (finalizer teardown)](2026-06-27-oto-v3-4-player-close-no-op-finalizer-teardown.md) — the spike finding this productionizes; this decision builds on and applies it to shipped code, with the source now an in-memory `*bytes.Reader` (no fd) so the spike's fd-lifecycle debt does not apply here.
- [Listen-path true Pause/Resume via ebitengine/oto v3 in-process PCM player](2026-06-27-true-pause-via-oto-v3-no-cgo-in-process-player.md) — the engine being productionized.
- [afplay listen fallback dropped — oto is the sole listen engine](2026-06-27-afplay-listen-fallback-dropped-oto-is-the-sole.md) — sibling #101 productionization decision on the same single path.

## Revisit trigger

If the PCM source ever becomes fd-backed (e.g., streaming from a file instead of an in-memory `*bytes.Reader`), revisit teardown — the finalizer would then own an fd and the spike's deferred Option C (finalizer-aware fd ownership) becomes load-bearing.
