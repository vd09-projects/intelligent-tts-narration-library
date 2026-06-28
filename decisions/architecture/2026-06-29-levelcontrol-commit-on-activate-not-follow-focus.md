# LevelControl commits on Space/Enter/click; arrows move roving focus only

| Field    | Value            |
|----------|------------------|
| Date     | 2026-06-29       |
| Status   | accepted         |
| Category | architecture     |
| Tags     | issue-113, earshot, accessibility, a11y, radiogroup, apg, roving-tabindex, commit-on-activate, manual-selection, segmentedtoggle-divergence, billable-renarration, wcag |

## Context

Issue #113's per-block L1/L2/L3 control is an APG `role=radiogroup` segmented control. The existing `earshot/src/components/SegmentedToggle.tsx` template uses **select-follows-focus** — arrowing changes the selection. But in `LevelControl`, each selection commit triggers a **billable re-narration** (a `POST /narrate/block` with model/intelligence spend on an unseen level). Mirroring `SegmentedToggle` would fire three escalations when a user simply arrows L1→L2→L3 to explore.

## Options considered

### Option A: select-follows-focus parity with SegmentedToggle
- **Pros**: consistent with the existing control; less code.
- **Cons**: bills a re-narration on every arrow keypress; arrowing through the group fires escalations the user never intended.

### Option B (chosen): manual selection — arrows move focus, commit is explicit
- **Pros**: selection becomes a deliberate act; no accidental billable escalation; APG Radio Group pattern explicitly permits the manual-selection variant.
- **Cons**: intentional divergence from the `SegmentedToggle` precedent — must be documented so it isn't "fixed" later.

## Decision

`LevelControl` is a `role=radiogroup` with three `role=radio` cells and roving tabindex (one Tab stop). **←/→ and ↑/↓ move roving focus ONLY — they do not select.** Selection is **commit-on-activate**: it happens on **Space/Enter or click** of the focused cell, never on arrow traversal. Exactly one `aria-checked=true` (the committed level), held across the async window and snapped back on failure. Focus ring is now meaningfully distinct from selection (focus ≠ commit). This is an intentional, documented divergence from `SegmentedToggle`'s select-follows-focus, justified because each commit is a billable re-narration so accidental selection on arrow traversal is a real cost, not a UX nit. APG-permitted (Radio Group manual-selection variant); WCAG 2.1 AA baseline preserved.

## Consequences

- Two visually similar Earshot radiogroups now have different keyboard semantics — load-bearing and must stay that way; a review test (`arrow-moves-focus-no-escalation`) locks it: arrows must not call `onCommit` or fire a network post.
- A `role=status` (polite) announces loading; a `role=alert` (assertive) carries an escalation-returned refusal.

## Related decisions

- [Earshot UI finalized — Material list-detail, APG-grounded controls](2026-06-28-earshot-ui-finalized-material-list-detail-bottom.md) — established the per-block L1/L2/L3 radiogroup (not a disclosure); this refines its keyboard model.
- [Earshot listener-UI mockup signed off](2026-06-28-earshot-mockup-signed-off-design-approved-for-111.md) — the 3-state level control accepted in the mockup.
