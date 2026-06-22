# Over-long code-L2 reply: trim to first sentence, refuse only when the first sentence overruns

| Field    | Value            |
|----------|------------------|
| Date     | 2026-06-22       |
| Status   | accepted         |
| Category | tradeoff         |
| Tags     | code-l2, intelligence, honesty-rule, refuse-too-large, trim, planner, issue-60 |

## Context

Issue #60. The CodeUserL2 semantic gist (introduced by `2026-06-22-code-semantic-gist-l2-only`) asks the intelligence adapter for a one-sentence, ~30-word "what this code does" reply at L2. Adapters do not always honor the budget — a reply can come back multi-sentence or well over 30 words. Without enforcement, an over-long reply would be voiced verbatim, breaking the one-sentence gist contract and reading like prose instead of a gist.

The question: what does the planner do with an over-long code-L2 adapter reply? The honesty rule (non-negotiable: never fabricate) constrains the options — whatever is voiced must be the adapter's own words, honestly cut.

## Options considered

### Option A: Trim to first sentence; 30-word count as a hard ceiling that refuses only when the first sentence itself overruns
- **Pros**: Keeps voiceable honest content (the adapter's own first sentence, cut at a clean terminator seam — honest). The word count is a hard ceiling, not a mid-sentence chop. Refusal (`RefuseTooLarge`) fires only in the genuinely unsalvageable case where even the first sentence exceeds 30 words. Reuses an existing refuse sentinel.
- **Cons**: First-sentence scan needs careful seam detection (terminator + whitespace/EOF) to avoid splitting tokens like `v1.5.0`; accepts early-cut on abbreviations like `e.g.` as honest over-trimming.

### Option B: Reject the whole reply as a refusal whenever it exceeds the cap
- **Pros**: Simplest rule.
- **Cons**: Loses voiceable honest content unnecessarily — a reply whose first sentence is a perfectly good gist gets thrown away just because a second sentence followed.

### Option C: Truncate mid-sentence to fit the 30-word budget
- **Pros**: Always fits the word budget exactly.
- **Cons**: Rejected — mid-sentence truncation is a form of fabrication (it puts words in the adapter's mouth that form an incomplete/altered claim), violating the honesty rule.

## Decision

Chose **Option A**. For an over-long code-L2 adapter reply, TRIM to the first sentence — the adapter's own words, cut at a clean terminator seam, which is honest. The 30-word count is a HARD ceiling that triggers REFUSAL (`RefuseTooLarge`) only when even the first sentence overruns.

Rejected Option B because it discards voiceable honest content for no honesty gain. Rejected Option C because mid-sentence truncation is fabrication and violates the load-bearing honesty rule.

Enforcement lives at the single `callIntelligence` choke point in `planner/planner.go`, guarded by `ClassCode && L2`, reusing the existing `RefuseTooLarge` sentinel rather than minting a new one.

Sub-decision: the first-sentence scan requires whitespace or EOF after the terminator (keeps `v1.5.0` intact rather than splitting on the internal `.`) and accepts early-cut on abbreviations like `e.g.` as honest over-trimming (cutting too short is honest; the alternative — running the scan past a real sentence end — is worse). The trim uses a standalone helper, deliberately NOT routed through `splitProse`, because `splitProse`'s `proseMaxChars/2` size floor would defeat the trim.

## Consequences

- Code-L2 replies are now guaranteed one-sentence and within the word ceiling, or refused — the gist contract holds regardless of adapter behavior.
- Abbreviation-bearing first sentences (`e.g.`, `i.e.`) cut early; accepted as honest over-trimming.
- The trim helper is code-L2-specific and must stay off the `splitProse` path; a future refactor that consolidates them must preserve the no-size-floor behavior or the trim regresses.
- No new sentinel; `RefuseTooLarge` now carries this additional trigger condition.

## Related decisions

- [AI semantic gist for code at L2 only](2026-06-22-code-semantic-gist-l2-only.md) — extends it: this enforces the one-sentence/~30-word budget of the CodeUserL2 gist that decision introduced.
- [MCP sampling refuse sentinel token](../convention/2026-06-20-mcpsampling-refuse-sentinel-token.md) — applies the refuse-sentinel convention: reuses `RefuseTooLarge` rather than minting a new sentinel.

## Revisit trigger

If adapters reliably honor the one-sentence/30-word budget (making enforcement dead code), or if a second structured class needs the same first-sentence-trim behavior (justifying generalizing the standalone helper).
