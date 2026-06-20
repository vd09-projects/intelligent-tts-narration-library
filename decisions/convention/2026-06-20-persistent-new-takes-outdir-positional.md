# Persistent Sink.New takes outDir as a positional argument

- **Date:** 2026-06-20
- **Status:** accepted
- **Category:** convention
- **Tags:** [sink, persistent, api-shape, functional-options, mandatory-arg, issue-16]
- **Owner:** vd
- **Scope:** issue-16

## Context

`sink/ephemeral` uses functional options: `New(opts ...Option)`. No field is mandatory — the zero value plays through `afplay` on the system PATH. `sink/persistent` has a mandatory `OutDir` — there is no sensible default destination. The question: where does `OutDir` go in the constructor signature?

## Options considered

### Option A: `New(outDir string, opts ...Option)` — positional mandatory + optional functional Options (CHOSEN)
- **Pros**: Caller cannot omit `outDir` without a compile error. Functional options remain available for genuinely optional fields (`WithVoice`, `WithExpectedFormat`). API expresses mandatory-ness in the type system.
- **Cons**: Mild API shape divergence from `ephemeral.New(opts...)`.

### Option B: `New(opts ...Option)` with `WithOutDir(string)` option
- **Pros**: Consistent across both sinks.
- **Cons**: Caller can silently omit `WithOutDir` and get a runtime `ErrNoOutDir` (or worse, a `MkdirAll("")` succeeding and writing to CWD). Mandatory-via-runtime-check is a weaker contract than mandatory-via-compile-time-signature.

### Option C: `New(outDir string)` only, no options at all
- **Pros**: Maximally simple.
- **Cons**: Forecloses `WithVoice`, `WithExpectedFormat` — both decided independently as required seams.

## Decision

`persistent.New(outDir string, opts ...Option) *Sink`. The `OutDir` field of the returned Sink is set from the positional argument; `WithVoice` and `WithExpectedFormat` Options layer over the zero value's defaults.

A runtime guard (`ErrNoOutDir` returned from `Consume`) handles the case where a caller constructs a zero-value `Sink{}` directly. The constructor path is the canonical one; the runtime check is the safety net.

## Consequences

- New users of the API have to supply `outDir` at the call site — no chance of forgetting.
- The `ephemeral` vs `persistent` constructor shapes diverge slightly, but each expresses its own mandatory-vs-optional balance honestly.
- The runtime `ErrNoOutDir` guard exists because the public `Sink` struct is constructable with `&Sink{}` (a Go idiom we don't want to take away). It's a backstop, not the primary contract.

## Related decisions

- [Composition root pattern](../architecture/2026-06-18-pipeline-composition-root-pattern.md) — composition root owns concrete-edge construction; making mandatory args mandatory at the constructor reduces the chance of a composition-root bug.

## Revisit trigger

If `outDir` ever becomes optional (e.g. an in-memory persistent sink for tests), pivot the API. Today it's load-bearing — there is no sink behavior without a destination.
