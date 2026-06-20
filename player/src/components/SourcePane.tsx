import { useMemo } from 'react'
import type { ManifestBlock } from '../types/manifest.ts'
import type { Block, NarrationPlan } from '../types/plan.ts'

// SourcePane renders the raw source alongside the block list (plan D1).
//
// Behavior:
//   - If source !== null, render source.text as a <pre> with one <span> per
//     block covering its [start_line, end_line] range. Mouse hover or click
//     on a span sets sourceCursorBlockId (the App owns that state).
//   - If source === null, render an info notice + a per-block list of
//     raw_excerpt values as the fallback (D1 honesty rule: never fake
//     coverage we don't have).
//   - The active playback block gets a class for visual underline. Color
//     is paired with the active-block aria-label on the BlockRow side, so
//     no signal is color-only.

export interface SourcePaneProps {
  plan: NarrationPlan
  manifestBlocks: ManifestBlock[]
  source: string | null
  activeBlockId: string | null
  onCursorChange: (blockId: string | null) => void
}

interface SourceSpan {
  blockId: string
  className: string
  text: string
}

function buildSpans(source: string, plan: NarrationPlan): SourceSpan[] {
  const lines = source.split('\n')
  // Sort blocks by start_line; only blocks with a line-range source map can
  // be highlighted in the source pane. Others (image rects, raw_text spans)
  // still appear in the block list — they just don't paint here.
  const lineBlocks = [...plan.blocks]
    .filter((b) => typeof b.source_map.start_line === 'number' && typeof b.source_map.end_line === 'number')
    .sort((a, b) => (a.source_map.start_line ?? 0) - (b.source_map.start_line ?? 0))

  const spans: SourceSpan[] = []
  let cursor = 1 // line numbers are 1-indexed in plan.source_map
  for (const block of lineBlocks) {
    const start = block.source_map.start_line ?? 0
    const end = block.source_map.end_line ?? start
    // Emit any inter-block lines as a non-block span so we don't drop text.
    if (start > cursor) {
      spans.push({
        blockId: '',
        className: 'source-gap',
        text: lines.slice(cursor - 1, start - 1).join('\n') + (start > cursor ? '\n' : ''),
      })
    }
    spans.push({
      blockId: block.id,
      className: 'source-block',
      text: lines.slice(start - 1, end).join('\n') + '\n',
    })
    cursor = end + 1
  }
  if (cursor <= lines.length) {
    spans.push({
      blockId: '',
      className: 'source-gap',
      text: lines.slice(cursor - 1).join('\n'),
    })
  }
  return spans
}

export function SourcePane({
  plan,
  manifestBlocks,
  source,
  activeBlockId,
  onCursorChange,
}: SourcePaneProps) {
  const spans = useMemo(
    () => (source ? buildSpans(source, plan) : []),
    [source, plan],
  )
  const byId = new Map(manifestBlocks.map((m) => [m.id, m]))

  if (source === null) {
    return (
      <aside className="source-pane source-fallback" aria-label="Source (fallback)">
        <div className="info-banner" role="status">
          source.md not provided alongside this directory. Falling back to per-block raw_excerpt; line-number cues are advisory.
        </div>
        <ul className="source-fallback-list">
          {[...plan.blocks]
            .sort((a, b) => a.order - b.order)
            .map((block: Block) => {
              const mb = byId.get(block.id)
              if (!mb) return null
              const isActive = activeBlockId === block.id
              return (
                <li
                  key={block.id}
                  className={'source-fallback-row' + (isActive ? ' active' : '')}
                  data-block-id={block.id}
                  onMouseEnter={() => onCursorChange(block.id)}
                  onMouseLeave={() => onCursorChange(null)}
                  onClick={() => onCursorChange(block.id)}
                >
                  <code className="muted">#{block.id}</code>{' '}
                  <span>{block.source_map.raw_excerpt ?? '(no excerpt)'}</span>
                </li>
              )
            })}
        </ul>
      </aside>
    )
  }

  return (
    <aside className="source-pane" aria-label="Source">
      <pre
        className="source-pre"
        data-testid="source-pre"
        onMouseLeave={() => onCursorChange(null)}
      >
        {spans.map((s, i) =>
          s.blockId ? (
            <span
              key={`${s.blockId}-${i}`}
              data-block-id={s.blockId}
              className={
                s.className +
                (activeBlockId === s.blockId ? ' active' : '')
              }
              onMouseEnter={() => onCursorChange(s.blockId)}
              onClick={() => onCursorChange(s.blockId)}
            >
              {s.text}
            </span>
          ) : (
            <span key={`gap-${i}`} className={s.className}>
              {s.text}
            </span>
          ),
        )}
      </pre>
    </aside>
  )
}
