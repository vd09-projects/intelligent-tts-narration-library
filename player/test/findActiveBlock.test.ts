import { describe, expect, it } from 'vitest'
import { findActiveBlock } from '../src/lib/findActiveBlock.ts'
import type { ManifestBlock } from '../src/types/manifest.ts'

const blocks: ManifestBlock[] = [
  { id: 'a', class: 'heading', level: 1, status: 'voiced', start_ms: 0, end_ms: 100 },
  { id: 'b', class: 'prose', level: 1, status: 'voiced', start_ms: 100, end_ms: 250 },
  // Gap from 250 to 300 — inter-block silence.
  { id: 'c', class: 'code', level: 1, status: 'voiced', start_ms: 300, end_ms: 500 },
]

describe('findActiveBlock', () => {
  it.each([
    ['empty list returns null', [] as ManifestBlock[], 100, null],
    ['before block 0 returns null', blocks, -5, null],
    ['exactly at start_ms of block 0', blocks, 0, 'a'],
    ['middle of block 0', blocks, 42, 'a'],
    ['boundary tMs=100 lands on block b (half-open)', blocks, 100, 'b'],
    ['middle of block b', blocks, 175, 'b'],
    ['inter-block gap snaps forward to next block', blocks, 275, 'c'],
    ['middle of block c', blocks, 400, 'c'],
    ['at end_ms of last block returns null (post-roll)', blocks, 500, null],
    ['far past last block returns null', blocks, 5000, null],
  ])('%s', (_label, input, tMs, expected) => {
    expect(findActiveBlock(input, tMs)).toBe(expected)
  })

  it('binary search visits log-n blocks (smoke / property check)', () => {
    const many: ManifestBlock[] = []
    for (let i = 0; i < 200; i++) {
      many.push({
        id: `b${i}`,
        class: 'prose',
        level: 1,
        status: 'voiced',
        start_ms: i * 100,
        end_ms: i * 100 + 100,
      })
    }
    expect(findActiveBlock(many, 0)).toBe('b0')
    expect(findActiveBlock(many, 9950)).toBe('b99')
    expect(findActiveBlock(many, 199 * 100 + 50)).toBe('b199')
    expect(findActiveBlock(many, 200 * 100)).toBe(null)
  })
})
