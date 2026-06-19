<!-- rune-generated: 2026-06-18 | git: unknown | rune: 1.0 -->

# Intelligent TTS Narration Library

Go library that turns text into TTS narration by *meaning* rather than character — per-block leveling, pluggable edges, honest refusal over fabrication.

## Stack

- **Language:** Go (core + interfaces + edges). React (reference player).
- **Framework:** stdlib + `goldmark` (CommonMark AST segmenter); `github.com/k2-fsa/sherpa-onnx-go` (in-process upgrade path, deferred); official `github.com/modelcontextprotocol/go-sdk` v1.5.0.
- **Database:** none in core. Persistent sink writes filesystem directories (`audio.wav` + `plan.json` + `manifest.json`).
- **Deployment:** local-only. Hobby project, no recurring spend.
- **Key libs:** `goldmark` (segmentation), `cobra` (CLI), `modelcontextprotocol/go-sdk` (MCP server), Kokoro-82M (default voice, Apache-2.0, subprocess in phase one).

## Architecture

Hub-and-spoke around the **narration plan** — one JSON-on-wire schema serving every input and output. `plan/` (contract, zero deps) ← `planner/` (pure, no I/O, depends only on `plan/` and the `IntelligenceAdapter` interface) ← `pipeline/` (composition root) ← `cmd/narrate`, `cmd/narrate-mcp`. Four edges own all I/O: `adapter/` (file, mcptext, ocr), `intelligence/` (mcpsampling, anthropic), `render/sherpa`, `sink/` (ephemeral, persistent). The `planner` cannot open a file or touch a socket because it has no dependency that can. Per-block leveling (L1 gist / L2 summary / L3 detail) flows as `Block.Level` data; escalation re-renders one block without re-planning the document. React reference player at `player/` consumes plan + timeline JSON.

## Quick conventions

- **Error vs refusal boundary:** adapter I/O failure = error returned up the pipeline (stops). Readable-but-unvoiceable content = `Refusal` inside the plan (spoken + surfaced, plan still renders end-to-end). Never both.
- **Honesty rule (non-negotiable):** never fabricate. Bare image / unsupported diagram / oversized prose without intelligence → `Block.Status = refused` with short spoken notice + `SourceMap`. Refusal is data, not exception.
- **No I/O in `planner/` or `plan/`.** The composition root (`pipeline/`, `cmd/`) is the only place that knows concrete edges.
- **Voicing happens in the planner, not the renderer.** `Segment.Text` is final spoken words (e.g. `"replicas set to three"`, not `"replicas: 3"`). `VoicingDirective` is an optional phonetic hint the renderer may ignore.
- **Plan stays engine-neutral and pre-render.** Audio offsets live in `Timeline`, keyed by `block_id`, written by the renderer. Block-level sync only — no word timings (would contradict gist mode).
- **Schema versioning:** additive-compatible within a major `schema_version`. Consumers ignore unknown fields.
- **Lint:** `golangci-lint`. **Commits:** Conventional Commits (via `conventional-commits` skill). **Tests:** unit + table-driven + golden `plan.json` fixtures; no golden audio (audio validated by ear during `/verify`).
- **Commands:** drive all repeatable dev actions through `Makefile` targets. `make help` lists them. Use `make test` / `make test-manual` / `make bench` / `make run` / `make sanity` rather than retyping `go test ./...` etc. Add new repeatable workflows as Makefile targets so they stay consistent across sessions.

## Domain rules

- **Per-block leveling.** L1/L2/L3 is per `Block`, re-requestable. Escalation = `planner.Plan` re-runs for one block (raw text recoverable via `SourceMap` + content hash) → `Renderer.RenderBlock` patches just that block's audio + `BlockTiming`. Other blocks (audio + sync) untouched. Stale-elsewhere is acceptable.
- **Deterministic L1 for structured classes.** code / config / table / diagram_as_text / heading / list voice at all levels with no intelligence adapter. Only prose truly needs the adapter; intelligence enriches L2/L3 for structured classes but never blocks voicing.
- **Prose without intelligence.** Under ~120 words / ~45 s → read verbatim (`Status = degraded`). Over → refuse with `RefuseNoIntelligence` (`Status = refused`). Never a silent invented gist.
- **Oversized-block splitting.** Prose: ~20 lines / ~800 chars. Structured-with-clean-seams (func boundary / top-level YAML key / table row): ~60–80 lines / ~2000–3000 chars. Split only on clean structural seams — never arbitrary cuts.
- **Audio format:** 24 kHz mono PCM/WAV (Kokoro native rate, no resampling).
- **Voice selection:** ≥2 Kokoro voices wired phase one (`af_bella` female default + `am_michael` male). MCP `gender` arg picks one; `voice` hint in `PlanDefaults` is engine-neutral.
- **MCP `speak` tool shape:** `{text|source, level, sink, gender}`. `gender` default `female`. Summarization rides on client LLM via MCP sampling — zero extra cost.
- **Intelligence caching:** by `(block content hash, level, model)`. Escalation doesn't re-bill.
- **Persistent-sink stale behavior:** on `content_hash` or path mismatch, mark stale in `manifest.json`. Do not auto-regenerate.
- **Status enum:** `voiced` (level fully met) | `degraded` (voiced at lower fidelity than requested — e.g. prose read verbatim instead of gisted) | `refused` (not voiced, refusal carries reason + source map).

