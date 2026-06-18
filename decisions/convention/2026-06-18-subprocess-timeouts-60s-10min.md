# Phase-one subprocess timeouts: 60 s per-block, 10 min wall

- id: 2026-06-18-subprocess-timeouts-60s-10min
- date: 2026-06-18
- status: accepted
- category: convention
- tags: [render, subprocess, timeout, phase-one]

## Decision

`render.RenderOptions` exposes two timeouts:

- `PerBlockTimeout` — default 60 s. Caps a single `scripts/kokoro` subprocess invocation.
- `WallClockTimeout` — default 10 min. Caps the entire `Render` call (the orchestration loop across all blocks). Ignored by `RenderBlock` (single-block re-render).

Both are `time.Duration`. Zero means "use the default." Exceeded → returned as `sherpa.ErrTimeout` wrapping `context.DeadlineExceeded`. The wrapper subprocess is killed via the standard `exec.CommandContext` SIGKILL path.

## Why

Phase-one Kokoro on M-series silicon synthesizes a short sentence in 1–4 s and a long paragraph in 8–15 s. 60 s per block leaves comfortable headroom for slow first-load (model warm-up) and longer prose. 10 min wall handles the worst case of a 30-block document where each block takes ~20 s.

Returning timeouts as errors (not as refusals) honors the CLAUDE.md error/refusal boundary: the honesty rule applies only to *readable but unvoiceable source*, never to backend failure. A timeout means the backend is sick, not that the source is unvoiceable.

## Rejected alternatives

- **No timeout** — would let a hung subprocess stall the whole pipeline indefinitely. Rejected.
- **Single overall timeout, no per-block** — would let one bad block consume the whole budget. Per-block keeps failure scope tight.
- **Treat timeout as refusal** — would silently fabricate a "refused due to timeout" story for content that was probably perfectly voiceable. Violates the honesty rule.
