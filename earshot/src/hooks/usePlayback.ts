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
/** Fallback gate-clear if neither seeked nor loadedmetadata ever fires. */
const RESTORE_GATE_TIMEOUT_MS = 2000;

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

/** The transport command surface, distributed via context. */
export interface PlaybackControls {
  activeBlockId: string | null;
  /** Index of the active block in document order, or -1 when none. */
  activeBlockIndex: number;
  blockCount: number;
  /** Sorted block-start offsets (for the scrubber aria-valuetext). */
  blockStartsMs: number[];
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

  const resumeKeyStr = resumeScope ? resumeKey(`entry:${resumeScope}`) : null;

  const clearGate = useCallback(() => {
    restoringRef.current = false;
    if (gateTimerRef.current != null) {
      clearTimeout(gateTimerRef.current);
      gateTimerRef.current = null;
    }
  }, []);

  const setActive = useCallback((id: string | null) => {
    activeIdRef.current = id;
    setActiveBlockId(id);
  }, []);

  // initialize-not-transition (R3): seed the active block WITHOUT going through
  // the resume-persist transition writer. Sets the restoring gate so the rAF
  // loop stays muted until the seek lands, and re-derives startMs live from the
  // timeline (the store never persists an ms offset).
  const initializeActiveBlock = useCallback(
    (blockId: string) => {
      const idx = geom.ids.indexOf(blockId);
      if (idx < 0) return;
      restoringRef.current = true;
      lastWrittenRef.current = blockId; // echo-guard belt
      setActive(blockId);
      seek(geom.starts[idx]); // no autoplay — restore position only
      // Bounded fallback so the gate can never stay stuck muted if neither a
      // seeked nor a loadedmetadata event arrives.
      if (gateTimerRef.current != null) clearTimeout(gateTimerRef.current);
      gateTimerRef.current = window.setTimeout(clearGate, RESTORE_GATE_TIMEOUT_MS);
    },
    [geom.ids, geom.starts, seek, setActive, clearGate],
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
    // initializeActiveBlock is stable per geom; depend on the signature + scope.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [geom.signature, resumeKeyStr]);

  // Clear the restoring gate on the first seeked / loadedmetadata from the
  // shared <audio> — i.e. once currentTime has actually landed in the restored
  // block (R2). Read-only subscription; does not touch useAudio's contract.
  useEffect(() => {
    const el = audioRef.current;
    if (!el) return;
    const onLanded = () => clearGate();
    el.addEventListener("seeked", onLanded);
    el.addEventListener("loadedmetadata", onLanded);
    return () => {
      el.removeEventListener("seeked", onLanded);
      el.removeEventListener("loadedmetadata", onLanded);
    };
  }, [audioRef, clearGate]);

  // rAF active-block derivation. Writes SET_ACTIVE_BLOCK only on a block-id
  // change (Decision 2026-06-21). While restoring, writes NOTHING (R2): a
  // not-yet-loaded element reports currentTime 0 and would otherwise derive
  // block 0 and clobber the restored block.
  useEffect(() => {
    let raf = 0;
    const tick = () => {
      raf = requestAnimationFrame(tick);
      if (restoringRef.current) return;
      const el = audioRef.current;
      if (!el) return;
      const t = el.currentTime * 1000;
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
    lastWrittenRef.current = activeBlockId;
    const idx = geom.ids.indexOf(activeBlockId);
    writeResume(resumeKeyStr, {
      schemaVersion: RESUME_SCHEMA_VERSION,
      blockId: activeBlockId,
      blockOrder: idx,
      blockSignature: geom.signature,
      updatedAt: Date.now(),
    });
  }, [activeBlockId, resumeKeyStr, geom.ids, geom.signature]);

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

  return {
    activeBlockId,
    activeBlockIndex,
    blockCount: geom.ids.length,
    blockStartsMs: geom.starts,
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
  };
}
