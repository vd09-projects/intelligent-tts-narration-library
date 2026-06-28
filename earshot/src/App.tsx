// App.tsx — composition root for the Earshot shell. Mounts the two providers
// (Announcer = the polite live region store; Narration = the SINGLE owner of
// transcript/audio/active-block) and lays out the three-region list-detail shell
// (plan Step 2): one <header>, an <aside> session+file pane, one <main>
// transcript, and a bottom-fixed transport (role="toolbar").

import { useEffect, useState } from "react";
import { AnnouncerProvider, useAnnouncer } from "./state/Announcer";
import { NarrationProvider, useNarration } from "./state/NarrationContext";
import { AppHeader } from "./components/AppHeader";
import { SessionPane } from "./components/SessionPane";
import { FilePane } from "./components/FilePane";
import { TranscriptPane } from "./components/TranscriptPane";
import { TransportBar } from "./components/TransportBar";
import { LiveRegion } from "./components/LiveRegion";
import { ErrorBanner } from "./components/ErrorBanner";
import "./styles/layout.css";

function Shell() {
  const { request, audioError, playbackState } = useNarration();
  const { announce } = useAnnouncer();
  const [sessionOpen, setSessionOpen] = useState(true);

  // Polite playback announcement (assertive errors go through ErrorBanner).
  useEffect(() => {
    if (playbackState === "playing") {
      announce("Playing");
    } else if (playbackState === "loading") {
      announce("Preparing audio");
    }
  }, [playbackState, announce]);

  // App-level assertive surface: audio-load failure (always) + a session-narrate
  // fetch error. File-narrate and session-message errors surface in their panes.
  const globalError =
    audioError ??
    (request.status === "error" && request.source === "session" ? request.error : null);

  return (
    <div className={"app-shell" + (sessionOpen ? " session-open" : " session-collapsed")}>
      <LiveRegion />
      <AppHeader sessionOpen={sessionOpen} onToggleSession={() => setSessionOpen((o) => !o)} />
      <div className="app-shell__body">
        <aside id="session-pane" className="app-shell__aside" aria-label="Sessions and files">
          <FilePane />
          <SessionPane />
        </aside>
        <div className="app-shell__center">
          <ErrorBanner message={globalError} />
          <TranscriptPane />
        </div>
      </div>
      <TransportBar />
    </div>
  );
}

export default function App() {
  return (
    <AnnouncerProvider>
      <NarrationProvider>
        <Shell />
      </NarrationProvider>
    </AnnouncerProvider>
  );
}
