// SessionRow.tsx — one listbox option. Shows title snippet · turn · block count
// plus a per-row status indicator (spinner for loading, ✓ for ready, ✗ for error).

import type { SessionMessage } from "../api/types";
import type { NarrationEntry } from "../hooks/useNarrationSession";

function snippet(message: SessionMessage): string {
  const first = message.blocks[0]?.text ?? "";
  const oneLine = first.replace(/\s+/g, " ").trim();
  return oneLine.length > 80 ? oneLine.slice(0, 79) + "…" : oneLine || "(empty message)";
}

function EntryStatusBadge({ status }: { status: NarrationEntry["status"] }) {
  if (status === "loading") {
    return (
      <span className="session-row__badge session-row__badge--loading" aria-label="Narrating">
        <span className="session-row__spinner" aria-hidden="true" />
        <span className="visually-hidden">Narrating…</span>
      </span>
    );
  }
  if (status === "ready") {
    return (
      <span className="session-row__badge session-row__badge--ready" aria-label="Ready">
        ✓
      </span>
    );
  }
  if (status === "error") {
    return (
      <span className="session-row__badge session-row__badge--error" aria-label="Error">
        ✗
      </span>
    );
  }
  return null;
}

export function SessionRow({
  message,
  optionId,
  selected,
  active,
  entryStatus,
  onActivate,
}: {
  message: SessionMessage;
  optionId: string;
  selected: boolean;
  active: boolean;
  entryStatus?: NarrationEntry["status"];
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
      <div className="session-row__top">
        <span className="session-row__title">{snippet(message)}</span>
        {entryStatus ? <EntryStatusBadge status={entryStatus} /> : null}
      </div>
      <span className="session-row__meta">
        {message.role} · turn {message.turn} · {length} block{length === 1 ? "" : "s"}
      </span>
    </div>
  );
}
