import { useCallback, useEffect, useRef, useState } from 'react'
import { checkHealth } from '../lib/escalateClient.ts'
import { ESCALATE_BASE_URL } from '../lib/escalateConfig.ts'

// useServerMode probes the escalate server's /healthz on mount and exposes the
// resulting mode. Until the first probe resolves the mode is 'unknown' so
// BlockRow can render neutral controls (no flicker between fixture + server
// affordances).
//
//   'unknown' → first /healthz has not resolved yet
//   'server'  → /healthz returned 200 {status:"ok"} (live escalation available)
//   'fixture' → /healthz rejected / non-200 / timed out (copy-command fallback)
//
// recheck() re-probes /healthz WITHOUT touching plan/manifest state — it only
// flips this hook's mode. The App owns plan/manifest separately, so a recheck
// never clobbers a patched block.

export type ServerMode = 'unknown' | 'server' | 'fixture'

export interface ServerModeApi {
  mode: ServerMode
  baseUrl: string
  healthChecked: boolean
  recheck: () => void
}

export function useServerMode(
  baseUrl: string = ESCALATE_BASE_URL,
): ServerModeApi {
  const [mode, setMode] = useState<ServerMode>('unknown')
  const [healthChecked, setHealthChecked] = useState(false)
  // A monotonically increasing token guards against a slow stale probe
  // resolving after a newer one (recheck races / StrictMode double-invoke).
  const probeSeq = useRef(0)

  const probe = useCallback(() => {
    const seq = ++probeSeq.current
    void checkHealth(baseUrl).then((ok) => {
      if (seq !== probeSeq.current) return // a newer probe superseded this one
      setMode(ok ? 'server' : 'fixture')
      setHealthChecked(true)
    })
  }, [baseUrl])

  useEffect(() => {
    probe()
    // We intentionally do NOT clear mode here on unmount: under StrictMode the
    // double-invoke just runs probe() twice; the seq guard keeps the last one.
  }, [probe])

  const recheck = useCallback(() => {
    probe()
  }, [probe])

  return { mode, baseUrl, healthChecked, recheck }
}
