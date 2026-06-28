// LevelControl.tsx — per-block L1/L2/L3 fidelity control (#113).
//
// APG Radio Group (manual-selection variant): one tab stop (roving tabindex);
// ←/→ and ↑/↓ move roving focus ONLY; Home/End jump to first/last. Selection is
// COMMIT-ON-ACTIVATE — Space/Enter (native <button> activation) or a click on the
// FOCUSED cell. Exactly one aria-checked (the committed level, or the in-flight
// target held across the async window and snapped back on failure).
//
// INTENTIONAL DIVERGENCE from SegmentedToggle (which is select-follows-focus):
// each commit here triggers a BILLABLE re-narration, so selection must be a
// deliberate act, not a side effect of arrowing. APG explicitly permits this
// manual-selection variant (Decision v2 — a11y). We therefore do NOT call
// onCommit on arrow keys; the native button click (Space/Enter/mouse) is the
// only commit path, and arrow handlers preventDefault to move focus without
// activating.

import { useEffect, useRef, useState } from "react";
import type { Level } from "../api/types";
import type { BlockLevelingState } from "../hooks/useNarrationSession";

const LEVELS: Level[] = [1, 2, 3];

export function LevelControl({
  current,
  leveling,
  blockLabel,
  onCommit,
  disabled = false,
}: {
  /** The committed level (block.level). */
  current: Level;
  /** Per-block leveling status (loading/error/refused-inline + in-flight target). */
  leveling: BlockLevelingState;
  /** Names the control + the block for assistive tech. */
  blockLabel: string;
  /** Commit a new level (a billable re-narration on a cache miss). */
  onCommit?: (level: Level) => void;
  /** Disable the whole group (e.g. an older server without render_id). */
  disabled?: boolean;
}) {
  const busy = leveling.phase === "loading";
  // aria-checked tracks the in-flight target during the async window, else the
  // committed level (snaps back on error/refusal-result since target clears).
  const checkedLevel = leveling.target ?? current;
  const refs = useRef<(HTMLButtonElement | null)[]>([]);
  const [focusIndex, setFocusIndex] = useState(() => LEVELS.indexOf(checkedLevel));

  // Keep the roving tab stop on the checked cell when the committed level changes
  // (e.g. a successful escalate) and the user is not mid-arrowing.
  useEffect(() => {
    setFocusIndex(LEVELS.indexOf(checkedLevel));
  }, [checkedLevel]);

  const moveFocus = (next: number) => {
    setFocusIndex(next);
    refs.current[next]?.focus();
  };

  const onKeyDown = (e: React.KeyboardEvent, index: number) => {
    // Arrows/Home/End move ROVING FOCUS only — never commit. Space/Enter are NOT
    // handled here: they fall through to the native <button> click → onCommit.
    let next: number | null = null;
    if (e.key === "ArrowRight" || e.key === "ArrowDown") {
      next = (index + 1) % LEVELS.length;
    } else if (e.key === "ArrowLeft" || e.key === "ArrowUp") {
      next = (index - 1 + LEVELS.length) % LEVELS.length;
    } else if (e.key === "Home") {
      next = 0;
    } else if (e.key === "End") {
      next = LEVELS.length - 1;
    } else {
      return;
    }
    e.preventDefault();
    moveFocus(next);
  };

  const isAlert = leveling.phase === "error" || leveling.phase === "refused-inline";
  const statusText = busy
    ? `Re-narrating block at L${checkedLevel}…`
    : leveling.phase === "idle"
      ? (leveling.message ?? "")
      : "";
  const alertText = isAlert ? (leveling.message ?? "") : "";

  return (
    <div className="level-control" data-testid="level-control">
      <div
        role="radiogroup"
        aria-label={`Narration level for ${blockLabel}`}
        className="level-control__group"
      >
        {LEVELS.map((lvl, i) => {
          const checked = lvl === checkedLevel;
          return (
            <button
              key={lvl}
              ref={(el) => {
                refs.current[i] = el;
              }}
              type="button"
              role="radio"
              aria-checked={checked}
              aria-label={`Level ${lvl}`}
              tabIndex={i === focusIndex ? 0 : -1}
              disabled={disabled || busy}
              className={"level-control__cell" + (checked ? " is-checked" : "")}
              // Commit on activate: native Space/Enter on a focused button fires
              // this click, as does a mouse click. Arrows do NOT (preventDefault).
              onClick={() => onCommit?.(lvl)}
              onKeyDown={(e) => onKeyDown(e, i)}
            >
              <span aria-hidden="true">L{lvl}</span>
              {/* Non-color committed marker (an inset ring glyph), in addition to
                  color + aria-checked, so the selection is never color-only. */}
              {checked ? (
                <span className="level-control__marker" aria-hidden="true" data-testid="level-marker">
                  {" ●"}
                </span>
              ) : null}
            </button>
          );
        })}
      </div>
      {/* Live regions: present in the DOM (empty when idle) so updates announce.
          status = polite progress / "now at Ln"; alert = assertive error/refusal. */}
      <span role="status" aria-live="polite" className="level-control__status">
        {statusText}
      </span>
      <span role="alert" className="level-control__alert">
        {alertText}
      </span>
    </div>
  );
}
