# Tool response envelope receipt-only for v1

- **Date:** 2026-06-19
- **Status:** accepted
- **Category:** convention
- **Tags:** [cmd/narrate-mcp, mcp, response-envelope, schema-version, phase-one, issue-12]
- **Owner:** vd
- **Scope:** cmd-narrate-mcp-issue-12

## Context

The original ticket #12 sketch showed the `speak` tool response wrapping both a `plan` envelope and a `receipt` envelope. During plan-review v1, the API & Contract Reviewer and Tech Debt Sentinel corroborated that the response shape MUST be locked before the build, otherwise v1 ships ambiguous and clients break on the inevitable widening. The plan also flagged in its Open Question 1 that `SinkReceipt` might not carry enough to populate the `plan` block-counts without widening `Pipeline.Narrate`.

## Decision

Response v1 is **receipt-only**:

```json
{
  "receipt": {
    "blocks_played":      N,
    "total_duration_ms":  N,
    "out_dir":            "/tmp/narrate-mcp-..."
  }
}
```

A `plan` envelope (block counts by status, plan_id, level) can be added later as an additive change under CLAUDE.md's schema_version rule ("additive-compatible within a major `schema_version`. Consumers ignore unknown fields"). Locking the receipt-only shape now lets the build ship without contract drift; widening later is a non-breaking change.

`out_dir` is the renderer's per-call temp directory and is deleted by `defer RemoveAll` after the response returns. Included for debugging-window inspection only; clients must not rely on it persisting.

## Consequences

- Wire contract is stable from v1 onward.
- Future `plan` envelope additions don't require a schema_version bump (additive).
- Clients that want plan-block counts will wait for the follow-up; no out-of-band call needed since v1.

## Related decisions

- decisions/architecture/2026-06-18-pipeline-composition-root-pattern.md
- decisions/convention/2026-06-19-mcp-error-classifier-caller-vs-internal-split.md

## Revisit trigger

When a real MCP client surfaces a use case requiring per-block status counts before audio plays.
