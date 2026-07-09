# Grouped-decimal spans declined wholesale and left byte-identity; grouped-decimal voicing deferred, never half-implemented

| Field    | Value            |
|----------|------------------|
| Date     | 2026-07-10       |
| Status   | accepted         |
| Category | tradeoff         |
| Tags     | issue-139, planner, voicing, cardinal-spell-out, comma-grouped, grouped-decimal, commaGroupedRun, honesty-rule, never-partial, byte-identity, leave-as-digits, ambiguity-resolves-to-leave, leading-zero, build-review |

## Context

Build-review round 1 of #139 caught a value-altering PARTIAL render. A comma-grouped integer immediately followed by a decimal point (`1,234.5`) spelled the integer half — "one thousand two hundred thirty-four" — and orphaned the raw `.5`, producing "one thousand two hundred thirty-four.5". That mixes a spelled integer with a dangling raw fraction, altering the faithful value the reader hears and violating the honesty rule / never-partial faithful-value contract.

The question was whether to extend `commaGroupedRun` to also voice the fractional tail, or to decline the whole grouped-decimal span.

## Options considered

### Option A: Voice the grouped decimal too (`1,234.5` → "…point five")
- **Pros**: covers a real numeric form users write.
- **Cons**: expands scope mid-fix; a half-implemented version (integer spelled, fraction dropped or mis-joined) is exactly the value-altering partial that failed review. Getting it fully right is its own piece of work.

### Option B: Decline the grouped-decimal span wholesale, leave byte-identity
- **Pros**: the entire span falls through to the plain path and emits the original bytes verbatim (LEAVE) — a clunky-but-correct digit stream, never a partial. Keeps the fix small and provably faithful.
- **Cons**: `1,234.5` reads as raw digits until a future ticket adds proper grouped-decimal voicing.

## Decision

Chose **(b)**. `commaGroupedRun` DECLINES (`ok=false`) when the terminator after the last triple is a decimal point followed by a digit, so the entire grouped-decimal span falls through to the plain path and stays byte-identity (LEAVE). Grouped-decimal voicing (`1,234.5` → "one thousand two hundred thirty-four point five") is a candidate FUTURE ticket, not this one.

Also declined leading-zero first groups (`0,000`) so a format artifact never collapses to "zero".

Rationale: the never-partial fallback contract — emit the original bytes verbatim, never a partial or guessed rendering — is load-bearing. A half-spelled grouped decimal is worse than a clunky-but-correct digit stream. This extends the standing #138 rule "ambiguity resolves to LEAVE."

## Consequences

- `1,234.5` and similar grouped-decimal forms currently read as raw digits; correct grouped-decimal voicing is deferred to a future ticket, never half-shipped.
- The decline is a single guard in `commaGroupedRun`; no change to the plain path, which already emits byte-identity.
- Leading-zero grouped forms (`0,000`) are also declined, closing a "collapses to zero" artifact.

## Related decisions

- [Comma-grouped degroup handled as a step-0 pre-pass](../algorithm/2026-07-10-comma-grouped-degroup-prepass-not-tokenizer.md) — sibling #139 decision; this decline is a guard inside that pre-pass's `commaGroupedRun` validator.
- [Conservative "leave as digits" scope for cardinal spell-out](2026-07-10-conservative-leave-as-digits-cardinal-spell-out.md) — #138 sibling; establishes "ambiguity resolves to LEAVE", which this decision extends to grouped decimals.
- [Cardinal spell-out applies to the degradeProse no-intelligence path only](../architecture/2026-07-10-cardinals-degradeprose-no-intelligence-only.md) — #138 sibling; same path scoping.
- [voiceSpelled seam orders strip → spellNumbersInProse → lexicon-scan](../convention/2026-07-10-voicespelled-seam-strip-before-spell-before-lexicon.md) — #138 sibling; same `spellNumbersInProse` stage.

## Revisit trigger

If a follow-up ticket implements full grouped-decimal voicing (`1,234.5` → "…point five"), this decline is superseded for that form.
