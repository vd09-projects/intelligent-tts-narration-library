import { afterEach, describe, expect, it, vi } from 'vitest'
import { checkHealth, postEscalate } from '../src/lib/escalateClient.ts'
import type { Block } from '../src/types/plan.ts'
import type { EscalateSuccess } from '../src/types/manifest.ts'

const BASE = 'http://127.0.0.1:8080'

function mkBlock(over: Partial<Block> = {}): Block {
  return {
    id: 'b2',
    order: 2,
    class: 'prose',
    level: 3,
    status: 'voiced',
    source_map: { kind: 'line_range', start_line: 1, end_line: 4 },
    provenance: { voiced_by: 'planner', deterministic: true },
    ...over,
  }
}

function jsonResponse(status: number, body: unknown): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'Content-Type': 'application/json' },
  })
}

afterEach(() => {
  vi.restoreAllMocks()
  vi.unstubAllGlobals()
})

describe('checkHealth', () => {
  it('returns true on 200 {status:"ok"}', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => jsonResponse(200, { status: 'ok' })))
    await expect(checkHealth(BASE)).resolves.toBe(true)
  })

  it('returns false on a non-200', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => jsonResponse(500, { status: 'down' })))
    await expect(checkHealth(BASE)).resolves.toBe(false)
  })

  it('returns false when fetch rejects (server not running)', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => { throw new TypeError('Failed to fetch') }))
    await expect(checkHealth(BASE)).resolves.toBe(false)
  })

  it('returns false on a 200 whose body is not {status:"ok"}', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => jsonResponse(200, { status: 'weird' })))
    await expect(checkHealth(BASE)).resolves.toBe(false)
  })
})

describe('postEscalate — success shapes', () => {
  it('200 voiced/degraded → {kind:"patched"} with block+timing+audioRef', async () => {
    const body: EscalateSuccess = {
      block: mkBlock(),
      timing: { block_id: 'b2', start_ms: 150, end_ms: 600, audio_ref: 'audio.wav' },
      audio_ref: 'audio.wav',
    }
    vi.stubGlobal('fetch', vi.fn(async () => jsonResponse(200, body)))
    const r = await postEscalate(BASE, { dir: '/out', blockId: 'b2', level: 3 })
    expect(r.kind).toBe('patched')
    if (r.kind === 'patched') {
      expect(r.block.id).toBe('b2')
      expect(r.timing.end_ms).toBe(600)
      expect(r.audioRef).toBe('audio.wav')
    }
  })

  it('200 refused → {kind:"refused"} with block+refusal, no timing', async () => {
    const body: EscalateSuccess = {
      block: mkBlock({ status: 'refused' }),
      refusal: {
        reason: 'no_intelligence_available',
        message: 'No intelligence available to summarize.',
        spoken: true,
        source_map: { kind: 'line_range', start_line: 1, end_line: 4 },
      },
    }
    vi.stubGlobal('fetch', vi.fn(async () => jsonResponse(200, body)))
    const r = await postEscalate(BASE, { dir: '/out', blockId: 'b2', level: 3 })
    expect(r.kind).toBe('refused')
    if (r.kind === 'refused') {
      expect(r.refusal.reason).toBe('no_intelligence_available')
    }
  })

  it('sends snake_case body {dir, block_id, level}', async () => {
    const fetchMock = vi.fn(async () =>
      jsonResponse(200, {
        block: mkBlock(),
        timing: { block_id: 'b2', start_ms: 0, end_ms: 1, audio_ref: 'audio.wav' },
        audio_ref: 'audio.wav',
      } satisfies EscalateSuccess),
    )
    vi.stubGlobal('fetch', fetchMock)
    await postEscalate(BASE, { dir: '/abs/out', blockId: 'b2', level: 2 })
    const call = fetchMock.mock.calls[0] as unknown as [string, RequestInit | undefined]
    expect(JSON.parse(String(call[1]?.body))).toEqual({ dir: '/abs/out', block_id: 'b2', level: 2 })
  })
})

