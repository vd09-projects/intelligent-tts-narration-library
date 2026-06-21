import type { Manifest } from '../types/manifest.ts'
import type { ServerMode } from '../hooks/useServerMode.ts'
import { StaleBadge } from './StaleBadge.tsx'

// TopBar — title, load-directory affordance, voice indicator, stale badge,
// escalate-server mode indicator + (server-mode-only) manual dir field.
//
// Two load buttons exposed when the browser does not ship the File System
// Access API (Safari, Firefox): the FS-API button is disabled with a
// tooltip explaining why, and the <input type=file webkitdirectory> stays
// visible. Both buttons live behind real <button> elements so keyboard
// users get focus rings (a11y rule).
//
// Escalate-server affordances (issue #50):
//   - mode indicator: shows whether live escalation is available.
//   - dir field (server mode only, Q2): the server resolves `dir` server-side,
//     so the player cannot infer it from a blob-loaded directory. The user
//     types the absolute path the server should patch.
//   - AC9 hint banner: when mode === 'fixture', a non-modal inline hint that
//     escalation is copy-command-only until `make run-server` is running.

export interface TopBarProps {
  manifest: Manifest | null
  voice: string
  warnings: string[]
  supportsFsAccess: boolean
  onPickDirectory: () => void
  onFileListChange: (files: File[]) => void
  serverMode: ServerMode
  dir: string
  onDirChange: (dir: string) => void
}

export function TopBar({
  manifest,
  voice,
  warnings,
  supportsFsAccess,
  onPickDirectory,
  onFileListChange,
  serverMode,
  dir,
  onDirChange,
}: TopBarProps) {
  return (
    <header className="topbar" role="banner">
      <div className="topbar-left">
        <h1 className="topbar-title">Intelligent TTS Reference Player</h1>
        <span className="muted topbar-subtitle">
          block-level sync · honest refusal · escalate-on-demand
        </span>
      </div>
      <div className="topbar-right">
        <ServerModeIndicator mode={serverMode} />
        {serverMode === 'server' && (
          <label className="escalate-dir-field">
            <span className="muted">escalate dir:</span>{' '}
            <input
              type="text"
              className="escalate-dir-input"
              placeholder="/absolute/path/to/output/dir"
              value={dir}
              onChange={(e) => onDirChange(e.currentTarget.value)}
              aria-label="Escalate output directory (absolute path on the server host)"
              data-testid="escalate-dir-input"
            />
          </label>
        )}
        <span className="voice-indicator" aria-label={`Voice: ${voice || 'none'}`}>
          <span className="muted">voice:</span>{' '}
          <code>{voice || '(none recorded)'}</code>
        </span>
        <StaleBadge manifest={manifest} />
        <button
          type="button"
          onClick={onPickDirectory}
          disabled={!supportsFsAccess}
          title={
            supportsFsAccess
              ? 'Pick a sink/persistent output directory'
              : 'Browser does not support showDirectoryPicker; use the file input fallback below.'
          }
        >
          Load directory…
        </button>
        <label className="file-input-fallback">
          <span className="muted">or pick folder:</span>
          <input
            type="file"
            // webkitdirectory is non-standard; React passes unknown attrs to
            // the DOM, but TS doesn't know about them so we suppress the
            // typing error inline.
            // @ts-expect-error — non-standard but supported in all modern browsers.
            webkitdirectory=""
            directory=""
            multiple
            onChange={(e) => {
              const list = e.currentTarget.files
              if (list && list.length > 0) {
                onFileListChange(Array.from(list))
              }
            }}
          />
        </label>
      </div>
      {serverMode === 'fixture' && (
        <p
          className="escalate-server-hint"
          role="status"
          data-testid="escalate-server-hint"
        >
          Escalate server not reachable — run <code>make run-server</code> to
          enable in-place L2/L3 escalation. Until then, the per-block escalate
          control shows the CLI command to run instead.
        </p>
      )}
      {warnings.length > 0 && (
        <ul className="warnings" role="status" aria-label="Warnings">
          {warnings.map((w, i) => (
            <li key={i}>{w}</li>
          ))}
        </ul>
      )}
    </header>
  )
}

// ServerModeIndicator surfaces the escalate-server mode. 'unknown' renders a
// neutral probing label so there is no flicker between fixture/server controls
// before the first /healthz resolves.
function ServerModeIndicator({ mode }: { mode: ServerMode }) {
  const label =
    mode === 'server'
      ? 'live'
      : mode === 'fixture'
        ? 'offline (copy-command)'
        : 'checking…'
  return (
    <span
      className="server-mode-indicator"
      data-server-mode={mode}
      data-testid="server-mode-indicator"
      aria-label={`Escalate server: ${label}`}
    >
      <span className="muted">escalate:</span> <code>{label}</code>
    </span>
  )
}
