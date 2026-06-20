# `text` arg as transient sentinel — fast-error until ticket #17 lands

- **Date:** 2026-06-19
- **Status:** superseded
- **Superseded by:** [mcptext URI carries sha256(text); adapter cross-checks on Read](2026-06-20-mcptext-uri-sha256-cross-check.md) (2026-06-20, ticket #17)
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

- [mcptext URI carries sha256(text); adapter cross-checks on Read](2026-06-20-mcptext-uri-sha256-cross-check.md) — supersedes this decision. The transient sentinel was removed; the `text` arg now resolves end-to-end via `adapter/mcptext`, with the URI hash cross-check replacing the fast-error as the honesty-rule guard for the inline-text path.
- decisions/convention/2026-06-19-mcp-error-classifier-caller-vs-internal-split.md
- decisions/tradeoff/2026-06-18-persistent-sink-deferred-fast-error.md

## Revisit trigger

Triggered — ticket #17 landed (commit 9cb6e40). Sentinel removed; this decision is now historical context for *why* the schema field stayed in place during the gap between #12 and #17.
