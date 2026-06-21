# Bounded Voice fan-out concurrency default of 4 (defaultIntelligenceConcurrency)

| Field    | Value            |
|----------|------------------|
| Date     | 2026-06-22       |
| Status   | accepted         |
| Category | tradeoff         |
| Tags     | planner, concurrency, anthropic, rate-limit, 429, hobby-key, issue-46 |

## Context

Issue #46 fans out the planner's per-block intelligence Voice calls concurrently. Those
calls hit a backing intelligence adapter — in practice the Anthropic API on a personal
hobby key. Unbounded fan-out (one goroutine per block) on a document with many enrichable
blocks would issue a burst of simultaneous API requests, risking 429 rate-limit errors and
being impolite to a shared/free-tier endpoint. A bound is needed; the question is what
default.

## Options considered

### Option A: Unbounded fan-out (one goroutine per block)
- **Pros**: Maximum parallelism, lowest wall-clock latency.
- **Cons**: Burst 429s on a hobby key; impolite to the API; latency wins are illusory once
  retries/backoff kick in.

### Option B: Bounded fan-out, default 4 (`defaultIntelligenceConcurrency`)
- **Pros**: Meaningful speedup over serial for multi-block docs while keeping concurrent
  in-flight requests low enough to avoid burst 429s on a hobby key. Conservative,
  API-polite default.
- **Cons**: Not the theoretical minimum latency; an arbitrary-ish constant.

## Decision

Bound the Voice fan-out with a concurrency limiter defaulting to 4
(`defaultIntelligenceConcurrency`). The value is deliberately conservative: it stays polite
to the Anthropic API and avoids burst 429s on a hobby key, while still delivering a clear
speedup over the prior serial path for documents with several enrichable blocks. The
project is local-only with no recurring spend, so throughput is traded for politeness and
rate-limit safety.

## Consequences

- Multi-block enrichment is faster than serial but capped; very large documents stream
  through in batches of 4 concurrent intelligence calls.
- The constant is a single named knob (`defaultIntelligenceConcurrency`), easy to revisit
  or make configurable later without touching the fan-out structure.

## Revisit trigger

If a higher-tier API key or a local in-process intelligence backend removes the rate-limit
pressure, raise the default or expose it as a configurable option.
