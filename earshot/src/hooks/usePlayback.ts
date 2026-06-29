// usePlayback.ts — the single playback engine for the Earshot transport (#112).
//
// Instantiated EXACTLY ONCE, at the NarrationContext provider (see
// PlaybackContext), and distributed to TransportDeck / BlockScrubber / BlockRow
// via context. Multiple instances would each spin their own rAF loop and
// compete to write the active block against the one shared <audio> — so this
// hook is never called from a leaf component.
//
// It does NOT call useAudio itself: useAudio is owned once by
// useNarrationSession (the single <audio> element). usePlayback layers the
// block-quantized command surface + the rAF active-block derivation on top of
// that single audio, subscribing READ-ONLY to its seeked / loadedmetadata
// events. This reconciles the base plan's "wrap useAudio" with the
// single-shared-<audio> invariant.
//
// Sync is BLOCK-LEVEL ONLY (CLAUDE.md invariant). The rAF loop maps the clock
// to a block INDEX and writes state only on a block-id CHANGE (Decision
// 2026-06-21) — never per frame, never a sub-block offset.

import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import type { PlaybackState } from "./useAudio";
import type { Timeline } from "../api/types";
import {
  clamp,
  computeBlockSignature,
  findActiveBlockIndex,
} from "../state/playbackMath";
import {
  RESUME_SCHEMA_VERSION,
  readResume,
  resumeKey,
  writeResume,
} from "../state/resumeStore";

/** ±N blocks for the scrubber's PageUp / PageDown. */
export const PAGE_BLOCKS = 5;
/**
 * Fallback timeout for the restore gate. On fire it RE-ASSERTS the restore seek
 * (it never clears the gate / unmutes), so a deferred or aborted cold seek can
 * never silently let the rAF loop derive block 0 and clobber the saved resume.
 */
export const RESTORE_GATE_TIMEOUT_MS = 2000;

/** Inputs sourced from the single useNarrationSession owner. */
export interface UsePlaybackInput {
  audioRef: React.RefObject<HTMLAudioElement>;
  timeline: Timeline | undefined;
  /** Resume scope (the selected entry id); null disables persistence. */
  resumeScope: string | null;
  playbackState: PlaybackState;
  seek: (ms: number) => void;
  play: () => void;
  pause: () => void;
}

/**
 * Continuous audio-clock snapshot, pushed every animation frame to subscribers
 * (the scrubber fill + the transport clock). This is the AUDIO position, NOT a
 * text-sync signal: it never maps to a sub-block / word offset and seeking stays
 * block-quantized, so the block-level-sync invariant is untouched. Delivered
 * imperatively (subscriber mutates its own DOM) so the moving bar costs ZERO
 * per-frame React re-renders (Decision 2026-06-21 holds).
 */
export interface PlaybackProgress {
  /** Current playhead position, ms. */
  currentMs: number;
  /** Total clip duration, ms; 0 until the element knows its duration. */
  durationMs: number;
  /** currentMs / durationMs, clamped to [0, 1]; 0 when duration unknown. */
  fraction: number;
}

/** The transport command surface, distributed via context. */
export interface PlaybackControls {
  activeBlockId: string | null;
  /** Index of the active block in document order, or -1 when none. */
  activeBlockIndex: number;
  blockCount: number;
  /** Sorted block-start offsets (for the scrubber aria-valuetext). */
  blockStartsMs: number[];
  /**
   * Subscribe to the per-frame audio-clock snapshot. Returns an unsubscribe fn.
   * Subscribers update their own DOM imperatively — no React state, no re-render.
   */
  subscribeProgress: (fn: (p: PlaybackProgress) => void) => () => void;
  playbackState: PlaybackState;
  isPlaying: boolean;
  play: () => void;
  pause: () => void;
  toggle: () => void;
  seekToBlock: (blockId: string) => void;
  /** Seek the scrubber/transport to a block by index (clamped). */
  seekToIndex: (index: number) => void;
  /** Seek to a block's start AND play (BlockRow "⏯ from here"). */
  playFromBlock: (blockId: string) => void;
  /** ADR #77 block-quantized step: one block back / forward. */
  prevBlock: () => void;
  nextBlock: () => void;
  /** PageUp/PageDown: ±N blocks. */
  stepBlocks: (delta: number) => void;
  /** Scroll + focus the active BlockRow (distinct from seeking). */
  returnToPlayingBlock: () => void;
}

