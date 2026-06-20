# Anthropic retry policy — 429 only, max 2 retries, 30s cap

- **Date:** 2026-06-20
- **Status:** experimental
- **Category:** convention
- **Tags:** [intelligence, anthropic, retry, rate-limit, http, issue-15]
- **Owner:** vd
- **Scope:** issue-15

## Context

intelligence/anthropic needs to handle Anthropic API rate limiting (429). The spectrum of policies: no retry, simple exponential backoff, full token-bucket / circuit-breaker, also retry 5xx.

## Options considered

### Option A: 2 retries / 30s cap / Retry-After parsed / 429-only (CHOSEN)
- **Pros**: Handles the common case (transient 429 during a burst). Bounded — worst-case latency is ~60s of waiting then a final error. Respects server hints (Retry-After). Sleeper is injected for fast tests.
- **Cons**: Won't survive a sustained rate-limit incident (3 attempts then error). 5xx is not retried.

### Option B: No retry (return 429 as Go error immediately)
- **Pros**: Simplest. Caller decides.
- **Cons**: Forces every caller to implement retry. The 429 case is common enough that "transient rate-limit looks like a hard failure" is bad UX.

### Option C: Also retry 5xx
- **Pros**: Symmetry. 5xx is sometimes transient.
- **Cons**: Doubles the test matrix without a clear win. The Anthropic API treats 5xx as "your retry is welcome but I make no promises." Adding 5xx retry hides server-side instability rather than surfacing it.

### Option D: Honor arbitrary Retry-After
- **Pros**: Maximally respectful of server hints.
- **Cons**: A misconfigured backend saying "wait 5 minutes" would silently block a CLI invocation for 5 minutes. The cap (30s) makes the worst-case bounded; after the cap, the next attempt is tried regardless.

## Decision

Implementation in intelligence/anthropic/anthropic.go (`doWithRetry`):
- Max 2 retries (3 total attempts).
- Trigger only on 429.
- Parse Retry-After: integer seconds first, then HTTP-date. Cap each sleep at 30s.
- No header → exponential 1s, 2s.
- ctx.Done propagation via injected sleeper (`select { case <-time.After(d): case <-ctx.Done(): return ctx.Err() }`).
- Earlier-attempt errors discarded; only final attempt's surfaces.

## Consequences

- intelligence/anthropic/retry_test.go covers all 7 cases (then-success, header-seconds, header-HTTP-date, no-header-exponential, exhausted, capped-90s-to-30s, ctx-cancel-during-sleep).
- Sustained rate-limit incidents surface as a Go error after ~3s of trying — fast enough that the caller can decide to retry the whole pipeline run.
- A misconfigured backend with a multi-minute Retry-After is bounded at 30s per attempt.
- 5xx hides as-is — if it becomes common in practice, revisit.

## Related decisions

- [No Anthropic SDK — plain net/http + encoding/json](../library-choice/2026-06-20-no-anthropic-sdk.md) — the retry policy is this project's because we own the HTTP layer.
- [Anthropic HTTP test seam — roundTripFunc via WithHTTPClient](2026-06-20-anthropic-http-test-seam-roundtripfunc.md) — paired with WithSleeper for instant-returning test sleeps.

## Revisit trigger

If 5xx errors become common in practice, or if sustained rate-limiting becomes a real concern (e.g. the project starts using the API at higher volume).
