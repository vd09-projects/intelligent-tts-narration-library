import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { act, renderHook } from "@testing-library/react";
import { usePlayback, type UsePlaybackInput } from "./usePlayback";
import type { Timeline } from "../api/types";
import { computeBlockSignature } from "../state/playbackMath";
import { RESUME_SCHEMA_VERSION, resumeKey } from "../state/resumeStore";

// 4 contiguous blocks: b001 0–3200, b002 3200–6100, b003 6100–9000, b004 9000–12000.
const TIMELINE: Timeline = {
  plan_id: "p1",
  format: { sample_rate: 24000, channels: 1, encoding: "pcm_s16le" },
  blocks: [
    { block_id: "b001", start_ms: 0, end_ms: 3200 },
    { block_id: "b002", start_ms: 3200, end_ms: 6100 },
    { block_id: "b003", start_ms: 6100, end_ms: 9000 },
    { block_id: "b004", start_ms: 9000, end_ms: 12000 },
  ],
};
const SIG = computeBlockSignature(["b001", "b002", "b003", "b004"]);

class FakeAudio extends EventTarget {
  currentTime = 0;
}

let rafCb: FrameRequestCallback | null = null;
function frame() {
  act(() => {
    rafCb?.(0);
  });
}

beforeEach(() => {
  rafCb = null;
  vi.stubGlobal("requestAnimationFrame", (cb: FrameRequestCallback) => {
    rafCb = cb;
    return 1;
  });
  vi.stubGlobal("cancelAnimationFrame", () => {});
  localStorage.clear();
});
afterEach(() => {
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
  localStorage.clear();
});

function makeInput(over: Partial<UsePlaybackInput> = {}): {
  input: UsePlaybackInput;
  audio: FakeAudio;
  seek: ReturnType<typeof vi.fn>;
} {
  const audio = new FakeAudio();
  const seek = vi.fn((ms: number) => {
    audio.currentTime = Math.max(0, ms) / 1000;
  });
  const input: UsePlaybackInput = {
    audioRef: { current: audio as unknown as HTMLAudioElement },
    timeline: TIMELINE,
    resumeScope: null,
    playbackState: "playing",
    seek,
    play: vi.fn(),
    pause: vi.fn(),
    ...over,
  };
  return { input, audio, seek };
}

describe("usePlayback — rAF active-block derivation (Decision 2026-06-21)", () => {
  it("writes the active block ONLY on a block-id change, never per frame", () => {
    const { input, audio } = makeInput();
    let renders = 0;
    const { result } = renderHook(() => {
      renders++;
      return usePlayback(input);
    });

    audio.currentTime = 0;
    frame();
    expect(result.current.activeBlockId).toBe("b001");
    const rAfter1 = renders;

    // Sweep within block 0 — no id change, so no re-render from the loop.
    audio.currentTime = 1.0;
    frame();
    audio.currentTime = 3.199;
    frame();
    expect(result.current.activeBlockId).toBe("b001");
    expect(renders).toBe(rAfter1); // no per-frame writes

    // Cross into block 1 (boundary belongs to next block).
    audio.currentTime = 3.2;
    frame();
    expect(result.current.activeBlockId).toBe("b002");
    expect(renders).toBe(rAfter1 + 1); // exactly one write on the crossing
  });

  it("seekToBlock snaps to the block start; prev/next step exactly one block", () => {
    const { input, seek } = makeInput();
    const { result } = renderHook(() => usePlayback(input));

    act(() => result.current.seekToBlock("b003"));
    expect(seek).toHaveBeenLastCalledWith(6100);

    // Move the active pointer to b003, then step.
    frame();
    expect(result.current.activeBlockId).toBe("b003");

    act(() => result.current.prevBlock());
    expect(seek).toHaveBeenLastCalledWith(3200); // one block back
    frame();
    act(() => result.current.nextBlock());
    expect(seek).toHaveBeenLastCalledWith(6100); // one block forward from b002
  });

  it("clamps prev at the first block and next at the last", () => {
    const { input, seek } = makeInput();
    const { result } = renderHook(() => usePlayback(input));
    frame(); // active b001
    act(() => result.current.prevBlock());
    expect(seek).toHaveBeenLastCalledWith(0); // clamp at first

    act(() => result.current.seekToIndex(99));
    expect(seek).toHaveBeenLastCalledWith(9000); // clamp at last (b004)
  });

  it("resets tracking when the block-id signature changes (Decision 2026-06-22)", () => {
    const { input, audio } = makeInput();
    const { result, rerender } = renderHook((props: UsePlaybackInput) => usePlayback(props), {
      initialProps: input,
    });
    audio.currentTime = 5.0;
    frame();
    expect(result.current.activeBlockId).toBe("b002");

    // New document with different ids → tracking resets, no stale active block.
    const newTimeline: Timeline = {
      ...TIMELINE,
      blocks: [
        { block_id: "x1", start_ms: 0, end_ms: 1000 },
        { block_id: "x2", start_ms: 1000, end_ms: 2000 },
      ],
    };
    rerender({ ...input, timeline: newTimeline });
    expect(result.current.activeBlockId).toBeNull();
    expect(result.current.blockCount).toBe(2);
  });
});

