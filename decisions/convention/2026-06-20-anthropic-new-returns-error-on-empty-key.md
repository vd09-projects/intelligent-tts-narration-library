# anthropic.New returns error on empty API key

- **Date:** 2026-06-20
- **Status:** accepted
- **Category:** convention
- **Tags:** [intelligence, anthropic, constructor, error-handling, issue-15]
- **Owner:** vd
- **Scope:** issue-15

## Context

mcpsampling's `New(opts...)` returns `*Adapter` with no error — the constructor has no validatable fields (the MCP client is supplied per-call, not at construction). anthropic.New is different: an empty `apiKey` would surface as a 401 deep inside `Voice()` at first call, not as a misconfiguration at construction.

## Options considered

### Option A: `New(opts...) (*Adapter, error)` — return error on empty key (CHOSEN)
- **Pros**: Misconfiguration surfaces at construction. Composition root (cmd/narrate) handles the error or treats it as a programmer bug. Matches Go convention for validating-constructor functions.
- **Cons**: Different shape from mcpsampling.New. Callers have to handle the error.

### Option B: Panic on empty key
- **Pros**: Surfaces loudly.
- **Cons**: Panics are for programmer errors, not user-config errors. An empty env var is user-correctable.

### Option C: Silently accept empty key, fail at first API call
- **Pros**: Matches mcpsampling shape.
- **Cons**: Hides misconfiguration. The 401 surfaces in a stack frame far from the cause.

## Decision

`anthropic.New(opts ...Option) (*Adapter, error)`. Returns `errors.New("anthropic: WithAPIKey is required (got empty key)")` when `apiKey` is empty after applying opts. Also rejects empty `model` (defensive — defaults populate it, so this only fires if `WithModel("")` is explicitly called).

cmd/narrate's `chooseIntelligence` treats a non-nil error here as a programmer bug (panic with a clear message) because validate() already enforces the env var is set before construction.

## Consequences

- intelligence/anthropic/anthropic.go's New differs in shape from mcpsampling/mcpsampling.go's New.
- chooseIntelligence in cmd/narrate panics if construction fails after validation succeeded — a contract violation, not a user-correctable case.
- The compile-time interface assertion (`var _ intelligence.IntelligenceAdapter = (*Adapter)(nil)`) is unaffected by the constructor shape.

## Related decisions

- [--intelligence anthropic with missing env is a flag-validation error](2026-06-20-intelligence-anthropic-missing-env-is-flag-error.md) — the validate-before-construct contract this decision relies on.

## Revisit trigger

If mcpsampling.New gains validatable fields, align the two constructor shapes. Otherwise the asymmetry is intentional.
