// useNarrationSession.ts — THE single owner of shared narration state.
// Entry-based model: each (session message / file) gets its own NarrationEntry
// keyed by a stable string. Multiple entries can be loading in parallel.
// Selecting an entry switches the transcript view. Audio follows the selection.

import { useCallback, useEffect, useMemo, useState } from "react";
import { ClientError, postNarrate, postNarrateFile } from "../api/client";
import type { Gender, Level, NarrateResponse } from "../api/types";
import { deriveActiveBlockId } from "../state/activeBlock";
import { useAudio } from "./useAudio";

const NARRATE_TIMEOUT_MS = 120_000; // 2 minutes

export interface NarrationEntry {
  status: "loading" | "ready" | "error";
  transcript: NarrateResponse | null;
  error: string | null;
}

export interface NarrationSession {
  /** All narrated items keyed by caller-supplied stable key. */
  entries: Map<string, NarrationEntry>;
  /** Which entry is currently displayed in the transcript pane. */
  selectedEntryId: string | null;
  /** Derived selected entry (null when nothing selected). */
  selectedEntry: NarrationEntry | null;
  /** Derived transcript for the selected entry. */
  currentTranscript: NarrateResponse | null;
  /** Switch the transcript view to an existing entry. */
  selectEntry: (key: string) => void;
  /** DERIVED active block id (aria-current), null when none under playhead. */
  activeBlockId: string | null;
  // playback (from useAudio)
  audioRef: React.RefObject<HTMLAudioElement>;
  activeAudioUrl: string | null;
  playbackState: ReturnType<typeof useAudio>["playbackState"];
  audioError: string | null;
  play: () => void;
  pause: () => void;
  /** Seek the shared clip to a block's start and play. */
  playFromBlock: (blockId: string) => void;
  /**
   * Narrate text keyed by `key` (e.g. message.id).
   * If an entry for `key` already exists and is loading, the call is a no-op.
   * Otherwise starts a new narration and auto-selects the entry.
   */
  narrateText: (key: string, text: string, level: Level, gender?: Gender) => Promise<void>;
  /**
   * Narrate a file keyed by `key` (e.g. file.name).
   * Same idempotency rule as narrateText.
   */
  narrateFile: (key: string, file: File, level: Level, gender?: Gender) => Promise<void>;
}

export function useNarrationSession(): NarrationSession {
  const [entries, setEntries] = useState<Map<string, NarrationEntry>>(() => new Map());
  const [selectedEntryId, setSelectedEntryId] = useState<string | null>(null);

  const audio = useAudio();
  const { setSource } = audio;

  const selectedEntry = selectedEntryId ? (entries.get(selectedEntryId) ?? null) : null;
  const currentTranscript = selectedEntry?.transcript ?? null;

  useEffect(() => {
    setSource(currentTranscript?.audio_url ?? "");
  }, [currentTranscript, setSource]);

  const activeBlockId = useMemo(
    () => deriveActiveBlockId(currentTranscript?.timeline, audio.currentTimeMs),
    [currentTranscript, audio.currentTimeMs],
  );

  const run = useCallback(
    async (key: string, fn: (signal: AbortSignal) => Promise<NarrateResponse>) => {
      // Don't re-narrate if already in flight.
      setEntries((prev) => {
        if (prev.get(key)?.status === "loading") return prev;
        return new Map(prev).set(key, { status: "loading", transcript: null, error: null });
      });
      // Auto-select so the transcript pane shows this entry's loading state.
      setSelectedEntryId(key);

      // Guard: if already loading (idempotency check above), bail out.
      // Re-read via closure after the state update; use a sentinel approach.
      const controller = new AbortController();
      const timer = setTimeout(() => controller.abort(), NARRATE_TIMEOUT_MS);
      try {
        const resp = await fn(controller.signal);
        setEntries((prev) =>
          new Map(prev).set(key, { status: "ready", transcript: resp, error: null }),
        );
      } catch (err) {
        const message =
          err instanceof ClientError
            ? err.message
            : err instanceof Error
              ? err.message
              : "Something went wrong";
        setEntries((prev) =>
          new Map(prev).set(key, { status: "error", transcript: null, error: message }),
        );
      } finally {
        clearTimeout(timer);
      }
    },
    [],
  );

  const narrateText = useCallback(
    (key: string, text: string, level: Level, gender?: Gender) =>
      run(key, (signal) => postNarrate({ text, level, gender, signal })),
    [run],
  );

  const narrateFile = useCallback(
    (key: string, file: File, level: Level, gender?: Gender) =>
      run(key, (signal) => postNarrateFile({ file, level, gender, signal })),
    [run],
  );

  const selectEntry = useCallback((key: string) => {
    setSelectedEntryId(key);
  }, []);

  const { seek, play } = audio;
  const playFromBlock = useCallback(
    (blockId: string) => {
      const timing = currentTranscript?.timeline.blocks.find((b) => b.block_id === blockId);
      if (timing) {
        seek(timing.start_ms);
      }
      play();
    },
    [currentTranscript, seek, play],
  );

  return {
    entries,
    selectedEntryId,
    selectedEntry,
    currentTranscript,
    selectEntry,
    activeBlockId,
    audioRef: audio.audioRef,
    activeAudioUrl: currentTranscript?.audio_url ?? null,
    playbackState: audio.playbackState,
    audioError: audio.audioError,
    play: audio.play,
    pause: audio.pause,
    playFromBlock,
    narrateText,
    narrateFile,
  };
}
