import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import type { Manifest, ManifestBlock } from '../types/manifest.ts'
import { findActiveBlock } from '../lib/findActiveBlock.ts'

// usePlayback polls audio.currentTime via requestAnimationFrame and computes
// the active block id (plan D4). Two performance contracts:
//
//   1. React state is written ONLY when the active id changes (transition).
//      The rAF tick reads via a ref to skip re-renders at 60 Hz.
//   2. The hook stays paused (no rAF loop) while the audio is paused — no
//      need to spin while no time is advancing.
//
// Returns:
//   - activeBlockId: current block id under the playhead, or null pre/post-roll.
//   - currentTimeMs: latest sampled time (state, throttled by activeBlockId
//     transitions only). Useful for UI debug; not used for sync logic.
//   - seekToBlock(id): set audio.currentTime to block.start_ms / 1000.
//     Also bumps the activeBlockId immediately so the highlight tracks the
//     click without waiting for the next rAF tick.
//   - play() / pause(): proxies through to the HTMLMediaElement.

export interface PlaybackApi {
  activeBlockId: string | null
  currentTimeMs: number
  seekToBlock: (blockId: string) => void
  play: () => void
  pause: () => void
}

export function usePlayback(
  audio: HTMLAudioElement | null,
  manifest: Manifest | null,
): PlaybackApi {
  const [activeBlockId, setActiveBlockId] = useState<string | null>(null)
  const [currentTimeMs, setCurrentTimeMs] = useState(0)

  const activeRef = useRef<string | null>(null)
  const rafRef = useRef<number | null>(null)
  // React 19's hooks-lint forbids mutating hook args, so we read+write the
  // audio element through an effect-synced ref. The prop drives the ref;
  // callbacks read the ref so they don't appear to mutate the arg.
  const audioRef = useRef<HTMLAudioElement | null>(audio)
  useEffect(() => {
    audioRef.current = audio
  }, [audio])

  // Derive the sorted block list as memoised data — purer than re-sorting in
  // an effect, and the React 19 hooks-lint flagged the previous setState-in-
  // effect pattern. The sort is O(n log n) only when manifest identity
  // changes; on steady-state renders this returns the same array.
  const sortedBlocks = useMemo<ManifestBlock[]>(() => {
    if (!manifest) return []
    return [...manifest.blocks].sort((a, b) => a.start_ms - b.start_ms)
  }, [manifest])

  // Reset playback state when the manifest swaps (new directory loaded).
  // React 19's "adjust state during render" idiom: track prior manifest via
  // useState, compare to the current arg, queue functional resets. The
  // activeRef ref is reset inside the tick-rebinding effect below, which
  // also fires on manifest changes (via sortedBlocks).
  const [trackedManifest, setTrackedManifest] = useState<Manifest | null>(manifest)
  if (trackedManifest !== manifest) {
    setTrackedManifest(manifest)
    setActiveBlockId(null)
    setCurrentTimeMs(0)
  }

  // The rAF tick — defined once, captured in a ref so the rAF callback can
  // recurse without naming itself in its own deps.
  const tickRef = useRef<() => void>(() => {})

  // Re-bind the tick closure whenever the audio or sorted blocks change.
  // Doing it inside an effect (not during render) satisfies React 19's
  // "no ref writes during render" rule. Also resets activeRef so a stale
  // active id from the previous manifest does not survive a directory swap.
  useEffect(() => {
    activeRef.current = null
    tickRef.current = () => {
      const el = audioRef.current
      if (!el) {
        rafRef.current = null
        return
      }
      const tMs = Math.max(0, Math.floor(el.currentTime * 1000))
      const next = findActiveBlock(sortedBlocks, tMs)
      if (next !== activeRef.current) {
        activeRef.current = next
        setActiveBlockId(next)
        setCurrentTimeMs(tMs)
      }
      if (!el.paused && !el.ended) {
        rafRef.current = requestAnimationFrame(() => tickRef.current())
      } else {
        rafRef.current = null
      }
    }
  }, [audio, sortedBlocks])

  // Subscribe to play/pause/ended/seeked. Start/stop rAF accordingly.
  useEffect(() => {
    const el = audio
    if (!el) return
    const onPlay = () => {
      if (rafRef.current == null) {
        rafRef.current = requestAnimationFrame(() => tickRef.current())
      }
    }
    const settle = () => {
      if (rafRef.current != null) {
        cancelAnimationFrame(rafRef.current)
        rafRef.current = null
      }
      // Emit one tick so the UI shows the resting state correctly.
      const tMs = Math.max(0, Math.floor(el.currentTime * 1000))
      const next = findActiveBlock(sortedBlocks, tMs)
      if (next !== activeRef.current) {
        activeRef.current = next
        setActiveBlockId(next)
        setCurrentTimeMs(tMs)
      }
    }

    el.addEventListener('play', onPlay)
    el.addEventListener('pause', settle)
    el.addEventListener('ended', settle)
    el.addEventListener('seeked', settle)
    return () => {
      el.removeEventListener('play', onPlay)
      el.removeEventListener('pause', settle)
      el.removeEventListener('ended', settle)
      el.removeEventListener('seeked', settle)
      if (rafRef.current != null) {
        cancelAnimationFrame(rafRef.current)
        rafRef.current = null
      }
    }
  }, [audio, sortedBlocks])

  const seekToBlock = useCallback(
    (blockId: string) => {
      const el = audioRef.current
      if (!el) return
      const block = sortedBlocks.find((b) => b.id === blockId)
      if (!block) return
      el.currentTime = block.start_ms / 1000
      // Eager: bump the highlight immediately, don't wait for rAF or 'seeked'.
      if (activeRef.current !== blockId) {
        activeRef.current = blockId
        setActiveBlockId(blockId)
        setCurrentTimeMs(block.start_ms)
      }
    },
    [sortedBlocks],
  )

  const play = useCallback(() => {
    const el = audioRef.current
    if (!el) return
    void el.play().catch(() => {
      // play() rejects when the browser blocks autoplay; UI consumers can
      // call play() via a user gesture (the <audio controls> button).
    })
  }, [])

  const pause = useCallback(() => {
    audioRef.current?.pause()
  }, [])

  return { activeBlockId, currentTimeMs, seekToBlock, play, pause }
}
