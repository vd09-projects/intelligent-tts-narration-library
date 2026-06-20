# Player escalate UX is an inline expanded command card (not modal, not toast)

| Field    | Value            |
|----------|------------------|
| Date     | 2026-06-21       |
| Status   | accepted         |
| Category | convention       |
| Tags     | player, escalate, ux, accessibility, issue-18 |

## Context

Issue #18 requires per-block escalation: each block needs a "show me how to re-narrate this at L3" command surface. Phase one of the player does NOT do in-app re-render — instead it shows the literal CLI command the user should run on their host (`narrate --block <id> --level 3 --file <source.uri>`).

Three UX shapes for surfacing the command:

- **A — Modal dialog** on click.
- **B — Toast notification** on click.
- **C — Inline expanded card** under the block row.

## Options considered

### Option A: Modal dialog
- **Pros**: Familiar pattern; commands a lot of attention.
- **Cons**: Interrupts playback / scrubbing flow. Adds focus-trap accessibility surface. Demands a dismiss decision before the user can do anything else. Overweight for "here is a CLI string".

### Option B: Toast notification
- **Pros**: Lightweight; non-blocking.
- **Cons**: Disappears before the user can copy. Stacking semantics get awkward when multiple blocks escalate. Hard to attach a "Copy" button to a toast that respects screen readers.

### Option C: Inline expanded card under the block row
- **Pros**: No focus trap, no modal context — keyboard navigation stays natural. Stays open until dismissed or until another block is escalated. The "Copy" button and "Dismiss" button live in normal DOM tab order. Visually anchors the command to the block it escalates.
- **Cons**: Pushes content below the row down — the block list reflows when a card is open. Acceptable for a reference player.

## Decision

**Choose C.** Each non-refused `BlockRow` renders an "Escalate L3" button. Click sets `escalatedBlockId = block.id` (component-local state in `App`). The `BlockRow` checks this state; if `escalatedBlockId === block.id`, it renders `<EscalateCard />` directly below the row. The card shows the literal command in a `<code>` block, a "Copy" button using `navigator.clipboard.writeText`, a "Dismiss" button, and a footnote that in-app re-render is out of scope this phase.

Only one card is open at a time — escalating another block replaces the open card. Refused blocks do NOT show the Escalate button (escalating a refusal produces the same refusal; that's a future ticket).

## Consequences

- Inline reflow on expand. Acceptable visual cost; no scroll-anchor issues observed in manual testing.
- A11y is simple — no role="dialog", no focus management, no `aria-modal`. Just a normal expanded region.
- Future "in-app re-render" feature has a clean attach point (replace card body with a "Re-narrate" button + spinner) without re-architecting the UX.

## Related decisions

- This is the UI counterpart to the planner-side per-block escalation model (CLAUDE.md "Per-block leveling").