## Invariants

- `plan/` imports nothing from this project. Everything imports `plan/`.
- `planner/` imports only `plan/` and the `IntelligenceAdapter` interface. No concrete adapter. No I/O.
- `adapter/`, `render/`, `sink/`, `intelligence/` import `plan/` plus their own interface package.
- Only `pipeline/` and `cmd/` know which concrete engine, adapter, intelligence backend, or sink is in use.
- One narration-plan format serves every input and every output. No forks per adapter type.
- `Refusal` is never an error. Errors stop the pipeline; refusals are spoken and surfaced.
- Sync is block-level only. Word-level timing is forbidden (contradicts gist mode where spoken text ≠ source text).

## Gotchas

- **Piper engine is GPL** (`OHF-Voice/piper1-gpl`). Linking it makes the library GPL. Run Piper VITS *voice models* through Apache-2.0 `sherpa-onnx-go` instead, or shell out to a separate Piper process. Kokoro-82M is Apache-2.0 and is the default.
- **CGo deferred.** Phase-one renderer is subprocess (no `sherpa-onnx-go` in-process binding yet). Documented as upgrade path when latency / packaging start to matter.
- **Local-only means secrets get read aloud.** A secret in a config block could be spoken on the user's own machine. Awareness only, not a design driver in phase one.
- **English only phase one.** Multilingual deferred. Don't bake language assumptions into the planner deeper than `Locale = "en"` in `PlanDefaults`.

## Out of scope (phase one)

- No streaming / real-time narration. Planner needs whole input to decide voicing.
- No comprehension inside the core. Summarization, image description = caller-supplied `IntelligenceAdapter`.
- No image/graph interpretation in the library. Diagrams-as-text yes (Mermaid, ASCII, chart-as-YAML). Bare images → refused.
- No follow-up Q&A. This system speaks; it does not answer questions about what it spoke.
- No OCR / vision in the core. Edge adapters only, later phase.
- No multilingual. No multi-voice beyond female/male pair.
- No CI / no deploy / no monitoring. Local-only hobby project.

## Skills installed

- `rune` — project onboarding (re-run on major changes).
- `task-manager` — backlog + status tracking. Self-initializes on first run via `tasks/RUNE.md`.
- `mimir` — planning (option compare, task breakdown). Produces markdown; never invokes other skills.
- `sindri` — implementation (plan / build / iterate). Reads CLAUDE.md + own memory.
- `skald` — handoff persistence for mimir / sindri / multi-perspective-review outputs.
- `multi-perspective-review` — multi-lens code review.
- `decision-journal` — record load-bearing decisions (Decisions 1 & 2 in design doc are first candidates).
- `conventional-commits` — commit message discipline.

## Re-run rune when

- Primary language, framework, or TTS engine swap (Go → other / Kokoro → other / subprocess → CGo in-process).
- Core architectural boundaries are redrawn (e.g. planner gains I/O, plan schema majors).
- A core invariant is added, removed, or proven wrong (e.g. honesty rule scope changes, sync gains word-level).
- The project pivots — e.g. streaming added, OCR moved into core, multilingual.

Run `rune` to re-onboard. Incremental convention updates go through Sindri's memory suggestions.

## Reference docs (do not modify without re-rune)

- `problem-statement.md` — framing, leveling model, honesty rule, non-goals.
- `docs/solution-phase-design.md` — module layout, narration-plan schema, four edges + core, Decisions 1 & 2, Assumptions A1–A18.

<!-- success_in_6_months confidence: MED — TBD: surface via Sindri during use. Working interpretation: phase-one vertical slice runs end-to-end (file adapter → planner → sherpa+Kokoro → ephemeral sink), MCP server wired, React player renders block-level sync, ≥1 escalation flow demoed. -->
