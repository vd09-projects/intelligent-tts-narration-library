# Decision Index

<!--
  This file is maintained by the decision-journal skill.
  Entries are in YAML format for machine-friendly querying.
  Newest entries go at the top. Do not manually reorder.
-->

```yaml
decisions:
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
