# Repurpose `claude setup-token` subscription OAuth token as a raw-API credential via opt-in Bearer auth

- **Date:** 2026-06-21
- **Status:** accepted
- **Category:** tradeoff
- **Tags:** [intelligence/anthropic, auth, credentials, oauth, bearer, x-api-key, tos, issue-32]
- **Source:** harvested from intelligence-anthropic-oauth-bearer-32 build review, decision mark v1

## Context

The `intelligence/anthropic` direct-API adapter authenticated only via the `x-api-key` header — the form an Anthropic Console key (`sk-ant-api03-`) expects. A Claude *subscription* OAuth token minted by `claude setup-token` (prefix `sk-ant-oat01-`) is rejected with `401 invalid x-api-key` on that header, but the same token returns `200 OK` on `POST /v1/messages` when presented as `Authorization: Bearer <token>` together with `anthropic-beta: oauth-2025-04-20`. Wiring this in lets the hobby project run intelligence on a subscription the user already pays for, avoiding per-token Anthropic Console billing — directly serving the "no recurring spend" project constraint. The catch: repurposing a subscription OAuth token as a raw-API credential is a gray area against Anthropic's usage terms.

## Options considered

### Option A: opt-in Bearer auth mode, x-api-key stays default, auto-detect on oat prefix (chosen)
- **Pros**: Zero behavior change on the existing `sk-ant-api03-` path (counter-metric preserved). Subscription token "just works" via prefix auto-detect — no flag for the common case. Explicit `WithBearerAuth()` escape hatch. ToS risk is opt-in, never imposed.
- **Cons**: Two credential-header code paths to maintain. Auto-detect is silent. Carries a documented ToS risk the server may act on without notice.

### Option B: do not support subscription tokens
- **Pros**: Zero ToS exposure; single auth path.
- **Cons**: Forces a paid Console key on a local-only hobby project, contradicting the "no recurring spend" deployment constraint.

### Option C: make Bearer the default / always-on
- **Pros**: Simpler single path.
- **Cons**: Breaks the existing Console-key path; imposes the ToS gray-area risk on every user involuntarily. Rejected on backward-compat and honesty grounds.

## Decision

Add an **opt-in** `Authorization: Bearer` auth mode to the adapter. `x-api-key` remains the **default** and is never removed. Bearer is selected by an explicit `WithBearerAuth()` functional option, AND auto-detected when the API key carries the `sk-ant-oat01` prefix — an **explicit option always wins** over the prefix auto-detect. The Bearer path additionally sends `anthropic-beta: oauth-2025-04-20` and omits `x-api-key`; the default path sends only `x-api-key`. The `anthropic-beta` value and the oat prefix are pinned as named constants (`anthropicBetaOAuth`, `oatTokenPrefix`) so a future upstream bump is a one-line edit.

Accepted **with a documented Terms-of-Service gray-area caveat**: repurposing a `claude setup-token` OAuth token as a raw Anthropic API credential may be revoked or flagged by the server without notice (ref [anthropics/claude-code#1785](https://github.com/anthropics/claude-code/issues/1785)). The caveat lives verbatim in the `WithBearerAuth` option godoc and in the package doc header. This opt-in-default posture is precisely the mitigation — the risk is never imposed on the existing Console-key path.

## Consequences

- One existing local actor's already-held subscription token now authenticates; no new actor gains access and no privilege escalation occurs — it is a credential-form expansion, not a permission grant across actors.
- The server may revoke/flag the oat token at any time; if it does, the call 401s through the existing error path. No code change shields against this — it is an accepted, named risk.
- If `anthropic-beta: oauth-2025-04-20` changes upstream, the named constant is the single edit point.
- Auto-detect could false-positive on a hypothetical future non-oat token sharing the `sk-ant-oat01` prefix; the explicit option override is the escape hatch. (Build review noted that matching `oatTokenPrefix + "-"` would tighten this marginally — non-blocking follow-up, not adopted.)
- The header branch uses `req.Header.Set` (not `Add`) on the per-attempt rebuilt request, so a retry never accumulates duplicate auth headers, and `x-api-key` and `Bearer` are never co-set in one request.

## Related decisions

- No-Anthropic-SDK / stdlib-only HTTP (`library-choice`) — this change honors it: plain `net/http`, no new dependency.
- Construction-time key validation (`anthropic.New` returns error on empty key) — preserved; the auth-mode addition does not relax it.

## Revisit trigger

- If Anthropic publishes an official subscription-token API path, or formally permits/prohibits this usage, revisit whether Bearer should remain opt-in, become officially supported, or be removed.
- If the `anthropic-beta: oauth-2025-04-20` flow is deprecated upstream (401/400 on the beta header), revisit the pinned constant and the whole Bearer path.
