// useAudio.ts — binds an OPAQUE audio_url (D4) to a single <audio> element and
// surfaces playback state + a distinct audio-load-failure path.
//
// Design choices (plan Phase 4):
//   - audio_url is used VERBATIM as <audio src> (D4). We deliberately do NOT
//     fetch the wav into a Blob + createObjectURL: there is then no object URL to
//     leak, so the "revoke on replace/unmount" concern does not arise (the plan's
//     revocation requirement is conditional — "if createObjectURL is used").
//   - The native <audio> "error" event is the load-failure path. It is DISTINCT
//     from a fetch/narrate error (which the owner raises before any audio_url
//     exists): a fetch error means "couldn't get the narration"; an audio error
//     means "couldn't play audio".
//   - currentTimeMs is tracked from "timeupdate" so the owner can DERIVE the
//     active block from the timeline (never stored as independent state).

import { useCallback, useEffect, useRef, useState } from "react";

export type PlaybackState = "idle" | "loading" | "playing" | "paused" | "error";

export interface UseAudio {
  /** Attach to a single <audio> element rendered by the transport. */
  audioRef: React.RefObject<HTMLAudioElement>;
  playbackState: PlaybackState;
  /** Set iff the <audio> failed to load/play. Distinct from a fetch error. */
  audioError: string | null;
  /** Current playback position in milliseconds (for active-block derivation). */
  currentTimeMs: number;
  /** Point the element at a new opaque url and reset state. Empty string clears. */
  setSource: (audioUrl: string) => void;
  play: () => void;
  pause: () => void;
  /** Seek to a position in milliseconds (clamped to >= 0). */
  seek: (ms: number) => void;
  /**
   * Force the element to RE-FETCH its (unchanged, stable) src — the combined wav
   * was rewritten in place at the same URL after a block escalation (#113).
   * Preserves the current play position + play state across the load() so an
   * escalate-while-playing does not jump to zero. No URL mutation (D4 — the URL
   * is opaque; the server serves it Cache-Control: no-store).
   */
  reload: () => void;
}

const AUDIO_LOAD_FAILED = "Couldn't play audio";

export function useAudio(): UseAudio {
  const audioRef = useRef<HTMLAudioElement>(null);
  const [playbackState, setPlaybackState] = useState<PlaybackState>("idle");
  const [audioError, setAudioError] = useState<string | null>(null);
  const [currentTimeMs, setCurrentTimeMs] = useState(0);

  // Wire the element's events once it is mounted.
  useEffect(() => {
    const el = audioRef.current;
    if (!el) {
      return;
    }
    const onPlaying = () => {
      setPlaybackState("playing");
      setAudioError(null);
    };
    const onPause = () => {
      setPlaybackState((s) => (s === "error" ? s : "paused"));
    };
    const onEnded = () => setPlaybackState("paused");
    const onTimeUpdate = () => setCurrentTimeMs(Math.round(el.currentTime * 1000));
    const onError = () => {
      setPlaybackState("error");
      setAudioError(AUDIO_LOAD_FAILED);
    };
    el.addEventListener("playing", onPlaying);
    el.addEventListener("pause", onPause);
    el.addEventListener("ended", onEnded);
    el.addEventListener("timeupdate", onTimeUpdate);
    el.addEventListener("error", onError);
    return () => {
      el.removeEventListener("playing", onPlaying);
      el.removeEventListener("pause", onPause);
      el.removeEventListener("ended", onEnded);
      el.removeEventListener("timeupdate", onTimeUpdate);
      el.removeEventListener("error", onError);
    };
  }, []);

  const setSource = useCallback((audioUrl: string) => {
    const el = audioRef.current;
    setAudioError(null);
    setCurrentTimeMs(0);
    if (!el) {
      return;
    }
    if (audioUrl) {
      el.src = audioUrl; // opaque, verbatim (D4)
      el.load();
      setPlaybackState("loading");
    } else {
      el.removeAttribute("src");
      el.load();
      setPlaybackState("idle");
    }
  }, []);

  const play = useCallback(() => {
    const el = audioRef.current;
    if (!el) {
      return;
    }
    void el.play().catch(() => {
      setPlaybackState("error");
      setAudioError(AUDIO_LOAD_FAILED);
    });
  }, []);

  const pause = useCallback(() => {
    audioRef.current?.pause();
  }, []);

  const seek = useCallback((ms: number) => {
    const el = audioRef.current;
    if (el) {
      el.currentTime = Math.max(0, ms) / 1000;
      setCurrentTimeMs(Math.max(0, ms));
    }
  }, []);

  const reload = useCallback(() => {
    const el = audioRef.current;
    if (!el || !el.getAttribute("src")) {
      return; // nothing loaded — nothing to refresh
    }
    const resumeAt = el.currentTime;
    const wasPlaying = !el.paused && !el.ended;
    setPlaybackState("loading");
    const onReady = () => {
      el.removeEventListener("loadeddata", onReady);
      el.currentTime = resumeAt;
      setCurrentTimeMs(Math.round(resumeAt * 1000));
      if (wasPlaying) {
        void el.play().catch(() => {
          setPlaybackState("error");
          setAudioError(AUDIO_LOAD_FAILED);
        });
      }
    };
    el.addEventListener("loadeddata", onReady);
    el.load(); // re-fetch the rewritten bytes at the same opaque src
  }, []);

  return { audioRef, playbackState, audioError, currentTimeMs, setSource, play, pause, seek, reload };
}
