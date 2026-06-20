# --block patch into a persistent outDir: manifest is the INDEX, byte ranges are DERIVED

- **Date:** 2026-06-21
- **Status:** accepted
- **Category:** convention
- **Tags:** [sink/persistent, cmd, narrate, block-rerender, persistent-sink, manifest, route-a, honesty-rule, crash-consistency, issue-28]
- **Owner:** vd
- **Scope:** issue-28
- **Supersedes:** [--block X with --sink=persistent rejected at flag-validation](2026-06-20-block-with-persistent-sink-rejected-at-flag-time.md) (v1.9.0)

## Context

Issue #16 rejected `--block X --sink=persistent` at flag-validation (Decision v1.9.0) because routing a single-block render through the persistent sink's `Consume` would concatenate a one-block `audio.wav`, silently destroying the multi-block output. Issue #28 builds the patch path that makes the combination safe: re-render one block, splice its audio + manifest row into the existing outDir, every other block byte-preserved, all writes atomic. The honesty rule (refuse-don't-corrupt) stays load-bearing.

The central design question (AC #2): the manifest stores no byte offsets — how does the patch locate each block's bytes in `audio.wav`?

## Decision

**`manifest.json` is the authoritative block INDEX; per-block byte ranges in `audio.wav` are DERIVED, not stored (Route A).**

`PatchBlock` (a package-scope func in `sink/persistent`, mirroring the `CheckStale` precedent — not on the `OutputSink` interface, so `Consume` stays single-purpose) derives each block's byte range from `manifest.Blocks[i].StartMs/EndMs` + `AudioFormat` via the *same* `silenceBytes()` math `writeCombinedWAV` used to write the container. It harvests every non-target block's PCM from the existing `audio.wav` by that derived range and takes the target block's PCM fresh from the re-render.

The composition root (`cmd/narrate`) owns the branch — it detects `--block` + `--sink=persistent` + `--out`, captures the re-rendered block via a non-writing sink, and calls `PatchBlock` directly instead of letting the pipeline's `Consume` run. The sink owns the bytes.

### Two guarantees, INDEPENDENT (not a detect-and-recover pair)

- **F1 (input-guard):** before slicing, derive the expected `data`-chunk length from the manifest and compare it to the on-disk `data` length. Mismatch ⇒ `ErrContainerMismatch` refusal (exit 2); an unparseable WAV ⇒ hard error (exit 1). F1 guards the container the patch HARVESTS FROM. It also makes the (lossy, rounding-sensitive) timing→bytes derivation safe: any non-frame-aligned drift surfaces as an explicit refusal, never a silent mis-slice.
- **F2 (write-ordering):** stage all three tmp files, then commit by rename in the order `plan.json`, `manifest.json`, `audio.wav` LAST. The container the manifest indexes is the last thing to land.

F1 and F2 are **independent**. Crash-consistency of the OUTPUT is NOT carried by F1 detecting a half-committed state — it is carried by F2's bounded crash window PLUS the fact that a patch re-run always reconstructs and rewrites all three files, so any half-committed (new-manifest/old-audio) state is simply OVERWRITTEN by the next run. This closes the zero-delta hole: when the patched block re-renders to the same byte length but different PCM, a length check could not tell the interim state apart, but the re-run produces the correct final bytes regardless. (Earlier framing that "F1 is the recovery mechanism" was wrong and was removed during plan review F3.)

### Plan and manifest both stay multi-block and consistent

A patch reads the existing on-disk `plan.json` as the authoritative document plan and splices ONLY the target block's entry from the re-render (which, on the CLI path, is a one-block sub-plan); the same spliced target syncs the manifest block's `Class/Level/Status`. plan.json and manifest.json never truncate to one block and never disagree on the patched block's classification.

### `--expected-content-hash` stays OPTIONAL (Open Question #3, resolved)

The manifest `ContentHash` gate (plus `CheckStale`) already refuses cross-document patches without the flag. Requiring it would be redundant friction. The flag remains an optional belt-and-suspenders warning.

## Options considered

### Route A — derive byte ranges deterministically (CHOSEN)
- **Pros:** no manifest schema change (`ManifestSchemaVersion` stays 1); a derived offset re-runs the same math that wrote the file, so it cannot drift; idempotency falls out of the existing deterministic writer.
- **Cons:** the derivation is lossy for non-frame-aligned audio — mitigated by making F1 turn any disagreement into a refusal, not a mis-slice.

### Route B — store `byte_offset`/`byte_len` in `ManifestBlock` (REJECTED, retained as dormant fallback)
- **Pros:** explicit offsets, no derivation.
- **Cons:** a stored offset is a SECOND source of truth that can drift from the real container layout — a new, undetectable corruption class the honesty rule forbids. Only revived if a future requirement makes derivation impossible (Phase 2, dormant).

## Consequences

- `cmd/narrate --block <id> --sink=persistent --out <existing-dir>` now succeeds; the blanket flag refusal and its error string are removed (not bypassed).
- Stale / content-hash mismatch / unknown-block / container-mismatch / format-mismatch all refuse via exit 2; corrupt manifest / unreadable WAV → exit 1.
- The persistent-sink `audio.wav` invariant (every block in plan order, silence gaps) holds; only the target block's bytes change; non-target blocks are byte-identical.
- No new manifest field; consumers (React player, MCP) need no migration.

## Related decisions

- [Pipeline block re-render uses document-level content_hash](../architecture/2026-06-20-pipeline-block-rerender-uses-document-hash.md) — the hash semantics the patch's content-hash gate honors.
- [Persistent-sink atomic tmp+rename writes](2026-06-20-persistent-atomic-tmp-rename-writes.md) — the partial-write guard F2 extends to the three-file cross-write ordering.
- [Persistent-sink manifest carries no build timestamps](2026-06-20-persistent-manifest-no-build-timestamps.md) — what makes byte-deterministic idempotency provable.

## Revisit trigger

If a future requirement makes deterministic derivation impossible (e.g. variable-length container metadata between blocks), revive Route B (dormant Phase 2): add additive `byte_offset`/`byte_len` to `ManifestBlock`, keeping `ManifestSchemaVersion` additive.
