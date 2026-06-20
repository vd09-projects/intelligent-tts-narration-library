import type { ManifestBlock } from '../types/manifest.ts'
import type { NarrationPlan } from '../types/plan.ts'
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
export interface BlockListProps {
  plan: NarrationPlan
  manifestBlocks: ManifestBlock[]
  activeBlockId: string | null
  sourceCursorBlockId: string | null
  escalatedBlockId: string | null
  onSeek: (blockId: string) => void
  onToggleEscalate: (blockId: string) => void
  onDismissEscalate: () => void
}

export function BlockList({
  plan,
  manifestBlocks,
  activeBlockId,
  sourceCursorBlockId,
  escalatedBlockId,
  onSeek,
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
          onSeek={() => onSeek(block.id)}
          onToggleEscalate={() => onToggleEscalate(block.id)}
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
