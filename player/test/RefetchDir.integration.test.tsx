import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { cleanup, render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import App from '../src/App.tsx'
import planFixture from '../public/fixtures/sample/plan.json' with { type: 'json' }
import manifestFixture from '../public/fixtures/sample/manifest.json' with { type: 'json' }
import type { EscalateSuccess } from '../src/types/manifest.ts'

// #62 regression suite: the post-escalate re-fetch must resolve against the LIVE
// escalated dir over the server's GET /artifact route — NOT the hardwired
// fixture origin. These drive the full App so the live-ref wiring (dirRef +
// serverModeRef) is exercised end-to-end, not just the pure resolver.

const SOURCE_MD = Array.from({ length: 50 }, (_, i) => `line ${i + 1}`).join('\n')

function patchedSuccess(over: Partial<EscalateSuccess> = {}): EscalateSuccess {
  return {
    block: {
      id: 'b2',
      order: 2,
      class: 'prose',
      level: 3,
      status: 'voiced',
      source_map: { kind: 'line_range', start_line: 3, end_line: 3 },
      segments: [{ id: 's2', kind: 'speech', text: 'A fuller, escalated reading of block two.' }],
      provenance: { voiced_by: 'mcp_sampling', deterministic: false, level_asked: 3 },
    },
    timing: { block_id: 'b2', start_ms: 150, end_ms: 900, audio_ref: 'audio.wav' },
    audio_ref: 'audio.wav',
    ...over,
  }
}

interface StubOpts {
  healthOk?: boolean
  // audioOk: false → audio.wav re-fetch returns non-200 (stale audio path).
  audioOk?: boolean
}

function makeStub(opts: StubOpts = {}) {
  const healthOk = opts.healthOk ?? true
  const audioOk = opts.audioOk ?? true
  const calls: { url: string; method: string }[] = []
  const ok = (body: BodyInit, init?: ResponseInit) => new Response(body, { status: 200, ...init })
  const fn = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
    const url =
      typeof input === 'string' ? input : input instanceof URL ? input.toString() : (input as Request).url
    const method = init?.method ?? 'GET'
    calls.push({ url, method })

    if (url.includes('/healthz')) {
      return healthOk ? ok(JSON.stringify({ status: 'ok' })) : new Response('down', { status: 503 })
    }
    if (url.includes('/escalate') && method === 'POST') {
      return new Response(JSON.stringify(patchedSuccess()), {
        status: 200,
        headers: { 'Content-Type': 'application/json' },
      })
    }
    if (url.includes('plan.json')) return ok(JSON.stringify(planFixture))
    if (url.includes('manifest.json')) return ok(JSON.stringify(manifestFixture))
    if (url.includes('source.md')) return ok(SOURCE_MD)
    if (url.includes('audio.wav')) {
      // Only the post-escalate re-fetch (the #62 /artifact route) fails when
      // audioOk is false — the initial fixture load over FIXTURE_BASE must still
      // succeed so the App mounts.
      if (!audioOk && url.includes('/artifact')) return new Response('boom', { status: 500 })
      return ok(new Uint8Array([0x52, 0x49, 0x46, 0x46]).buffer)
    }
    return new Response('not found', { status: 404 })
  })
  return { fn, calls }
}

beforeEach(() => {
  Object.defineProperty(window.HTMLMediaElement.prototype, 'play', {
    configurable: true,
    value: vi.fn().mockResolvedValue(undefined),
  })
  Object.defineProperty(window.HTMLMediaElement.prototype, 'pause', {
    configurable: true,
    value: vi.fn(),
  })
  Object.defineProperty(navigator, 'clipboard', {
    configurable: true,
    value: { writeText: vi.fn().mockResolvedValue(undefined) },
  })
})

afterEach(() => {
  cleanup()
  vi.restoreAllMocks()
  vi.unstubAllGlobals()
})

async function renderServerMode(stub: ReturnType<typeof makeStub>, dir = '/abs/out') {
  vi.stubGlobal('fetch', stub.fn)
  const user = userEvent.setup()
  render(<App />)
  await waitFor(() => screen.getByTestId('block-list'))
  await waitFor(() =>
    expect(screen.getByTestId('server-mode-indicator')).toHaveAttribute('data-server-mode', 'server'),
  )
  if (dir) {
    const dirInput = screen.getByTestId('escalate-dir-input')
    await user.clear(dirInput)
    await user.type(dirInput, dir)
  }
  return user
}

function rowFor(blockId: string): HTMLElement {
  const el = document.querySelector(`[data-block-id="${blockId}"]`)
  if (!el) throw new Error(`row ${blockId} not found`)
  return el as HTMLElement
}

