# Player audio sync via requestAnimationFrame; React state writes only on block transition

| Field    | Value            |
|----------|------------------|
| Date     | 2026-06-21       |
| Status   | accepted         |
| Category | convention       |
| Tags     | player, audio-sync, react, performance, block-level-sync, issue-18 |

## Context

The reference player highlights the currently-playing block as the audio progresses. The library's block-level-sync invariant (CLAUDE.md) means highlight transitions only happen at block boundaries, not per word or per frame. Two timing sources:

- **A — `HTMLMediaElement.timeupdate` event.** Fires roughly every 250 ms.
- **B — `requestAnimationFrame` polling.** Fires every paint (~16 ms at 60 Hz).
- **C — `setInterval(..., N)`.** Coarse, jittery, always re-renders.

Some blocks in `docs/samples/sample.md` are <500 ms long. A 250 ms tick misses block boundaries badly enough that highlight transitions feel laggy. But naïve rAF that writes React state every paint would re-render the whole tree 60 times per second for no visible reason.

## Options considered

### Option A: timeupdate event, dispatch on every fire
- **Pros**: Trivially simple — one event listener.
- **Cons**: 250 ms granularity misses sub-500 ms blocks. Highlight visibly lags audio.

### Option B: rAF polling + ref guard, dispatch only on block-id transition
- **Pros**: Frame-accurate timing; React re-renders only when the active block actually changes (most frames are no-ops — the rAF loop reads `audio.currentTime`, runs binary search, compares to a ref, returns). Performance-clean.
- **Cons**: Slightly more code than option A — needs a ref to dedupe transitions.

### Option C: setInterval polling
- **Pros**: Predictable cadence regardless of frame rate.
- **Cons**: Either always re-renders (waste) or has the same coarseness problem as timeupdate at usable intervals. Continues firing in background tabs (wasteful).

## Decision

**Choose B.** `usePlayback` runs a `requestAnimationFrame` loop that samples `audio.currentTime`, converts to ms, runs `findActiveBlock` (binary search over `manifest.blocks` by `start_ms`), and dispatches `SET_ACTIVE_BLOCK` *only* when the block id differs from the ref-tracked previous id. The audio element drives time; React drives presentation; the two are decoupled by the rAF tick + ref guard.

`findActiveBlock` is a pure function with table-driven unit tests covering pre-roll, intra-block, inter-block gap, and post-roll cases.

## Consequences

- Background tabs throttle rAF — playback sync pauses when the player is hidden. Acceptable (a feature, not a bug).
- React DevTools shows render commits only at block transitions, not 60 Hz. Easy to verify performance behavior.
- The ref guard pattern (`activeBlockIdRef.current` written before `setActiveBlockId`) is documented in code so a future reader doesn't "simplify" it back into per-frame state updates.

## Related decisions

- Block-level sync only is a project-wide invariant (CLAUDE.md). This decision is the UI-side manifestation.
