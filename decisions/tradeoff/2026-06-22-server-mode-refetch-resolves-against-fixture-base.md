# Server-mode re-fetch resolves against FIXTURE_BASE, not an arbitrary server dir (phase-one limitation)

| Field    | Value            |
|----------|------------------|
| Date     | 2026-06-22       |
| Status   | revisit-later    |
| Category | tradeoff         |
| Tags     | player, escalate, server-mode, repointAudio, reloadManifest, fixture-base, phase-one-limitation, deferred, issue-50 |

## Context

Task #50, React player server mode. After a successful escalate patch, the player re-fetches the changed outputs via `repointAudio` (the audio blob) and `reloadManifest` (the offsets). These re-fetch helpers resolve their URLs against `FIXTURE_BASE` — the player's bundled fixture serving root — rather than against the arbitrary absolute server directory the user supplied in the TopBar field.

This means server-mode re-fetch after a patch works correctly only when the server dir coincides with FIXTURE_BASE; for an arbitrary server dir the re-fetch points at the wrong root. Fixing it properly requires changing the server contract so the player can fetch a patched block's outputs back from an arbitrary served directory.

## Options considered

### Option A: Accept the FIXTURE_BASE-relative re-fetch as a known phase-one limitation
- **Pros**: Ships the escalate feature now. The escalate POST itself works against the arbitrary dir; only the post-patch re-fetch is constrained. Avoids expanding the server contract under time pressure.
- **Cons**: Server-mode re-fetch against an arbitrary dir is not correct; the patched bytes/offsets won't be pulled back for non-FIXTURE_BASE dirs.

### Option B: Change the server contract now to serve arbitrary patched dirs back
- **Pros**: Fully correct server-mode re-fetch for any dir.
- **Cons**: Larger scope — a server-contract change — beyond task #50. Out of phase-one budget.

## Decision

Chose Option A. The FIXTURE_BASE-relative resolution of `repointAudio` / `reloadManifest` is accepted as a known phase-one limitation. A full fix needs a server-contract change to serve patched outputs back from an arbitrary directory, deferred as a follow-up.

## Consequences

- Escalate ships in task #50; post-patch re-fetch is correct only when the served dir is FIXTURE_BASE.
- For an arbitrary server dir, the patched audio/manifest are not re-fetched correctly — a real gap, documented rather than silently shipped.
- A follow-up task is owed for the server-contract change.

## Revisit trigger

When a server-contract change lands that lets the player fetch a patched block's outputs back from an arbitrary served directory — at that point `repointAudio` / `reloadManifest` should resolve against the user-supplied server dir instead of FIXTURE_BASE.

## Related decisions

- [Manual absolute-path dir field in TopBar is the server-mode escalate enabler](../architecture/2026-06-22-topbar-manual-absolute-dir-field-enables-server-escalate.md) — the arbitrary dir the re-fetch cannot yet resolve against.
- [Player playback unit stays the whole audio.wav](../architecture/2026-06-22-player-playback-unit-stays-whole-audio-wav.md) — the blob `repointAudio` re-fetches.
