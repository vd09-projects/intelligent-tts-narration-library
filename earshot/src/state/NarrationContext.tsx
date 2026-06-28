// NarrationContext.tsx — exposes the single useNarrationSession owner to both
// panes (plan State Ownership). NarrationProvider calls the owner hook ONCE and
// shares it; useNarration() reads it. This prevents two divergent sources
// feeding one TranscriptPane.

import { createContext, useContext } from "react";
import {
  useNarrationSession,
  type NarrationSession,
} from "../hooks/useNarrationSession";

const NarrationContext = createContext<NarrationSession | null>(null);

export function NarrationProvider({ children }: { children: React.ReactNode }) {
  const session = useNarrationSession();
  return (
    <NarrationContext.Provider value={session}>
      {children}
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
