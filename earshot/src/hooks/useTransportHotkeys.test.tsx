import { afterEach, describe, expect, it, vi } from "vitest";
import { act, fireEvent, render } from "@testing-library/react";
import {
  transportHotkeyAction,
  useTransportHotkeys,
  type TransportHotkeyAction,
} from "./useTransportHotkeys";
import { PlaybackProvider } from "../state/PlaybackContext";
import { TransportDeck } from "../components/TransportDeck";
import type { PlaybackControls } from "./usePlayback";

// useNarration drives hasAudio (via activeAudioUrl) + supplies audioRef to the
// real TransportDeck used in the no-clash test. Mutated per test.
const narration = vi.hoisted(() => ({ url: "x.wav" as string | null }));
vi.mock("../state/NarrationContext", () => ({
  useNarration: () => ({ audioRef: { current: null }, activeAudioUrl: narration.url }),
}));

// Capture polite announcements without the live-region machinery.
const announceSpy = vi.hoisted(() => vi.fn());
vi.mock("../state/Announcer", () => ({
  useAnnouncer: () => ({ announce: announceSpy }),
}));

function makeControls(over: Partial<PlaybackControls> = {}): PlaybackControls {
  return {
    activeBlockId: "b001",
    activeBlockIndex: 0,
    blockCount: 4,
    blockStartsMs: [0, 3200, 6100, 9000],
    subscribeProgress: vi.fn(() => () => {}),
    playbackState: "paused",
    isPlaying: false,
    play: vi.fn(),
    pause: vi.fn(),
    toggle: vi.fn(),
    seekToBlock: vi.fn(),
    seekToIndex: vi.fn(),
    playFromBlock: vi.fn(),
    prevBlock: vi.fn(),
    nextBlock: vi.fn(),
    stepBlocks: vi.fn(),
    // Default: the seek moved the playhead (returns true) so the polite
    // announcement fires. The boundary/no-op case overrides this with `() => false`.
    skipSeconds: vi.fn(() => true),
    returnToPlayingBlock: vi.fn(),
    ...over,
  };
}

function Probe() {
  useTransportHotkeys();
  return null;
}

/** Dispatch a real keydown so defaultPrevented reflects the hook's preventDefault. */
function press(key: string): KeyboardEvent {
  const ev = new KeyboardEvent("keydown", { key, bubbles: true, cancelable: true });
  act(() => {
    document.dispatchEvent(ev);
  });
  return ev;
}

afterEach(() => {
  narration.url = "x.wav";
  document.body.innerHTML = "";
  vi.clearAllMocks();
});

