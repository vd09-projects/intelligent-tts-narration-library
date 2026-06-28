import { afterEach, describe, expect, it, vi } from "vitest";
import { act, render, renderHook, waitFor } from "@testing-library/react";
import { useNarrationSession, type NarrationSession } from "./useNarrationSession";
import { createMockFetch } from "../mocks/server";
import narrateMixed from "../fixtures/narrate.mixed.json";
import type { Block, NarrateBlockResponse } from "../api/types";

afterEach(() => {
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

const KEY = "msg-1";

// An escalate of b001 → L3 whose audio grew (3200→5000 ms), shifting every
// downstream sibling's offset. audio_url stays the SAME stable opaque URL.
const escalateB001L3: NarrateBlockResponse = {
  block: { ...(narrateMixed.blocks[0] as Block), level: 3 },
  timing: { block_id: "b001", start_ms: 0, end_ms: 5000 },
  audio_url: narrateMixed.audio_url,
  timeline: {
    plan_id: narrateMixed.timeline.plan_id,
    format: narrateMixed.timeline.format,
    blocks: [
      { block_id: "b001", start_ms: 0, end_ms: 5000 },
      { block_id: "b002", start_ms: 5000, end_ms: 7900 },
      { block_id: "b003", start_ms: 7900, end_ms: 10800 },
      { block_id: "b004", start_ms: 10800, end_ms: 14300 },
    ],
  },
};

function blockLevel(s: NarrationSession, id: string): number {
  return s.currentTranscript!.blocks.find((b) => b.id === id)!.level;
}
function blockStart(s: NarrationSession, id: string): number {
  return s.currentTranscript!.timeline.blocks.find((b) => b.block_id === id)!.start_ms;
}

async function narrateOnce(result: { current: NarrationSession }) {
  await act(async () => {
    await result.current.narrateText(KEY, "hello", 1);
  });
  await waitFor(() => expect(result.current.currentTranscript).not.toBeNull());
}

describe("useNarrationSession — setBlockLevel cache + timeline swap (#113)", () => {
  it("escalate posts once and adopts the server timeline wholesale; downstream sibling shifts", async () => {
    // review test — downstream-sibling-seek-correct (reinterpreted AC6).
    const blockCalls: number[] = [];
    vi.stubGlobal(
      "fetch",
      createMockFetch({
        narrateBlock: escalateB001L3,
        onRequest: (r) => {
          if (r.url.endsWith("/narrate/block")) blockCalls.push(1);
        },
      }),
    );
    const { result } = renderHook(() => useNarrationSession());
    await narrateOnce(result);

    // Baseline T0: b002 starts at 3200.
    expect(blockStart(result.current, "b002")).toBe(3200);

    await act(async () => {
      result.current.setBlockLevel(KEY, "b001", 3);
    });
    await waitFor(() => expect(blockLevel(result.current, "b001")).toBe(3));

    expect(blockCalls).toHaveLength(1); // exactly one POST /narrate/block
    // Only b001 changed level; siblings untouched in CONTENT but their OFFSETS
    // adopt the server timeline wholesale: b002 shifted 3200 → 5000.
    expect(blockLevel(result.current, "b002")).toBe(1);
    expect(blockStart(result.current, "b002")).toBe(5000);
    expect(blockStart(result.current, "b003")).toBe(7900);
  });

  it("returning to the original level is a ZERO-network, zero-rebill swap (cache hit)", async () => {
    // review test — return-to-initial no-model-rebill.
    const blockCalls: number[] = [];
    vi.stubGlobal(
      "fetch",
      createMockFetch({
        narrateBlock: escalateB001L3,
        onRequest: (r) => {
          if (r.url.endsWith("/narrate/block")) blockCalls.push(1);
        },
      }),
    );
    const { result } = renderHook(() => useNarrationSession());
    await narrateOnce(result);

    await act(async () => {
      result.current.setBlockLevel(KEY, "b001", 3);
    });
    await waitFor(() => expect(blockLevel(result.current, "b001")).toBe(3));
    expect(blockCalls).toHaveLength(1);

    // Return to L1 — seeded on initial load → cache hit, NO new POST.
    await act(async () => {
      result.current.setBlockLevel(KEY, "b001", 1);
    });
    await waitFor(() => expect(blockLevel(result.current, "b001")).toBe(1));
    expect(blockCalls).toHaveLength(1); // still one — no re-bill
  });

  it("cache-hit de-escalate restores the PAIRED timeline snapshot, not the post-escalate one", async () => {
    // review test — cache-hit timeline-snapshot (F1 not reintroduced offline).
    vi.stubGlobal("fetch", createMockFetch({ narrateBlock: escalateB001L3 }));
    const { result } = renderHook(() => useNarrationSession());
    await narrateOnce(result);

    await act(async () => {
      result.current.setBlockLevel(KEY, "b001", 3);
    });
    await waitFor(() => expect(blockStart(result.current, "b002")).toBe(5000)); // T1

    await act(async () => {
      result.current.setBlockLevel(KEY, "b001", 1);
    });
    // Restored to L1 → the timeline paired with L1 (T0): b002 back to 3200, NOT 5000.
    await waitFor(() => expect(blockStart(result.current, "b002")).toBe(3200));
    expect(blockStart(result.current, "b003")).toBe(6100);
  });

  it("escalation-returned refusal keeps the prior level + control; surfaces inline (F4)", async () => {
    // review test — escalate→refusal-keeps-control.
    const refusalResp: NarrateBlockResponse = {
      block: { ...(narrateMixed.blocks[0] as Block), status: "refused" },
      refusal: {
        reason: "too_large_for_level",
        message: "Can't voice this at L3.",
        spoken: true,
        source_map: { kind: "char_span" },
      },
      audio_url: narrateMixed.audio_url,
      timeline: narrateMixed.timeline as NarrateBlockResponse["timeline"],
    };
    vi.stubGlobal("fetch", createMockFetch({ narrateBlock: refusalResp }));
    const { result } = renderHook(() => useNarrationSession());
    await narrateOnce(result);

    await act(async () => {
      result.current.setBlockLevel(KEY, "b001", 3);
    });
    await waitFor(() =>
      expect(result.current.blockLeveling.get(`${KEY}::b001`)?.phase).toBe("refused-inline"),
    );
    // Block NOT flipped to refused — prior committed level + voiced status kept.
    expect(blockLevel(result.current, "b001")).toBe(1);
    expect(result.current.currentTranscript!.blocks.find((b) => b.id === "b001")!.status).toBe(
      "voiced",
    );
    expect(result.current.blockLeveling.get(`${KEY}::b001`)?.message).toMatch(/can't voice/i);
  });

  it("transport error keeps the prior level and surfaces an inline error", async () => {
    vi.stubGlobal("fetch", createMockFetch({ narrateBlock: "error" }));
    const { result } = renderHook(() => useNarrationSession());
    await narrateOnce(result);

    await act(async () => {
      result.current.setBlockLevel(KEY, "b001", 3);
    });
    await waitFor(() =>
      expect(result.current.blockLeveling.get(`${KEY}::b001`)?.phase).toBe("error"),
    );
    expect(blockLevel(result.current, "b001")).toBe(1); // unchanged
  });

  it("re-entrancy: two quick commits resolve last-selected-wins; stale resolve ignored", async () => {
    const calls: number[] = [];
    const jsonResp = (body: unknown, status = 200) =>
      new Response(JSON.stringify(body), { status, headers: { "Content-Type": "application/json" } });
    const escalateTo = (level: number): NarrateBlockResponse => ({
      block: { ...(narrateMixed.blocks[0] as Block), level: level as 1 | 2 | 3 },
      timing: { block_id: "b001", start_ms: 0, end_ms: 4000 },
      audio_url: narrateMixed.audio_url,
      timeline: narrateMixed.timeline as NarrateBlockResponse["timeline"],
    });
    vi.stubGlobal("fetch", (input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input);
      if (url.endsWith("/narrate/block")) {
        const level = JSON.parse(String(init?.body)).level as number;
        calls.push(level);
        // L2 resolves SLOWER than L3, so the superseded L2 lands last.
        const delay = level === 2 ? 40 : 0;
        return new Promise<Response>((res) => setTimeout(() => res(jsonResp(escalateTo(level))), delay));
      }
      if (url.endsWith("/narrate")) return Promise.resolve(jsonResp(narrateMixed));
      return Promise.resolve(jsonResp({ reason: "nope", message: "x" }, 404));
    });

    const { result } = renderHook(() => useNarrationSession());
    await narrateOnce(result);

    await act(async () => {
      result.current.setBlockLevel(KEY, "b001", 2); // slow
      result.current.setBlockLevel(KEY, "b001", 3); // fast, last-selected
    });
    await waitFor(() => expect(blockLevel(result.current, "b001")).toBe(3));
    // Let the stale L2 response arrive — it must be IGNORED (target is now L3).
    await new Promise((r) => setTimeout(r, 80));
    expect(calls).toEqual([2, 3]);
    expect(blockLevel(result.current, "b001")).toBe(3);
  });
});

describe("useNarrationSession — reload nonce drives useAudio.reload (#113)", () => {
  function captureSession(onSession: (s: NarrationSession) => void) {
    function Harness() {
      const s = useNarrationSession();
      onSession(s);
      return <audio ref={s.audioRef} />;
    }
    return Harness;
  }

  it("a swap on the CURRENT entry triggers exactly one audioEl.load() (no setSource double-fire)", async () => {
    vi.stubGlobal("fetch", createMockFetch({ narrateBlock: escalateB001L3 }));
    const loadSpy = vi.spyOn(window.HTMLMediaElement.prototype, "load").mockImplementation(() => {});

    let session: NarrationSession = null as unknown as NarrationSession;
    const Harness = captureSession((s) => {
      session = s;
    });
    render(<Harness />);

    await act(async () => {
      await session.narrateText(KEY, "hello", 1);
    });
    await waitFor(() => expect(session.currentTranscript).not.toBeNull());

    // After the initial source set, reset the spy and do a cache-hit swap on the
    // current entry. audio_url is unchanged, so setSource must NOT re-fire; only
    // the reload nonce calls load().
    loadSpy.mockClear();
    await act(async () => {
      session.setBlockLevel(KEY, "b001", 1); // seeded level → cache hit → reload
    });
    await waitFor(() => expect(loadSpy).toHaveBeenCalledTimes(1));
  });
});
