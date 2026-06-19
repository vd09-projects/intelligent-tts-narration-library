# mcpsampling prompt templates stay inside the package for #13

- **Date:** 2026-06-20
- **Status:** experimental
- **Category:** convention
- **Tags:** intelligence, mcpsampling, prompts, templates, issue-13, issue-15, deferred-abstraction

## Context

#13 ships the first concrete `IntelligenceAdapter` — `intelligence/mcpsampling/`. The per-class prompt templates (`DefaultPromptTemplates` in `prompts.go`) are the LLM-side framing for each `plan.Class`. #15 (direct-API Anthropic adapter) will be the second adapter, and its issue body already mandates *"extract to shared `internal/intelligencetmpl` package to avoid drift."*

The question: ship the shared package up-front (in #13, before the second user exists) or keep templates inside the mcpsampling package and lift them when #15 lands?

## Decision

Keep `DefaultPromptTemplates` inside `intelligence/mcpsampling/prompts.go` for #13. When #15 lands, lift to `internal/intelligencetmpl/` (or `intelligence/intelligencetmpl/`) as the opening commit of that PR. The lift is a single-file move + import-rewrite — strictly smaller than co-designing two adapters in one PR.

## Justification

- Speculative abstraction has a cost: a shared package designed against one consumer almost always needs reshaping when the second consumer lands. Better to design the package against two real users than guess from one.
- The mcpsampling adapter is the first non-planner producer of LLM prompts in the project; the templates' shape will likely change between #13 and #15 (e.g., refusal sentinel format, max-tokens-by-level heuristic, system-prompt locale fixing). Locking the shape in a shared package early would freeze a decision based on insufficient information.
- The lift is mechanical: file move + import path rewrite + a deps_test.go entry. Estimated <1 hour of work, smaller than the up-front abstraction would cost in #13.
- The plan's risk table calls this out explicitly: *"Cache-key collisions for blocks that share content hash but differ in Facts — Acceptable for phase one"* establishes the project's tolerance for "good enough for phase one" decisions.

## Rejected alternatives

- **Ship `internal/intelligencetmpl` up-front in #13.** Adds a package + tests + interface surface to #13 with one consumer. Locks in the prompt shape before the second consumer's requirements are known. Premature abstraction.
- **Inline the templates in every adapter that needs them.** Duplication breeds drift — exactly the smell #15's issue body calls out. Rejected.

## Consequences

- `intelligence/mcpsampling/prompts.go` carries `DefaultPromptTemplates` for the lifetime of #13.
- #15 opens with a `refactor(intelligence): lift prompt templates to internal/intelligencetmpl` commit before its own implementation work.
- Until #15 lands, `intelligence/mcpsampling/` is the canonical source of truth for the prompt shape.
- The `WithPromptTemplates` override option (Phase 3) is the escape hatch if a caller needs to substitute templates without waiting for #15.

## Related decisions

- [2026-06-18-default-lexicon-frozen-per-class](../convention/) — the precedent: ship a frozen default + an override option, lift to shared if a second consumer materializes. (If that decision filename differs, the substantive precedent is the planner's `DefaultLexicon`.)
- This decision's amend candidate: when #15 lands, mark **superseded by `<2026-XX-XX-intelligencetmpl-shared-package>`** here.

## Revisit trigger

- When #15 (`intelligence/anthropic/`) opens. The lift commit goes in first, then this decision is superseded.
- If a third intelligence adapter is proposed before #15 lands — that pulls the lift forward.

## Source

Inline mark `**Decision (v1) — convention: experimental.**` in `planner-task.md v2` for scope `intelligence-mcpsampling-issue-13`. Implemented in commits `c254564` (Phase 1 scaffold) through `9161141` (Phase 5 wiring).
