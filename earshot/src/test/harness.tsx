// harness.tsx — shared test helpers: provider wrapper + an axe a11y assertion.

import { render, type RenderResult } from "@testing-library/react";
import axe from "axe-core";
import { expect } from "vitest";
import { AnnouncerProvider } from "../state/Announcer";
import { NarrationProvider } from "../state/NarrationContext";

/** Render a component inside both shell providers (the single owner + announcer). */
export function renderWithProviders(ui: React.ReactNode): RenderResult {
  return render(
    <AnnouncerProvider>
      <NarrationProvider>{ui}</NarrationProvider>
    </AnnouncerProvider>,
  );
}

/** Assert no serious/critical axe violations on a container (plan A11y smoke). */
export async function expectNoSeriousA11y(container: HTMLElement): Promise<void> {
  const results = await axe.run(container, {
    resultTypes: ["violations"],
    // color-contrast cannot be computed under jsdom (no canvas getContext); the
    // real theme's contrast is verified by eye at /verify (plan Color & Contrast).
    rules: { "color-contrast": { enabled: false } },
  });
  const serious = results.violations.filter(
    (v) => v.impact === "critical" || v.impact === "serious",
  );
  const summary = serious.map((v) => `${v.id}: ${v.help}`).join("\n");
  expect(serious, summary).toHaveLength(0);
}
