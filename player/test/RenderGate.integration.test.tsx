import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { cleanup, render, screen, waitFor } from '@testing-library/react'
import App from '../src/App.tsx'
import planFixture from '../public/fixtures/sample/plan.json' with { type: 'json' }
import manifestFixture from '../public/fixtures/sample/manifest.json' with { type: 'json' }

// Companion-mode (#76) integration: with VITE_COMPANION_DIR set, the player
// drives the readiness handshake. A `rendered` triple — INCLUDING one that
// carries refused blocks — must LOAD the plan (refusal-is-data), never the
// render_failed banner. This is the AC3 "rendered-with-refusal loads, not
// failure" guarantee end-to-end through App + RenderGate + loadFromServer.

const COMPANION_DIR = '/abs/companion/out'

// readinessState lets a test flip what GET /readiness reports over time.
function makeCompanionStub(readinessStatus: () => { status?: number; body: unknown }) {
  const ok = (body: BodyInit) => new Response(body, { status: 200 })
  return vi.fn(async (input: RequestInfo | URL) => {
    const url =
      typeof input === 'string'
        ? input
        : input instanceof URL
          ? input.toString()
          : (input as Request).url

    if (url.includes('/healthz')) return ok(JSON.stringify({ status: 'ok' }))
    if (url.includes('/readiness')) {
      const r = readinessStatus()
      return new Response(JSON.stringify(r.body), {
        status: r.status ?? 200,
        headers: { 'Content-Type': 'application/json' },
      })
    }
    // The companion HTTP load + any fixture mount fetch both resolve from the
    // in-memory fixtures (active ignores the fixture in companion mode anyway).
    if (url.includes('plan.json')) return ok(JSON.stringify(planFixture))
    if (url.includes('manifest.json')) return ok(JSON.stringify(manifestFixture))
    if (url.includes('source.md')) return ok('line 1\nline 2')
    if (url.includes('audio.wav')) return ok(new Uint8Array([0x52, 0x49, 0x46, 0x46]).buffer)
    return new Response('not found', { status: 404 })
  })
}

beforeEach(() => {
  vi.stubEnv('VITE_COMPANION_DIR', COMPANION_DIR)
  Object.defineProperty(window.HTMLMediaElement.prototype, 'play', {
    configurable: true,
    value: vi.fn().mockResolvedValue(undefined),
  })
  Object.defineProperty(window.HTMLMediaElement.prototype, 'pause', {
    configurable: true,
    value: vi.fn(),
  })
})

afterEach(() => {
  cleanup()
  vi.unstubAllEnvs()
  vi.restoreAllMocks()
  vi.unstubAllGlobals()
})

describe('Companion mode — RenderGate three-state load', () => {
  it('rendered-with-refusal → plan loads (block list + refusal badges), NOT the failure banner', async () => {
    vi.stubGlobal('fetch', makeCompanionStub(() => ({ body: { status: 'rendered' } })))
    render(<App />)

    // The plan loads end-to-end: the block list renders, the refused blocks
    // surface their badges (refusal-is-data → a successful load).
    await waitFor(() => expect(screen.getByTestId('block-list')).toBeInTheDocument())
    const refused = screen.getAllByTestId('refusal-badge')
    expect(refused.length).toBe(2)
    // It is NOT the failure terminal.
    expect(screen.queryByTestId('render-gate-failed')).not.toBeInTheDocument()
    expect(screen.queryByTestId('render-gate-rendering')).not.toBeInTheDocument()
  })

  it('no_out_dir → defined render_failed banner with the make run-companion hint', async () => {
    vi.stubGlobal(
      'fetch',
      makeCompanionStub(() => ({ status: 404, body: { reason: 'no_out_dir', message: 'gone' } })),
    )
    render(<App />)

    await waitFor(() =>
      expect(screen.getByTestId('render-gate-failed')).toBeInTheDocument(),
    )
    const banner = screen.getByTestId('render-gate-failed')
    expect(banner.textContent).toMatch(/render_failed/)
    expect(banner.textContent).toMatch(/no_out_dir/)
    expect(banner.textContent).toMatch(/make run-companion/)
    // The plan never loaded.
    expect(screen.queryByTestId('block-list')).not.toBeInTheDocument()
  })

  it('rendered then the triple load throws → render_failed banner with a distinct load_failed reason (not no_out_dir)', async () => {
    // readiness says rendered, but the subsequent loadFromServerDir audio fetch
    // 500s → loadFromServerDir throws → companionError → the App surfaces the
    // failure banner with a `load_failed:` reason so this terminal is NOT
    // conflated with the no_out_dir (wrong-path) terminal. The one previously
    // unpinned terminal branch (Test Coverage suggestion #4).
    const ok = (body: BodyInit) => new Response(body, { status: 200 })
    const fetchStub = vi.fn(async (input: RequestInfo | URL) => {
      const url =
        typeof input === 'string'
          ? input
          : input instanceof URL
            ? input.toString()
            : (input as Request).url
      if (url.includes('/healthz')) return ok(JSON.stringify({ status: 'ok' }))
      if (url.includes('/readiness')) {
        return new Response(JSON.stringify({ status: 'rendered' }), {
          status: 200,
          headers: { 'Content-Type': 'application/json' },
        })
      }
      if (url.includes('plan.json')) return ok(JSON.stringify(planFixture))
      if (url.includes('manifest.json')) return ok(JSON.stringify(manifestFixture))
      // The audio leaf fails the post-readiness load — a leaf that 404s/500s in a
      // race after readiness reported rendered.
      if (url.includes('audio.wav')) return new Response('boom', { status: 500 })
      return new Response('not found', { status: 404 })
    })
    vi.stubGlobal('fetch', fetchStub)
    render(<App />)

    await waitFor(() => expect(screen.getByTestId('render-gate-failed')).toBeInTheDocument())
    const banner = screen.getByTestId('render-gate-failed')
    expect(banner.textContent).toMatch(/render_failed/)
    expect(banner.textContent).toMatch(/load_failed/)
    // Distinct from the wrong-path terminal — it is NOT a no_out_dir banner.
    expect(banner.textContent).not.toMatch(/no_out_dir/)
    // The plan never loaded.
    expect(screen.queryByTestId('block-list')).not.toBeInTheDocument()
  })

  it('rendering → existing loading spinner ("Waiting for render…"), no failure', async () => {
    vi.stubGlobal('fetch', makeCompanionStub(() => ({ body: { status: 'rendering' } })))
    render(<App />)

    await waitFor(() =>
      expect(screen.getByTestId('render-gate-rendering')).toBeInTheDocument(),
    )
    expect(screen.getByTestId('render-gate-rendering').textContent).toMatch(/Waiting for render/)
    expect(screen.queryByTestId('render-gate-failed')).not.toBeInTheDocument()
    expect(screen.queryByTestId('block-list')).not.toBeInTheDocument()
  })
})
