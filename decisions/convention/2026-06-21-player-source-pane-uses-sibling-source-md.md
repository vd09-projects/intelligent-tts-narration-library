# Player source pane consumes sibling source.md (not reconstruction from raw_excerpt)

| Field    | Value            |
|----------|------------------|
| Date     | 2026-06-21       |
| Status   | accepted         |
| Category | convention       |
| Tags     | player, source-map, honesty-rule, ux, issue-18 |

## Context

The React reference player (`player/`, issue #18) needs to show the original source text in a side pane and highlight the block whose `SourceMap` covers the user's cursor. The library already exposes `Block.SourceMap.RawExcerpt` (per-block) and `Block.SourceMap.{StartLine,EndLine}` (1-indexed). Two ways to populate the pane:

- **A — Reconstruct source by concatenating `raw_excerpt` per block.** No external file needed; everything lives in `plan.json`.
- **B — Load a sibling `source.md` from the persistent-sink output directory and project highlights onto it via `start_line`/`end_line`.**

The planner is allowed to trim trailing whitespace, drop blank lines between blocks, and otherwise normalize before storing `raw_excerpt`. Concatenating excerpts would therefore drift from the true source line numbers — and the line numbers in each `SourceMap` were computed against the original document, not against the reconstruction.

## Options considered

### Option A: Reconstruct from raw_excerpt
- **Pros**: No external file required; `plan.json` is self-contained.
- **Cons**: Line numbers in the reconstruction don't match `start_line`/`end_line` (raw_excerpt is normalized). Fabricates a "source view" that disagrees with the rest of the plan. Violates the honesty rule applied to UI.

### Option B: Load sibling source.md, fall back honestly when absent
- **Pros**: Line numbers match `start_line`/`end_line` by construction. Cursor-tracked highlight is exact. If `source.md` is absent the player surfaces a banner and falls back to per-block `raw_excerpt` with an explicit "line numbers are advisory" warning — no silent fabrication.
- **Cons**: Persistent-sink output directory must include `source.md` (caller bundles it). One more file to load; one more `fetch` round-trip.

## Decision

**Choose B.** The player looks for `source.md` (or any `.md` whose basename matches `plan.source.uri`) alongside `plan.json` / `manifest.json` / `audio.wav` in the loaded directory. If present, the source pane renders it verbatim with one `<span data-block-id>` per block range derived from `start_line`/`end_line`. If absent, the pane shows a notice — "source file not bundled with this output" — and falls back to per-block `raw_excerpt` rendering with a banner that line numbers are advisory.

This extends the library's honesty rule from the audio layer into the UI: never silently fabricate a source view. Refusal-as-data has a UI analogue — visible degraded-rendering with an explicit banner.

## Consequences

- `sink/persistent` does not currently write `source.md` automatically. Demo workflows must copy or re-emit it (the bundled fixture under `player/public/fixtures/sample/source.md` does so).
- The `loadDirectory` helper is required to tolerate the missing-file case without throwing; honesty rule extends to data-load failure modes.
- A future ticket may make the persistent sink emit `source.md` to remove the friction.

## Related decisions

- [Persistent sink atomic tmp+rename writes](../convention/2026-06-20-persistent-atomic-tmp-rename-writes.md) — honesty rule applied to bytes-on-disk.
- [[2026-06-21-player-dual-data-loading-fixture-and-picker]] — co-decision on how the directory is supplied.
