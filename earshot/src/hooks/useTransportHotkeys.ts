// useTransportHotkeys.ts — global keyboard transport shortcuts (#134).
//
// Layers YouTube-style media keys onto the shipped #112 transport:
//   Space      → play / pause (toggle)
//   ArrowLeft  → skip −SEEK_STEP_SECONDS
//   ArrowRight → skip +SEEK_STEP_SECONDS
//
// Mounted EXACTLY ONCE (in Shell) and reads the SINGLE usePlayback instance via
// usePlaybackControls() — never a second hook instance. A document-level keydown
// listener is registered once at mount and reads a latestRef snapshot per event
// (the REF pattern) so it is immune to controls-identity churn and never lifts
// hasAudio into shell state (progress ticks neither re-render the shell nor
// re-bind the listener).
//
// Registration is on the BUBBLE phase: every ARIA widget that owns Space/arrows
// locally (the #112 toolbar + slider, the #111 listbox, the #113 radiogroup, and
// any future widget) handles the key FIRST. The focus guard below is therefore
// the SOLE real guarantee against clashes — a React synthetic stopPropagation
// does not reliably stop a native document listener.

import { useEffect, useRef } from "react";
import { useNarration } from "../state/NarrationContext";
import { usePlaybackControls } from "../state/PlaybackContext";
import { useAnnouncer } from "../state/Announcer";
import { SEEK_STEP_SECONDS } from "./usePlayback";

/** The opt-in allow-list anchor: the global handler only wakes inside this. */
export const HOTKEYS_REGION_ATTR = "data-transport-hotkeys";

/**
 * Native-activatable or value-owning controls that own Space themselves. When
 * focus is on one of these (even inside the opted-in region), Space must reach
 * the control, not the global toggle. Matched against the focused element
 * directly (not an ancestor) — these are leaf controls.
 */
const SPACE_BAIL_SELECTOR = [
  "button",
  '[role="button"]',
  "a[href]",
  "input",
  "textarea",
  "select",
  '[role="slider"]',
  '[role="radio"]',
  '[role="option"]',
  '[role="menuitem"]',
  '[role="tab"]',
  // Only an explicitly-enabled contenteditable owns text entry. A bare
  // [contenteditable] would also match contenteditable="false" — never use it.
  '[contenteditable=""]',
  '[contenteditable="true"]',
].join(", ");

/**
 * Composite / roving widgets and text entry that own the arrow keys themselves.
 * Matched against the focused element OR any ancestor (closest), since a roving
 * child (e.g. a radio inside a radiogroup) is what actually holds focus.
 */
const ARROW_BAIL_SELECTOR = [
  '[role="toolbar"]',
  '[role="slider"]',
  '[role="radiogroup"]',
  '[role="listbox"]',
  '[role="menu"]',
  '[role="menubar"]',
  '[role="tablist"]',
  '[role="grid"]',
  '[role="tree"]',
  '[role="spinbutton"]',
  '[role="combobox"]',
  // text entry
  "input",
  "textarea",
  "select",
  '[contenteditable=""]',
  '[contenteditable="true"]',
].join(", ");

const SPACE_KEY = " ";

/** The transport action a key resolves to, or null when the hook stands down. */
export type TransportHotkeyAction = "toggle" | "back" | "forward";

/**
 * Pure focus-guard + key-mapping (load-bearing). Returns the transport action a
 * key should drive given the currently focused element, or null when the hook
 * must stand down (key not handled, or focus is on a context that owns the key).
 *
 * ALLOW-LIST, not deny-list: the handler is inert by default and acts ONLY when
 * focus is on the document body (or nothing) or inside an element explicitly
 * marked [data-transport-hotkeys]. This makes every other widget — current and
 * future — safe with no per-widget maintenance. The per-control bails inside the
 * region are defense-in-depth so native/roving controls keep their own keys.
 */
export function transportHotkeyAction(
  key: string,
  activeElement: Element | null,
): TransportHotkeyAction | null {
  const isHotkey = key === SPACE_KEY || key === "ArrowLeft" || key === "ArrowRight";
  if (!isHotkey) return null;

  // Allow-list gate: body / nothing focused, or inside the opted-in region.
  const inRegion =
    activeElement == null ||
    activeElement === document.body ||
    activeElement.closest(`[${HOTKEYS_REGION_ATTR}]`) != null;
  if (!inRegion) return null;

  if (key === SPACE_KEY) {
    if (activeElement != null && activeElement.matches(SPACE_BAIL_SELECTOR)) return null;
    return "toggle";
  }

  // ArrowLeft / ArrowRight.
  if (activeElement != null && activeElement.closest(ARROW_BAIL_SELECTOR) != null) return null;
  return key === "ArrowLeft" ? "back" : "forward";
}

/**
 * Mount-once global transport hotkeys. Call exactly once, in Shell, inside the
 * Announcer + Narration + Playback providers. Returns nothing — it only wires a
 * document listener for its lifetime.
 */
export function useTransportHotkeys(): void {
  const controls = usePlaybackControls();
  const { activeAudioUrl } = useNarration();
  const { announce } = useAnnouncer();
  const hasAudio = Boolean(activeAudioUrl);

  // REF pattern: refresh the snapshot every render; the listener reads it per
  // event so it always sees current values without re-binding (and without
  // lifting hasAudio into shell state).
  const latestRef = useRef({ hasAudio, controls, announce });
  latestRef.current = { hasAudio, controls, announce };

  useEffect(() => {
    function onKeyDown(e: KeyboardEvent) {
      const { hasAudio, controls, announce } = latestRef.current;
      if (!hasAudio) return; // no loaded wav → all keys are silent no-ops
      const action = transportHotkeyAction(e.key, document.activeElement);
      if (action == null) return; // not handled → do NOT preventDefault

      // Handled: claim the key (e.g. stop Space from scrolling the page). This
      // fires on EVERY handled+owned key — even when the seek below turns out to
      // be a clamped no-op — so the transport keeps owning the key in-region and
      // scroll prevention never regresses. Only the announcement is conditional.
      e.preventDefault();
      switch (action) {
        case "toggle":
          controls.toggle();
          break;
        case "back": {
          // Announce off the ACTUAL move: skipSeconds returns false on a clamped
          // boundary / pre-metadata no-op, so a silent playhead that didn't move
          // never claims "Back N seconds". Focus stays put on a seek, hence the
          // polite live-region announcement for SR users when it DOES move.
          const moved = controls.skipSeconds(-SEEK_STEP_SECONDS);
          if (moved) announce(`Back ${SEEK_STEP_SECONDS} seconds`);
          break;
        }
        case "forward": {
          const moved = controls.skipSeconds(SEEK_STEP_SECONDS);
          if (moved) announce(`Forward ${SEEK_STEP_SECONDS} seconds`);
          break;
        }
      }
    }

    // Bubble phase (no capture flag): widget handlers run first; the focus guard
    // is the real guarantee against clashes.
    document.addEventListener("keydown", onKeyDown);
    return () => document.removeEventListener("keydown", onKeyDown);
    // Mount-once: the listener body reads latestRef + module-level helpers only,
    // so empty deps is correct AND deliberate (the REF pattern) — it must never
    // re-bind on render.
  }, []);
}
