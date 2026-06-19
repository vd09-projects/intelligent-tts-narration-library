# MCP error classifier — caller-error vs internal-error split

- **Date:** 2026-06-19
- **Status:** accepted
- **Category:** convention
- **Tags:** [cmd/narrate-mcp, mcp, error-handling, classifier, honesty-rule, phase-one, issue-12]
- **Owner:** vd
- **Scope:** cmd-narrate-mcp-issue-12

## Context

The original plan mapped all adapter/renderer/sink failures to a single MCP `internal_error`. The Error Handling & Resilience Inspector flagged this in plan-review v1 (B2): file-not-found and permission-denied are caller errors (bad request shape), not server-internal failures. A single `internal_error` bucket would prevent MCP clients from self-correcting on bad paths.

## Decision

Pipeline errors are classified into three buckets, with wire-message prefixes that make the split observable in the `CallToolResult.IsError` content:

| Source | Wire prefix |
|---|---|
| arg validation failure (XOR, enum, range) | `caller-error: invalid_argument:` |
| `text` arg supplied | `caller-error: invalid_argument:` (text not implemented) |
| `sink=persistent` | `caller-error: invalid_argument:` (sink not implemented) |
| adapter `fs.ErrNotExist` (wrapped) | `caller-error: invalid_argument: source not found:` |
| adapter `fs.ErrPermission` (wrapped) | `caller-error: invalid_argument: source permission denied:` |
| renderer / sink failure | `internal_error: pipeline failure:` |
| context cancelled / deadline | `cancelled:` |

Split rule: anything the caller could fix by changing the request is `caller-error`; anything the server cannot fulfill regardless of request shape is `internal_error`.

The MCP SDK design returns tool errors via `IsError=true` content (not raw `error` from the handler), so the classification text is the actual wire contract. The classifier function `classifyPipelineErr` is testable independently of the SDK.

## Consequences

- MCP clients (Claude Desktop, Claude Code, mcp CLI) can pattern-match on the prefix to decide whether to retry with a different request.
- Refusals are NOT errors per CLAUDE.md honesty rule: refused blocks stay inside the plan, audio plays, and the call returns a normal receipt.
- The classifier is unit-tested against both synthetic `fakePathError` and the real `adapter/file` error path.

## Related decisions

- decisions/convention/2026-06-19-mcp-speak-response-receipt-only-envelope.md
- decisions/convention/2026-06-18-refused-block-message-rendered.md

## Revisit trigger

When MCP standardizes a typed error code enum for tools (currently the spec relies on `IsError` + free-form text).
