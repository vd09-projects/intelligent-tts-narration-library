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

import { useRef } from "react";
import { usePlaybackControls } from "../state/PlaybackContext";
import { PAGE_BLOCKS } from "../hooks/usePlayback";
import { formatSpokenTime, nearestIndexFromPointer } from "../state/playbackMath";

export function BlockScrubber() {
  const { activeBlockIndex, blockCount, blockStartsMs, prevBlock, nextBlock, stepBlocks, seekToIndex } =
    usePlaybackControls();
  const trackRef = useRef<HTMLDivElement>(null);
  const draggingRef = useRef(false);

  // No blocks → nothing to scrub. Render nothing (the deck shows its own hint).
  if (blockCount === 0) return null;

  const max = blockCount - 1;
  const index = activeBlockIndex >= 0 ? activeBlockIndex : 0;
  const startMs = blockStartsMs[index] ?? 0;
  const valueText = `Block ${index + 1} of ${blockCount}, ${formatSpokenTime(startMs)}`;
  const fillPct = max > 0 ? (index / max) * 100 : 0;

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
        <span className="block-scrubber__fill" style={{ width: `${fillPct}%` }} />
      </span>
    </div>
  );
}
