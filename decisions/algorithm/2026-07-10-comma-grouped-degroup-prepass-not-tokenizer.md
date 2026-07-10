# Comma-grouped degroup handled as a step-0 pre-pass, not by extending the maximal-run tokenizer

| Field    | Value            |
|----------|------------------|
| Date     | 2026-07-10       |
| Status   | accepted         |
| Category | algorithm        |
| Tags     | issue-139, planner, voicing, cardinal-spell-out, comma-grouped, spellNumbersInProse, commaGroupedRun, spellableRun, maximal-run-tokenizer, pre-pass, degroup |

## Context

#139 needed to voice well-formed US comma-grouped integers (`24,700` → "twenty-four thousand seven hundred") that #138 deliberately left as digits (see the conservative "leave as digits" scope decision). The cardinal spell-out lives in `spellNumbersInProse` on the `degradeProse` no-intelligence verbatim path.

Two implementation sites were possible for adding comma-grouped support:

- **(a)** Teach the existing maximal-run tokenizer loop to swallow structural commas as it scans, so a comma-grouped span reads as one run inside the same scan that already handles plain digit runs.
- **(b)** Add a dedicated comma-grouped detect-and-degroup pre-pass at the head of digit-run handling — a "step 0" that runs BEFORE the plain scan.

## Options considered

### Option A: Extend the maximal-run tokenizer to swallow commas
- **Pros**: single scanning loop; no second pass over the text.
- **Cons**: couples the comma-vs-punctuation judgment into the same loop that already carries the decimal-dot lookahead — one `switch` now has to disambiguate "structural grouping comma" from "sentence comma" while simultaneously deciding decimal-dot adjacency. Entangles two different comma judgments (grouping vs. punctuation) with the decimal handling in one place.

### Option B: Dedicated comma-grouped detect+degroup pre-pass (step 0)
- **Pros**: keeps the maximal-run scanning loop and the `spellableRun` guard predicate uncoupled from comma handling; reuses existing safety gates unchanged; malformed spans fall through cleanly to the untouched plain path.
- **Cons**: a second inspection of the span head before the plain scan (negligible cost).

## Decision

Chose **(b)**, the step-0 pre-pass.

A new pure validator `commaGroupedRun` matches `\d{1,3}(,\d{3})+`. The pre-pass strips the commas to a clean digit string and then REUSES the existing safety machinery rather than reinventing it:

- the existing `spellableRun` neighbour gate, anchored on the grouped span's **start**;
- the 1–6 length gate, applied to the **degrouped** digit count;
- `spellNumberToken` to render the degrouped integer.

Any malformed, oversized, or bad-neighbour span falls through to the unchanged plain path and is LEFT as digits. The `>=7`-digit LEAVE applies naturally to the degrouped count (`1,000,000` → LEAVE, because degrouped it is 7 digits). The retained plain-path comma-far-digit rejection inside `spellableRun` remains the load-bearing partial-spell guard for malformed spans like `12,3456`.

Rejected **(a)** because it couples two different comma judgments (grouping comma vs. punctuation comma) into one loop and entangles that call with the decimal-dot lookahead already living in the same switch.

## Consequences

- The scanning loop and the `spellableRun` predicate stay comma-agnostic; comma logic is isolated in `commaGroupedRun` and the pre-pass.
- Reuse of the existing neighbour + length gates means the safety envelope for grouped integers is provably the same as for plain integers — no parallel, drift-prone guard.
- A second pass over the span head is accepted as negligible cost for the decoupling gained.

## Related decisions

- [Conservative "leave as digits" scope for cardinal spell-out](../tradeoff/2026-07-10-conservative-leave-as-digits-cardinal-spell-out.md) — #138 sibling; deferred comma-grouped as a follow-up, which this decision implements.
- [voiceSpelled seam orders strip → spellNumbersInProse → lexicon-scan](../convention/2026-07-10-voicespelled-seam-strip-before-spell-before-lexicon.md) — #138 sibling; the pre-pass runs inside the same `spellNumbersInProse` stage this seam orders.
- [Cardinal spell-out applies to the degradeProse no-intelligence path only](../architecture/2026-07-10-cardinals-degradeprose-no-intelligence-only.md) — #138 sibling; the comma-grouped pre-pass inherits the same path-scoping.
- [Grouped-decimal spans declined wholesale and left byte-identity](../tradeoff/2026-07-10-grouped-decimal-declined-wholesale-left-byte-identity.md) — sibling #139 decision; the never-partial fallback that bounds this pre-pass's scope.
