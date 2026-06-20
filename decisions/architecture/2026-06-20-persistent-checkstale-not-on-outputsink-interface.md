# Persistent sink's CheckStale is NOT part of the OutputSink interface

- **Date:** 2026-06-20
- **Status:** accepted
- **Category:** architecture
- **Tags:** [sink, persistent, interface-design, read-side-query, ephemeral, issue-16]
- **Owner:** vd
- **Scope:** issue-16

## Context

`sink/persistent.CheckStale(outDir, plan) (stale bool, reason string, err error)` is a read-side query that asks "is this persisted output still aligned with the supplied plan?". It compares `manifest.json` content_hash + source URI/kind against the plan's `Source` fields. The natural question: should `CheckStale` live on the `sink.OutputSink` interface so callers can ask any sink "are you stale?".

## Options considered

### Option A: `CheckStale` is package-scope, NOT on the interface (CHOSEN)
- **Pros**: The `OutputSink` interface stays narrow ("write the bytes"). Ephemeral sinks don't have a `manifest.json` to check — adding `CheckStale` to the interface would force a stub implementation on `sink/ephemeral` that always returns `(false, "", nil)`. The staleness concept is genuinely persistent-sink-specific. Callers that care about staleness know they're using the persistent sink; the package-scope function is discoverable via godoc.
- **Cons**: Two-sink-style branching at the call site if a caller wants to handle both ("if persistent, check stale; else skip"). Today there is no such caller.

### Option B: Extend `OutputSink` with `CheckStale(plan) (bool, string, error)`
- **Pros**: Polymorphic API. Future sinks (e.g. S3, blob storage) inherit the contract.
- **Cons**: Forces a stub on ephemeral. The interface gains a method most implementations have no semantic meaning for. Premature abstraction.

### Option C: A separate `StalenessChecker` interface that persistent implements
- **Pros**: Composable; callers type-assert at the call site.
- **Cons**: Speculative — no second implementation needed today. Adds an interface for the sake of having an interface.

## Decision

`CheckStale` lives in `sink/persistent` as a package-scope function (not a method, not on an interface). Callers that need staleness import `sink/persistent` directly and call `persistent.CheckStale(outDir, plan)`. The `OutputSink` interface remains exactly "write the bytes" via `Consume`.

The compile-time interface assertion `var _ sink.OutputSink = (*Sink)(nil)` documents that the sink implements `OutputSink` and nothing more.

## Consequences

- Adding a future sink (S3, in-memory, etc.) does not force a `CheckStale` implementation. The new sink declares its own staleness semantics if needed.
- A caller that wants "ask any sink" semantics must today choose persistent explicitly. This is honest: ephemeral cannot be stale; the question is malformed for it.
- If a second sink genuinely shares the same content-hash + source-URI staleness semantics, lifting `CheckStale` to a shared interface is a clean refactor — the persistent implementation becomes a method.

## Related decisions

- [Sink imports render for RenderResult](2026-06-19-sink-imports-render-for-renderresult.md) — establishes the OutputSink interface's narrow shape.

## Revisit trigger

If a second sink with shared content-hash staleness semantics ships (e.g. a future S3 sink that also writes a manifest), promote `CheckStale` to a shared interface (`StalenessChecker`) and update callers via type-assertion or composition.
