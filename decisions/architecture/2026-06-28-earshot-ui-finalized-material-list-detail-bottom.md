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

## Correction (2026-06-28, post-merge)

This decision's "reuse `usePlayback`/escalation logic, rebuild UI" framing assumed `player/` as
the substrate to finalize. That premise is **superseded by the prior same-day decision**
(`earshot-rebuild-server-driven-listener-ui`, referenced by #107/#109/#110/#111): Earshot is a
**server-driven rebuild** — `player/` is deleted (#107), a new `earshot/` web app talks to a Go
`narrate-server` HTTP bridge (#109) because the browser cannot run Kokoro. The **design
conclusions still hold** (Material list-detail, bottom transport, W3C APG roles,
radiogroup-not-disclosure, the BlockRow/seek bug fixes), but they apply to the new `earshot/`
app, not as client-side reuse of `player/`. The "reuse client-side components" point below is
therefore moot — #107 (delete `player/`) is **confirmed**, not a hedge.

The build follow-ups I created (#116/#117) were **duplicates** of the existing Earshot
decomposition (#111 scaffold, #112 playback engine, #113 leveling UI) and have been closed; the
design refinements were folded into #111/#112/#113. The only net-new follow-up is #115 (clickable
Artifact mockup / sign-off gate). #108 was the design gate feeding #111/#112/#113 — not a source
of new build tickets.

## Consequences

- Build follow-ups are NOT new tickets — #108's design enriches existing #111/#112/#113 (server-
  driven Earshot rebuild). The one net-new follow-up is #115 (clickable Artifact / sign-off gate).
- The per-block 3-level control carries the most design risk (bounded-negative grounding) and is
  explicitly flagged for user-testing.
- Honesty rule preserved in UI: refusals spoken + surfaced, degraded never shown as a real gist,
  stale chip with no auto-regenerate, escalate inline-only.
- #107 (delete `player/`) is confirmed by the established server-driven direction (see
  Correction above); the gap analysis serves as port-guidance only, not client-side reuse.

## Related decisions

<!-- Links to other decisions that influenced or were influenced by this one -->

## Experiments

Adversarial verification (Stage 3): 8 blind claim-verifiers over 7 load-bearing claims; all
VERIFIED, none killed. Detail in `research/earshot-ui-design-108/_VERIFICATION.md`.

## Revisit trigger

Revisit the transport-anchor (bottom→top) and the L1/L2/L3 control pattern if the clickable
Artifact user-test shows the three-state control reads as unintuitive or the bottom deck
confuses users. Revisit reuse scope if issue #107 deletes `player/`.
