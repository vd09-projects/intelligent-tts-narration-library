# IntelligenceAdapter interface lives in `intelligence/` package, not `planner/`

| Field    | Value            |
|----------|------------------|
| Date     | 2026-06-18       |
| Status   | accepted         |
| Category | architecture     |
| Tags     | intelligence, planner, interface, module-layout, phase-one |

## Context

The design doc §3.2 specifies `IntelligenceAdapter` as the narrow seam between the planner core and any LLM (MCP-sampling, Anthropic, etc.) that can comprehend a block at a requested level. The interface is the only thing about comprehension the planner needs to know.

Go has two conventional choices for interface placement:

- **Consumer-side**: the package that uses the interface declares it. The producer implements without importing.
- **Producer-side**: the package that defines the abstraction owns the interface; consumers import it.

The design doc explicitly lays out:

```
intelligence/        # PLUGGABLE COMPREHENSION (edge). Interface + impls.
├── intelligence.go  #   IntelligenceAdapter interface + request/result types
├── mcpsampling/     #   client-LLM-over-MCP                       [phase 4]
└── anthropic/       #   direct-API, user's own key                [phase 2/3, optional]
```

The question for the build was whether to honour the design-doc layout or to follow the more idiomatic "interfaces at the consumer" Go pattern by putting `IntelligenceAdapter` inside `planner/`.

## Options considered

### Option A: interface in `intelligence/` package (chosen, per design doc §3.2)
- **Pros**: matches the design-doc module layout; future concrete adapters (`intelligence/mcpsampling`, `intelligence/anthropic`) implement the interface from sibling subpackages without circular dependency; `intelligence/` is the natural import for any code that needs to inject or mock comprehension; `intelligence/deps_test.go` proves the package is interface-only (allowlist = `plan/`).
- **Cons**: planner takes a dependency on `intelligence/` for the type; small deviation from the Go consumer-side idiom.

### Option B: interface in `planner/`; impls in `intelligence/<x>`
- **Pros**: matches the typical Go convention.
- **Cons**: `intelligence/mcpsampling` would have to import `planner/` to see the interface, inverting the dependency direction from the design doc; concrete adapter packages would not be independently testable without pulling in the planner; the planner would re-export an interface that conceptually lives elsewhere.

### Option C: interface in a third dedicated package (e.g. `interfaces/intelligence.go`)
- **Pros**: neither planner nor intelligence owns it.
- **Cons**: design-doc divergence; extra package for no real benefit; over-engineered.

## Decision

`IntelligenceAdapter`, `IntelligenceRequest`, and `IntelligenceResult` live in `intelligence/intelligence.go`. The `planner/` package imports `intelligence/` for the interface type only — never for a concrete adapter. The composition root (`pipeline/`, `cmd/`) is the only place that knows which concrete adapter is in use.

This produces the dependency graph the design doc §1 mandates:

```
plan/  ← everything (zero deps)
intelligence/  → plan/                              (interface only, no I/O)
planner/  → plan/ + intelligence/                   (no I/O, no concrete adapters)
intelligence/mcpsampling/  → plan/ + intelligence/  (implements the interface)
intelligence/anthropic/    → plan/ + intelligence/  (implements the interface)
pipeline/, cmd/  → all concrete impls               (composition root)
```

`intelligence/deps_test.go` allowlist enforces that `intelligence/` itself imports only `plan/` — keeping the interface package pure and importable from anywhere without dependency surprise.

## Consequences

- The Go consumer-side idiom is bent slightly; in return the project gets a module layout that matches the design doc one-to-one. Future adapter authors find the interface where they expect it.
- The `planner/` package keeps a tiny dependency on `intelligence/` (interface only); the deps invariant test allows it because `intelligence/` is allowlisted as a planner dep.
- `scriptedIntel` mock for tests lives in `planner/planner_test.go` — under the `planner` package, so it never leaks into the production import graph. A future reusable test fake might be promoted to `intelligence/intelligencetest`.

## Related decisions

- [Plan zero-deps invariant enforced via `go list -deps` subprocess](../schema/2026-06-18-plan-zero-deps-via-go-list-subprocess.md) — same enforcement mechanism, applied to `intelligence/` so the interface package stays pure.

## Revisit trigger

- The interface gains a method that requires `planner/`-specific types (would force re-evaluation of where it belongs).
- Multi-package "interfaces collection" patterns gain traction in the codebase.
- A future generic comprehension layer becomes shared between planner and a non-planner consumer (e.g. an OCR caption layer) — re-evaluate whether the interface needs a more neutral home.