// ---------------------------------------------------------------------------
// Pure focus guard (load-bearing) — the SOLE real clash guarantee.
// ---------------------------------------------------------------------------
describe("transportHotkeyAction — allow-list focus guard (table-driven)", () => {
  const REGION = "data-transport-hotkeys";

  /** Build a focus target; nest it under role wrappers and optionally the region. */
  function target(inner: string, { inRegion }: { inRegion: boolean }): Element {
    const host = document.createElement("div");
    if (inRegion) host.setAttribute(REGION, "");
    host.innerHTML = inner;
    document.body.appendChild(host);
    return host.querySelector("[data-target]") as Element;
  }

  type Row = {
    name: string;
    el: () => Element | null;
    space: TransportHotkeyAction | null;
    left: TransportHotkeyAction | null;
    right: TransportHotkeyAction | null;
  };

  const rows: Row[] = [
    { name: "nothing focused (null)", el: () => null, space: "toggle", left: "back", right: "forward" },
    {
      name: "document.body",
      el: () => document.body,
      space: "toggle",
      left: "back",
      right: "forward",
    },
    {
      name: "plain element inside the opted-in region",
      el: () => target('<div data-target tabindex="-1"></div>', { inRegion: true }),
      space: "toggle",
      left: "back",
      right: "forward",
    },
    {
      // A button owns Space (activation) but not the arrows — seeking past a
      // focused button is harmless and desirable.
      name: "<button> inside the region (Space bails, arrows act)",
      el: () => target("<button data-target>Go</button>", { inRegion: true }),
      space: null,
      left: "back",
      right: "forward",
    },
    {
      name: "<a href> inside the region (Space bails, arrows act)",
      el: () => target('<a href="#x" data-target>link</a>', { inRegion: true }),
      space: null,
      left: "back",
      right: "forward",
    },
    {
      name: "<input> inside the region (all bail — text entry)",
      el: () => target("<input data-target />", { inRegion: true }),
      space: null,
      left: null,
      right: null,
    },
    {
      name: "<select> inside the region (all bail)",
      el: () => target("<select data-target></select>", { inRegion: true }),
      space: null,
      left: null,
      right: null,
    },
    {
      // #113 radiogroup lives INSIDE the transcript region — proves the
      // defense-in-depth bail works even when the allow-list lets focus through.
      name: "#113 [role=radio] inside [role=radiogroup] inside the region",
      el: () =>
        target(
          '<div role="radiogroup"><span role="radio" tabindex="0" data-target></span></div>',
          { inRegion: true },
        ),
      space: null,
      left: null,
      right: null,
    },
    {
      // #112 toolbar is a separate tab stop OUTSIDE the transcript — the
      // allow-list alone bails it.
      name: "#112 toolbar button OUTSIDE the region (allow-list bail)",
      el: () =>
        target('<div role="toolbar"><button data-target>p</button></div>', { inRegion: false }),
      space: null,
      left: null,
      right: null,
    },
    {
      name: "#112 [role=slider] OUTSIDE the region (allow-list bail)",
      el: () => target('<div role="slider" tabindex="0" data-target></div>', { inRegion: false }),
      space: null,
      left: null,
      right: null,
    },
    {
      // #111 session pane lives in the aside, outside the region.
      name: "#111 [role=option] inside [role=listbox] OUTSIDE the region",
      el: () =>
        target('<div role="listbox"><div role="option" tabindex="0" data-target></div></div>', {
          inRegion: false,
        }),
      space: null,
      left: null,
      right: null,
    },
    {
      name: '[contenteditable="true"] inside the region (all bail)',
      el: () => target('<div contenteditable="true" data-target></div>', { inRegion: true }),
      space: null,
      left: null,
      right: null,
    },
    {
      // contenteditable="false" is NOT editable — treated as a plain element.
      name: '[contenteditable="false"] inside the region (acts — not editable)',
      el: () => target('<div contenteditable="false" tabindex="-1" data-target></div>', {
        inRegion: true,
      }),
      space: "toggle",
      left: "back",
      right: "forward",
    },
  ];

  it.each(rows)("$name", ({ el, space, left, right }) => {
    const node = el();
    expect(transportHotkeyAction(" ", node)).toBe(space);
    expect(transportHotkeyAction("ArrowLeft", node)).toBe(left);
    expect(transportHotkeyAction("ArrowRight", node)).toBe(right);
  });

  it("ignores keys it does not own", () => {
    expect(transportHotkeyAction("k", document.body)).toBeNull();
    expect(transportHotkeyAction("Enter", document.body)).toBeNull();
    expect(transportHotkeyAction("ArrowUp", document.body)).toBeNull();
  });
});

