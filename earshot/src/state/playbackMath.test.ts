import { describe, expect, it } from "vitest";
import {
  clamp,
  computeBlockSignature,
  findActiveBlockIndex,
  formatSpokenTime,
  nearestIndexFromPointer,
  nearestIndexFromRatio,
} from "./playbackMath";

describe("findActiveBlockIndex — block-quantized, half-open [start,end)", () => {
  // Contiguous 4-block timeline: 0–3200, 3200–6100, 6100–9000, 9000–12000.
  const starts = [0, 3200, 6100, 9000];
  const ends = [3200, 6100, 9000, 12000];

  it.each([
    ["at zero → block 0", 0, 0],
    ["mid block 0", 1500, 0],
    ["exact boundary belongs to NEXT block", 3200, 1],
    ["mid block 1", 5000, 1],
    ["mid last block", 11000, 3],
    ["one ms before end of last", 11999, 3],
  ])("%s", (_label, t, expected) => {
    expect(findActiveBlockIndex(starts, ends, t)).toBe(expected);
  });

  it("returns -1 before the first block, at/after the last end, and for empty", () => {
    expect(findActiveBlockIndex(starts, ends, -1)).toBe(-1);
    expect(findActiveBlockIndex(starts, ends, 12000)).toBe(-1); // half-open end
    expect(findActiveBlockIndex(starts, ends, 99999)).toBe(-1);
    expect(findActiveBlockIndex([], [], 100)).toBe(-1);
  });

  it("respects a gap between blocks (t inside the gap → -1)", () => {
    // block 0: 0–1000, gap, block 1: 2000–3000
    expect(findActiveBlockIndex([0, 2000], [1000, 3000], 1500)).toBe(-1);
    expect(findActiveBlockIndex([0, 2000], [1000, 3000], 2500)).toBe(1);
  });
});

describe("clamp / nearestIndexFromRatio / nearestIndexFromPointer", () => {
  it("clamps", () => {
    expect(clamp(5, 0, 3)).toBe(3);
    expect(clamp(-2, 0, 3)).toBe(0);
    expect(clamp(2, 0, 3)).toBe(2);
  });

  it.each([
    [0, 4, 0],
    [0.1, 4, 0],
    [0.5, 4, 2],
    [0.9, 4, 3],
    [1, 4, 3],
    [1.5, 4, 3], // over-drag clamps to last
    [-1, 4, 0], // under-drag clamps to first
  ])("ratio %f over %i stops → %i", (ratio, count, expected) => {
    expect(nearestIndexFromRatio(ratio, count)).toBe(expected);
  });

  it("single-stop and empty timelines", () => {
    expect(nearestIndexFromRatio(0.7, 1)).toBe(0);
    expect(nearestIndexFromRatio(0.7, 0)).toBe(-1);
  });

  it("pointer X within a track rect snaps to the nearest stop", () => {
    const rect = { left: 100, width: 400 }; // 4 stops at x = 100,233,367,500
    expect(nearestIndexFromPointer(100, rect, 4)).toBe(0);
    expect(nearestIndexFromPointer(300, rect, 4)).toBe(2); // ratio 0.5
    expect(nearestIndexFromPointer(500, rect, 4)).toBe(3);
    expect(nearestIndexFromPointer(9999, rect, 4)).toBe(3); // clamp right
  });
});

describe("formatSpokenTime — block-start mm:ss only", () => {
  it.each([
    [0, "0:00"],
    [1000, "0:01"],
    [45000, "0:45"],
    [105000, "1:45"],
    [600000, "10:00"],
    [-5, "0:00"],
    [Number.NaN, "0:00"],
  ])("%f ms → %s", (ms, expected) => {
    expect(formatSpokenTime(ms)).toBe(expected);
  });
});

describe("computeBlockSignature — order-independent staleness guard", () => {
  it("is stable across reordering of the same id set", () => {
    expect(computeBlockSignature(["b001", "b002", "b003"])).toBe(
      computeBlockSignature(["b003", "b001", "b002"]),
    );
  });

  it("changes when the block set changes (add / remove / rename)", () => {
    const base = computeBlockSignature(["b001", "b002"]);
    expect(computeBlockSignature(["b001", "b002", "b003"])).not.toBe(base);
    expect(computeBlockSignature(["b001"])).not.toBe(base);
    expect(computeBlockSignature(["b001", "b099"])).not.toBe(base);
  });

  it("is an 8-char hex string", () => {
    expect(computeBlockSignature(["x", "y"])).toMatch(/^[0-9a-f]{8}$/);
    expect(computeBlockSignature([])).toMatch(/^[0-9a-f]{8}$/);
  });

  it("joins on a real, non-empty delimiter (no id-concatenation collision)", () => {
    // Two id sets that would COLLIDE if joined with an empty separator
    // ("ab"+"c" === "a"+"bc") must yield different signatures — proves a
    // genuine delimiter sits between ids. The delimiter is a visible "\n"
    // (newline), NOT the literal NUL byte that previously made this source
    // file read as binary to git.
    expect(computeBlockSignature(["ab", "c"])).not.toBe(computeBlockSignature(["a", "bc"]));
  });

  it("never emits a NUL byte in the signature", () => {
    for (const ids of [["b001", "b002"], ["x"], [], ["a", "b", "c"]]) {
      expect(computeBlockSignature(ids)).not.toContain("\u0000");
    }
  });
});
