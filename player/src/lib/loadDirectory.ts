import type { Manifest } from '../types/manifest.ts'
import type { NarrationPlan } from '../types/plan.ts'
import { artifactUrl, type RefetchBaseInputs } from './refetchBase.ts'

// Schema version the player was authored against. We warn-not-crash on
// mismatch (CLAUDE.md "Schema versioning: additive-compatible within a major
// schema_version"). The player ignores unknown fields by default.
export const EXPECTED_PLAN_SCHEMA = '1'
export const EXPECTED_MANIFEST_SCHEMA = 1

export interface LoadedDirectory {
  plan: NarrationPlan
  manifest: Manifest
  audioUrl: string
  source: string | null
  warnings: string[]
}

// LoadInput unifies the two paths into loadDirectory:
//   - File System Access API → FileSystemDirectoryHandle
//   - <input type=file webkitdirectory> → File[] (FileList coerced)
// The bundled fixture path goes through fetch + buildFromFetch, not this
// function — the directory-loader path runs in the user's browser context.
export type LoadInput =
  | { kind: 'fs-handle'; handle: FileSystemDirectoryHandle }
  | { kind: 'file-list'; files: File[] }

interface ResolvedFiles {
  planFile: File
  manifestFile: File
  audioFile: File
  sourceFile: File | null
}

