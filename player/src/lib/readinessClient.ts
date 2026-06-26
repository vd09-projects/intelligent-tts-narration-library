import { POLL_FETCH_TIMEOUT_MS } from './readinessConfig.ts'

// readinessClient — the only module that talks to GET /readiness. Pure (no
// React), fully mockable via a stubbed global fetch, mirroring
// escalateClient.checkHealth. It maps the server's render-handshake surface
// (cmd/narrate-server readinessHandler, #76) into a discriminated result the
// hook switches on:
//
//   200 {status:"rendered"}        → {kind:'rendered'}
//   200 {status:"rendering"}       → {kind:'rendering'}
//   404 {reason:"no_out_dir"}      → {kind:'failed', reason:'no_out_dir'}
//   transport reject/abort/malformed/anything else → {kind:'unreachable'}
//
// The collapse of every transport fault (fetch reject, AbortController timeout,
// CORS, malformed body, unexpected status) into 'unreachable' is deliberate: the
// hook counts unreachables toward its give-up bound, so a dead server from poll
// #1 is just a fast run of 'unreachable' attempts — never an unhandled throw and
// never a perpetual spinner.

export type ReadinessResult =
  | { kind: 'rendered' }
  | { kind: 'rendering' }
  | { kind: 'failed'; reason: string }
  | { kind: 'unreachable' }

export async function getReadiness(
  baseUrl: string,
  dir: string,
): Promise<ReadinessResult> {
  let res: Response
  try {
    res = await fetchWithTimeout(
      `${trimTrailingSlash(baseUrl)}/readiness?dir=${encodeURIComponent(dir)}`,
      { method: 'GET' },
      POLL_FETCH_TIMEOUT_MS,
    )
  } catch {
    // fetch reject (network down / CORS) or AbortError (timeout). Counted as an
    // 'unreachable' attempt by the hook — kept off the server enum on purpose.
    return { kind: 'unreachable' }
  }

  if (!res.ok) {
    // The only DEFINED terminal token on a non-200 is no_out_dir (404). Any other
    // error status / unparseable error body collapses to 'unreachable' so the
    // give-up bound still governs (never an unbounded special case).
    const reason = await readReason(res)
    if (reason === 'no_out_dir') {
      return { kind: 'failed', reason }
    }
    return { kind: 'unreachable' }
  }

  let body: { status?: string }
  try {
    body = (await res.json()) as { status?: string }
  } catch {
    return { kind: 'unreachable' }
  }
  if (body.status === 'rendered') return { kind: 'rendered' }
  if (body.status === 'rendering') return { kind: 'rendering' }
  // A 200 with an unrecognized status is a malformed handshake → treat as a
  // (bounded) unreachable rather than inventing a phase.
  return { kind: 'unreachable' }
}

// readReason pulls the {reason} token out of the single ErrorResponse envelope.
// A missing/unparseable body yields '' so the caller falls through to
// 'unreachable'.
async function readReason(res: Response): Promise<string> {
  try {
    const body = (await res.json()) as { reason?: string }
    return typeof body.reason === 'string' ? body.reason : ''
  } catch {
    return ''
  }
}

// DUP: this AbortController+timeout helper is identical to escalateClient.ts's
// private fetchWithTimeout; extract to a shared player/src/lib/fetchWithTimeout.ts
// (out of scope for #76 — would touch the unmodified escalate client).
//
// fetchWithTimeout wraps fetch in an AbortController that fires after timeoutMs.
// An aborted fetch rejects, which getReadiness translates into 'unreachable'.
// (Mirrors escalateClient's private helper — kept local so readinessClient stays
// a self-contained, separately-testable wire module.)
async function fetchWithTimeout(
  url: string,
  init: RequestInit,
  timeoutMs: number,
): Promise<Response> {
  const controller = new AbortController()
  const timer = setTimeout(() => controller.abort(), timeoutMs)
  try {
    return await fetch(url, { ...init, signal: controller.signal })
  } finally {
    clearTimeout(timer)
  }
}

function trimTrailingSlash(url: string): string {
  return url.endsWith('/') ? url.slice(0, -1) : url
}
