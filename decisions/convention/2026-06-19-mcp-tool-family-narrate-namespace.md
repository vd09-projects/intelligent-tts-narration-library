# MCP tool family — `narrate.*` namespace

- **Date:** 2026-06-19
- **Status:** accepted
- **Category:** convention
- **Tags:** [cmd/narrate-mcp, mcp, tool-naming, namespace, documentation, phase-one, issue-12]
- **Owner:** vd
- **Scope:** cmd-narrate-mcp-issue-12

## Context

Plan-review v1 raised two questions about MCP tool layout: (1) tool name `speak` is fine but unscoped — future tools (e.g. block escalation) need a clear naming home; (2) the README must commit to a primary MCP client for the install snippet so the contract is testable.

## Decision

This server's MCP tool family is `narrate.*`. The single tool `speak` is the canonical entry point for phase one. Future tools (e.g. `narrate.escalate` for per-block level upgrade, `narrate.preview` for plan-only inspection) belong under the same server.

README MCP-client install snippet targets **Claude Desktop's `~/Library/Application Support/Claude/claude_desktop_config.json`** as canonical. The `mcp` CLI snippet is documented as secondary, for power-user smoke testing. Other MCP clients (Claude Code, etc.) are linked through upstream docs rather than transcribed (drift risk).

Documented in the package comment of `cmd/narrate-mcp/main.go` (`Tool family: narrate.* — currently 'speak' is the only registered tool`) and the README section "MCP server (`cmd/narrate-mcp`)".

## Consequences

- Future tools have a clear naming home and don't compete for the unscoped name.
- README install instructions have one canonical example, not three drifting ones.
- A user who follows the Claude Desktop snippet will succeed; a user who reaches for `mcp` CLI gets the secondary path with a doc-drift caveat.

## Related decisions

- decisions/convention/2026-06-19-mcp-speak-response-receipt-only-envelope.md
- decisions/convention/2026-06-19-text-arg-transient-sentinel.md

## Revisit trigger

When a second tool joins the server (validates the `narrate.*` family naming choice).
