import { useCallback, useEffect, useRef, useState } from 'react'
import type { Level } from '../types/plan.ts'
import type { EscalateReason } from '../types/manifest.ts'
import { postEscalate, type EscalateResult } from '../lib/escalateClient.ts'

// useEscalation owns the per-row escalation state machine and the patch flow.
// It does NOT own plan/manifest — those live in the App so a patch survives a
// /healthz recheck. The hook calls back into the App via onPatched / onRefused
// / reloadManifest.
//
// Per-row state:
//   idle    — no escalation in flight (default; absent from the map)
//   loading — request in flight (spinner on the row)
//   error   — server error envelope OR transport 'unreachable'
//   refused — server returned a refusal (RefusalBadge inline)
//   noop    — content-identical 200 ("no visible change" ack — not an error)
//
// STALE-CLOSURE AVOIDANCE (review item 1): escalate() is keyed on blockId only
// and is a stable useCallback (empty deps). It reads the mutable dir/baseUrl and
// the patch callbacks through a ref that the App refreshes every render, NOT via
// closure capture. So changing the dir field after first render and THEN
// escalating submits the NEW dir.

export type RowStatus = 'idle' | 'loading' | 'error' | 'refused' | 'noop'

export interface RowState {
  status: RowStatus
  error?: { reason: EscalateReason; message: string }
  // staleDownstream: a committed patch whose post-patch manifest re-fetch
  // failed. The patch is RETAINED; this flag drives a soft inline notice. Never
  // a rollback (plan step d8).
  staleDownstream?: boolean
}

// EscalationContext is the live, mutable context the App refreshes each render.
// The hook reads it through a ref so callbacks never capture a stale dir.
export interface EscalationContext {
  baseUrl: string
  dir: string
  // onPatched commits a voiced/degraded patch into App-owned plan/manifest and
  // reports back two facts:
  //   refetchOk — whether the post-patch manifest re-fetch succeeded. false →
  //     the patch committed but downstream offsets could not be refreshed, so
  //     the row gets staleDownstream (patch retained, never rolled back).
  //   changed   — whether the patched block actually differed from what was
  //     already in state. The server has no content-identical short-circuit
  //     (it re-renders), so a same-level escalate returns 200 with identical
  //     bytes. changed === false drives the "no visible change" ack (plan d5).
  onPatched: (
    blockId: string,
    result: Extract<EscalateResult, { kind: 'patched' }>,
  ) => Promise<{ refetchOk: boolean; changed: boolean }>
  // onRefused commits a refused patch (status + refusal) into App-owned plan.
  onRefused: (blockId: string, result: Extract<EscalateResult, { kind: 'refused' }>) => void
}

export interface EscalationApi {
  rowState: (blockId: string) => RowState
  escalate: (blockId: string, level: Level) => Promise<void>
}

const IDLE: RowState = { status: 'idle' }

export function useEscalation(context: EscalationContext): EscalationApi {
  const [rows, setRows] = useState<Map<string, RowState>>(new Map())

  // Keep the latest context in a ref so escalate() (stable identity) always
  // reads the current dir/baseUrl/callbacks — never a stale closure capture.
  // Synced in an effect (not during render) per React 19's "no ref writes
  // during render" rule. escalate() fires on user interaction (post-commit),
  // so the effect has always committed the latest context before it runs.
  const ctxRef = useRef(context)
  useEffect(() => {
    ctxRef.current = context
  }, [context])

  const setRow = useCallback((blockId: string, next: RowState) => {
    setRows((prev) => {
      const m = new Map(prev)
      m.set(blockId, next)
      return m
    })
  }, [])

  const rowState = useCallback(
    (blockId: string): RowState => rows.get(blockId) ?? IDLE,
    [rows],
  )

  const escalate = useCallback(
    async (blockId: string, level: Level): Promise<void> => {
      const { baseUrl, dir, onPatched, onRefused } = ctxRef.current
      setRow(blockId, { status: 'loading' })
      try {
        const result = await postEscalate(baseUrl, { dir, blockId, level })
        switch (result.kind) {
          case 'patched': {
            const { refetchOk, changed } = await onPatched(blockId, result)
            if (!refetchOk) {
              // Patch committed but the re-fetch failed — keep the patch, flag
              // staleDownstream (soft notice). Never roll back (plan d8).
              setRow(blockId, { status: 'idle', staleDownstream: true })
            } else if (!changed) {
              // Content-identical 200 (same-level re-render). Not a silent
              // no-op, not an error — a quiet "no visible change" ack (plan d5).
              setRow(blockId, { status: 'noop' })
            } else {
              setRow(blockId, IDLE)
            }
            return
          }
          case 'refused': {
            onRefused(blockId, result)
            setRow(blockId, { status: 'refused' })
            return
          }
          case 'error': {
            setRow(blockId, {
              status: 'error',
              error: { reason: result.reason, message: result.message },
            })
            return
          }
        }
      } catch (e) {
        // Defensive: postEscalate is contracted never to throw, but if some
        // future change does, the row must clear its spinner and show an error
        // rather than hang on 'loading'.
        setRow(blockId, {
          status: 'error',
          error: {
            reason: 'internal',
            message: e instanceof Error ? e.message : String(e),
          },
        })
      }
    },
    [setRow],
  )

  return { rowState, escalate }
}
