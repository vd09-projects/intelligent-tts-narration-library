# Anthropic HTTP test seam — roundTripFunc via WithHTTPClient

- **Date:** 2026-06-20
- **Status:** accepted
- **Category:** convention
- **Tags:** [intelligence, anthropic, testing, http, stdlib, issue-15]
- **Owner:** vd
- **Scope:** issue-15

## Context

intelligence/anthropic needs an HTTP test seam so the suite runs without live API calls. Two natural choices: a custom `Transport` interface on the package, or pass an `*http.Client` whose `Transport` is set to a stdlib-compatible `http.RoundTripper`.

## Options considered

### Option A: `WithHTTPClient(*http.Client)` Option + tests use roundTripFunc (CHOSEN)
- **Pros**: Stays close to stdlib. roundTripFunc is a tiny adapter (`type roundTripFunc func(*http.Request) (*http.Response, error)` with one `RoundTrip` method). No project-specific test surface. Users who want higher-level mocks can wrap *http.Client themselves.
- **Cons**: Tests have to define roundTripFunc in their own file (one-liner).

### Option B: Custom `Transport` interface on the package
- **Pros**: Smaller call site in tests.
- **Cons**: Adds project surface for what http.RoundTripper already names. Couples the package to a custom seam users would need to learn.

### Option C: `httptest.Server` per test
- **Pros**: Most realistic — actual HTTP round-trip.
- **Cons**: Heavier (spins up a listener). Slower. roundTripFunc is sufficient for the request-inspection / response-canning the suite needs.

## Decision

`WithHTTPClient(*http.Client)` Option. Tests construct `&http.Client{Transport: roundTripFunc(...)}` and pass it in. The seam is stdlib-native.

## Consequences

- intelligence/anthropic/anthropic_test.go + retry_test.go + cache_test.go all use the same `newAdapter(t, rt, opts...)` helper that wraps the rt in a Client and applies WithHTTPClient.
- No project-specific Transport interface. Users wanting integration tests with httptest.Server can build their own client and pass it in.
- The sleeper test seam (Decision v5) follows the same pattern: inject via Option, default to stdlib behavior.

## Related decisions

- [No Anthropic SDK — plain net/http + encoding/json](../library-choice/2026-06-20-no-anthropic-sdk.md) — sibling decision that this test seam piggybacks on.
- [Anthropic retry policy — 429 only, bounded](2026-06-20-anthropic-retry-policy-429-only.md) — also uses an injected-seam pattern (WithSleeper).

## Revisit trigger

If users start needing a higher-level abstraction (e.g. observable transport, retry middleware) — that would call for an explicit Transport interface.
