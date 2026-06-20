import type { Block, NarrationPlan } from '../types/plan.ts'
import type { ManifestBlock } from '../types/manifest.ts'
import { RefusalBadge } from './RefusalBadge.tsx'
import { EscalateCard } from './EscalateCard.tsx'
import { escalateCommand } from '../lib/escalateCommand.ts'

// BlockRow renders one row of the block list. Carries three concerns:
//   1. Click anywhere (the row) → seekToBlock (mouse-only shortcut).
//   2. The "Seek" button is keyboard-focusable and does the same.
//   3. The "Escalate L3" button toggles an inline EscalateCard.
//
// aria-current="true" when this row is the active playback block. The row
// also gets a class for the visual highlight, but the screen-reader cue is
// the aria attribute.
//
// Hidden when status === "refused": the Escalate button. Refused blocks
// have nothing yet to escalate to.

export interface BlockRowProps {
  block: Block
  manifestBlock: ManifestBlock
  plan: NarrationPlan
  isActive: boolean
  isSourceCursor: boolean
  isEscalated: boolean
  onSeek: () => void
  onToggleEscalate: () => void
  onDismissEscalate: () => void
}

function spokenSummary(block: Block): string {
  if (block.refusal) {
    return block.refusal.message
  }
  if (block.segments && block.segments.length > 0) {
    return block.segments
      .filter((s) => s.kind === 'speech' && typeof s.text === 'string')
      .map((s) => s.text)
      .join(' ')
  }
  return block.source_map.raw_excerpt ?? '(no excerpt)'
}

export function BlockRow({
  block,
  manifestBlock,
  plan,
  isActive,
  isSourceCursor,
  isEscalated,
  onSeek,
  onToggleEscalate,
  onDismissEscalate,
}: BlockRowProps) {
  const refused = block.status === 'refused'
  const className =
    'block-row' +
    (isActive ? ' active' : '') +
    (isSourceCursor ? ' cursor' : '') +
    (refused ? ' refused' : '')

  const command = escalateCommand(block, plan)
  const spoken = spokenSummary(block)
  const truncated = spoken.length > 280 ? spoken.slice(0, 280) + '…' : spoken

  return (
    <li
      className={className}
      role="listitem"
      aria-current={isActive ? 'true' : undefined}
      data-block-id={block.id}
      data-block-status={block.status}
      data-block-class={block.class}
    >
      <div
        className="block-row-clickable"
        onClick={onSeek}
        // The clickable wrapper is a mouse convenience only. Keyboard users
        // use the explicit "Seek" button below; the wrapper has no tabindex
        // so it does not double-trip in tab order (a11y rule).
        aria-hidden="true"
      >
        <div className="block-meta">
          <span className="badge badge-class">{block.class}</span>
          <span className="badge badge-level">L{block.level}</span>
          <span className="badge badge-status" data-status={block.status}>
            {block.status}
          </span>
          <span className="muted">
            {manifestBlock.start_ms}–{manifestBlock.end_ms} ms
          </span>
        </div>
        {refused && block.refusal ? (
          <RefusalBadge refusal={block.refusal} />
        ) : (
          <p className="block-text">{truncated}</p>
        )}
      </div>

      <div className="block-actions">
        <button type="button" onClick={onSeek} aria-label={`Seek to block ${block.id}`}>
          Seek
        </button>
        {!refused && (
          <button
            type="button"
            onClick={onToggleEscalate}
            aria-expanded={isEscalated}
            aria-controls={`escalate-${block.id}`}
          >
            Escalate L3
          </button>
        )}
      </div>

      {isEscalated && !refused && (
        <div id={`escalate-${block.id}`}>
          <EscalateCard command={command} onDismiss={onDismissEscalate} />
        </div>
      )}
    </li>
  )
}
