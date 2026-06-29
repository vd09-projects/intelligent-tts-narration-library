import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { act, renderHook } from "@testing-library/react";
import { RESTORE_GATE_TIMEOUT_MS, usePlayback, type UsePlaybackInput } from "./usePlayback";
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
  // Default to a loaded element (HAVE_METADATA). Tests simulating a cold,
  // not-yet-loaded element set readyState = 0 (HAVE_NOTHING).
  readyState = 1;
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
  // Seed a valid resume entry for the given block and return its store key.
  function seedResume(blockId: string, blockOrder: number): string {
    const key = resumeKey("entry:msg-1");
    localStorage.setItem(
      key,
      JSON.stringify({
        schemaVersion: RESUME_SCHEMA_VERSION,
        blockId,
        blockOrder,
        blockSignature: SIG,
        updatedAt: 1,
      }),
    );
    return key;
  }

  it("stays muted on a seeked that did NOT land in the restored block; clears on one that does", () => {
    seedResume("b003", 2); // range 6100–9000
    const setSpy = vi.spyOn(window.localStorage, "setItem");

    // seek is a NO-OP: simulate a not-yet-loaded element stuck at time 0.
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
    expect(result.current.activeBlockId).toBe("b003");

    // A premature 'seeked' while currentTime is still 0 must NOT clear the gate
    // (0 is not inside b003's [6100, 9000)) — the rAF loop stays muted.
    audio.currentTime = 0;
    act(() => audio.dispatchEvent(new Event("seeked")));
    frame();
    expect(result.current.activeBlockId).toBe("b003"); // gate held — not clobbered
    expect(setSpy).not.toHaveBeenCalled(); // no resume write while restoring

    // A 'seeked' that LANDS inside b003 clears the gate.
    audio.currentTime = 6.1; // 6100ms ∈ [6100, 9000)
    act(() => audio.dispatchEvent(new Event("seeked")));
    // Now the rAF loop is live again: drop the clock to 0 and it derives b001.
    audio.currentTime = 0;
    frame();
    expect(result.current.activeBlockId).toBe("b001");
  });

  it("timeout fallback RE-ASSERTS the seek and never clobbers the saved resume with block 0", () => {
    vi.useFakeTimers({ toFake: ["setTimeout", "clearTimeout"] });
    try {
      const key = seedResume("b003", 2);
      const before = localStorage.getItem(key);

      // Cold element: never loads metadata, and seek never moves currentTime
      // (a currentTime set at HAVE_NOTHING is deferred → the seek never lands).
      const audio = new FakeAudio();
      audio.readyState = 0; // HAVE_NOTHING
      const seek = vi.fn(); // no-op: simulates the deferred/aborted cold seek
      const input: UsePlaybackInput = {
        audioRef: { current: audio as unknown as HTMLAudioElement },
        timeline: TIMELINE,
        resumeScope: "msg-1",
        playbackState: "playing",
        seek,
        play: vi.fn(),
        pause: vi.fn(),
      };
      const { result } = renderHook(() => usePlayback(input));
      expect(result.current.activeBlockId).toBe("b003");
      expect(seek).toHaveBeenCalledTimes(1); // initial restore seek (did not land)

      // No 'seeked' ever fires. rAF ticks read currentTime 0 but the gate mutes.
      audio.currentTime = 0;
      frame();
      expect(result.current.activeBlockId).toBe("b003"); // gate held

      // Advance past the fallback timeout.
      act(() => vi.advanceTimersByTime(RESTORE_GATE_TIMEOUT_MS + 10));

      // Fallback RE-ASSERTED the seek to b003's start — did NOT clear the gate.
      expect(seek).toHaveBeenCalledTimes(2);
      expect(seek).toHaveBeenLastCalledWith(6100);
      frame();
      expect(result.current.activeBlockId).toBe("b003"); // STILL the restored block

      // The saved resume entry is UNCHANGED — block 0 was never persisted over it.
      expect(localStorage.getItem(key)).toBe(before);
      expect(JSON.parse(localStorage.getItem(key)!).blockId).toBe("b003");
    } finally {
      vi.useRealTimers();
    }
  });

  it("belt: writer skips block 0 while readyState < HAVE_METADATA, persists it once loaded", () => {
    // No restore (no seeded entry) → the gate is NOT set. The belt guard alone
    // must stop a "fell back to block 0" write while the element has no metadata,
    // yet allow a genuine block-0 position once metadata is present.
    const setSpy = vi.spyOn(window.localStorage, "setItem");
    const audio = new FakeAudio();
    const input: UsePlaybackInput = {
      audioRef: { current: audio as unknown as HTMLAudioElement },
      timeline: TIMELINE,
      resumeScope: "msg-1",
      playbackState: "playing",
      seek: vi.fn(),
      play: vi.fn(),
      pause: vi.fn(),
    };
    const { result } = renderHook(() => usePlayback(input));

    // Cold: HAVE_NOTHING + an un-landed currentTime 0 → derives block 0, NOT persisted.
    audio.readyState = 0;
    audio.currentTime = 0;
    frame();
    expect(result.current.activeBlockId).toBe("b001"); // derivation shows it
    expect(setSpy).not.toHaveBeenCalled(); // block-0-at-readyState-0 skipped

    // Metadata available now: a real transition to b002 persists, and a genuine
    // return to block 0 persists too (the guard only gates readyState < 1).
    audio.readyState = 1;
    audio.currentTime = 4.0; // 4000ms ∈ b002 [3200, 6100)
    frame();
    audio.currentTime = 0; // back to block 0, with metadata
    frame();
    expect(result.current.activeBlockId).toBe("b001");
    expect(setSpy).toHaveBeenCalledTimes(2);
    expect(JSON.parse(localStorage.getItem(resumeKey("entry:msg-1"))!).blockId).toBe("b001");
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

describe("usePlayback — continuous audio-progress channel (moving bar + clock)", () => {
  it("emits currentMs/durationMs/fraction every frame WITHOUT a React re-render", () => {
    const { input, audio } = makeInput();
    (audio as unknown as { duration: number }).duration = 40; // 40s clip
    let renders = 0;
    const { result } = renderHook(() => {
      renders++;
      return usePlayback(input);
    });

    const got: { currentMs: number; durationMs: number; fraction: number }[] = [];
    act(() => {
      result.current.subscribeProgress((p) => got.push(p));
    });

    // Settle on block b001, then record the render count.
    audio.currentTime = 0;
    frame();
    const rBefore = renders;

    // Sweep WITHIN block b001 (0–3.2s) so the active block never changes: any
    // render increment would have to come from the progress channel.
    audio.currentTime = 1.0;
    frame();
    audio.currentTime = 2.0;
    frame();

    const last = got.at(-1)!;
    expect(last.currentMs).toBe(2000);
    expect(last.durationMs).toBe(40000);
    expect(last.fraction).toBeCloseTo(0.05, 6); // continuous, not block-quantized
    expect(renders).toBe(rBefore); // imperative channel — zero per-frame churn
  });

  it("reports fraction 0 while the duration is unknown (NaN), and unsubscribe stops delivery", () => {
    const { input, audio } = makeInput(); // FakeAudio has no duration → NaN
    const { result } = renderHook(() => usePlayback(input));
    const got: { currentMs: number; durationMs: number; fraction: number }[] = [];
    let unsub = () => {};
    act(() => {
      unsub = result.current.subscribeProgress((p) => got.push(p));
    });

    audio.currentTime = 5.0;
    frame();
    expect(got.at(-1)).toMatchObject({ currentMs: 5000, durationMs: 0, fraction: 0 });

    act(() => unsub());
    const n = got.length;
    audio.currentTime = 8.0;
    frame();
    expect(got.length).toBe(n); // no further deliveries after unsubscribe
  });
});
