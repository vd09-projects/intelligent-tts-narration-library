# Shared transcript parser lives in internal/transcript with the speak_last skip as caller policy; Message.Turn is an emit-index, not a stable id

| Field    | Value            |
|----------|------------------|
| Date     | 2026-06-28       |
| Status   | accepted         |
| Category | architecture     |
| Tags     | transcript-parser, internal-package, claude-code-jsonl, speak_last, tool-agnostic, caller-policy, emit-index, not-a-stable-id, additive-extensible, pagination-cursor, decoupled-listen-path, issue-106, issue-109, adr-86, adr-81 |

## Context

The Claude Code transcript `.jsonl` parser was baked into `cmd/narrate-mcp`'s `lastAssistantText`, hard-wired two ways: it returned only the **last** assistant turn, AND it carried knowledge of the `speak_last` tool name so it could skip narrate-mcp's own self-invocation. That coupling was fine while `speak_last` was the only consumer, but two new consumers now need the **full ordered message list**: Earshot session loading and narrate-server #109 (`GET /sessions/{id}/messages`). A last-turn-only, tool-aware function cannot serve either.

Constraints honored: the primary listen path must stay decoupled from any durable sink (standing order); `speak_last` behavior must remain byte-identical (the 6-row `TestLastAssistantText` oracle is unchanged); and real user turns carry array content (e.g. `tool_result`), not just strings.

## Options considered

### Option A: Extract a tool-agnostic shared parser into internal/transcript (chosen)
- **Pros**: off the public module surface (`internal/`), importable by both `cmd/` roots without one binary depending on another's surface; parser stays pure and tool-agnostic; both #109 and Earshot get the full ordered list; `speak_last` self-skip becomes a thin caller-side filter.
- **Cons**: introduces a third `internal/` sibling package; callers must own their own filtering policy.

### Option B: Export the parser from cmd/narrate-mcp
- **Cons**: would make one command binary depend on another command binary's surface — an inversion the project structure avoids.

### Option C: Keep the speak_last name inside the parser
- **Cons**: re-bakes exactly the tool/parser coupling this ticket removed; every new consumer would inherit a skip it doesn't want.

### Option D: Assume user content is a plain string
- **Cons**: wrong — real user turns carry array content like `tool_result`; a string-only assumption drops or mis-parses those turns.

## Decision

1. **Extract a shared parser into a new internal package `internal/transcript`** (sibling to `internal/errclass` and `internal/intelligencetmpl`), exposing `ParseMessages(path) ([]Message, error)` that returns the **full ordered** `[]Message{Turn, Role, Text, ToolNames}`. `internal/` keeps it off the public module surface while letting both `cmd/` roots import it without one binary depending on another.

2. **The parser stays TOOL-AGNOSTIC.** It records `tool_use` names in `ToolNames` but knows no specific tool. The `speak_last` self-invocation skip stays **caller-side policy** in `cmd/narrate-mcp` (`slices.Contains` over `ToolNames`), not parser logic.

3. **`Message.Turn` is an EMIT-INDEX** — position among emitted messages, tool-only turns included — and it **renumbers** if a previously-skipped/unparseable line later parses. It is explicitly **NOT a stable identifier**.

## Consequences

- **Forward contract for #109:** narrate-server must NOT use `Message.Turn` as a pagination cursor; it should prefer a line-derived stable id (transcript timestamp/UUID).
- `Message` is **additively extensible** — #109 may widen it with a timestamp/UUID field without breaking the schema or existing consumers.
- `speak_last` remains byte-identical: it is a pure parser refactor plus a caller-side filter; the 6-row `TestLastAssistantText` oracle is unchanged.
- Honors the standing order "keep the primary listen path decoupled from any durable sink" — nothing about the listen/durable boundary moves here.

## Related decisions

- [Cap the MCP speak per-block transcript via head-keep tail-truncate](2026-06-27-cap-mcp-speak-transcript-head-keep-tail-truncate.md) — prior decision (#86) governing the MCP speak transcript that this parser feeds.
- [Channel-2 live observer mechanism: append-only JSONL + tail -f](2026-06-26-channel2-mechanism-jsonl-tail-over-mcp-progress.md) — prior decision (#81) establishing the channel-2 JSONL substrate this parser reads.

## Revisit trigger

When #109 (`GET /sessions/{id}/messages`) is implemented: confirm it adopts a line-derived stable id (timestamp/UUID) as its pagination cursor rather than `Message.Turn`, and widen `Message` additively if it needs that id on the wire.
