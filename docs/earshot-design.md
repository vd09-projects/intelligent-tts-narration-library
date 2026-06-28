# Earshot — Listener UI + MCP Design Spec

Status: **draft / pre-build**. Supersedes the `player/` reference player.
Owner: vikrantdhawan. Date: 2026-06-28.

This is an additive design doc. It does not modify `problem-statement.md` or
`docs/solution-phase-design.md`. It records the shape agreed for the listener
front-end and the MCP tool surface. Build happens in a separate session.

---

## 1. Goal

Make the library usable for three real listening use cases without forcing the
terminal. The narration core (planner, leveling, render, honesty/refusal) is
kept. The passive, fixture-driven `player/` is deleted and replaced by **Earshot**,
a driver UI backed by a small local HTTP server.

## 2. Use cases

1. **Listen to a chat session.** Give a session ID. Earshot loads the full chat
   history from local transcripts, shows the messages, and lets the user click a
   message (or a chunk of a big message) to hear it. Play / pause / seek / resume.
2. **Read out a big file.** Drop a file in Earshot; it is read aloud. Same
   playback controls. Optional save-to-disk.
3. **Text → audio file via MCP.** A Claude client calls a tool with text and an
   output path; one `.wav` lands at that path. No path → speak it out.

## 3. MCP tool surface (3 tools)

| Tool | State | Behaviour |
|---|---|---|
| `speak` | exists, unchanged | text/source → render → play (ephemeral). |
| `speak_to_file` | **new** | `{text \| source, output_path?, level, gender}`. Input is inline text **or** a path to a text/`.md` file. With `output_path` (file or directory): render to a single `.wav`, write it there, return the path. Without: fall back to speak (play). **Priority tool — built first.** |
| `speak_last` | exists, **generalized** | today reads newest local `.jsonl` and speaks the last assistant turn. Its transcript parser is generalized (see §6) to also feed Earshot's message list. The tool's own behaviour is unchanged. |

Decision: `speak_to_file` is a **separate tool**, not an `output_path` option on
`speak`. Keeps each tool's contract single-purpose.

Note: today the persistent sink writes a **3-file directory**
(`audio.wav` + `plan.json` + `manifest.json`). `speak_to_file` needs a
**single-file** output — render → concat to one wav → copy to `output_path`.
The persistent sink's wav-concat math is reused; the JSON sidecars are not
written for this tool.

## 4. Earshot web app (replaces `player/`)

New directory `earshot/`. Built from scratch. The old `player/` is deleted
along with its fixture/escalate-CLI-card/sourcepane/companion machinery — it
solves problems Earshot does not have and carries bugs we do not want.

Panes:
- **Session pane** — input a session ID → message list (chat history) → click a
  message → hear it. Big message is chunked into blocks; user can pick a block.
- **File pane** — drop / pick a file → read out.
- **Playback** — play, pause, seek, and **resume position** across reloads
  (client-side `localStorage`, keyed by session+message / file). This is the
  "persistence" the user means for listening — distinct from save-to-disk.
- **Leveling** — every block shows L1 / L2 / L3 escalate. Clicking escalate
  re-renders just that block at the higher level (reuses the existing per-block
  patch path) and swaps that block's audio in place.

Design intent: not the static grid. The look is decided during build with the
`frontend-design` skill; this spec only fixes structure and behaviour.

## 5. `narrate-server` (new local HTTP bridge)

The browser cannot run Kokoro. Earshot talks to a small local Go HTTP server
that drives the existing pipeline and serves audio. This is the one genuinely
new backend piece; everything hangs off it.

Proposed endpoints (contract, refine at build):

| Method + path | Purpose |
|---|---|
| `GET /sessions/{id}/messages` | Glob `~/.claude/projects/*/{id}.jsonl`, parse, return ordered messages `[{turn, role, text, blocks[]}]`. Big text pre-chunked into blocks. |
| `POST /narrate` | `{text \| message_ref, level, gender}` → render → `{audio_url, blocks[], timeline}`. |
| `POST /narrate/block` | `{render_id, block_id, level}` → re-render one block (escalation) → patched block audio + timing. |
| `POST /narrate/file` | `{path}` (or upload) → render a file → same shape as `/narrate`. |
| `GET /audio/{render_id}.wav` | Serve the rendered wav (blob source for `<audio>`). |

The server is composition-root code (knows concrete adapters/sinks). The
planner stays I/O-free. No new schema fork — same narration plan internally.

## 6. Session-ID → message list (key reuse)

The filename of a transcript **is** the session UUID:
`~/.claude/projects/<project-hash>/<session-id>.jsonl`. So a session ID maps to
a file by glob — no cloud API, no auth.

`speak_last`'s `lastAssistantText` already parses these `.jsonl` files (handles
the 16 MiB line buffer, `tool_use` vs `text` blocks, the self-invocation skip).
Generalize it into a shared parser that returns the **full ordered message list**
(user + assistant turns), not just the last assistant turn. `speak_last` keeps
calling it for "last assistant"; the server calls it for "all messages".

Chunking a big message into blocks reuses the planner's existing
oversized-block splitting (clean structural seams only — never arbitrary cuts).

## 7. Reuse / delete / new

- **Reuse:** planner, plan schema, per-block leveling + escalation/patch, render,
  ephemeral + persistent (wav-concat) paths, transcript `.jsonl` parser.
- **Delete:** `player/` entirely (fixture loader, escalate-CLI card, source-code
  pane, companion mode, escalate HTTP client).
- **New:** `narrate-server`, `speak_to_file` MCP tool, `earshot/` web app,
  the generalized transcript parser (shared by tool + server).

## 8. Open risks / to settle at build

- Render lifecycle: how long server keeps a `render_id`'s wav (temp dir GC).
- Concurrent sessions: two `.jsonl` with same mtime — pin by exact session ID
  (the glob already disambiguates; mtime tiebreak not needed here).
- Secret-read-aloud: local-only means a secret in a config block could be
  spoken. Awareness only, per CLAUDE.md — not a phase-one design driver.
- Resume granularity: store playback position per message-block, not word-level
  (word-level timing is a forbidden invariant).

## 9. Decisions to record

**Decision (Earshot v1) — architecture: accepted.** Rebuild the listener as a
server-driven UI (`earshot/` + `narrate-server`); delete the passive `player/`.
Rationale: player was fixture-driven and bug-prone; the real use cases need a
driver UI with session loading and live playback, which a passive preview cannot
provide.

**Decision (Earshot v1) — session source: accepted.** Resolve session ID to a
local transcript file by glob; do not build a claude.ai cloud API integration.
Rationale: the `.jsonl` filename is the session UUID, the parser already exists,
and it avoids auth/API unknowns entirely.

**Decision (Earshot v1) — tool split: accepted.** Ship `speak_to_file` as a
separate MCP tool rather than extending `speak`. Rationale: single-purpose tool
contracts; `speak` stays play-only.
