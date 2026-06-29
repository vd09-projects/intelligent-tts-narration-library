# Global hotkey focus guard is an opt-in allow-list

| Field    | Value            |
|----------|------------------|
| Date     | 2026-06-29       |
| Status   | accepted         |
| Category | convention       |
| Tags     | issue-134, earshot, keyboard, transport, playback, a11y, accessibility, aria, focus-guard, allow-list, data-transport-hotkeys, safe-by-default |

## Context

Issue #134 adds global transport hotkeys (Space, ←/→) to Earshot. earshot/ now exposes several keyboard-interactive ARIA widgets that own Space and/or arrows locally: the #112 role=toolbar roving button group and role=slider block scrubber, the #111 role=listbox session pane, and the #113 role=radiogroup per-block level control. A global handler must not clash with any of them, nor with any widget added later. The original v1 plan used a deny-list enumerating widgets to bail on; plan review flagged it as incomplete (BLOCKING-1) because a deny-list must enumerate every widget and silently breaks when a new one is added.

## Options considered

### Option A: deny-list — handler active by default, bails on a known set of widget roles
- **Pros**: simple to reason about for the widgets that exist today.
- **Cons**: must enumerate every keyboard-interactive widget; silently breaks (hijacks keys) the moment a new ARIA widget is added without updating the list; unbounded per-widget maintenance.

### Option B: allow-list — handler inert by default, acts only inside an explicitly marked region or on body
- **Pros**: every current and future ARIA widget is safe-by-default with zero per-widget maintenance; the global handler is opt-in, so adding a widget anywhere outside the marked region cannot be hijacked.
- **Cons**: the transcript region must be explicitly marked with `[data-transport-hotkeys]`.

## Decision

The global transport handler is **inert by default** and acts ONLY when focus is on `document.body`/null OR inside an explicitly marked `[data-transport-hotkeys]` region (the transcript `<main>`). This is load-bearing and supersedes the earlier deny-list approach. The allow-list makes the #111 role=listbox, #113 role=radiogroup, #112 role=toolbar/role=slider, and any FUTURE keyboard-interactive ARIA widget safe-by-default with no per-widget maintenance, versus a deny-list that must enumerate every widget and silently breaks when a new one is added. A trimmed native-control bail inside the marked region (button, a[href], input, [role=slider], [role=radio], etc.) is defense-in-depth only.

## Consequences

- New ARIA widgets are safe without touching the hotkey hook, as long as they live outside `[data-transport-hotkeys]` or own their own keys.
- The transcript `<main>` carries `data-transport-hotkeys` plus the seek `aria-keyshortcuts`; the play/pause button keeps `aria-keyshortcuts="Space"`.
- The in-region native-control bail is redundant under the allow-list and is kept only as defense-in-depth (partial-disagreement noted: full role-by-role deny enumeration is deliberately not the primary mechanism).
- A table-driven focus-guard test proves each non-allowed context bails and that body/transcript focus acts.

## Related decisions

- [Single document keydown listener via the ref pattern](../architecture/2026-06-29-single-document-keydown-listener-via-ref-pattern.md) — the listener that consults this guard on every keydown.
- [Left/Right arrows do ±10s time seek on the wav playhead, not block-step](../tradeoff/2026-06-29-arrow-keys-10s-time-seek-not-block-step.md) — the seek action this guard scopes to the transcript region.

## Revisit trigger

Revisit if a future interactive widget must live INSIDE the transcript region yet own Space/arrows — the in-region defense-in-depth bail would then need that widget's role added, or the widget would need its own marked sub-region.
