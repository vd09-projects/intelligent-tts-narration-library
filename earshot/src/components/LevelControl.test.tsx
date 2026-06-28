import { describe, expect, it, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { LevelControl } from "./LevelControl";
import { IDLE_LEVELING, type BlockLevelingState } from "../hooks/useNarrationSession";

function renderControl(over: {
  current?: 1 | 2 | 3;
  leveling?: BlockLevelingState;
  onCommit?: (lvl: 1 | 2 | 3) => void;
  disabled?: boolean;
} = {}) {
  const onCommit = over.onCommit ?? vi.fn();
  render(
    <LevelControl
      current={over.current ?? 1}
      leveling={over.leveling ?? IDLE_LEVELING}
      blockLabel="Block 1"
      onCommit={onCommit}
      disabled={over.disabled}
    />,
  );
  return { onCommit };
}

describe("LevelControl — APG radiogroup, commit-on-activate (#113)", () => {
  it("exposes one tab stop and a single aria-checked at the committed level", () => {
    renderControl({ current: 2 });
    const radios = screen.getAllByRole("radio");
    expect(radios).toHaveLength(3);
    const checked = radios.filter((r) => r.getAttribute("aria-checked") === "true");
    expect(checked).toHaveLength(1);
    expect(checked[0]).toHaveAccessibleName("Level 2");
    // Roving tabindex: exactly one tabbable cell.
    expect(radios.filter((r) => r.getAttribute("tabindex") === "0")).toHaveLength(1);
  });

  it("the committed cell carries a NON-COLOR marker (not color alone)", () => {
    renderControl({ current: 1 });
    // A marker glyph + the is-checked class accompany aria-checked/color.
    const marker = screen.getByTestId("level-marker");
    expect(marker).toBeInTheDocument();
    expect(screen.getByRole("radio", { name: "Level 1" })).toHaveClass("is-checked");
  });

  it("ARROWS move roving focus only — they do NOT commit (no escalation fired)", async () => {
    // review test — arrow-moves-focus-without-firing-escalation.
    const user = userEvent.setup();
    const { onCommit } = renderControl({ current: 1 });
    const l1 = screen.getByRole("radio", { name: "Level 1" });
    l1.focus();
    await user.keyboard("{ArrowRight}{ArrowRight}{ArrowLeft}{ArrowDown}{ArrowUp}");
    // Focus moved, but selection did NOT — onCommit never called on arrow keys.
    expect(onCommit).not.toHaveBeenCalled();
    // aria-checked is still L1 (no select-follows-focus).
    expect(l1).toHaveAttribute("aria-checked", "true");
  });

  it("Space/Enter on the FOCUSED cell commits (commit-on-activate)", async () => {
    const user = userEvent.setup();
    const { onCommit } = renderControl({ current: 1 });
    screen.getByRole("radio", { name: "Level 1" }).focus();
    await user.keyboard("{ArrowRight}{ArrowRight}"); // roving-focus to L3
    await user.keyboard(" "); // Space commits the focused cell
    expect(onCommit).toHaveBeenCalledTimes(1);
    expect(onCommit).toHaveBeenCalledWith(3);
  });

  it("click commits the clicked level", async () => {
    const user = userEvent.setup();
    const { onCommit } = renderControl({ current: 1 });
    await user.click(screen.getByRole("radio", { name: "Level 2" }));
    expect(onCommit).toHaveBeenCalledWith(2);
  });

  it("loading: role=status announces progress and holds the TARGET as aria-checked", () => {
    renderControl({ current: 1, leveling: { phase: "loading", message: null, target: 3 } });
    // The in-flight target is shown checked across the async window.
    expect(screen.getByRole("radio", { name: "Level 3" })).toHaveAttribute("aria-checked", "true");
    expect(screen.getByRole("status")).toHaveTextContent(/re-narrating block at l3/i);
    // Cells disabled while busy.
    expect(screen.getByRole("radio", { name: "Level 1" })).toBeDisabled();
  });

  it("refusal-result: role=alert carries the message; aria-checked snaps back to committed", () => {
    renderControl({
      current: 1,
      leveling: { phase: "refused-inline", message: "Block can't be voiced at L3; still at L1", target: null },
    });
    expect(screen.getByRole("alert")).toHaveTextContent(/can't be voiced at l3/i);
    // Snapped back: committed L1 is checked, not the failed target.
    expect(screen.getByRole("radio", { name: "Level 1" })).toHaveAttribute("aria-checked", "true");
  });
});
