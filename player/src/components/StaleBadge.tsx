import type { Manifest } from '../types/manifest.ts'

// StaleBadge surfaces manifest.stale (CLAUDE.md "Persistent-sink stale
// behavior: on content_hash or path mismatch, mark stale in manifest.json.
// Do not auto-regenerate."). Color (amber) is paired with the literal word
// "STALE" so color is never the sole signal (a11y rule from the plan).
//
// Renders nothing when the manifest is fresh — taking visual space for a
// non-event would teach the user to ignore the badge.
export function StaleBadge({ manifest }: { manifest: Manifest | null }) {
  if (!manifest?.stale) return null
  const reason = manifest.stale_reason ?? 'reason not recorded'
  return (
    <span
      className="badge badge-stale"
      role="status"
      title={`Manifest stale: ${reason}`}
      data-testid="stale-badge"
    >
      <span aria-hidden="true">!</span> STALE
    </span>
  )
}
