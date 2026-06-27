# Ship speak_to_file as a separate MCP tool, not an option on speak

| Field    | Value            |
|----------|------------------|
| Date     | 2026-06-28       |
| Status   | accepted         |
| Category | architecture     |
| Tags     | earshot, mcp, speak-to-file, speak, single-wav-output, output-path, persistent-sink, tool-surface, priority-tool |

## Context

Use case 3 wants text **or** a `.md`/text file converted to one audio file at a
caller-given path (and, with no path, spoken). Today the MCP `speak` tool plays
ephemerally only; the persistent sink (CLI-only) writes a **3-file directory**
(`audio.wav`+`plan.json`+`manifest.json`), not a single wav at a path. The
question is whether to add an `output_path` to `speak` or ship a new tool.

## Options considered

### Option A: Add `output_path` to `speak`
- **Pros**: one tool.
- **Cons**: overloads a play-only tool with a write-file mode; mixed contract
  (sometimes plays, sometimes writes); harder to describe to the client LLM.

### Option B: Separate `speak_to_file` tool
- **Pros**: single-purpose contract; `speak` stays play-only; clear semantics —
  `{text | source, output_path?, level, gender}`, input is inline text or a
  path to a text/`.md` file, writes one `.wav` to `output_path` (file or dir),
  falls back to speak when no path. Reuses the persistent sink's wav-concat math
  without writing the JSON sidecars.
- **Cons**: a third tool to register and document.

## Decision

Chose **Option B** — ship `speak_to_file` as a separate MCP tool. This is the
**priority tool, built first**. Rationale: single-purpose tool contracts keep
each tool easy for the client LLM to invoke correctly; `speak` stays play-only;
the file-output path is genuinely new surface (single wav vs the existing 3-file
directory sink) and deserves its own tool rather than a mode flag.

## Consequences

- New MCP tool registration in `cmd/narrate-mcp`.
- Reuses persistent-sink wav-concat; does **not** emit `plan.json`/`manifest.json`
  for this tool's output (single-file contract).
- `output_path` accepting a file or a directory needs a documented resolution
  rule (settle at build).

## Related decisions

- [Rebuild as server-driven Earshot UI](2026-06-28-earshot-rebuild-server-driven-listener-ui.md) — companion decision, same feature.

## Revisit trigger

If a caller later needs the full plan/manifest alongside the wav, reconsider
whether `speak_to_file` should optionally emit the directory form.
