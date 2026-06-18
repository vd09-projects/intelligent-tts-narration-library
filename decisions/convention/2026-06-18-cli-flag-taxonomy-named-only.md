# cmd/narrate CLI flag taxonomy: named flags only

- **Date:** 2026-06-18
- **Status:** accepted
- **Category:** convention
- **Tags:** [cmd/narrate, cli, cobra, flags, phase-one, issue-7]
- **Source:** harvested from cmd-narrate-issue-7 build summary v1, decision mark v2

## Context

`cmd/narrate` is the first user-facing CLI in the project. Phase one wants the flag surface to be intentional: the slice is small but the binary will likely grow as MCP / persistent sink / intelligence adapter land. A flag taxonomy decided here will set the precedent for `cmd/narrate-mcp` and any future `cmd/*` binary.

## Decision

`cmd/narrate` uses **named flags only** — no positional arguments. The flag set is:

| Flag | Type | Default | Choices |
|---|---|---|---|
| `--file` | string (required) | — | filesystem path |
| `--level` | int | `1` | `1`, `2`, `3` |
| `--sink` | string | `ephemeral` | `ephemeral`, `persistent` (persistent rejected in vertical slice) |
| `--gender` | string | `female` | `female`, `male` |

Female default per the problem statement (`af_bella` is the phase-one default voice).

## Rejected alternatives

1. **Positional `narrate FILE` arg.** Shorter at the call site but less self-documenting in `--help` output and harder to extend (any future required input would compete for positional slots).
2. **`--voice` taking the engine voice id directly.** Would couple the user-layer to the renderer's voice ids (`af_bella`, `am_michael`). `--gender` stays engine-neutral at the user surface; the mapping to engine voice ids lives in `cmd/narrate/main.go`'s `genderToVoice` map.

## Consequences

- Future flags (`--out-dir`, `--voice`, `--locale`, etc.) follow the same named-only pattern.
- `cmd/narrate-mcp` will likely mirror this set as MCP request fields — keeping the CLI and MCP shapes parallel.
- Validation lives in `flagSet.validate()`. Validation failures wrap `errFlagValidation` so the exit-code router can distinguish them from pipeline errors (exit 2 vs exit 1).

## Related decisions

- Persistent-sink-deferred-fast-error (`tradeoff/2026-06-18-persistent-sink-deferred-fast-error.md`) — the `--sink=persistent` choice that drives this flag's runtime behavior.
- Voice resolution order (`convention/2026-06-18-voice-resolution-order.md`) — what `--gender` ultimately maps to in the renderer.
