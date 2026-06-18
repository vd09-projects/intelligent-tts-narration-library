# --sink=persistent deferred to phase 2 with fast-error

- **Date:** 2026-06-18
- **Status:** accepted
- **Category:** tradeoff
- **Tags:** [cmd/narrate, sink, persistent, honesty-rule, phase-one, issue-7]
- **Source:** harvested from cmd-narrate-issue-7 build summary v1, decision mark v3

## Context

Issue #7 ships the vertical slice with the **ephemeral** sink (afplay-driven local playback). The **persistent** sink (writes `audio.wav` + `plan.json` + `manifest.json` to disk) is on the roadmap for phase 2 but not part of this slice. The CLI surface still needs to acknowledge the persistent sink's existence — the `--sink` flag was always going to be a two-value enum — so the question was how to handle the unimplemented half.

## Decision

The vertical slice **rejects `--sink=persistent` fast** with the stable error message:

> persistent sink not implemented in vertical slice (issue #7)

Routed via the `errPersistentNotImplemented` sentinel (`var errPersistentNotImplemented = errors.New(persistentNotImplementedMsg)`), which `cmd/narrate/main.go`'s `exitCodeFor` classifies via `errors.Is` to exit code 2 (matching `--sink` flag validation semantics).

## Rejected alternative

**Silent fallback to ephemeral when `--sink=persistent` is requested.** The user would get audio playback but no persistent artifacts and no warning. This violates the honesty rule (CLAUDE.md): refusal is data, never hidden behavior. The same principle that drives the bare-image refusal during narration applies here at the CLI surface.

> Honest contract beats silent fallback.

## Consequences

- A phase-2 ticket lifts the rejection by wiring `sink/persistent` and swapping the error branch for a real construction call. The flag enum and CLI shape do not change.
- The sentinel `errPersistentNotImplemented` is the migration target — when phase 2 lands, deleting the sentinel from the codebase finds every site that referenced it.
- Tests assert the rejection both via `errors.Is(err, errPersistentNotImplemented)` (the recommended pattern) and via the exit-code routing path.

## Related decisions

- Pipeline composition root pattern (`architecture/2026-06-18-pipeline-composition-root-pattern.md`) — the pipeline is sink-agnostic; the CLI layer is where the rejection lives.
- CLI flag taxonomy (`convention/2026-06-18-cli-flag-taxonomy-named-only.md`) — defines the `--sink` flag this decision constrains.
