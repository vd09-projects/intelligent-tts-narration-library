import { describe, expect, it, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import { BlockRow } from "./BlockRow";
import type { Block } from "../api/types";

function makeBlock(over: Partial<Block>): Block {
  return {
    id: "b1",
    order: 0,
    class: "prose",
    level: 1,
    status: "voiced",
    source_map: { kind: "char_span", raw_excerpt: "raw source text" },
    segments: [{ id: "s0", kind: "speech", text: "spoken words" }],
    provenance: { voiced_by: "intelligence", deterministic: false },
    ...over,
  };
}

describe("BlockRow status rendering (honesty invariant)", () => {
  it("voiced renders the spoken text with no degraded/refused marker", () => {
    render(<BlockRow block={makeBlock({})} view="spoken" isActive={false} onPlay={() => {}} />);
    expect(screen.getByText("spoken words")).toBeInTheDocument();
    expect(screen.queryByTestId("degraded-marker")).not.toBeInTheDocument();
  });

  it("source view shows the raw source excerpt instead of spoken text", () => {
    render(<BlockRow block={makeBlock({})} view="source" isActive={false} onPlay={() => {}} />);
    expect(screen.getByText("raw source text")).toBeInTheDocument();
  });

  it("degraded renders a NON-COLOR marker distinct from voiced (D5)", () => {
    render(
      <BlockRow
        block={makeBlock({ status: "degraded", provenance: { voiced_by: "verbatim", deterministic: true } })}
        view="spoken"
        isActive={false}
        onPlay={() => {}}
      />,
    );
    const marker = screen.getByTestId("degraded-marker");
    expect(marker).toBeInTheDocument();
    expect(marker).toHaveTextContent(/degraded/i); // text label, not color alone
  });

  it("refused renders refusal message + source map and NO level/play control", () => {
    const refused = makeBlock({
      status: "refused",
      segments: undefined,
      refusal: {
        reason: "no_intelligence_available",
        message: "Too long to read verbatim, no summary engine.",
        spoken: true,
        source_map: { kind: "line_range", start_line: 4, end_line: 40, raw_excerpt: "..." },
      },
    });
    render(<BlockRow block={refused} view="spoken" isActive={false} onPlay={() => {}} />);
    expect(screen.getByText(/Too long to read verbatim/)).toBeInTheDocument();
    expect(screen.getByText(/lines 4–40/)).toBeInTheDocument();
    // No play affordance and no level control on a refused block.
    expect(screen.queryByRole("button")).not.toBeInTheDocument();
  });

  it("active block carries aria-current and a play affordance fires onPlay", async () => {
    const onPlay = vi.fn();
    render(<BlockRow block={makeBlock({})} view="spoken" isActive onPlay={onPlay} />);
    const article = screen.getByText("spoken words").closest("article");
    expect(article).toHaveAttribute("aria-current", "true");
    screen.getByRole("button", { name: /play from block 1/i }).click();
    expect(onPlay).toHaveBeenCalledWith("b1");
  });
});

describe("BlockRow — LevelControl mounting (#113)", () => {
  it("mounts LevelControl on voiced and on degraded (degraded marker coexists)", () => {
    const { rerender } = render(
      <BlockRow block={makeBlock({})} view="spoken" isActive={false} onPlay={() => {}} onCommitLevel={() => {}} />,
    );
    expect(screen.getByRole("radiogroup", { name: /narration level/i })).toBeInTheDocument();

    rerender(
      <BlockRow
        block={makeBlock({ status: "degraded", level: 1 })}
        view="spoken"
        isActive={false}
        onPlay={() => {}}
        onCommitLevel={() => {}}
      />,
    );
    expect(screen.getByRole("radiogroup", { name: /narration level/i })).toBeInTheDocument();
    expect(screen.getByTestId("degraded-marker")).toBeInTheDocument();
  });

  it("does NOT mount LevelControl on an ORIGINALLY-refused block", () => {
    const refused = makeBlock({
      status: "refused",
      segments: undefined,
      refusal: {
        reason: "no_intelligence_available",
        message: "Too long.",
        spoken: true,
        source_map: { kind: "line_range", start_line: 1, end_line: 9, raw_excerpt: "..." },
      },
    });
    render(<BlockRow block={refused} view="spoken" isActive={false} onPlay={() => {}} onCommitLevel={() => {}} />);
    expect(screen.queryByRole("radiogroup")).not.toBeInTheDocument();
  });

  it("an escalation-returned refusal keeps the control + shows a role=alert (block not blanked)", () => {
    render(
      <BlockRow
        block={makeBlock({})} // still voiced — the hook did NOT flip it to refused
        view="spoken"
        isActive={false}
        onPlay={() => {}}
        onCommitLevel={() => {}}
        leveling={{ phase: "refused-inline", message: "Block can't be voiced at L3; still at L1", target: null }}
      />,
    );
    // Control still mounted, block text still present (not blanked).
    expect(screen.getByRole("radiogroup", { name: /narration level/i })).toBeInTheDocument();
    expect(screen.getByText("spoken words")).toBeInTheDocument();
    // Refusal surfaced inline assertively.
    expect(screen.getByRole("alert")).toHaveTextContent(/can't be voiced at l3/i);
  });

  it("renders the control DISABLED when no onCommitLevel (older server, no render_id)", () => {
    render(<BlockRow block={makeBlock({})} view="spoken" isActive={false} onPlay={() => {}} />);
    const radios = screen.getAllByRole("radio");
    expect(radios.length).toBe(3);
    radios.forEach((r) => expect(r).toBeDisabled());
  });
});
