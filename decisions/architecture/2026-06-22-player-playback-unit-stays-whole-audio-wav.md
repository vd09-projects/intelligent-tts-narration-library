# Player playback unit stays the whole audio.wav; re-point only the playing block on patch

| Field    | Value            |
|----------|------------------|
| Date     | 2026-06-22       |
| Status   | accepted         |
| Category | architecture     |
| Tags     | player, escalate, block-level-sync, audio-playback, react, issue-50 |

## Context

Task #50 added in-place Escalate L2/L3 buttons to the React player. Escalation re-renders one block's audio and patches the persistent-sink output (new `audio.wav` bytes for that block, new manifest offsets). The player had to decide what its unit of playback is once a single block's audio can change underneath a running session.

The core library's block-level-sync invariant forbids word-level timing and treats the whole document audio as one engine-neutral blob with offsets keyed by `block_id` in the manifest. The player needed an update model consistent with that invariant.

## Options considered

### Option A: Keep the whole audio.wav as the playback unit
- **Pros**: Honors the block-level-sync invariant directly. One `<audio>` element, one blob, offsets from manifest. Patch handling reduces to: re-point/re-fetch the single blob only when the patched block is the one currently playing; otherwise just update manifest offsets in state.
- **Cons**: Re-fetching the blob mid-play for the active block requires preserving playhead position.

### Option B: Per-block `<audio>` elements
- **Pros**: A patched block could swap just its own element's source without touching others.
- **Cons**: Effectively introduces per-block audio wiring, which drifts toward the word/segment-level sync the core invariant forbids. More elements, more state, sequencing logic the manifest already encodes.

## Decision

Chose Option A. The player's playback unit stays the whole `audio.wav`. On a block patch: re-point/re-fetch the single blob ONLY when the patched block is the one currently playing; otherwise update the manifest offsets in state and leave the audio element alone. This keeps the player faithful to the block-level-sync invariant and rejects per-block `<audio>` wiring.

## Consequences

- Player stays aligned with the core invariant — no per-block audio elements, no path toward word-level sync.
- Patching a non-playing block is cheap: state-only manifest offset update, no audio reload.
- Patching the currently-playing block requires re-pointing the blob and preserving the playhead.

## Related decisions

- [Per-block WAVs stay separate; renderer does not concatenate](../architecture/2026-06-18-per-block-wavs-no-concat-in-renderer.md) — renderer-side counterpart; sink concatenates into the single blob the player consumes.
- [Player audio sync via requestAnimationFrame; React state writes only on block transition](../convention/2026-06-21-player-raf-audio-sync-transition-only-rerender.md) — the sync loop that runs over this single blob.
