# Earshot transport deck exposes two keyboard tab stops, not one (APG toolbar + separate slider)

| Field    | Value            |
|----------|------------------|
| Date     | 2026-06-29       |
| Status   | accepted         |
| Category | architecture     |
| Tags     | issue-112, earshot, accessibility, a11y, apg, transport-deck, role-toolbar, roving-tabindex, role-slider, block-scrubber, tab-stop, stoppropagation, arrow-key-ownership, adr-108, playback-engine |

## Context

#108 finalized the Earshot transport as a `role=toolbar` button group with roving tabindex, with the original intent that the whole deck be a SINGLE composite keyboard tab stop. The #112 build hit a concrete conflict: the deck also contains the block scrubber, a `role=slider` whose value is changed with the arrow keys. A toolbar's roving handler also owns Left/Right (move between buttons). One composite tab stop cannot give the arrow keys two owners — toolbar-button-traversal vs slider-value-change collide.

## Options considered

### Option A: single composite tab stop for the whole deck (original #108 intent)
- **Pros**: one Tab lands on the entire transport.
- **Cons**: arrow keys are claimed by both the toolbar roving handler and the slider value; ambiguous ownership, not APG-conformant.

### Option B: two adjacent tab stops — toolbar + separate slider (chosen)
- **Pros**: APG-conformant; each control owns its own arrow keys cleanly.
- **Cons**: relaxes the #108 single-composite-stop goal; the deck now costs two Tab presses to traverse.

## Decision

The transport deck deliberately exposes TWO keyboard tab stops:

1. A `role=toolbar` button group as a single roving-tabindex stop — Left/Right move focus between transport buttons.
2. The `role=slider` block scrubber as its own SEPARATE, adjacent tab stop that owns its arrow keys and `stopPropagation()`s them so they never reach the toolbar roving handler.

The earlier "single composite tab stop for the whole deck" goal from #108 is explicitly relaxed. This is the APG-conformant resolution of the toolbar-arrows-vs-slider-value-arrows ownership conflict.

## Consequences

- Keyboard users Tab twice to cross the deck (toolbar, then scrubber) — accepted as the cost of unambiguous arrow-key ownership.
- The scrubber must `stopPropagation()` on its arrow keys; if that is removed, arrow presses would leak into the toolbar roving handler and move button focus instead of scrubbing.
- Extends/implements ADR #108 rather than overturning it — the toolbar + roving-tabindex pattern stands; only the single-composite-stop sub-goal is relaxed.

## Related decisions

- [Earshot UI finalized — Material list-detail, bottom transport, APG-grounded controls](2026-06-28-earshot-ui-finalized-material-list-detail-bottom.md) — #108, the parent that set the toolbar + roving-tabindex pattern and the original single-stop intent.
- [LevelControl commits on Space/Enter/click; arrows move roving focus only](2026-06-29-levelcontrol-commit-on-activate-not-follow-focus.md) — sibling APG roving-tabindex control with its own deliberate keyboard semantics.

## Revisit trigger

If user testing shows the two-Tab traversal is confusing, or if a future APG revision offers a single-stop pattern that resolves the arrow-ownership conflict, reconsider collapsing back toward one composite stop.
