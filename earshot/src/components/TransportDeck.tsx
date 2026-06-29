// TransportDeck.tsx — bottom-anchored playback transport (#112, step 4).
// Replaces the placeholder TransportBar.
//
// Accessibility model (R1): TWO tab stops.
//   (a) a role="toolbar" button GROUP — prev / play-pause / next / return —
//       managed as a SINGLE roving-tabindex tab stop (ArrowLeft/Right move
//       roving focus, Home/End jump to ends; APG Toolbar pattern).
//   (b) the BlockScrubber (role="slider"), a SEPARATE adjacent tab stop that
//       owns its own arrow keys.
//
// play/pause uses the media ACTION-LABEL model: the visible + accessible label
// IS the action ("Play" when paused, "Pause" when playing), NO aria-pressed
// (ADR #108). The single shared <audio> is visually hidden (no controls) — the
// deck drives it. All controls READ the one shared usePlayback instance from
// context (R4); the deck never calls usePlayback() locally.

import { useEffect, useRef, useState } from "react";
import { useNarration } from "../state/NarrationContext";
import { usePlaybackControls } from "../state/PlaybackContext";
import { formatSpokenTime } from "../state/playbackMath";
import { SEEK_STEP_SECONDS } from "../hooks/usePlayback";
import { BlockScrubber } from "./BlockScrubber";

// Elapsed / total time readout. Driven imperatively off the per-frame audio
// clock (no React state → no per-frame re-render). aria-hidden: the slider's
// aria-valuetext is the single spoken source for position (avoids double
// announcement); this readout is the visual clock. Duration shows "--:--"
// until the element reports it.
function TransportClock() {
  const { subscribeProgress } = usePlaybackControls();
  const curRef = useRef<HTMLSpanElement>(null);
  const durRef = useRef<HTMLSpanElement>(null);
  useEffect(
    () =>
      subscribeProgress((p) => {
        if (curRef.current) curRef.current.textContent = formatSpokenTime(p.currentMs);
        if (durRef.current) {
          durRef.current.textContent = p.durationMs > 0 ? formatSpokenTime(p.durationMs) : "--:--";
        }
      }),
    [subscribeProgress],
  );
  return (
    <span className="transport-clock" aria-hidden="true" data-testid="transport-clock">
      <span ref={curRef} className="transport-clock__cur">
        0:00
      </span>
      <span className="transport-clock__sep"> / </span>
      <span ref={durRef} className="transport-clock__dur">
        --:--
      </span>
    </span>
  );
}

export function TransportDeck() {
  const { audioRef, activeAudioUrl } = useNarration();
  const { activeBlockId, isPlaying, blockCount, toggle, prevBlock, nextBlock, returnToPlayingBlock } =
    usePlaybackControls();

  // Roving tabindex across the 4 toolbar buttons (single tab stop).
  const [roving, setRoving] = useState(0);
  const btnRefs = useRef<(HTMLButtonElement | null)[]>([]);
  const hasAudio = Boolean(activeAudioUrl);
  const disabled = !hasAudio || blockCount === 0;

  function focusButton(i: number) {
    setRoving(i);
    btnRefs.current[i]?.focus();
  }

  function onToolbarKeyDown(e: React.KeyboardEvent) {
    const last = btnRefs.current.length - 1;
    switch (e.key) {
      case "ArrowRight":
      case "ArrowDown":
        focusButton(Math.min(roving + 1, last));
        break;
      case "ArrowLeft":
      case "ArrowUp":
        focusButton(Math.max(roving - 1, 0));
        break;
      case "Home":
        focusButton(0);
        break;
      case "End":
        focusButton(last);
        break;
      default:
        return; // not a roving key — let it through
    }
    e.preventDefault();
  }

  // Button order in the roving group: prev, play/pause, next, return.
  // "Return to playing block" needs a playing block to return to — disable it
  // until one exists, so an idle press isn't a silent no-op (it scrolls+focuses
  // the active BlockRow, which is null before playback has placed the playhead).
  const buttons: {
    label: string;
    glyph: string;
    onClick: () => void;
    disabled?: boolean;
    keyShortcuts?: string;
  }[] = [
    { label: "Previous block", glyph: "⏮", onClick: prevBlock },
    // aria-keyshortcuts="Space" advertises the global play/pause hotkey (#134) on
    // the play/pause control itself. The ←/→ seek shortcuts are advertised on the
    // transcript region, not here, to avoid clashing with the toolbar's roving ←/→.
    {
      label: isPlaying ? "Pause" : "Play",
      glyph: isPlaying ? "⏸" : "▶",
      onClick: toggle,
      keyShortcuts: "Space",
    },
    { label: "Next block", glyph: "⏭", onClick: nextBlock },
    {
      label: "Return to playing block",
      glyph: "⟲",
      onClick: returnToPlayingBlock,
      disabled: activeBlockId == null,
    },
  ];

  return (
    <div className="transport-bar" data-testid="transport-bar">
      {/* The opaque audio_url (D4) is set imperatively by useAudio.setSource.
          Visually hidden, no native controls — the deck drives it.
          preload="metadata" (not "none") so the element reaches HAVE_METADATA on
          its own: a cold resume restore can then land its seek and fire 'seeked',
          instead of a deferred currentTime set that never lands (the block-0
          clobber). Metadata only — no full audio prefetch. */}
      <audio
        ref={audioRef}
        className="transport-bar__audio"
        preload="metadata"
        data-testid="audio-element"
        aria-label="Narration audio"
      />

      <div
        role="toolbar"
        aria-label="Playback transport"
        className="transport-deck__group"
        onKeyDown={onToolbarKeyDown}
      >
        {buttons.map((b, i) => (
          <button
            key={b.label}
            ref={(el) => {
              btnRefs.current[i] = el;
            }}
            type="button"
            className="transport-deck__btn"
            tabIndex={i === roving ? 0 : -1}
            aria-label={b.label}
            aria-keyshortcuts={b.keyShortcuts}
            disabled={disabled || b.disabled}
            onClick={b.onClick}
          >
            <span aria-hidden="true">{b.glyph}</span>
          </button>
        ))}
      </div>

      <BlockScrubber />

      <TransportClock />

      {hasAudio ? (
        // Visible discoverability legend (#134). The "10s" derives from
        // SEEK_STEP_SECONDS so the displayed step can never drift from the hook.
        <span className="transport-bar__hint" data-testid="transport-legend">
          Space play/pause · ← → {SEEK_STEP_SECONDS}s
        </span>
      ) : (
        <span className="transport-bar__hint">No audio loaded</span>
      )}
    </div>
  );
}
