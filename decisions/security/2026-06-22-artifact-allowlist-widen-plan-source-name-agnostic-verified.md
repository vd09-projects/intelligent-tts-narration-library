# Artifact allowlist widened to plan.json + source.md only; downstream name-agnosticism a verified precondition proven by symmetric traversal coverage

| Field    | Value            |
|----------|------------------|
| Date     | 2026-06-22       |
| Status   | accepted         |
| Category | security         |
| Tags     | cmd/narrate-server, artifact-route, allowlist, plan.json, source.md, path-traversal, EvalSymlinks, filepath-rel, loopback, CORS, name-agnosticism, symmetric-coverage, issue-70 |

## Context

Issue #70 lets the player load a persistent-sink output dir by typed absolute server path (no folder picker), which requires the escalate server's `GET /artifact` route to serve `plan.json` and `source.md` in addition to the existing `{audio.wav, manifest.json}`. The route's containment was established in #62 (allowlist-before-join + `filepath.EvalSymlinks` + `filepath.Rel` boundary check, `:533` separator/`..` guard, loopback-only bind, pinned CORS). Widening the allowlist is the single security-relevant change in #70 and the highest-risk item (security regression via allowlist drift).

A v1 mark asserted the downstream pipeline is name-agnostic as prose. Round-1 review rejected that as unbacked, producing a v2 that makes name-agnosticism a verified precondition with test backing. The v2 mark supersedes v1.

## Options considered

### Option A: Minimal-surface widening — add exactly two map entries, verify name-agnosticism first, prove with symmetric tests
- **Pros**: Containment pipeline stays byte-for-byte unchanged; no new `ErrorResponse.reason` token; the closed allowlist stays a closed (now four-name) set; symmetric traversal-reject coverage for both new names removes the asymmetric-coverage gap.
- **Cons**: Requires an explicit pre-edit verification gate (Phase-1 step 0) and new test rows for both names.

### Option B: Widen the map on the prose claim that the pipeline is name-agnostic (the v1 position)
- **Pros**: Less work; no explicit gate.
- **Cons**: The name-agnosticism claim is unbacked; asymmetric test coverage (traversal rows only for one name) leaves a real regression risk if a name-specific branch existed.

## Decision

Widen `artifactAllowlist` (main.go ~492-495) by exactly two entries: `"plan.json": "application/json"` and `"source.md": "text/markdown; charset=utf-8"`. Touch nothing else in the containment pipeline — the allowlist-before-join, `EvalSymlinks`, `filepath.Rel`, the `:533` separator/`..` guard, the per-dir mutex, the loopback-only bind, and the pinned CORS are all byte-for-byte unchanged. No new `ErrorResponse.reason` token (closed enum). Content-Type strings are pinned from the map (no MIME sniffing).

Name-agnosticism of every step after the allowlist lookup is a **verified precondition** (Phase-1 step 0: re-read `resolveArtifactPath` 599-642 and the `:533` guard, confirm in the review checklist no name-specific branch exists; STOP and escalate if any is found), **not** a prose assertion. It is **proven** by symmetric traversal-reject coverage for BOTH widened names: `../source.md`, `sub/source.md`, and `sub/plan.json` are added alongside the existing `../plan.json`, giving both names the identical reject net. `source.md` absent flows through the existing `EvalSymlinks` -> `ErrNotExist` -> source_not_found 404 path; it never introduces a 500.

This v2 decision supersedes the v1 security mark, whose reliance on the unbacked name-agnosticism claim and asymmetric coverage is closed here.

## Consequences

- The only security-review-relevant diff is two added map entries; the rest of the pipeline diff is empty.
- Full `artifact_test.go` security suite (Traversal, SiblingSharedPrefix, SymlinkEscape, AbsentArtifact/Dir, CORS, MethodNotAllowed) stays green with the same reason codes, now including the new symmetric rows.
- The closed allowlist becomes a closed four-name set; no directory listing, browse root, or arbitrary filenames.

## Related decisions

- [Read-only GET /artifact route serves the live escalated dir; player re-fetch resolves against it](../architecture/2026-06-22-artifact-route-serves-live-dir-resolves-refetch.md) — #62 established the containment pipeline this widening extends without modifying. Minimal-surface widening per that decision.
- [Loopback enforcement — refuse to start on non-loopback bind](../security/2026-06-21-loopback-enforcement-refuse-to-start-on-non-loopback-bind.md) — the loopback bind this widening rides unchanged.

## Revisit trigger

If a future change touches `resolveArtifactPath`, the `:533` guard, or any post-allowlist step in a name-specific way; or if the allowlist grows beyond the closed four-name set (re-verify name-agnosticism and re-establish symmetric coverage for any new name).
