# `sink/` imports `render/` for `RenderResult` and `AudioStream`

- id: 2026-06-19-sink-imports-render-for-renderresult
- date: 2026-06-19
- status: accepted
- category: architecture
- tags: [sink, render, layering, dependency-direction, phase-one]

## Decision

`sink/` (and its subpackages — `sink/ephemeral`, future `sink/persistent`) imports `render/` for the `RenderResult` and `AudioStream` types. The dependency direction is:

```
plan/    ← imports nothing
render/  ← imports plan/
sink/    ← imports plan/ + render/
```

This stays a DAG. The composition root (`pipeline/`, `cmd/`) imports all three.

## Why

Sinks consume what renderers produce. The type that names "audio bytes + format + per-block timings" is *renderer output* — it is born in `render/`. Re-declaring it in `sink/` would fork the contract and force every new sink backend to translate. Importing the producer's type at the consumer is the natural, low-coupling direction.

`plan/`'s zero-dependency invariant is preserved. `render/` already imports `plan/`. `sink/` now joins as a downstream consumer. The CLAUDE.md invariant "`adapter/`, `render/`, `sink/`, `intelligence/` import `plan/` plus their own interface package" stays intact — `render/` is `sink/`'s upstream peer, not a foreign concern.

## Rejected alternatives

- **Hoist `RenderResult` / `AudioStream` into `plan/`.** Rejected because audio bytes and WAV format are not part of the engine-neutral plan contract — they are renderer output. Putting them in `plan/` would mean every plan consumer (planner, `adapter/mcptext`, future input adapters) drags in audio types it never touches. Violates the CLAUDE.md invariant "`plan/` imports nothing from this project. Everything imports `plan/`" by inflating `plan/`'s surface.
- **Define a sink-local `AudioInput` interface that the composition root adapts `RenderResult` into.** Rejected for phase-one YAGNI — one renderer, one ephemeral sink, one persistent sink (planned). The adapter layer would be ceremony with no second implementation to justify it. Revisit if a second renderer-output shape appears (e.g. streaming chunks).

## Related decisions

- [Per-block WAVs stay separate; renderer does not concatenate](../architecture/2026-06-18-per-block-wavs-no-concat-in-renderer.md) — establishes that `RenderResult` carries per-block `AudioStream`s, which is what `sink/` consumes.
- [`AudioStream` is an on-disk handle](../architecture/2026-06-18-audiostream-on-disk-handle.md) — defines the type `sink/` now depends on.
