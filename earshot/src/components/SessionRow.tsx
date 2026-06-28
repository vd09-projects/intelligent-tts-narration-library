// SessionRow.tsx — one listbox option (plan Step 4). Shows a title snippet ·
// turn · length, plus an optional non-color status chip. role="option" with
// aria-selected; the parent listbox owns roving focus via aria-activedescendant.

import type { SessionMessage } from "../api/types";
import { StatusChip, type ChipKind } from "./StatusChip";

function snippet(message: SessionMessage): string {
  const first = message.blocks[0]?.text ?? "";
  const oneLine = first.replace(/\s+/g, " ").trim();
  return oneLine.length > 80 ? oneLine.slice(0, 79) + "…" : oneLine || "(empty message)";
}

export function SessionRow({
  message,
  optionId,
  selected,
  active,
  chip,
  onActivate,
}: {
  message: SessionMessage;
  optionId: string;
  selected: boolean;
  active: boolean;
  chip?: ChipKind;
  onActivate: () => void;
}) {
  const length = message.blocks.length;
  return (
    <div
      id={optionId}
      role="option"
      aria-selected={selected}
      className={"session-row" + (active ? " is-active" : "") + (selected ? " is-selected" : "")}
      onClick={onActivate}
    >
      <span className="session-row__title">{snippet(message)}</span>
      <span className="session-row__meta">
        {message.role} · turn {message.turn} · {length} block{length === 1 ? "" : "s"}
      </span>
      {chip ? <StatusChip kind={chip} /> : null}
    </div>
  );
}
