import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import App from "./App";
import { createMockFetch, type MockOptions } from "./mocks/server";
import { expectNoSeriousA11y } from "./test/harness";

function stubFetch(opts: MockOptions = {}) {
  vi.stubGlobal("fetch", createMockFetch(opts));
}

async function loadSession(user: ReturnType<typeof userEvent.setup>, id = "demo-session-001") {
  await user.type(screen.getByLabelText("Session ID"), id);
  await user.click(screen.getByRole("button", { name: "Load" }));
}

afterEach(() => {
  vi.unstubAllGlobals();
});

describe("App shell — list-detail layout", () => {
  beforeEach(() => stubFetch());

  it("renders the three regions with correct landmarks (exactly one main)", () => {
    render(<App />);
    expect(screen.getByRole("banner")).toBeInTheDocument(); // <header>
    expect(screen.getAllByRole("main")).toHaveLength(1); // transcript
    expect(screen.getByRole("toolbar", { name: "Playback transport" })).toBeInTheDocument();
    expect(screen.getByTestId("live-region")).toBeInTheDocument(); // mounted at load
    expect(screen.getByTestId("audio-element")).toBeInTheDocument();
  });

  it("collapse toggle flips the shell state via aria-expanded (CSS-only, no trap)", async () => {
    const user = userEvent.setup();
    const { container } = render(<App />);
    const toggle = screen.getByRole("button", { name: /sessions/i });
    expect(toggle).toHaveAttribute("aria-expanded", "true");
    expect(container.querySelector(".app-shell")).toHaveClass("session-open");
    await user.click(toggle);
    expect(toggle).toHaveAttribute("aria-expanded", "false");
    expect(container.querySelector(".app-shell")).toHaveClass("session-collapsed");
  });
});

describe("Session pane — listbox + listen path", () => {
  it("idle → loading hint distinct from empty", async () => {
    stubFetch({ messages: "empty" });
    const user = userEvent.setup();
    render(<App />);
    expect(screen.getByTestId("session-idle")).toBeInTheDocument();
    await loadSession(user, "empty-session-000");
    expect(await screen.findByTestId("session-empty")).toBeInTheDocument();
    expect(screen.queryByTestId("session-idle")).not.toBeInTheDocument();
    expect(screen.getByTestId("live-region")).toHaveTextContent("No messages in this session");
  });

  it("populates the listbox and tracks the active option with arrow keys", async () => {
    stubFetch();
    const user = userEvent.setup();
    render(<App />);
    await loadSession(user);
    const listbox = await screen.findByRole("listbox", { name: "Session messages" });
    const options = within(listbox).getAllByRole("option");
    expect(options).toHaveLength(3);
    expect(listbox).toHaveAttribute("tabindex", "0"); // single tab stop
    expect(listbox).toHaveAttribute("aria-activedescendant", "session-option-0");
    listbox.focus();
    await user.keyboard("{ArrowDown}");
    expect(listbox).toHaveAttribute("aria-activedescendant", "session-option-1");
  });

  it("selecting a message narrates and renders the transcript in block order", async () => {
    stubFetch();
    const user = userEvent.setup();
    render(<App />);
    await loadSession(user);
    const options = await screen.findAllByRole("option");
    await user.click(options[1]);

    await waitFor(() => {
      const rows = document.querySelectorAll("[data-block-id]");
      expect(rows).toHaveLength(4);
    });
    const ids = Array.from(document.querySelectorAll("[data-block-id]")).map((n) =>
      n.getAttribute("data-block-id"),
    );
    expect(ids).toEqual(["b001", "b002", "b003", "b004"]);
    expect(options[1]).toHaveAttribute("aria-selected", "true");
  });

  it("Spoken/Source toggle defaults to Spoken and flips content", async () => {
    stubFetch();
    const user = userEvent.setup();
    render(<App />);
    await loadSession(user);
    const options = await screen.findAllByRole("option");
    await user.click(options[1]);
    await screen.findByRole("radiogroup", { name: "Transcript view" });

    // Scope assertions to the transcript <main> — the session row snippet also
    // contains the source phrase, so a document-wide query would false-match.
    const transcript = screen.getByRole("main");

    // Spoken default: b001 spoken text present, source-only phrase absent.
    expect(
      within(transcript).getByText(/Set the replica count to three, then apply the spec\./),
    ).toBeInTheDocument();
    expect(within(transcript).queryByText(/in the deployment spec/)).not.toBeInTheDocument();

    await user.click(screen.getByRole("radio", { name: "Source" }));
    expect(await within(transcript).findByText(/in the deployment spec/)).toBeInTheDocument();
  });

  it("renders honest status markers: degraded non-color marker + refused refusal, no level control", async () => {
    stubFetch();
    const user = userEvent.setup();
    render(<App />);
    await loadSession(user);
    const options = await screen.findAllByRole("option");
    await user.click(options[1]);
    await waitFor(() => expect(screen.getByTestId("degraded-marker")).toBeInTheDocument());

    // Refused block: refusal message + source map; no per-block level control.
    expect(screen.getByText(/too long to read verbatim/i)).toBeInTheDocument();
    const refused = document.querySelector('[data-status="refused"]')!;
    expect(refused).toBeInTheDocument();
    expect(within(refused as HTMLElement).queryByRole("button")).not.toBeInTheDocument();
  });

  it("sad path: a messages fetch reject surfaces a focused, assertive error", async () => {
    stubFetch({ reject: true });
    const user = userEvent.setup();
    render(<App />);
    await loadSession(user);
    const banner = await screen.findByTestId("error-banner");
    expect(banner).toHaveAttribute("role", "alert"); // assertive
    expect(banner).toHaveFocus(); // focus moved to actionable info
  });
});

