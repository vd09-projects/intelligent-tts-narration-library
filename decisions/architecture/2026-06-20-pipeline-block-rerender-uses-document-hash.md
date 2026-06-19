# Pipeline block re-render uses document-level content_hash

- **Date:** 2026-06-20
- **Status:** accepted
- **Category:** architecture
- **Tags:** [pipeline, block-rerender, content-hash, staleness, plan-schema, phase-one, issue-14]
- **Source:** harvested from cmd-narrate-block-rerender-14 build summary v1, decision mark v1

## Context

Issue #14 adds a `narrate --block <id> --level {1|2|3}` flow so users can listen at gist, pause, and escalate one block to a higher leveling depth — without re-narrating the document. The CLI also accepts an optional `--expected-content-hash <hex>` so a caller who captured a block id at one point in time can detect that the underlying document has changed under them before the escalation runs.

The design question: against what does `--expected-content-hash` compare? Three options were on the table.

1. **Per-block content hash on `plan.Block`.** Add a new `ContentHash string` field to `plan.Block` populated by the planner from the block's raw text. The CLI compares `--expected-content-hash` against this per-block hash.
2. **Document-level content hash on `plan.SourceRef.ContentHash`** (already in the plan schema, written by every adapter). The CLI compares against this — a hash of the WHOLE document.
3. **Both** — surface per-block AND document-level hashes; let callers choose.

## Decision

Compare `--expected-content-hash` against the document-level `plan.NarrationPlan.Source.ContentHash`. Surface the comparison result via `pipeline.NarrateResult.BlockHashMismatch` (warning, not error — non-fatal: the re-render still proceeds). The document hash is exposed to callers via `pipeline.NarrateResult.DocumentContentHash` and printed by `cmd/narrate` as the trailing `content_hash=<hex>` key on the stdout summary line.

Block-level hashes are **not** added to the plan schema.

## Rejected alternatives

1. **Per-block ContentHash on `plan.Block`.** Rejected because:
   - The plan schema is JSON-on-wire and versioned additive-compatible. Every new field adds a forward-compat burden.
   - The planner regenerates blocks deterministically from the raw document (the segmenter is pure goldmark + heuristics — same input always yields the same blocks). So a block-level hash carries no information not already implied by the document hash: if the document hash matches, every block hash also matches; if the document hash differs, at least one block hash MAY differ, but the right user signal is still "the document is stale, you should re-roster".
   - Per-block hashes would tempt callers to use them as cache keys for incremental re-render, but the planner regen path is sub-100ms — the perf incentive does not exist in phase one.
   - Adds a new code path to maintain (planner has to compute the hash; tests have to cover its stability across plan re-runs).

2. **Both.** Same maintenance cost as per-block-only, plus a documentation burden: which one is canonical for `--expected-content-hash`? Users would have to learn two semantics. Pick one and stop.

## Consequences

- Callers (CLI and MCP) capture the document hash from the stdout summary or the `NarrateResult.DocumentContentHash` field, persist it alongside the chosen block id, and pass it back via `NarrateRequest.ExpectedContentHash` on a later `--block` re-render.
- A mismatch is non-fatal: the re-render still runs (the user asked for it). A stderr warning surfaces the drift so the caller knows the audio they heard may not align with their stale roster.
- `--expected-content-hash` without `--block` is a flag error (`validate()` rejects). Without `--block` the pipeline takes the whole-doc path and never checks the hash — the guard would be silently ineffective, so we surface the misuse instead.
- Future work on a streaming / incremental planner (currently out of scope per CLAUDE.md) may revisit this if the planner stops being deterministic across re-runs.
- The plan schema stays narrower: `schema_version` does not need to bump for this feature.

## Related decisions

- Pipeline composition root pattern (`architecture/2026-06-18-pipeline-composition-root-pattern.md`) — `Pipeline.Narrate` stays the single public method even with the new single-block branch; no `NarrateBlock` method added.
- Plan schema versioning (implicit in CLAUDE.md `## Quick conventions` — additive-compatible within a major `schema_version`) — informs the rejection of per-block hashes.

## Revisit trigger

If the planner ever loses determinism (e.g., adds non-deterministic intelligence calls that change segmentation), per-block hashes become necessary. Revisit then.
