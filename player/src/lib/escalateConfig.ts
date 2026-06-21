// escalateConfig centralizes the escalate-server base URL so it is read in ONE
// place (the typed-config rule) rather than scattered import.meta.env reads in
// business logic. Q1: baseUrl comes from the Vite env VITE_ESCALATE_BASE_URL,
// defaulting to the loopback the server binds by default.
//
// escalateClient takes baseUrl as a typed parameter (no hardcoding inside the
// client) — this module supplies the default the App threads through.
const DEFAULT_ESCALATE_BASE_URL = 'http://127.0.0.1:8080'

// import.meta.env is statically replaced by Vite at build time; under vitest it
// is also defined. We narrow defensively so a missing/empty value falls back.
function readBaseUrl(): string {
  const env = import.meta.env as Record<string, string | undefined>
  const fromEnv = env.VITE_ESCALATE_BASE_URL
  return fromEnv && fromEnv.length > 0 ? fromEnv : DEFAULT_ESCALATE_BASE_URL
}

export const ESCALATE_BASE_URL: string = readBaseUrl()

// HEALTHZ_TIMEOUT_MS / ESCALATE_TIMEOUT_MS bound the two fetches so a dead
// server surfaces as 'unreachable' quickly (health) and a hung render surfaces
// as 'unreachable' eventually (escalate). The server's own request-timeout is
// 120s; we give escalate a little more headroom than that on the client so the
// server's own 408 'cancelled' wins the race when the render is the slow part.
export const HEALTHZ_TIMEOUT_MS = 1500
export const ESCALATE_TIMEOUT_MS = 130_000