describe("usePlayback — restoring gate (R2)", () => {
  it("mutes active-block AND resume writes at currentTime 0 while restoring, clears on seeked", () => {
    // Seed a valid resume entry for b003.
    const key = resumeKey("entry:msg-1");
    localStorage.setItem(
      key,
      JSON.stringify({
        schemaVersion: RESUME_SCHEMA_VERSION,
        blockId: "b003",
        blockOrder: 2,
        blockSignature: SIG,
        updatedAt: 1,
      }),
    );
    const setSpy = vi.spyOn(window.localStorage, "setItem");

    // seek is a NO-OP here: simulate a not-yet-loaded element stuck at time 0.
    const audio = new FakeAudio();
    const input: UsePlaybackInput = {
      audioRef: { current: audio as unknown as HTMLAudioElement },
      timeline: TIMELINE,
      resumeScope: "msg-1",
      playbackState: "paused",
      seek: vi.fn(), // does not move currentTime
      play: vi.fn(),
      pause: vi.fn(),
    };
    const { result } = renderHook(() => usePlayback(input));

    // Restored block is seeded via the initialize path.
    expect(result.current.activeBlockId).toBe("b003");

    // Clock pinned at 0 → would derive b001, but the gate mutes the write.
    audio.currentTime = 0;
    frame();
    expect(result.current.activeBlockId).toBe("b003"); // NOT clobbered to b001
    expect(setSpy).not.toHaveBeenCalled(); // NO resume write while restoring

    // First seeked clears the gate.
    act(() => {
      audio.dispatchEvent(new Event("seeked"));
    });
    // Now the rAF loop is live again: at time 0 it derives b001.
    frame();
    expect(result.current.activeBlockId).toBe("b001");
  });
});

describe("usePlayback — initialize-not-transition restore (R3)", () => {
  it("a pure restore persists NOTHING; a later transition to a DIFFERENT block does persist", () => {
    const key = resumeKey("entry:msg-1");
    localStorage.setItem(
      key,
      JSON.stringify({
        schemaVersion: RESUME_SCHEMA_VERSION,
        blockId: "b002",
        blockOrder: 1,
        blockSignature: SIG,
        updatedAt: 1,
      }),
    );
    const setSpy = vi.spyOn(window.localStorage, "setItem");

    const { input, audio } = makeInput({ resumeScope: "msg-1" });
    const { result } = renderHook(() => usePlayback(input));

    // Restore seeded b002 and seek-ed to its start (3200ms); writes nothing.
    expect(result.current.activeBlockId).toBe("b002");
    expect(audio.currentTime).toBeCloseTo(3.2);

    // Land the seek, then re-derive at the same block — still silent.
    act(() => {
      audio.dispatchEvent(new Event("seeked"));
    });
    frame();
    expect(result.current.activeBlockId).toBe("b002");
    expect(setSpy).not.toHaveBeenCalled(); // pure restore persists NOTHING

    // Genuine transition to a different block → persists b003.
    audio.currentTime = 7.0;
    frame();
    expect(result.current.activeBlockId).toBe("b003");
    expect(setSpy).toHaveBeenCalledTimes(1);
    const stored = JSON.parse(localStorage.getItem(key)!);
    expect(stored.blockId).toBe("b003");
    expect(stored).not.toHaveProperty("startMs"); // block-level only
  });
});
