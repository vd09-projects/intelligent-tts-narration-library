import type { Block, Level, Refusal } from '../types/plan.ts'
import type {
  BlockTiming,
  EscalateErrorBody,
  EscalateReason,
  EscalateSuccess,
} from '../types/manifest.ts'
import { ESCALATE_TIMEOUT_MS, HEALTHZ_TIMEOUT_MS } from './escalateConfig.ts'

// escalateClient — the only module that talks to cmd/narrate-server. Pure
// (no React), fully mockable via a stubbed global fetch.
//
// The server contract (verified against cmd/narrate-server/main.go):
//   POST /escalate {dir, block_id, level}  (snake_case)
//     200 voiced/degraded → {block, timing, audio_ref}
//     200 refused         → {block, refusal}  (timing/audio_ref ABSENT)
//     non-2xx             → {reason, message} (single ErrorResponse envelope)
//   GET  /healthz → 200 {"status":"ok"}
//
// Both calls are wrapped in try/catch + an AbortController timeout. Any
// fetch-reject / abort / CORS failure collapses to the client-only
// reason:'unreachable' — the network is just another error class to the row.

export interface EscalateArgs {
  dir: string
  blockId: string
  level: Level
}

// EscalateResult is the discriminated union the hook switches on. 'patched'
// carries the new spoken block + timing + audio_ref; 'refused' carries the
// block + refusal; 'error' carries the reason token (server enum ∪
// 'unreachable') + a human message + the HTTP status when there was one.
export type EscalateResult =
  // `audioRef` is the top-level wire `audio_ref`; the value the player actually
  // consumes flows through `timing.audio_ref` into BlockTiming/manifest. The
  // top-level field is retained here for parity with the server wire shape
  // (debug / fidelity) — consumers should read `timing.audio_ref`.
  | { kind: 'patched'; block: Block; timing: BlockTiming; audioRef: string }
  | { kind: 'refused'; block: Block; refusal: Refusal }
  | { kind: 'error'; reason: EscalateReason; message: string; status?: number }

// checkHealth probes GET {baseUrl}/healthz with a short timeout. Returns true
// only on a 200 whose body is {status:"ok"}; everything else (non-200, reject,
// timeout, malformed body) is false — the caller treats false as "no server".
export async function checkHealth(baseUrl: string): Promise<boolean> {
  try {
    const res = await fetchWithTimeout(
      `${trimTrailingSlash(baseUrl)}/healthz`,
      { method: 'GET' },
      HEALTHZ_TIMEOUT_MS,
    )
    if (!res.ok) return false
    const body = (await res.json()) as { status?: string }
    return body.status === 'ok'
  } catch {
    // Reject / abort / CORS / malformed JSON — server is not reachable.
    return false
  }
}

// postEscalate issues POST {baseUrl}/escalate and maps the response into an
// EscalateResult. It never throws: transport faults become
// {kind:'error', reason:'unreachable'}.
export async function postEscalate(
  baseUrl: string,
  { dir, blockId, level }: EscalateArgs,
): Promise<EscalateResult> {
  let res: Response
  try {
    res = await fetchWithTimeout(
      `${trimTrailingSlash(baseUrl)}/escalate`,
      {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ dir, block_id: blockId, level }),
      },
      ESCALATE_TIMEOUT_MS,
    )
  } catch (e) {
    // fetch reject (network down / CORS) or AbortError (timeout). Both are the
    // client-only 'unreachable' class — kept off the server enum on purpose.
    return {
      kind: 'error',
      reason: 'unreachable',
      message: unreachableMessage(e),
    }
  }

  if (!res.ok) {
    const body = await readErrorBody(res)
    return {
      kind: 'error',
      reason: body.reason,
      message: body.message,
      status: res.status,
    }
  }

  let success: EscalateSuccess
  try {
    success = (await res.json()) as EscalateSuccess
  } catch (e) {
    return {
      kind: 'error',
      reason: 'readback_failed',
      message: `escalate succeeded (HTTP ${res.status}) but the response body was not valid JSON: ${asMessage(e)}`,
      status: res.status,
    }
  }

  // refused → block + refusal present, timing/audio_ref absent.
  if (success.refusal) {
    return { kind: 'refused', block: success.block, refusal: success.refusal }
  }

  // voiced/degraded → block + timing + audio_ref present.
  if (success.timing && typeof success.audio_ref === 'string') {
    return {
      kind: 'patched',
      block: success.block,
      timing: success.timing,
      audioRef: success.audio_ref,
    }
  }

  // 200 with neither a refusal nor a complete timing/audio_ref. Treat as an
  // internal contract violation rather than silently dropping the patch — the
  // row reflects exactly what the server returned (honesty rule).
  return {
    kind: 'error',
    reason: 'internal',
    message:
      'escalate returned 200 but the body was neither a refusal nor a complete voiced/degraded patch (missing timing or audio_ref).',
    status: res.status,
  }
}

// readErrorBody parses the single {reason, message} envelope. If the body is
// missing or malformed (a proxy 502 page, say), it synthesizes a generic
// 'internal' error tagged with the status so the row still shows something.
async function readErrorBody(res: Response): Promise<EscalateErrorBody> {
  try {
    const body = (await res.json()) as Partial<EscalateErrorBody>
    if (typeof body.reason === 'string' && typeof body.message === 'string') {
      return { reason: body.reason as EscalateReason, message: body.message }
    }
  } catch {
    // fall through to the synthesized envelope
  }
  return {
    reason: 'internal',
    message: `escalate server returned HTTP ${res.status} with no usable error body`,
  }
}

// fetchWithTimeout wraps fetch in an AbortController that fires after timeoutMs.
// An aborted fetch rejects, which the callers translate into 'unreachable'.
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

function unreachableMessage(e: unknown): string {
  if (e instanceof Error && e.name === 'AbortError') {
    return 'Escalate request timed out — the server did not respond in time.'
  }
  return `Escalate server not reachable: ${asMessage(e)}`
}

function asMessage(e: unknown): string {
  return e instanceof Error ? e.message : String(e)
}

function trimTrailingSlash(url: string): string {
  return url.endsWith('/') ? url.slice(0, -1) : url
}
