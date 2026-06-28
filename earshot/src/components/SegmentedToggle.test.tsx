import { describe, expect, it, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState } from "react";
import { SegmentedToggle, type TranscriptView } from "./SegmentedToggle";

function Harness({ initial = "spoken" as TranscriptView }) {
  const [v, setV] = useState<TranscriptView>(initial);
  return <SegmentedToggle value={v} onChange={setV} />;
}

describe("SegmentedToggle (radiogroup, roving tabindex)", () => {
  it("defaults to Spoken and exposes a single tab stop", () => {
    render(<Harness />);
    const spoken = screen.getByRole("radio", { name: "Spoken" });
    const source = screen.getByRole("radio", { name: "Source" });
    expect(spoken).toBeChecked();
    expect(source).not.toBeChecked();
    // Roving tabindex: only the checked option is tabbable.
    expect(spoken).toHaveAttribute("tabindex", "0");
    expect(source).toHaveAttribute("tabindex", "-1");
  });

  it("ArrowRight selects Source in a single step (selection follows focus)", async () => {
    const user = userEvent.setup();
    render(<Harness />);
    const spoken = screen.getByRole("radio", { name: "Spoken" });
    spoken.focus();
    await user.keyboard("{ArrowRight}");
    expect(screen.getByRole("radio", { name: "Source" })).toBeChecked();
  });

  it("calls onChange on click", async () => {
    const user = userEvent.setup();
    const onChange = vi.fn();
    render(<SegmentedToggle value="spoken" onChange={onChange} />);
    await user.click(screen.getByRole("radio", { name: "Source" }));
    expect(onChange).toHaveBeenCalledWith("source");
  });
});
