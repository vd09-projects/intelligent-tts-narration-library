# Cardinal spell-out applies to the degradeProse no-intelligence path only; adapter-voiced prose is out of scope

| Field    | Value            |
|----------|------------------|
| Date     | 2026-07-10       |
| Status   | accepted         |
| Category | architecture     |
| Tags     | issue-138, planner, voicing, cardinal-spell-out, degradeProse, no-intelligence-path, adapter-authoritative, no-double-processing, boundary |

## Context

Issue #138's deterministic cardinal spell-out could in principle run on any prose
the planner voices. But prose reaches spoken text via two distinct paths: the
no-intelligence verbatim fallback (`degradeProse`, `Status = degraded`, short
prose read verbatim), and the intelligence-adapter path (the adapter returns
voiced/summarized text). The question was whether the number pass should touch
both.

## Options considered

### Option A: Run cardinals on all voiced prose, including adapter output
- **Pros**: uniform number handling regardless of path.
- **Cons**: adapter output is authoritative spoken text; a second deterministic
  pass over it risks double-processing (re-spelling numbers the adapter already
  voiced correctly, or corrupting adapter-intended phrasing); the planner would
  be second-guessing the intelligence layer.

### Option B: Cardinals apply to the degradeProse no-intelligence path only
- **Pros**: adapter output stays authoritative and untouched; no
  double-processing risk; the deterministic pass owns exactly the path where no
  intelligence exists to voice numbers.
- **Cons**: number handling differs by path (deterministic vs
  adapter-determined) — accepted, since the adapter is trusted to voice its own
  output.

## Decision

Chose Option B. Cardinal spell-out runs ONLY on the `degradeProse`
no-intelligence verbatim path. Adapter-voiced prose is explicitly out of scope:
adapter output is authoritative, and a second deterministic pass over it would
risk double-processing.

Reasoning: the deterministic speller exists precisely to fill the gap where no
intelligence adapter is available to voice numbers. Where an adapter *is* present,
its output is the trusted final spoken text and the planner must not
re-transform it.

## Consequences

- Number handling is path-dependent: deterministic on the no-intelligence path,
  adapter-owned on the intelligence path. This is an accepted, intentional
  boundary, not an inconsistency to fix.
- Keeps the intelligence layer authoritative and the deterministic pass narrowly
  scoped.

## Related decisions

- [voiceSpelled seam orders strip → spellNumbersInProse → lexicon-scan](../convention/2026-07-10-voicespelled-seam-strip-before-spell-before-lexicon.md) — the seam this path routes through.
- [Conservative "leave as digits" scope for cardinal spell-out](../tradeoff/2026-07-10-conservative-leave-as-digits-cardinal-spell-out.md) — the digit scope applied on this path.

## Revisit trigger

Revisit if adapter-voiced prose is later found to emit bare digit streams the
adapter itself won't spell, such that a guarded deterministic post-pass over
adapter output becomes worth the double-processing risk.
