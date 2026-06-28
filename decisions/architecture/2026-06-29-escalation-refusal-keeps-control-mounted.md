# An escalation that RETURNS a refusal keeps the control mounted (error path), distinct from an originally-refused block

| Field    | Value            |
|----------|------------------|
| Date     | 2026-06-29       |
| Status   | accepted         |
| Category | architecture     |
| Tags     | issue-113, earshot, refusal, honesty-rule, blockrow, levelcontrol, originally-refused, escalation-refusal, role-alert, error-path, hide-on-refused |

## Context

Issue #113's `LevelControl` is hidden on refused blocks per the AC. But "refused" conflates two cases: (1) an **originally-refused** document block — a block the first `/narrate` render returned as refused (e.g. a bare image); and (2) a **voiced/degraded block the user escalates, where the higher level comes back refused**. Treating both identically (unconditional hide-on-refused) would mean a user who escalates a voiced block into a refusal loses the control entirely and is stranded with a blanked block — unable to return to the level they could hear a moment ago.

## Options considered

### Option A (v1): unconditional hide-on-refused
- **Pros**: one rule.
- **Cons**: an escalation-returned refusal blanks the block and removes the control; the user loses the prior playable level and any way to retry.

### Option B (chosen): distinguish originally-refused from escalation-returned refusal
- **Pros**: originally-refused stays hidden (honors the AC); an escalation refusal is treated like the error path — control stays, prior level stays playable, refusal surfaced inline.
- **Cons**: slightly more branching in `BlockRow`.

## Decision

Restrict hide-on-refused to **originally-refused blocks only** (a block the document first rendered as refused — it never gets a control). An **escalation that RETURNS a refusal** is treated like the error path: do **not** flip the block to refused, keep `LevelControl` mounted, keep the prior committed level selected and **playable**, snap `aria-checked` back to the prior level, and surface the refusal inline via `role=alert` ("Block can't be voiced at L{n}; still at L{prior}"). Never blank the block. This keeps the user on the control to retry and never strands them. Rejected: v1's unconditional hide-on-refused.

## Consequences

- `BlockRow` branches on *origin* of the refusal, not just `block.status`; a review test (`escalate→refusal-keeps-control`) locks it — the control stays mounted at the prior level and renders the refusal via `role=alert`, block not blanked.
- Consistent with the honesty rule: the refusal is still surfaced (inline, assertive), just not as a block-blanking state transition.

## Related decisions

- [Earshot UI finalized — APG-grounded controls](2026-06-28-earshot-ui-finalized-material-list-detail-bottom.md) — established the radiogroup level control whose visibility this refines.
- [LevelControl commits on activate; arrows move focus only](2026-06-29-levelcontrol-commit-on-activate-not-follow-focus.md) — the `role=alert` region this refusal path drives.
