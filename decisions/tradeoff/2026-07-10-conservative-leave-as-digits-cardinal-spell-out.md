# Conservative "leave as digits" scope for cardinal spell-out

| Field    | Value            |
|----------|------------------|
| Date     | 2026-07-10       |
| Status   | accepted         |
| Category | tradeoff         |
| Tags     | issue-138, planner, voicing, cardinal-spell-out, spellCardinal, leave-as-digits, conservative-scope, comma-grouped-deferred, negatives-deferred, ambiguity-resolves-to-leave |

## Context

Issue #138 adds deterministic cardinal number spell-out to the no-intelligence
verbatim prose path. The core tension: a *wrong* cardinal reading (mis-voicing a
version string, a ratio, an identifier, or a grouped number) is worse for a
listener than a clunky-but-correct digit stream. Many numeric-looking tokens are
ambiguous — is `-5` "minus five" or a hyphen-joined fragment? Is `24,700` one
number or two? — and the pass runs with no intelligence adapter to disambiguate.

## Options considered

### Option A: Spell everything numeric aggressively
Attempt to read comma-grouped numbers, negatives, decimals, and adjacent-form
numbers as cardinals.
- **Pros**: fewer bare digit streams reach the renderer.
- **Cons**: high risk of confidently-wrong readings (sign vs hyphen ambiguity,
  grouped-vs-separate, version/hex/identifier mis-reads); a wrong reading is
  worse than a correct digit stream; needs a much larger, riskier speller.

### Option B: Conservative scope — spell only unambiguous plain integers/decimals
Spell only plain standalone 1–6 digit integers and decimals; leave everything
ambiguous as digits.
- **Pros**: every spell-out is safe; ambiguity resolves to the honest fallback
  (leave as digits); small, auditable `spellCardinal`.
- **Cons**: some legitimately-spellable numbers (comma-grouped, negatives) are
  left as digits for now.

## Decision

Chose Option B. `spellCardinal` spells ONLY plain standalone 1–6 digit integers
and decimals. It leaves as digits: comma-grouped numbers (`24,700`), negatives
(`-5`, `-5` — the sign-vs-hyphen ambiguity is unresolved), dotted versions, hex,
runs of ≥7 digits, and any run adjacent to `:` / `=` / `/` / `-` or to a letter
or `_`.

Reasoning: a wrong cardinal reading is worse than a clunky-but-correct digit
stream, so every ambiguous case resolves to "leave as digits." This keeps the
speller small and every spell-out provably safe.

## Consequences

- Comma-grouped numbers and negatives are explicitly DEFERRED, not rejected — a
  follow-up ticket tracks comma-grouped support. They read as digits until then.
- The pass is conservative by design: expanding scope later means adding cases to
  a proven-safe base, never loosening a guard under pressure.
- Establishes the standing rule for this pass: ambiguity resolves to leave.

## Related decisions

- [voiceSpelled seam orders strip → spellNumbersInProse → lexicon-scan](../convention/2026-07-10-voicespelled-seam-strip-before-spell-before-lexicon.md) — the seam that sequences this pass; adjacency guards depend on running on stripped text.
- [Cardinal spell-out applies to the no-intelligence degradeProse path only](../architecture/2026-07-10-cardinals-degradeprose-no-intelligence-only.md) — the path scope.
- [List item ordinal cue: spelled 1–10, numeric beyond](../algorithm/2026-06-21-list-ordinal-cue-spelled-to-ten-numeric-beyond.md) — that decision deliberately avoided a general speller; this session ships `spellCardinal`, an accepted consistency debt to converge later.

## Revisit trigger

Revisit when the deferred comma-grouped follow-up ticket is picked up, or if the
digit-stream fallback on a common case (grouped numbers, negatives) proves to
read badly enough to justify the added mis-read risk.
