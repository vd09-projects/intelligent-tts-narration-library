<!-- rune-generated: 2026-06-18 | git: unknown | rune: 1.0 -->

# Learned Patterns — Intelligent TTS Narration Library

Bootstrap state — grown by Sindri over time. Seed entries below reflect the design doc + problem statement; treat them as starting hot-spot map, not learned patterns yet.

## Known Hot Spots

- `plan/` — load-bearing JSON contract consumed by Go core + React player + future MCP clients — every change needs schema_version review + golden fixture update + cross-consumer check.
- `planner/segment.go` — segmentation is the seam between input shape and downstream classification; bad seams break leveling silently — review against goldmark AST behavior on weird markdown (nested fenced blocks, indented lists).
- `planner/voice.go` — symbol/identifier/path lexicon is shipped + user-overridable; default lexicon edits affect all users — keep table-driven tests on every entry.
- `planner/level.go` — per-class L1/L2/L3 rules; oversized-block split thresholds live here (prose ~20 lines / ~800 chars; structured ~60-80 lines / ~2000-3000 chars).
- `planner/degrade.go` — the nil-intelligence path is the honesty rule's last line; verbatim-vs-refuse boundary at ~120 words / ~45 s lives here.
- `render/sherpa/` — subprocess interface to Kokoro-82M phase one; process spawn per block; failure modes (timeout, missing binary, malformed audio) need clean translation to refusal or error.
- `sink/persistent/` — manifest.json holds `content_hash`. Stale detection on hash/path mismatch — must not auto-regenerate.
- `intelligence/` — caching key is `(block content hash, level, model)`. Wrong key = surprise rebill.

## Recurring False Positives

- (none yet — bootstrap)

## Established Conventions (Not in CLAUDE.md)

- Source map fields use snake_case in JSON (`start_line`, `end_line`, `raw_excerpt`) matching the design doc §2.4 schema.
- Block IDs follow `b###` zero-padded order (e.g. `b007`). Stable within a plan, regenerated on re-plan.
- Segment IDs follow `s#` within a block.
- ULIDs for `PlanID`. RFC3339 for `CreatedAt`.

## Code-time assumption amendments (sindri-relevant)

These amend design-doc Assumptions A1–A18 and affect how sindri writes the implementation. Cross-skill workflow rules (commit cadence, task ordering, handoff dir status) live elsewhere — see `tasks/RUNE.md` Notes for task-manager rules and CLAUDE.md for project invariants.

- **A15 amended:** ≥2 Kokoro voices wired in phase one (female default + male), driven by MCP `gender` arg. Original "single configured default voice" assumption no longer holds. `voice` hint in `PlanDefaults` stays engine-neutral. When implementing `render/sherpa/` (issue #5) and the CLI `--gender` flag (issue #7), wire both voices from day one.
- **A7 nuance:** prose split threshold = ~20 lines / ~800 chars. Structured-with-clean-seams (func boundary / top-level YAML key / table row) = ~60–80 lines / ~2000–3000 chars. NOT a single uniform threshold. When implementing `planner/level.go` (issue #4) the two thresholds must be separate constants — don't collapse them.

## Accepted Debt

- Phase-one subprocess renderer carries process-spawn latency per block — `render/sherpa` — follow-up: migrate to in-process `sherpa-onnx-go` when latency or packaging matter.
- No retry/backoff on intelligence adapter calls — `intelligence/` — follow-up: revisit when first real LLM integration lands (phase 2/3).
- Sequential block rendering — `pipeline/` — follow-up: block-parallel allowed later for long docs (A17).
- No multilingual support — `planner/level.go`, `render/sherpa/` — follow-up: deferred; revisit after phase one.

confidence: HIGH
