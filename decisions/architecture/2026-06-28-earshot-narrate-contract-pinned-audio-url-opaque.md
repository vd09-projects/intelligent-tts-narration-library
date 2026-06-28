# Earshot pins the narrate-server contract in types.ts and treats audio_url as an opaque URL

| Field    | Value            |
|----------|------------------|
| Date     | 2026-06-28       |
| Status   | accepted         |
| Category | architecture     |
| Tags     | issue-111, earshot, narrate-server, http-bridge, contract-isolation, audio-url-opaque, message-ref, types-ts, divergence-point, issue-109 |

## Context

Earshot (#111) consumes the `narrate-server` (#109) HTTP bridge: `GET /sessions/{id}/messages`, `POST /narrate`, `POST /narrate/file`, each returning `{audio_url, blocks[], timeline}`. The client was built against committed fixtures before #109 was live, so the contract had to be pinned somewhere stable, and the response's `audio_url` had to be either consumed as-is or decomposed into a `render_id` to rebuild the audio path client-side.

## Options considered

### Option A: extract a `render_id` and reconstruct `GET /audio/{render_id}.wav`
- **Pros**: gives the client an explicit handle to the render.
- **Cons**: bakes the server's audio path scheme into the client; more assumptions; breaks the moment the server changes its URL scheme; pure downside since the client never needs to address the audio by id.

### Option B: treat `audio_url` as an opaque URL used verbatim
- **Pros**: a server audio-path format change cannot break Earshot; the client just feeds the string to `<audio src>`; minimal coupling.
- **Cons**: the client cannot reason about the render id (it never needs to in the shell).

## Decision

All #109/plan shape knowledge lives in `earshot/src/api/types.ts`; components import view models and never re-derive a shape. `audio_url` is treated as an **opaque URL used verbatim** — fed directly to `<audio src>` with no `render_id` parsing and no `/audio/{id}.wav` reconstruction (D4). The assumed `message_ref` shape is the **single, clearly-commented mock↔live divergence point** in `types.ts`: both the fixture and the mock reference it, so reconciling mock vs live when #109 confirms the real shape is a one-line change rather than a scattered refactor. This was settled at the human gate and verified in the build review (contract isolation called "textbook").

## Consequences

- A server-side change to the audio URL scheme is invisible to the client by construction.
- Response-shape drift from the documented §5 contract is contained: all shape knowledge is isolated in `types.ts`, and `message_ref` is the only place a live-contract mismatch can require a change.
- Open string-unions (`| (string & {})`) implement the schema-versioning rule so unknown enum values round-trip and render via a neutral fallback.
- Known limitation parked to `/verify`: today's narrate path sends assembled `text`, not `message_ref`, so returned `block.id`s are per-render, not per-message — confirm acceptable when `message_ref` lands live.

## Related decisions

- [Earshot file input is multipart upload, not {path} JSON](2026-06-28-earshot-file-input-multipart-upload-not-path-json.md) — sibling #111 contract decision.
- [GET /sessions/{id}/messages structural-split-only, not planner.Plan](2026-06-28-sessions-messages-structural-split-not-planner.md) — the #109 server side of this contract.

## Revisit trigger

Revisit at the live `/verify` once #109 is runnable: confirm the real `message_ref` shape (re-pin the one divergence point), and confirm per-render block identity is acceptable. Revisit the opaque-`audio_url` stance only if the client ever needs to address the render by id.
