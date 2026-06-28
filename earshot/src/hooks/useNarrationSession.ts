// useNarrationSession.ts — THE single owner of shared narration state (plan
// State Ownership). Mounted once in App via NarrationProvider. Both SessionPane
// and FilePane dispatch into this owner; neither holds its own transcript/audio
// copy. TranscriptPane and TransportBar read from it.
//
// Owned state:
//   - currentTranscript  : the active NarrateResponse view model
//   - request status     : idle | loading | error (+ which source, + message)
//   - playback (via useAudio): activeAudioUrl is OPAQUE (D4)
//   - activeBlockId      : DERIVED from playback position vs timeline, NOT stored

import { useCallback, useEffect, useMemo, useState } from "react";
import { ClientError, postNarrate, postNarrateFile } from "../api/client";
import type { Gender, Level, NarrateResponse } from "../api/types";
import { deriveActiveBlockId } from "../state/activeBlock";
import { useAudio } from "./useAudio";

export type RequestSource = "session" | "file";

export interface RequestState {
  status: "idle" | "loading" | "error";
  /** Honest, human-readable error text for the assertive live region. */
  error: string | null;
  /** Which pane's request is in flight / errored (for distinct messaging). */
  source: RequestSource | null;
}

export interface NarrationSession {
  currentTranscript: NarrateResponse | null;
  request: RequestState;
  /** DERIVED active block id (aria-current), null when none under playhead. */
  activeBlockId: string | null;
  // playback (from useAudio)
  audioRef: React.RefObject<HTMLAudioElement>;
  activeAudioUrl: string | null;
  playbackState: ReturnType<typeof useAudio>["playbackState"];
  audioError: string | null;
  play: () => void;
  pause: () => void;
  /** Seek the shared clip to a block's start and play (BlockRow Space/Enter). */
  playFromBlock: (blockId: string) => void;
  // dispatchers (both panes call these)
  narrateText: (text: string, level: Level, gender?: Gender) => Promise<void>;
  narrateFile: (file: File, level: Level, gender?: Gender) => Promise<void>;
}

export function useNarrationSession(): NarrationSession {
  const [currentTranscript, setCurrentTranscript] = useState<NarrateResponse | null>(null);
  const [request, setRequest] = useState<RequestState>({
    status: "idle",
    error: null,
    source: null,
  });

  const audio = useAudio();
  const { setSource } = audio;

  // When a new transcript lands, point the audio element at its opaque url.
  const activeAudioUrl = currentTranscript?.audio_url ?? null;
  useEffect(() => {
    setSource(activeAudioUrl ?? "");
  }, [activeAudioUrl, setSource]);

  const activeBlockId = useMemo(
    () => deriveActiveBlockId(currentTranscript?.timeline, audio.currentTimeMs),
    [currentTranscript, audio.currentTimeMs],
  );

  // Shared request runner: set loading, run, store transcript or honest error.
  const run = useCallback(
    async (source: RequestSource, fn: () => Promise<NarrateResponse>) => {
      setRequest({ status: "loading", error: null, source });
      try {
        const resp = await fn();
        setCurrentTranscript(resp);
        setRequest({ status: "idle", error: null, source: null });
      } catch (err) {
        const message =
          err instanceof ClientError
            ? err.message
            : err instanceof Error
              ? err.message
              : "Something went wrong";
        setRequest({ status: "error", error: message, source });
      }
    },
    [],
  );

  const narrateText = useCallback(
    (text: string, level: Level, gender?: Gender) =>
      run("session", () => postNarrate({ text, level, gender })),
    [run],
  );

  const narrateFile = useCallback(
    (file: File, level: Level, gender?: Gender) =>
      run("file", () => postNarrateFile({ file, level, gender })),
    [run],
  );

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
    currentTranscript,
    request,
    activeBlockId,
    audioRef: audio.audioRef,
    activeAudioUrl,
    playbackState: audio.playbackState,
    audioError: audio.audioError,
    play: audio.play,
    pause: audio.pause,
    playFromBlock,
    narrateText,
    narrateFile,
  };
}
