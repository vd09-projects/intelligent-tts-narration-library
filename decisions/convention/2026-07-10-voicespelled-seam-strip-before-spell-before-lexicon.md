# voiceSpelled seam orders strip → spellNumbersInProse → lexicon-scan

| Field    | Value            |
|----------|------------------|
| Date     | 2026-07-10       |
| Status   | accepted         |
| Category | convention       |
| Tags     | issue-138, planner, voicing, cardinal-spell-out, voiceSpelled, stripInlineMarkdown, spellNumbersInProse, scanLexicon, ordering-contract, pre-voice-transform, plan-review-regression |

## Context

Issue #138 adds a deterministic cardinal number spell-out pass to the planner's
no-intelligence verbatim prose path. The number pass must inspect source
adjacency (guarding out comma-grouped, `:`/`=`/`/`-adjacent, letter-adjacent
runs) and then hand voiceable text to the existing lexicon byte-scan.

The planner's `voice()` calls `stripInlineMarkdown()` as its very first internal
line. Running the number pass on *raw* (un-stripped) text was tried first and
produced a silent regression, caught in plan review round 1: a token like
`**24700**` was guarded correctly as digits by the number pass (it saw the `**`
adjacency), but was then stripped by `stripInlineMarkdown` down to a bare
`24700`, so the guard's protection was undone and a bare digit stream reached the
renderer. The number pass and the strip pass disagreed about what the "source"
adjacency was because they ran in the wrong order.

## Options considered

### Option A: Rely on stripInlineMarkdown idempotency (double-strip)
Run the number pass on raw text, and lean on `stripInlineMarkdown` being
idempotent so a second strip inside `voice()` is a no-op.
- **Pros**: no new seam; smallest diff.
- **Cons**: idempotency is an *implicit, untested* invariant — nothing names or
  guards it, and a future edit to `stripInlineMarkdown` could break it silently;
  the ordering contract lives nowhere in the code; the `**24700**` regression
  shows the number pass and the strip pass must agree on the same text, which
  double-strip does not guarantee.

### Option B: Explicit voiceSpelled seam with a fixed ordering
Introduce a `voiceSpelled` seam that runs on markdown-STRIPPED text and runs the
number pass BEFORE the lexicon byte-scan, sharing one `scanLexicon` body with
`voice()`.
- **Pros**: the seam *names* the ordering contract (strip → spellNumbersInProse →
  lexicon-scan) instead of leaving it implicit; `voice()` and `voiceSpelled()`
  share one `scanLexicon` body so they cannot drift; the number pass and the
  lexicon scan see the same stripped text the renderer will get.
- **Cons**: one extra seam function in the planner.

## Decision

Chose Option B. The number pass MUST run on markdown-stripped text and MUST run
before the lexicon byte-scan. This ordering is made explicit via a `voiceSpelled`
seam rather than being left to `stripInlineMarkdown` idempotency.

Reasoning: idempotency is an implicit, untested invariant — depending on it makes
correctness hinge on a property nothing guards. The seam names the ordering
contract in code, and by sharing a single `scanLexicon` body between `voice()`
and `voiceSpelled()`, the two entry points cannot silently diverge in how they
apply the lexicon. Running the number pass on stripped text also means it guards
against the *real* adjacency the renderer will see, not the pre-strip markdown.

## Consequences

- Any future pre-voice transform that inspects source adjacency must slot into
  the same seam, after strip and before the lexicon scan — this decision is the
  nearest precedent for that ordering question.
- One extra seam function is maintained in the planner; the shared `scanLexicon`
  body keeps `voice()`/`voiceSpelled()` in lockstep.

## Related decisions

- [Conservative "leave as digits" scope for cardinal spell-out](../tradeoff/2026-07-10-conservative-leave-as-digits-cardinal-spell-out.md) — the scope of the number pass this seam sequences.
- [Cardinal spell-out applies to the no-intelligence degradeProse path only](../architecture/2026-07-10-cardinals-degradeprose-no-intelligence-only.md) — the path this seam operates on.
- [DefaultLexicon shipped frozen + user-overridable via WithLexicon](2026-06-18-default-lexicon-shipped-frozen-overridable.md) — the lexicon byte-scan this pass runs before.

## Revisit trigger

Revisit if a second pre-voice transform needs a different position relative to
strip or the lexicon scan, or if `voice()` and `voiceSpelled()` ever need to
apply the lexicon differently (the shared `scanLexicon` body would then have to
split).
