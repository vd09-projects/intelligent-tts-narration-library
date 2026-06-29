# Earshot resume restore gate clears only on a landed seek, never on a bare timeout

| Field    | Value            |
|----------|------------------|
| Date     | 2026-06-29       |
| Status   | accepted         |
| Category | architecture     |
| Tags     | issue-112, earshot, resume, restore-gate, useplayback, raf, seeked-event, preload-metadata, clobber, honest-over-clobber, code-review-found, playback-engine |

## Context

#112 adds resume to the Earshot playback engine: on load the deck must seek the shared `<audio>` to the saved block's start and resume there. A `restoring` gate in `usePlayback` mutes two things while restore is in flight — (a) the requestAnimationFrame active-block derivation (so rAF doesn't see `currentTime 0` and report block 0) and (b) the resume-writer (so it doesn't persist a position before the seek lands).

The first build cleared the gate on a bare 2s timeout. With `<audio preload="none">` / `preload="metadata"`, the issued seek can fail to land — `currentTime` stays at 0 because no media data is loaded yet. Clearing the gate on the timeout then let rAF derive block 0 and let the resume-writer persist block 0, clobbering the saved position the gate existed to protect. Code review (round 1) caught that the timeout path re-opened the exact clobber the gate was meant to prevent.

## Options considered

### Option A: clear the gate on a bare timeout (first build)
- **Pros**: simple; gate always releases.
- **Cons**: on a seek that never lands (preload none/metadata at currentTime 0), rAF derives block 0 and the resume-writer clobbers the saved position. Reintroduces the bug.

### Option B: clear the gate only on a real landed `seeked` (chosen)
- **Pros**: gate releases only when the seek genuinely landed inside the restored block; no clobber.
- **Cons**: on a genuinely unloadable URL the gate can stay muted for the whole session.

## Decision

The `restoring` gate clears ONLY when a real `seeked` event lands with `currentTime` inside the restored block's `[start, nextStart)` range. A bare timeout does NOT clear the gate. Supporting changes:

- Deck audio uses `preload="metadata"` + an explicit `load()` so the seek has media data to land against.
- The 2s timeout RE-ASSERTS the seek rather than clearing the gate.
- The resume-writer refuses to persist block 0 while `readyState < 1`.

Trade-off explicitly accepted: on a genuinely unloadable URL the gate can remain muted for the session (honest-over-clobber) — better to stay muted than to clobber the user's saved position with a fabricated block-0.

## Consequences

- No silent clobber of the saved resume position when the seek is slow or fails to land.
- A genuinely broken audio URL leaves the gate muted for the session — rAF active-block derivation and resume-writing stay suppressed. Accepted as the honest failure mode.
- Restore correctness now depends on the `seeked` event firing with a landed `currentTime`, which `preload="metadata"` + `load()` makes reliable.

## Related decisions

- [Single usePlayback instance at the NarrationContext provider](2026-06-29-single-useplayback-instance-at-narrationcontext.md) — the gate lives in the single shared usePlayback instance.
- [Earshot resume persists block identity only, never an ms offset](../convention/2026-06-29-earshot-resume-persists-block-identity-only.md) — the saved position this gate protects.
- [usePlayback reset keyed on block id + signature, not manifest identity](2026-06-22-useplayback-reset-on-block-id-signature-not-manifest-identity.md) — related rAF/reset behavior.

## Revisit trigger

If the audio element moves to `preload="auto"` or to a streaming source where `seeked` semantics change, re-evaluate the landed-seek condition and the re-assert-on-timeout behavior.
