# Single document keydown listener via the ref pattern

| Field    | Value            |
|----------|------------------|
| Date     | 2026-06-29       |
| Status   | accepted         |
| Category | architecture     |
| Tags     | issue-134, earshot, keyboard, transport, playback, react, ref-pattern, event-listener, stale-closure, usetransporthotkeys |

## Context

Issue #134's global transport hotkeys (Space, ←/→) need a document-level keydown listener in the Earshot React shell. The naive approach re-binds the listener whenever its dependencies change (controls identity, hasAudio), which churns add/remove pairs and risks stale closures over render values. Playback progress ticks frequently, so any state that the listener depends on must not force re-renders or re-binds of the shell.

## Options considered

### Option A: dep-array re-binding
- **Pros**: idiomatic-looking useEffect with explicit deps.
- **Cons**: the listener add/remove pair fires every time controls identity churns; tempts lifting `hasAudio` into shell state, which makes progress ticks re-render the shell and re-bind the listener.

### Option B: ref pattern — one mount-time listener reading latestRef.current
- **Pros**: immune to controls-identity churn (one add/remove pair for the component's life); does NOT lift hasAudio into shell state, so playback progress ticks neither re-render the shell nor re-bind the listener; no stale closures because the body reads latestRef.current at event time.
- **Cons**: slightly less obvious than a dep-array effect; requires assigning latestRef.current on each render.

## Decision

The global hotkey hook (`useTransportHotkeys`) keeps exactly ONE mount-time document keydown listener — a `useEffect` with `[]` deps and cleanup on unmount — whose body reads `latestRef.current` rather than closing over render values. Each render assigns `latestRef.current = { hasAudio, controls }`; the listener destructures from `latestRef.current` per event. The listener is registered on the BUBBLE phase. Chosen over dep-array re-binding.

## Consequences

- Exactly one add/remove pair over the component's lifetime; no listener churn.
- hasAudio stays out of shell state; progress ticks do not re-render the shell or touch the listener.
- A stale-closure regression test (mount with no audio, load audio, press Space → toggle() fires) locks the pattern.
- Bubble-phase registration means widget handlers run first; the focus-guard allow-list is the sole real guarantee against clashes (a React synthetic stopPropagation does not reliably stop a native document listener).

## Related decisions

- [Global hotkey focus guard is an opt-in allow-list](../convention/2026-06-29-global-hotkey-focus-guard-opt-in-allow-list.md) — the guard that this single bubble-phase listener consults before acting.
- [Single usePlayback instance lives at the NarrationContext provider](2026-06-29-single-useplayback-instance-at-narrationcontext.md) — the hook consumes that one instance via usePlaybackControls().

## Revisit trigger

Revisit if Earshot ever needs multiple concurrent keyboard contexts that cannot share one document listener.
