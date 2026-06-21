# Fix planner test-seam data race by threading clock/plan-id seams per-call (Option B)

| Field    | Value            |
|----------|------------------|
| Date     | 2026-06-21       |
| Status   | accepted         |
| Category | architecture     |
| Tags     | planner, data-race, t.Parallel, test-seam, concurrency, voiceoptions, purity |

## Context

`make test-race` flagged data races in the `planner` package. Root cause: two unsynchronized package-level test seams (`nowFunc`, `newPlanIDFunc`) were mutated by parallel `t.Parallel()` tests while a sibling `Plan()` call read them concurrently. The races were a test-infrastructure footgun — package globals reached into by tests to inject a fake clock and deterministic plan IDs — but they violated the planner's purity posture and made the parallel test suite non-deterministic under `-race`.

Constraint: the planner must stay pure (no I/O, depends only on `plan/` and the `IntelligenceAdapter` interface), and the test suite should keep running tests in parallel.

## Options considered

### Option A: Drop `t.Parallel()` from the ~8 culprit tests
- **Pros**: Sanctioned stopgap; smallest diff; immediately silences `-race`.
- **Cons**: Leaves the mutable package-global footgun in place — the next test or caller that touches `nowFunc`/`newPlanIDFunc` reintroduces the race. Loses parallel-test speed on those tests. Treats the symptom, not the cause.

### Option B: Eliminate the globals; thread `withClock`/`withPlanID` per-call (CHOSEN)
- **Pros**: Root-cause fix — there is no shared mutable state left to race on. Adds unexported `withClock`/`withPlanID` `VoiceOption`s resolved to locals inside `Plan()`, defaulting to wall-clock and `plan.NewPlanID`. Tests stay parallel. Planner stays pure (seams are per-call inputs, not ambient globals).
- **Cons**: Slightly larger surface — option plumbing through `Plan()`; requires surfacing resolved options past the parse step (see related Decision 2).

### Option C: Guard the globals with `sync.RWMutex`
- **Pros**: Silences `-race`.
- **Cons**: Serializes access but does not stop logical cross-talk between parallel tests (one test's injected clock is still globally visible to another). Adds lock machinery to a pure hot path. Worst structural fit — keeps the global, adds cost, and doesn't actually isolate tests.

## Decision

Chose **Option B**: delete the `nowFunc` / `newPlanIDFunc` package globals and thread the clock and plan-id seams per-call as unexported `VoiceOption`s (`withClock`, `withPlanID`), resolved to locals inside `Plan()` with wall-clock and `plan.NewPlanID` as defaults.

This is the root-cause fix: with no shared mutable package state, there is nothing left to race on, the test suite keeps `t.Parallel()`, and the planner's purity invariant is preserved (the seams become explicit per-call inputs rather than ambient globals). Both the plan review (7-reviewer panel) and the build review APPROVE'd Option B. Option A was rejected because the global footgun remains; Option C was rejected as the worst structural fit (serializes without isolating, and adds lock machinery to a pure path).

## Consequences

- No package-level mutable test seams remain in `planner/` — future tests inject via per-call options.
- `Plan()` now resolves clock and plan-id from parsed options; this required surfacing resolved options past the parse step (previously only the compiled lexicon escaped the parse). See related Decision 2 for the API-shape tradeoff made to keep that change minimal-blast-radius.

## Related decisions

- [Preserve the variadic compileLexicon(opts...) signature when surfacing resolved voiceOptions to Plan()](../tradeoff/2026-06-21-preserve-variadic-compilelexicon-signature.md) — the implementation tradeoff made to surface the resolved options without breaking unrelated call sites.

## Experiments

`make test-race` was the detector. Post-fix, the planner package runs `t.Parallel()` tests clean under `-race`.

<!-- Note (source-accuracy fact, not a decision): plan.NewPlanID() returns a plain string — there is NO named plan.PlanID type. Use string for the resolved plan-id local. -->
