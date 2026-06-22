# MCP SDK discrepancy note — ticket says mark3labs/mcp-go, project uses official go-sdk (open question for #73)

| Field    | Value            |
|----------|------------------|
| Date     | 2026-06-23       |
| Status   | revisit-later    |
| Category | library-choice   |
| Tags     | mcp-sdk, mark3labs, modelcontextprotocol-go-sdk, discrepancy, note, open-question, ticket-72, issue-73, v3-adr |

## Context

Ticket #72, v3 ADR review (`review-findings-plan-v3.md`, `planner-architecture.md` v3). During the review a discrepancy was surfaced and preserved from v2 as still-correct: the originating ticket text refers to the MCP SDK `mark3labs/mcp-go`, but the project actually uses the **official** `github.com/modelcontextprotocol/go-sdk` v1.5.0 (the SDK whose transitive requirement bumped the Go minimum to 1.25 in issue #12).

This is not a decision to change anything now — it is a note flagged as relevant to follow-up implementation ticket #73, recorded so that #73 inherits the context and the implementer does not act on the ticket's stale SDK reference.

## Decision

Recorded as a **note / open question for #73**, not an action: the MCP SDK named in the ticket (`mark3labs/mcp-go`) does **not** match the SDK in use (official `github.com/modelcontextprotocol/go-sdk` v1.5.0). #73 should resolve the discrepancy — treat the official go-sdk as authoritative (it is what ships) and correct/disregard the ticket's `mark3labs/mcp-go` reference. No code change ships from #72 (the ADR is analysis-only).

## Consequences

- #73 inherits an explicit flag to not introduce or assume `mark3labs/mcp-go`; the official go-sdk is the in-use SDK.
- If left unaddressed, an implementer following the ticket text literally could pull in the wrong SDK and re-fork the MCP server wiring.

## Related decisions

- [Terminal "listen, not read" is the existing speak → ephemeral → afplay path](../architecture/2026-06-23-terminal-listen-not-read-is-ephemeral-afplay-audio-only.md) — the v3 ADR this note rides alongside.

## Revisit trigger

Resolve on follow-up ticket #73: confirm the official `github.com/modelcontextprotocol/go-sdk` v1.5.0 is authoritative and the ticket's `mark3labs/mcp-go` reference is corrected or disregarded. Close this note once #73 confirms the SDK choice.
