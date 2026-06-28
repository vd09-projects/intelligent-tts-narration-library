// SegmentedToggle.tsx — Spoken | Source segmented control (plan Step 3).
//
// APG radiogroup with ROVING TABINDEX: one tab stop for the whole group; ←/→
// (and ↑/↓) select the adjacent option in a SINGLE step (selection follows
// focus — no second Space/Enter needed). Spoken is the default. Non-color: the
// selected option carries aria-checked + a text-weight/underline marker, not
// color alone.

import { useCallback, useRef } from "react";

export type TranscriptView = "spoken" | "source";

const OPTIONS: { value: TranscriptView; label: string }[] = [
  { value: "spoken", label: "Spoken" },
  { value: "source", label: "Source" },
];

export function SegmentedToggle({
  value,
  onChange,
  label = "Transcript view",
}: {
  value: TranscriptView;
  onChange: (v: TranscriptView) => void;
  label?: string;
}) {
  const refs = useRef<(HTMLButtonElement | null)[]>([]);

  const onKeyDown = useCallback(
    (e: React.KeyboardEvent, index: number) => {
      let next = index;
      if (e.key === "ArrowRight" || e.key === "ArrowDown") {
        next = (index + 1) % OPTIONS.length;
      } else if (e.key === "ArrowLeft" || e.key === "ArrowUp") {
        next = (index - 1 + OPTIONS.length) % OPTIONS.length;
      } else {
        return;
      }
      e.preventDefault();
      onChange(OPTIONS[next].value); // selection follows focus, single step
      refs.current[next]?.focus();
    },
    [onChange],
  );

  return (
    <div role="radiogroup" aria-label={label} className="segmented-toggle">
      {OPTIONS.map((opt, i) => {
        const checked = opt.value === value;
        return (
          <button
            key={opt.value}
            ref={(el) => {
              refs.current[i] = el;
            }}
            type="button"
            role="radio"
            aria-checked={checked}
            tabIndex={checked ? 0 : -1}
            className={"segmented-toggle__option" + (checked ? " is-checked" : "")}
            onClick={() => onChange(opt.value)}
            onKeyDown={(e) => onKeyDown(e, i)}
          >
            {opt.label}
          </button>
        );
      })}
    </div>
  );
}
