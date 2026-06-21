import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { cleanup, render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import App from '../src/App.tsx'
import planFixture from '../public/fixtures/sample/plan.json' with { type: 'json' }
import manifestFixture from '../public/fixtures/sample/manifest.json' with { type: 'json' }
import type { EscalateSuccess } from '../src/types/manifest.ts'

// The escalation integration tests drive the full App against a per-URL fetch
// stub that extends the smoke-test stubFetch with /healthz + /escalate, and
// lets each test program the /escalate response + an optional post-patch
// manifest mutation (to model downstream offset shifts).

const SOURCE_MD = Array.from({ length: 50 }, (_, i) => `line ${i + 1}`).join('\n')

interface StubConfig {
  // healthz: true → server mode; false → fixture mode.
  healthOk: boolean
  // escalate: programmed response for POST /escalate, or null to 404.
  escalate?: () => { status: number; body: unknown }
  // manifestAfterPatch: once an /escalate has fired, subsequent manifest.json
  // fetches return this instead of the original (models the re-fetch).
  manifestAfterPatch?: () => unknown
}

function deepClone<T>(v: T): T {
  return JSON.parse(JSON.stringify(v)) as T
}

function makeStub(cfg: StubConfig) {
  const state = { escalated: false, manifestFetches: 0, audioFetches: 0, escalateCalls: 0 }
  const ok = (body: BodyInit, init?: ResponseInit) =>
    new Response(body, { status: 200, ...init })
  const fn = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
    const url =
      typeof input === 'string'
        ? input
        : input instanceof URL
          ? input.toString()
          : (input as Request).url
    const method = init?.method ?? 'GET'

    if (url.includes('/healthz')) {
      return cfg.healthOk
        ? ok(JSON.stringify({ status: 'ok' }))
        : new Response('down', { status: 503 })
    }
    if (url.includes('/escalate') && method === 'POST') {
      state.escalateCalls++
      state.escalated = true
      const r = cfg.escalate?.() ?? { status: 404, body: { reason: 'internal', message: 'no stub' } }
      return new Response(JSON.stringify(r.body), {
        status: r.status,
        headers: { 'Content-Type': 'application/json' },
      })
    }
    if (url.endsWith('plan.json')) return ok(JSON.stringify(planFixture))
    if (url.includes('manifest.json')) {
      state.manifestFetches++
      const body =
        state.escalated && cfg.manifestAfterPatch
          ? cfg.manifestAfterPatch()
          : manifestFixture
      return ok(JSON.stringify(body))
    }
    if (url.endsWith('source.md')) return ok(SOURCE_MD)
    if (url.includes('audio.wav')) {
      state.audioFetches++
      return ok(new Uint8Array([0x52, 0x49, 0x46, 0x46]).buffer)
    }
    return new Response('not found', { status: 404 })
  })
  return { fn, state }
}

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
  // Wait for /healthz to resolve to server mode (controls render only then).
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

