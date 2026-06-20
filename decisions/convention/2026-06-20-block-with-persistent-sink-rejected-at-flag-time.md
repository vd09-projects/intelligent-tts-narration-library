# --block X with --sink=persistent rejected at flag-validation

- **Date:** 2026-06-20
- **Status:** accepted
- **Category:** convention
- **Tags:** [cmd, narrate, persistent-sink, block-rerender, flag-validation, honesty-rule, issue-16]
- **Owner:** vd
- **Scope:** issue-16

## Context

Issue #14 added `cmd/narrate --block <id>` for per-block re-render. The pipeline returns a single-block `RenderResult` containing only that block's audio. Issue #16 wires the persistent sink. Combining the two — `--block X --sink=persistent --out /existing/dir` — would cause `Consume` to faithfully concatenate the single-block render into a one-block `audio.wav`, silently overwriting the multi-block output that was previously written.

The honesty rule (CLAUDE.md) is load-bearing: errors stop, refusals are surfaced, partial state is never honest. A silent multi-block-to-one-block overwrite is worse than any of those three.

## Options considered

### Option A: Reject `--block X --sink=persistent` at flag-validation (CHOSEN)
- **Pros**: Loud refusal before any pipeline work happens. The error message names both flags and points at the planned follow-up ticket for block-level patch into an existing persistent outDir. Caller corrects their command line and re-runs. Honesty rule preserved.
- **Cons**: A future "patch one block into the existing audio.wav" use case has to wait for that follow-up ticket.

### Option B: Silently fall back to ephemeral when both flags are set
- **Pros**: Caller still gets per-block audio (through speakers).
- **Cons**: Caller asked for persistent and got ephemeral. The user-supplied `--out` is now silently ignored. Worse than the explicit rejection.

### Option C: Try to splice the new block's audio into the existing `audio.wav` based on the existing manifest
- **Pros**: Implements the use case directly.
- **Cons**: Complex (re-read the existing audio.wav, locate the block's byte range, slice and concat). Manifest staleness corner cases (what if `content_hash` mismatched?). Out of scope for issue #16's AC.

### Option D: Implement Option C as part of issue #16
- **Pros**: One ticket covers everything.
- **Cons**: Scope creep. The plan would not have shipped this round.

## Decision

`flagSet.validate()` returns an error if `Block != "" && Sink == "persistent"`. The error message:

> --block and --sink=persistent are not yet supported together (block-level patch into a persistent outDir is a follow-up)

routes through `errFlagValidation` to exit code 2. A follow-up ticket queues for the block-level patch design.

## Consequences

- Callers who type the combination get an explicit error naming both flags and explaining the gap.
- The persistent-sink `audio.wav` invariant ("contains every block in plan order, concatenated with silence gaps") holds — no silent partial overwrite.
- The follow-up ticket can design the block-level patch with the manifest as the authoritative index for byte-range slicing.

## Related decisions

- [Pipeline block re-render uses document-level content_hash](../architecture/2026-06-20-pipeline-block-rerender-uses-document-hash.md) — establishes the document-level hash as the staleness check; the persistent-sink follow-up patch would need to honor the same hash semantics.
- [Persistent-sink atomic tmp+rename writes](2026-06-20-persistent-atomic-tmp-rename-writes.md) — partial-write guard the follow-up patch design will inherit.

## Revisit trigger

When the follow-up ticket designing block-level patch into an existing persistent outDir lands, this rejection becomes obsolete. Update the validate() guard, the error message reference, and remove this decision's "experimental" status flag if applicable.
