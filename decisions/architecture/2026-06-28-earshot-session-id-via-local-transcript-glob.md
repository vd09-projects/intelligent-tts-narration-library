# Resolve a session ID to a local transcript file by glob — no cloud API

| Field    | Value            |
|----------|------------------|
| Date     | 2026-06-28       |
| Status   | accepted         |
| Category | architecture     |
| Tags     | earshot, session-id, transcript-jsonl, claude-projects, glob, message-list, speak-last, no-cloud-api, no-auth |

## Context

Earshot's session pane needs the full chat history for a given session so the
user can click a message (or a chunk of a big message) and hear it. The user
initially framed this as "give a cloud session ID," which suggested a claude.ai
web API integration with auth. On clarification, the user is fine resolving the
ID against the **local** Claude Code transcripts.

## Options considered

### Option A: claude.ai cloud session API
- **Pros**: works for sessions not present on the local machine.
- **Cons**: unknown/unstable API surface, auth + token handling, network
  dependency, scope creep — none of it needed for the actual workflow.

### Option B: Resolve session ID → local transcript file by glob
- **Pros**: the transcript filename **is** the session UUID
  (`~/.claude/projects/<project-hash>/<session-id>.jsonl`), so the ID maps to a
  file by glob; the parser already exists in `speak_last`'s `lastAssistantText`
  (16 MiB line buffer, `tool_use` vs `text` handling); zero auth, zero network.
- **Cons**: only covers sessions whose transcripts exist locally.

## Decision

Chose **Option B**. Resolve a session ID to its `.jsonl` by glob and parse it
locally; do **not** build a claude.ai cloud integration. Generalize
`speak_last`'s parser into a shared function returning the **full ordered message
list** (user + assistant turns) instead of only the last assistant turn;
`speak_last` keeps calling it for "last assistant," `narrate-server` calls it for
"all messages." Big messages are chunked into blocks via the planner's existing
oversized-block splitting (clean structural seams only). Rationale: the parser
already exists, the filename-is-UUID mapping is exact, and it removes all
auth/API unknowns.

## Consequences

- New shared transcript parser (used by both `speak_last` and `narrate-server`);
  `speak_last`'s own behaviour unchanged.
- Sessions without a local transcript are out of reach (acceptable for the
  local-only hobby scope).

## Related decisions

- [Rebuild as server-driven Earshot UI](2026-06-28-earshot-rebuild-server-driven-listener-ui.md) — companion decision, same feature.

## Revisit trigger

If the user needs to listen to sessions that never touched this machine, the
cloud-API option (rejected here) must be reconsidered.
