# `SinkReceipt.TotalDurationMs` reports planned duration, not subprocess wall time

- id: 2026-06-19-sink-receipt-planned-duration-not-wall-time
- date: 2026-06-19
- status: accepted
- category: convention
- tags: [sink, ephemeral, receipt, telemetry, phase-one]

## Decision

`SinkReceipt.TotalDurationMs` is the sum of `BlockTiming.EndMs - StartMs` across the plan being sunk — the **planned** spoken duration. It is computed from the `RenderResult.Plan.Timeline` before any audio plays. It is **not** the wall-clock span around the `afplay` subprocess.

If a future telemetry need surfaces, add a separate `ActualDurationMs` field rather than redefining this one.

## Why

Planned duration is the honest, deterministic value sinks promise to callers. Callers ask "how long is this narration?" — the answer lives in the plan, not in the playback runtime. Wall time is contaminated by:

- Subprocess startup cost (afplay fork + exec, ~30–100 ms).
- OS scheduler jitter.
- In tests, the `play` seam is stubbed (see [stubbed-play-seam](2026-06-19-ephemeral-stubbed-play-seam-build-tag.md)), so wall time is effectively zero — which would make the receipt useless under `go test ./...`.

Keeping `TotalDurationMs` derived from the plan also means receipts are stable across re-runs and across sink backends (ephemeral, persistent) — the same plan produces the same receipt duration regardless of how it is played back.

## Rejected alternatives

- **Measure span around the `afplay` subprocess.** Rejected because (a) it makes unit tests with a stub seam report a meaningless value, (b) it conflates planner output with renderer/sink runtime, and (c) it would diverge across sink backends for the same plan, breaking the engine-neutral receipt contract.
- **Two fields up front (`PlannedDurationMs` + `ActualDurationMs`).** Rejected for YAGNI in phase one — the second field has no consumer yet. Easy to add later without breaking schema (additive-compatible per CLAUDE.md).
