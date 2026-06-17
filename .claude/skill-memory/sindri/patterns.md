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

## Workflow conventions (project-specific)

### Task ordering — three places, not prose

Backlog dependency / unblock order MUST live in:
1. **`status:blocked` label** on the blocked issue (so `gh issue list --search "-label:status:blocked"` filters it out of "Up Next").
2. **`Blocked by: #N`** field in the body (top-level, not buried in Notes) so task-manager Mode 3 parses it to surface newly-unblockable items when blocker closes.
3. **Priority label** + `updatedAt` for ordering within the unblocked bucket.

Mentioning dependencies in prose-only Notes is wrong — Mode 3 won't see them and nothing un-blocks automatically. Caught this on first backlog seed (issues #4-7 needed re-edit).

### Per-phase commits for setup/scaffolding work

When committing setup-session output (or any multi-phase scaffolding run), split by phase — one commit per rune output / scaffold / settings / backlog-prep — rather than the default single `chore: initial project setup` commit. Easier to revert one phase, easier to read the git log story. User-confirmed pattern this session.

### gh issue creation prerequisites

Before the first `gh issue create` against an empty repo:
1. `git init` + `git remote add origin <url>` (gh resolves repo from remote)
2. Verify repo exists (`gh repo view <owner>/<name>`)
3. Provision the full 15-label set via `gh label create --force --color <hex> --repo <owner>/<name>` for all 4 axes (`priority:{critical,high,medium,low}`, `rune:{dev,vibe,research,analysis}`, `source:{session,decision,user,discovery}`, `status:{in-progress,blocked,cancelled}`). Without these, label-on-create fails silently or noisily depending on gh version.

Pushing the local commits is a SEPARATE step — setup-session never pushes. gh issue create works against the remote independent of whether local commits have been pushed.

### `.claude/handoff/` is gitignored

Per-machine, regenerable via skald. Never commit. CLAUDE.md + `.claude/skill-memory/` ARE committed (project memory the team needs).

### task-manager self-bootstrap

task-manager doesn't have a `rune.md` manifest — rune-skill skips it. It self-initializes from `templates/RUNE.md` on first invocation via its own `references/setup.md` flow. For setup-session runs, pre-init `tasks/RUNE.md` (and provision github labels) BEFORE delegating to task-manager Create mode, so Phase 7 invocation is a clean task-create rather than a noisy init+create.

### Sensitive A-numbered assumptions (amended this session)

- **A15 amended:** ≥2 Kokoro voices wired in phase one (female default + male), driven by MCP `gender` arg. Original "single configured default voice" assumption no longer holds. `voice` hint in `PlanDefaults` stays engine-neutral.
- **A7 nuance:** prose split threshold = ~20 lines / ~800 chars. Structured-with-clean-seams (func boundary / top-level YAML key / table row) = ~60–80 lines / ~2000–3000 chars. NOT a single uniform threshold.

## Accepted Debt

- Phase-one subprocess renderer carries process-spawn latency per block — `render/sherpa` — follow-up: migrate to in-process `sherpa-onnx-go` when latency or packaging matter.
- No retry/backoff on intelligence adapter calls — `intelligence/` — follow-up: revisit when first real LLM integration lands (phase 2/3).
- Sequential block rendering — `pipeline/` — follow-up: block-parallel allowed later for long docs (A17).
- No multilingual support — `planner/level.go`, `render/sherpa/` — follow-up: deferred; revisit after phase one.

confidence: HIGH
