# --intelligence anthropic with missing env is a flag-validation error

- **Date:** 2026-06-20
- **Status:** experimental
- **Category:** convention
- **Tags:** [cmd, narrate, intelligence, anthropic, flag-validation, exit-codes, issue-15]
- **Owner:** vd
- **Scope:** issue-15

## Context

cmd/narrate's new `--intelligence anthropic` flag requires `ANTHROPIC_API_KEY` in the environment. When the user opts in but the env var is empty, three options exist: silent fallback to "none", runtime error during the API call, or flag-validation error at startup.

## Options considered

### Option A: Flag-validation error, exit 2 (CHOSEN)
- **Pros**: Loud and immediate. Matches the existing `--block × --sink=persistent` flag-validation precedent. Caller-correctable: fix the env or the flag, re-run. Same exit code as `--level 99` or `--sink garbage`.
- **Cons**: The error message names the env var, so reading it requires terminal scrollback.

### Option B: Silent fallback to `--intelligence none`
- **Pros**: Pipeline still runs.
- **Cons**: Hides the misconfiguration. Caller explicitly opted in to intelligence; getting the degraded path silently is worse than an explicit error. Output looks normal until the caller wonders why prose is being refused.

### Option C: Runtime error during first API call
- **Pros**: Defers the check to the actual call site.
- **Cons**: Caller has already paid for planning + setup. The error surfaces deep in the pipeline as "anthropic: 401 invalid api key" rather than "your env var is empty." Same exit code as a pipeline failure (1) — mixes flag misconfiguration with system failure.

## Decision

`flagSet.validate()` checks `os.Getenv("ANTHROPIC_API_KEY")` when `--intelligence == "anthropic"`. Empty → error with message naming both the flag and the env var. Wrapped in `errFlagValidation` so `exitCodeFor` routes to exit 2.

Validation runs inside `runNarrate` (after cobra parsing), not as `PreRunE`, so `--help` still works.

## Consequences

- Exit code 2 covers both "unknown --intelligence value" and "anthropic + empty env."
- The validate test (`TestFlagSet_Validate_AnthropicWithoutEnvVar`) uses `t.Setenv` to drive the check sequentially.
- Construction in `chooseIntelligence` cannot fail meaningfully — validate() guarantees the env is set; a non-nil error from `anthropic.New` is a programmer bug, surfaced via panic with a clear message.

## Related decisions

- [--block X with --sink=persistent rejected at flag-validation](2026-06-20-block-with-persistent-sink-rejected-at-flag-time.md) — sibling precedent. Same exit-2-via-errFlagValidation routing.

## Revisit trigger

When `--intelligence` adds a third value that also has external dependencies (e.g. `--intelligence openai` would need `OPENAI_API_KEY`). The validate switch grows; consider extracting a per-adapter env-key registry.