describe('postEscalate — error shapes (server envelope)', () => {
  const errorCases: Array<{ status: number; reason: string }> = [
    { status: 400, reason: 'invalid_level' },
    { status: 400, reason: 'source_not_found' },
    { status: 404, reason: 'source_not_found' },
    { status: 405, reason: 'method_not_allowed' },
    { status: 409, reason: 'stale_patch' },
    { status: 409, reason: 'content_hash_mismatch' },
    { status: 409, reason: 'unknown_block' },
    { status: 500, reason: 'internal' },
  ]

  it.each(errorCases)(
    '$status $reason → {kind:"error"} carrying reason+message+status',
    async ({ status, reason }) => {
      vi.stubGlobal(
        'fetch',
        vi.fn(async () => jsonResponse(status, { reason, message: `${reason} happened` })),
      )
      const r = await postEscalate(BASE, { dir: '/out', blockId: 'b2', level: 3 })
      expect(r.kind).toBe('error')
      if (r.kind === 'error') {
        expect(r.reason).toBe(reason)
        expect(r.message).toBe(`${reason} happened`)
        expect(r.status).toBe(status)
      }
    },
  )

  it('UNKNOWN reason token → still {kind:"error"} (no crash; default branch)', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn(async () =>
        jsonResponse(409, { reason: 'some_future_token', message: 'newly added server reason' }),
      ),
    )
    const r = await postEscalate(BASE, { dir: '/out', blockId: 'b2', level: 3 })
    expect(r.kind).toBe('error')
    if (r.kind === 'error') {
      expect(r.reason).toBe('some_future_token')
      expect(r.message).toBe('newly added server reason')
    }
  })

  it('non-2xx with no usable body → synthesized internal error', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => new Response('<html>502</html>', { status: 502 })))
    const r = await postEscalate(BASE, { dir: '/out', blockId: 'b2', level: 3 })
    expect(r.kind).toBe('error')
    if (r.kind === 'error') {
      expect(r.reason).toBe('internal')
      expect(r.status).toBe(502)
    }
  })
})

describe('postEscalate — transport faults map to unreachable', () => {
  it('fetch reject (network down / CORS) → {kind:"error", reason:"unreachable"}', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => { throw new TypeError('Failed to fetch') }))
    const r = await postEscalate(BASE, { dir: '/out', blockId: 'b2', level: 3 })
    expect(r.kind).toBe('error')
    if (r.kind === 'error') expect(r.reason).toBe('unreachable')
  })

  it('abort/timeout → {kind:"error", reason:"unreachable"}', async () => {
    // Simulate the AbortController firing: reject with an AbortError.
    vi.stubGlobal(
      'fetch',
      vi.fn(async (_url: string, init?: RequestInit) => {
        return await new Promise<Response>((_resolve, reject) => {
          init?.signal?.addEventListener('abort', () => {
            const err = new Error('aborted')
            err.name = 'AbortError'
            reject(err)
          })
        })
      }),
    )
    vi.useFakeTimers()
    const p = postEscalate(BASE, { dir: '/out', blockId: 'b2', level: 3 })
    await vi.advanceTimersByTimeAsync(200_000)
    const r = await p
    vi.useRealTimers()
    expect(r.kind).toBe('error')
    if (r.kind === 'error') expect(r.reason).toBe('unreachable')
  })

  it('200 with malformed JSON → readback_failed error (no crash)', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn(async () => new Response('{not json', { status: 200, headers: { 'Content-Type': 'application/json' } })),
    )
    const r = await postEscalate(BASE, { dir: '/out', blockId: 'b2', level: 3 })
    expect(r.kind).toBe('error')
    if (r.kind === 'error') expect(r.reason).toBe('readback_failed')
  })

  it('200 missing both refusal and timing → internal contract-violation error', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => jsonResponse(200, { block: mkBlock() })))
    const r = await postEscalate(BASE, { dir: '/out', blockId: 'b2', level: 3 })
    expect(r.kind).toBe('error')
    if (r.kind === 'error') expect(r.reason).toBe('internal')
  })
})
