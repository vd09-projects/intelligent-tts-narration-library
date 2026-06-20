# mcptext URI carries sha256(text); adapter cross-checks on Read

- **Date:** 2026-06-20
- **Status:** accepted
- **Category:** convention
- **Tags:** [adapter, mcptext, uri, sha256, content-hash, composition-root, issue-17]
- **Owner:** vd
- **Scope:** issue-17

## Context

The MCP `speak` tool's A16 spec accepts `{text | source}`. Ticket #12 shipped only the `source` path with a transient `errTextNotImplemented` sentinel; ticket #17 lands the in-memory `adapter/mcptext` and resolves `text` end-to-end. The planner is adapter-agnostic — it sees only a `RawDocument` + `plan.SourceRef` — but the SourceRef must carry provenance that distinguishes inline-text input from a file read so that downstream consumers (plan.json readers, caching, escalation) can reason about origin.

A file's URI is its absolute filesystem path — a natural identity. An inline-text payload has no equivalent natural identity. The question: what string goes in `SourceRef.URI` for inline text, and how do we catch a class of composition-root bugs where the caller computes the URI from one string and then constructs the adapter with a different string?

## Options considered

### Option A: URI = `mcp://inline/<sha256-hex>`; adapter cross-checks on Read (CHOSEN)
- **Pros**: URI carries provenance the planner can stamp into plan.json; the adapter detects wiring drift loudly at first call; no plan-schema change; no per-block hash required.
- **Cons**: One sha256 per Read (cheap — single-pass over the bytes the adapter already holds); the cross-check is a runtime cost paid for a wiring-bug safety net.

### Option B: URI = opaque token (`mcp://inline/<uuid>` or `mcp://inline/`) with no cross-check
- **Pros**: Simpler. No hash compute in the adapter.
- **Cons**: A composition-root bug where the URI was computed from one string and the adapter constructed with another goes undetected — the planner silently sees mismatched bytes vs. provenance. ContentHash on the returned RawDocument would still be sha256(text), but with no anchor anywhere asserting the URI's hex matches it.

### Option C: Per-block hash added to plan.Block schema; URI stays opaque
- **Pros**: Block-level staleness signal usable elsewhere (e.g. cmd/narrate's --expected-content-hash for per-block re-render).
- **Cons**: Speculative schema add for a 2-adapter problem (file + mcptext). The pipeline-block-rerender decision (2026-06-20) already pinned hash comparisons to the document-level `plan.Source.ContentHash` — adding a per-block hash would fork the model. Rejected.

## Decision

Composition root assembles the URI as `mcp://inline/<hex-sha256-of-text>`. The adapter exposes a `URIFor(text)` helper so the assembly has exactly one definition. On `Read`, the adapter computes `sha256(a.text)` and rejects the call if either:

1. The URI does not begin with `mcp://inline/` — wrong scheme.
2. The URI's hex suffix does not equal the computed hash — text-vs-URI drift.

Mismatch is a **terminal error** wrapped with the `mcptext adapter: ...` prefix, not a `Refusal`. The honesty-rule boundary applies to readable-but-unvoiceable source content; this is a wiring bug, not a document defect.

## Consequences

- Composition-root drift surfaces at the first Read, with a clear "uri hash mismatch" error message naming both the URI and the computed hash. Caller sees the bug instead of silently corrupted provenance.
- `URIFor` is exported so the composition root and any future caller share one URI-construction routine. The shape is canonical; changing it requires a `mcptext@<version>` bump (Source.Adapter field).
- Plan schema is unchanged — `SourceRef.URI` and `SourceRef.ContentHash` were already fields. No schema_version impact.
- One sha256 per Read. The adapter already needs the hash for `Source.ContentHash`; the cross-check reuses the same compute.

## Related decisions

- [text arg as transient sentinel — fast-error until ticket #17 lands](2026-06-19-text-arg-transient-sentinel.md) — superseded by this decision. The sentinel was always documented as transient (Decision v4 of `cmd/narrate-mcp/main.go`); this decision replaces it (Decision v6).
- [Pipeline block re-render uses document-level content_hash](../architecture/2026-06-20-pipeline-block-rerender-uses-document-hash.md) — establishes that hash comparisons target `plan.Source.ContentHash`. mcptext's URI hash is the same scheme applied to the URI suffix.
- [adapter offset-map line walker duplicated between file + mcptext; shared adapterutil deferred](2026-06-20-adapter-offsetmap-duplication-deferred-extraction.md) — companion decision for the offset-map duplication chosen during the same ticket.

## Revisit trigger

If a third inline-bytes adapter arrives (e.g. an in-memory clipboard adapter, an MCP-resource adapter, or any non-file source that needs identity), revisit whether the URI scheme + hash cross-check pattern should be lifted to a shared helper alongside the offset-map walker.

Also revisit if the planner ever loses determinism — same revisit trigger as the document-hash decision.
