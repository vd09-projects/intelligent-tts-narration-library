# resolveOutputPath rule for speak_to_file output_path (file vs directory)

| Field    | Value            |
|----------|------------------|
| Date     | 2026-06-28       |
| Status   | accepted         |
| Category | architecture     |
| Tags     | speak-to-file, output-path, path-resolution, file-vs-dir, resolveOutputPath |

## Context

`speak_to_file`'s `output_path` arg may name either a directory (write a derived
filename into it) or a full file path. The separate-tool decision left
"`output_path` file-vs-dir resolution rule" as an explicit **settle-at-build**
open item. This decision resolves it.

## Decision

`resolveOutputPath` disambiguates without a separate flag:

- If `output_path` denotes a **directory** — a trailing separator, `"."` or
  `".."`, or an already-existing directory — write a **derived filename** into
  that directory.
- Otherwise treat it as a **file path**, appending `.wav` (case-insensitive
  check, so no double extension) if the extension is missing.
- `MkdirAll` the parent directory.
- The result is absolutized and `Clean`ed.

Rationale: callers may reasonably pass either a directory or a full file path;
this rule disambiguates the two from the value itself rather than requiring a
separate `is_dir`-style flag.

## Consequences

- Directory inputs get a derived filename; file inputs are honored verbatim
  (with `.wav` ensured).
- A case-insensitive extension check avoids `foo.WAV.wav` double extensions.
- Parent directories are created on demand; the returned path is always
  absolute and cleaned.

## Related decisions

- [Ship speak_to_file as a separate MCP tool](2026-06-28-speak-to-file-separate-mcp-tool.md) — names this as the settle-at-build open item this decision closes.
- [Uniform speakToFileResponse envelope](2026-06-28-speak-to-file-uniform-response-envelope.md) — the resolved path is what populates `output_path` on the path branch.
