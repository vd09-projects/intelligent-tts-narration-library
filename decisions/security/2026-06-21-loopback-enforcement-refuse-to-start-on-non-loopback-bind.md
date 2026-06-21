# Loopback enforcement — refuse to start (non-zero exit) on any non-loopback bind host, not bind-all-then-filter

| Field    | Value            |
|----------|------------------|
| Date     | 2026-06-21       |
| Status   | accepted         |
| Category | security         |
| Tags     | cmd/narrate-server, http-escalate, loopback, bind-host, local-only, fail-closed, secrets, issue-49 |

## Context

Issue #49 adds an HTTP escalate endpoint (`cmd/narrate-server`). The library is local-only (CLAUDE.md "Deployment: local-only", and the gotcha that a secret in a config block could be spoken aloud on the user's own machine). An HTTP server introduces a network surface that did not previously exist. The question: how should the server defend against being accidentally exposed on a public interface?

## Options considered

### Option A: Refuse to start — non-zero exit on any non-loopback bind host
- **Pros**: Fail-closed. The server physically cannot bind a public interface, so it cannot be accidentally exposed even via misconfiguration. The error is loud and immediate at startup, not a silent runtime condition. Matches the project's honesty/refuse-don't-corrupt discipline.
- **Cons**: A user with a legitimate non-loopback need (none in phase one) must change the code. Slightly less flexible.

### Option B: Bind all interfaces, then filter requests at runtime by source address
- **Pros**: More flexible; could allow LAN access with an allowlist later.
- **Cons**: Fail-open by construction — the listener is already bound to a public interface, so a filter bug, a misordered middleware, or a future refactor re-exposes it. The dangerous state (publicly bound socket) exists even when filtering works. Wrong default for a local-only tool that may speak secrets.

## Decision

Adopt **Option A**: the server **refuses to start (non-zero exit)** if the configured bind host is not a loopback address, rather than binding all interfaces and filtering at runtime.

Reasoning: you cannot accidentally expose the server if it refuses to bind a public interface in the first place. This is fail-closed — the dangerous socket state never exists. It aligns directly with the local-only CLAUDE.md posture and the gotcha that secrets may be read aloud on the user's machine; the network surface stays confined to the loopback interface by construction, not by a runtime check that could regress.

## Consequences

- The escalate server is reachable only from the local host. No LAN/public exposure is possible without an explicit, deliberate code change.
- Misconfiguration surfaces as an immediate startup failure (non-zero exit), not a silent security hole.
- If multi-host access is ever wanted, it requires a conscious decision plus an auth story — it cannot happen by accident.

## Related decisions

- [MCP error classifier — caller-error vs internal-error split](../convention/2026-06-19-mcp-error-classifier-caller-vs-internal-split.md) — sibling local-only entry point; same refuse-don't-corrupt posture at a different surface.

## Revisit trigger

Revisit if a genuine multi-host / remote-escalate use case appears — at which point loopback enforcement must be paired with an explicit auth mechanism before being relaxed.
