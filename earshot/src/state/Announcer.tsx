// Announcer.tsx — a single POLITE live-region message store, shared across the
// shell (plan Live Region & Announcement Plan). The live node itself is mounted
// once at load (LiveRegion), BEFORE any message — never injected at announcement
// time — so a screen reader observes it. Assertive announcements (fetch / audio
// errors) are handled by ErrorBanner (role="alert"), which also takes focus.

import { createContext, useCallback, useContext, useMemo, useState } from "react";

interface Announcer {
  politeMessage: string;
  announce: (message: string) => void;
}

const AnnouncerContext = createContext<Announcer | null>(null);

export function AnnouncerProvider({ children }: { children: React.ReactNode }) {
  const [politeMessage, setPoliteMessage] = useState("");
  const announce = useCallback((message: string) => setPoliteMessage(message), []);
  const value = useMemo(() => ({ politeMessage, announce }), [politeMessage, announce]);
  return <AnnouncerContext.Provider value={value}>{children}</AnnouncerContext.Provider>;
}

export function useAnnouncer(): Announcer {
  const ctx = useContext(AnnouncerContext);
  if (!ctx) {
    throw new Error("useAnnouncer must be used within an AnnouncerProvider");
  }
  return ctx;
}
