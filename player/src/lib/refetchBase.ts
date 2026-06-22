// refetchBase resolves WHERE the player re-reads patched artifacts (audio.wav,
// manifest.json) from after an in-place escalate (#62).
//
// Two modes:
//   - server mode: the patched output lives at the user-supplied `dir`, an
//     arbitrary path the escalate server can statically serve. The artifact URL
//     is `<serverBaseUrl>/artifact?dir=<encoded dir>&name=<artifact>` — the
//     #62 GET /artifact route. The dir is URL-encoded so spaces / special chars
//     survive the query string.
//   - fixture mode (or server mode with no dir yet): the bundled fixture is
//     served same-origin under FIXTURE_BASE, so the URL is
//     `<FIXTURE_BASE>/<artifact>`.
//
// This is a PURE function of (serverMode, dir, serverBaseUrl, fixtureBase). The
// App reads serverMode + dir from live refs at CALL time (not render time) so a
// dir typed after the initial render is honored without a stale closure.

export interface RefetchBaseInputs {
  serverMode: boolean
  dir: string
  serverBaseUrl: string
  fixtureBase: string
}

// artifactUrl builds the full URL to re-fetch one artifact (e.g. "audio.wav" or
// "manifest.json"). The caller already knows the artifact name; this centralizes
// the server-vs-fixture origin + query/path shape so both re-fetch call sites
// resolve identically.
export function artifactUrl(name: string, inputs: RefetchBaseInputs): string {
  const { serverMode, dir, serverBaseUrl, fixtureBase } = inputs
  if (serverMode && dir !== '') {
    const base = serverBaseUrl.replace(/\/$/, '')
    return `${base}/artifact?dir=${encodeURIComponent(dir)}&name=${encodeURIComponent(name)}`
  }
  const base = fixtureBase.replace(/\/$/, '')
  return `${base}/${name}`
}
