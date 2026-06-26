import { afterEach, describe, expect, it, vi } from 'vitest'
import { getReadiness } from '../src/lib/readinessClient.ts'

const BASE = 'http://127.0.0.1:8080'
const DIR = '/abs/out'

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

describe('getReadiness — wire mapping (cmd/narrate-server /readiness)', () => {
  it('200 {status:"rendered"} → {kind:"rendered"}', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => jsonResponse(200, { status: 'rendered' })))
    await expect(getReadiness(BASE, DIR)).resolves.toEqual({ kind: 'rendered' })
  })

  it('200 {status:"rendering"} → {kind:"rendering"}', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => jsonResponse(200, { status: 'rendering' })))
    await expect(getReadiness(BASE, DIR)).resolves.toEqual({ kind: 'rendering' })
  })

  it('404 {reason:"no_out_dir"} → {kind:"failed", reason:"no_out_dir"}', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn(async () => jsonResponse(404, { reason: 'no_out_dir', message: 'output dir does not exist' })),
    )
    await expect(getReadiness(BASE, DIR)).resolves.toEqual({ kind: 'failed', reason: 'no_out_dir' })
  })

  it('sends the dir as a URL-encoded ?dir= query param', async () => {
    const fetchMock = vi.fn<(url: RequestInfo | URL) => Promise<Response>>(
      async () => jsonResponse(200, { status: 'rendering' }),
    )
    vi.stubGlobal('fetch', fetchMock)
    await getReadiness(BASE, '/abs/out dir')
    const url = String(fetchMock.mock.calls[0]?.[0])
    expect(url).toBe('http://127.0.0.1:8080/readiness?dir=%2Fabs%2Fout%20dir')
  })
})

describe('getReadiness — every transport/contract fault collapses to unreachable', () => {
  it('fetch reject (network down / CORS) → {kind:"unreachable"}', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => { throw new TypeError('Failed to fetch') }))
    await expect(getReadiness(BASE, DIR)).resolves.toEqual({ kind: 'unreachable' })
  })

  it('abort/timeout → {kind:"unreachable"}', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn(async (_url: string, init?: RequestInit) =>
        new Promise<Response>((_resolve, reject) => {
          init?.signal?.addEventListener('abort', () => {
            const err = new Error('aborted')
            err.name = 'AbortError'
            reject(err)
          })
        }),
      ),
    )
    vi.useFakeTimers()
    const p = getReadiness(BASE, DIR)
    await vi.advanceTimersByTimeAsync(5000)
    await expect(p).resolves.toEqual({ kind: 'unreachable' })
    vi.useRealTimers()
  })

  it('200 with malformed JSON → {kind:"unreachable"} (no crash)', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => new Response('{not json', { status: 200 })))
    await expect(getReadiness(BASE, DIR)).resolves.toEqual({ kind: 'unreachable' })
  })

  it('200 with an unrecognized status string → {kind:"unreachable"}', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => jsonResponse(200, { status: 'weird' })))
    await expect(getReadiness(BASE, DIR)).resolves.toEqual({ kind: 'unreachable' })
  })

  it('non-404 error status (e.g. 500) → {kind:"unreachable"}, NOT failed', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => jsonResponse(500, { reason: 'internal', message: 'boom' })))
    await expect(getReadiness(BASE, DIR)).resolves.toEqual({ kind: 'unreachable' })
  })

  it('404 with a non-no_out_dir reason → {kind:"unreachable"} (only no_out_dir is terminal)', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => jsonResponse(404, { reason: 'source_not_found', message: 'x' })))
    await expect(getReadiness(BASE, DIR)).resolves.toEqual({ kind: 'unreachable' })
  })
})
