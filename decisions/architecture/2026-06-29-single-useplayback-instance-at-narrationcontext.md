# Single usePlayback instance lives at the NarrationContext provider

| Field    | Value            |
|----------|------------------|
| Date     | 2026-06-29       |
| Status   | accepted         |
| Category | architecture     |
| Tags     | issue-112, earshot, useplayback, narrationcontext, playbackcontext, single-instance, raf-sync-loop, restore-gate, transport-command-surface, shared-audio, context-provider, playback-engine |

## Context

The Earshot playback engine centers on `usePlayback`, which owns three things: the requestAnimationFrame sync loop (active-block derivation), the transport command surface (play/pause/seek/next/prev), and the `restoring` gate for resume. Multiple components consume playback — `TransportDeck`, `BlockScrubber`, `BlockRow`. If each instantiated its own `usePlayback`, each would fork the rAF loop, the restoring gate, and the active-block truth over a single shared `<audio>` element, producing competing writers and divergent state.

## Decision

`usePlayback` is instantiated EXACTLY ONCE, at the `NarrationContext` / `PlaybackContext` provider. All consumers (`TransportDeck`, `BlockScrubber`, `BlockRow`) read its command surface from context rather than calling `usePlayback` themselves.

A second instance would fork the rAF loop, the restoring gate, and active-block truth over the single shared `<audio>` — three sources of truth fighting over one element.

## Consequences

- One rAF loop, one restoring gate, one active-block truth — no competing writers over the shared `<audio>`.
- Consumers depend on the provider being mounted above them; using the command surface outside the provider is a programming error.
- Reinforces the whole-wav playback-unit and rAF-transition-only-rerender decisions by ensuring there is a single owner of those mechanisms.

## Related decisions

- [Player playback unit stays the whole audio.wav](2026-06-22-player-playback-unit-stays-whole-audio-wav.md) — reinforced: one shared audio element.
- [Player rAF audio sync re-renders on transition only](2026-06-21-player-raf-audio-sync-transition-only-rerender.md) — reinforced: the single rAF loop lives in the one instance.
- [Earshot restore gate clears only on a landed seek](2026-06-29-restore-gate-clears-only-on-landed-seek.md) — the restoring gate that this single instance owns.

## Revisit trigger

If Earshot ever needs to play more than one narration simultaneously (multiple concurrent audio elements), the single-instance assumption breaks and the loop/gate ownership must be rethought.
