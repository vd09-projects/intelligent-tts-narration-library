# Persistent-sink manifest carries no build timestamp

- **Date:** 2026-06-20
- **Status:** accepted
- **Category:** convention
- **Tags:** [sink, persistent, manifest, idempotency, determinism, issue-16]
- **Owner:** vd
- **Scope:** issue-16

## Context

`sink/persistent` writes a `manifest.json` next to `audio.wav` and `plan.json`. The acceptance criterion for issue #16 includes "idempotent re-write to same dir" — re-running `Consume` on unchanged inputs must produce byte-identical outputs for all three files. A `built_at` or `created_at` timestamp in the manifest would break that property.

`plan.json` is already timestamp-free at write time (the `NarrationPlan.CreatedAt` field is set by the planner and preserved verbatim).

## Options considered

### Option A: No build timestamp anywhere in the manifest (CHOSEN)
- **Pros**: Idempotent rewrite is trivially provable — same input bytes go in, same output bytes come out. CI/diff tooling can compare two persistent-sink runs byte-for-byte. The manifest stays minimal.
- **Cons**: Operators who want "when was this written?" must fall back to filesystem mtime.

### Option B: Optional timestamp gated by an environment variable
- **Pros**: Covers the "when was this rendered?" use case for callers who need it.
- **Cons**: Speculative — no concrete use case yet. Two code paths (timestamped vs not) means two test matrices. The env-var gate is the kind of seam that grows surprising default behavior.

### Option C: Always-on timestamp, fail the idempotency AC
- **Pros**: Simple.
- **Cons**: Breaks a load-bearing acceptance criterion.

## Decision

The `Manifest` struct contains no field for build/render timestamp. `Consume` produces byte-identical output bytes for identical input bytes, including `manifest.json`. The pinned idempotency test (`TestConsume_IdempotentRewrite`) asserts byte-equality on all three output files across two consecutive `Consume` calls.

If a future caller needs "when was this rendered?", they can read the filesystem mtime (`os.Stat(outDir+"/manifest.json").ModTime()`). The sink does not encode that data into the JSON.

## Consequences

- The byte-identical-rewrite guarantee is a usable property: tooling can decide "should I re-run?" by comparing a candidate fresh run's output to the persisted output. Today only `CheckStale` exposes that comparison via content_hash; future tooling could compare manifest bytes directly.
- No env-var seam grows. Behavior is predictable.
- The manifest field set stays minimal, lowering the surface area for `ManifestSchemaVersion` bumps.

## Related decisions

- [Persistent-sink ManifestSchemaVersion starts at 1 additive](../schema/2026-06-20-persistent-manifest-schema-version-additive.md) — companion schema decision.
- [Persistent-sink Consume + manifest writes are atomic tmp+rename](2026-06-20-persistent-atomic-tmp-rename-writes.md) — the other half of the "no partial state on disk" guarantee.

## Revisit trigger

If multiple callers independently end up wrapping `Consume` with their own timestamp-recording layer, lift the timestamp into the manifest as an optional field (still default-off, still gated, so the byte-identical guarantee persists for callers who don't opt in).
