import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { cleanup, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import App from '../src/App.tsx'
import planFixture from '../public/fixtures/sample/plan.json' with { type: 'json' }
import manifestFixture from '../public/fixtures/sample/manifest.json' with { type: 'json' }

// Minimal source.md so SourcePane has something to draw with line ranges.
const SOURCE_MD = Array.from({ length: 50 }, (_, i) => `line ${i + 1}`).join('\n')

// Stub fetch so useFixture's parallel fetches resolve from in-memory bytes.
function stubFetch() {
  const ok = (body: BodyInit, init?: ResponseInit) => new Response(body, { status: 200, ...init })
  return vi.fn(async (input: RequestInfo | URL) => {
    const url = typeof input === 'string' ? input : input instanceof URL ? input.toString() : (input as Request).url
    // /healthz is intentionally NOT served here → useServerMode resolves to
    // FIXTURE mode, which is the surface these smoke ACs assert (the legacy
    // copy-command EscalateCard). Live escalation is covered in
    // Escalate.integration.test.tsx.
    if (url.includes('/healthz')) return new Response('down', { status: 503 })
    if (url.endsWith('plan.json')) return ok(JSON.stringify(planFixture))
    if (url.includes('manifest.json')) return ok(JSON.stringify(manifestFixture))
    if (url.endsWith('source.md')) return ok(SOURCE_MD)
    if (url.includes('audio.wav')) return ok(new Uint8Array([0x52, 0x49, 0x46, 0x46]).buffer)
    return new Response('not found', { status: 404 })
  })
}

beforeEach(() => {
  vi.stubGlobal('fetch', stubFetch())
  // jsdom doesn't implement HTMLMediaElement.play / pause — silence both.
  Object.defineProperty(window.HTMLMediaElement.prototype, 'play', {
    configurable: true,
    value: vi.fn().mockResolvedValue(undefined),
  })
  Object.defineProperty(window.HTMLMediaElement.prototype, 'pause', {
    configurable: true,
    value: vi.fn(),
  })
  // Clipboard polyfill for the Copy button test.
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

describe('App smoke — 8 acceptance criteria from issue #18', () => {
  it('AC1: app initializes (Vite + React + TS scaffold renders without crashing)', async () => {
    render(<App />)
    await waitFor(() => {
      expect(screen.getByText(/Intelligent TTS Reference Player/i)).toBeInTheDocument()
    })
  })

  it('AC2: loads the bundled fixture directory (plan + manifest + audio)', async () => {
    render(<App />)
    await waitFor(() => {
      // Fixture has 12 blocks; smoke check the first block is in the DOM.
      expect(screen.getByTestId('block-list')).toBeInTheDocument()
    })
    expect(screen.getByTestId('audio-element')).toBeInTheDocument()
    // Voice indicator surfaces the manifest voice. (The string "af_bella"
    // also appears inside block segment text — query the voice indicator
    // specifically by aria-label.)
    expect(screen.getByLabelText(/Voice: af_bella/)).toBeInTheDocument()
  })

  it('AC3: block-level scrubber — Seek button per block + active-block highlight wiring', async () => {
    render(<App />)
    await waitFor(() => screen.getByTestId('block-list'))
    const seekButtons = screen.getAllByRole('button', { name: /^Seek to block/ })
    // 12 blocks in the fixture → 12 seek buttons.
    expect(seekButtons.length).toBe(12)

    const user = userEvent.setup()
    await user.click(seekButtons[2])
    // After clicking, the row whose data-block-id matches gets aria-current
    // because usePlayback.seekToBlock dispatches an eager state bump.
    await waitFor(() => {
      const active = document.querySelector('[aria-current="true"]')
      expect(active).not.toBeNull()
    })
  })

  it('AC4: source pane renders alongside block list', async () => {
    render(<App />)
    await waitFor(() => screen.getByTestId('block-list'))
    expect(screen.getByTestId('source-pre')).toBeInTheDocument()
  })

  it('AC5 (fixture mode): per-block Escalate L3 button surfaces the literal CLI command', async () => {
    render(<App />)
    await waitFor(() => screen.getByTestId('block-list'))
    // Wait for /healthz to resolve to FIXTURE mode — only then does the legacy
    // copy-command toggle render (issue #50 mode split).
    await waitFor(() =>
      expect(screen.getByTestId('server-mode-indicator')).toHaveAttribute(
        'data-server-mode',
        'fixture',
      ),
    )
    const escButtons = screen.getAllByRole('button', { name: /^Escalate L3$/ })
    // 10 voiced/degraded blocks have the toggle; 2 refused blocks hide it.
    expect(escButtons.length).toBe(10)

    const user = userEvent.setup()
    await user.click(escButtons[0])
    const card = await screen.findByTestId('escalate-card')
    expect(card).toBeInTheDocument()
    // The literal command embeds the source URI from plan.source.uri.
    expect(card.textContent).toMatch(
      /narrate --block b1 --level 3 --file docs\/samples\/sample\.md/,
    )
  })

  it('AC6: refused blocks render with a REFUSED badge + spoken notice inline', async () => {
    render(<App />)
    await waitFor(() => screen.getByTestId('block-list'))
    const refused = screen.getAllByTestId('refusal-badge')
    // Fixture has two refused blocks (b4 prose-no-intelligence, b11 bare-image).
    expect(refused.length).toBe(2)
    // Each carries a machine reason + a human message.
    expect(refused[0].textContent).toMatch(/REFUSED/)
    expect(refused[0].textContent).toMatch(/no_intelligence_available|bare_image_no_description/)
  })

  it('AC7: directory loader is exposed in the top bar (Load directory… button)', async () => {
    render(<App />)
    await waitFor(() => screen.getByTestId('block-list'))
    expect(screen.getByRole('button', { name: /Load directory/i })).toBeInTheDocument()
  })

  it('AC8: full document fixture (12 blocks covering every Class + every Status) renders end-to-end', async () => {
    render(<App />)
    await waitFor(() => screen.getByTestId('block-list'))
    const rows = document.querySelectorAll('.block-row')
    expect(rows.length).toBe(12)

    // Every Class enum value mentioned in plan.json should appear as a row.
    const classesPresent = new Set(
      Array.from(rows).map((r) => r.getAttribute('data-block-class')),
    )
    expect(classesPresent.has('heading')).toBe(true)
    expect(classesPresent.has('prose')).toBe(true)
    expect(classesPresent.has('code')).toBe(true)
    expect(classesPresent.has('table')).toBe(true)
    expect(classesPresent.has('list')).toBe(true)
    expect(classesPresent.has('unknown')).toBe(true)

    // Every Status enum value too — voiced + degraded + refused.
    const statusesPresent = new Set(
      Array.from(rows).map((r) => r.getAttribute('data-block-status')),
    )
    expect(statusesPresent.has('voiced')).toBe(true)
    expect(statusesPresent.has('degraded')).toBe(true)
    expect(statusesPresent.has('refused')).toBe(true)
  })

  it('tab order reaches all primary interactive elements (TopBar → BlockList → audio)', async () => {
    render(<App />)
    await waitFor(() => screen.getByTestId('block-list'))

    // Collect focusable elements in document order — the test asserts the
    // TopBar buttons precede the BlockList buttons which precede the audio.
    const all = Array.from(
      document.querySelectorAll<HTMLElement>(
        'button, input, [tabindex]:not([tabindex="-1"]), audio[controls]',
      ),
    )
    const topBarBtn = all.find((el) => el.textContent?.match(/Load directory/))
    const firstSeekBtn = all.find((el) => el.getAttribute('aria-label')?.startsWith('Seek to block'))
    const audio = document.querySelector('audio') as HTMLAudioElement
    expect(topBarBtn && firstSeekBtn && audio).toBeTruthy()
    if (!topBarBtn || !firstSeekBtn) return
    expect(all.indexOf(topBarBtn)).toBeLessThan(all.indexOf(firstSeekBtn))
    // The audio element comes after the block list buttons.
    const allWithAudio = Array.from(document.querySelectorAll<HTMLElement>('button, input, audio'))
    expect(allWithAudio.indexOf(audio)).toBeGreaterThan(allWithAudio.indexOf(firstSeekBtn))
  })
})
