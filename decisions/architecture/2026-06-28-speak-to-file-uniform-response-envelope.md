# speak_to_file returns a uniform speakToFileResponse envelope across both path and no-path branches

| Field    | Value            |
|----------|------------------|
| Date     | 2026-06-28       |
| Status   | accepted         |
| Category | architecture     |
| Tags     | speak-to-file, mcp, response-envelope, tool-contract, dual-channel |

## Context

`speak_to_file` has two runtime branches: it **writes a file** when
`output_path` is given, and **falls back to speaking ephemerally** when it is
not. The question is what each branch returns to the client LLM.

## Options considered

### Option A: Different shapes per branch
- **Pros**: each branch returns exactly its own data.
- **Cons**: the client LLM must handle two contracts from one tool; the no-path
  branch would leak `speak`'s envelope shape directly.

### Option B: One uniform envelope
- **Pros**: a single stable contract regardless of whether a file was written;
  mirrors `speak`'s dual-channel (human transcript + structured result) shape;
  the no-path case is expressed as `output_path: ""`.
- **Cons**: the no-path branch must re-wrap the reused `speak` result.

## Decision

Chose **Option B**. Both branches return the same `speakToFileResponse`
envelope, mirroring `speak`'s dual-channel (human transcript + structured
result) shape. The no-path branch reuses `runSpeakWithCache` and **re-wraps**
its result into `speakToFileResponse{output_path: ""}`. Rationale: one stable
contract for the client LLM regardless of whether a file was written.

## Consequences

- The client LLM parses one shape for `speak_to_file` always.
- `output_path == ""` is the wire signal that the tool spoke instead of writing.
- The no-path branch depends on `runSpeakWithCache` and a re-wrap step rather
  than returning `speak`'s envelope directly.

## Related decisions

- [Ship speak_to_file as a separate MCP tool](2026-06-28-speak-to-file-separate-mcp-tool.md) — the tool this envelope serves.
- [resolveOutputPath file-vs-dir rule](2026-06-28-speak-to-file-output-path-file-vs-dir-rule.md) — what sets `output_path` on the path branch.
