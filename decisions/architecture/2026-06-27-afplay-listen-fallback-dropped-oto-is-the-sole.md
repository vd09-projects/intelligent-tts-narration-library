# afplay listen fallback dropped — oto is the sole listen engine

| Field    | Value            |
|----------|------------------|
| Date     | 2026-06-27       |
| Status   | accepted         |
| Category | architecture     |
| Tags     | listen-path, oto, afplay, single-engine, build-tags, engine-flag, honesty-rule, device-open-error, no-fallback, zero-cgo, purego, issue-101, issue-100 |

## Context

Issue #101 productionizes the oto v3 listen-path that the #100 spike validated by ear. Up to this point `cmd/narrate` carried a `//go:build oto` / `//go:build !oto` tag pair: the in-process oto v3 player behind the `oto` tag, and an afplay-based listen fallback (plus an `--engine` escape hatch) behind `!oto`. The spike (#100) and the research (#92) established that oto v3.4 via `purego` is zero-CGo, so it builds under `CGO_ENABLED=0` with no packaging regression — removing the original reason the afplay fallback existed (avoiding a CGo dependency for the default build).

Maintaining two listen engines plus a build-tag matrix is pure cost, especially once the prior decision `2026-06-26-afplay-sigstop-sigcont-no-true-pause` established that afplay+SIGSTOP structurally cannot deliver true device-level Pause/Resume — i.e., the second engine is strictly inferior on the listen path's defining feature.

## Options considered

### Option A: oto is the sole listen engine; collapse the build-tag pair
- **Pros**: One code path, no build-tag matrix, no inferior second engine; oto is zero-CGo so the default build is unchanged in packaging terms; true Pause/Resume always available.
- **Cons**: No in-process fallback if a device cannot be opened — listen mode hard-fails (as an error) on that host.

### Option B: keep afplay fallback + `--engine` escape hatch
- **Pros**: A degraded listen still works where no audio device can be opened by oto.
- **Cons**: Two engines to maintain, build-tag matrix, and the fallback cannot do true Pause/Resume — it silently degrades the headline feature; ongoing cost for a path the project does not want.

## Decision

Collapse the `//go:build oto` / `!oto` tag pair so the in-process oto v3 player is the **default and only** listen engine in `cmd/narrate`. The afplay listen fallback and the `--engine` escape hatch are removed.

There is no second listen engine. If oto cannot open an audio device, listen mode returns a **wrapped, actionable error UP the pipeline**. Per the CLAUDE.md honesty rule, an engine/device-open failure is an *error* (it stops the pipeline), **never** a `Refusal` — refusals are for readable-but-unvoiceable content, not for an output device that won't open.

The non-interactive `speak`/MCP path still uses `sink/ephemeral` afplay independently for play-then-delete playback and is **untouched** by this change — afplay is dropped only as a *listen* engine, not as the ephemeral-sink player.

Rationale: afplay+SIGSTOP structurally cannot give true device-level Pause/Resume (see `2026-06-26-afplay-sigstop-sigcont-no-true-pause`), so the fallback was a strictly inferior engine; the build-tag matrix was maintenance cost with no upside. oto v3.4 via `purego` is zero-CGo (`CGO_ENABLED=0` builds), so it can be the default with no packaging regression.

## Consequences

- Single listen code path; no build-tag matrix in `cmd/narrate`; no `--engine` flag surface.
- On a host where oto cannot open a device, listen mode fails with a wrapped, actionable error rather than silently degrading — this is intentional and correct per the error/refusal boundary.
- The ephemeral-sink afplay path is independent and continues to serve `speak`/MCP playback.

## Related decisions

- [Listen-path true Pause/Resume via ebitengine/oto v3 in-process PCM player](2026-06-27-true-pause-via-oto-v3-no-cgo-in-process-player.md) — the engine this decision makes the sole listen engine.
- [afplay SIGSTOP/SIGCONT cannot deliver a true Pause](2026-06-26-afplay-sigstop-sigcont-no-true-pause.md) — why the dropped fallback was strictly inferior on the listen path.
- [oto v3.4 player teardown is Pause()+drop-reference, not Close()](2026-06-27-oto-v3-4-player-teardown-is-pause-drop-reference.md) — sibling #101 productionization decision on the same single path.

## Revisit trigger

If a target host genuinely cannot open an audio device via oto and a degraded (no-true-pause) listen is judged better than an error there — only then reconsider a fallback engine.
