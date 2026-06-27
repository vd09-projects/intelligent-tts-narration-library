# Earshot UI finalized — Material list-detail, bottom transport, APG-grounded controls

| Field    | Value            |
|----------|------------------|
| Date     | 2026-06-28       |
| Status   | accepted         |
| Category | architecture     |
| Tags     | earshot, player-ui, react, list-detail, material-design-3, w3c-apg, accessibility, transport-bar, per-block-leveling, radiogroup, novel-synthesis, claude-artifacts, honesty-rule, quality-over-reuse, issue-108 |

## Context

Issue #108 asked to finalize the Earshot reference-player UI (`player/`) — produce a
production-grade layout + per-control interaction spec and record explicit approval before the
remaining UI build tickets. The existing player is a single-document fixture viewer (raw
`<audio controls>`, flat `BlockList`/`BlockRow`, inline `EscalateCard`, top-bar directory load).
The user finds it static, hard to use, and buggy, and wants a properly product-designed UI that
is *honestly better* than the current player — not a beautification pass over prior bad
decisions.

Researched via huginn (frame → fan-out → opinion → adversarial verify → design loop). All 7
load-bearing claims were adversarially re-verified and held. Full report:
`research/earshot-ui-design-108/report.md`.

## Options considered

### Option A: Material list-detail layout, bottom-anchored transport, APG-grounded controls (chosen)
- **Pros**: documented canonical layout (MD3); audio-first bottom-transport convention
  (Spotify/Podcasts/Speechify); every control maps to a verified W3C APG role; pairs with the
  block-level "return to playing block" control; honest three-state level control.
- **Cons**: transport anchor (bottom vs top) is a design call, not a verifiable fact; per-block
  3-level control is a novel synthesis (weakest-grounded surface).

### Option B: Beautify the existing player UI in place
- **Pros**: cheapest; reuses all current components.
- **Cons**: inherits the flagged bugs (dual `aria-hidden` mouse-only seek wrapper; up-only
  escalate that hides L1/L2/L3 states); anchors to prior bad decisions the user explicitly
  rejected.

### Option C: claude.ai "Claude Design" for the mockup (rejected)
- **Pros**: most polished mockup output.
- **Cons**: no free tier — cheapest qualifying plan is Claude Pro $20/mo; violates the
  no-recurring-spend constraint of this local-only hobby project.

## Decision

Finalize Earshot as a **Material list-detail layout**: left net-new **session pane**, center
**spoken transcript** (rebuilt `BlockList`/`BlockRow`), and a **bottom-anchored persistent
transport deck** (`role="toolbar"` + roving tabindex) replacing the raw `<audio controls>`.
Transport anchor decided = **bottom** (delegated design call) on audio-first convention, clean
scrolling reading column, pairing with the block-level "⟲ return to playing" control, and touch
reach — with an acknowledged tradeoff and a cheap one-CSS-region flip if user-testing disagrees.

Interaction is grounded in **verified W3C APG roles** (toolbar transport, slider scrubber —
block-quantized with `aria-valuetext`, listbox session list, inline escalate card — never
modal/toast). The per-block **L1/L2/L3 control is an accessible `radiogroup` segmented control**
(not a disclosure — a disclosure cannot honestly model the L2 *middle* state), designed as an
explicit **novel synthesis** with cited lineage (Shneiderman details-on-demand + screen-reader
verbosity levels + summarizer length sliders).

Mockup production method = **free Claude Artifacts** (interactive, downloadable React in the
target stack; only Live/AI/persistent artifacts are paid — a static mockup needs none).

Per the user's **quality-over-reuse** steer: reuse `usePlayback`/escalation **logic**, but
**rebuild** the UI for production quality — fixing the dual-seek and up-only-escalate bugs.

## Consequences

- Build follow-ups are consolidated into 3 bigger chunks (build shell + transport + rebuilt
  transcript; per-block L1/L2/L3 segmented control; clickable Artifact mockup + user-test).
- The per-block 3-level control carries the most design risk (bounded-negative grounding) and is
  explicitly flagged for user-testing.
- Honesty rule preserved in UI: refusals spoken + surfaced, degraded never shown as a real gist,
  stale chip with no auto-regenerate, escalate inline-only.
- If issue #107 deletes `player/`, "reuse" weakens — but the gap analysis still scopes which
  logic/patterns to port. Reuse was treated as not load-bearing.

## Related decisions

<!-- Links to other decisions that influenced or were influenced by this one -->

## Experiments

Adversarial verification (Stage 3): 8 blind claim-verifiers over 7 load-bearing claims; all
VERIFIED, none killed. Detail in `research/earshot-ui-design-108/_VERIFICATION.md`.

## Revisit trigger

Revisit the transport-anchor (bottom→top) and the L1/L2/L3 control pattern if the clickable
Artifact user-test shows the three-state control reads as unintuitive or the bottom deck
confuses users. Revisit reuse scope if issue #107 deletes `player/`.
