# Escalate response shaped from post-patch on-disk read-back (plan.json + manifest.json), not from NarrateResult; add additive persistent.ReadManifest

| Field    | Value            |
|----------|------------------|
| Date     | 2026-06-21       |
| Status   | accepted         |
| Category | architecture     |
| Tags     | cmd/narrate-server, http-escalate, narrateresult, readback, persistent-sink, readmanifest, seam-gap, plan-schema, issue-49 |

## Context

Issue #49's HTTP escalate endpoint re-renders one block to a higher level via `persistent.PatchBlock` and must return a response describing the patched block (updated `Block`, `BlockTiming`, and `audio_ref`). The implementation needs a source of truth for that updated state.

Build review surfaced **seam gap R1**: `NarrateResult` (the in-memory result of the narrate/patch operation) does **not** expose the updated per-block `Block` / `BlockTiming` / `audio_ref`. The information the response needs is written to disk by the persistent sink but is not handed back through the call's return value.

Two ways to close the gap: change `plan/` or `NarrateResult` to carry the updated block state through the call, or read the already-written state back off disk.

## Options considered

### Option A: Shape the response from post-patch on-disk state (read back plan.json + manifest.json)
- **Pros**: The on-disk `plan.json` + `manifest.json` are the authoritative post-patch artifacts (per the manifest-is-the-index decision), so reading them back yields exactly what was persisted — no risk of the response diverging from disk. No change to `plan/` (engine-neutral, zero-deps) or to `NarrateResult`'s contract. Smallest blast radius; the only new surface is an additive exported `persistent.ReadManifest`.
- **Cons**: An extra read of files just written (minor I/O). Couples the HTTP handler to the persistent sink's on-disk layout. Leaves R1 as a known seam gap rather than closing it at the source.

### Option B: Enrich NarrateResult (and possibly plan/) to expose the updated Block/BlockTiming/audio_ref
- **Pros**: Closes seam gap R1 at the source; the response could be built purely from the in-memory return value with no read-back.
- **Cons**: Touches `NarrateResult` (a contract used by other callers) and risks pressure on `plan/`, which CLAUDE.md keeps engine-neutral and zero-deps. Larger blast radius for a single endpoint's response-shaping need.

## Decision

Adopt **Option A**. Shape the escalate response from **post-patch on-disk state** — read back `plan.json` and `manifest.json` from the patched output directory — because `NarrateResult` does not expose the updated `Block` / `BlockTiming` / `audio_ref` (seam gap R1). Add an **additive exported `persistent.ReadManifest`** to read the manifest, rather than modifying `plan/` or `NarrateResult`.

Reasoning: the on-disk artifacts are already the authoritative post-patch state, so reading them back is correct-by-construction and keeps the change minimal. Enriching `NarrateResult`/`plan/` for one endpoint's response would widen contracts (and risk the engine-neutral `plan/` surface) for marginal benefit.

This is accepted **with the explicit note that R1 remains a known seam gap** — a future `NarrateResult` enrichment could close it and let the handler drop the read-back.

## Consequences

- The escalate handler reads `plan.json` + `manifest.json` back from disk after `PatchBlock`, coupling it to the persistent sink's on-disk layout.
- A new exported `persistent.ReadManifest` is part of the package API (additive — no existing behavior changed).
- Seam gap R1 is documented but not closed; the read-back is the standing workaround.

## Related decisions

- [--block patch into a persistent outDir: manifest is the INDEX, byte ranges are DERIVED](../convention/2026-06-21-persistent-block-patch-manifest-index-derived-ranges.md) — establishes manifest.json + plan.json as the authoritative on-disk artifacts this read-back relies on.
- [Persistent sink's CheckStale is NOT part of the OutputSink interface](2026-06-20-persistent-checkstale-not-on-outputsink-interface.md) — same pattern: package-scope read-side functions on persistent rather than widening a shared interface.

## Revisit trigger

Revisit (and drop the on-disk read-back) if `NarrateResult` is ever enriched to expose the updated `Block` / `BlockTiming` / `audio_ref`, closing seam gap R1 at the source.
