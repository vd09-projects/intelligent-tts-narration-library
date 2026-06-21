import { describe, expect, it } from 'vitest'
import { escalateTargets } from '../src/lib/escalateTargets.ts'
import type { Class, Level } from '../src/types/plan.ts'

describe('escalateTargets', () => {
  const proseCases: Array<{ level: Level; up: Level[] }> = [
    { level: 1, up: [2, 3] },
    { level: 2, up: [3] },
    { level: 3, up: [] },
  ]

  it.each(proseCases)(
    'prose @ L%s offers the levels above it ($up)',
    ({ level, up }) => {
      const t = escalateTargets(level, 'prose')
      expect(t.up).toEqual(up)
      expect(t.canDowngrade).toBe(false)
    },
  )

  const structured: Class[] = [
    'code',
    'config',
    'table',
    'diagram_as_text',
    'heading',
    'list',
  ]

  it.each(structured)(
    'structured class %s offers no escalate targets at any level (deterministic-L1 rule)',
    (cls) => {
      for (const level of [1, 2, 3] as Level[]) {
        const t = escalateTargets(level, cls)
        expect(t.up).toEqual([])
        expect(t.canDowngrade).toBe(false)
      }
    },
  )

  it('non-structured non-prose class (example) escalates like prose', () => {
    expect(escalateTargets(1, 'example').up).toEqual([2, 3])
    expect(escalateTargets(2, 'example').up).toEqual([3])
  })

  it('unknown class escalates like prose (only structured classes are exempt)', () => {
    expect(escalateTargets(1, 'unknown').up).toEqual([2, 3])
  })

  it('canDowngrade is always false (downgrade-to-L1 out of scope)', () => {
    for (const level of [1, 2, 3] as Level[]) {
      expect(escalateTargets(level, 'prose').canDowngrade).toBe(false)
      expect(escalateTargets(level, 'code').canDowngrade).toBe(false)
    }
  })
})
