import { describe, expect, it } from 'vitest'
import { escalateCommand } from '../src/lib/escalateCommand.ts'
import type { Block, NarrationPlan } from '../src/types/plan.ts'

function mkBlock(id: string): Block {
  return {
    id,
    order: 1,
    class: 'prose',
    level: 1,
    status: 'voiced',
    source_map: { kind: 'line_range', start_line: 1, end_line: 1 },
    provenance: { voiced_by: 'planner', deterministic: true },
  }
}

function mkPlan(kind: 'file' | 'mcp_text' | 'raw_text', uri?: string): NarrationPlan {
  return {
    schema_version: '1',
    plan_id: 'TESTPLANID',
    created_at: '2026-06-20T00:00:00Z',
    source: { kind, uri, content_hash: 'sha256:x', adapter: 'file' },
    defaults: { level: 1, locale: 'en' },
    blocks: [],
  }
}

describe('escalateCommand', () => {
  it('file source emits the file uri', () => {
    const cmd = escalateCommand(mkBlock('b7'), mkPlan('file', 'docs/samples/sample.md'))
    expect(cmd).toBe('narrate --block b7 --level 3 --file docs/samples/sample.md')
  })

  it('mcp_text source uses <source> placeholder', () => {
    const cmd = escalateCommand(mkBlock('b3'), mkPlan('mcp_text'))
    expect(cmd).toBe('narrate --block b3 --level 3 --file <source>')
  })

  it('raw_text source uses <source> placeholder', () => {
    const cmd = escalateCommand(mkBlock('b1'), mkPlan('raw_text'))
    expect(cmd).toBe('narrate --block b1 --level 3 --file <source>')
  })

  it('file source with empty uri falls back to placeholder', () => {
    const cmd = escalateCommand(mkBlock('b4'), mkPlan('file', ''))
    expect(cmd).toBe('narrate --block b4 --level 3 --file <source>')
  })

  it('unknown source kind falls back to placeholder', () => {
    const cmd = escalateCommand(mkBlock('b9'), mkPlan('ocr_screenshot' as never))
    expect(cmd).toBe('narrate --block b9 --level 3 --file <source>')
  })
})
