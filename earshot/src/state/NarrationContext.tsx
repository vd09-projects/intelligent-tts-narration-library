// NarrationContext.tsx — exposes the single useNarrationSession owner to both
// panes (plan State Ownership). NarrationProvider calls the owner hook ONCE and
// shares it; useNarration() reads it. This prevents two divergent sources
// feeding one TranscriptPane.

import { createContext, useContext } from "react";
import {
  useNarrationSession,
  type NarrationSession,
} from "../hooks/useNarrationSession";
import { usePlayback } from "../hooks/usePlayback";
import { PlaybackProvider } from "./PlaybackContext";

const NarrationContext = createContext<NarrationSession | null>(null);

export function NarrationProvider({ children }: { children: React.ReactNode }) {
  const session = useNarrationSession();
  // SINGLE usePlayback instance (#112, R4): created once here and distributed
  // via PlaybackContext. One rAF loop, one active-block writer, one <audio>.
  const playback = usePlayback({
    audioRef: session.audioRef,
    timeline: session.currentTranscript?.timeline,
    resumeScope: session.selectedEntryId,
    playbackState: session.playbackState,
    seek: session.seek,
    play: session.play,
    pause: session.pause,
  });
  return (
    <NarrationContext.Provider value={session}>
      <PlaybackProvider value={playback}>{children}</PlaybackProvider>
    </NarrationContext.Provider>
  );
}

/** Read the shared narration owner. Throws if used outside the provider. */
export function useNarration(): NarrationSession {
  const ctx = useContext(NarrationContext);
  if (!ctx) {
    throw new Error("useNarration must be used within a NarrationProvider");
  }
  return ctx;
}
