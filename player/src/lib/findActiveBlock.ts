import type { ManifestBlock } from '../types/manifest.ts'

// findActiveBlock returns the id of the block whose [start_ms, end_ms) window
// contains tMs, or null when tMs is before the first block or after the last.
//
// Invariant (CLAUDE.md "Sync is block-level only"): we only look at block-level
// start/end. There are no per-word/per-segment timings to inspect.
//
// Binary search over manifest.blocks. Caller MUST pass blocks sorted by
// start_ms ascending — the sink writes them in document order which matches.
// O(log n) per tick is cheap (rAF runs ~60 Hz; n is small in practice but the
// algorithm wants to scale honestly).
//
// Boundary semantics:
//   - tMs < blocks[0].start_ms          → null  (pre-roll silence)
//   - tMs ≥ blocks[last].end_ms         → null  (post-roll / EOF)
//   - blocks[i].start_ms ≤ tMs < end_ms → blocks[i].id
//   - inter-block gaps (start[i+1] > end[i]) → snap to the next block when
//     tMs is in the gap; this matches the listener's intuition that the
//     highlight should already be on the upcoming block during a pause.
//
// Empty input returns null. Negative tMs returns null.
export function findActiveBlock(
  blocks: readonly ManifestBlock[],
  tMs: number,
): string | null {
  if (blocks.length === 0) return null
  if (tMs < blocks[0].start_ms) return null
  const last = blocks[blocks.length - 1]
  if (tMs >= last.end_ms) return null

  let lo = 0
  let hi = blocks.length - 1
  while (lo <= hi) {
    const mid = (lo + hi) >>> 1
    const b = blocks[mid]
    if (tMs < b.start_ms) {
      hi = mid - 1
    } else if (tMs >= b.end_ms) {
      lo = mid + 1
    } else {
      return b.id
    }
  }
  // tMs landed in an inter-block gap. `lo` now points to the next block.
  // Snap forward so the listener sees the highlight move ahead of the audio
  // rather than dropping back to "no block".
  if (lo < blocks.length) return blocks[lo].id
  return null
}
