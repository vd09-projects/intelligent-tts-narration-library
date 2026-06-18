# Single canonical demo doc at docs/samples/sample.md

- **Date:** 2026-06-18
- **Status:** accepted
- **Category:** convention
- **Tags:** [docs, samples, fixtures, demo, phase-one, issue-7]
- **Source:** harvested from cmd-narrate-issue-7 build summary v1, decision mark v4

## Context

The vertical-slice CLI needed a markdown document to narrate — for the README quickstart, the manual smoke test, and the planner benchmark. The question was whether to ship one canonical doc covering all block classes (prose, code, list, table, bare-image refusal), or a directory of fixtures, one per content type.

## Decision

One canonical file at `docs/samples/sample.md` (~561 words) covers all phase-one block classes in a single document:

- ≥1 prose paragraph (≥120 words so deterministic L1 verbatim-read activates cleanly)
- ≥1 fenced code block (Go)
- ≥1 ordered or unordered list
- ≥1 Markdown table
- ≥1 bare image (`![chart](nonexistent-chart.png)`) — the honest refusal block

The README quickstart points users at this file, the manual smoke test runs `cmd/narrate --file docs/samples/sample.md`, and the planner benchmark reads the same path. One file felt everywhere.

## Rejected alternative

**Directory of fixtures** (`docs/samples/prose.md`, `docs/samples/code.md`, `docs/samples/table.md`, ...) — one file per content class. The downsides outweigh the upside:

- Splits the listener's attention across three commands to hear all classes.
- Each fixture drifts independently — a `wc -w` constraint or a prose-too-short fix in one doesn't propagate to siblings.
- The README quickstart would have to either pick one (defeating the purpose) or list several (noise for a 30-second demo).

> One file felt everywhere beats a directory of fixtures.

## Consequences

- Future content classes (diagrams-as-text, multi-language code) get appended to `sample.md` rather than added as new files.
- If `sample.md` ever needs to grow beyond the 400–600 word band (e.g., for stress testing the oversized-block split), the rule changes: stress fixtures live alongside as `docs/samples/stress-*.md` but the canonical demo stays at `sample.md`.
- The benchmark perf numbers (planner ~0.344 ms/op at 290× headroom) are baselined against the 561-word content; any major edit to `sample.md` should rerun the bench and capture the new number in the next issue's close note.

## Related decisions

- Manual smoke build-tag gating (`convention/2026-06-18-manual-smoke-build-tag-gating.md`) — the smoke test runs against this file.
- Two-track benchmark methodology (`convention/2026-06-18-two-track-benchmark-methodology.md`) — both bench shapes use this file as input.
