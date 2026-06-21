# Preserve the variadic compileLexicon(opts...) signature when surfacing resolved voiceOptions to Plan()

| Field    | Value            |
|----------|------------------|
| Date     | 2026-06-21       |
| Status   | accepted         |
| Category | tradeoff         |
| Tags     | planner, voiceoptions, compilelexicon, api-shape, blast-radius, test-call-sites |

## Context

Decision 1 (Option B for the planner test-seam race fix) required surfacing the resolved clock and plan-id from parsed options into `Plan()`. Previously only the compiled lexicon escaped the option-parse step — `compileLexicon(opts...)` consumed the variadic options and returned just the lexicon, so the clock/planID seams had nowhere to ride out of the parse.

The implementer needed the parse to yield more than the lexicon, without disrupting the ~25 existing call sites that already invoke the variadic `compileLexicon(opts...)` for unrelated reasons.

## Options considered

### Option A: Change `compileLexicon`'s signature to return the full resolved options
- **Pros**: One canonical parse entry point; no delegating wrapper.
- **Cons**: Breaks ~25 unrelated test call sites that depend on the existing variadic signature — large blast radius for a change that is incidental to the race fix.

### Option B: Add fields to `voiceOptions`, introduce `resolveVoiceOptions` + `compileLexiconCfg`, keep `compileLexicon(opts...)` as a delegating wrapper (CHOSEN)
- **Pros**: `clock`/`planID` fields added to the `voiceOptions` struct; `resolveVoiceOptions` produces the fully-resolved struct and `compileLexiconCfg` compiles from it; the existing variadic `compileLexicon(opts...)` is kept and now delegates to `compileLexiconCfg`. The ~25 call sites are untouched. Minimal blast radius, faithful to Option B's spirit.
- **Cons**: Two compile entry points (`compileLexicon` wrapper + `compileLexiconCfg`) — a future reader may wonder why the variadic form survives instead of being collapsed.

## Decision

Chose **Option B**: add `clock`/`planID` fields to the `voiceOptions` struct, introduce `resolveVoiceOptions` + `compileLexiconCfg`, and **keep** the existing variadic `compileLexicon(opts...)` signature (now delegating to `compileLexiconCfg`) rather than changing it.

This is a deliberate minimal-blast-radius API-shape tradeoff: surfacing the resolved options past the parse step is what the race fix needs, but rewriting the variadic signature would have forced churn across ~25 unrelated test call sites. Keeping the wrapper isolates the change to the parse internals and stays faithful to the spirit of the chosen race fix. Recorded because the surviving variadic signature is a deliberate choice a future reader might otherwise flag as redundant.

## Consequences

- Two lexicon-compile entry points coexist intentionally: `compileLexicon(opts...)` (back-compat wrapper) and `compileLexiconCfg` (struct-driven). If the variadic call sites are ever migrated, the wrapper can be collapsed then.
- `voiceOptions` now carries `clock` and `planID`, resolved via `resolveVoiceOptions` and consumed inside `Plan()`.

## Related decisions

- [Fix planner test-seam data race by threading clock/plan-id seams per-call (Option B)](../architecture/2026-06-21-planner-test-seam-race-thread-clock-planid-per-call.md) — the parent decision this tradeoff serves.

## Revisit trigger

When the ~25 variadic `compileLexicon(opts...)` call sites are migrated to the struct-driven form (or otherwise touched en masse), collapse the wrapper into `compileLexiconCfg`.
