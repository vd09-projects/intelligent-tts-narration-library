import { describe, expect, it } from "vitest";
import { sourceText, spokenText } from "./blockText";
import type { Block } from "../api/types";

const block: Block = {
  id: "b1",
  order: 0,
  class: "code",
  level: 1,
  status: "voiced",
  source_map: { kind: "char_span", raw_excerpt: "replicas: 3" },
  segments: [
    { id: "s0", kind: "speech", text: "replicas set to three" },
    { id: "s1", kind: "pause", pause_ms: 200 },
  ],
  provenance: { voiced_by: "planner", deterministic: true },
};

describe("blockText", () => {
  it("spokenText joins only speech segments (final spoken words)", () => {
    expect(spokenText(block)).toBe("replicas set to three");
  });

  it("sourceText returns the raw source excerpt", () => {
    expect(sourceText(block)).toBe("replicas: 3");
  });

  it("spoken and source can differ (gist mode honesty surface)", () => {
    expect(spokenText(block)).not.toBe(sourceText(block));
  });
});