// loadDirectory reads a sink/persistent output directory (or its equivalent
// FileList from <input webkitdirectory>) and returns the parsed plan +
// manifest + an object-URL for the audio + optional source.md.
//
// Strictness contract (plan D1, D2, R3, R4):
//   - plan.json + manifest.json + audio.wav are REQUIRED. Missing any one →
//     throws an Error with a precise message naming the missing file.
//   - source.md is OPTIONAL. Missing → returns source: null (the UI then
//     falls back to per-block raw_excerpt with a banner — honesty rule).
//   - schema_version mismatch → push a string into `warnings` (NOT throw).
//     The UI surfaces them as a StaleBadge tooltip + info notice.
//   - The returned audioUrl is a `blob:` URL. The caller MUST revoke the
//     PREVIOUS audioUrl before replacing state (R3 blob URL leak).
export async function loadDirectory(input: LoadInput): Promise<LoadedDirectory> {
  const files = await resolveFiles(input)
  const [planText, manifestText, sourceText] = await Promise.all([
    files.planFile.text(),
    files.manifestFile.text(),
    files.sourceFile ? files.sourceFile.text() : Promise.resolve(null),
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

  // Create the blob URL last — only after parsing succeeded — so a parse
  // failure never leaks a URL the caller wasn't told about.
  const audioBlob = new Blob([await files.audioFile.arrayBuffer()], {
    type: 'audio/wav',
  })
  const audioUrl = URL.createObjectURL(audioBlob)

  return {
    plan,
    manifest,
    audioUrl,
    source: sourceText,
    warnings,
  }
}

// loadFromServerDir loads a whole sink/persistent output directory by typed
// SERVER path (issue #70), fetching {plan.json, manifest.json, audio.wav,
// source.md?} over the escalate server's GET /artifact route instead of a
// browser folder picker. `inputs` is the same RefetchBaseInputs the re-fetch
// path uses; loadFromServerDir always resolves in server mode (serverMode:true,
// the passed dir), so the bundled fixture is never reached from here.
//
// Strictness contract (mirrors loadDirectory + plan Decisions v2):
//   - plan.json + manifest.json + audio.wav are REQUIRED. A non-OK HTTP status
//     on ANY of the three throws a precise per-file Error (load aborts — the
//     honest-error path; the caller surfaces it where the picker error renders).
//   - JSON parse failure on plan.json / manifest.json throws a precise per-file
//     Error too.
//   - source.md is OPTIONAL and TOLERANT (Decision v2 resilience):
//       200            → text
//       404            → source:null, silent (the expected absent case)
//       any other      → source:null PLUS a warning "source unavailable: <…>"
//                        (non-OK-non-404, network, timeout, abort) — NEVER throws.
//   - schema_version mismatch → warning via collectWarnings (NOT throw).
//   - The audio blob URL is created via URL.createObjectURL ONLY AFTER plan +
//     manifest parse succeed AND after the audio bytes are read. The byte read
//     is the last throwable step and precedes createObjectURL, so no code
//     between create and return can throw — there is no post-create leak window
//     to guard (Decision v2 state-management, R3). A failure during the read
//     leaks nothing because no URL exists yet.
//
// Triple-consistency caveat (Decision v1 consistency): the three required GETs
// are independent requests; the server's per-dir mutex makes each request
// internally consistent but the {plan,manifest,audio} TRIPLE is not atomic
// across the three fetches. Accepted for cold loads; manifest.stale is the
// safety net. No cross-request lock (would break the per-file route contract).
export async function loadFromServerDir(
  dir: string,
  inputs: RefetchBaseInputs,
): Promise<LoadedDirectory> {
  // Force server-mode resolution against the typed dir regardless of the caller's
  // current mode flag: this entry point is server-load-by-path by definition.
  const resolved: RefetchBaseInputs = { ...inputs, serverMode: true, dir }
  const url = (name: string) => artifactUrl(name, resolved)

  // --- Required files: fetch + precise per-file abort on !res.ok. -----------
  const planRes = await fetch(url('plan.json'), { cache: 'no-store' })
  if (!planRes.ok) {
    throw new Error(`failed to load plan.json: HTTP ${planRes.status}`)
  }
  const manifestRes = await fetch(url('manifest.json'), { cache: 'no-store' })
  if (!manifestRes.ok) {
    throw new Error(`failed to load manifest.json: HTTP ${manifestRes.status}`)
  }
  const audioRes = await fetch(url('audio.wav'), { cache: 'no-store' })
  if (!audioRes.ok) {
    throw new Error(`failed to load audio.wav: HTTP ${audioRes.status}`)
  }

  const planText = await planRes.text()
  const manifestText = await manifestRes.text()

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

  // --- Optional source.md: tolerant, never throws (Decision v2 resilience). --
  let source: string | null = null
  let sourceWarning: string | null = null
  try {
    const sourceRes = await fetch(url('source.md'), { cache: 'no-store' })
    if (sourceRes.ok) {
      source = await sourceRes.text()
    } else if (sourceRes.status === 404) {
      // Expected absent case — source:null, silent.
      source = null
    } else {
      // Present-but-unfetchable (e.g. 500). Surface as a warning, not a crash.
      sourceWarning = `source unavailable: HTTP ${sourceRes.status}`
    }
  } catch (e) {
    // Network / timeout / abort — still never aborts the load.
    sourceWarning = `source unavailable: ${(e as Error).message}`
  }

  const warnings = collectWarnings(plan, manifest)
  if (sourceWarning) warnings.push(sourceWarning)

  // --- Audio blob URL LAST: create only after plan+manifest parsed. The byte
  // read (audioRes.arrayBuffer) is the last throwable surface and runs BEFORE
  // createObjectURL — nothing between createObjectURL and the object-literal
  // return can throw, so there is no post-create failure window to guard (R3
  // blob-leak: the only leak surface is a throw AFTER create, which cannot
  // happen here). A throw during arrayBuffer() leaks nothing — no URL exists
  // yet. The previous try/revoke wrapper was dead code and has been removed. --
  const audioBlob = new Blob([await audioRes.arrayBuffer()], { type: 'audio/wav' })
  const audioUrl = URL.createObjectURL(audioBlob)
  return {
    plan,
    manifest,
    audioUrl,
    source,
    warnings,
  }
}

// ManifestReload is the result of re-reading ONLY manifest.json after an
// escalation patch. No audio blob is created (the audio.wav is re-fetched
// separately and only when the patched block is the one playing — R3 blob-URL
// hygiene). Warnings are recomputed so the manifest schema/stale signals stay
// fresh; the caller REPLACES (not appends) its manifest-derived warnings so a
// re-fetch does not duplicate them in the TopBar.
export interface ManifestReload {
  manifest: Manifest
  warnings: string[]
}

// reloadManifest re-fetches manifest.json from `manifestUrl` (a fully-formed
// URL — fixture: "/fixtures/sample/manifest.json"; server mode: the #62
// "<base>/artifact?dir=…&name=manifest.json" route) and re-parses it. It
// deliberately does NOT touch plan.json or audio.wav — the patch flow keeps the
// existing blob URL and only re-points audio when the patched block is playing.
//
// The URL is resolved by the caller (App, via lib/refetchBase.artifactUrl) so
// this helper stays origin-agnostic — it just fetches + parses whatever URL it
// is handed (#62: re-fetch resolves against the live escalated dir, not a
// hardwired base).
//
// The plan is needed only to recompute schema-mismatch warnings against the
// fresh manifest; it is not re-read. Throws on HTTP failure or invalid JSON so
// the caller can keep the just-committed patch and flag staleDownstream rather
// than rolling back (plan d8).
export async function reloadManifest(
  manifestUrl: string,
  plan: NarrationPlan,
): Promise<ManifestReload> {
  const res = await fetch(manifestUrl, { cache: 'no-store' })
  if (!res.ok) {
    throw new Error(`re-fetch manifest.json failed: HTTP ${res.status}`)
  }
  const text = await res.text()
  let manifest: Manifest
  try {
    manifest = JSON.parse(text) as Manifest
  } catch (e) {
    throw new Error(`re-fetched manifest.json is not valid JSON: ${(e as Error).message}`, {
      cause: e,
    })
  }
  return { manifest, warnings: collectWarnings(plan, manifest) }
}

function collectWarnings(plan: NarrationPlan, manifest: Manifest): string[] {
  const w: string[] = []
  // Compare MAJOR version only — CLAUDE.md: "additive-compatible within a
  // major schema_version". "1.0" / "1.3" stay compatible with major "1".
  const planMajor = String(plan.schema_version).split('.')[0]
  if (plan.schema_version && planMajor !== EXPECTED_PLAN_SCHEMA) {
    w.push(
      `plan.json schema_version ${plan.schema_version} differs from expected major ${EXPECTED_PLAN_SCHEMA}; unknown fields will be ignored.`,
    )
  }
  if (
    typeof manifest.schema_version === 'number' &&
    manifest.schema_version !== EXPECTED_MANIFEST_SCHEMA
  ) {
    w.push(
      `manifest.json schema_version ${manifest.schema_version} differs from expected ${EXPECTED_MANIFEST_SCHEMA}; unknown fields will be ignored.`,
    )
  }
  if (manifest.stale) {
    w.push(
      `manifest.stale = true (${manifest.stale_reason ?? 'no reason given'}); audio may not match the current source.`,
    )
  }
  return w
}

async function resolveFiles(input: LoadInput): Promise<ResolvedFiles> {
  if (input.kind === 'fs-handle') {
    return resolveFromHandle(input.handle)
  }
  return resolveFromFileList(input.files)
}

async function resolveFromHandle(
  handle: FileSystemDirectoryHandle,
): Promise<ResolvedFiles> {
  const byName = new Map<string, File>()
  for await (const child of handle.values()) {
    if (child.kind !== 'file') continue
    const fh = await handle.getFileHandle(child.name)
    byName.set(child.name, await fh.getFile())
  }
  return assemble(byName, `directory "${handle.name}"`)
}

function resolveFromFileList(files: File[]): ResolvedFiles {
  const byName = new Map<string, File>()
  for (const f of files) {
    // webkitdirectory gives us files with relative paths; key by basename
    // (last segment of webkitRelativePath or name) so callers can use either
    // input shape without re-keying.
    const rel = (f as File & { webkitRelativePath?: string }).webkitRelativePath
    const base = rel ? rel.split('/').pop()! : f.name
    byName.set(base, f)
  }
  return assemble(byName, 'selected directory')
}

function assemble(byName: Map<string, File>, label: string): ResolvedFiles {
  const planFile = byName.get('plan.json')
  const manifestFile = byName.get('manifest.json')
  const audioFile = byName.get('audio.wav')
  const sourceFile = byName.get('source.md') ?? null

  if (!planFile) throw new Error(`${label} is missing plan.json`)
  if (!manifestFile) throw new Error(`${label} is missing manifest.json`)
  if (!audioFile) throw new Error(`${label} is missing audio.wav`)

  return { planFile, manifestFile, audioFile, sourceFile }
}
