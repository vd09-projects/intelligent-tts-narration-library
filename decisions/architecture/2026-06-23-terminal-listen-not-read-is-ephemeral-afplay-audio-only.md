# Terminal "listen, not read" is the existing speak → ephemeral sink → afplay path (audio-only, no UI)

| Field    | Value            |
|----------|------------------|
| Date     | 2026-06-23       |
| Status   | accepted         |
| Category | architecture     |
| Tags     | mcp, speak, ephemeral-sink, afplay, listen-not-read, terminal, ticket-72, issue-73, v3-adr |

## Context

Ticket #72: v2→v3 correction of the ADR for "narrate a Claude Code response over MCP — listen, not read" (`.claude/handoff/narrate-claude-code-mcp-listen/planner-architecture.md`, v3, approved; review `review-findings-plan-v3.md`, APPROVE, 0 blocking).

A user in a Claude Code terminal receives a large assistant response and wants to **hear** it rather than read it. The v2 ADR routed this listen-mode through `cmd/narrate-server` producing a durable `sink/persistent` outDir served to the React browser player, and treated that as *the* listen path. A blocking user question during plan-review approval — "a terminal user just wants to hear it; why stand up a server and a browser tab?" — forced a verified code investigation.

The investigation (authoritative, file:line) showed the audio-out-of-speakers path already ships:
- The MCP `speak` tool wires the ephemeral sink (`cmd/narrate-mcp/main.go:237`, `ephemeral.New()`), which shells out to macOS `afplay` (`sink/ephemeral/ephemeral.go:200`; default binary `"afplay"` at `ephemeral.go:37`). It plays each per-block WAV, returns a receipt-only envelope (`main.go:182-186`), and deletes its temp dir (`main.go:287-293`).
- The MCP host (the Claude Code terminal) does not play audio — it only renders the receipt JSON as text. The `narrate-mcp` process plays the sound itself, before returning. This is the load-bearing fact v2 missed: the audio mechanism is the tool process + `afplay`, not the host and not a browser.
- `audio.wav` / per-block WAVs are standard 24 kHz mono PCM s16le — directly playable by `afplay` (`render/render.go:116-124`).

## Options considered

### Option A: Primary listen path = speak → ephemeral → afplay (audio-only, no UI) — CHOSEN
- **Pros**: Already exists and ships today; zero new playback machinery; matches the actual terminal user need (make my speakers say it); no server, no persistent outDir, no browser tab; failures surface as a normal MCP tool error / failed receipt; lower coupling.
- **Cons**: No on-screen block highlight / transport on this path (covered by the optional visual companion — see Decision B).

### Option B (v2): Listen path = narrate-server + persistent outDir + React browser player
- **Pros**: Gives on-screen highlight + transport for free.
- **Cons**: Over-engineers the terminal need — stands up a server and a browser tab for a user who only wants audio; nothing in Go auto-opens a browser (`cmd/narrate-server` only prints its URL to stderr, `main.go:385`; the player is hand-launched via `pnpm dev`), so it is not even automatic; higher coupling; introduces the two-surface friction and render-failure-spinner machinery as if mandatory.

## Decision

The **primary** "listen, not read" path is the existing `speak → sink/ephemeral → afplay` — audio out of the speakers, which already exists. No UI is required and none is built for this path. The host renders the receipt-only envelope as text; the audio already happened inside the `narrate-mcp` process before the receipt returned.

This supersedes the v2 approach that made the narrate-server + React player + durable outDir the listen path. That route is demoted to an optional, explicitly secondary visual companion (see Decision B). The two-surface friction (v2 Risk 1) and render-failure-spinner machinery (v2 blocker 2 / Risk 5) are re-scoped to apply only to that optional visual path; the terminal audio path has no spinner and no browser tab.

## Consequences

- The terminal audio path needs no sync surface at all — it just plays per-block WAVs in order. Block-level highlight is relevant only to the optional visual companion.
- Partial-playback (e.g. block 7 of 12 fails → user heard 1–6, call returns an error) is **defined behavior**, not a bug, for listen-not-read. Carried to #73.
- Errors on this path are synchronous Go errors returned up `runSpeak` (MkdirTemp failure, pipeline/render error with defer cleanup still firing, afplay failure under bounded `context.WithTimeout`). The error-vs-refusal boundary holds: error → no audio/no artifact; refusal → spoken data in a produced plan that still plays.
- On the primary path a refusal is **heard, not seen** (no player visual refusal display). #73 must confirm the spoken refusal notice is intelligible as audio on its own.

## Related decisions

- [React player is optional and reuses the existing player (#50)](2026-06-23-react-player-optional-reuses-existing-player-50.md) — the demoted visual companion path.
- [Primary listen path stays decoupled from any durable sink](../tradeoff/2026-06-23-primary-listen-path-decoupled-from-durable-sink.md) — the standing guardrail this composition rests on.
- [MCP `speak` response is a receipt-only envelope](../convention/2026-06-19-mcp-speak-response-receipt-only-envelope.md) — the v1 envelope decision this path is consistent with.

## Revisit trigger

If streaming / real-time narration is ever added (currently a phase-one non-goal), or if the host gains the ability to play audio from a tool return value, re-examine whether the in-process afplay path is still the right primary surface.
