# Persistent-sink ManifestSchemaVersion starts at 1, additive-compatible

- **Date:** 2026-06-20
- **Status:** accepted
- **Category:** schema
- **Tags:** [sink, persistent, manifest, schema-version, additive-compatibility, issue-16]
- **Owner:** vd
- **Scope:** issue-16

## Context

`sink/persistent/manifest.json` is a new long-lived artifact that callers (the React reference player, future MCP tooling, the user's terminal `cat`) will parse. It needs a versioning convention so consumers can detect schema drift and refuse outputs they don't understand.

The `plan/` package already uses a `SchemaVersion = "1.0"` constant under additive-compatibility-within-major rules. The manifest could mirror that semver-shape or pick its own.

## Options considered

### Option A: `const ManifestSchemaVersion = 1` (int), additive-compatible (CHOSEN)
- **Pros**: One source of truth, code-grep-able. A test pins the const value so accidental bumps surface in review. Mirrors the plan-schema convention. Integers are cheap to JSON-encode without ambiguity.
- **Cons**: Versioning a not-yet-shared artifact is speculative-ish — but the migration cost when it's needed is much higher than the cost of carrying the field today.

### Option B: SchemaVersion-as-method (e.g. `Manifest.SchemaVersion() int`)
- **Pros**: Methods can encode logic.
- **Cons**: Over-engineered. The version is data, not behavior.

### Option C: Tag the manifest with a free-form version string
- **Pros**: Permissive.
- **Cons**: Permissive in the wrong direction — consumers can't reliably compare versions; downstream code paths can't enum-switch on it.

## Decision

`ManifestSchemaVersion` is a package-level `const int = 1`. Field additions to the `Manifest` struct are backward-compatible within the major version: JSON unmarshallers ignore unknown fields per the project's broader additive-compatibility convention. Removing or renaming a field requires a major bump (`ManifestSchemaVersion = 2`).

A test (`TestManifest_SchemaVersionIsOne`) pins the current value so an accidental bump triggers review attention.

## Consequences

- A consumer that reads a future `Manifest{SchemaVersion: 1, ...}` with new optional fields it doesn't know about will still parse successfully.
- A consumer reading `Manifest{SchemaVersion: 2, ...}` should refuse — though the refusal logic lives in the consumer, not in the sink. The sink only writes the current major's manifests.
- The const becomes a load-bearing review surface: bumping it from 1 to 2 is a code change reviewers will flag.

## Related decisions

- [plan_schema_version is "1.0" additive-compatible](2026-06-18-plan-zero-deps-via-go-list-subprocess.md) — same versioning convention applied to the plan schema (informal — there's no standalone decision file for plan SchemaVersion yet; CLAUDE.md captures the rule).

## Revisit trigger

When `ManifestSchemaVersion` first needs to advance (a field rename, a field removal, or a semantic shift in an existing field's meaning), revisit whether to introduce migration tooling or to keep the "consumers refuse unknown majors" stance.
