# Earshot uses a parallel per-entry narration model with in-memory client-side dedup

| Field    | Value            |
|----------|------------------|
| Date     | 2026-06-29       |
| Status   | accepted         |
| Category | architecture     |
| Tags     | issue-111, earshot, state-management, parallel-narration, in-memory-cache, client-side-dedup, entries-map, issue-112, issue-126 |

## Context

The original #111 scaffold modeled narration state as a single shared `currentTranscript` in `useNarrationSession`: one narration at a time, and revisiting any message (or returning to a session after switching away) re-issued `POST /narrate` and re-rendered from scratch. User feedback during the build asked for three things: a loading indicator, a longer timeout, and the ability to load multiple chats in parallel — plus a follow-up question about whether already-encoded blocks are reused rather than re-encoded.

## Options considered

### Option A: single shared `currentTranscript` (original)
- **Pros**: simplest state; one transcript, one audio source.
- **Cons**: only one narration in flight at a time; every revisit (including session-switch-and-return) re-narrates and re-renders; no client-side reuse; cannot show per-row progress.

### Option B: per-entry `entries` Map (chosen)
- **Pros**: multiple narrations concurrent; each row/file shows its own loading/ready/error badge; an already-`ready` entry switches the transcript view instantly with zero re-narrate; the Map survives session/tab switches within one page lifetime, so navigate-away-and-back reuses prior renders.
- **Cons**: more state; keyed identity must be stable and collision-free across sessions (mitigated: server StableIDs — line uuid else `h:`-hash).

## Decision

`useNarrationSession` owns an `entries` Map keyed by a stable string — `message.id` for session messages, `file.name` for files — each value holding `{status: loading|ready|error, transcript, error}`. This replaces the single-`currentTranscript` model. Multiple narrations run concurrently; each session row and the file pane render their own status badge. Selecting an already-`ready` entry switches the transcript view **instantly with no re-narrate** — an in-memory, client-side dedup. The Map persists across session/tab switches for the page lifetime, so leaving a session and returning reuses prior renders. Each request is bounded by a 2-minute `AbortController` timeout (raised from 30 s after a large-block timeout report).

REJECTED: the single shared `currentTranscript` model (one narration at a time; re-narrates and re-renders on every revisit).

## Consequences

- The "switch away from a session and come back" case the user raised is deduped client-side — no second encode within one page session.
- **Limitation (parked, not solved):** the cache is in-memory only — it is lost on page reload, and there is NO persistent or server-side block-audio cache. The narrate-server itself re-renders every `POST /narrate` (no content-hash audio reuse). True persistent audio caching is explicitly deferred (user accepted this).
- Entry identity depends on stable keys; cross-session `message.id` collisions would alias entries (relies on server StableIDs being unique).
- #112 (resume persistence) will need a `localStorage` layer keyed by session+message / file — a persistent superset of this in-memory key, and should build on this entry model rather than replace it.

## Related decisions

- [Earshot narrate-server contract pinned in types.ts; audio_url opaque](2026-06-28-earshot-narrate-contract-pinned-audio-url-opaque.md) — the contract these entries consume.
- [Earshot file input is multipart upload, not {path} JSON](2026-06-28-earshot-file-input-multipart-upload-not-path-json.md) — the file-keyed entry path.
- Content-hash escalation cache for /narrate/block (#126, dormant) — the server-side counterpart to this client-side dedup; would make reuse survive reload/restart.

## Revisit trigger

Revisit when #112 (resume persistence) lands — fold this in-memory Map into a localStorage-backed store keyed by session+message / file. Revisit the in-memory-only limitation if repeat re-render cost becomes observable (the #126 server-side content-hash cache trigger).