describe("Audio-load failure path", () => {
  it("surfaces a DISTINCT 'Couldn't play audio' message on an <audio> error", async () => {
    stubFetch();
    const user = userEvent.setup();
    render(<App />);
    await loadSession(user);
    const options = await screen.findAllByRole("option");
    await user.click(options[0]);
    await waitFor(() => expect(document.querySelectorAll("[data-block-id]").length).toBeGreaterThan(0));

    fireEvent.error(screen.getByTestId("audio-element"));
    const banner = await screen.findByText(/couldn't play audio/i);
    expect(banner).toBeInTheDocument();
  });
});

describe("File pane — drop/pick → multipart", () => {
  it("picking a file sends MULTIPART /narrate/file and renders via the shared owner", async () => {
    let fileReqBody: unknown;
    vi.stubGlobal(
      "fetch",
      createMockFetch({
        onRequest: (r) => {
          if (r.url.endsWith("/narrate/file")) {
            fileReqBody = r.init?.body;
          }
        },
      }),
    );
    const user = userEvent.setup();
    render(<App />);

    const file = new File(["# Doc\nbody text"], "doc.md", { type: "text/markdown" });
    await user.upload(screen.getByTestId("file-input"), file);

    await waitFor(() => expect(fileReqBody).toBeInstanceOf(FormData));
    expect(screen.getByTestId("file-selected")).toHaveTextContent("doc.md");
    // Shared owner: the same transcript pane renders the result.
    await waitFor(() => expect(document.querySelectorAll("[data-block-id]").length).toBe(4));
  });

  it("the file picker is a real keyboard-operable control (no div onClick)", () => {
    stubFetch();
    render(<App />);
    expect(screen.getByRole("button", { name: /choose a file/i })).toBeInTheDocument();
    expect(screen.getByTestId("file-input")).toHaveAttribute("type", "file");
  });
});

describe("Refusal-first focus target", () => {
  it("a refusal-first transcript still exposes a focusable first block", async () => {
    stubFetch({ narrate: "refusalFirst" });
    const user = userEvent.setup();
    render(<App />);
    await loadSession(user);
    const options = await screen.findAllByRole("option");
    await user.click(options[0]);
    await waitFor(() => expect(document.querySelector('[data-status="refused"]')).toBeInTheDocument());
    const first = document.querySelector("[data-block-id]") as HTMLElement;
    expect(first).toHaveAttribute("tabindex", "-1"); // programmatically focusable
    expect(first.getAttribute("data-status")).toBe("refused");
  });
});

describe("Automated a11y smoke", () => {
  it("the assembled shell has no serious/critical axe violations", async () => {
    stubFetch();
    const { container } = render(<App />);
    await expectNoSeriousA11y(container);
  });

  it("the populated transcript has no serious/critical axe violations", async () => {
    stubFetch();
    const user = userEvent.setup();
    const { container } = render(<App />);
    await loadSession(user);
    const options = await screen.findAllByRole("option");
    await user.click(options[1]);
    await waitFor(() => expect(document.querySelectorAll("[data-block-id]").length).toBe(4));
    await expectNoSeriousA11y(container);
  });
});
