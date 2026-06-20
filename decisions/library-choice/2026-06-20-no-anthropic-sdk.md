# No Anthropic SDK — plain net/http + encoding/json

- **Date:** 2026-06-20
- **Status:** accepted
- **Category:** library-choice
- **Tags:** [intelligence, anthropic, http, stdlib, dependencies, issue-15]
- **Owner:** vd
- **Scope:** issue-15

## Context

intelligence/anthropic talks to the Anthropic Messages API. The official `github.com/anthropics/anthropic-sdk-go` is permissively licensed and would work, but its surface is broader than this project needs (batch API, files API, streaming, MCP-client features). The Messages API request/response shape we use is small and stable.

## Options considered

### Option A: Plain net/http + encoding/json (CHOSEN)
- **Pros**: Zero new go.mod dependencies. No transitive release-cadence coupling. ~80 LOC of request/response structs. Full control over retry / Retry-After parse / context.Cancel flow. Test seam (roundTripFunc) is stdlib.
- **Cons**: We own the struct definitions; field name changes upstream require manual updates.

### Option B: Adopt `github.com/anthropics/anthropic-sdk-go`
- **Pros**: Idiomatic. Field-name drift handled by upstream.
- **Cons**: Pulls in batch / files / streaming / MCP-client surface we do not use. Couples release cadence. Higher upgrade-impact surface for a hobby project. Custom retry / sleeper injection (Decision v5) would be awkward to wedge in.

## Decision

Plain net/http + encoding/json. Request/response structs live in intelligence/anthropic/api.go. Retry semantics are this project's, not the SDK's.

## Consequences

- intelligence/anthropic has no external go-module dependencies; deps_test allowlist stays narrow.
- An API field-shape change upstream is a manual edit here; mitigation is "we use a tiny surface."
- If the project starts using batch API, streaming, or the SDK's higher-level abstractions, revisit and switch.

## Related decisions

- [HTTP test seam = roundTripFunc via WithHTTPClient](../convention/2026-06-20-anthropic-http-test-seam-roundtripfunc.md) — the stdlib-only test seam this decision enables.
- [Anthropic retry policy — 429 only, bounded](../convention/2026-06-20-anthropic-retry-policy-429-only.md) — control the SDK abdicates.

## Revisit trigger

If the project starts using the Anthropic batch API, streaming, or the SDK's prompt-caching abstractions.
