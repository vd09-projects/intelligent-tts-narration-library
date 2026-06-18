# Decision Index

<!--
  This file is maintained by the decision-journal skill.
  Entries are in YAML format for machine-friendly querying.
  Newest entries go at the top. Do not manually reorder.
-->

```yaml
decisions:
  - id: 2026-06-18-intelligence-adapter-lives-in-intelligence-pkg
    title: "IntelligenceAdapter interface lives in `intelligence/` package, not `planner/`"
    date: 2026-06-18
    status: accepted
    category: architecture
    tags: [intelligence, planner, interface, module-layout, phase-one]
    path: architecture/2026-06-18-intelligence-adapter-lives-in-intelligence-pkg.md
    summary: "Place IntelligenceAdapter interface in intelligence/ per design doc §3.2; planner depends on it; future concrete adapters (mcpsampling, anthropic) implement from sibling subpackages without circular deps; intelligence/ allowlist is only plan/."

  - id: 2026-06-18-planner-deps-invariant-checks-direct-imports
    title: "Planner deps invariant checks direct .Imports, not transitive -deps"
    date: 2026-06-18
    status: accepted
    category: tradeoff
    tags: [planner, invariant, dependencies, goldmark, testing, phase-one]
    path: tradeoff/2026-06-18-planner-deps-invariant-checks-direct-imports.md
    summary: "Scope planner/'s no-IO invariant test to direct .Imports because goldmark (sanctioned segmenter) transitively pulls in os/syscall/net/url; the CLAUDE.md invariant is about source-file imports, not unavoidable transitive deps."

  - id: 2026-06-18-two-oversized-split-thresholds
    title: "Two oversized-split thresholds: prose vs structured"
    date: 2026-06-18
    status: accepted
    category: convention
    tags: [planner, level, oversized-split, phase-one, heuristics]
    path: convention/2026-06-18-two-oversized-split-thresholds.md
    summary: "Encode separate constants — prose 20 lines / 800 chars; structured 70 lines / 2500 chars — because prose audio-coherence is the binding constraint for prose while structured content tolerates longer runs (clean seams). Diagrams are intentionally not split."

  - id: 2026-06-18-default-lexicon-shipped-frozen-overridable
    title: "DefaultLexicon shipped frozen + user-overridable via WithLexicon"
    date: 2026-06-18
    status: accepted
    category: convention
    tags: [planner, voice, lexicon, phase-one, user-override]
    path: convention/2026-06-18-default-lexicon-shipped-frozen-overridable.md
    summary: "Ship an opinionated baseline lexicon (arrows, paths, dev acronyms) plus a WithLexicon(extra) overlay so user entries win on key collision; empty default would produce silent silly voicings like '->' as 'dash greater than'."

  - id: 2026-06-18-segmenter-walks-top-level-ast-only
    title: "Segmenter walks top-level AST children only, not the full tree"
    date: 2026-06-18
    status: accepted
    category: tradeoff
    tags: [planner, segmentation, goldmark, source-map, phase-one]
    path: tradeoff/2026-06-18-segmenter-walks-top-level-ast-only.md
    summary: "Walk goldmark's top-level children only so plan.SourceMap line ranges stay contiguous and non-overlapping — block-level sync is the load-bearing invariant. Tradeoff: a fenced code block nested in a blockquote folds into the blockquote's text."

  - id: 2026-06-18-planner-classifier-sniff-order
    title: "Planner classifier — deterministic priority sniff order"
    date: 2026-06-18
    status: accepted
    category: convention
    tags: [planner, classify, segmentation, determinism, phase-one]
    path: convention/2026-06-18-planner-classifier-sniff-order.md
    summary: "Apply 9 rules first-match-wins: heading → list → image → table → fenced-code-subtype → plaintext pipe-table → plaintext config sniff → ASCII diagram → prose default. ClassExample reserved for future intelligence-driven reclassification. Each rule has a named test case."

  - id: 2026-06-18-plan-zero-deps-via-go-list-subprocess
    title: "Zero-deps invariant enforced via `go list -deps` subprocess"
    date: 2026-06-18
    status: accepted
    category: schema
    tags: [plan, zero-deps, testing, invariant, go-tooling]
    path: schema/2026-06-18-plan-zero-deps-via-go-list-subprocess.md
    summary: "Enforce the plan/ zero-internal-deps invariant via a `go list -deps` subprocess in plan/deps_test.go scoped by module-qualified import path; rejected in-process AST traversal because the natural library (golang.org/x/tools) would itself be a non-stdlib import."

  - id: 2026-06-18-plan-testdata-verbatim-from-design-doc
    title: "Testdata fixtures committed verbatim from design doc §2.7"
    date: 2026-06-18
    status: accepted
    category: schema
    tags: [plan, testdata, fixtures, schema, documentation-drift]
    path: schema/2026-06-18-plan-testdata-verbatim-from-design-doc.md
    summary: "Commit plan/testdata/ JSON fixtures verbatim from the design doc §2.7 examples (voiced config, refused image, composed full plan) so schema-doc drift is caught by round-trip test failure at PR review."

  - id: 2026-06-18-plan-id-ulid-stdlib-only
    title: "PlanID — ULID generated with stdlib only"
    date: 2026-06-18
    status: accepted
    category: schema
    tags: [plan, ulid, zero-deps, schema, honesty-rule]
    path: schema/2026-06-18-plan-id-ulid-stdlib-only.md
    summary: "Implement NewPlanID() inline using time+crypto/rand+encoding/binary only — keeps plan/ zero-deps. Trades same-millisecond monotonicity for the invariant; panics on rand failure rather than silently fabricating a weak ID."

  - id: 2026-06-18-kokoro-distribution-kokoro-onnx-over-kokoro
    title: "Kokoro distribution — kokoro-onnx over kokoro"
    date: 2026-06-18
    status: accepted
    category: library-choice
    tags: [tts, rendering, kokoro, onnx, dependency, phase-one]
    path: library-choice/2026-06-18-kokoro-distribution-kokoro-onnx-over-kokoro.md
    summary: "Use kokoro-onnx 0.5.0 (MIT pkg, Apache-2.0 weights) as the phase-one TTS runtime via a venv-backed subprocess wrapper; rejected the torch-based kokoro 0.9.4 (~2 GB) and a nonexistent precompiled binary."
```
