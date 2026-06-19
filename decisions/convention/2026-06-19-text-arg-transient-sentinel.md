# `text` arg as transient sentinel — fast-error until ticket #17 lands

- **Date:** 2026-06-19
- **Status:** accepted
- **Category:** convention
- **Tags:** [cmd/narrate-mcp, mcp, text-arg, transient-sentinel, honesty-rule, phase-one, issue-12]
- **Owner:** vd
- **Scope:** cmd-narrate-mcp-issue-12

## Context

The MCP `speak` tool's A16 spec accepts either `source` (file path) or `text` (inline markdown). This ticket implements only the `source` path; the `text` path needs an in-memory adapter that is the scope of follow-up ticket #17 (mcptext adapter). Plan v2 + review v1 chose explicit honest fast-error over silently dropping the arg or silently falling back to "source-only".

## Decision

The `text` arg remains in the JSON schema for forward compatibility — clients calling the future-ready API don't need to know which release implemented it. The handler validates with sentinel `errTextNotImplemented`:

```
caller-error: invalid_argument: text arg not implemented in issue #12 (see mcptext adapter ticket #17); use source
```

When ticket #17 lands, the sentinel is removed and the validate() XOR branch routes `text != ""` through the new in-memory adapter instead of returning the sentinel. The arg schema does not change. The Decision v4 inline package comment in `cmd/narrate-mcp/main.go` documents the sentinel as transient.

## Consequences

- MCP clients calling `speak` with `text` set get a clear actionable error instead of a silent "did nothing" success.
- The Decision-recording fits the project's honesty rule (errors stop, refusals are data): this case is "caller asked for something we cannot fulfill" — error, not refusal.
- When #17 lands, the change is internal-only — no schema break, no client migration.

## Related decisions

- decisions/convention/2026-06-19-mcp-error-classifier-caller-vs-internal-split.md
- decisions/tradeoff/2026-06-18-persistent-sink-deferred-fast-error.md

## Revisit trigger

Ticket #17 (mcptext in-memory adapter) — that ticket removes this sentinel and supersedes this decision.
