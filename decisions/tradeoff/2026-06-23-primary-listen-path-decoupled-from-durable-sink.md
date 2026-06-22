# Keep the primary listen path decoupled from any durable sink (standing guardrail)

| Field    | Value            |
|----------|------------------|
| Date     | 2026-06-23       |
| Status   | accepted         |
| Category | tradeoff         |
| Tags     | guardrail, decoupling, persistent-sink, ephemeral-sink, speak, sink-lifetime, two-invocations, ticket-72, issue-73, v3-adr |

## Context

Ticket #72, v3 ADR correction. The plan review (`review-findings-plan-v3.md`) surfaced this as the **single highest-value guardrail for #73** and as a CORROBORATED finding (2+ reviewers: Dependency & Coupling + Tech Debt Sentinel): the primary listen path must stay decoupled from any durable sink.

The primary terminal "listen, not read" path is `speak → sink/ephemeral → afplay` (audio-only). The optional visual companion (React player + `cmd/narrate-server`) reads a durable `sink/persistent` outDir. The risk for the implementation ticket (#73) is that an implementer re-introduces sink-lifetime coupling by teeing the persistent artifact off the `speak` call — making the primary audio path also write/keep a durable directory.

## Options considered

### Option A: Two invocations, one render core — visual companion is a separate `--sink persistent` render — CHOSEN
- **Pros**: Primary `speak` path keeps only `cmd/narrate-mcp → pipeline → sherpa + ephemeral → afplay` (all already wired); ephemeral sink lifetime unchanged (temp dir deleted on return); no durable-sink coupling on the speak call; strictly lower coupling than v2; `no-I/O-in-planner` holds (buffering at cmd/caller layer; planner gets one whole input).
- **Cons**: A user who wants both audio and a persisted artifact runs two invocations rather than one.

### Option B: Tee the persistent outDir off the speak call
- **Pros**: One invocation yields both audio and a durable artifact.
- **Cons**: Re-introduces sink-lifetime coupling onto the primary path; makes `speak` depend on a durable sink; contradicts the receipt-only/ephemeral envelope intent; the exact regression the guardrail exists to prevent.

## Decision

The primary terminal narration path stays **decoupled from the persistent/durable sink**. A visual companion that needs persisted artifacts is reached by a **separate `--sink persistent` invocation**, not a tee off the `speak` call. This is a standing guardrail / standing order: it constrains follow-up implementation ticket #73 so an implementer does not re-couple `speak` to a durable sink. Intent to record verbatim on #73: "two paths, one render core."

## Consequences

- #73 inherits this as a hard constraint: opting into the visual companion is a separate render invocation, not a tee off speak.
- Keeps the ephemeral sink's "play then delete temp dir" lifetime intact; `out_dir` on the receipt stays a debugging-window field clients must not depend on.
- Where the L2 listen-mode default lives (a `speakArgs`/`PipelineDefaults` field vs a caller-passed level) is an open pin for #73 (Ripple Effect Analyst) — orthogonal to this guardrail but flagged alongside it.

## Related decisions

- [Terminal "listen, not read" is the existing speak → ephemeral → afplay path](../architecture/2026-06-23-terminal-listen-not-read-is-ephemeral-afplay-audio-only.md) — the primary path this guardrail protects.
- [React player is optional and reuses the existing player (#50)](../architecture/2026-06-23-react-player-optional-reuses-existing-player-50.md) — the path that legitimately uses the durable sink, via its own invocation.

## Revisit trigger

If a single-invocation "play and persist" workflow becomes a real user need on #73 or later, revisit this guardrail explicitly rather than letting the coupling creep in — and re-confirm the ephemeral sink lifetime is not compromised.
