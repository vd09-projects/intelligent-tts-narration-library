# errclass imports intelligence/mcpsampling so all shared classification lives in one place (Option A)

> **SUPERSEDED (2026-06-22, issue #58).** The two `mcpsampling` sentinel branches this decision added to `errclass.Classify` (`ErrNoSamplingClient`, `ErrUnexpectedContentKind`) were discovered to be **DEAD**: both returned `ClassInternal`, which is identical to the default arm (`ClassInternal` is the iota zero value / fail-safe default). The branches therefore produced zero classification benefit, and the import `internal/errclass -> intelligence/mcpsampling` was pure cost. Issue #58 removed both branches and the import, restoring the "only `pipeline/` and `cmd/` know concrete backends" invariant.
>
> **New basis for supersession (NOT the pre-authorized trigger).** This is superseded on **dead-branch grounds**, NOT the originally-anticipated "cross-package import cycle or layering-lint flag" revisit trigger that this decision pre-authorized below. No cycle and no layering flag ever appeared; the branches were simply no-ops.
>
> **Relationship to the original Options.** The outcome coincides with the originally-**rejected Option B** for the two sentinels (no concrete adapter import in `errclass`) — but for a different reason than Option B contemplated: not to avoid a coupling cost, but because the branches were dead. Crucially, Option B's stated con (re-duplicating sampling-sentinel logic at the MCP root, leaving two classifiers) does **NOT** materialize here: there is nothing to re-handle, because the sentinels classify to `ClassInternal` via the default arm exactly as before. The original "all shared classification lives in one place" standing order therefore **still holds** — classification remains in one place; #58 removed only a no-op branch, not a classification fact.
>
> See: issue #58.

| Field    | Value            |
|----------|------------------|
| Date     | 2026-06-21       |
| Status   | superseded       |
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
- **Superseded by issue #58 (2026-06-22)** — the two sentinel branches were found to be dead (both `ClassInternal`, identical to the default arm). #58 removed the branches and the `errclass -> intelligence/mcpsampling` import. See the SUPERSEDED note at the top of this file.

## Revisit trigger

~~If a cross-package import cycle appears (e.g. `intelligence/mcpsampling` gaining an `internal/` dependency) or a layering rule starts flagging `errclass -> intelligence/mcpsampling`, fall back to Option B for the two sampling sentinels only.~~

**Moot — this trigger never fired.** Issue #58 superseded this decision on dead-branch grounds before any cycle or layering flag appeared. The import no longer exists, so there is nothing left to revisit on the original trigger.
