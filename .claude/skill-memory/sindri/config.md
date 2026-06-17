<!-- rune-generated: 2026-06-18 | git: unknown | rune: 1.0 -->

# Sindri Config — Intelligent TTS Narration Library

## Language

primary_language: go
language_version: go1.22+
secondary_language: typescript (React reference player only, under `player/`)

## Scope

- **In scope:** entire Go module under repo root (`plan/`, `planner/`, `adapter/`, `intelligence/`, `render/`, `sink/`, `pipeline/`, `cmd/`).
- **In scope, lower scrutiny:** `player/` (React reference player) — phase 5, downstream consumer of plan + timeline JSON.
- **Out of scope this project:** modifying `problem-statement.md` and `docs/solution-phase-design.md` without a re-rune. They are input docs; design changes route through the design doc + rune re-run, not Sindri.

## Quality Overrides

Stricter:
- `plan/` — schema is load-bearing and consumed by non-Go clients. **Every change** needs:
  - golden `plan.json` fixture updated
  - `SchemaVersion` reviewed (bump on breaking change; additive within major)
  - JSON tags match field names exactly
  - no Go-only types leak into JSON (no `time.Time`, use RFC3339 strings)
- `planner/` — no I/O imports allowed. CI-equivalent gate: `go list -deps ./planner/...` must not include `os`, `net`, `net/http`, `io/ioutil`, syscall packages, or any concrete adapter/render/sink package.
- Honesty-rule code paths (`refused`, `degraded` Status) — every new RefusalReason needs a test case asserting `Spoken = true` and `SourceMap` populated.

Looser:
- `player/` — reference implementation, not the contract. Lint warnings tolerated; tests at smoke level only.
- `cmd/` — wiring code. Lower coverage bar; integration test at pipeline level covers it.

## Interrogation Defaults

- default_stage: build
- test_framework: standard `testing` package + table-driven tests + golden fixtures
- golden_fixture_location: `testdata/` under each package
- performance_target: phase-one slice — narration of a 500-word markdown doc end-to-end under 10 s on developer laptop (renderer dominates; planner should be <100 ms of that)
- lint: `golangci-lint run` clean
- commit_format: Conventional Commits

## Persona Integration

No domain persona skill installed.

confidence: HIGH
