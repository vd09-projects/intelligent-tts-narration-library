# Code blocks default to L2 in listen-mode via an additive `CodeMinLevel` floor on `planner.Request`

| Field    | Value            |
|----------|------------------|
| Date     | 2026-06-23       |
| Status   | accepted         |
| Category | architecture     |
| Tags     | code-min-level, listen-mode, planner-request, pipeline-defaults, per-block-leveling, floor-field, api-contract, schema-neutral, issue-73, classcode |

## Context

Issue #73 Part A. In listen-mode (the terminal `speak` path), code blocks should default to L2 (semantic summary) rather than the document-wide effective level, so a code-heavy document is heard as meaning rather than read out character-for-character. This resolves the open pin carried by the #72 v3 ADR guardrail decision (`primary-listen-path-decoupled-from-durable-sink`): *"where the L2 listen-mode default lives (speakArgs/PipelineDefaults vs caller-passed level)."*

Constraints in play:
- The on-wire `plan.json` schema must stay engine-neutral and additive-compatible — no new persisted field per `CLAUDE.md` schema-versioning rule.
- The planner does no I/O and imports only `plan/` + the `IntelligenceAdapter` interface; per-class leveling must be a planner-read field, not a composition-root concern.
- Block IDs and classes are assigned **inside** `Plan` — so any per-class floor must be readable at plan time, not resolved against block-ID-keyed `Overrides` at the composition root.
- An explicit caller request for L3 on a code block (e.g. someone who genuinely wants the detail) must survive the default.
- Non-listen callers must see zero behavioral drift.

## Options considered

### Option A: Additive `CodeMinLevel plan.Level` floor field on `planner.Request`
- **Pros**: Read inside the existing structural pass as `target = max(effectiveLevel, CodeMinLevel)` for `ClassCode` only. Zero value is a no-op (preserves zero drift for non-listen callers). A floor, so explicit L3 survives. Lives on `planner.Request`, which the ephemeral primary path already constructs — no persisted round-trip, on-wire `plan.json` untouched. Surfaced via `pipeline.PipelineDefaults` set by `cmd/narrate-mcp` `newPipeline`. Per-class read happens where classes are actually assigned (inside `Plan`).
- **Cons**: One more field on `planner.Request`; a single hard-coded class (code) rather than a general mechanism.

### Option B: General `map[plan.Class]plan.Level` per-class override map
- **Pros**: Covers any future class (diagram, table) needing a per-class floor.
- **Cons**: Speculative generality — #73 needs exactly one class. More surface to test and reason about for no current caller.

### Option C: Put the default on `plan.PlanDefaults` / composition-root `Overrides`
- **Cons**: `PlanDefaults` rides the on-wire `plan.json` — would touch the engine-neutral persisted schema for an ephemeral-path-only concern. `Overrides` is block-ID-keyed and resolved at the composition root, but block IDs/classes don't exist until *inside* `Plan` — wrong layer for a per-class rule.

## Decision

**Option A.** Express code-blocks-default-to-L2 as an additive declarative **floor** field `CodeMinLevel plan.Level` on `planner.Request`, read inside the existing structural pass as `target = max(effectiveLevel, CodeMinLevel)` for `ClassCode` only, and surfaced via `pipeline.PipelineDefaults` set by `cmd/narrate-mcp` `newPipeline`.

Why a **floor**, not a hard-set: an explicit L3 listen request must survive (`max` keeps the higher of the two), and the zero value is a no-op that preserves zero drift for every non-listen caller.

Why on **`planner.Request`**, not `plan.PlanDefaults`: the ephemeral primary path needs no persisted round-trip; keeping the field off the plan keeps the on-wire `plan.json` schema untouched and engine-neutral. And because block IDs/classes are assigned **inside** `Plan`, a per-class floor must be a planner-read field — not composition-root `Overrides` resolution, which is block-ID-keyed and runs at the wrong layer.

Rejected the general `map[plan.Class]plan.Level` per-class override map (Option B) as speculative generality: a single `CodeMinLevel` covers all of #73.

## Consequences

- `planner.Request` gains one optional field; default-zero behavior is unchanged for existing callers, so non-listen paths see no drift.
- The terminal listen path now hears code at L2 by default while an explicit L3 request still wins.
- If a future ticket needs a per-class floor for `diagram` / `table`, the single-field approach will need revisiting — at which point the rejected `map[plan.Class]plan.Level` is the natural generalization.
- Standing honesty boundary is reaffirmed (reuse of existing #48 behavior, not a new decision): code-L2-with-no-adapter → `Status = degraded` (deterministic count + decls); adapter-refuses → `Status = refused`.

## Related decisions

- [Keep the primary listen path decoupled from any durable sink](../tradeoff/2026-06-23-primary-listen-path-decoupled-from-durable-sink.md) — resolves that decision's open pin on where the L2 listen-mode default lives (answer: `CodeMinLevel` on `planner.Request`, surfaced via `PipelineDefaults`).
- [Code semantic gist is L2-only](../tradeoff/2026-06-22-code-semantic-gist-l2-only.md) — this floor defaults code to that L2 gist behavior in listen-mode.

## Revisit trigger

Revisit if a future ticket needs per-class floors for additional classes (diagram / table) — at that point promote `CodeMinLevel` to a `map[plan.Class]plan.Level` per-class floor map (the rejected Option B).
