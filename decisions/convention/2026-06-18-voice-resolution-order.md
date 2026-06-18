# Voice resolution order: opts > plan defaults > backend default

- id: 2026-06-18-voice-resolution-order
- date: 2026-06-18
- status: accepted
- category: convention
- tags: [render, voice, kokoro, phase-one]

## Decision

The `render/sherpa` backend resolves the engine voice id for each render call in this order:

1. **`RenderOptions.Voice`** — an engine voice id (e.g. `"af_bella"`, `"am_michael"`). Used as-is when non-empty AND in the backend's supported set. Non-empty + unknown → `sherpa.ErrUnsupportedVoice` (caller bug, errors the call).
2. **`plan.NarrationPlan.Defaults.Voice`** — the engine-neutral hint stamped by the planner. Used when it happens to match a known engine id. Unknown hint → **silent fallback** to (3), never an error.
3. **Backend default** — `"af_bella"` for `render/sherpa`. Phase-one female default per CLAUDE.md.

## Why

`RenderOptions.Voice` is an engine-specific override; if the caller passes a value, they intend it and an unknown value is a programming error worth surfacing.

`PlanDefaults.Voice` is — by CLAUDE.md — *engine-neutral*. The planner can stamp `"af_bella"` if it happens to know, but it could equally stamp `"default-female"` or leave it blank. The renderer must not error on an opaque hint; that would put a coupling between planner and renderer that the engine-neutral contract explicitly forbids.

## Rejected alternatives

- **Plan defaults always wins** — would let a stale plan force a now-unsupported voice forever.
- **Unknown plan-hint errors** — couples planner to renderer (planner would have to know engine voice ids).
- **Caller must explicitly map gender→voice id** — duplicates the work the MCP `speak` tool's `gender` arg already does. Backend default `"af_bella"` is the gender-female phase-one answer.
