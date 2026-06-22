# source.md failure modes pinned: 200->text, 404->null silent, all other failures->null + warning, never abort

| Field    | Value            |
|----------|------------------|
| Date     | 2026-06-22       |
| Status   | accepted         |
| Category | resilience       |
| Tags     | player, loadFromServerDir, source.md, honesty-rule, collectWarnings, source-null, graceful-fallback, optional-artifact, issue-70 |

## Context

The #70 load-by-path lib function `loadFromServerDir` fetches `{plan.json, manifest.json, audio.wav}` as required files plus `source.md` as an optional one. The brief left the `source.md` non-404 failure behavior as an open question (round-1 review item 2 demanded it be pinned now). The honesty rule says a missing required artifact must produce a precise error, while an absent or unreadable optional `source.md` must degrade gracefully (banner + per-block raw_excerpt) and never crash or 500.

## Decision

`source.md` is fetched separately from the three required files and handled tolerantly, with all failure modes pinned:

- **200** -> use the response text as the source.
- **404** -> `source: null`, silent (no warning) — absent source is a normal, expected state.
- **Any other outcome** (non-OK-non-404 status, network failure, timeout, abort) -> `source: null` PLUS a `collectWarnings`-style warning `"source unavailable: <status-or-reason>"`. Never throw, never abort.

The three required files keep the honest-error path: any `!res.ok` on plan/manifest/audio throws a precise per-file message and the load aborts. JSON parse failures on plan/manifest also throw precise per-file messages.

This is the honesty rule applied at the fetch layer: a present-but-unreadable source is surfaced as a warning (not silently swallowed, not crashed); an absent source degrades silently because absence is expected; required artifacts fail loud. Resolves the former Open Question 1.

## Consequences

- Player source pane already handles `source: null` -> banner + raw_excerpt, so no UI change is needed for the null case.
- The `source unavailable` warning rides the existing `warnings` array alongside schema warnings.
- Test matrix: source 404 -> null, no warning; source 500 -> null + warning, required files still load; missing required -> precise throw.

## Related decisions

- [Artifact allowlist widened to plan.json + source.md only](../security/2026-06-22-artifact-allowlist-widen-plan-source-name-agnostic-verified.md) — `source.md` absent flows the server's existing EvalSymlinks -> 404 path that this client-side handling depends on.

## Revisit trigger

If `source.md` ever becomes a required artifact, or if the player needs to distinguish source-unavailable reasons beyond a single warning string.
