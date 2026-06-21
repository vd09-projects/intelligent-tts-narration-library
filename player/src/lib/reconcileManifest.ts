import type { Manifest, ManifestBlock } from '../types/manifest.ts'

// reconcileManifest merges a freshly re-fetched manifest (`next`) into the one
// currently held in state (`prev`), preserving per-block object identity for
// rows that did not change. Returns a NEW top-level object so React sees the
// manifest as changed, but reuses the prior block object reference whenever the
// incoming block is deep-equal — so React.memo(BlockRow) skips re-rendering
// unchanged rows after an escalation patch.
//
// Reuse identity ⇒ a row whose timing/status/etc. did not move keeps the exact
// same object reference across the patch, so its memoized row never re-renders.
//
// NOTE: this does a deep-equal per block, which is fine because the block count
// is bounded (reference-player scale — a single narrated document). Do NOT copy
// this per-block deep-equal into a large-document path; it is O(blocks × fields)
// per reconcile.
export function reconcileManifest(prev: Manifest, next: Manifest): Manifest {
  const prevById = new Map(prev.blocks.map((b) => [b.id, b]))
  const blocks = next.blocks.map((nb) => {
    const pb = prevById.get(nb.id)
    return pb && blocksEqual(pb, nb) ? pb : nb
  })
  return { ...next, blocks }
}

// blocksEqual is a manifest-row memo-identity check: did this timing row change,
// such that React.memo(BlockRow) must re-render it? NOT the same notion as
// App.tsx `blocksEqualForChange`, which is a plan-block *content* change check
// (did the spoken output change, for the user-facing "no visible change" ack).
function blocksEqual(a: ManifestBlock, b: ManifestBlock): boolean {
  return (
    a.id === b.id &&
    a.class === b.class &&
    a.level === b.level &&
    a.status === b.status &&
    a.start_ms === b.start_ms &&
    a.end_ms === b.end_ms &&
    a.audio_ref === b.audio_ref
  )
}
