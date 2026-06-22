import { describe, expect, it } from 'vitest'
import { artifactUrl, type RefetchBaseInputs } from '../src/lib/refetchBase.ts'

const FIXTURE = '/fixtures/sample'
const SERVER = 'http://127.0.0.1:8080'

function inputs(over: Partial<RefetchBaseInputs> = {}): RefetchBaseInputs {
  return {
    serverMode: false,
    dir: '',
    serverBaseUrl: SERVER,
    fixtureBase: FIXTURE,
    ...over,
  }
}

describe('artifactUrl — server mode', () => {
  it('builds /artifact?dir=&name= against the server base URL', () => {
    const url = artifactUrl('audio.wav', inputs({ serverMode: true, dir: '/abs/out' }))
    expect(url).toBe(`${SERVER}/artifact?dir=%2Fabs%2Fout&name=audio.wav`)
  })

  it('URL-encodes a dir containing spaces / special chars', () => {
    const url = artifactUrl('manifest.json', inputs({ serverMode: true, dir: '/abs/my out dir' }))
    expect(url).toBe(`${SERVER}/artifact?dir=%2Fabs%2Fmy%20out%20dir&name=manifest.json`)
  })

  it('trims a trailing slash on the server base URL', () => {
    const url = artifactUrl('audio.wav', inputs({ serverMode: true, dir: '/d', serverBaseUrl: `${SERVER}/` }))
    expect(url).toBe(`${SERVER}/artifact?dir=%2Fd&name=audio.wav`)
  })
})

describe('artifactUrl — fixture mode', () => {
  it('returns FIXTURE_BASE/<name> when not in server mode', () => {
    expect(artifactUrl('audio.wav', inputs())).toBe(`${FIXTURE}/audio.wav`)
    expect(artifactUrl('manifest.json', inputs())).toBe(`${FIXTURE}/manifest.json`)
  })

  it('falls back to fixture when serverMode is true but dir is empty', () => {
    const url = artifactUrl('manifest.json', inputs({ serverMode: true, dir: '' }))
    expect(url).toBe(`${FIXTURE}/manifest.json`)
  })
})

describe('artifactUrl — purity / call-time inputs', () => {
  it('reflects a dir change between calls (no captured state)', () => {
    const first = artifactUrl('audio.wav', inputs({ serverMode: true, dir: '/first' }))
    const second = artifactUrl('audio.wav', inputs({ serverMode: true, dir: '/second' }))
    expect(first).toContain('dir=%2Ffirst')
    expect(second).toContain('dir=%2Fsecond')
    expect(first).not.toBe(second)
  })
})
