import { useEffect, useMemo, useRef, useState } from 'react'
// React 19 hooks-lint flags `setState`-in-effect; we therefore reset
// `escalatedBlockId` / `sourceCursorBlockId` synchronously inside render
// by comparing the previous plan_id via a ref. The pattern is exactly the
// one the React 19 docs prescribe for "reset derived state when a prop
// changes" (https://react.dev/learn/you-might-not-need-an-effect).
import { useFixture } from './hooks/useFixture.ts'
import { useDirectoryLoader } from './hooks/useDirectoryLoader.ts'
import { usePlayback } from './hooks/usePlayback.ts'
import { TopBar } from './components/TopBar.tsx'
import { BlockList } from './components/BlockList.tsx'
import { SourcePane } from './components/SourcePane.tsx'

// App is the composition root for the player. Loads the fixture on mount,
// optionally swaps to a directory the user picks at runtime, and threads
// the playback state into BlockList + SourcePane + the bottom audio bar.
//
// State machine (plan: useReducer is fine but useState here is small enough):
//   - fixture: { loading, data, error }   (initial mount)
//   - loader:  { status, data, error, ... } (user picked a directory)
//   When loader.data is present, it takes precedence over fixture.data.
//   That way the user can swap inputs without a hard reload.
//
// Local UI state owned here:
//   - escalatedBlockId   (one card open at a time)
//   - sourceCursorBlockId (hover/click target in SourcePane; decoupled from
//                          playback's activeBlockId)

export default function App() {
  const fixture = useFixture()
  const loader = useDirectoryLoader()

  const active = loader.data ?? fixture.data
  const audioRef = useRef<HTMLAudioElement | null>(null)
  const [audioEl, setAudioEl] = useState<HTMLAudioElement | null>(null)
  const playback = usePlayback(audioEl, active?.manifest ?? null)

  const [escalatedBlockId, setEscalatedBlockId] = useState<string | null>(null)
  const [sourceCursorBlockId, setSourceCursorBlockId] = useState<string | null>(null)

  // Reset escalation + cursor on directory swap (render-time pattern; see
  // import-block comment above).
  const lastPlanIdRef = useRef<string | null>(active?.plan.plan_id ?? null)
  const currentPlanId = active?.plan.plan_id ?? null
  if (lastPlanIdRef.current !== currentPlanId) {
    lastPlanIdRef.current = currentPlanId
    if (escalatedBlockId !== null) setEscalatedBlockId(null)
    if (sourceCursorBlockId !== null) setSourceCursorBlockId(null)
  }

  // Capture the audio element ref into state so usePlayback sees the
  // mounted node (refs alone don't re-trigger effects).
  useEffect(() => {
    setAudioEl(audioRef.current)
  }, [active?.audioUrl])

  const warnings = useMemo(() => {
    const w: string[] = []
    if (active?.warnings) w.push(...active.warnings)
    if (loader.error) w.push(`load failed: ${loader.error}`)
    if (!active && fixture.error) w.push(`fixture load failed: ${fixture.error}`)
    return w
  }, [active, loader.error, fixture.error])

  // Loading / error gates.
  if (fixture.loading && !loader.data) {
    return (
      <div className="app">
        <p className="loading">Loading bundled fixture…</p>
      </div>
    )
  }

  if (!active) {
    return (
      <div className="app">
        <p className="error">
          No directory loaded.{' '}
          {loader.error ?? fixture.error ?? 'Pick a sink/persistent output directory to continue.'}
        </p>
      </div>
    )
  }

  const { plan, manifest, audioUrl, source } = active

  return (
    <div className="app">
      <TopBar
        manifest={manifest}
        voice={manifest.voice}
        warnings={warnings}
        supportsFsAccess={loader.supportsFsAccess}
        onPickDirectory={loader.pickDirectory}
        onFileListChange={loader.loadFromFileList}
      />

      <main className="main">
        <BlockList
          plan={plan}
          manifestBlocks={manifest.blocks}
          activeBlockId={playback.activeBlockId}
          sourceCursorBlockId={sourceCursorBlockId}
          escalatedBlockId={escalatedBlockId}
          onSeek={(id) => {
            playback.seekToBlock(id)
            playback.play()
          }}
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
          src={audioUrl}
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