export function usePlayback(input: UsePlaybackInput): PlaybackControls {
  const { audioRef, timeline, resumeScope, playbackState, seek, play, pause } = input;

  // Sorted block geometry, memoized on the timeline. Recomputed only when the
  // document changes (a new narration or an escalation rewriting offsets).
  const geom = useMemo(() => {
    const blocks = [...(timeline?.blocks ?? [])].sort((a, b) => a.start_ms - b.start_ms);
    const ids = blocks.map((b) => b.block_id);
    return {
      ids,
      starts: blocks.map((b) => b.start_ms),
      ends: blocks.map((b) => b.end_ms),
      signature: computeBlockSignature(ids),
    };
  }, [timeline]);

  const [activeBlockId, setActiveBlockId] = useState<string | null>(null);

  // Refs the rAF loop and the resume writer read without re-subscribing.
  const activeIdRef = useRef<string | null>(null);
  const restoringRef = useRef(false);
  // Last value the resume writer persisted (echo guard): a restore seeds this
  // with the restored block so the initialize-not-transition seed never
  // re-writes the entry it just read.
  const lastWrittenRef = useRef<string | null>(null);
  const gateTimerRef = useRef<number | null>(null);
  // The block the restore is trying to land on, plus its [start, nextStart)
  // range. Used to verify a 'seeked' actually landed inside the restored block
  // before the gate clears (a bare timer must never unmute into a stale 0).
  const restoreTargetRef = useRef<{ startMs: number; nextStartMs: number } | null>(null);

  // Per-frame audio-clock subscribers (scrubber fill + transport clock). A Set
  // so multiple consumers can register; each mutates its own DOM imperatively.
  const progressSubsRef = useRef<Set<(p: PlaybackProgress) => void>>(new Set());
  const subscribeProgress = useCallback((fn: (p: PlaybackProgress) => void) => {
    progressSubsRef.current.add(fn);
    return () => {
      progressSubsRef.current.delete(fn);
    };
  }, []);

  const resumeKeyStr = resumeScope ? resumeKey(`entry:${resumeScope}`) : null;

  const clearGate = useCallback(() => {
    restoringRef.current = false;
    restoreTargetRef.current = null;
    if (gateTimerRef.current != null) {
      clearTimeout(gateTimerRef.current);
      gateTimerRef.current = null;
    }
  }, []);

  const setActive = useCallback((id: string | null) => {
    activeIdRef.current = id;
    setActiveBlockId(id);
  }, []);

  // Re-assert the restore: force metadata to load and re-issue the seek to the
  // restored block's start. The gate STAYS set — only a 'seeked' that actually
  // lands inside the block clears it (see the gate effect). Called from the
  // bounded fallback timer so a deferred/aborted cold seek (preload defers a
  // currentTime set at HAVE_NOTHING) never unmutes into a stale currentTime 0.
  const reassertRestore = useCallback(() => {
    const el = audioRef.current;
    const target = restoreTargetRef.current;
    if (!el || !target) return;
    el.load?.(); // reach HAVE_METADATA so the seek can actually land
    seek(target.startMs);
  }, [audioRef, seek]);

  // initialize-not-transition (R3): seed the active block WITHOUT going through
  // the resume-persist transition writer. Sets the restoring gate so the rAF
  // loop stays muted until the seek lands, and re-derives startMs live from the
  // timeline (the store never persists an ms offset).
  const initializeActiveBlock = useCallback(
    (blockId: string) => {
      const idx = geom.ids.indexOf(blockId);
      if (idx < 0) return;
      restoringRef.current = true;
      restoreTargetRef.current = {
        startMs: geom.starts[idx],
        nextStartMs: idx + 1 < geom.starts.length ? geom.starts[idx + 1] : Number.POSITIVE_INFINITY,
      };
      lastWrittenRef.current = blockId; // echo-guard belt
      setActive(blockId);
      // preload="metadata" + this load() let the element reach HAVE_METADATA so
      // the restore seek can land (a currentTime set at HAVE_NOTHING is deferred).
      audioRef.current?.load?.();
      seek(geom.starts[idx]); // no autoplay — restore position only
      // Bounded fallback: on fire it RE-ASSERTS the seek (does NOT clear the
      // gate), so the gate can never unmute into a stale currentTime 0 / block 0.
      if (gateTimerRef.current != null) clearTimeout(gateTimerRef.current);
      gateTimerRef.current = window.setTimeout(reassertRestore, RESTORE_GATE_TIMEOUT_MS);
    },
    [geom.ids, geom.starts, seek, setActive, reassertRestore, audioRef],
  );

  // On a document change (signature) reset tracking, then restore from a valid
  // resume entry via the initialize path. A pure restore persists NOTHING.
  useEffect(() => {
    activeIdRef.current = null;
    lastWrittenRef.current = null;
    clearGate();
    if (!resumeKeyStr || geom.ids.length === 0) {
      setActive(null);
      return;
    }
    const entry = readResume(resumeKeyStr, geom.signature);
    if (entry && geom.ids.includes(entry.blockId)) {
      initializeActiveBlock(entry.blockId);
    } else {
      setActive(null);
    }
    // DELIBERATE exhaustive-deps disable (not incidental): initializeActiveBlock,
    // clearGate and setActive are all stable for a given geom, and this effect
    // must re-run ONLY when the document (signature) or the resume scope changes.
    // Listing the callbacks would re-fire the restore on unrelated renders.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [geom.signature, resumeKeyStr]);

  // Restore gate clear (R2): keep the rAF loop + resume writer muted until the
  // restore seek has ACTUALLY landed inside the restored block. Read-only
  // subscription; does not touch useAudio's contract.
  //   - 'loadedmetadata' only means the element can now seek (under preload it
  //     may have reset currentTime to 0), so we RE-ISSUE the restore seek there
  //     rather than clearing the gate.
  //   - 'seeked' clears the gate ONLY when currentTime lands in [start,
  //     nextStart). A 'seeked' that did NOT land (e.g. a load() reset to 0)
  //     keeps the gate set — never unmute into a stale currentTime 0 (the old
  //     block-0 clobber). A bare timer never clears the gate (see reassertRestore).
  useEffect(() => {
    const el = audioRef.current;
    if (!el) return;
    const onLoadedMetadata = () => {
      if (!restoringRef.current) return;
      const target = restoreTargetRef.current;
      if (target) seek(target.startMs); // metadata ready — now the seek can land
    };
    const onSeeked = () => {
      if (!restoringRef.current) return;
      const target = restoreTargetRef.current;
      if (!target) {
        clearGate();
        return;
      }
      const t = el.currentTime * 1000;
      if (t >= target.startMs && t < target.nextStartMs) {
        clearGate(); // landed inside the restored block — safe to unmute
      }
      // else: a seek that did NOT land in the restored block — stay muted; the
      // loadedmetadata re-seek or the fallback re-assert will retry.
    };
    el.addEventListener("seeked", onSeeked);
    el.addEventListener("loadedmetadata", onLoadedMetadata);
    return () => {
      el.removeEventListener("seeked", onSeeked);
      el.removeEventListener("loadedmetadata", onLoadedMetadata);
    };
  }, [audioRef, clearGate, seek]);

  // rAF active-block derivation. Writes SET_ACTIVE_BLOCK only on a block-id
  // change (Decision 2026-06-21). While restoring, writes NOTHING (R2): a
  // not-yet-loaded element reports currentTime 0 and would otherwise derive
  // block 0 and clobber the restored block.
  useEffect(() => {
    let raf = 0;
    const tick = () => {
      raf = requestAnimationFrame(tick);
      const el = audioRef.current;
      if (!el) return;
      const t = el.currentTime * 1000;

      // Continuous audio-clock channel (moving bar + time readout). Pushed
      // imperatively to subscribers every frame — NO React state write, so the
      // no-per-frame-re-render decision holds. Emitted even while restoring so
      // the clock stays live; it is display-only and never gates anything.
      if (progressSubsRef.current.size > 0) {
        const durMs = Number.isFinite(el.duration) ? el.duration * 1000 : 0;
        const frac = durMs > 0 ? clamp(t / durMs, 0, 1) : 0;
        const p: PlaybackProgress = { currentMs: t, durationMs: durMs, fraction: frac };
        progressSubsRef.current.forEach((fn) => fn(p));
      }

      // Block derivation stays MUTED while restoring (R2): a not-yet-loaded
      // element reports currentTime 0 and would clobber the restored block.
      if (restoringRef.current) return;
      const idx = findActiveBlockIndex(geom.starts, geom.ends, t);
      const id = idx >= 0 ? geom.ids[idx] : null;
      if (id !== activeIdRef.current) {
        setActive(id);
      }
    };
    raf = requestAnimationFrame(tick);
    return () => cancelAnimationFrame(raf);
  }, [audioRef, geom.starts, geom.ends, geom.ids, setActive]);

  // Resume writer: persist on a genuine active-block transition. Muted while
  // restoring (R2) and on the first value equal to the restored block (R3 echo
  // guard), so a pure restore writes nothing.
  useEffect(() => {
    if (restoringRef.current) return;
    if (!resumeKeyStr || activeBlockId == null) return;
    if (activeBlockId === lastWrittenRef.current) return;
    const idx = geom.ids.indexOf(activeBlockId);
    // Belt-and-suspenders (R2/R3 backstop): never overwrite a real saved resume
    // with a "fell back to block 0" position before the element has metadata. If
    // we derived index 0 while readyState < HAVE_METADATA, the clock is an
    // un-landed currentTime 0, not a genuine position — skip the write. Once the
    // element has metadata, a genuine block-0 position persists normally.
    const el = audioRef.current;
    if (idx === 0 && el && el.readyState < 1 /* HAVE_METADATA */) return;
    lastWrittenRef.current = activeBlockId;
    writeResume(resumeKeyStr, {
      schemaVersion: RESUME_SCHEMA_VERSION,
      blockId: activeBlockId,
      blockOrder: idx,
      blockSignature: geom.signature,
      updatedAt: Date.now(),
    });
  }, [activeBlockId, resumeKeyStr, geom.ids, geom.signature, audioRef]);

  useEffect(() => () => clearGate(), [clearGate]);

  const activeBlockIndex = useMemo(
    () => (activeBlockId ? geom.ids.indexOf(activeBlockId) : -1),
    [activeBlockId, geom.ids],
  );

  const isPlaying = playbackState === "playing";

  const seekToIndex = useCallback(
    (index: number) => {
      if (geom.starts.length === 0) return;
      const i = clamp(index, 0, geom.starts.length - 1);
      seek(geom.starts[i]);
    },
    [geom.starts, seek],
  );

  const seekToBlock = useCallback(
    (blockId: string) => {
      const idx = geom.ids.indexOf(blockId);
      if (idx >= 0) seek(geom.starts[idx]);
    },
    [geom.ids, geom.starts, seek],
  );

  const playFromBlock = useCallback(
    (blockId: string) => {
      seekToBlock(blockId);
      play();
    },
    [seekToBlock, play],
  );

  // Step base: from the active block, else block 0.
  const baseIndex = activeBlockIndex >= 0 ? activeBlockIndex : 0;
  const prevBlock = useCallback(() => seekToIndex(baseIndex - 1), [seekToIndex, baseIndex]);
  const nextBlock = useCallback(() => seekToIndex(baseIndex + 1), [seekToIndex, baseIndex]);
  const stepBlocks = useCallback(
    (delta: number) => seekToIndex(baseIndex + delta),
    [seekToIndex, baseIndex],
  );

  const toggle = useCallback(() => {
    if (isPlaying) pause();
    else play();
  }, [isPlaying, play, pause]);

  const returnToPlayingBlock = useCallback(() => {
    if (!activeBlockId || typeof document === "undefined") return;
    const row = document.querySelector<HTMLElement>(`[data-block-id="${activeBlockId}"]`);
    const target = row?.querySelector<HTMLElement>(".block-row__play") ?? row;
    target?.scrollIntoView({ block: "center" });
    target?.focus();
  }, [activeBlockId]);

  // Memoize the command surface so an unrelated NarrationProvider re-render does
  // not mint a fresh context value and re-render every consumer (incl. the whole
  // BlockRow list). Identity changes only when a listed dependency actually does.
  return useMemo(
    () => ({
      activeBlockId,
      activeBlockIndex,
      blockCount: geom.ids.length,
      blockStartsMs: geom.starts,
      subscribeProgress,
      playbackState,
      isPlaying,
      play,
      pause,
      toggle,
      seekToBlock,
      seekToIndex,
      playFromBlock,
      prevBlock,
      nextBlock,
      stepBlocks,
      returnToPlayingBlock,
    }),
    [
      activeBlockId,
      activeBlockIndex,
      geom.ids,
      geom.starts,
      subscribeProgress,
      playbackState,
      isPlaying,
      play,
      pause,
      toggle,
      seekToBlock,
      seekToIndex,
      playFromBlock,
      prevBlock,
      nextBlock,
      stepBlocks,
      returnToPlayingBlock,
    ],
  );
}
