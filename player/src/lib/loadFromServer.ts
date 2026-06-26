import type { Manifest } from '../types/manifest.ts'
import type { NarrationPlan } from '../types/plan.ts'
import { collectWarnings, type LoadedDirectory } from './loadDirectory.ts'
import { artifactUrl } from './refetchBase.ts'

// loadFromServer — the HTTP analogue of loadDirectory's resolveFiles, for
// companion mode (#76). Once useRenderReadiness reports `rendered`, the player
// loads the INITIAL triple over the same #62 GET /artifact route it already uses
// for escalate re-fetches (plan.json was allowlisted for exactly this). The disk
// picker cannot be auto-polled, so companion mode loads over one HTTP origin.
//
// It assembles a LoadedDirectory through loadDirectory's collectWarnings + the
// same blob-URL discipline, so the App's existing load contract is reused
// verbatim — every downstream behavior (block highlight, play/pause, escalate,
// re-fetch) is the already-shipped path. source.md is NOT served over /artifact
// (not allowlisted), so source is null and the UI falls back to per-block
// raw_excerpt (honesty rule) exactly as a directory missing source.md would.
export async function loadFromServerDir(
  baseUrl: string,
  dir: string,
): Promise<LoadedDirectory> {
  const url = (name: string): string =>
    artifactUrl(name, {
      serverMode: true,
      dir,
      serverBaseUrl: baseUrl,
      fixtureBase: '',
    })

  const [planRes, manifestRes, audioRes] = await Promise.all([
    fetch(url('plan.json'), { cache: 'no-store' }),
    fetch(url('manifest.json'), { cache: 'no-store' }),
    fetch(url('audio.wav'), { cache: 'no-store' }),
  ])
  if (!planRes.ok) throw new Error(`re-fetch plan.json failed: HTTP ${planRes.status}`)
  if (!manifestRes.ok) throw new Error(`re-fetch manifest.json failed: HTTP ${manifestRes.status}`)
  if (!audioRes.ok) throw new Error(`re-fetch audio.wav failed: HTTP ${audioRes.status}`)

  const [planText, manifestText, audioBuffer] = await Promise.all([
    planRes.text(),
    manifestRes.text(),
    audioRes.arrayBuffer(),
  ])

  let plan: NarrationPlan
  try {
    plan = JSON.parse(planText) as NarrationPlan
  } catch (e) {
    throw new Error(`plan.json is not valid JSON: ${(e as Error).message}`, { cause: e })
  }
  let manifest: Manifest
  try {
    manifest = JSON.parse(manifestText) as Manifest
  } catch (e) {
    throw new Error(`manifest.json is not valid JSON: ${(e as Error).message}`, { cause: e })
  }

  const warnings = collectWarnings(plan, manifest)

  // Create the blob URL last — only after both parses succeeded — so a parse
  // failure never leaks a URL the caller wasn't told about (R3, same as
  // loadDirectory). The caller owns revoking this URL.
  const audioBlob = new Blob([audioBuffer], { type: 'audio/wav' })
  const audioUrl = URL.createObjectURL(audioBlob)

  return { plan, manifest, audioUrl, source: null, warnings }
}
