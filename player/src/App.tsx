import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
// React 19 hooks-lint flags `setState`-in-effect; we therefore reset
// `sourceCursorBlockId` and seed plan/manifest synchronously inside render
// by comparing the previous plan_id via a ref. The pattern is exactly the
// one the React 19 docs prescribe for "reset derived state when a prop
// changes" (https://react.dev/learn/you-might-not-need-an-effect).
import { useFixture, FIXTURE_BASE } from './hooks/useFixture.ts'
import { useDirectoryLoader } from './hooks/useDirectoryLoader.ts'
import { usePlayback } from './hooks/usePlayback.ts'
import { useServerMode } from './hooks/useServerMode.ts'
import { useEscalation } from './hooks/useEscalation.ts'
import type { EscalateResult } from './lib/escalateClient.ts'
import { reconcileManifest } from './lib/reconcileManifest.ts'
import { reloadManifest as reloadManifestFile } from './lib/loadDirectory.ts'
import type { Manifest } from './types/manifest.ts'
import type { Block, NarrationPlan } from './types/plan.ts'
import { TopBar } from './components/TopBar.tsx'
import { BlockList } from './components/BlockList.tsx'
import { SourcePane } from './components/SourcePane.tsx'

// App is the composition root for the player. Loads the fixture on mount,
// optionally swaps to a directory the user picks at runtime, and threads the
// playback + escalation state into BlockList + SourcePane + the bottom bar.
//
// Plan/manifest ownership (issue #50): the player now PATCHES blocks in place,
// so App owns mutable copies of plan + manifest (seeded once per plan_id from
// the loaded source). Escalation replaces one plan block + reconciles the
// manifest WITHOUT clobbering other rows or re-seeding from the immutable
// source. A /healthz recheck never touches this state.

