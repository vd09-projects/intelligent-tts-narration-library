# Render-failure signal is a server/player readiness collaboration (on-disk truth + client-owned give-up bound), not a render-side sentinel

| Field    | Value            |
|----------|------------------|
| Date     | 2026-06-26       |
| Status   | accepted         |
| Category | architecture     |
| Tags     | architecture, http-api, react, readiness, mcp, ttsplayer, issue-76, render-failed, no-out-dir |

## Context

Ticket #76, Part B — the optional React visual companion for MCP listen-mode. The companion needs to know whether a rendered narration directory is ready to play, still rendering, or will never appear, and must never leave the user staring at a perpetual spinner — including the case where the backing server is dead from the very first poll.

The narration companion artifacts are produced out-of-band by a separate `cmd/narrate --sink persistent` invocation (the standing decoupling order; Risk 6 / AC6). The `speak` MCP tool keeps its ephemeral play-then-delete lifetime and must not be teed for this purpose. So the player cannot observe render progress through the MCP call itself — it has to discover readiness from the filesystem via the server.

The open question: where does the "render failed / never completes" signal live? On the render/sink side as an artifact, or as an emergent property the player concludes on its own?

## Options considered

### Option A: Reuse the existing `source_not_found` reason token for the dir-absent case (maximal reuse)
- **Pros**: No new enum token; one fewer machine-stable string to document.
- **Cons**: AC3 names "no-outDir" as a distinct outcome; the append-only reason enum exists precisely so a clear, machine-stable token can be added rather than overloading an existing one. `source_not_found` is scoped to the escalate / artifact source-resolution path and overloading it muddies both call sites. REJECTED.

### Option B: A render-side failure SENTINEL FILE written by the sink
- **Pros**: An explicit terminal signal on disk; player logic is trivial (poll for sentinel).
- **Cons**: Changes the sink contract and adds a new artifact the sink must be trusted to write even mid-crash — which a crashed render cannot guarantee. The server should report on-disk truth only. REJECTED.

### Option C (chosen): Server/player collaboration — server reports on-disk truth, player owns a bounded give-up
- **Pros**: Sink contract unchanged; server stays a pure on-disk-truth reporter; "never completes" and "server gone" are detected by the client where the user-facing timeout policy belongs.
- **Cons**: The give-up bound lives in the client, so the terminal `render_failed` phase is a player-side conclusion, not a server fact — acceptable given the server genuinely cannot distinguish "slow" from "dead render" from disk alone.

## Decision

Model the render-failure signal as a **server/player collaboration** built on on-disk truth plus a client-owned give-up bound — not a render-side sentinel.

**Server side (`cmd/narrate-server`):** expose `GET /readiness?dir=` returning on-disk truth as a tri-state:
- `200 {status: rendered}` — `plan.json` + `manifest.json` + `audio.wav` all present and non-empty.
- `200 {status: rendering}` — dir exists but the triple is incomplete.
- `404` with a NEW closed-enum reason token `no_out_dir` — the dir is absent.

`no_out_dir` is **appended** to the server's closed, append-only reason enum (sibling to the existing `source_not_found`), honoring additive / append-only schema discipline. `source_not_found` stays scoped to the escalate / artifact source-resolution path and is not overloaded for the dir-absent case.

**Player side:** `useRenderReadiness` owns the bounded give-up — polls at 1 Hz with `MAX_POLL_ATTEMPTS = 120` absolute cap and an `UNREACHABLE_FASTFAIL = 8` consecutive-transport-reject dead-server fast path. Both "never completes" and "server gone" collapse into a single DEFINED terminal phase `render_failed`. Guarantee: zero perpetual spinners, even when the server is dead from poll #1.

**Correctness dependency:** the `rendered` guarantee relies on the persistent sink publishing each leaf via atomic `tmp` + `os.Rename`. Without atomic publish, the non-empty (`size > 0`) check could read a truncated, still-streaming file and falsely report `rendered`.

## Consequences

- Terminal `render_failed` is a client-side conclusion, not a server-reported fact — the server cannot tell "slow" from "dead" from disk alone, and this is by design.
- The sink contract is untouched; no new sink artifact and no `speak` tee. The companion's artifacts continue to come from a separate `cmd/narrate --sink persistent` invocation, and `speak`'s ephemeral play-then-delete lifetime stays intact.
- The append-only reason enum gains `no_out_dir`; consumers must treat the enum as additive and ignore unknown tokens.
- The `rendered` guarantee is only as sound as the sink's atomic-publish behavior — a regression to non-atomic writes would silently weaken it.

## Related decisions

<!-- - [Title](../category/YYYY-MM-DD-slug.md) — brief note on relationship -->

## Revisit trigger

Revisit if the server ever gains a real render-progress signal (e.g. a render process reporting liveness), which could let `render_failed` become a server fact rather than a player-side timeout conclusion; or if a non-atomic sink publish path is introduced, which would invalidate the `size > 0` readiness check.
