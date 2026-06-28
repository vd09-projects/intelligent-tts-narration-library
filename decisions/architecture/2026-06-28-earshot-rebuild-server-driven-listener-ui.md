# Rebuild the listener as a server-driven UI (Earshot); delete the passive player/

| Field    | Value            |
|----------|------------------|
| Date     | 2026-06-28       |
| Status   | accepted         |
| Category | architecture     |
| Tags     | earshot, player, listener-ui, narrate-server, http-bridge, rebuild, leveling-ui, playback |

## Context

The reference player (`player/`) is a passive, fixture-driven React preview: it
loads a fixed `plan.json`+`manifest.json`+`audio.wav` triple, highlights blocks
at native-`<audio>` granularity, and offers escalate via copy-CLI or an HTTP
patch seam. It has no session loading, no play/pause/seek beyond the browser
default, and no resume-position persistence. The user's real use cases are:
(1) listen to a chat session's messages, (2) read out a big file, (3) text → wav
via MCP. The player solves none of (1)/(2) and the user reports it as
"static/boring" and a source of more bugs than value.

## Options considered

### Option A: Extend the existing player/
- **Pros**: reuse fixture loader, escalate client, source pane.
- **Cons**: carries fixture/companion/sourcepane machinery irrelevant to the use
  cases; the passive preview model cannot drive session loading + live playback;
  inherits its bug surface.

### Option B: Rebuild as a server-driven UI (Earshot) + new local HTTP server
- **Pros**: UI becomes the driver for use cases 1+2; a small local Go server
  (`narrate-server`) runs the pipeline (browser can't run Kokoro) and serves
  audio; clean slate drops dead machinery; reuses planner/leveling/render core.
- **Cons**: one genuinely new backend piece (the server); more up-front build.

## Decision

Chose **Option B**. Build **Earshot** (new `earshot/` web app) backed by a new
local `narrate-server` HTTP bridge; delete `player/` and its fixture / escalate-
CLI-card / source-code-pane / companion-mode code. The narration core (planner,
plan schema, per-block leveling + escalation/patch, render, ephemeral+persistent
paths) is reused unchanged. Rationale: the use cases need a driver UI with
session loading and live playback (play/pause/seek + resume), which a passive
preview architecturally cannot provide; rebuilding is cheaper than retrofitting
and sheds the player's bug-prone surface.

## Consequences

- New component: `narrate-server` (composition-root code; planner stays I/O-free).
- No new plan-schema fork — same narration plan internally.
- `player/` history removed; its escalate-HTTP seam concepts feed the new server.
- Render-id wav lifecycle (temp-dir GC) becomes a server concern to settle.

## Related decisions

- [Earshot session source via local transcript glob](2026-06-28-earshot-session-id-via-local-transcript-glob.md) — companion decision, same feature.
- [speak_to_file as a separate MCP tool](2026-06-28-speak-to-file-separate-mcp-tool.md) — companion decision, same feature.

## Revisit trigger

If a hosted/multi-user deployment is ever wanted (contradicts the local-only,
secrets-read-aloud assumption), the server-on-localhost model must be revisited.
