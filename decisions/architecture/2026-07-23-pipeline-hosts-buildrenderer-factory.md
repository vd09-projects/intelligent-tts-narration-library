# pipeline.BuildRenderer is the shared renderer-factory home; pipeline/ now imports the concrete engines

| Field    | Value            |
|----------|------------------|
| Date     | 2026-07-23       |
| Status   | accepted         |
| Category | architecture     |
| Tags     | rvc, voice-conversion, buildrenderer, renderer-factory, composition-root, pipeline, render-sherpa, render-rvc, concrete-engines, import-cycle, engine-neutral, planner-deps, plan-deps, issue-146, issue-145 |

## Context

Issue #146 wires a single user-facing voice knob (`--voice` on the CLI, the `voice` arg on the MCP tools, a launch flag on narrate-server) that selects either plain Kokoro (24 kHz) or the `render/rvc` decorator wrapping Kokoro (40 kHz). All three `package main` composition roots — `cmd/narrate`, `cmd/narrate-mcp`, `cmd/narrate-server` — need the *same* logic to turn that knob into a concrete `render.Renderer` plus the matching `plan.AudioFormat` (so the format-validating persistent sink and the renderer stay coupled by construction — Decision D1).

The question was **where that shared `BuildRenderer(voice) (render.Renderer, plan.AudioFormat, error)` factory should live** — a factory that, unlike the previously interface-only `pipeline/`, must import the concrete `render/sherpa` and `render/rvc` engines. CLAUDE.md's invariant names the composition root as "(pipeline/, cmd/)" and blesses only those two to know concrete edges; `planner/` and `plan/` must stay engine-neutral and I/O-free.

## Options considered

### Option A: host BuildRenderer in the `render/` root package
- **Pros**: reads as "the renderer package builds renderers".
- **Cons**: creates an import cycle — `render` would import `render/sherpa` and `render/rvc`, which already import `render` for the `Renderer` interface and shared types (`render → render/sherpa → render`). Rejected.

### Option B: a new `render/renderbuild` or `internal/renderbuild` package
- **Pros**: keeps `pipeline/` interface-only.
- **Cons**: introduces a THIRD concrete-engine-aware location beyond the two CLAUDE.md sanctions ("only pipeline/ and cmd/ know concrete edges"), a strained reading of the invariant for no real benefit — the factory would still be imported only by the roots that pipeline/ already serves. Rejected.

### Option C: host BuildRenderer in `pipeline/` — CHOSEN
- **Pros**: `cmd/` already depends on `pipeline/`, so no new dependency direction is created; `pipeline/` importing concrete `render/sherpa` + `render/rvc` creates NO cycle because neither engine imports `pipeline/`; keeps concrete-engine knowledge inside the two locations CLAUDE.md already blesses (pipeline/ + cmd/); one factory, three thin callers.
- **Cons**: `pipeline/` moves from interface-only to concrete-engine-importing. Accepted — it is already the composition root that holds concrete edge instances at wiring time; importing the engines it composes is consistent, not a boundary break.

## Decision

**The shared `BuildRenderer(rvcVoice string) (render.Renderer, plan.AudioFormat, error)` factory lives in `pipeline/`** (`pipeline/build_renderer.go`). As a consequence **`pipeline/` now imports the concrete `render/sherpa` and `render/rvc` engines**, where it was previously interface-only.

`rvcVoice == ""` returns the plain Kokoro engine plus `render.DefaultFormat()` (24 kHz), byte-identical to prior behavior. `rvcVoice != ""` returns the `render/rvc` decorator wrapping Kokoro plus `rvc.OutputFormat()` (40 kHz); the RVC target slug is passed straight into `rvc.Config.Voice` — this factory translates nothing (the decorator owns the target→Kokoro-source map, per the sibling #145 decision). Returning the `AudioFormat` alongside the renderer is the load-bearing move for D1: each root hands the same format object to both the renderer and the sink's `WithExpectedFormat`.

## Consequences

- All three composition roots call one factory; adding a fourth root or a new engine is a single-file change in `pipeline/`.
- `pipeline/` is now concrete-engine-aware. This is within the sanctioned composition-root boundary — CLAUDE.md permits pipeline/ + cmd/ to know concrete edges.
- An unknown `rvcVoice` returns `rvc.ErrUnsupportedVoice` with a nil renderer — never a silent fallback to Kokoro (honesty rule). Roots validate membership eagerly (`rvc.IsSupportedVoice`), so in production this error is an unreachable construction-time backstop.
- A future engine must not host its own parallel factory in `render/` (import cycle) or a third `renderbuild` package (a new concrete-edge location) — it belongs here.

## Safety / verification

- The `planner/` and `plan/` engine-neutrality + I/O-free invariant is unaffected: `pipeline/` sits above them and neither imports it. This is machine-guarded by `planner/deps_test.go` and `plan/deps_test.go`, which assert those packages' own `Imports` gain no new engine or I/O dependency (a `go list` over the package's direct imports, test files excluded).
- The multi-perspective build review APPROVED this boundary choice.

## Related decisions

- [Pipeline composition root pattern](2026-06-18-pipeline-composition-root-pattern.md) — establishes `pipeline.Pipeline` as the only struct holding concrete edge instances; this decision extends that root to also host the shared renderer factory (and therefore import the concrete engines).
- [RVC decorator owns the target->{Kokoro source, index_rate, pitch} map; translation happens exactly once](2026-07-22-rvc-decorator-owns-voice-map-single-translation.md) — why `BuildRenderer` passes the RVC target slug straight through and translates nothing.
- [manifest.voice records the RVC character slug for RVC renders (Option A), not the hidden Kokoro source](../tradeoff/2026-07-23-rvc-manifest-voice-records-character-slug.md) — Decision D6, the sibling #146 provenance decision this one complements; both derive their format/voice from the single `BuildRenderer` origin.

## Revisit trigger

Reconsider if a second consumer outside the composition roots (pipeline/ + cmd/) legitimately needs to build a renderer from a voice knob — at that point extract the factory to a shared package rather than importing `pipeline/` from a non-root, or if `render/` is ever restructured so that a root `render` package no longer imports its sub-engines (removing the cycle that rules out Option A).
