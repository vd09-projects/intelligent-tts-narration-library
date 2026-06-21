import type { ManifestBlock } from '../types/manifest.ts'
import type { Level, NarrationPlan } from '../types/plan.ts'
import type { ServerMode } from '../hooks/useServerMode.ts'
import type { RowState } from '../hooks/useEscalation.ts'
import { BlockRow } from './BlockRow.tsx'

// BlockList renders all blocks in document order. role="list" +
// aria-label keep the structure announced to screen readers; the children
// inherit role="listitem" via BlockRow.
//
// We zip plan.blocks with manifest.blocks by id. Plan blocks not present in
// the manifest (sink wrote them but the renderer skipped them) are skipped
// — block-level sync is meaningless without timing. We do NOT show a
// silent error: the plan-level Diagnostic list is where unwanted-skip
// signals belong.
//
// Escalation props (issue #50) are threaded straight through to BlockRow; the
// per-row rowState lookup is passed as a function so only the escalating row's
// state changes (React.memo(BlockRow) keeps siblings from re-rendering).
export interface BlockListProps {
  plan: NarrationPlan
  manifestBlocks: ManifestBlock[]
  activeBlockId: string | null
  sourceCursorBlockId: string | null
  escalatedBlockId: string | null
  serverMode: ServerMode
  dir: string
  rowState: (blockId: string) => RowState
  onSeek: (blockId: string) => void
  onEscalate: (blockId: string, level: Level) => void
  onToggleEscalate: (blockId: string) => void
  onDismissEscalate: () => void
}

export function BlockList({
  plan,
  manifestBlocks,
  activeBlockId,
  sourceCursorBlockId,
  escalatedBlockId,
  serverMode,
  dir,
  rowState,
  onSeek,
  onEscalate,
  onToggleEscalate,
  onDismissEscalate,
}: BlockListProps) {
  const byId = new Map(manifestBlocks.map((m) => [m.id, m]))
  const rows = [...plan.blocks]
    .sort((a, b) => a.order - b.order)
    .map((block) => {
      const mb = byId.get(block.id)
      if (!mb) return null
      return (
        <BlockRow
          key={block.id}
          block={block}
          manifestBlock={mb}
          plan={plan}
          isActive={activeBlockId === block.id}
          isSourceCursor={sourceCursorBlockId === block.id}
          isEscalated={escalatedBlockId === block.id}
          serverMode={serverMode}
          dir={dir}
          rowState={rowState(block.id)}
          onSeek={onSeek}
          onEscalate={onEscalate}
          onToggleEscalate={onToggleEscalate}
          onDismissEscalate={onDismissEscalate}
        />
      )
    })

  return (
    <ul
      className="block-list"
      role="list"
      aria-label="Narration blocks"
      data-testid="block-list"
    >
      {rows}
    </ul>
  )
}
