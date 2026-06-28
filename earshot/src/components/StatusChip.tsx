// StatusChip.tsx — a non-color status marker on a session row (plan Step 4 +
// Color & Contrast). Carries a TEXT label + icon, never color alone. Kinds:
// "stale" (manifest staleness) and "error" (a per-row fetch/render fault).
//
// TODO: drive the stale-vs-error source from the live #109 contract field in
// useSessionMessages once the session-row metadata shape is confirmed, because
// the shell currently has no server field to populate it and defaults to none.

export type ChipKind = "stale" | "error";

const LABEL: Record<ChipKind, string> = {
  stale: "Stale",
  error: "Error",
};

const ICON: Record<ChipKind, string> = {
  stale: "⚠",
  error: "✕",
};

export function StatusChip({ kind }: { kind: ChipKind }) {
  return (
    <span className={"status-chip status-chip--" + kind} data-kind={kind}>
      <span aria-hidden="true" className="status-chip__icon">
        {ICON[kind]}
      </span>
      {LABEL[kind]}
    </span>
  );
}
