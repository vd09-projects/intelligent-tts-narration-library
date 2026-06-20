# Refusal-parser duplicate-not-lift between mcpsampling and anthropic

- **Date:** 2026-06-20
- **Status:** experimental
- **Category:** convention
- **Tags:** [intelligence, anthropic, mcpsampling, refusal, code-reuse, issue-15]
- **Owner:** vd
- **Scope:** issue-15

## Context

Issue #15 lands the second concrete `intelligence.IntelligenceAdapter` (intelligence/anthropic), alongside the existing intelligence/mcpsampling. Both adapters use the same refusal contract — the LLM emits `__REFUSE__ <note>` as the literal first non-whitespace text of the assistant reply. Both parse it with a 10-line function that strips the sentinel and returns `(note, refused)`.

The natural temptation: lift `refuseSentinel` const + `parseRefusal` function into `internal/intelligencetmpl/` (already lifted in Phase 1 for prompts). But the system-prompt contract test in mcpsampling asserts the sentinel appears in every prompt — lifting also requires lifting that test, doubling surface area for the opening commit.

## Options considered

### Option A: Duplicate the 10-line parser (CHOSEN)
- **Pros**: Each adapter is self-contained for refusal parsing. No new shared surface to coordinate. The duplication is small enough that future drift is caught by grep + deps_test.
- **Cons**: ~15 LOC of accepted duplication. If a third adapter materializes the duplication grows.

### Option B: Lift to `internal/intelligencetmpl/`
- **Pros**: Single source of truth for the refusal contract.
- **Cons**: Also requires lifting the contract test that asserts the sentinel appears in every prompt — too much surface area for the same PR that introduces the second adapter. Couples the lift to the system-prompt machinery.

## Decision

Keep the parser duplicated. Generalize when a 3rd adapter materializes — the "two consumers before lift" principle.

## Consequences

- intelligence/anthropic/refusal.go is a near-verbatim copy of mcpsampling's parser. Future edits to the contract require updating both.
- Mitigation: a grep-based audit (or a manual `make sanity` check before each release) catches divergence.
- The lift becomes a follow-up ticket when a 3rd adapter ships.

## Related decisions

- [Cache machinery duplicate-not-lift between mcpsampling and anthropic](2026-06-20-cache-machinery-duplicate-not-lift.md) — same principle applied to cache machinery.

## Revisit trigger

When a 3rd intelligence adapter is proposed (e.g., OpenAI, Bedrock).
