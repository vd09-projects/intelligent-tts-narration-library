import { useState } from 'react'

// EscalateCard is the inline expansion that appears under a BlockRow when
// the user hits "Escalate L3" (plan D5). Shows the literal CLI command the
// user can run on their host to re-narrate that block at L3 detail.
//
// No modal, no toast — inline keeps the keyboard focus on the row.
// The Copy button uses navigator.clipboard.writeText which is gated to
// user-gesture in all browsers; we tolerate a rejected promise quietly.
export function EscalateCard({
  command,
  onDismiss,
}: {
  command: string
  onDismiss: () => void
}) {
  const [copied, setCopied] = useState(false)

  const onCopy = async () => {
    try {
      await navigator.clipboard.writeText(command)
      setCopied(true)
      window.setTimeout(() => setCopied(false), 1500)
    } catch {
      // Clipboard denied (e.g. insecure context). The literal command is
      // visible in the <code>; the user can hand-copy. Honesty rule: don't
      // pretend the copy succeeded.
      setCopied(false)
    }
  }

  return (
    <div className="escalate-card" data-testid="escalate-card" role="region" aria-label="Escalate command">
      <p className="escalate-blurb">
        Run this command on your host to re-narrate this block at L3 detail
        and reload the directory to hear the result:
      </p>
      <pre className="escalate-command">
        <code>{command}</code>
      </pre>
      <div className="escalate-actions">
        <button type="button" onClick={onCopy}>
          {copied ? 'Copied!' : 'Copy command'}
        </button>
        <button type="button" onClick={onDismiss}>
          Dismiss
        </button>
      </div>
      <p className="escalate-footnote muted">
        Phase one: this player does not re-run narration in-app. After the
        CLI command finishes writing the output directory, reload the
        directory to refresh the audio + plan.
      </p>
    </div>
  )
}
