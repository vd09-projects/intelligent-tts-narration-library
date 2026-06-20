# Multi-Perspective Review Config — Intelligent TTS Narration Library

<!-- rune-generated: 2026-06-21 | git: 5305d9c | rune: 1.0 -->

## Reviewer Overrides

always_include:
  - API Contract Reviewer (the narration plan is the one JSON-on-wire contract; additive-compat-within-major-schema_version invariant must hold)
  - Domain Logic Reviewer (honesty rule + per-block leveling are load-bearing; refusal-is-data not exception)
  - Error Handling & Resilience Inspector (error-vs-refusal boundary is core: adapter I/O failure = error stops pipeline; readable-but-unvoiceable = Refusal; never both)
  - Tech Debt Sentinel (honesty-rule sentinel — fabrication / silent invented gist is a blocker, plus CGo-deferred / subprocess upgrade-path debt is tracked)

always_exclude:
  - Accessibility Reviewer (no UI in core; React reference player is out-of-scope minor)
  - CSS & Styling Reviewer (no styling in the Go library)
  - FE Performance & Rendering Reviewer (reference player only, not the deliverable)
  - Infra & Deployment Reviewer (local-only hobby project; no CI / deploy / monitoring phase one)
  - Data Integrity & Migration Reviewer (no DB in core; persistent sink writes plain filesystem dirs)

## Project Context

domain: Intelligent TTS narration library — turns text into TTS narration by meaning (per-block L1/L2/L3 leveling), honest refusal over fabrication
primary_languages: Go 1.25+ (core + interfaces + edges); React (reference player only)
architecture: Hub-and-spoke around the narration plan (one JSON schema). plan/ (zero deps) ← planner/ (pure, no I/O) ← pipeline/ (composition root) ← cmd/. Four I/O edges: adapter/, intelligence/, render/sherpa, sink/ (ephemeral, persistent).
urgency_default: normal
debt_tolerance: normal <!-- but honesty-rule violations are zero-tolerance: see Voice Tuning -->

## Custom Triage Rules

- Any change touching `plan/` or `planner/` → always include API Contract Reviewer + Domain Logic Reviewer (the contract + voicing-in-planner rule).
- Any change touching an edge (`adapter/`, `intelligence/`, `render/`, `sink/`) → always include Error Handling & Resilience Inspector + Concurrency & State Safety Reviewer (I/O boundary, subprocess + ctx-cancel; cf. #11/#29 sink work).
- Any change to `schema_version` or plan fields → treat as scope: large regardless of line count; include Backward-Compat Reviewer (additive-compatible-within-major invariant; consumers ignore unknown fields).
- Any `I/O` (file open, socket, exec) appearing under `planner/` or `plan/` → automatic blocking finding (invariant: no I/O in planner or plan).

## Reviewer Voice Tuning

- Tech Debt Sentinel: zero-tolerance on the honesty rule — any fabrication, bare-image voicing, or silent invented gist (vs. `Status = refused` / `degraded` with `SourceMap`) is a blocking finding, not a nit.
- Domain Logic Reviewer: enforce status enum semantics (`voiced` / `degraded` / `refused`) and that voicing happens in the planner (`Segment.Text` = final spoken words), not the renderer.
- Error Handling & Resilience Inspector: enforce error-vs-refusal boundary strictly — `Refusal` is never returned as an error; errors stop the pipeline, refusals are spoken and surfaced. Never both for one cause.
- Naming Clarity Guardian: enforce Go conventions (PascalCase exports, no stutter); plan field json tags stay engine-neutral.
- Concurrency & State Safety Reviewer: subprocess + context-cancellation correctness in the render/sink edges — joined-error fidelity on cancel, no swallowed exit errors.
