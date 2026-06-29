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

import { useRef, useState } from "react";
import { useNarration } from "../state/NarrationContext";
import { usePlaybackControls } from "../state/PlaybackContext";
import { BlockScrubber } from "./BlockScrubber";

export function TransportDeck() {
  const { audioRef, activeAudioUrl } = useNarration();
  const { isPlaying, blockCount, toggle, prevBlock, nextBlock, returnToPlayingBlock } =
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
  const buttons: { label: string; glyph: string; onClick: () => void }[] = [
    { label: "Previous block", glyph: "⏮", onClick: prevBlock },
    { label: isPlaying ? "Pause" : "Play", glyph: isPlaying ? "⏸" : "▶", onClick: toggle },
    { label: "Next block", glyph: "⏭", onClick: nextBlock },
    { label: "Return to playing block", glyph: "⟲", onClick: returnToPlayingBlock },
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
            disabled={disabled}
            onClick={b.onClick}
          >
            <span aria-hidden="true">{b.glyph}</span>
          </button>
        ))}
      </div>

      <BlockScrubber />

      {!hasAudio ? <span className="transport-bar__hint">No audio loaded</span> : null}
    </div>
  );
}
