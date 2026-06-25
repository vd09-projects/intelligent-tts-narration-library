# Channel-2 live observer mechanism: append-only JSONL + tail -f, not MCP notifications/progress

| Field    | Value            |
|----------|------------------|
| Date     | 2026-06-26       |
| Status   | accepted         |
| Category | architecture     |
| Tags     | observability, channel-2, jsonl, tail, mcp-progress, notifyprogress, decoupled-observer, narrate-observe, issue-81, issue-77 |

## Context

Issue #81 implements ADR #77 (D5) **Channel 2**: a user-launched, read-only observer
that shows LIVE during-playback progress in a second terminal — the surface the
synchronous Channel-1 MCP receipt structurally cannot provide (the `speak` handler
blocks on afplay per block via `sink/ephemeral.Consume`, so its inline receipt is
assembled only after the last block finishes).

Two mechanisms were on the table from the ADR: append-only JSONL the handler writes
one line per block to (tailed by `cmd/narrate-observe`), versus native MCP
`notifications/progress` (`Session.NotifyProgress`, go-sdk v1.5.0) emitted from inside
the blocking handler.

## Options considered

### Option A: append-only JSONL + `tail -f` (chosen)
- **Pros**: surfaces to a USER in a 2nd terminal (the exact D5 requirement); zero deps;
  ephemeral `/tmp` scratch file with no durable-sink coupling; live (emitted before each
  blocking `play()`); decoupled and user-launched.
- **Cons**: a side file on disk (mitigated: 0600, OS reaps `/tmp`); a second binary to run.

### Option B: native MCP `notifications/progress` (NotifyProgress)
- **Pros**: lighter; no side file; protocol-native.
- **Cons**: surfaces to the MCP CLIENT, not a human terminal; invisible without client
  `progressToken` cooperation + UI; not user-launchable on demand. Wrong audience for D5.

## Decision

Append-only JSONL + `tail -f` is the **default** Channel-2 mechanism for #81. It is the
only option that meets the D5 requirement of a user-facing, second-terminal live view
with zero dependencies and no durable coupling. NotifyProgress is deferred, not rejected:
when a real MCP-client UI later wants progress, add it **additively** alongside JSONL
(same `BlockEvent` shape), never as a replacement. The `schema`/`v` discriminator on the
wire line is the cheap insurance that keeps that addition compatible.

## Consequences

- A new `cmd/narrate-observe` binary + a JSONL wire contract to maintain (duplicated,
  deliberately, between writer and reader — "the wire is the contract").
- Opt-in via env (`NARRATE_OBSERVE_FILE` > `NARRATE_OBSERVE` truthy > off); off keeps the
  speak response byte-identical.
- No source/spoken text on the wire (secret-leak avoidance); 0600 scratch file.

## Related decisions

- [ADR: Playback observability & control model (issue #77)](2026-06-24-playback-observability-control-model-issue-77.md) — this is the build of that ADR's D5 Channel 2.
- [Observer seam placement: sink reads the plan param](2026-06-26-observer-seam-sink-reads-plan-param.md) — sibling decision from the same build.

## Revisit trigger

When a real MCP-client UI wants progress, or a non-macOS playback backend lands — then
spike NotifyProgress as an additive second emitter sharing the `BlockEvent` shape.
