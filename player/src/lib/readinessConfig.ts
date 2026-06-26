// readinessConfig centralizes the companion-mode render-handshake tuning in ONE
// place (the typed-config rule), mirroring escalateConfig.ts. All four constants
// are Vite-env-overridable (VITE_READINESS_*) so a pathological doc can widen the
// bound without a code change; the companion target dir comes from
// VITE_COMPANION_DIR (read at call time so it stays test-stubbable).
//
// These drive useRenderReadiness's bounded poll loop:
//   POLL_INTERVAL_MS       — 1 Hz against the loopback server; gentle, and a ~1 s
//                            completion-latency is invisible for a hand-launched
//                            companion.
//   POLL_FETCH_TIMEOUT_MS  — per-poll AbortController budget (reuses the
//                            HEALTHZ_TIMEOUT_MS precedent). A dead server's poll
//                            rejects quickly so it counts as a failed attempt.
//   MAX_POLL_ATTEMPTS      — absolute cap (~2 min wall), sitting at the server's
//                            own 120 s --request-timeout ceiling. A render that
//                            produced no complete triple by then is "never coming".
//   UNREACHABLE_FASTFAIL   — consecutive transport rejects that trip an immediate
//                            render_failed (~≤20 s) without waiting out the full
//                            cap. Covers "server process is gone from poll #1".

const DEFAULT_POLL_INTERVAL_MS = 1000
const DEFAULT_POLL_FETCH_TIMEOUT_MS = 1500
const DEFAULT_MAX_POLL_ATTEMPTS = 120
const DEFAULT_UNREACHABLE_FASTFAIL = 8

// readPositiveInt narrows a Vite-env override to a positive integer; a
// missing/empty/non-numeric/≤0 value falls back to the default (defensive, same
// posture escalateConfig takes on its base-url read).
function readPositiveInt(key: string, fallback: number): number {
  const env = import.meta.env as Record<string, string | undefined>
  const raw = env[key]
  if (!raw) return fallback
  const n = Number.parseInt(raw, 10)
  return Number.isFinite(n) && n > 0 ? n : fallback
}

export const POLL_INTERVAL_MS: number = readPositiveInt(
  'VITE_READINESS_POLL_INTERVAL_MS',
  DEFAULT_POLL_INTERVAL_MS,
)
export const POLL_FETCH_TIMEOUT_MS: number = readPositiveInt(
  'VITE_READINESS_POLL_FETCH_TIMEOUT_MS',
  DEFAULT_POLL_FETCH_TIMEOUT_MS,
)
export const MAX_POLL_ATTEMPTS: number = readPositiveInt(
  'VITE_READINESS_MAX_POLL_ATTEMPTS',
  DEFAULT_MAX_POLL_ATTEMPTS,
)
export const UNREACHABLE_FASTFAIL: number = readPositiveInt(
  'VITE_READINESS_UNREACHABLE_FASTFAIL',
  DEFAULT_UNREACHABLE_FASTFAIL,
)

// readCompanionDir returns the VITE_COMPANION_DIR target (the one-click
// `make run-companion` handoff). Empty string means companion mode is OFF — the
// player then behaves exactly as today (bundled fixture + manual picker, no
// readiness machinery). Read at CALL time (not a module-load const) so tests can
// vi.stubEnv it and the App stays the single reader.
export function readCompanionDir(): string {
  const env = import.meta.env as Record<string, string | undefined>
  const dir = env.VITE_COMPANION_DIR
  return dir && dir.length > 0 ? dir : ''
}
