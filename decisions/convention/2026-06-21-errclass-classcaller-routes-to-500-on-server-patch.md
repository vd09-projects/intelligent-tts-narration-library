# ClassInternal is the zero value; ClassCaller routes to 500 on the server patch path (category-vs-wire split)

| Field    | Value            |
|----------|------------------|
| Date     | 2026-06-21       |
| Status   | accepted         |
| Category | convention       |
| Tags     | internal/errclass, cmd/narrate-server, cmd/narrate-mcp, error-classification, zero-value, fail-safe, wire-mapping, issue-51 |

## Context

`internal/errclass` (task #51) decides only the CATEGORY of a pipeline/patch error (`ClassInternal`, `ClassCaller`, `ClassCancelled`). It owns nothing about wire format — no strings, no HTTP codes, no error wrapping. Each composition root keeps its own per-root mapping from `Class` to its wire contract. Two design points needed pinning: (1) which constant is the iota zero value, and (2) where a `ClassCaller` error lands on the server's patch path.

## Decision

**ClassInternal is the iota zero value.** Both roots' `default` branch is internal — MCP emits `"internal_error: pipeline failure"`, the server returns `500 reasonInternal`. Making `ClassInternal` the zero value means a forgotten, uninitialized, or unrecognized path **fails safe to internal**, matching today's behavior. (Even `Classify(nil)`, which never reaches the function in production since both roots guard non-nil, returns `ClassInternal` by this default — documented as defined-but-unreached and exercised by the unit test.)

**On the server patch path, `ClassCaller` routes to `500 reasonInternal`, NOT a 4xx.** This is a deliberate **category-vs-wire split**: the SAME caller-class error (e.g. `fs.ErrNotExist`, `fs.ErrPermission`) maps to a 4xx `invalid_argument` on the MCP root but to a `500 reasonInternal` on the server patch path. `errclass` returns the category; each root chooses the wire mapping. The server adds NO caller case — `ClassCaller` falls through to the existing `500` default branch, preserving today's behavior. (The server's read-path source-resolution 400/404 lives in `classifySourceErr` and is untouched.)

The governing convention: **errclass owns category only; each root owns its own Class -> wire mapping** (strings, HTTP status, reason tokens, and any `%w` wrapping). The same category can legitimately produce different wire outcomes in different roots.

## Consequences

- A new/forgotten classification path degrades to internal rather than mis-reporting as caller-correctable — fail-safe by construction.
- Identical errors produce different wire responses across roots by design; anyone reading only the category must remember the wire mapping is per-root. This is intentional, not an inconsistency to "fix" by unifying the mapping.
- Adding fs/caller 4xx handling to the server patch path is explicitly out of scope — it maps to 500 today and stays 500.

## Related decisions

- [MCP error classifier caller-vs-internal split](2026-06-19-mcp-error-classifier-caller-vs-internal-split.md) — the original per-root classification split that #51 consolidates; this decision records how the consolidated category maps differently per root.

## Revisit trigger

If a real use case requires the server patch path to distinguish caller-correctable errors as 4xx (e.g. a richer escalate API contract), the server would add an explicit `ClassCaller` case instead of falling through to the 500 default.
