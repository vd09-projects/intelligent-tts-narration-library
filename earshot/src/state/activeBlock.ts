// activeBlock.ts — pure derivation of the active block from playback position.
//
// active-block is DERIVED, never stored (plan State Ownership). Given the
// timeline (block_id → [start_ms, end_ms)) and the current <audio> position,
// return the block_id under the playhead. Half-open intervals [start, end) so a
// boundary tick belongs to the next block. Returns null when nothing matches
// (before the first block, after the last, or no timeline).

import type { Timeline } from "../api/types";

export function deriveActiveBlockId(
  timeline: Timeline | undefined,
  currentTimeMs: number,
): string | null {
  if (!timeline || timeline.blocks.length === 0) {
    return null;
  }
  for (const t of timeline.blocks) {
    if (currentTimeMs >= t.start_ms && currentTimeMs < t.end_ms) {
      return t.block_id;
    }
  }
  return null;
}
