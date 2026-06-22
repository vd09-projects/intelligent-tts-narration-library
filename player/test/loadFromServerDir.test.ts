import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { loadFromServerDir } from '../src/lib/loadDirectory.ts'
import type { RefetchBaseInputs } from '../src/lib/refetchBase.ts'

// #70 load-by-path: loadFromServerDir fetches {plan,manifest,audio,source?} over
// the server's GET /artifact route. These unit tests pin the strictness contract
// (required-file abort, tolerant source.md, schema warn-not-throw) and the blob-
// URL leak-safety (no leaked object URL on a post-create failure).

const SERVER = 'http://127.0.0.1:8080'
const DIR = '/abs/out'

function inputs(over: Partial<RefetchBaseInputs> = {}): RefetchBaseInputs {
  return {
    serverMode: true,
    dir: DIR,
    serverBaseUrl: SERVER,
    fixtureBase: '/fixtures/sample',
    ...over,
  }
}

const PLAN = { schema_version: '1', plan_id: 'p1', blocks: [] }
const MANIFEST = { schema_version: 1, voice: 'af_bella', blocks: [], stale: false }
const AUDIO = new Uint8Array([0x52, 0x49, 0x46, 0x46]).buffer

interface StubOpts {
  planStatus?: number
  manifestStatus?: number
  audioStatus?: number
  sourceStatus?: number
  sourceBody?: string
  planBody?: string
  manifestBody?: string
  sourceThrows?: boolean
}

function stubFetch(opts: StubOpts = {}) {
  const ok = (body: BodyInit, status = 200) => new Response(body, { status })
  return vi.fn(async (input: RequestInfo | URL) => {
    const url = input.toString()
    if (url.includes('plan.json')) {
      const s = opts.planStatus ?? 200
      if (s !== 200) return new Response('x', { status: s })
      return ok(opts.planBody ?? JSON.stringify(PLAN))
    }
    if (url.includes('manifest.json')) {
      const s = opts.manifestStatus ?? 200
      if (s !== 200) return new Response('x', { status: s })
      return ok(opts.manifestBody ?? JSON.stringify(MANIFEST))
    }
    if (url.includes('audio.wav')) {
      const s = opts.audioStatus ?? 200
      if (s !== 200) return new Response('x', { status: s })
      return ok(AUDIO)
    }
    if (url.includes('source.md')) {
      if (opts.sourceThrows) throw new Error('network down')
      const s = opts.sourceStatus ?? 200
      if (s !== 200) return new Response('x', { status: s })
      return ok(opts.sourceBody ?? '# source\n')
    }
    return new Response('not found', { status: 404 })
  })
}

let createSpy: ReturnType<typeof vi.spyOn>
let revokeSpy: ReturnType<typeof vi.spyOn>

beforeEach(() => {
  let n = 0
  createSpy = vi.spyOn(URL, 'createObjectURL').mockImplementation(() => `blob:url-${++n}`)
  revokeSpy = vi.spyOn(URL, 'revokeObjectURL').mockImplementation(() => {})
})

afterEach(() => {
  vi.restoreAllMocks()
  vi.unstubAllGlobals()
})

describe('loadFromServerDir — happy path', () => {
  it('all-200 → LoadedDirectory with plan, manifest, audioUrl, source, no warnings', async () => {
    vi.stubGlobal('fetch', stubFetch())
    const got = await loadFromServerDir(DIR, inputs())
    expect(got.plan.plan_id).toBe('p1')
    expect(got.manifest.voice).toBe('af_bella')
    expect(got.audioUrl).toMatch(/^blob:/)
    expect(got.source).toBe('# source\n')
    expect(got.warnings).toEqual([])
  })

  it('resolves all four URLs against the server /artifact route for the typed dir', async () => {
    const fn = stubFetch()
    vi.stubGlobal('fetch', fn)
    await loadFromServerDir(DIR, inputs())
    const urls = fn.mock.calls.map((c) => c[0].toString())
    for (const name of ['plan.json', 'manifest.json', 'audio.wav', 'source.md']) {
      const hit = urls.find((u) => u.includes(`name=${name}`))
      expect(hit, `expected /artifact fetch for ${name}`).toBeTruthy()
      expect(hit!).toContain('dir=%2Fabs%2Fout')
    }
  })
})

describe('loadFromServerDir — optional source.md (Decision v2 resilience)', () => {
  it('source 404 → source:null, NO warning', async () => {
    vi.stubGlobal('fetch', stubFetch({ sourceStatus: 404 }))
    const got = await loadFromServerDir(DIR, inputs())
    expect(got.source).toBeNull()
    expect(got.warnings).toEqual([])
  })

  it('source 500 → source:null + "source unavailable" warning, required files still load', async () => {
    vi.stubGlobal('fetch', stubFetch({ sourceStatus: 500 }))
    const got = await loadFromServerDir(DIR, inputs())
    expect(got.source).toBeNull()
    expect(got.plan.plan_id).toBe('p1')
    expect(got.audioUrl).toMatch(/^blob:/)
    expect(got.warnings.some((w) => w.startsWith('source unavailable'))).toBe(true)
  })

  it('source network throw → source:null + warning, never aborts the load', async () => {
    vi.stubGlobal('fetch', stubFetch({ sourceThrows: true }))
    const got = await loadFromServerDir(DIR, inputs())
    expect(got.source).toBeNull()
    expect(got.warnings.some((w) => w.startsWith('source unavailable'))).toBe(true)
  })
})

describe('loadFromServerDir — required-file failures abort with a precise error', () => {
  it('plan.json non-OK → throws naming plan.json', async () => {
    vi.stubGlobal('fetch', stubFetch({ planStatus: 404 }))
    await expect(loadFromServerDir(DIR, inputs())).rejects.toThrow(/plan\.json.*404/)
  })

  it('manifest.json non-OK → throws naming manifest.json', async () => {
    vi.stubGlobal('fetch', stubFetch({ manifestStatus: 500 }))
    await expect(loadFromServerDir(DIR, inputs())).rejects.toThrow(/manifest\.json.*500/)
  })

  it('audio.wav non-OK → throws naming audio.wav', async () => {
    vi.stubGlobal('fetch', stubFetch({ audioStatus: 404 }))
    await expect(loadFromServerDir(DIR, inputs())).rejects.toThrow(/audio\.wav.*404/)
  })

  it('does NOT create an object URL when a required file fails before the audio blob', async () => {
    vi.stubGlobal('fetch', stubFetch({ planStatus: 404 }))
    await expect(loadFromServerDir(DIR, inputs())).rejects.toThrow()
    expect(createSpy).not.toHaveBeenCalled()
    expect(revokeSpy).not.toHaveBeenCalled()
  })
})

describe('loadFromServerDir — schema mismatch warns, never throws', () => {
  it('plan schema_version major drift → warning, still loads', async () => {
    vi.stubGlobal(
      'fetch',
      stubFetch({ planBody: JSON.stringify({ ...PLAN, schema_version: '2' }) }),
    )
    const got = await loadFromServerDir(DIR, inputs())
    expect(got.warnings.some((w) => w.includes('schema_version'))).toBe(true)
    expect(got.plan).toBeTruthy()
  })

  it('invalid plan JSON → throws (parse error is a hard fail, not a warning)', async () => {
    vi.stubGlobal('fetch', stubFetch({ planBody: 'not json {' }))
    await expect(loadFromServerDir(DIR, inputs())).rejects.toThrow(/plan\.json is not valid JSON/)
  })
})