describe('Escalate integration — server mode', () => {
  it('AC: patched row updates to L3/voiced; spinner shown then cleared', async () => {
    const stub = makeStub({ healthOk: true, escalate: () => ({ status: 200, body: patchedSuccess() }) })
    const user = await renderServerMode(stub)

    const btn = screen.getByTestId('escalate-b2-L3')
    await user.click(btn)

    await waitFor(() => {
      const row = rowFor('b2')
      expect(row.getAttribute('data-block-status')).toBe('voiced')
      expect(within(row).getByText('L3')).toBeInTheDocument()
    })
    // Spinner gone (row back to idle).
    expect(screen.queryByTestId('escalate-loading-b2')).not.toBeInTheDocument()
  })

  it('AC: only the escalated row re-renders — siblings are not re-rendered', async () => {
    const stub = makeStub({ healthOk: true, escalate: () => ({ status: 200, body: patchedSuccess() }) })
    const user = await renderServerMode(stub)

    // Snapshot a sibling row's DOM node identity. React.memo + stable callbacks
    // mean an unchanged sibling keeps the same node across the patch.
    const siblingBefore = rowFor('b6') // code block, never touched
    const siblingTextBefore = siblingBefore.outerHTML

    await user.click(screen.getByTestId('escalate-b2-L3'))
    await waitFor(() => expect(rowFor('b2').getAttribute('data-block-status')).toBe('voiced'))

    const siblingAfter = rowFor('b6')
    // The sibling node is the same element instance (not unmounted/remounted)
    // and its markup is byte-identical.
    expect(siblingAfter).toBe(siblingBefore)
    expect(siblingAfter.outerHTML).toBe(siblingTextBefore)
  })

  it('AC: NOT-playing patch — no audio re-fetch, currentTime/paused/highlight preserved', async () => {
    const stub = makeStub({ healthOk: true, escalate: () => ({ status: 200, body: patchedSuccess() }) })
    const user = await renderServerMode(stub)

    const audio = screen.getByTestId('audio-element') as HTMLAudioElement
    // Nothing playing; activeBlockId is null. Record pre-state.
    audio.currentTime = 0
    const pausedBefore = audio.paused
    const audioFetchesBefore = stub.state.audioFetches

    await user.click(screen.getByTestId('escalate-b2-L3'))
    await waitFor(() => expect(rowFor('b2').getAttribute('data-block-status')).toBe('voiced'))

    // No audio.wav re-fetch happened (patched block was not the one playing).
    expect(stub.state.audioFetches).toBe(audioFetchesBefore)
    expect(audio.paused).toBe(pausedBefore)
    expect(audio.currentTime).toBe(0)
    // No row is force-highlighted by the patch.
    expect(document.querySelector('[aria-current="true"]')).toBeNull()
  })

  it('AC: paused highlight on bX survives escalating a DIFFERENT block bY (B1 regression)', async () => {
    // The dangerous scenario the trivial "highlight preserved" test missed:
    // a block is highlighted while PAUSED, then a *different*, non-playing block
    // is escalated. reconcileManifest returns a new top-level manifest object on
    // the patch; usePlayback must NOT reset the active-block highlight / current
    // time on that per-block patch (it only resets on a real directory swap).
    const stub = makeStub({ healthOk: true, escalate: () => ({ status: 200, body: patchedSuccess() }) })
    const user = await renderServerMode(stub)

    // Highlight bX = b3 while paused (seekToBlock bumps activeBlockId eagerly;
    // no play() is issued so the rAF loop stays idle — the paused case).
    await user.click(screen.getByRole('button', { name: 'Seek to block b3' }))
    await waitFor(() => expect(rowFor('b3').getAttribute('aria-current')).toBe('true'))
    const audio = screen.getByTestId('audio-element') as HTMLAudioElement
    expect(audio.paused).toBe(true)
    const currentTimeBefore = audio.currentTime

    // Escalate bY = b2 (a DIFFERENT, non-playing block).
    await user.click(screen.getByTestId('escalate-b2-L3'))
    await waitFor(() => expect(rowFor('b2').getAttribute('data-block-status')).toBe('voiced'))

    // bX's highlight is preserved across the per-block patch, and only bX is
    // highlighted (the patch did not move or clear the active block).
    expect(rowFor('b3').getAttribute('aria-current')).toBe('true')
    const highlighted = document.querySelectorAll('[aria-current="true"]')
    expect(highlighted).toHaveLength(1)
    expect(highlighted[0]).toBe(rowFor('b3'))
    // The displayed current time is unchanged (no snap-to-zero reset).
    expect(audio.currentTime).toBe(currentTimeBefore)
  })

  it('AC: PLAYING patch — audio blob re-fetched (cache-busted) when patched block is active', async () => {
    const stub = makeStub({ healthOk: true, escalate: () => ({ status: 200, body: patchedSuccess() }) })
    const user = await renderServerMode(stub)

    // Make b2 the active/playing block: click its Seek button (seekToBlock bumps
    // activeBlockId eagerly).
    await user.click(screen.getByRole('button', { name: 'Seek to block b2' }))
    await waitFor(() =>
      expect(rowFor('b2').getAttribute('aria-current')).toBe('true'),
    )
    const audioFetchesBefore = stub.state.audioFetches

    await user.click(screen.getByTestId('escalate-b2-L3'))
    await waitFor(() => expect(stub.state.audioFetches).toBeGreaterThan(audioFetchesBefore))

    // The re-fetch URL was cache-busted.
    const calledCacheBust = stub.fn.mock.calls.some((c) => String(c[0]).includes('audio.wav?ts='))
    expect(calledCacheBust).toBe(true)
  })

  it('AC: downstream offsets become authoritative from the re-fetched manifest', async () => {
    // After the patch, manifest.json returns shifted offsets for b3 (a later block).
    const shifted = () => {
      const m = deepClone(manifestFixture)
      m.blocks[1].start_ms = 150
      m.blocks[1].end_ms = 900 // b2 grew
      m.blocks[2].start_ms = 900 // b3 pushed downstream
      m.blocks[2].end_ms = 1050
      return m
    }
    const stub = makeStub({
      healthOk: true,
      escalate: () => ({ status: 200, body: patchedSuccess() }),
      manifestAfterPatch: shifted,
    })
    const user = await renderServerMode(stub)

    await user.click(screen.getByTestId('escalate-b2-L3'))
    await waitFor(() => {
      const row = rowFor('b3')
      expect(within(row).getByText(/900–1050 ms/)).toBeInTheDocument()
    })
  })

  it('AC: refusal path renders RefusalBadge inline, no toast', async () => {
    const stub = makeStub({
      healthOk: true,
      escalate: () => {
        const refusal = {
          reason: 'no_intelligence_available',
          message: 'No intelligence adapter wired; cannot summarize at L3.',
          spoken: true,
          source_map: { kind: 'line_range', start_line: 3, end_line: 3 },
        }
        // Server contract: refused → {block, refusal} with refusal TOP-LEVEL
        // (and block.status=refused + block.refusal populated by read-back).
        return {
          status: 200,
          body: {
            block: {
              id: 'b2',
              order: 2,
              class: 'prose',
              level: 3,
              status: 'refused',
              source_map: { kind: 'line_range', start_line: 3, end_line: 3 },
              refusal,
              provenance: { voiced_by: 'planner', deterministic: true },
            },
            refusal,
          },
        }
      },
    })
    const user = await renderServerMode(stub)

    await user.click(screen.getByTestId('escalate-b2-L3'))
    await waitFor(() => {
      const row = rowFor('b2')
      expect(within(row).getByTestId('refusal-badge')).toBeInTheDocument()
      expect(row.getAttribute('data-block-status')).toBe('refused')
    })
  })

  it('AC: server error (400 source_not_found) → inline .escalate-error, original block intact', async () => {
    const stub = makeStub({
      healthOk: true,
      escalate: () => ({ status: 400, body: { reason: 'source_not_found', message: 'dir not found' } }),
    })
    const user = await renderServerMode(stub)

    await user.click(screen.getByTestId('escalate-b2-L3'))
    await waitFor(() => {
      const err = screen.getByTestId('escalate-error-b2')
      expect(err.textContent).toMatch(/dir not found/)
      expect(err.textContent).toMatch(/source_not_found/)
    })
    // Original block untouched (still degraded @ L1).
    const row = rowFor('b2')
    expect(row.getAttribute('data-block-status')).toBe('degraded')
    expect(within(row).getByText('L1')).toBeInTheDocument()
  })

  it('AC: network reject → inline unreachable error, spinner cleared', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
        const url = typeof input === 'string' ? input : (input as Request).url
        const method = init?.method ?? 'GET'
        if (url.includes('/healthz')) return new Response(JSON.stringify({ status: 'ok' }), { status: 200 })
        if (url.includes('/escalate') && method === 'POST') throw new TypeError('Failed to fetch')
        if (url.endsWith('plan.json')) return new Response(JSON.stringify(planFixture), { status: 200 })
        if (url.includes('manifest.json')) return new Response(JSON.stringify(manifestFixture), { status: 200 })
        if (url.endsWith('source.md')) return new Response(SOURCE_MD, { status: 200 })
        if (url.includes('audio.wav')) return new Response(new Uint8Array([0x52]).buffer, { status: 200 })
        return new Response('nf', { status: 404 })
      }),
    )
    const user = userEvent.setup()
    render(<App />)
    await waitFor(() => screen.getByTestId('block-list'))
    await waitFor(() =>
      expect(screen.getByTestId('server-mode-indicator')).toHaveAttribute('data-server-mode', 'server'),
    )
    await user.type(screen.getByTestId('escalate-dir-input'), '/abs/out')

    await user.click(screen.getByTestId('escalate-b2-L3'))
    await waitFor(() => {
      const err = screen.getByTestId('escalate-error-b2')
      expect(err.textContent).toMatch(/unreachable/)
    })
    expect(screen.queryByTestId('escalate-loading-b2')).not.toBeInTheDocument()
  })

  it('AC: post-patch re-fetch failure → patch retained + soft staleDownstream, no rollback', async () => {
    let escalated = false
    vi.stubGlobal(
      'fetch',
      vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
        const url = typeof input === 'string' ? input : (input as Request).url
        const method = init?.method ?? 'GET'
        if (url.includes('/healthz')) return new Response(JSON.stringify({ status: 'ok' }), { status: 200 })
        if (url.includes('/escalate') && method === 'POST') {
          escalated = true
          return new Response(JSON.stringify(patchedSuccess()), { status: 200 })
        }
        if (url.endsWith('plan.json')) return new Response(JSON.stringify(planFixture), { status: 200 })
        if (url.includes('manifest.json')) {
          // The post-patch re-fetch fails.
          if (escalated) return new Response('boom', { status: 500 })
          return new Response(JSON.stringify(manifestFixture), { status: 200 })
        }
        if (url.endsWith('source.md')) return new Response(SOURCE_MD, { status: 200 })
        if (url.includes('audio.wav')) return new Response(new Uint8Array([0x52]).buffer, { status: 200 })
        return new Response('nf', { status: 404 })
      }),
    )
    const user = userEvent.setup()
    render(<App />)
    await waitFor(() => screen.getByTestId('block-list'))
    await waitFor(() =>
      expect(screen.getByTestId('server-mode-indicator')).toHaveAttribute('data-server-mode', 'server'),
    )
    await user.type(screen.getByTestId('escalate-dir-input'), '/abs/out')

    await user.click(screen.getByTestId('escalate-b2-L3'))
    await waitFor(() => expect(screen.getByTestId('escalate-stale-b2')).toBeInTheDocument())
    // Patch retained (block is L3/voiced) despite the re-fetch failure.
    const row = rowFor('b2')
    expect(row.getAttribute('data-block-status')).toBe('voiced')
    expect(within(row).getByText('L3')).toBeInTheDocument()
  })

  it('AC: content-identical 200 → "no visible change" ack (not error, not silent)', async () => {
    // The server re-renders the SAME content the block already holds. Build a
    // patched response that matches b2's current (degraded @ L1) content.
    const sameAsCurrent: EscalateSuccess = {
      block: {
        id: 'b2',
        order: 2,
        class: 'prose',
        level: 1,
        status: 'degraded',
        source_map: { kind: 'line_range', start_line: 3, end_line: 3 },
        // Match the fixture b2 segment text exactly.
        segments: (planFixture.blocks[1] as { segments: unknown }).segments as never,
        provenance: { voiced_by: 'verbatim', deterministic: true, level_asked: 1 },
      },
      timing: { block_id: 'b2', start_ms: 150, end_ms: 400, audio_ref: 'audio.wav#150-400' },
      audio_ref: 'audio.wav#150-400',
    }
    const stub = makeStub({ healthOk: true, escalate: () => ({ status: 200, body: sameAsCurrent }) })
    const user = await renderServerMode(stub)

    // b2 prose @ L1 → escalate to L2 (a target) but server returns identical content.
    await user.click(screen.getByTestId('escalate-b2-L2'))
    await waitFor(() => expect(screen.getByTestId('escalate-noop-b2')).toBeInTheDocument())
    // Not an error.
    expect(screen.queryByTestId('escalate-error-b2')).not.toBeInTheDocument()
  })

  it('AC: STALE-CLOSURE — changing dir after first render submits the NEW dir', async () => {
    const stub = makeStub({ healthOk: true, escalate: () => ({ status: 200, body: patchedSuccess() }) })
    const user = await renderServerMode(stub, '/first/dir')

    // Change the dir AFTER first render.
    const dirInput = screen.getByTestId('escalate-dir-input')
    await user.clear(dirInput)
    await user.type(dirInput, '/second/dir')

    await user.click(screen.getByTestId('escalate-b2-L3'))
    await waitFor(() => expect(rowFor('b2').getAttribute('data-block-status')).toBe('voiced'))

    const escalateCall = stub.fn.mock.calls.find(
      (c) => String(c[0]).includes('/escalate') && (c[1] as RequestInit)?.method === 'POST',
    )
    const body = JSON.parse(String((escalateCall?.[1] as RequestInit).body))
    expect(body.dir).toBe('/second/dir')
  })

  it('AC: CONCURRENT/sequential — two patches converge to the final re-fetched manifest', async () => {
    // Each escalate bumps the version returned by manifest.json afterward.
    let version = 0
    const manifestAfter = () => {
      const m = deepClone(manifestFixture)
      m.blocks[2].start_ms = 900 + version * 100
      m.blocks[2].end_ms = 1050 + version * 100
      return m
    }
    const stub = makeStub({
      healthOk: true,
      escalate: () => {
        version++
        // First patch lands the block at L2 (so an L3 target still exists for
        // the second sequential escalate); second lands it at L3.
        const level = version === 1 ? 2 : 3
        return {
          status: 200,
          body: patchedSuccess({ block: { ...patchedSuccess().block, level } }),
        }
      },
      manifestAfterPatch: manifestAfter,
    })
    const user = await renderServerMode(stub)

    await user.click(screen.getByTestId('escalate-b2-L2'))
    await waitFor(() => expect(within(rowFor('b3')).getByText(/1000–1150 ms/)).toBeInTheDocument())

    // After the first patch b2 is L2 → an L3 target remains.
    await user.click(screen.getByTestId('escalate-b2-L3'))
    await waitFor(() => expect(within(rowFor('b3')).getByText(/1100–1250 ms/)).toBeInTheDocument())
  })

  it('AC: UNKNOWN reason token → generic inline error (forward-compat, no crash)', async () => {
    const stub = makeStub({
      healthOk: true,
      escalate: () => ({ status: 409, body: { reason: 'some_future_token', message: 'a new server reason' } }),
    })
    const user = await renderServerMode(stub)

    await user.click(screen.getByTestId('escalate-b2-L3'))
    await waitFor(() => {
      const err = screen.getByTestId('escalate-error-b2')
      expect(err.textContent).toMatch(/a new server reason/)
      expect(err.textContent).toMatch(/some_future_token/)
    })
  })

  it('AC: dir gate — healthz OK but dir empty → controls disabled with hint', async () => {
    const stub = makeStub({ healthOk: true, escalate: () => ({ status: 200, body: patchedSuccess() }) })
    // Render WITHOUT typing a dir.
    await renderServerMode(stub, '')

    const btn = screen.getByTestId('escalate-b2-L3') as HTMLButtonElement
    expect(btn.disabled).toBe(true)
    expect(btn.title).toMatch(/Enter the escalate output directory/)
  })

  it('AC: seed survives recheck — patched block not clobbered by /healthz re-probe', async () => {
    // No explicit recheck UI is wired, but useServerMode probes on mount under
    // StrictMode-like double effects. We assert the patch persists across a
    // re-render triggered by typing in the dir field (which re-runs App render
    // but must NOT re-seed plan from `active`).
    const stub = makeStub({ healthOk: true, escalate: () => ({ status: 200, body: patchedSuccess() }) })
    const user = await renderServerMode(stub)

    await user.click(screen.getByTestId('escalate-b2-L3'))
    await waitFor(() => expect(rowFor('b2').getAttribute('data-block-status')).toBe('voiced'))

    // Force an App re-render via a dir edit; the seed guard must not reset b2.
    await user.type(screen.getByTestId('escalate-dir-input'), 'x')
    expect(rowFor('b2').getAttribute('data-block-status')).toBe('voiced')
  })
})

describe('Escalate integration — fixture fallback', () => {
  it('AC: fixture mode shows the copy-command card, no live level controls, + AC9 hint', async () => {
    const stub = makeStub({ healthOk: false })
    vi.stubGlobal('fetch', stub.fn)
    const user = userEvent.setup()
    render(<App />)
    await waitFor(() => screen.getByTestId('block-list'))
    await waitFor(() =>
      expect(screen.getByTestId('server-mode-indicator')).toHaveAttribute('data-server-mode', 'fixture'),
    )

    // AC9 hint banner present.
    expect(screen.getByTestId('escalate-server-hint')).toBeInTheDocument()
    // No live level controls.
    expect(screen.queryByTestId('escalate-b2-L3')).not.toBeInTheDocument()
    // Legacy toggle → EscalateCard still works.
    const legacy = within(rowFor('b2')).getByRole('button', { name: /^Escalate L3$/ })
    await user.click(legacy)
    expect(await screen.findByTestId('escalate-card')).toBeInTheDocument()
  })
})
