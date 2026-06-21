# List preamble: titled list reuses the source title, bare list generates "List of N items."

| Field    | Value            |
|----------|------------------|
| Date     | 2026-06-21       |
| Status   | accepted         |
| Category | convention       |
| Tags     | list, preamble, honesty-rule, voicing, planner, ticket-45 |

## Context

Ticket #45 — when voicing a list, the planner emits a spoken preamble before the
items so the listener knows a list is starting and what it is about. Lists arrive
in two shapes: a list immediately preceded by a source title/label line, and a
bare list with no preceding label. The question was what the preamble should say
in each case, under the project honesty rule (never fabricate; reuse real source
labels rather than invent).

## Decision

A list with a preceding source title reuses that title as the spoken preamble,
with a trailing colon normalised to a period (e.g. source "Steps to deploy:" →
spoken preamble "Steps to deploy."). A bare list with no preceding title generates
a synthetic preamble "List of N items." where N is the item count.

Reasoning (honesty rule): when the source already supplies a real label, speak the
real label rather than fabricating a generic one — the spoken narration stays
faithful to what the source actually said. Only when there is genuinely no source
label do we fall back to a generated, factual, non-fabricated preamble ("List of N
items." states a true count, invents no semantic content).

## Consequences

- Spoken preamble for titled lists tracks the source wording verbatim (minus colon
  normalisation), so source edits flow through without planner-side rephrasing.
- The generated "List of N items." preamble is deliberately neutral and factual —
  it asserts only the count, never an invented topic.
- Depends on reliable title detection (see the colon-gated title detection
  decision) to decide which branch a list takes.

## Related decisions

- [Colon-gated list title detection under goldmark marker stripping](../tradeoff/2026-06-21-colon-gated-list-title-detection-goldmark.md) — supplies the titled-vs-bare signal this rule branches on.

## Revisit trigger

Revisit if the honesty-rule framing changes, or if a non-colon title signal
becomes available (then the titled branch could fire more often / more reliably).
