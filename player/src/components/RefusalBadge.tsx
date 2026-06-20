import type { Refusal } from '../types/plan.ts'

// RefusalBadge renders the honesty-rule signal next to a refused block
// (CLAUDE.md "Refusal is data, not error" + "Refused blocks display the
// refusal message inline with a visible refused badge").
//
// Three signals carried simultaneously so color never stands alone:
//   1. Red pill + the literal text "REFUSED"
//   2. The machine reason (enum value)
//   3. The human spoken_notice — the same prose the listener heard
//
// role="status" so screen readers announce the refusal as a status update,
// not a button-equivalent. The badge is decorative-action-wise.
export function RefusalBadge({ refusal }: { refusal: Refusal }) {
  return (
    <div className="refusal" role="status" data-testid="refusal-badge">
      <span className="badge badge-refused">
        <span aria-hidden="true">⨯</span> REFUSED
      </span>
      <code className="refusal-reason">{refusal.reason}</code>
      <p className="refusal-notice">{refusal.message}</p>
      {refusal.spoken && (
        <p className="refusal-spoken">
          <span className="muted">spoken notice:</span> “{spokenLabel(refusal)}”
        </p>
      )}
    </div>
  )
}

function spokenLabel(refusal: Refusal): string {
  // The refusal.message is what the listener heard (sink writes it into the
  // plan), but we surface message directly above. If the sink ever provides
  // a distinct verbatim transcript we can switch this off — phase one just
  // echoes the message so the UI is honest about what was voiced.
  return refusal.message
}