export default function App() {
  const fixture = useFixture()
  const loader = useDirectoryLoader()
  const server = useServerMode()

  const active = loader.data ?? fixture.data
  const audioRef = useRef<HTMLAudioElement | null>(null)
  const [audioEl, setAudioEl] = useState<HTMLAudioElement | null>(null)

  // App-owned, patchable plan + manifest. Seeded once per plan_id from the
  // loaded source (the seed guard below), then mutated in place by escalation.
  const [plan, setPlan] = useState<NarrationPlan | null>(null)
  const [manifest, setManifest] = useState<Manifest | null>(null)
  // The seeded plan_id is tracked in state (not a ref) so the seed guard can run
  // during render without violating React 19's no-ref-access-during-render rule.
  // This is the same "adjust state during render" idiom usePlayback uses for its
  // manifest tracking. Setting the same value is a no-op, so the guard stays
  // idempotent under StrictMode's double-invoke (seededPlanId stays the brief's
  // semantics, just stored as state rather than a ref).
  const [seededPlanId, setSeededPlanId] = useState<string | null>(null)

  const [sourceCursorBlockId, setSourceCursorBlockId] = useState<string | null>(null)
  const [escalatedBlockId, setEscalatedBlockId] = useState<string | null>(null)
  const [extraWarnings, setExtraWarnings] = useState<string[]>([])
  // Manual escalate dir (Q2). Server-resolvable absolute path; empty until the
  // user types it. Threaded into postEscalate via the escalation context.
  const [dir, setDir] = useState('')

  // Seed-once on a new plan_id (directory swap or first fixture load).
  // Idempotent under StrictMode double-invoke; survives recheck() because
  // recheck only flips server.mode, not `active`. Never re-seeds while the same
  // plan_id is loaded — that would clobber a patched block back to source.
  // Runs during render per the React 19 "adjust state during render" idiom.
  const currentPlanId = active?.plan.plan_id ?? null
  if (currentPlanId && active && currentPlanId !== seededPlanId) {
    setSeededPlanId(currentPlanId)
    setPlan(active.plan)
    setManifest(active.manifest)
    setExtraWarnings([])
    if (sourceCursorBlockId !== null) setSourceCursorBlockId(null)
    if (escalatedBlockId !== null) setEscalatedBlockId(null)
  }

  const playback = usePlayback(audioEl, manifest)

  // audioUrl from the loaded source. Escalation may re-point this to a fresh
  // blob (revoke-before-replace) ONLY when the patched block is playing.
  const baseAudioUrl = active?.audioUrl ?? null
  const [audioUrl, setAudioUrl] = useState<string | null>(baseAudioUrl)
  const repointedUrlRef = useRef<string | null>(null)
  // Adopt a new source audio URL via the React 19 "adjust state during render"
  // idiom (track prior value in state, compare, queue the update) — same
  // pattern usePlayback uses for manifest swaps. The previous escalation blob is
  // revoked in the effect below (ref writes are not allowed during render).
  const [trackedBaseAudioUrl, setTrackedBaseAudioUrl] = useState<string | null>(baseAudioUrl)
  if (baseAudioUrl && trackedBaseAudioUrl !== baseAudioUrl) {
    setTrackedBaseAudioUrl(baseAudioUrl)
    setAudioUrl(baseAudioUrl)
  }
  // Revoke the prior escalation-repointed blob when the source audio swaps
  // (directory swap / first load). The source blobs themselves are owned + freed
  // by their loader hooks; we only own the escalation re-point blob here (R3).
  useEffect(() => {
    return () => {
      if (repointedUrlRef.current) {
        URL.revokeObjectURL(repointedUrlRef.current)
        repointedUrlRef.current = null
      }
    }
  }, [baseAudioUrl])

  // Live refs so the stable escalation callbacks never read a stale plan /
  // manifest / playback (review item 1: stale-closure avoidance). Synced in an
  // effect (not during render) per the React 19 hooks-lint "no ref writes
  // during render" rule — the same effect-synced-ref idiom usePlayback uses.
  // Escalation handlers fire on user interaction (post-commit), so the effect
  // has always run by the time a ref is read.
  const planRef = useRef(plan)
  const manifestRef = useRef(manifest)
  const playbackRef = useRef(playback)
  useEffect(() => {
    planRef.current = plan
    manifestRef.current = manifest
    playbackRef.current = playback
  }, [plan, manifest, playback])

  // Capture the audio element ref into state so usePlayback sees the mounted
  // node (refs alone don't re-trigger effects). Re-runs when audioUrl swaps.
  useEffect(() => {
    setAudioEl(audioRef.current)
  }, [audioUrl])

  // repointAudio re-fetches audio.wav (cache-busted) and swaps the <audio> src
  // to a fresh blob, revoking the previous escalation blob first (R3). The base
  // (source-loaded) blob is owned by its loader hook and not revoked here.
  const repointAudio = useCallback(async (): Promise<void> => {
    try {
      // KNOWN GAP (served-origin re-fetch): in server mode the patched output
      // lives at the user's `dir`, but the audio re-fetch is hardwired to the
      // served fixture origin (FIXTURE_BASE), not `dir`. A server-mode user who
      // points `dir` at a non-fixture outDir re-fetches the WRONG audio.wav.
      // Fixing it properly needs the server to statically serve an arbitrary
      // outDir's artifacts over HTTP — a server-contract change, out of this
      // front-end-only scope. The patched row stays correct meanwhile because
      // offsets are updated optimistically from the response `timing`. See the
      // matching gap in reloadManifest below.
      const res = await fetch(`${FIXTURE_BASE}/audio.wav?ts=${Date.now()}`, {
        cache: 'no-store',
      })
      if (!res.ok) return
      const blob = new Blob([await res.arrayBuffer()], { type: 'audio/wav' })
      const url = URL.createObjectURL(blob)
      if (repointedUrlRef.current) URL.revokeObjectURL(repointedUrlRef.current)
      repointedUrlRef.current = url
      setAudioUrl(url)
    } catch {
      // Audio re-fetch failure is non-fatal: offsets already updated, the old
      // blob still plays. The patch is retained.
    }
  }, [])

  // reloadManifest re-fetches manifest.json and reconciles it into App-owned
  // state. Returns true on success; false (leaving the prior manifest in place)
  // on any re-fetch failure so the caller can flag staleDownstream rather than
  // roll back the just-committed patch (plan d8).
  const reloadManifest = useCallback(async (): Promise<boolean> => {
    const cur = planRef.current
    const prev = manifestRef.current
    if (!cur || !prev) return false
    try {
      // KNOWN GAP (served-origin re-fetch): like repointAudio above, this
      // re-reads manifest.json from the served fixture origin (FIXTURE_BASE),
      // not the server-mode `dir`. A server-mode user pointing `dir` at a
      // non-fixture outDir re-fetches the WRONG manifest.json (stale downstream
      // offsets). Same root cause + same out-of-scope fix (server must serve an
      // arbitrary outDir over HTTP); the just-patched row stays correct via the
      // optimistic timing update in patchVoicedBlock.
      const { manifest: next, warnings } = await reloadManifestFile(FIXTURE_BASE, cur)
      setManifest((m) => reconcileManifest(m ?? prev, next))
      // REPLACE (not append) manifest-derived warnings so a re-fetch does not
      // duplicate schema/stale lines in the TopBar (plan: dedup on re-fetch).
      setExtraWarnings(warnings)
      return true
    } catch {
      return false
    }
  }, [])

  // patchVoicedBlock replaces ONE plan block with the escalated block and
  // updates its manifest timing. Other blocks (and their identities) are
  // untouched. It then re-fetches the manifest (downstream offsets become
  // authoritative) and re-points the audio blob iff the patched block is the one
  // playing. Returns { refetchOk, changed } for the row state machine.
  const patchVoicedBlock = useCallback(
    async (
      blockId: string,
      result: Extract<EscalateResult, { kind: 'patched' }>,
    ): Promise<{ refetchOk: boolean; changed: boolean }> => {
      const prevPlan = planRef.current
      const prevManifest = manifestRef.current
      if (!prevPlan || !prevManifest) return { refetchOk: false, changed: false }

      const prevBlock = prevPlan.blocks.find((b) => b.id === blockId) ?? null
      const prevTiming = prevManifest.blocks.find((b) => b.id === blockId) ?? null
      const changed =
        !blocksEqualForChange(prevBlock, result.block) ||
        prevTiming?.start_ms !== result.timing.start_ms ||
        prevTiming?.end_ms !== result.timing.end_ms ||
        prevTiming?.audio_ref !== result.timing.audio_ref

      setPlan((p) =>
        p ? { ...p, blocks: p.blocks.map((b) => (b.id === blockId ? result.block : b)) } : p,
      )
      setManifest((m) =>
        m
          ? {
              ...m,
              blocks: m.blocks.map((mb) =>
                mb.id === blockId
                  ? {
                      ...mb,
                      level: result.block.level,
                      status: result.block.status,
                      start_ms: result.timing.start_ms,
                      end_ms: result.timing.end_ms,
                      audio_ref: result.timing.audio_ref ?? mb.audio_ref,
                    }
                  : mb,
              ),
            }
          : m,
      )

      // Audio re-point (Q3): only re-fetch + rebuild the audio blob when the
      // patched block is the one currently playing. Otherwise the whole
      // audio.wav blob is unchanged and we only update offsets above.
      if (playbackRef.current.activeBlockId === blockId) {
        await repointAudio()
      }

      const refetchOk = await reloadManifest()
      return { refetchOk, changed }
    },
    [reloadManifest, repointAudio],
  )

  // Known limitation (plan d4): a refusal patch updates level/status in place but
  // does NOT re-fetch the manifest. A refusal that shrinks a block to nothing
  // could in principle shift downstream on-disk offsets; the player keeps the
  // prior offsets. Acceptable phase-one because the server re-splices audio.wav
  // regardless, and refusals do not change spoken audio length in practice.
  const patchRefusedBlock = useCallback(
    (blockId: string, result: Extract<EscalateResult, { kind: 'refused' }>) => {
      setPlan((p) =>
        p ? { ...p, blocks: p.blocks.map((b) => (b.id === blockId ? result.block : b)) } : p,
      )
      setManifest((m) =>
        m
          ? {
              ...m,
              blocks: m.blocks.map((mb) =>
                mb.id === blockId
                  ? { ...mb, level: result.block.level, status: result.block.status }
                  : mb,
              ),
            }
          : m,
      )
    },
    [],
  )

  const escalation = useEscalation({
    baseUrl: server.baseUrl,
    dir,
    onPatched: patchVoicedBlock,
    onRefused: patchRefusedBlock,
  })

  const warnings = useMemo(() => {
    const w: string[] = []
    if (active?.warnings) w.push(...active.warnings)
    w.push(...extraWarnings)
    if (loader.error) w.push(`load failed: ${loader.error}`)
    if (!active && fixture.error) w.push(`fixture load failed: ${fixture.error}`)
    return w
  }, [active, extraWarnings, loader.error, fixture.error])

  // Loading / error gates.
  if (fixture.loading && !loader.data) {
    return (
      <div className="app">
        <p className="loading">Loading bundled fixture…</p>
      </div>
    )
  }

  if (!active || !plan || !manifest) {
    return (
      <div className="app">
        <p className="error">
          No directory loaded.{' '}
          {loader.error ?? fixture.error ?? 'Pick a sink/persistent output directory to continue.'}
        </p>
      </div>
    )
  }

  const source = active.source

  return (
    <div className="app">
      <TopBar
        manifest={manifest}
        voice={manifest.voice}
        warnings={warnings}
        supportsFsAccess={loader.supportsFsAccess}
        onPickDirectory={loader.pickDirectory}
        onFileListChange={loader.loadFromFileList}
        serverMode={server.mode}
        dir={dir}
        onDirChange={setDir}
      />

      <main className="main">
        <BlockList
          plan={plan}
          manifestBlocks={manifest.blocks}
          activeBlockId={playback.activeBlockId}
          sourceCursorBlockId={sourceCursorBlockId}
          escalatedBlockId={escalatedBlockId}
          serverMode={server.mode}
          dir={dir}
          rowState={escalation.rowState}
          onSeek={(id) => {
            playback.seekToBlock(id)
            playback.play()
          }}
          onEscalate={escalation.escalate}
          onToggleEscalate={(id) =>
            setEscalatedBlockId((cur) => (cur === id ? null : id))
          }
          onDismissEscalate={() => setEscalatedBlockId(null)}
        />
        <SourcePane
          plan={plan}
          manifestBlocks={manifest.blocks}
          source={source}
          activeBlockId={playback.activeBlockId}
          onCursorChange={setSourceCursorBlockId}
        />
      </main>

      <footer className="audio-bar">
        <audio
          ref={audioRef}
          src={audioUrl ?? undefined}
          controls
          preload="auto"
          data-testid="audio-element"
        >
          Your browser does not support audio playback.
        </audio>
      </footer>
    </div>
  )
}

// blocksEqualForChange compares the spoken-content-bearing fields of a block to
// decide whether an escalation produced a visible change (plan d5 "no visible
// change" ack). It compares level, status, refusal, and the joined segment text
// — the things a listener/reader would notice. A deep structural compare would
// be brittle against provenance churn (model id, timestamps).
//
// NOT the same notion as reconcileManifest.ts `blocksEqual`: that one is a
// manifest-row memo-identity check (did the *timing row* change, to preserve
// object identity for React.memo). This one is a plan-block *content* change
// check (did the spoken output change, for the user-facing noop ack).
function blocksEqualForChange(a: Block | null, b: Block): boolean {
  if (!a) return false
  if (a.level !== b.level || a.status !== b.status) return false
  if ((a.refusal?.message ?? '') !== (b.refusal?.message ?? '')) return false
  return segmentText(a) === segmentText(b)
}

function segmentText(block: Block): string {
  if (!block.segments) return ''
  return block.segments
    .filter((s) => s.kind === 'speech' && typeof s.text === 'string')
    .map((s) => s.text)
    .join(' ')
}
