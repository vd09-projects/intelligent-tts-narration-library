import { memo } from 'react'
import type { Block, Level, NarrationPlan } from '../types/plan.ts'
import type { ManifestBlock } from '../types/manifest.ts'
import type { ServerMode } from '../hooks/useServerMode.ts'
import type { RowState } from '../hooks/useEscalation.ts'
import { RefusalBadge } from './RefusalBadge.tsx'
import { EscalateCard } from './EscalateCard.tsx'
import { escalateCommand } from '../lib/escalateCommand.ts'
import { escalateTargets } from '../lib/escalateTargets.ts'

// BlockRow renders one row of the block list. Concerns:
//   1. Click anywhere (the row) → seekToBlock (mouse-only shortcut).
//   2. The "Seek" button is keyboard-focusable and does the same.
//   3. Escalation controls — mode-dependent (issue #50):
//        - server mode: live "Escalate L{n}" buttons (one per escalate target),
//          spinner + disabled while loading, inline error/refusal/stale notice.
//          Disabled-with-hint when the dir field is empty (Q2 gate).
//        - fixture mode: the legacy single toggle → EscalateCard (copy command).
//        - unknown mode: neutral — no escalation control yet (no flicker).
//
// Wrapped in React.memo so a patch to one row does NOT re-render its siblings
// (the parent passes stable callbacks + per-row rowState).
//
// aria-current="true" when this row is the active playback block.
// Hidden when status === "refused": escalation controls. Refused blocks have
// nothing to escalate to (they were not voiced at all).

export interface BlockRowProps {
  block: Block
  manifestBlock: ManifestBlock
  plan: NarrationPlan
  isActive: boolean
  isSourceCursor: boolean
  isEscalated: boolean
  serverMode: ServerMode
  dir: string
  rowState: RowState
  onSeek: (blockId: string) => void
  onEscalate: (blockId: string, level: Level) => void
  onToggleEscalate: (blockId: string) => void
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

function BlockRowImpl({
  block,
  manifestBlock,
  plan,
  isActive,
  isSourceCursor,
  isEscalated,
  serverMode,
  dir,
  rowState,
  onSeek,
  onEscalate,
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

  const loading = rowState.status === 'loading'
  const targets = escalateTargets(block.level, block.class)
  const dirMissing = serverMode === 'server' && dir.trim() === ''

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
        onClick={() => onSeek(block.id)}
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
        <button
          type="button"
          onClick={() => onSeek(block.id)}
          aria-label={`Seek to block ${block.id}`}
        >
          Seek
        </button>

        {/* Server mode: live per-level escalate buttons (prose only — structured
            classes have no targets per the deterministic-L1 rule). */}
        {serverMode === 'server' &&
          !refused &&
          targets.up.map((lvl) => (
            <button
              key={lvl}
              type="button"
              className="escalate-level-btn"
              disabled={loading || dirMissing}
              title={
                dirMissing
                  ? 'Enter the escalate output directory (top bar) to enable live escalation'
                  : `Re-narrate this block at L${lvl} via the escalate server`
              }
              data-testid={`escalate-${block.id}-L${lvl}`}
              onClick={() => onEscalate(block.id, lvl)}
            >
              {loading ? (
                <span className="escalate-spinner" aria-hidden="true" />
              ) : null}
              Escalate L{lvl}
            </button>
          ))}

        {/* Fixture mode: legacy single toggle → copy-command EscalateCard. */}
        {serverMode === 'fixture' && !refused && (
          <button
            type="button"
            onClick={() => onToggleEscalate(block.id)}
            aria-expanded={isEscalated}
            aria-controls={`escalate-${block.id}`}
          >
            Escalate L3
          </button>
        )}
      </div>

      {/* Inline, on-the-row status — never a toast/modal (honesty rule). */}
      {loading && (
        <p className="escalate-status muted" role="status" data-testid={`escalate-loading-${block.id}`}>
          Escalating…
        </p>
      )}
      {rowState.status === 'error' && rowState.error && (
        <p
          className="escalate-error"
          role="status"
          data-testid={`escalate-error-${block.id}`}
        >
          Escalate failed: {rowState.error.message}{' '}
          <code className="escalate-error-reason">{rowState.error.reason}</code>
        </p>
      )}
      {rowState.status === 'noop' && (
        <p
          className="escalate-noop muted"
          role="status"
          data-testid={`escalate-noop-${block.id}`}
        >
          No visible change — the block already reads the same at this level.
        </p>
      )}
      {rowState.staleDownstream && (
        <p
          className="escalate-stale muted"
          role="status"
          data-testid={`escalate-stale-${block.id}`}
        >
          Block updated, but downstream timings could not be refreshed. Reload the
          directory to resync later blocks.
        </p>
      )}

      {serverMode === 'fixture' && isEscalated && !refused && (
        <div id={`escalate-${block.id}`}>
          <EscalateCard command={command} onDismiss={onDismissEscalate} />
        </div>
      )}
    </li>
  )
}

export const BlockRow = memo(BlockRowImpl)
