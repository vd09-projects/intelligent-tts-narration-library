# Colon-gated list title detection under goldmark marker stripping

| Field    | Value            |
|----------|------------------|
| Date     | 2026-06-21       |
| Status   | accepted         |
| Category | tradeoff         |
| Tags     | list, title-detection, goldmark, heuristic, false-positive, planner, ticket-45 |

## Context

Ticket #45 — the list-preamble rule (titled list reuses the source title; bare list
generates "List of N items.") needs to decide whether a list has a preceding title
line. The segmenter is goldmark's CommonMark AST. goldmark strips the marker from
the first list item, so by the time the planner sees the AST it cannot structurally
distinguish a true title line that precedes the list from a de-markered first list
item — both surface as a plain text line ahead of the remaining items.

## Options considered

### Option A: Structural detection via the AST
- **Pros**: would be exact if the AST preserved the distinction.
- **Cons**: not available — goldmark has already stripped the first item's marker,
  so the structural signal that would separate "title" from "first item" is gone.

### Option B: Colon-gated heuristic (a trailing colon promotes the line to a title)
- **Pros**: a trailing colon is the one reliable textual signal left that a line is
  a label introducing the list ("Steps to deploy:"); cheap, deterministic.
- **Cons**: constraint-driven heuristic with a known false-positive direction — a
  genuine first list item that legitimately ends in a colon can be mis-promoted to
  a title and dropped from the spoken item set.

## Decision

Chose Option B. Treat a trailing colon on the line preceding the list items as the
signal that the line is a title (and therefore eligible for the titled-preamble
branch). This is explicitly a constraint-driven heuristic forced by goldmark's
marker stripping, not a principled structural test.

The false-positive direction is documented and accepted: a real first item ending
in a colon can be misread as a title. The bias is chosen knowingly — the colon is
the only reliable distinguishing signal available, and titled-introduction lines
ending in a colon are far more common than list items ending in a colon.

## Consequences

- Known failure mode: a genuine first list item ending in ":" is promoted to a
  title and consumed as the preamble instead of being voiced as item one.
- Detection accuracy is bounded by the segmenter's behavior; a different segmenter
  (or AST access that preserves the first marker) would change the calculus.

## Related decisions

- [List preamble: titled reuses source, bare generates "List of N items."](../convention/2026-06-21-list-preamble-titled-reuses-source-bare-generates.md) — the rule that consumes this title signal.

## Revisit trigger

Revisit if the segmenter changes (or exposes the stripped first-item marker), if
goldmark's list handling changes, or if the documented false positive (first item
ending in a colon mis-promoted to a title) shows up in real input.
