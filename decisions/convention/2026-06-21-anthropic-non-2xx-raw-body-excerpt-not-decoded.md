# Anthropic non-2xx errors surface a raw body excerpt, not a decoded error type

| Field    | Value            |
|----------|------------------|
| Date     | 2026-06-21       |
| Status   | accepted         |
| Category | convention       |
| Tags     | intelligence/anthropic, error-handling, non-2xx, convention, issue-34, dead-code |

## Context

Issue #34 (a lint-cleanup chore) surfaced an unused `errorResponse` type in `intelligence/anthropic/api.go`, flagged by `golangci-lint` (`unused`). Its doc comment described an aspirational design: decode the Anthropic `{type, error: {type, message}}` body, capture `.error.message` for the wrapped Go error, and fall back to a truncated raw-body excerpt on decode failure.

The actual adapter never adopted that design. The non-2xx path in `intelligence/anthropic/anthropic.go` (`Voice`, ~line 258) builds its error directly:

```go
return intelligence.IntelligenceResult{}, fmt.Errorf("anthropic: http %d: %s", statusCode, errBodyExcerpt(respBody))
```

`errBodyExcerpt` emits up to 512 bytes of the raw (truncated) response body with no JSON decode. This behavior is locked in by existing tests `TestVoice_Unauthorized401` and `TestVoice_BadRequest400`. The `errorResponse` type was referenced nowhere in the tree.

## Options considered

### Option A: Remove the unused `errorResponse` type (chosen)
- **Pros**: Clears the lint finding; deletes a misleading doc comment that claimed a decode-then-fallback behavior the code never had; keeps the lint chore scoped to cleanup; no change to tested error semantics.
- **Cons**: Loses the pre-written struct shape if structured error decoding is ever wanted (cheap to re-add).

### Option B: Wire `errorResponse` into the non-2xx path
- **Pros**: Richer error string (decoded `.error.message` instead of raw excerpt).
- **Cons**: Changes the tested error-string semantics (would break `TestVoice_Unauthorized401` / `TestVoice_BadRequest400`); adds a feature under a lint-cleanup ticket — out of scope.

## Decision

Remove the never-wired `errorResponse` type rather than wire it in. The chosen non-2xx error surface for the anthropic adapter is the **raw truncated body excerpt** (`errBodyExcerpt`, ≤512 bytes), not a machine-readable decoded error object. The pipeline's upstream classifier owns retry-vs-fail classification, so the adapter does not need structured error decoding — a human-readable error string is the goal. Wiring `errorResponse` in would change tested error semantics and constitutes a feature, out of scope for a lint chore.

## Consequences

- Adapter error strings for non-2xx responses remain raw-body excerpts; downstream code must not depend on a structured error shape from this adapter.
- If structured error classification is ever needed in the adapter, re-introducing a typed decode is a deliberate future feature (see Revisit trigger), not a silent change — it must update the two existing 4xx tests.

## Related decisions

- [Repurpose claude setup-token OAuth token as a raw-API credential via opt-in Bearer auth](../tradeoff/2026-06-21-oauth-bearer-subscription-token-as-api-credential.md) — same adapter (`intelligence/anthropic`), auth surface rather than error surface.

## Revisit trigger

If the adapter ever needs to branch behavior on Anthropic error *type* (e.g., distinguish `overloaded_error` from `invalid_request_error` for adapter-local handling rather than deferring to the upstream classifier), revisit and re-introduce a typed decode — and update `TestVoice_Unauthorized401` / `TestVoice_BadRequest400` accordingly.
