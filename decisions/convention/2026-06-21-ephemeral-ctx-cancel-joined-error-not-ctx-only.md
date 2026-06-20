# Ephemeral sink ctx-cancel surfaces a joined error (ctx cause + process exit), not ctx-only

| Field    | Value            |
|----------|------------------|
| Date     | 2026-06-21       |
| Status   | accepted         |
| Category | convention       |
| Tags     | sink/ephemeral, ctx-cancel, errors.Join, error-handling, issue-11 |

## Context

`playWithAfplay` (sink/ephemeral) runs `afplay <wav>` under a per-block deadline and a parent context. On cancellation/timeout the cancel branch (`case <-callCtx.Done():`) drained the wait channel but returned only `callCtx.Err()`, discarding whatever `cmd.Wait()` produced for the killed child. Issue #11 AC#5 asked to join the two instead of dropping one. The change had to keep the existing `TestSink_Consume_TableDriven` ctx-cancel case green (it asserts `errors.Is(err, context.Canceled)`).

## Options considered

### Option A: errors.Join(callCtx.Err(), waitErr)
- **Pros**: callers see both the cancellation cause and the process death; `errors.Join` preserves `errors.Is(err, context.Canceled / DeadlineExceeded)`; drops `waitErr` when nil (clean reap collapses to bare ctx error). Carries two independent causes.
- **Cons**: returned error is now a multi-error; consumers iterating with `Unwrap() []error` see >=2 entries.

### Option B: keep ctx.Err() only
- **Pros**: simplest.
- **Cons**: loses the process-death signal ("signal: killed"), which is the diagnostic value AC#5 wanted.

### Option C: fmt.Errorf("%w", ...) chain
- **Pros**: single error chain.
- **Cons**: a single `%w` chain cannot carry two independent leaf causes; would have to pick one to be `Is`-matchable.

## Decision

Return `errors.Join(callCtx.Err(), waitErr)` from the cancel branch, where `waitErr` is the drained `cmd.Wait()` result (captured in both the already-reaped and the killGrace-then-Kill paths). Fixed at the `playWithAfplay` layer, NOT `Consume` — `Consume`'s ctx-precedence on the between-blocks short-circuit is correct and unchanged. Consistent with the project rule that timeouts/cancellation are errors, not refusals.

## Consequences

- The cancel-path error is a multi-error; downstream code matching with `errors.Is` is unaffected, but code that type-asserts a single concrete error type must use `errors.As`/`Unwrap() []error`.
- Locked by `TestPlayWithAfplay_CtxCancel_Joins`, which asserts the joined error carries >=2 errors via `Unwrap() []error` and still satisfies `errors.Is(context.Canceled)`.

## Related decisions

- [Timeouts are errors, not refusals (render/sherpa)](../convention/) — same error-vs-refusal boundary applied at the renderer layer.

## Revisit trigger

If a second sink or a wire layer needs to serialize this error, revisit whether a structured cancellation error type is warranted instead of a bare join.
