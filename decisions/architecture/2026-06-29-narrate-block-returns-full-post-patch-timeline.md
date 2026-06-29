# /narrate/block returns the FULL post-patch timeline; client replaces its timeline wholesale

| Field    | Value            |
|----------|------------------|
| Date     | 2026-06-29       |
| Status   | accepted         |
| Category | architecture     |
| Tags     | issue-113, earshot, narrate-server, narrate-block, timeline, patchblock, downstream-offsets, whole-timeline-replace, frozen-escalate-core, block-level-sync, api-contract |

## Context

Issue #113 escalates one block in place via `POST /narrate/block`. The patch path runs `capturingSink → sink/persistent.PatchBlock → on-disk readBack`. **Verified:** `PatchBlock` rewrites the single combined `audio.wav` and **shifts all downstream offsets** (`TestPatchBlock_OverExisting_Longer`: a +60 ms grow on `b1` moves `b2`'s start 410→460 ms). Today `runNarrateBlock`'s `readBack` returns only the **one** patched block's timing, so the client cannot learn the shifted sibling offsets — seeking to a downstream sibling would land on the wrong audio. The v1 plan replaced only the one patched timing and left siblings byte-identical ("stale-elsewhere accepted"), which is correct for a per-directory sink but wrong for the single-combined-wav render where one patch reflows the whole file.

## Options considered

### Option A (v1, superseded): per-block merge isolation, leave siblings stale
- **Pros**: minimal; matches the per-dir sink mental model.
- **Cons**: under the single-combined-wav render, every downstream sibling offset is stale after a patch; sibling seeks land on the wrong audio. Wrong for this render model.

### Option B (chosen): full post-patch timeline on the response, client replaces wholesale
- **Pros**: client adopts the server's authoritative offsets with zero client-side offset math (honors block-level-sync-only); sibling *content* untouched, sibling *offsets* track the server.
- **Cons**: one additive response field.

## Decision

Add a `timeline` field to **`narrateBlockResponse` only**, built inside `runNarrateBlock` from the **post-patch manifest** (the same on-disk manifest `readBack` already loads). The Earshot client replaces the entry's whole `transcript.timeline` **wholesale** with `response.timeline` while replacing only the one patched block in `transcript.blocks` (by id). The client never recomputes offsets. **The frozen `escalateResponse` and the shared `escalateInDir` core are NOT touched** — the new field is assembled in the `/narrate/block` handler *after* the shared core returns, so `runEscalate` stays byte-identical. **AC6 reinterpreted:** sibling content untouched; sibling offsets track the server timeline. Supersedes the initially-planned per-block merge-isolation approach (that approach was never journaled).

## Consequences

- `narrateBlockResponse` JSON gains an additive `timeline`; an older server lacking it falls back to a single-timing merge (documented degrade).
- The client's whole-timeline pointer swap is O(1); no client-side offset arithmetic enters the codebase, preserving the no-word-timing / block-level-sync invariant.
- A no-network de-escalate must restore the timeline that was authoritative *at that level* — see the seed-snapshot-cache decision (F1).

## Related decisions

- [POST /narrate/block escalation persists a 3-file dir per render_id](2026-06-28-post-narrate-block-escalation-persists-a-3-file.md) — the PatchBlock path whose downstream-offset shift forces this.
- [Earshot seeds a per-(blockId,level) snapshot cache with paired timeline](2026-06-29-earshot-seed-level-snapshot-cache-no-model-rebill.md) — pairs each cached level with the timeline authoritative at that level so de-escalate offsets stay correct.
