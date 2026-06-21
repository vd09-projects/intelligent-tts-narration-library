# errclass imports intelligence/mcpsampling so all shared classification lives in one place (Option A)

| Field    | Value            |
|----------|------------------|
| Date     | 2026-06-21       |
| Status   | accepted         |
| Category | architecture     |
| Tags     | internal/errclass, intelligence/mcpsampling, error-classification, import-coupling, layering, dedup, issue-51 |

## Context

Task #51 consolidated the duplicated caller-vs-internal-vs-cancel classification ladder (the `// DUP` marker in `cmd/narrate-server`, plus `classifyPipelineErr` in `cmd/narrate-mcp`) into `internal/errclass`. Two of the branches in that ladder match `intelligence/mcpsampling`'s adapter sentinels — `ErrNoSamplingClient` and `ErrUnexpectedContentKind` — and classify them as `ClassInternal`. The question: should the shared classifier KNOW about those two concrete adapter sentinels, or should the sampling-sentinel recognition stay at the MCP root?

## Options considered

### Option A: errclass imports intelligence/mcpsampling and recognizes the two sentinels (chosen)
- **Pros**: ALL shared classification lives in ONE place. Honors the prior decision that the sampling sentinels route to internal as a *classification* fact. No re-duplication; one classifier per root.
- **Cons**: A CLASSIFICATION package gains a latent coupling to a concrete adapter package (`errclass -> intelligence/mcpsampling`). The edge must be verified cycle-free and documented.

### Option B: errclass omits the sampling sentinels; MCP re-handles them locally
- **Pros**: `errclass` stays free of any adapter dependency.
- **Cons**: Re-duplicates the very sampling-sentinel logic #51 set out to consolidate, and leaves the MCP root with TWO classifiers (errclass + a local one). Rejected.

## Decision

**Chosen: Option A** — `errclass` imports `intelligence/mcpsampling` solely to recognize `ErrNoSamplingClient` and `ErrUnexpectedContentKind` and classify both as `ClassInternal`, keeping all shared classification in one place.

P1 verification (required before the rest of the package was written) confirmed:
- **No import cycle.** `intelligence/mcpsampling` imports only `plan/`, `intelligence/`, and the MCP SDK; it never reaches into `internal/`. The edge `errclass -> mcpsampling` is one-directional.
- **No layering-lint flag** on the import.

The latent coupling is documented as a **named edge** in the `errclass.go` package doc comment, so future maintainers find `errclass -> intelligence/mcpsampling` understood rather than discovered. The fallback (Option B for the two sampling sentinels only, recording the deviation) was held in reserve for a real cycle/layering problem; none surfaced.

## Consequences

- A classification package knowingly depends on one concrete adapter package. This is acceptable because the dependency is a single direction, cycle-free, and documented.
- If `intelligence/mcpsampling` ever needs to import anything under `internal/`, this edge becomes a cycle and Option B becomes mandatory for the two sentinels.

## Related decisions

- [MCP error classifier caller-vs-internal split](../convention/2026-06-19-mcp-error-classifier-caller-vs-internal-split.md) — the original per-root split that #51 consolidates into errclass; this decision is the consolidation's architectural commitment.

## Revisit trigger

If a cross-package import cycle appears (e.g. `intelligence/mcpsampling` gaining an `internal/` dependency) or a layering rule starts flagging `errclass -> intelligence/mcpsampling`, fall back to Option B for the two sampling sentinels only.
