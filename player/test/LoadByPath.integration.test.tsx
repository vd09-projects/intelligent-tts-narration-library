import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { cleanup, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import App from '../src/App.tsx'
import planFixture from '../public/fixtures/sample/plan.json' with { type: 'json' }
import manifestFixture from '../public/fixtures/sample/manifest.json' with { type: 'json' }

// #70 load-by-path integration: in server mode, typing a dir and clicking "Load"
// must render that plan end-to-end over the GET /artifact route (no folder
// picker), and disable every load trigger while the load is in flight.

// A distinct plan served from the typed dir so we can prove it REPLACED the
// fixture (different plan_id → App re-seeds; only b1/b2 exist).
const LOADED_PLAN = {
  schema_version: '1',
  plan_id: '01HLOADBYPATHSERVERDIR70',
  locale: 'en',
  source: { kind: 'file', uri: 'x.md', content_hash: 'h' },
  blocks: [
    {
      id: 'b1',
      order: 1,
      class: 'heading',
      level: 1,
      status: 'voiced',
      source_map: { kind: 'line_range', start_line: 1, end_line: 1 },
      segments: [{ id: 's1', kind: 'speech', text: 'Loaded heading.' }],
      provenance: { voiced_by: 'deterministic', deterministic: true, level_asked: 1 },
    },
    {
      id: 'b2',
      order: 2,
      class: 'prose',
      level: 1,
      status: 'voiced',
      source_map: { kind: 'line_range', start_line: 2, end_line: 2 },
      segments: [{ id: 's2', kind: 'speech', text: 'Loaded prose.' }],
      provenance: { voiced_by: 'deterministic', deterministic: true, level_asked: 1 },
    },
  ],
}
const LOADED_MANIFEST = {
  schema_version: 1,
  voice: 'am_michael',
  stale: false,
  blocks: [
    { id: 'b1', start_ms: 0, end_ms: 100, audio_ref: 'audio.wav', class: 'heading', level: 1, status: 'voiced' },
    { id: 'b2', start_ms: 100, end_ms: 300, audio_ref: 'audio.wav', class: 'prose', level: 1, status: 'voiced' },
  ],
}
const AUDIO = new Uint8Array([0x52, 0x49, 0x46, 0x46]).buffer

interface StubOpts {
  // gateLoadAudio: hold the load-by-path audio.wav fetch open so we can assert
  // controls are disabled mid-flight.
  gateLoadAudio?: boolean
}

function makeStub(opts: StubOpts = {}) {
  let releaseLoadAudio: () => void = () => {}
  const loadAudioGate = opts.gateLoadAudio
    ? new Promise<void>((r) => {
        releaseLoadAudio = r
      })
    : Promise.resolve()
  const ok = (b: BodyInit) => new Response(b, { status: 200 })
  const fn = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
    const url = input.toString()
    const method = init?.method ?? 'GET'
    if (url.includes('/healthz')) return ok(JSON.stringify({ status: 'ok' }))
    // The load-by-path route hits /artifact for the typed dir.
    if (url.includes('/artifact')) {
      if (url.includes('name=plan.json')) return ok(JSON.stringify(LOADED_PLAN))
      if (url.includes('name=manifest.json')) return ok(JSON.stringify(LOADED_MANIFEST))
      if (url.includes('name=source.md')) return ok('# loaded source\n')
      if (url.includes('name=audio.wav')) {
        await loadAudioGate
        return ok(AUDIO)
      }
    }
    // Initial fixture load (FIXTURE_BASE) so the App mounts before we click Load.
    if (url.includes('plan.json')) return ok(JSON.stringify(planFixture))
    if (url.includes('manifest.json')) return ok(JSON.stringify(manifestFixture))
    if (url.includes('source.md')) return ok('# fixture source\n')
    if (url.includes('audio.wav')) return ok(AUDIO)
    void method
    return new Response('nf', { status: 404 })
  })
  return { fn, release: () => releaseLoadAudio() }
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
  vi.spyOn(URL, 'createObjectURL').mockImplementation(() => 'blob:loaded')
  vi.spyOn(URL, 'revokeObjectURL').mockImplementation(() => {})
})

afterEach(() => {
  cleanup()
  vi.restoreAllMocks()
  vi.unstubAllGlobals()
})

async function renderServerMode(stub: ReturnType<typeof makeStub>) {
  vi.stubGlobal('fetch', stub.fn)
  const user = userEvent.setup()
  render(<App />)
  await waitFor(() => screen.getByTestId('block-list'))
  await waitFor(() =>
    expect(screen.getByTestId('server-mode-indicator')).toHaveAttribute('data-server-mode', 'server'),
  )
  return user
}

describe('#70 load plan by typed server dir path', () => {
  it('typing a dir + clicking Load renders that plan over /artifact (no picker)', async () => {
    const stub = makeStub()
    const user = await renderServerMode(stub)

    // Fixture is loaded first → it has b12.
    expect(document.querySelector('[data-block-id="b12"]')).toBeTruthy()

    const dirInput = screen.getByTestId('escalate-dir-input')
    await user.clear(dirInput)
    await user.type(dirInput, '/loaded/dir')
    await user.click(screen.getByTestId('load-dir-button'))

    // The loaded plan (only b1, b2) replaced the fixture.
    await waitFor(() => expect(document.querySelector('[data-block-id="b2"]')).toBeTruthy())
    expect(document.querySelector('[data-block-id="b12"]')).toBeFalsy()

    // All four artifacts were fetched from /artifact for the typed dir.
    const urls = stub.fn.mock.calls.map((c) => c[0].toString())
    for (const name of ['plan.json', 'manifest.json', 'audio.wav', 'source.md']) {
      const hit = urls.find((u) => u.includes('/artifact') && u.includes(`name=${name}`))
      expect(hit, `expected /artifact load for ${name}`).toBeTruthy()
      expect(hit!).toContain('dir=%2Floaded%2Fdir')
    }
  })

  it('Load button + pickers are disabled while a load is in flight', async () => {
    const stub = makeStub({ gateLoadAudio: true })
    const user = await renderServerMode(stub)

    const dirInput = screen.getByTestId('escalate-dir-input')
    await user.clear(dirInput)
    await user.type(dirInput, '/loaded/dir')
    await user.click(screen.getByTestId('load-dir-button'))

    // Mid-flight (audio fetch gated): every load trigger disabled.
    await waitFor(() =>
      expect(screen.getByTestId('load-dir-button')).toBeDisabled(),
    )
    expect(screen.getByRole('button', { name: /Load directory/ })).toBeDisabled()

    // Release the gate; the load completes and controls re-enable.
    stub.release()
    await waitFor(() => expect(document.querySelector('[data-block-id="b2"]')).toBeTruthy())
    await waitFor(() => expect(screen.getByTestId('load-dir-button')).not.toBeDisabled())
  })

  it('Load button is disabled when the dir field is empty', async () => {
    const stub = makeStub()
    await renderServerMode(stub)
    expect(screen.getByTestId('load-dir-button')).toBeDisabled()
  })
})
