# Earshot listener-UI mockup signed off — design approved for the #111 build

| Field    | Value            |
|----------|------------------|
| Date     | 2026-06-28       |
| Status   | accepted         |
| Category | architecture     |
| Tags     | earshot, mockup, sign-off, leveling-ui, transport-anchor, session-pane, file-pane, issue-115, issue-111 |

## Context

Issue #115 built a clickable, free Claude-Artifact React mockup
(`earshot-mockup/EarshotMockup.jsx`) as the sign-off gate before the finalized
Earshot listener UI is built (#111). The mockup made the two weakest-grounded
surfaces of the approved design clickable and user-testable:

- **Probe 1** — the per-block L1/L2/L3 segmented level control. This is a novel
  3-state synthesis and, under Model A, *is* the inline escalation surface (no
  standalone escalate card).
- **Probe 2** — the bottom-anchored transport, a delegated top-vs-bottom call.

The user exercised the mockup directly (served locally via `make preview-mockup`)
and ran both probes. During testing two affordances specified in the design
(`docs/earshot-design.md` §4) were found missing from the mockup and were fixed
inline rather than deferred (per the standing low-priority-inline order).

## Options considered

### Option A: Approve the design as-is (defaults held)
- **Pros**: Both probed surfaces read intuitively in the clickable mockup; no
  evidence to justify a redesign; unblocks #111 immediately.
- **Cons**: Commits to a novel 3-state control and a non-conventional bottom
  transport anchor on the strength of a single-user mock test.

### Option B: Flip the transport to a top anchor
- **Pros**: Top anchor is the more conventional placement.
- **Cons**: No confusion observed with the bottom anchor in testing; flipping
  would discard the deliberate finalized-design choice without cause.

### Option C: Replace the 3-state segmented level control
- **Pros**: Would remove the risk of a novel control reading ambiguously.
- **Cons**: The control read clearly in the mock; replacing it would lose the
  per-block leveling affordance that is the core of the product.

## Decision

**Approved as-is (Option A).** The user signed off on the Earshot design through
the #115 mockup. Specifically:

- **Probe 1 (L1/L2/L3 level control): accepted.** The per-block segmented
  control reads intuitively as a gist→detail fidelity ladder; the inline
  escalation model (Model A — the radiogroup is the escalation surface) is kept.
- **Probe 2 (transport anchor): accepted, no flip.** The bottom-anchored
  transport did not confuse; the bottom-anchor default holds.

Two gaps surfaced during testing were fixed inline in the mockup and are part of
the approved design surface for #111:

- **Session-ID entry** in the Session pane (design §4: "input a session ID →
  message list") — a typed/pasted session ID resolves against local transcripts;
  an unknown ID returns the honest glob-miss notice, never a fabricated session.
- **File pane** for use case 2 (design §2/§4: "drop a file → read out") — a
  `[Sessions | File]` source toggle with a drag-and-drop / pick dropzone, sharing
  the same leveling + transport, including oversized-section chunking on clean
  seams (design §6).

This sign-off is the gate that **unblocks #111** (the finalized `earshot/` build).
The mockup is a throwaway artifact: approved patterns are hand-ported into
`earshot/`, not wired in directly.

## Consequences

- #111 may proceed; the approved interaction patterns (3-state level control,
  bottom transport, session-ID entry, File pane) are the build target.
- The two probed surfaces are validated only by a single-user mock test, not by
  broader usability testing — acceptable for a local-only hobby project.
- The mockup's honest mock behaviours (glob-miss notice, "simulated" File-pane
  label) document the real-system limitations the #111 build must implement for
  real (narrate-server glob + `POST /narrate/file`).

## Related decisions

- [Earshot UI finalized — material list-detail, bottom transport](2026-06-28-earshot-ui-finalized-material-list-detail-bottom.md) — this sign-off validates that finalized design (the surface #115 mocked).
- [Rebuild the listener as a server-driven UI (Earshot); delete player/](2026-06-28-earshot-rebuild-server-driven-listener-ui.md) — the rebuild this UI sits on top of.
- [Resolve a session ID to a local transcript file by glob](2026-06-28-earshot-session-id-via-local-transcript-glob.md) — the model behind the session-ID entry added during sign-off.

## Revisit trigger

Reconsider if, during the #111 build, the 3-state level control or bottom
transport proves unworkable in the real (non-mock) app, or if broader usability
testing contradicts the single-user sign-off.
