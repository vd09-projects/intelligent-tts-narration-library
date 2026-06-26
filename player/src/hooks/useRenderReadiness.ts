import { useEffect, useState } from 'react'
import { getReadiness } from '../lib/readinessClient.ts'
import {
  MAX_POLL_ATTEMPTS,
  POLL_INTERVAL_MS,
  UNREACHABLE_FASTFAIL,
} from '../lib/readinessConfig.ts'

// useRenderReadiness is the bounded poll loop + give-up state machine for the
// companion player (#76). It converts the server's on-disk readiness handshake
// into ONE of three resolved UI phases — never a perpetual spinner:
//
//   'idle'      — companion mode OFF (enabled=false / no dir). No polling.
//   'rendering' — polling; server says rendering, or we are retrying an
//                 unreachable server within the give-up bound. (spinner)
//   'rendered'  — terminal: server reported a complete triple. (load the plan)
//   'failed'    — terminal: the single DEFINED render-failure state, reached from
//                 any of: server no_out_dir; UNREACHABLE_FASTFAIL consecutive
//                 transport rejects (dead server); MAX_POLL_ATTEMPTS exhausted
//                 while still rendering. (failure banner + handoff hint)
//
// Dead-server invariant (AC4): transport rejects are COUNTED attempts, so the
// bound holds regardless of whether the server ever answered. The loop is a
// single self-scheduling timer (mirroring useServerMode's seq-guarded probe, not
// a chain of reactive effects) and tears its timer down on unmount.

export type ReadinessPhase = 'idle' | 'rendering' | 'rendered' | 'failed'

export interface RenderReadinessInput {
  baseUrl: string
  dir: string
  enabled: boolean
}

export interface RenderReadinessState {
  phase: ReadinessPhase
  // reason is the machine-stable cause when phase==='failed' (else null):
  //   'no_out_dir'        — server reported the dir does not exist.
  //   'server_unreachable'— UNREACHABLE_FASTFAIL consecutive transport rejects.
  //   'max_poll_attempts' — the absolute attempt cap was reached while rendering.
  reason: string | null
  attempts: number
}

export function useRenderReadiness({
  baseUrl,
  dir,
  enabled,
}: RenderReadinessInput): RenderReadinessState {
  // Companion mode OFF (disabled / no dir) → stay idle, never poll. This
  // inertness is the AC6 guardrail in code: the default fixture/picker path fires
  // no readiness poll.
  const polling = enabled && dir !== ''

  const [state, setState] = useState<RenderReadinessState>(() => ({
    phase: polling ? 'rendering' : 'idle',
    reason: null,
    attempts: 0,
  }))

  // Reset derived state when the poll target changes, using the "adjust state
  // during render" idiom (the same pattern App's seed guard + usePlayback use).
  // Done here — NOT via a synchronous setState inside the effect — so the loop
  // effect only ever writes state from inside its async tick (mirroring
  // useServerMode's .then(setMode)), keeping react-hooks/set-state-in-effect happy.
  const targetKey = `${baseUrl}|${dir}|${enabled}`
  const [trackedKey, setTrackedKey] = useState(targetKey)
  if (targetKey !== trackedKey) {
    setTrackedKey(targetKey)
    setState({ phase: polling ? 'rendering' : 'idle', reason: null, attempts: 0 })
  }

  useEffect(() => {
    if (!polling) return

    let cancelled = false
    let timer: ReturnType<typeof setTimeout> | null = null
    let attempts = 0
    let consecutiveUnreachable = 0

    const tick = async (): Promise<void> => {
      if (cancelled) return
      const result = await getReadiness(baseUrl, dir)
      if (cancelled) return

      attempts += 1

      switch (result.kind) {
        case 'rendered':
          setState({ phase: 'rendered', reason: null, attempts })
          return // terminal — no reschedule
        case 'failed':
          // Server's defined terminal token (no_out_dir) → immediate render_failed.
          setState({ phase: 'failed', reason: result.reason, attempts })
          return // terminal
        case 'unreachable':
          consecutiveUnreachable += 1
          if (consecutiveUnreachable >= UNREACHABLE_FASTFAIL) {
            setState({ phase: 'failed', reason: 'server_unreachable', attempts })
            return // terminal (dead-server fast path)
          }
          break
        case 'rendering':
          consecutiveUnreachable = 0
          break
      }

      // Still in-flight (rendering, or an unreachable below the fast-fail bound):
      // honor the absolute cap, else schedule the next poll one interval later.
      if (attempts >= MAX_POLL_ATTEMPTS) {
        setState({ phase: 'failed', reason: 'max_poll_attempts', attempts })
        return // terminal (bound expired)
      }
      setState({ phase: 'rendering', reason: null, attempts })
      timer = setTimeout(() => void tick(), POLL_INTERVAL_MS)
    }

    void tick()

    return () => {
      cancelled = true
      if (timer) clearTimeout(timer)
    }
  }, [baseUrl, dir, polling])

  return state
}
