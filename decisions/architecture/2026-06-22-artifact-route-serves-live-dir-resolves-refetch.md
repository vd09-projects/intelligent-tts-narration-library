# Read-only GET /artifact route serves the live escalated dir; player re-fetch resolves against it

| Field    | Value            |
|----------|------------------|
| Date     | 2026-06-22       |
| Status   | accepted         |
| Category | architecture     |
| Tags     | player, escalate, server-mode, artifact-route, live-dir, path-traversal, allowlist, EvalSymlinks, loopback, CORS, issue-62 |

## Context

Issue #62 (server-escalate-livedir). The prior phase-one limitation (recorded as `revisit-later`) was that after an escalate patch, the player's `repointAudio` / `reloadManifest` re-fetched the changed outputs against `FIXTURE_BASE` — the bundled fixture serving root — rather than the arbitrary absolute server directory the user supplied in the TopBar field. Post-patch re-fetch was therefore correct only when the served dir happened to coincide with `FIXTURE_BASE`. Fixing it properly requires a server-contract change so the player can fetch a patched block's outputs back from an arbitrary served directory.

## Options considered

### Option A: Add a read-only artifact-serving route + resolve re-fetch against the live dir
- **Pros**: Fully correct server-mode re-fetch for any dir. Closes the documented phase-one gap. Reuses the existing loopback bind + pinned CORS, so no new exposure surface. Containment is structural (allowlist + symlink-resolved boundary check), not string-prefix guesswork.
- **Cons**: Adds a route and a path-containment surface that must be defended against traversal.

### Option B: Keep FIXTURE_BASE-relative re-fetch (the deferred status quo)
- **Pros**: No new route, no new attack surface.
- **Cons**: Leaves the known correctness gap open; arbitrary server dirs still re-fetch from the wrong root.

## Decision

Chose Option A. #62 added a read-only `GET /artifact?dir=&name=` route to `cmd/narrate-server` that statically serves only `{manifest.json, audio.wav}` from the escalated directory. Containment is enforced by allowlist-before-join (the `name` must be one of the two permitted filenames before any path is constructed), then `filepath.EvalSymlinks` plus a `filepath.Rel` boundary-containment check — deliberately NOT a raw string-prefix comparison, which is defeatable by symlinks and sibling-prefix dirs. The route rides the existing loopback bind and the already-pinned CORS policy, so no new network exposure is introduced. On the client, the player resolves its re-fetch base against the live dir via a pure resolver that reads effect-synced refs at call time, so `repointAudio` / `reloadManifest` now point at the user-supplied directory instead of `FIXTURE_BASE`.

## Consequences

- Server-mode re-fetch is now correct for any absolute dir, not just `FIXTURE_BASE`. The prior `revisit-later` limitation is resolved and superseded.
- A new path-containment surface exists; correctness depends on the allowlist + `EvalSymlinks` + `Rel` containment holding (string-prefix containment is explicitly rejected).
- The route is read-only and scoped to two filenames; it does not widen what the server can write.
- Local-only / loopback-bound + pinned CORS remains the trust boundary; awareness that local artifacts (including any secrets in them) are served back to the same machine.

## Related decisions

- [Server-mode re-fetch resolves against FIXTURE_BASE (phase-one limitation)](../tradeoff/2026-06-22-server-mode-refetch-resolves-against-fixture-base.md) — the limitation this decision resolves and supersedes.
- [Manual absolute-path dir field in TopBar is the server-mode escalate enabler](2026-06-22-topbar-manual-absolute-dir-field-enables-server-escalate.md) — the arbitrary dir this route now serves re-fetches from.
- [Read-side per-dir mutex in /artifact keyed on filepath.Abs(dir)](../concurrency/2026-06-22-artifact-read-side-per-dir-mutex.md) — guards the triple this route serves against torn cross-file reads.

## Revisit trigger

If the served-back artifact set ever needs to grow beyond `{manifest.json, audio.wav}`, or if the server stops being loopback-only — at that point the allowlist and CORS/bind assumptions must be re-examined.
