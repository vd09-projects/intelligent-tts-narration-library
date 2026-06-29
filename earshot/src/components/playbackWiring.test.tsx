// playbackWiring.test.tsx — the R4 single-instance spine. Asserts usePlayback
// is instantiated EXACTLY ONCE and distributed via context, that exactly one
// rAF loop / one <audio> is created, and that BlockRow's onPlay and the deck
// controls drive the SAME shared playback instance + the one shared <audio>.

import { afterEach, describe, expect, it, vi } from "vitest";
import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import App from "../App";
import { AnnouncerProvider } from "../state/Announcer";
import { NarrationProvider } from "../state/NarrationContext";
import { usePlaybackControls } from "../state/PlaybackContext";
import type { PlaybackControls } from "../hooks/usePlayback";
import { createMockFetch } from "../mocks/server";

afterEach(() => {
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

describe("playback wiring — single usePlayback instance (R4)", () => {
  it("distributes ONE playback instance to every consumer", () => {
    const seen: PlaybackControls[] = [];
    function Consumer() {
      seen.push(usePlaybackControls());
      return null;
    }
    render(
      <AnnouncerProvider>
        <NarrationProvider>
          <Consumer />
          <Consumer />
          <Consumer />
        </NarrationProvider>
      </AnnouncerProvider>,
    );
    // All consumers read the identical object → one provider-level instance.
    expect(seen.length).toBeGreaterThanOrEqual(3);
    for (const ref of seen) {
      expect(ref).toBe(seen[0]);
    }
  });

  it("starts exactly ONE rAF loop for the whole tree", () => {
    const rafSpy = vi.spyOn(globalThis, "requestAnimationFrame");
    render(
      <AnnouncerProvider>
        <NarrationProvider>
          <div />
        </NarrationProvider>
      </AnnouncerProvider>,
    );
    // The single usePlayback effect schedules the loop once on mount. Two
    // instances would schedule two independent initial frames.
    expect(rafSpy).toHaveBeenCalledTimes(1);
  });

  it("renders exactly one shared <audio> element under the deck", () => {
    vi.stubGlobal("fetch", createMockFetch());
    render(<App />);
    expect(screen.getAllByTestId("audio-element")).toHaveLength(1);
    expect(screen.getAllByRole("toolbar", { name: "Playback transport" })).toHaveLength(1);
  });

  it("BlockRow play affordance and the deck drive the same shared <audio>", async () => {
    vi.stubGlobal("fetch", createMockFetch());
    const user = userEvent.setup();
    render(<App />);

    // Load a session and narrate a message so blocks + the deck are live.
    await user.click(screen.getByRole("tab", { name: "Sessions" }));
    await user.type(screen.getByLabelText("Session ID"), "demo-session-001");
    await user.click(screen.getByRole("button", { name: "Load" }));
    const options = await screen.findAllByRole("option");
    await user.click(options[1]);
    await waitFor(() =>
      expect(document.querySelectorAll("[data-block-id]").length).toBeGreaterThan(0),
    );

    // The one <audio> the deck owns is the same element BlockRow's seek targets.
    const audio = screen.getByTestId("audio-element") as HTMLAudioElement;
    const seekSpy = vi.fn();
    Object.defineProperty(audio, "currentTime", {
      configurable: true,
      get: () => 0,
      set: seekSpy,
    });

    // Click a BlockRow "Play from block N" affordance → seeks the shared audio.
    const rows = document.querySelectorAll("[data-block-id]");
    const secondRow = rows[1] as HTMLElement;
    const playBtn = within(secondRow).getByRole("button", { name: /play from block/i });
    await user.click(playBtn);
    expect(seekSpy).toHaveBeenCalled(); // BlockRow drove the deck's single <audio>
  });
});
