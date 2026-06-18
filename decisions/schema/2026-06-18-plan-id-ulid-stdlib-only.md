# PlanID — ULID generated with stdlib only

| Field    | Value            |
|----------|------------------|
| Date     | 2026-06-18       |
| Status   | accepted         |
| Category | schema           |
| Tags     | plan, ulid, zero-deps, schema, honesty-rule |

## Context

`plan/` is the load-bearing JSON contract every part of the system produces or
consumes. A hard invariant (CLAUDE.md + the design doc §2.1): `plan/` imports
nothing from this project, and nothing externally beyond the Go standard
library. The schema specifies `PlanID` as a ULID (26-char Crockford base32 —
48-bit ms timestamp + 80-bit randomness).

`NewPlanID()` lives in the package so callers don't each need their own ULID
implementation. The question was: how do we produce a ULID without importing a
third-party library and breaking the zero-deps rule?

## Options considered

### Option A: Import `github.com/oklog/ulid`
- **Pros**: battle-tested, monotonic-entropy variant available, one line of code.
- **Cons**: breaks the zero-deps invariant. `plan/` is the schema contract; any
  internal or external import here inverts the dependency direction and
  pollutes every downstream consumer's `go.sum`.

### Option B: Caller must always supply a PlanID
- **Pros**: maximally minimal package surface; no generation code at all.
- **Cons**: every caller (planner, adapter tests, fixtures) re-implements ULID;
  divergence likely; primitive identity ergonomics suffer.

### Option C (chosen): Inline ULID implementation, stdlib only
- **Pros**: keeps zero-deps invariant. ULID spec is small enough to implement
  honestly inline (6 bytes BE ms timestamp + 10 bytes `crypto/rand` entropy →
  26 chars Crockford base32). Property-tested for length, charset, and
  uniqueness across 100 generations.
- **Cons**: not monotonic within the same millisecond — two calls in the same
  ms have independent 80-bit tails (~2^-40 collision probability per pair).
  Acceptable for narration plans (one per document, not per request). If
  monotonicity is later needed, the caller can layer on a dep — `plan/`
  stays clean.

## Decision

**Option C.** Implement `NewPlanID()` in `plan/plan.go` using `time`,
`crypto/rand`, and `encoding/binary` only. Write the 48-bit ms timestamp into
the first 6 bytes via a scratch `[8]byte` + `copy()` to avoid the trap of
writing past the timestamp boundary into entropy land. Encode all 16 bytes
through a Crockford base32 alphabet constant.

On `crypto/rand.Read` failure: **panic**, not return a sentinel `""`. Silently
fabricating a weak ID would violate the package's own honesty rule. On
darwin/linux `crypto/rand` does not fail in practice; if it does, the system
is unhealthy enough that aborting is the honest move.

## Consequences

- `plan/` stays zero-dep, verified by `TestInvariant_ZeroInternalDeps` and
  `go list -deps plan`.
- ULIDs are correct per spec for length, character set, and uniqueness.
- ULIDs are **not** monotonic within the same millisecond. If a future
  consumer needs strict ordering of IDs generated <1 ms apart, that consumer
  layers a monotonic library on top — `plan/` does not absorb the dep.
- `NewPlanID()` panics on `crypto/rand` failure. Callers that want to handle
  rand failure gracefully must guard with their own wrapper or skip the
  helper and generate IDs themselves.

## Related decisions

- [Testdata fixtures committed verbatim from design doc §2.7](2026-06-18-plan-testdata-verbatim-from-design-doc.md) — the round-trip test that pins this ULID's character class lives in the same package.

## Experiments

`TestNewPlanID_Properties` generates 100 ULIDs, asserts each is length 26,
matches `^[0-9A-HJKMNP-TV-Z]{26}$` (Crockford base32 minus I,L,O,U), and is
unique within the sample. Run on every commit.

## Revisit trigger

Revisit if:
- A downstream consumer demands monotonicity within the same millisecond.
- A second TTS project in the org needs the same ULID helper and we want a
  shared module (in which case `plan/` adopting the dep is acceptable
  because the dep is now first-party).
- `crypto/rand` semantics change in a Go stdlib version we depend on.
