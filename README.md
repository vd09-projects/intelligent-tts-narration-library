# Intelligent TTS Narration Library

Go library for shape-aware TTS narration. Voices text by *meaning*, not character — per-block leveling (gist / summary / detail), pluggable edges (input adapter, intelligence adapter, renderer, output sink), honest refusal over fabrication.

See `problem-statement.md` for framing and `docs/solution-phase-design.md` for module layout and the narration-plan schema.

## Quickstart

Prerequisites:

- Go 1.22+.
- macOS (phase one uses `afplay` for the ephemeral sink).
- A working `scripts/kokoro` wrapper — see `render/sherpa/README.md` for the Kokoro-onnx setup (Python venv + ONNX model files).

Run the vertical-slice CLI against the canonical sample document:

```sh
go run ./cmd/narrate --file docs/samples/sample.md
```

Expected: speakers play the document one block at a time, including an honest spoken refusal for the bare-image block. Exit code `0` on success.

## CLI flags

| Flag | Default | Choices | Meaning |
|---|---|---|---|
| `--file` | — (required) | path | Markdown document to narrate. |
| `--level` | `1` | `1` / `2` / `3` | Per-block leveling target: 1 = gist, 2 = summary, 3 = detail. |
| `--sink` | `ephemeral` | `ephemeral` / `persistent` | Output sink. `persistent` is not implemented in this slice and exits non-zero. |
| `--gender` | `female` | `female` / `male` | Voice gender. `female` → `af_bella`, `male` → `am_michael`. |

Exit codes: `0` success; `1` adapter / planner / renderer / sink error; `2` flag error or `--sink=persistent`.

## Running the tests

Default test pass — no audio, no subprocess to Kokoro / afplay:

```sh
go test ./...
```

Benchmarks — planner-only and end-to-end with stub edges:

```sh
go test -bench=BenchmarkNarrate -benchmem ./pipeline/...
```

Manual end-to-end smoke (real binary, real audio, listener confirms refusal by ear):

```sh
go test -tags manual ./pipeline/...
```

The manual smoke must run from the repo root so `./scripts/kokoro` resolves.

## Architecture, briefly

`pipeline.Pipeline` is the composition root — the only struct that holds concrete edge instances. It wires four edges around the intelligence-light `planner/` core: `adapter/` for input, `intelligence/` for optional comprehension, `render/` for audio, `sink/` for delivery. Plans flow through the pipeline as a single `plan.NarrationPlan` JSON contract; the renderer attaches a `plan.Timeline` keyed by `block_id` (block-level sync only — no word timings).

Phase one runs with no intelligence adapter wired. The planner takes the deterministic + degraded path: structured classes (code, config, table, heading, list) voice at every level; prose under ~120 words is read verbatim; prose over that limit and bare images are refused honestly with a spoken notice. Refusal is data, not error.

## Skills

This project is developed with a set of Claude Code skills:

- `rune` — project onboarding.
- `task-manager` — backlog + status, GitHub Issues backend.
- `mimir` — planning (architecture or task breakdown).
- `sindri` — implementation (plan / build / iterate).
- `skald` — handoff persistence for mimir / sindri / multi-perspective-review output.
- `multi-perspective-review` — multi-lens review.
- `decision-journal` — record load-bearing decisions.
- `conventional-commits` — commit message discipline.

Skill artifacts live under `.claude/handoff/{scope}/` (audit trail) and `decisions/` (journal).
