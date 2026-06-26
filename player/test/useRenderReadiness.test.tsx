import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { act, renderHook } from '@testing-library/react'
import { useRenderReadiness } from '../src/hooks/useRenderReadiness.ts'

// These tests pin the bounded poll loop + give-up state machine (#76 AC3/AC4)
// with fake timers: the dead-server fast path and the absolute attempt cap both
// resolve to the single DEFINED render_failed terminal — no perpetual spinner.

const BASE = 'http://127.0.0.1:8080'
const DIR = '/abs/out'

// The config defaults the tests assume (readinessConfig.ts, no env overrides).
const POLL_INTERVAL_MS = 1000
const UNREACHABLE_FASTFAIL = 8
const MAX_POLL_ATTEMPTS = 120

function jsonResponse(status: number, body: unknown): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'Content-Type': 'application/json' },
  })
}

// flush advances fake timers by ms and drains the microtasks the poll loop
// chains behind each fetch, wrapped in act() so React state settles.
async function flush(ms: number): Promise<void> {
  await act(async () => {
    await vi.advanceTimersByTimeAsync(ms)
  })
}

beforeEach(() => {
  vi.useFakeTimers()
})

afterEach(() => {
  vi.useRealTimers()
  vi.restoreAllMocks()
  vi.unstubAllGlobals()
})

describe('useRenderReadiness — terminal resolution', () => {
  it('enabled=false stays idle and never polls', async () => {
    const fetchMock = vi.fn(async () => jsonResponse(200, { status: 'rendering' }))
    vi.stubGlobal('fetch', fetchMock)
    const { result } = renderHook(() =>
      useRenderReadiness({ baseUrl: BASE, dir: DIR, enabled: false }),
    )
    await flush(5000)
    expect(result.current.phase).toBe('idle')
    expect(fetchMock).not.toHaveBeenCalled()
  })

  it('rendering → phase "rendering" (spinner), keeps polling within the bound', async () => {
    const fetchMock = vi.fn(async () => jsonResponse(200, { status: 'rendering' }))
    vi.stubGlobal('fetch', fetchMock)
    const { result } = renderHook(() =>
      useRenderReadiness({ baseUrl: BASE, dir: DIR, enabled: true }),
    )
    await flush(0)
    expect(result.current.phase).toBe('rendering')
    expect(result.current.attempts).toBe(1)
    // A second interval elapses → another poll, still rendering.
    await flush(POLL_INTERVAL_MS)
    expect(result.current.phase).toBe('rendering')
    expect(result.current.attempts).toBe(2)
  })

  it('rendered → phase "rendered" (terminal); loop stops (no further fetch)', async () => {
    const fetchMock = vi.fn(async () => jsonResponse(200, { status: 'rendered' }))
    vi.stubGlobal('fetch', fetchMock)
    const { result } = renderHook(() =>
      useRenderReadiness({ baseUrl: BASE, dir: DIR, enabled: true }),
    )
    await flush(0)
    expect(result.current.phase).toBe('rendered')
    const callsAtTerminal = fetchMock.mock.calls.length
    // No further polling after the terminal phase.
    await flush(POLL_INTERVAL_MS * 5)
    expect(fetchMock.mock.calls.length).toBe(callsAtTerminal)
  })

  it('server no_out_dir token → immediate render_failed (terminal)', async () => {
    const fetchMock = vi.fn(async () => jsonResponse(404, { reason: 'no_out_dir', message: 'gone' }))
    vi.stubGlobal('fetch', fetchMock)
    const { result } = renderHook(() =>
      useRenderReadiness({ baseUrl: BASE, dir: DIR, enabled: true }),
    )
    await flush(0)
    expect(result.current.phase).toBe('failed')
    expect(result.current.reason).toBe('no_out_dir')
    expect(fetchMock).toHaveBeenCalledTimes(1)
  })

  it('dead server from poll #1 → render_failed at UNREACHABLE_FASTFAIL consecutive rejects', async () => {
    const fetchMock = vi.fn(async () => { throw new TypeError('Failed to fetch') })
    vi.stubGlobal('fetch', fetchMock)
    const { result } = renderHook(() =>
      useRenderReadiness({ baseUrl: BASE, dir: DIR, enabled: true }),
    )
    // First (immediate) poll + 7 scheduled polls = 8 consecutive unreachables.
    await flush(POLL_INTERVAL_MS * UNREACHABLE_FASTFAIL)
    expect(result.current.phase).toBe('failed')
    expect(result.current.reason).toBe('server_unreachable')
    expect(fetchMock).toHaveBeenCalledTimes(UNREACHABLE_FASTFAIL)
    // Terminal: no further polling.
    await flush(POLL_INTERVAL_MS * 5)
    expect(fetchMock).toHaveBeenCalledTimes(UNREACHABLE_FASTFAIL)
  })

  it('never-completing render → render_failed at MAX_POLL_ATTEMPTS (bound expiry)', async () => {
    const fetchMock = vi.fn(async () => jsonResponse(200, { status: 'rendering' }))
    vi.stubGlobal('fetch', fetchMock)
    const { result } = renderHook(() =>
      useRenderReadiness({ baseUrl: BASE, dir: DIR, enabled: true }),
    )
    await flush(POLL_INTERVAL_MS * MAX_POLL_ATTEMPTS)
    expect(result.current.phase).toBe('failed')
    expect(result.current.reason).toBe('max_poll_attempts')
    expect(result.current.attempts).toBe(MAX_POLL_ATTEMPTS)
    expect(fetchMock).toHaveBeenCalledTimes(MAX_POLL_ATTEMPTS)
  })

  it('stops polling on unmount (timer torn down)', async () => {
    const fetchMock = vi.fn(async () => jsonResponse(200, { status: 'rendering' }))
    vi.stubGlobal('fetch', fetchMock)
    const { unmount } = renderHook(() =>
      useRenderReadiness({ baseUrl: BASE, dir: DIR, enabled: true }),
    )
    await flush(0)
    const callsBefore = fetchMock.mock.calls.length
    expect(callsBefore).toBe(1)
    unmount()
    await flush(POLL_INTERVAL_MS * 5)
    expect(fetchMock.mock.calls.length).toBe(callsBefore)
  })
})
