import { afterEach, describe, expect, it, vi } from "vitest";
import { fireEvent, render, screen } from "@testing-library/react";
import { BlockScrubber } from "./BlockScrubber";
import { PlaybackProvider } from "../state/PlaybackContext";
import type { PlaybackControls } from "../hooks/usePlayback";

function makePlayback(over: Partial<PlaybackControls> = {}): PlaybackControls {
  return {
    activeBlockId: "b001",
    activeBlockIndex: 0,
    blockCount: 4,
    blockStartsMs: [0, 3200, 6100, 105000],
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

function renderScrubber(pb: PlaybackControls, onParentKey?: (e: unknown) => void) {
  return render(
    <PlaybackProvider value={pb}>
      <div onKeyDown={onParentKey}>
        <BlockScrubber />
      </div>
    </PlaybackProvider>,
  );
}

afterEach(() => vi.restoreAllMocks());

describe("BlockScrubber — block-quantized slider semantics", () => {
  it("exposes role=slider with min/max/now and block-index aria-valuetext", () => {
    renderScrubber(makePlayback({ activeBlockIndex: 2, blockStartsMs: [0, 3200, 105000, 200000] }));
    const slider = screen.getByRole("slider", { name: "Block position" });
    expect(slider).toHaveAttribute("aria-valuemin", "0");
    expect(slider).toHaveAttribute("aria-valuemax", "3"); // n-1
    expect(slider).toHaveAttribute("aria-valuenow", "2"); // active index
    // valuetext = block i of n + spoken block-start time (1:45 for 105000ms).
    expect(slider).toHaveAttribute("aria-valuetext", "Block 3 of 4, 1:45");
  });

  it("renders nothing when there are no blocks", () => {
    const { container } = renderScrubber(makePlayback({ blockCount: 0, blockStartsMs: [] }));
    expect(container.querySelector('[role="slider"]')).toBeNull();
  });

  it("falls back to index 0 and announces 'No block selected' when no block is active (-1)", () => {
    renderScrubber(makePlayback({ activeBlockIndex: -1, activeBlockId: null }));
    const slider = screen.getByRole("slider");
    expect(slider).toHaveAttribute("aria-valuenow", "0");
    expect(slider).toHaveAttribute("aria-valuetext", "No block selected");
  });
});

describe("BlockScrubber — continuous progress fill (audio clock, not block index)", () => {
  it("drives the fill width off the per-frame audio-progress channel, not the block index", () => {
    // Capture the progress subscriber the scrubber registers, then push a
    // mid-clip fraction and assert the fill reflects the CONTINUOUS position —
    // proving the bar no longer jumps in block-index steps.
    let emit: ((p: { currentMs: number; durationMs: number; fraction: number }) => void) | null = null;
    const pb = makePlayback({
      activeBlockIndex: 0, // index 0 would be ~0% under the OLD index-based fill
      subscribeProgress: vi.fn((fn) => {
        emit = fn;
        return () => {};
      }),
    });
    const { container } = renderScrubber(pb);
    const fill = container.querySelector<HTMLElement>(".block-scrubber__fill");
    expect(fill).not.toBeNull();
    expect(fill!.style.width).toBe("0%"); // starts empty

    emit!({ currentMs: 21000, durationMs: 42000, fraction: 0.5 });
    expect(fill!.style.width).toBe("50%"); // continuous, independent of block index
  });
});

describe("BlockScrubber — keyboard owns its arrows (APG Slider)", () => {
  it("ArrowRight = next block, ArrowLeft = prev block", () => {
    const pb = makePlayback();
    renderScrubber(pb);
    const slider = screen.getByRole("slider");
    fireEvent.keyDown(slider, { key: "ArrowRight" });
    expect(pb.nextBlock).toHaveBeenCalledTimes(1);
    fireEvent.keyDown(slider, { key: "ArrowLeft" });
    expect(pb.prevBlock).toHaveBeenCalledTimes(1);
  });

  it("Home/End jump to first/last; PageUp/PageDown step ±N", () => {
    const pb = makePlayback();
    renderScrubber(pb);
    const slider = screen.getByRole("slider");
    fireEvent.keyDown(slider, { key: "Home" });
    expect(pb.seekToIndex).toHaveBeenCalledWith(0);
    fireEvent.keyDown(slider, { key: "End" });
    expect(pb.seekToIndex).toHaveBeenCalledWith(3); // n-1
    fireEvent.keyDown(slider, { key: "PageUp" });
    expect(pb.stepBlocks).toHaveBeenCalledWith(5);
    fireEvent.keyDown(slider, { key: "PageDown" });
    expect(pb.stepBlocks).toHaveBeenCalledWith(-5);
  });

  it("R1: arrow keys it handles do NOT propagate to a parent toolbar handler", () => {
    const pb = makePlayback();
    const parentKey = vi.fn();
    renderScrubber(pb, parentKey);
    const slider = screen.getByRole("slider");

    // ArrowRight is handled by the slider → stopPropagation, parent not reached.
    fireEvent.keyDown(slider, { key: "ArrowRight" });
    expect(pb.nextBlock).toHaveBeenCalled();
    expect(parentKey).not.toHaveBeenCalled();

    // A key the slider does NOT handle still bubbles (no over-eager stop).
    fireEvent.keyDown(slider, { key: "Tab" });
    expect(parentKey).toHaveBeenCalledTimes(1);
  });
});

describe("BlockScrubber — pointer drag snaps to nearest block", () => {
  it("a pointerdown at an arbitrary x snaps to the nearest block start", () => {
    const pb = makePlayback();
    vi.spyOn(HTMLElement.prototype, "getBoundingClientRect").mockReturnValue({
      left: 100,
      width: 400,
      top: 0,
      right: 500,
      bottom: 0,
      height: 0,
      x: 100,
      y: 0,
      toJSON: () => ({}),
    } as DOMRect);
    renderScrubber(pb);
    const slider = screen.getByRole("slider");

    // jsdom drops clientX off synthetic PointerEvents, so dispatch a real
    // MouseEvent typed "pointerdown" (carries clientX) to React's handler.
    // x=300 → ratio 0.5 over 4 stops → nearest index 2.
    fireEvent(slider, new MouseEvent("pointerdown", { clientX: 300, bubbles: true }));
    expect(pb.seekToIndex).toHaveBeenLastCalledWith(2);

    // Over-drag past the right edge clamps to the last block.
    fireEvent(slider, new MouseEvent("pointerdown", { clientX: 9999, bubbles: true }));
    expect(pb.seekToIndex).toHaveBeenLastCalledWith(3);
  });
});
