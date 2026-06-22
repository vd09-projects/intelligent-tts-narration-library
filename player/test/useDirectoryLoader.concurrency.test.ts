import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { act, renderHook, waitFor } from '@testing-library/react'
import { useDirectoryLoader } from '../src/hooks/useDirectoryLoader.ts'

// #70 single-in-flight: two concurrent loadFromServerDir triggers must NOT race.
// Only one load runs; the second is dropped while the first is in flight, so the
// shared revokeRef is never raced and isLoading toggles cleanly.

const SERVER = 'http://127.0.0.1:8080'
const PLAN = { schema_version: '1', plan_id: 'p1', blocks: [] }
const MANIFEST = { schema_version: 1, voice: 'af_bella', blocks: [], stale: false }
const AUDIO = new Uint8Array([0x52, 0x49, 0x46, 0x46]).buffer

// A controllable fetch: audio.wav resolution is gated on `release()` so the
// first load is held mid-flight while we fire the second.
function gatedFetch() {
  let release!: () => void
  const gate = new Promise<void>((r) => {
    release = r
  })
  let audioCount = 0
  const ok = (b: BodyInit) => new Response(b, { status: 200 })
  const fn = vi.fn(async (input: RequestInfo | URL) => {
    const url = input.toString()
    if (url.includes('plan.json')) return ok(JSON.stringify(PLAN))
    if (url.includes('manifest.json')) return ok(JSON.stringify(MANIFEST))
    if (url.includes('source.md')) return ok('# source\n')
    if (url.includes('audio.wav')) {
      audioCount++
      await gate
      return ok(AUDIO)
    }
    return new Response('nf', { status: 404 })
  })
  return { fn, release, audioCount: () => audioCount }
}

let createCount = 0

beforeEach(() => {
  createCount = 0
  vi.spyOn(URL, 'createObjectURL').mockImplementation(() => `blob:url-${++createCount}`)
  vi.spyOn(URL, 'revokeObjectURL').mockImplementation(() => {})
})

afterEach(() => {
  vi.restoreAllMocks()
  vi.unstubAllGlobals()
})

describe('useDirectoryLoader — single in-flight load', () => {
  it('two concurrent loadFromServerDir triggers commit only one load', async () => {
    const stub = gatedFetch()
    vi.stubGlobal('fetch', stub.fn)

    const { result } = renderHook(() => useDirectoryLoader())

    // Fire two loads in the SAME tick, before the first resolves.
    let firstDone: Promise<void>
    let secondDone: Promise<void>
    act(() => {
      firstDone = result.current.loadFromServerDir('/abs/out', SERVER)
      secondDone = result.current.loadFromServerDir('/abs/out', SERVER)
    })

    // First is in flight → isLoading true. The second was dropped synchronously
    // by the in-flight guard, so only ONE audio fetch was ever issued.
    await waitFor(() => expect(result.current.isLoading).toBe(true))
    expect(stub.audioCount()).toBe(1)

    // Release the gate; both promises settle.
    act(() => {
      stub.release()
    })
    await act(async () => {
      await firstDone
      await secondDone
    })

    // Exactly one load committed; exactly one blob URL created (no raced second).
    expect(result.current.status).toBe('loaded')
    expect(result.current.data?.plan.plan_id).toBe('p1')
    expect(createCount).toBe(1)
    expect(result.current.isLoading).toBe(false)
  })

  it('isLoading is false again after a load completes, allowing a subsequent load', async () => {
    const stub = gatedFetch()
    vi.stubGlobal('fetch', stub.fn)
    const { result } = renderHook(() => useDirectoryLoader())

    let done: Promise<void>
    act(() => {
      done = result.current.loadFromServerDir('/abs/out', SERVER)
    })
    await waitFor(() => expect(result.current.isLoading).toBe(true))
    act(() => stub.release())
    await act(async () => {
      await done
    })
    expect(result.current.isLoading).toBe(false)

    // A second load is now permitted (guard cleared).
    const stub2 = gatedFetch()
    vi.stubGlobal('fetch', stub2.fn)
    let done2: Promise<void>
    act(() => {
      done2 = result.current.loadFromServerDir('/other', SERVER)
    })
    await waitFor(() => expect(stub2.audioCount()).toBe(1))
    act(() => stub2.release())
    await act(async () => {
      await done2
    })
    expect(result.current.status).toBe('loaded')
  })
})
