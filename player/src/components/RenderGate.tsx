import type { ReactNode } from 'react'

// RenderGate is the companion-mode three-state load gate (#76). It renders the
// existing spinner markup while the separate `--sink persistent` render is still
// in flight, the single DEFINED failure banner when the render never completed,
// or its children (the shipped player UI) once the triple has loaded.
//
// It is INERT outside companion mode — App only mounts it when VITE_COMPANION_DIR
// is set, so the default fixture/picker path never touches this component (AC1
// "no new UI surface"; AC6 inertness). The spinner reuses the existing
// `<p className="loading">` and the banner the existing `<p className="error">`,
// so no new visual surface is introduced beyond the failure copy + handoff hint.

export type RenderGatePhase = 'rendering' | 'failed' | 'rendered'

export interface RenderGateProps {
  phase: RenderGatePhase
  // reason is the machine-stable render_failed cause (no_out_dir /
  // server_unreachable / max_poll_attempts), surfaced in the banner.
  reason?: string | null
  children?: ReactNode
}

export function RenderGate({ phase, reason, children }: RenderGateProps) {
  if (phase === 'rendered') {
    return <>{children}</>
  }

  if (phase === 'failed') {
    return (
      <div className="app">
        <p className="error" data-testid="render-gate-failed">
          <strong>render_failed</strong>
          {reason ? <> ({reason})</> : null} — the companion render did not
          complete. Re-run <code>make run-companion</code> to retry, or check the{' '}
          <code>cmd/narrate --sink persistent</code> process for errors.
        </p>
      </div>
    )
  }

  // phase === 'rendering'
  return (
    <div className="app">
      <p className="loading" data-testid="render-gate-rendering">
        Waiting for render…
      </p>
    </div>
  )
}