// ---------------------------------------------------------------------------
// Hook dispatch + REF pattern + announcement.
// ---------------------------------------------------------------------------
describe("useTransportHotkeys — dispatch from body/transcript focus", () => {
  function mount(controls: PlaybackControls) {
    return render(
      <PlaybackProvider value={controls}>
        <Probe />
      </PlaybackProvider>,
    );
  }

  it("Space toggles, ←/→ skip ∓step, and preventDefault fires only on handled keys", () => {
    const controls = makeControls();
    mount(controls);

    const space = press(" ");
    expect(controls.toggle).toHaveBeenCalledTimes(1);
    expect(space.defaultPrevented).toBe(true);

    const left = press("ArrowLeft");
    expect(controls.skipSeconds).toHaveBeenLastCalledWith(-10);
    expect(left.defaultPrevented).toBe(true);

    const right = press("ArrowRight");
    expect(controls.skipSeconds).toHaveBeenLastCalledWith(10);
    expect(right.defaultPrevented).toBe(true);

    // An unowned key is neither handled nor prevented.
    const other = press("k");
    expect(other.defaultPrevented).toBe(false);
    expect(controls.toggle).toHaveBeenCalledTimes(1);
  });

  it("posts exactly one polite announcement per handled seek, naming direction + step", () => {
    mount(makeControls());
    press("ArrowRight");
    expect(announceSpy).toHaveBeenCalledTimes(1);
    expect(announceSpy).toHaveBeenLastCalledWith("Forward 10 seconds");
    press("ArrowLeft");
    expect(announceSpy).toHaveBeenCalledTimes(2);
    expect(announceSpy).toHaveBeenLastCalledWith("Back 10 seconds");
  });

  it("Space (play/pause) does not post a seek announcement", () => {
    mount(makeControls());
    press(" ");
    expect(announceSpy).not.toHaveBeenCalled();
  });

  it("does NOT announce when the seek is a boundary/non-finite no-op, but still preventDefaults the owned key", () => {
    // skipSeconds reports the playhead did not move (clamped at a boundary or
    // pre-loadedmetadata). The announcement must stay silent — but the key is
    // still owned by the transport in-region, so preventDefault still fires.
    const controls = makeControls({ skipSeconds: vi.fn(() => false) });
    mount(controls);

    const right = press("ArrowRight");
    expect(controls.skipSeconds).toHaveBeenLastCalledWith(10);
    expect(announceSpy).not.toHaveBeenCalled(); // no phantom "Forward 10 seconds"
    expect(right.defaultPrevented).toBe(true); // owned key — still claimed

    const left = press("ArrowLeft");
    expect(controls.skipSeconds).toHaveBeenLastCalledWith(-10);
    expect(announceSpy).not.toHaveBeenCalled();
    expect(left.defaultPrevented).toBe(true);
  });

  it("is silent on every key when no audio is loaded, and does not preventDefault", () => {
    narration.url = null;
    const controls = makeControls();
    mount(controls);
    const space = press(" ");
    const left = press("ArrowLeft");
    expect(controls.toggle).not.toHaveBeenCalled();
    expect(controls.skipSeconds).not.toHaveBeenCalled();
    expect(announceSpy).not.toHaveBeenCalled();
    expect(space.defaultPrevented).toBe(false);
    expect(left.defaultPrevented).toBe(false);
  });

  it("registers exactly one document keydown listener for its lifetime", () => {
    const addSpy = vi.spyOn(document, "addEventListener");
    const { rerender } = mount(makeControls());
    rerender(
      <PlaybackProvider value={makeControls()}>
        <Probe />
      </PlaybackProvider>,
    );
    const keydownAdds = addSpy.mock.calls.filter(([type]) => type === "keydown");
    expect(keydownAdds).toHaveLength(1); // re-render does NOT re-bind
    addSpy.mockRestore();
  });

  it("removes the listener on unmount", () => {
    const controls = makeControls();
    const { unmount } = mount(controls);
    unmount();
    press(" ");
    expect(controls.toggle).not.toHaveBeenCalled();
  });

  it("stale-closure regression: mount with no audio, load audio, Space → toggle fires", () => {
    narration.url = null;
    const controls = makeControls();
    const tree = () => (
      <PlaybackProvider value={controls}>
        <Probe />
      </PlaybackProvider>
    );
    const { rerender } = render(tree());
    press(" ");
    expect(controls.toggle).not.toHaveBeenCalled(); // no audio yet

    // Audio arrives; a re-render refreshes the listener's latestRef snapshot.
    narration.url = "x.wav";
    rerender(tree()); // fresh element so React actually re-renders
    press(" ");
    expect(controls.toggle).toHaveBeenCalledTimes(1);
  });
});

// ---------------------------------------------------------------------------
// No-clash: the real #112 toolbar handles its own ←/→ while the hook stands down
// on the same keypress (representative; the pure table covers slider/listbox/
// radiogroup contexts).
// ---------------------------------------------------------------------------
describe("useTransportHotkeys — no clash with the #112 toolbar", () => {
  it("toolbar ArrowRight roves the buttons and the hook does NOT seek", () => {
    const controls = makeControls();
    const { getByRole } = render(
      <PlaybackProvider value={controls}>
        <Probe />
        <TransportDeck />
      </PlaybackProvider>,
    );
    const prev = getByRole("button", { name: "Previous block" });
    act(() => prev.focus());

    // Fire on the focused toolbar button so React's roving handler runs, and the
    // event bubbles to the document where the global hook reads activeElement.
    fireEvent.keyDown(prev, { key: "ArrowRight" });

    expect(getByRole("button", { name: "Play" })).toHaveFocus(); // roving moved
    expect(controls.skipSeconds).not.toHaveBeenCalled(); // hook stood down
  });

  it("an OWNED key (ArrowRight) on a bailing element leaves defaultPrevented === false", () => {
    // Closes the bailed-but-owned half of the carried guard suggestion at the
    // integration level: ArrowRight IS a transport key, but focus is on a
    // [role=slider] (an ARROW_BAIL context) inside the opted-in region. The hook
    // must NOT preventDefault and must NOT seek — proven on the real dispatched
    // event, not just the pure function returning null.
    const controls = makeControls();
    const host = document.createElement("div");
    host.setAttribute("data-transport-hotkeys", "");
    host.innerHTML = '<div role="slider" tabindex="0" data-target></div>';
    document.body.appendChild(host);
    render(
      <PlaybackProvider value={controls}>
        <Probe />
      </PlaybackProvider>,
    );
    const slider = host.querySelector<HTMLElement>("[data-target]")!;
    act(() => slider.focus());
    expect(document.activeElement).toBe(slider);

    const right = press("ArrowRight");
    expect(right.defaultPrevented).toBe(false); // hook stood down — did not claim the key
    expect(controls.skipSeconds).not.toHaveBeenCalled();
  });
});
