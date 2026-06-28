# POST /narrate/block escalation persists a 3-file persistent-sink dir per render_id (Option 1)

| Field    | Value            |
|----------|------------------|
| Date     | 2026-06-28       |
| Status   | accepted         |
| Category | architecture     |
| Tags     | narrate-server, http-bridge, escalation-endpoint, narrate-block, render-id, persistent-sink, patchblock, byte-preserving, three-file-dir, audio-url, posix-unlink, os-rename-atomic, content-hash-cache-dormant, supersedes, issue-110, issue-109 |

## Context

`POST /narrate/block` (#110) re-renders ONE block of a prior `POST /narrate` render while byte-preserving every other block. The escalation endpoint needs server-side state from the original render to patch a single block: the plan, the timeline/manifest, the per-block byte ranges, and the audio. The research scope is `research/narrate-block-render-id-state-110/`.

The crux is a storage-model gap. `/narrate` (#109) currently persists ONLY a combined WAV plus a `createdAt` — the plan, the timeline, the per-block WAVs, AND the source text are all discarded [verified]. So nothing on disk today supports a single-block patch. The existing, tested patch path (`persistent.PatchBlock`) requires all 3 persistent-sink files (`audio.wav` + `plan.json` + `manifest.json`) present and returns `ErrNothingToPatch` on absence [verified]. This forces a change to how `/narrate` stores a render if that render may later be escalated.

This decision SUPERSEDES the single-wav claim recorded in `2026-06-28-render-id-wav-ttl-reaper-orphan-scan-lifecycle.md` (the claim that the render uses the single-wav `WAVFileSink` variant with no `plan.json` / `manifest.json` sidecars). It trips — does not violate — the WAVFileSink-no-sidecars revisit trigger.

## Options considered

### Option 1: 3-file persistent-sink dir per render_id, keyed by render_id (CHOSEN)
- **Pros**: Byte-preserving guarantee for untouched blocks. The patch path is made entirely of already-proven, tested code — `PatchBlock` passes its patch tests; byte ranges are derived from the manifest timing [verified]. POSIX open-fd survival across `unlink` holds on Linux + macOS, so the lock-free reaper/serve path stays safe [verified]. `os.Rename` is atomic same-dir/same-fs on Linux + macOS [verified]. The endpoint reuses the proven `persistent.PatchBlock` + readBack path and returns an `escalateResponse` with an `audio_url` field (not an `audio_ref` + dir).
- **Cons**: `/narrate` must move from single-wav to a 3-file dir per render_id under `tempRoot/{render_id}/` (keyed by render_id, NOT a user-supplied dir) — a storage-model change. EXDEV applies cross-fs; durability needs `fsync`.

### Option 2: In-memory plan/state held server-side between /narrate and /narrate/block
- **Pros**: No disk round-trip; avoids re-reading files on escalation.
- **Cons**: Reopens seam-gap R1. Causes an RSS balloon — unbounded server memory growth holding plans/audio for every render that might be escalated. Rejected.

### Option 3: Lazy materialization — reconstruct the 3-file dir on first escalation
- **Pros**: Keeps `/narrate` cheap until an escalation actually arrives.
- **Cons**: Cannot materialize the manifest, because `/narrate` discards the plan AND the source text — there is nothing to rebuild from [verified]. Adds a new crash window and file/dir heterogeneity. Rejected as a trap.

## Decision

For `POST /narrate/block`, narrate-server persists a **3-file persistent-sink directory per render_id** (`audio.wav` + `plan.json` + `manifest.json`) under `tempRoot/{render_id}/`, keyed by render_id (NOT a user-supplied dir). The escalation endpoint reuses the proven `persistent.PatchBlock` + readBack path and returns an `escalateResponse` with an `audio_url` field (not an `audio_ref` + dir). This is Option 1 of the three weighed options.

Reasoning: Option 1 is the only option that gives a byte-preserving guarantee for untouched blocks via a patch path made entirely of already-proven, tested code. `PatchBlock` requires all 3 files present (returns `ErrNothingToPatch` on absence), which is precisely why a full 3-file dir is mandatory and why the single-wav-only model can no longer hold for escalatable renders. Option 2 reopens seam-gap R1 and balloons RSS. Option 3 is unbuildable — the discarded plan and source text leave nothing to reconstruct the manifest from — and adds a crash window plus file/dir heterogeneity.

The content-hash escalation cache stays DORMANT in phase one: per-block hashes were already rejected by `2026-06-20-pipeline-block-rerender-uses-document-hash.md`, and regeneration is sub-100ms, so caching earns nothing now.

## Consequences

- `/narrate` must move from the single-wav `WAVFileSink` storage model to the 3-file persistent-sink dir per render_id (for renders that may be escalated). This supersedes the single-wav claim in the render-id wav lifecycle decision and trips its WAVFileSink-no-sidecars revisit trigger.
- The escalation patch path inherits the proven correctness of `persistent.PatchBlock` and the POSIX/`os.Rename` atomicity properties already relied on by the serve/reaper path.
- Durability caveat: `os.Rename` atomicity is same-dir/same-fs only (EXDEV cross-fs); durability needs `fsync`.
- The content-hash escalation cache remains unused phase one; escalation regenerates the block (sub-100ms) rather than consulting a cache.

## Related decisions

- [render_id wav lifecycle — TTL reaper plus orphan-scan, deletes outside the store write-lock](../architecture/2026-06-28-render-id-wav-ttl-reaper-orphan-scan-lifecycle.md) — SUPERSEDED by this decision on the single-wav storage claim; under #110 the render moves to the 3-file persistent-sink dir per render_id.
- [Pipeline block re-render uses the document hash (per-block hashes rejected)](../architecture/2026-06-20-pipeline-block-rerender-uses-document-hash.md) — why the content-hash escalation cache stays dormant phase one.
- [WAVFileSink reuses persistent-sink wav-concat math but writes only the combined wav, no JSON sidecars](../architecture/2026-06-28-wavfilesink-reuses-wav-concat-no-sidecars.md) — the single-wav sink variant this decision moves away from for escalatable renders.

## Experiments

All load-bearing claims were adversarially verified in Stage 3 of the #110 research (`research/narrate-block-render-id-state-110/`):
- Byte-preserving untouched blocks via `PatchBlock` + manifest-derived byte ranges. [verified]
- POSIX open-fd survival across `unlink` on Linux + macOS. [verified]
- `os.Rename` atomic same-dir/same-fs on Linux + macOS (EXDEV cross-fs; durability needs fsync). [verified]
- `PatchBlock` returns `ErrNothingToPatch` when any of the 3 files is absent. [verified]
- `/narrate` (#109) persists only a combined wav + createdAt; plan, timeline, per-block wavs, and source text are discarded. [verified]

## Revisit trigger

Revisit if a hosted/multi-user deployment changes the tempRoot lifecycle, if escalation regeneration stops being sub-100ms (making the content-hash cache worth waking), or if the storage backend becomes non-POSIX (breaking the open-fd-survives-unlink and same-fs `os.Rename` assumptions).
