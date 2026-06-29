// BlockScrubber.tsx — block-quantized transport slider (#112, step 5).
//
// role="slider" whose discrete stops are the document's sorted block STARTS
// (NOT continuous seconds) — the scrubber is block-quantized, matching the
// block-level-sync invariant (no sub-block / word position anywhere).
//
// Accessibility (R1): this is its OWN tab stop, ADJACENT to the toolbar button
// group — not inside the toolbar's roving group. The slider OWNS its arrow keys
// (Left/Right change the block value) and stopPropagation()s every key it
// handles so they never reach the toolbar's roving handler. APG Slider pattern.

import { useEffect, useRef } from "react";
import { usePlaybackControls } from "../state/PlaybackContext";
import { PAGE_BLOCKS } from "../hooks/usePlayback";
import { formatSpokenTime, nearestIndexFromPointer } from "../state/playbackMath";

export function BlockScrubber() {
  const {
    activeBlockIndex,
    blockCount,
    blockStartsMs,
    subscribeProgress,
    prevBlock,
    nextBlock,
    stepBlocks,
    seekToIndex,
  } = usePlaybackControls();
  const trackRef = useRef<HTMLDivElement>(null);
  const fillRef = useRef<HTMLSpanElement>(null);
  const draggingRef = useRef(false);

  // The visual fill tracks the CONTINUOUS audio clock (currentTime / duration),
  // updated imperatively every animation frame — so the bar moves smoothly
  // instead of jumping block-to-block. This is display only: aria-valuenow and
  // all SEEKING stay block-quantized (below), so the block-level-sync invariant
  // is untouched. Mutating style.width directly keeps it off the React render
  // path (no per-frame re-render).
  useEffect(
    () =>
      subscribeProgress((p) => {
        if (fillRef.current) fillRef.current.style.width = `${p.fraction * 100}%`;
      }),
    [subscribeProgress],
  );

  // No blocks → nothing to scrub. Render nothing (the deck shows its own hint).
  if (blockCount === 0) return null;

  const max = blockCount - 1;
  const hasActive = activeBlockIndex >= 0;
  const index = hasActive ? activeBlockIndex : 0;
  const startMs = blockStartsMs[index] ?? 0;
  // When nothing is active (-1), the slider falls back to index 0 for
  // aria-valuenow but announces "No block selected" so a screen-reader user
  // isn't told "Block 1" before playback has placed the playhead anywhere.
  const valueText = hasActive
    ? `Block ${index + 1} of ${blockCount}, ${formatSpokenTime(startMs)}`
    : "No block selected";

  function onKeyDown(e: React.KeyboardEvent) {
    let handled = true;
    switch (e.key) {
      case "ArrowLeft":
      case "ArrowDown":
        prevBlock();
        break;
      case "ArrowRight":
      case "ArrowUp":
        nextBlock();
        break;
      case "Home":
        seekToIndex(0);
        break;
      case "End":
        seekToIndex(max);
        break;
      case "PageUp":
        stepBlocks(PAGE_BLOCKS);
        break;
      case "PageDown":
        stepBlocks(-PAGE_BLOCKS);
        break;
      default:
        handled = false;
    }
    if (handled) {
      e.preventDefault();
      // R1: the slider owns these keys; never let them bubble to the toolbar
      // roving handler (which also binds Left/Right/Home/End).
      e.stopPropagation();
    }
  }

  function commitFromPointer(clientX: number) {
    const rect = trackRef.current?.getBoundingClientRect();
    if (!rect) return;
    const i = nearestIndexFromPointer(clientX, { left: rect.left, width: rect.width }, blockCount);
    if (i >= 0) seekToIndex(i);
  }

  function onPointerDown(e: React.PointerEvent<HTMLDivElement>) {
    draggingRef.current = true;
    e.currentTarget.setPointerCapture?.(e.pointerId);
    commitFromPointer(e.clientX);
  }
  function onPointerMove(e: React.PointerEvent<HTMLDivElement>) {
    if (draggingRef.current) commitFromPointer(e.clientX);
  }
  function onPointerUp(e: React.PointerEvent<HTMLDivElement>) {
    draggingRef.current = false;
    e.currentTarget.releasePointerCapture?.(e.pointerId);
  }

  return (
    <div
      ref={trackRef}
      role="slider"
      tabIndex={0}
      aria-label="Block position"
      aria-valuemin={0}
      aria-valuemax={max}
      aria-valuenow={index}
      aria-valuetext={valueText}
      className="block-scrubber"
      data-testid="block-scrubber"
      onKeyDown={onKeyDown}
      onPointerDown={onPointerDown}
      onPointerMove={onPointerMove}
      onPointerUp={onPointerUp}
    >
      <span className="block-scrubber__track" aria-hidden="true">
        <span ref={fillRef} className="block-scrubber__fill" style={{ width: "0%" }} />
      </span>
    </div>
  );
}
