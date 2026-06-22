# React player is optional and reuses the existing player (#50) — no new UI

| Field    | Value            |
|----------|------------------|
| Date     | 2026-06-23       |
| Status   | accepted         |
| Category | architecture     |
| Tags     | react-player, visual-companion, narrate-server, issue-50, opt-in, reuse, ticket-72, issue-73, v3-adr |

## Context

Ticket #72, v3 ADR correction. With the primary terminal "listen" path settled as audio-only `speak → ephemeral → afplay` (see related decision), the question remained: what serves the *second, optional* need of a user who also wants to **watch** — an on-screen section highlighting the current block, a pause/play transport, and a visible loading indicator?

The system already has this surface: the existing React reference player (`player/`) fed by a `sink/persistent` outDir served by `cmd/narrate-server` (block highlight + escalate + spinner from issue #50, live-dir serving from #62). The verified investigation confirmed nothing in Go auto-opens a browser — `cmd/narrate-server` only prints its URL to stderr (`main.go:385`) and the player is launched manually via `pnpm dev` (`Makefile:103`, `player/README.md`).

## Options considered

### Option A: Reuse the existing player (#50) as an optional, opt-in visual companion — CHOSEN
- **Pros**: No new UI to build; reuses already-shipped block highlight + transport + escalate + spinner; clearly separated from the primary terminal flow; honest AC6 reuse audit (`player/`, `narrate-server`, `sink/persistent` fenced as optional).
- **Cons**: Requires a separate, hand-launched invocation; the visual path carries the two-surface friction and render-failure-spinner machinery (re-scoped to apply only here).

### Option B: Build a new UI for the visual companion
- **Pros**: Could be tailored to listen-mode.
- **Cons**: Pure rebuild of a surface that already exists; contradicts reuse-over-rebuild; no driver justifies it.

## Decision

Optional visual sync reuses the **existing** React reference player from issue #50. No new UI is built. The visual companion is a separate, opt-in path — not part of the primary terminal flow and not auto-opened. A user who wants on-screen block highlight + transport + escalation launches the player + `cmd/narrate-server` manually against a durable persistent outDir; the terminal-audio user never touches it.

## Consequences

- The render-failure-spinner machinery and two-surface friction (v2 Risk 1 / blocker 2 / Risk 5) are re-scoped to apply **only** to this optional visual path. The terminal audio path has no spinner and no browser tab.
- The visual companion needs a durable persistent outDir, which is reached by a separate `--sink persistent` invocation — see the decoupling guardrail decision; this is what keeps the optional path from re-coupling onto the primary `speak` call.
- Minor naming drift to pin on #73: the Makefile target vs the underlying `pnpm dev` (`Makefile:103`) — carry the exact target name onto #73.

## Related decisions

- [Terminal "listen, not read" is the existing speak → ephemeral → afplay path](2026-06-23-terminal-listen-not-read-is-ephemeral-afplay-audio-only.md) — the primary path this companion sits beside.
- [Primary listen path stays decoupled from any durable sink](../tradeoff/2026-06-23-primary-listen-path-decoupled-from-durable-sink.md) — how the opt-in is reached without coupling.

## Revisit trigger

If a terminal-native visual surface (in-terminal highlight without a browser) becomes feasible, or if the visual companion needs to ship as a default rather than opt-in, revisit whether reusing the browser player is still the right answer.
