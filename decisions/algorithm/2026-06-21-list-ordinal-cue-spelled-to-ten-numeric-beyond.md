# List item ordinal cue: spelled ordinals 1–10, numeric "item N" fallback beyond ten

| Field    | Value            |
|----------|------------------|
| Date     | 2026-06-21       |
| Status   | accepted         |
| Category | algorithm        |
| Tags     | list, ordinals, voicing, planner, ticket-45 |

## Context

Ticket #45 — speak list items with ordinal cues so a listener can track position
in a spoken list. The planner needs to emit a spoken ordinal prefix per list item
(e.g. "First, …", "Second, …"). The open question was how far to carry spelled
ordinals before they stop helping: spelling ordinals indefinitely ("Eleventh",
"Twenty-third", "One hundred and second") requires a general ordinal-spelling
engine and produces long, hard-to-track cues on long lists.

## Options considered

### Option A: Spelled ordinals for all items
- **Pros**: uniform cue style; reads naturally for small lists.
- **Cons**: needs a full ordinal-spelling engine (tens, hundreds, compound forms);
  long spelled ordinals on big lists hurt legibility and listener tracking; more
  surface area to get wrong.

### Option B: Spelled ordinals 1–10 from a frozen table, numeric "item N" beyond
- **Pros**: no ordinal-spelling engine — a fixed First..Tenth lookup table;
  long lists stay legible ("item 11", "item 12" …); bounded, auditable behavior.
- **Cons**: cue style is not uniform across the 10/11 boundary.

## Decision

Chose Option B. Items 1–10 use spelled ordinals from a frozen lookup table
(First, Second, … Tenth). Items 11 and beyond use a numeric fallback of the form
"item N" rather than spelled ordinals past ten.

Reasoning: deliberately avoids building and maintaining an ordinal-spelling engine,
and keeps long lists legible — a spoken "item 23" tracks better than "twenty-third"
when the listener is counting position. The 10-item table is small, frozen, and
trivially correct.

## Consequences

- Cue style intentionally changes shape at the 10→11 boundary (spelled → numeric).
- Adding spelled ordinals past ten later is a localized change (extend/replace the
  table or add a speller) with no schema impact.

## Revisit trigger

Revisit if user feedback shows the spelled/numeric boundary at 10 reads as
jarring, or if a general ordinal-spelling capability is introduced for another
reason (then the numeric fallback could be reconsidered).
