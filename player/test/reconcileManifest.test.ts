import { describe, expect, it } from 'vitest'
import { reconcileManifest } from '../src/lib/reconcileManifest.ts'
import type { Manifest, ManifestBlock } from '../src/types/manifest.ts'

function mkBlock(over: Partial<ManifestBlock> & { id: string }): ManifestBlock {
  return {
    class: 'prose',
    level: 1,
    status: 'voiced',
    start_ms: 0,
    end_ms: 100,
    audio_ref: 'audio.wav#0-100',
    ...over,
  }
}

function mkManifest(blocks: ManifestBlock[]): Manifest {
  return {
    schema_version: 1,
    plan_schema_version: '1',
    source_kind: 'file',
    source_uri: 'docs/samples/sample.md',
    content_hash: 'sha256:x',
    audio_format: { sample_rate: 24000, channels: 1, encoding: 'pcm_s16le' },
    voice: 'af_bella',
    blocks,
    stale: false,
  }
}

describe('reconcileManifest', () => {
  it('returns a NEW top-level object even when nothing changed', () => {
    const prev = mkManifest([mkBlock({ id: 'b1' }), mkBlock({ id: 'b2' })])
    const next = mkManifest([mkBlock({ id: 'b1' }), mkBlock({ id: 'b2' })])
    const out = reconcileManifest(prev, next)
    expect(out).not.toBe(prev)
    expect(out).not.toBe(next)
  })

  it('reuses the PRIOR block reference for deep-equal (unchanged) blocks', () => {
    const prev = mkManifest([mkBlock({ id: 'b1' }), mkBlock({ id: 'b2' })])
    // next has structurally-equal but distinct block objects (a fresh fetch).
    const next = mkManifest([mkBlock({ id: 'b1' }), mkBlock({ id: 'b2' })])
    const out = reconcileManifest(prev, next)
    expect(out.blocks[0]).toBe(prev.blocks[0])
    expect(out.blocks[1]).toBe(prev.blocks[1])
  })

  it('substitutes the NEW block object for a changed block (timing moved)', () => {
    const prev = mkManifest([
      mkBlock({ id: 'b1' }),
      mkBlock({ id: 'b2', start_ms: 100, end_ms: 200 }),
    ])
    const next = mkManifest([
      mkBlock({ id: 'b1' }),
      // b2's downstream offsets shifted after the patch.
      mkBlock({ id: 'b2', start_ms: 100, end_ms: 350 }),
    ])
    const out = reconcileManifest(prev, next)
    expect(out.blocks[0]).toBe(prev.blocks[0]) // unchanged → same ref
    expect(out.blocks[1]).toBe(next.blocks[1]) // changed → new ref
    expect(out.blocks[1].end_ms).toBe(350)
  })

  it('detects a status/level change as changed (escalated block)', () => {
    const prev = mkManifest([mkBlock({ id: 'b1', level: 1, status: 'degraded' })])
    const next = mkManifest([mkBlock({ id: 'b1', level: 3, status: 'voiced' })])
    const out = reconcileManifest(prev, next)
    expect(out.blocks[0]).toBe(next.blocks[0])
    expect(out.blocks[0].level).toBe(3)
  })

  it('treats an audio_ref change as changed', () => {
    const prev = mkManifest([mkBlock({ id: 'b1', audio_ref: 'audio.wav#0-100' })])
    const next = mkManifest([mkBlock({ id: 'b1', audio_ref: 'audio.wav#0-250' })])
    const out = reconcileManifest(prev, next)
    expect(out.blocks[0]).toBe(next.blocks[0])
  })

  it('uses the new block for ids absent from prev (newly appeared)', () => {
    const prev = mkManifest([mkBlock({ id: 'b1' })])
    const next = mkManifest([mkBlock({ id: 'b1' }), mkBlock({ id: 'b2' })])
    const out = reconcileManifest(prev, next)
    expect(out.blocks[0]).toBe(prev.blocks[0])
    expect(out.blocks[1]).toBe(next.blocks[1])
  })
})
