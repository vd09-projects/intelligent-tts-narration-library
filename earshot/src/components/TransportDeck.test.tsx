import { afterEach, describe, expect, it, vi } from "vitest";
import { fireEvent, render, screen } from "@testing-library/react";
import { TransportDeck } from "./TransportDeck";
import { PlaybackProvider } from "../state/PlaybackContext";
import type { PlaybackControls } from "../hooks/usePlayback";

// Mock the narration owner so the deck gets a controllable audio_url without the
// heavy session. h.url is mutated per test (no-audio case).
const h = vi.hoisted(() => ({ url: "x.wav" as string | null }));
vi.mock("../state/NarrationContext", () => ({
  useNarration: () => ({ audioRef: { current: null }, activeAudioUrl: h.url }),
}));

function makePlayback(over: Partial<PlaybackControls> = {}): PlaybackControls {
  return {
    activeBlockId: "b001",
    activeBlockIndex: 0,
    blockCount: 4,
    blockStartsMs: [0, 3200, 6100, 9000],
    subscribeProgress: vi.fn(() => () => {}),
    playbackState: "paused",
    isPlaying: false,
    play: vi.fn(),
    pause: vi.fn(),
    toggle: vi.fn(),
    seekToBlock: vi.fn(),
    seekToIndex: vi.fn(),
    playFromBlock: vi.fn(),
    prevBlock: vi.fn(),
    nextBlock: vi.fn(),
    stepBlocks: vi.fn(),
    returnToPlayingBlock: vi.fn(),
    ...over,
  };
}

function renderDeck(pb: PlaybackControls) {
  return render(
    <PlaybackProvider value={pb}>
      <TransportDeck />
    </PlaybackProvider>,
  );
}

afterEach(() => {
  h.url = "x.wav";
  vi.clearAllMocks();
});

describe("TransportDeck — APG toolbar roving tabindex (single tab stop)", () => {
  it("exposes exactly one toolbar tab stop: one button has tabindex 0", () => {
    renderDeck(makePlayback());
    const toolbar = screen.getByRole("toolbar", { name: "Playback transport" });
    const buttons = screen.getAllByRole("button");
    expect(buttons).toHaveLength(4);
    const tabbable = buttons.filter((b) => b.getAttribute("tabindex") === "0");
    expect(tabbable).toHaveLength(1);
    expect(toolbar).toBeInTheDocument();
  });

  it("ArrowRight/Left move roving focus; Home/End jump to ends", () => {
    renderDeck(makePlayback());
    const toolbar = screen.getByRole("toolbar", { name: "Playback transport" });
    const prev = screen.getByRole("button", { name: "Previous block" });
    const playPause = screen.getByRole("button", { name: "Play" });
    const ret = screen.getByRole("button", { name: "Return to playing block" });

    expect(prev).toHaveAttribute("tabindex", "0"); // first is the initial stop

    fireEvent.keyDown(toolbar, { key: "ArrowRight" });
    expect(playPause).toHaveFocus();
    expect(playPause).toHaveAttribute("tabindex", "0");
    expect(prev).toHaveAttribute("tabindex", "-1");

    fireEvent.keyDown(toolbar, { key: "End" });
    expect(ret).toHaveFocus();

    fireEvent.keyDown(toolbar, { key: "Home" });
    expect(prev).toHaveFocus();
  });

  it("R1: a toolbar ArrowRight moves button focus and does NOT change the block value", () => {
    const pb = makePlayback();
    renderDeck(pb);
    const toolbar = screen.getByRole("toolbar", { name: "Playback transport" });
    fireEvent.keyDown(toolbar, { key: "ArrowRight" });
    expect(screen.getByRole("button", { name: "Play" })).toHaveFocus();
    // The toolbar's arrow is roving-only — it must not step the scrubber value.
    expect(pb.nextBlock).not.toHaveBeenCalled();
    expect(pb.seekToIndex).not.toHaveBeenCalled();
  });
});

describe("TransportDeck — controls", () => {
  it("play/pause uses the action-label model (label = action, NO aria-pressed)", () => {
    const pb = makePlayback({ isPlaying: false });
    const { rerender } = renderDeck(pb);
    const playBtn = screen.getByRole("button", { name: "Play" });
    expect(playBtn).not.toHaveAttribute("aria-pressed");
    fireEvent.click(playBtn);
    expect(pb.toggle).toHaveBeenCalledTimes(1);

    // When playing, the label flips to "Pause" (still no aria-pressed).
    rerender(
      <PlaybackProvider value={makePlayback({ isPlaying: true })}>
        <TransportDeck />
      </PlaybackProvider>,
    );
    const pauseBtn = screen.getByRole("button", { name: "Pause" });
    expect(pauseBtn).not.toHaveAttribute("aria-pressed");
  });

  it("prev/next/return invoke the shared playback commands", () => {
    const pb = makePlayback();
    renderDeck(pb);
    fireEvent.click(screen.getByRole("button", { name: "Previous block" }));
    expect(pb.prevBlock).toHaveBeenCalledTimes(1);
    fireEvent.click(screen.getByRole("button", { name: "Next block" }));
    expect(pb.nextBlock).toHaveBeenCalledTimes(1);
    fireEvent.click(screen.getByRole("button", { name: "Return to playing block" }));
    expect(pb.returnToPlayingBlock).toHaveBeenCalledTimes(1);
  });

  it("renders the scrubber as a separate tab stop adjacent to the toolbar", () => {
    renderDeck(makePlayback());
    expect(screen.getByRole("slider", { name: "Block position" })).toBeInTheDocument();
  });

  it("disables 'Return to playing block' until a block is actually playing", () => {
    // No active block yet → nothing to return to → button disabled (not a
    // silent no-op). The other transport buttons stay enabled.
    renderDeck(makePlayback({ activeBlockId: null }));
    expect(screen.getByRole("button", { name: "Return to playing block" })).toBeDisabled();
    expect(screen.getByRole("button", { name: "Previous block" })).not.toBeDisabled();
  });

  it("enables 'Return to playing block' once a block is active", () => {
    renderDeck(makePlayback({ activeBlockId: "b001" }));
    expect(screen.getByRole("button", { name: "Return to playing block" })).not.toBeDisabled();
  });

  it("renders the elapsed/total time clock and subscribes to the audio progress channel", () => {
    const pb = makePlayback();
    renderDeck(pb);
    const clock = screen.getByTestId("transport-clock");
    expect(clock).toBeInTheDocument();
    expect(clock).toHaveTextContent("0:00");
    expect(clock).toHaveTextContent("--:--"); // duration unknown until the element reports it
    // The clock drives itself off the per-frame progress channel.
    expect(pb.subscribeProgress).toHaveBeenCalled();
  });

  it("shows the 'No audio loaded' hint and disables controls when there is no audio", () => {
    h.url = null;
    renderDeck(makePlayback({ blockCount: 0 }));
    expect(screen.getByText("No audio loaded")).toBeInTheDocument();
    screen.getAllByRole("button").forEach((b) => expect(b).toBeDisabled());
  });
});
