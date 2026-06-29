// App.tsx — composition root. Three-region shell with a tabbed aside (File /
// Sessions), a center transcript + error banner, and a bottom transport.

import { useEffect, useState } from "react";
import { AnnouncerProvider, useAnnouncer } from "./state/Announcer";
import { NarrationProvider, useNarration } from "./state/NarrationContext";
import { useTransportHotkeys } from "./hooks/useTransportHotkeys";
import { AppHeader } from "./components/AppHeader";
import { SessionPane } from "./components/SessionPane";
import { FilePane } from "./components/FilePane";
import { TranscriptPane } from "./components/TranscriptPane";
import { TransportDeck } from "./components/TransportDeck";
import { LiveRegion } from "./components/LiveRegion";
import { ErrorBanner } from "./components/ErrorBanner";
import "./styles/layout.css";

type AsideTab = "file" | "session";

function Shell() {
  const { selectedEntry, audioError, playbackState } = useNarration();
  const { announce } = useAnnouncer();
  const [sessionOpen, setSessionOpen] = useState(true);
  const [activeTab, setActiveTab] = useState<AsideTab>("file");

  // Global keyboard transport (#134): Space play/pause, ←/→ ±10s. Mounted once
  // here so it reads the single shared playback engine; the focus guard keeps it
  // inert outside the body / the [data-transport-hotkeys] transcript region.
  useTransportHotkeys();

  useEffect(() => {
    if (playbackState === "playing") {
      announce("Playing");
    } else if (playbackState === "loading") {
      announce("Preparing audio");
    }
  }, [playbackState, announce]);

  const globalError =
    audioError ?? (selectedEntry?.status === "error" ? selectedEntry.error : null);

  return (
    <div className={"app-shell" + (sessionOpen ? " session-open" : " session-collapsed")}>
      <LiveRegion />
      <AppHeader sessionOpen={sessionOpen} onToggleSession={() => setSessionOpen((o) => !o)} />
      <div className="app-shell__body">
        <aside id="session-pane" className="app-shell__aside" aria-label="Sessions and files">
          <div role="tablist" aria-label="Input source" className="aside-tabs">
            <button
              role="tab"
              id="tab-file"
              aria-selected={activeTab === "file"}
              aria-controls="tabpanel-file"
              className={"aside-tab" + (activeTab === "file" ? " is-active" : "")}
              onClick={() => setActiveTab("file")}
            >
              File
            </button>
            <button
              role="tab"
              id="tab-session"
              aria-selected={activeTab === "session"}
              aria-controls="tabpanel-session"
              className={"aside-tab" + (activeTab === "session" ? " is-active" : "")}
              onClick={() => setActiveTab("session")}
            >
              Sessions
            </button>
          </div>

          <div
            role="tabpanel"
            id="tabpanel-file"
            aria-labelledby="tab-file"
            hidden={activeTab !== "file"}
          >
            <FilePane />
          </div>
          <div
            role="tabpanel"
            id="tabpanel-session"
            aria-labelledby="tab-session"
            hidden={activeTab !== "session"}
          >
            <SessionPane />
          </div>
        </aside>

        <div className="app-shell__center">
          <ErrorBanner message={globalError} />
          <TranscriptPane />
        </div>
      </div>
      <TransportDeck />
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
