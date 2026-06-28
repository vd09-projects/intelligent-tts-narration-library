# Expose render_id as an additive narrateResponse field, not parsed out of audio_url

| Field    | Value            |
|----------|------------------|
| Date     | 2026-06-29       |
| Status   | accepted         |
| Category | architecture     |
| Tags     | issue-113, earshot, narrate-server, render-id, audio-url-opaque, additive-field, types-ts, escalation, api-contract |

## Context

Issue #113 makes Earshot the first client consumer of the already-shipped `POST /narrate/block` escalation endpoint (#110). To escalate a block, the client must send the `render_id` of the prior `/narrate` render. But the established Earshot contract (`2026-06-28-earshot-narrate-contract-pinned-audio-url-opaque`, decision D4) treats `audio_url` as an **opaque** URL fed verbatim to `<audio src>` — the client never parses a `render_id` out of it or reconstructs `/audio/{id}.wav`. The escalation client needs the render id; the contract forbids extracting it from the only field that currently carries it.

## Options considered

### Option A: Parse render_id out of audio_url
- **Pros**: no server change.
- **Cons**: directly violates D4 (audio_url opaque); bakes the server's `/audio/{render_id}.wav` URL scheme into the client; breaks on any URL-scheme change. Pure downside.

### Option B: Add render_id as its own additive field on narrateResponse
- **Pros**: preserves D4 — audio_url stays opaque; render id is a first-class field; additive within the major `schema_version` so any other `/narrate` HTTP client ignores it safely.
- **Cons**: one tiny server change (`narrate.go`) plus its `types.ts` mirror.

## Decision

Add `RenderID string` (`json:"render_id"`) to the server's `narrateResponse` in `cmd/narrate-server/narrate.go`, populated from the render id already computed for the audio URL, and mirror it as `render_id: string` on `NarrateResponse` in `earshot/src/api/types.ts`. The escalation client reads `entry.transcript.render_id` directly and never touches `audio_url`. The field is additive/ignorable; an older response lacking `render_id` simply disables escalate gracefully (`LevelControl` disabled). This preserves the audio-url-opaque invariant (D4) while giving the escalation path the render id it needs.

## Consequences

- `narrateResponse` JSON gains a field consumed by Earshot and any other `/narrate` client — must stay additive/ignorable forever within the major schema version.
- The client now has two reasons to hold a render: playing audio (opaque url) and escalating a block (render_id) — cleanly separated, neither leaking into the other.

## Related decisions

- [Earshot pins the narrate-server contract and treats audio_url as opaque](2026-06-28-earshot-narrate-contract-pinned-audio-url-opaque.md) — this decision preserves that D4 invariant by adding a field instead of parsing the URL.
- [POST /narrate/block escalation persists a 3-file dir per render_id](2026-06-28-post-narrate-block-escalation-persists-a-3-file.md) — the endpoint that consumes render_id.
