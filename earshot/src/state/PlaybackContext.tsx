// PlaybackContext.tsx — distributes the SINGLE usePlayback instance to the tree
// (#112, R4). usePlayback is instantiated exactly once (at NarrationProvider)
// and shared here; TransportDeck, BlockScrubber, and (via TranscriptPane)
// BlockRow all READ the same command surface + activeBlockId, driving the one
// shared <audio>. No leaf component calls usePlayback() locally.

import { createContext, useContext } from "react";
import type { PlaybackControls } from "../hooks/usePlayback";

const PlaybackContext = createContext<PlaybackControls | null>(null);

export function PlaybackProvider({
  value,
  children,
}: {
  value: PlaybackControls;
  children: React.ReactNode;
}) {
  return <PlaybackContext.Provider value={value}>{children}</PlaybackContext.Provider>;
}

/** Read the shared playback engine. Throws if used outside the provider. */
export function usePlaybackControls(): PlaybackControls {
  const ctx = useContext(PlaybackContext);
  if (!ctx) {
    throw new Error("usePlaybackControls must be used within a PlaybackProvider");
  }
  return ctx;
}
