import type { Manifest } from '../types/manifest.ts'
import { StaleBadge } from './StaleBadge.tsx'

// TopBar — title, load-directory affordance, voice indicator, stale badge.
//
// Two load buttons exposed when the browser does not ship the File System
// Access API (Safari, Firefox): the FS-API button is disabled with a
// tooltip explaining why, and the <input type=file webkitdirectory> stays
// visible. Both buttons live behind real <button> elements so keyboard
// users get focus rings (a11y rule).

export interface TopBarProps {
  manifest: Manifest | null
  voice: string
  warnings: string[]
  supportsFsAccess: boolean
  onPickDirectory: () => void
  onFileListChange: (files: File[]) => void
}

export function TopBar({
  manifest,
  voice,
  warnings,
  supportsFsAccess,
  onPickDirectory,
  onFileListChange,
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