describe('#62 re-fetch resolves against the live escalated dir', () => {
  it('server mode: manifest + audio re-fetch hit /artifact for the user dir (not FIXTURE_BASE)', async () => {
    const stub = makeStub()
    // Make b2 the playing block so the audio re-fetch fires too.
    const user = await renderServerMode(stub, '/abs/out')
    await user.click(screen.getByRole('button', { name: 'Seek to block b2' }))
    await waitFor(() => expect(rowFor('b2').getAttribute('aria-current')).toBe('true'))

    await user.click(screen.getByTestId('escalate-b2-L3'))
    await waitFor(() => expect(rowFor('b2').getAttribute('data-block-status')).toBe('voiced'))

    // Post-escalate manifest re-fetch went to /artifact?...&name=manifest.json.
    const manifestRefetch = stub.calls.find(
      (c) => c.method === 'GET' && c.url.includes('/artifact') && c.url.includes('name=manifest.json'),
    )
    expect(manifestRefetch, 'manifest re-fetch must use /artifact').toBeTruthy()
    expect(manifestRefetch!.url).toContain('dir=%2Fabs%2Fout')

    // Post-escalate audio re-fetch went to /artifact?...&name=audio.wav.
    const audioRefetch = stub.calls.find(
      (c) => c.method === 'GET' && c.url.includes('/artifact') && c.url.includes('name=audio.wav'),
    )
    expect(audioRefetch, 'audio re-fetch must use /artifact').toBeTruthy()
    expect(audioRefetch!.url).toContain('dir=%2Fabs%2Fout')

    // No post-escalate re-fetch leaked to the fixture origin.
    const escIdx = stub.calls.findIndex((c) => c.url.includes('/escalate') && c.method === 'POST')
    const afterEscalate = stub.calls.slice(escIdx + 1)
    const leakedToFixture = afterEscalate.some(
      (c) => c.url.includes('/fixtures/sample/manifest.json') || c.url.includes('/fixtures/sample/audio.wav'),
    )
    expect(leakedToFixture, 're-fetch must NOT hit the fixture origin in server mode').toBe(false)
  })

  it('STALE-DIR: changing dir via TopBar after render targets the NEW dir on re-fetch', async () => {
    const stub = makeStub()
    const user = await renderServerMode(stub, '/first/dir')

    // Change dir AFTER initial render (the live-ref regression — dirRef must be
    // in the effect-synced set, or the re-fetch would target /first/dir).
    const dirInput = screen.getByTestId('escalate-dir-input')
    await user.clear(dirInput)
    await user.type(dirInput, '/second/dir')

    await user.click(screen.getByTestId('escalate-b2-L3'))
    await waitFor(() => expect(rowFor('b2').getAttribute('data-block-status')).toBe('voiced'))

    const manifestRefetch = stub.calls.find(
      (c) => c.method === 'GET' && c.url.includes('/artifact') && c.url.includes('name=manifest.json'),
    )
    expect(manifestRefetch).toBeTruthy()
    expect(manifestRefetch!.url).toContain('dir=%2Fsecond%2Fdir')
    expect(manifestRefetch!.url).not.toContain('dir=%2Ffirst%2Fdir')
  })

  it('audio re-fetch NON-OK → patch retained + staleDownstream flagged (no rollback)', async () => {
    // The playing block is patched, audio.wav re-fetch fails (500). The patch is
    // kept (b2 → L3/voiced) and the row surfaces staleDownstream.
    const stub = makeStub({ audioOk: false })
    const user = await renderServerMode(stub, '/abs/out')
    await user.click(screen.getByRole('button', { name: 'Seek to block b2' }))
    await waitFor(() => expect(rowFor('b2').getAttribute('aria-current')).toBe('true'))

    await user.click(screen.getByTestId('escalate-b2-L3'))
    await waitFor(() => expect(screen.getByTestId('escalate-stale-b2')).toBeInTheDocument())

    const row = rowFor('b2')
    expect(row.getAttribute('data-block-status')).toBe('voiced')
    expect(within(row).getByText('L3')).toBeInTheDocument()
  })
})

describe('#62 fixture mode keeps FIXTURE_BASE', () => {
  it('fixture mode never builds an /artifact URL', async () => {
    const stub = makeStub({ healthOk: false })
    vi.stubGlobal('fetch', stub.fn)
    render(<App />)
    await waitFor(() => screen.getByTestId('block-list'))
    await waitFor(() =>
      expect(screen.getByTestId('server-mode-indicator')).toHaveAttribute('data-server-mode', 'fixture'),
    )
    // The initial fixture load uses FIXTURE_BASE and no /artifact route is ever hit.
    const hitArtifact = stub.calls.some((c) => c.url.includes('/artifact'))
    expect(hitArtifact).toBe(false)
    const hitFixture = stub.calls.some((c) => c.url.includes('/fixtures/sample/manifest.json'))
    expect(hitFixture).toBe(true)
  })
})
