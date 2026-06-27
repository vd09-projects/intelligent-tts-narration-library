# Low-priority mid-build issues: fix inline or drop — never defer to new tickets

| Field    | Value            |
|----------|------------------|
| Date     | 2026-06-28       |
| Status   | accepted (standing order) |
| Category | process          |
| Tags     | process, workflow, review, backlog, standing-order |

## Context

Standing project philosophy for how to handle low-priority issues that surface
*mid-build* (e.g. during a build session or a code review on an in-flight PR).
The concern is backlog hygiene: small, non-blocking findings should not silently
spawn a long tail of follow-up tickets that never get done, nor should the
decision journal fill up with a record of every trivial deferral.

This became standing philosophy during the #105 `speak_to_file` build (PR #114).
Two non-blocking review suggestions — S1 (sink-arg leak) and S3 (mcptext
URI-scheme DUP) — were fixed inline in the same PR rather than spun out as
follow-up tickets.

## Decision

Low-priority issues surfaced mid-build are **fixed inline in the same PR** or
**dropped** — NOT deferred to new tickets.

Corollary guardrails:

- **Don't pick a tiny issue just so you can defer it.** The existence of a
  cheap-to-file ticket is not a reason to create one. If it's small enough to
  defer, it's usually small enough to either fix now or drop.
- **Don't log every trivial deferral in the decision journal either.** This
  single standing-order entry *is* the record. Individual trivial cases are not
  journaled — they are handled (fixed or dropped) and left at that.

## Consequences

- Backlog stays free of low-value follow-up tickets that would otherwise accrue
  and never be actioned.
- The decision journal stays signal-dense — no per-fix or per-deferral noise;
  this one entry stands in for the whole class.
- Tradeoff: a genuinely worthwhile-but-out-of-scope issue must clear a higher
  bar to become a ticket. That bar is intentional — "fix inline or drop" is the
  default, and spinning out a ticket is the exception requiring real
  justification (clearly worth doing AND genuinely cannot ride in this PR).

## Related decisions

<!-- none -->

## Revisit trigger

Revisit if mid-build findings start being non-trivially valuable often enough
that "fix inline or drop" is losing real work — i.e. if dropping is repeatedly
costing the project, the standing order may need a lightweight escalation path.
