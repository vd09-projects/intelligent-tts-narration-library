import { describe, expect, it } from "vitest";
import { deriveActiveBlockId } from "./activeBlock";
import type { Timeline } from "../api/types";

const timeline: Timeline = {
  plan_id: "p1",
  format: { sample_rate: 24000, channels: 1, encoding: "wav" },
  blocks: [
    { block_id: "b001", start_ms: 0, end_ms: 1000 },
    { block_id: "b002", start_ms: 1000, end_ms: 2000 },
  ],
};

describe("deriveActiveBlockId", () => {
  it("returns the block under the playhead (half-open interval)", () => {
    expect(deriveActiveBlockId(timeline, 0)).toBe("b001");
    expect(deriveActiveBlockId(timeline, 999)).toBe("b001");
    expect(deriveActiveBlockId(timeline, 1000)).toBe("b002");
  });

  it("returns null past the end or with no timeline", () => {
    expect(deriveActiveBlockId(timeline, 2000)).toBeNull();
    expect(deriveActiveBlockId(undefined, 0)).toBeNull();
    expect(deriveActiveBlockId({ ...timeline, blocks: [] }, 0)).toBeNull();
  });
});
