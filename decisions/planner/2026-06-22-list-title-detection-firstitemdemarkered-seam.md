# Issue #54 colon-gated list-title detection resolved via upstream firstItemDemarkered seam; AC1 taken as documented divergence

| Field    | Value            |
|----------|------------------|
| Date     | 2026-06-22       |
| Status   | accepted         |
| Category | planner          |
| Tags     | planner, level.go, segment.go, list-voicing, honesty-rule, issue-54 |

## Context

`splitListTitle` (planner/level.go) used a single signal — a trailing colon on the
first non-marker line — to decide whether a list block has a leading title. goldmark
strips the marker off the first list item before the block reaches `levelList`, making
that first line ambiguous: a real title is indistinguishable from a de-markered item
one.

This caused two failure directions:
- **Direction 2:** a bare list whose genuine first item ends in a colon (e.g.
  "Notes:") was mis-promoted to a title and dropped from the items — undercounting N
  and shifting every `ordinalCue`.
- **Direction 1 (the inverse):** AC1's case — a non-colon leading plaintext label
  that should be recognised as a preamble.

The prior decision (2026-06-21, ticket #45) chose the colon-gated heuristic
knowingly and documented Direction 2 as an accepted false positive. Issue #54
resolves that false positive by construction rather than living with it.

## Options considered

### Option A: Narrow the colon heuristic using marker-stripped asymmetry
- **Pros**: stays inside the existing single-signal approach.
- **Cons**: a heuristic explaining a heuristic; the AC1 relaxation it implies is
  risky. Rejected.

### Option B-markdown: Segmenter sets a firstItemDemarkered seam flag
- **Pros**: exact, not heuristic, on the goldmark path. Kills Direction 2 by
  construction. No `plan/` schema change; no I/O added to the planner; honesty rule
  preserved. Chosen.
- **Cons**: leaves the trailing-colon branch in place for the plaintext-fallback
  shape (latent debt, see Consequences).

### Option B-plaintext: Titled-seam segmenter change (fold adjacent label into list)
- **Pros**: would deliver literal AC1 behavior on the plaintext path.
- **Cons**: re-opens the top-level-walk segmenter tradeoff; larger scope. Deferred
  out of scope to a separate ticket.

## Decision

Adopt **Option B-markdown**. The segmenter sets a planner-internal
`firstItemDemarkered` flag on `rawBlock` (populated in `rawBlockFromNode` as
`hint == hintList`), threaded into `splitListTitle`. When true, the colon title
branch is skipped and the colon-terminated first item is counted in N. This kills
Direction 2 (AC2) by construction on the goldmark path — exact, not heuristic.
AC3/AC4/AC5 fall out cleanly. No `plan/` schema change; no I/O added to the planner;
the honesty rule is preserved.

**AC1 divergence (the key recorded decision):** AC1 — a non-colon leading plaintext
label recognised as a preamble — is taken as a **wontfix-by-design** ticket
divergence rather than expanding scope into the larger titled-seam segmenter change
(Option B-plaintext). Rationale:

- In true markdown the non-colon leading title is **already voiced correctly** — as
  its own top-level prose block, not as a list preamble. goldmark does not fold a
  preceding label paragraph into the following list node, so nothing is lost.
- On the plaintext-fallback path the leading label is indistinguishable from a
  marker-less first item without guessing, and guessing would violate the project's
  non-negotiable **honesty rule** (never fabricate).
- Expanding scope (Option B-plaintext folding adjacent labels) re-opens the
  top-level-walk segmenter tradeoff and is deferred to a separate ticket if literal
  AC1 behavior is ever wanted.

## Consequences

- Direction 2 (AC2) eliminated by construction on the goldmark path; AC3/AC4/AC5
  satisfied.
- AC1 is a documented divergence (wontfix-by-design), not a regression: the
  non-colon leading title is voiced as its own prose block in true markdown.
- **Latent debt (accepted):** the trailing-colon title branch is retained for the
  plaintext-fallback shape (`firstItemDemarkered == false`). Its reachability is
  uncertain — only reachable if a `hintProse` plaintext block is classified as a
  list downstream. Booked as accepted latent debt.

## Related decisions

- [Colon-gated list title detection under goldmark marker stripping](../tradeoff/2026-06-21-colon-gated-list-title-detection-goldmark.md) — superseded by this decision; the false-positive direction it knowingly accepted is now resolved by construction via the firstItemDemarkered seam.
- [List preamble: titled reuses source, bare generates "List of N items."](../convention/2026-06-21-list-preamble-titled-reuses-source-bare-generates.md) — the rule that consumes the title-vs-bare signal this decision now resolves exactly.

## Revisit trigger

Revisit if literal AC1 behavior (non-colon leading label folded as a list preamble)
is ever wanted on the plaintext-fallback path — that re-opens the Option B-plaintext
titled-seam segmenter change and the top-level-walk tradeoff. Also revisit the
retained plaintext-fallback colon branch if a `hintProse` block ever becomes
reachable as a list downstream (making the latent debt live).
