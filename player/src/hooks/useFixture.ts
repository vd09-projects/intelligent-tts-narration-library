import { useEffect, useRef, useState } from 'react'
import type { Manifest } from '../types/manifest.ts'
import type { NarrationPlan } from '../types/plan.ts'
import {
  EXPECTED_MANIFEST_SCHEMA,
  EXPECTED_PLAN_SCHEMA,
  type LoadedDirectory,
} from '../lib/loadDirectory.ts'

// useFixture loads the bundled fixture from /fixtures/sample/. Used on first
// mount so the player has something to show without prompting for a
// directory; the user can then click "Load directory…" to swap in a real
// sink/persistent output (plan D2).
//
// We fetch + parse the three required files in parallel and create a blob
// URL for audio.wav. On unmount the URL is revoked (R3 blob URL leak).
export interface FixtureState {
  loading: boolean
  data: LoadedDirectory | null
  error: string | null
}

const FIXTURE_BASE = '/fixtures/sample'

export function useFixture(): FixtureState {
  const [state, setState] = useState<FixtureState>({
    loading: true,
    data: null,
    error: null,
  })
  const revokeRef = useRef<string | null>(null)

  useEffect(() => {
    let cancelled = false
    void (async () => {
      try {
        const [planRes, manifestRes, sourceRes, audioRes] = await Promise.all([
          fetch(`${FIXTURE_BASE}/plan.json`),
          fetch(`${FIXTURE_BASE}/manifest.json`),
          fetch(`${FIXTURE_BASE}/source.md`),
          fetch(`${FIXTURE_BASE}/audio.wav`),
        ])
        if (!planRes.ok) throw new Error(`plan.json HTTP ${planRes.status}`)
        if (!manifestRes.ok) throw new Error(`manifest.json HTTP ${manifestRes.status}`)
        if (!audioRes.ok) throw new Error(`audio.wav HTTP ${audioRes.status}`)
        // source.md is optional (D1 + honesty rule).

        const plan = (await planRes.json()) as NarrationPlan
        const manifest = (await manifestRes.json()) as Manifest
        const source = sourceRes.ok ? await sourceRes.text() : null
        const audioBlob = new Blob([await audioRes.arrayBuffer()], {
          type: 'audio/wav',
        })
        const audioUrl = URL.createObjectURL(audioBlob)
        if (cancelled) {
          URL.revokeObjectURL(audioUrl)
          return
        }
        if (revokeRef.current) URL.revokeObjectURL(revokeRef.current)
        revokeRef.current = audioUrl

        setState({
          loading: false,
          data: {
            plan,
            manifest,
            audioUrl,
            source,
            warnings: collectFixtureWarnings(plan, manifest),
          },
          error: null,
        })
      } catch (e) {
        if (cancelled) return
        setState({
          loading: false,
          data: null,
          error: (e as Error).message,
        })
      }
    })()
    return () => {
      cancelled = true
      if (revokeRef.current) {
        URL.revokeObjectURL(revokeRef.current)
        revokeRef.current = null
      }
    }
  }, [])

  return state
}

function collectFixtureWarnings(plan: NarrationPlan, manifest: Manifest): string[] {
  const w: string[] = []
  if (plan.schema_version && plan.schema_version !== EXPECTED_PLAN_SCHEMA) {
    w.push(`plan.json schema_version ${plan.schema_version} differs from expected ${EXPECTED_PLAN_SCHEMA}.`)
  }
  if (typeof manifest.schema_version === 'number' && manifest.schema_version !== EXPECTED_MANIFEST_SCHEMA) {
    w.push(`manifest.json schema_version ${manifest.schema_version} differs from expected ${EXPECTED_MANIFEST_SCHEMA}.`)
  }
  if (manifest.stale) {
    w.push(`manifest.stale = true (${manifest.stale_reason ?? 'no reason given'}).`)
  }
  return w
}
