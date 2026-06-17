---
name: setup-session
description: >
  Cold-start project bootstrap skill. Takes a problem statement + named skills,
  grills the user hard until every gap is closed, then produces CLAUDE.md,
  per-skill rune memory, .claude/ wiring, a directory skeleton, stub project
  agents (only if defaults are insufficient), an initial task-manager backlog,
  and an initial git commit. Run once per project before /build-session.
  Triggers — "set up new project", "bootstrap this repo", "/setup-session",
  "scaffold project with skills", "kick off new codebase".
  Output: CLAUDE.md, .claude/skill-memory/*, .claude/settings.json, .claude/agents/*,
  scaffolded source tree, .gitignore, README.md, seeded task-manager backlog,
  initial git commit.
---

# Setup Session

End-to-end **cold-start orchestrator**. Where `build-session` ships ONE task on an existing project, `setup-session` produces the project itself: docs, skills, agents, structure, backlog, first commit.

It is hard-grill by design. If the problem statement is silent on a gap that matters, the skill asks. It asks in batches (3–5 questions), branches on answers, and refuses to proceed when a [CRITICAL] gap is open.

---

## INVARIANTS

- Never overwrite files in a non-empty target dir without the user explicitly approving each one.
- Never invent stack/tool choices the user did not confirm. Inference is OK; silent commitment is not.
- Never skip the grill. Even a "trivial" project gets the question pass — Phase 2 may be short, but it runs.
- Never run `npm install` / `pip install` / `go mod tidy`. Manifests only. Dependency resolution is the user's call.
- Never push to a remote. Local `git init` + first commit only.
- Delegate CLAUDE.md + per-skill memory writes to the `rune` skill. Do not duplicate rune's question bank or templates.
- Stub a project-specific agent ONLY when `build-session` plus the installed skills cannot cover the workflow (e.g., domain expert needed, custom review persona, non-standard release flow). Default = no new agents.

---

## PRE-FLIGHT

1. **Cwd.** Print `pwd`. Confirm with user this is the project root.
2. **Repo state.** Check existence of: `.git/`, `CLAUDE.md`, `.claude/`, `package.json` / `pyproject.toml` / `go.mod`, source dirs. Classify as:
   - **greenfield** — empty or near-empty dir
   - **adopt** — files exist, no `.claude/` or `CLAUDE.md`
   - **rewire** — `.claude/` and/or `CLAUDE.md` already present
3. **Skills installed.** `ls ~/.claude/skills/` and `ls .claude/skills/ 2>/dev/null`. Confirm with user which skills should be wired into this project. Required default set: `rune`, `task-manager`. Recommended for any non-trivial project: `skald`, `mimir`, `sindri`, `multi-perspective-review`, `decision-journal`, `conventional-commits`. Note which are missing; the user can decline to install them now and rerun later.
4. **Mode branch:**
   - greenfield → proceed all phases.
   - adopt → confirm with user that scaffold step (Phase 4) skips lang-manifest creation if one already exists.
   - rewire → require explicit `--force` in the invocation OR ask "Overwrite existing CLAUDE.md / .claude/? (y / merge / abort)". On "merge", run rune in merge mode (it diffs and asks per-file).

Log: `[PREFLIGHT] Mode: {greenfield|adopt|rewire}. Skills present: {list}. Skills missing: {list}.`

---

## PHASE 1 — Intake

Accept whichever of these the user provided:

- Problem statement (paragraph or PRD)
- Named skill list (subset of installed skills the user wants wired)
- Stack hints (e.g., "Next.js + Postgres")
- Existing files (README, design doc) — read if present

Echo back, in 5–10 bullet points, what you understood. Mark each as **confirmed**, **inferred**, or **gap**. The user corrects before Phase 2 starts.

If the problem statement is one sentence, do not invent the rest — record the rest as gaps and grill them in Phase 2.

---

## PHASE 2 — Hard Grill

Load `references/grill-questions.md`. Question blocks:

| Block | When asked |
|---|---|
| **Project shape** [CRITICAL] | Always. What is being built, who uses it, what success means. |
| **Stack** [CRITICAL] | Always. Lang, framework, runtime, data store, deploy target. |
| **Constraints** [CRITICAL] | Always. Privacy / compliance / perf / cost ceilings, hard deadlines. |
| **Boundaries & integrations** [HIGH] | If problem mentions external systems or auth. |
| **Quality bar** [HIGH] | Always. Test depth, review depth, definition of done. |
| **Ops & lifecycle** [MEDIUM] | If deploy target ≠ "local only". CI, monitoring, on-call. |
| **Team & ownership** [MEDIUM] | If >1 contributor implied. |
| **Conventions** [MEDIUM] | Style, lint, commit format, branch model. |

Rules:

- Batches of 3–5 via `AskUserQuestion`. Never one-at-a-time.
- If the intake already answered a question, do not re-ask — confirm in a single sentence and move on.
- Branch aggressively: no DB → skip DB questions; greenfield → skip "existing patterns".
- Required [CRITICAL] unanswered → block. State which question is blocking and why.
- "Don't know" on a recommended question → record as `confidence: MED` and proceed. Sindri / build-session will resurface it later.
- After each batch, summarize what changed in the running project profile (one sentence per delta) so the user can spot drift early.

Termination condition: every [CRITICAL] block answered, every [HIGH] block answered or explicitly deferred. Print the full profile back to the user. User says "looks good" → Phase 3.

---

## PHASE 3 — Run Rune (delegated)

Spawn a sub-task: invoke `rune` skill with the Phase 2 profile assembled as a PRD body. Pass the user-confirmed skill list as the discovery hint so rune only generates memory for installed + selected skills.

Rune writes:

- `CLAUDE.md` (project memory, with frontmatter + confidence tags)
- `.claude/skill-memory/{skill}/*` for each skill whose `rune.md` manifest is present

Setup-session does not duplicate this — it consumes rune's output. If rune blocks ("missing [CRITICAL]"), surface the block; do not paper over it.

Log: `[PHASE 3] Rune complete. Written: {file list}. MED-confidence items: {N}.`

---

## PHASE 4 — Scaffold

Write the minimum source tree implied by the stack answer. Examples:

| Stack | Skeleton |
|---|---|
| Next.js (App Router) | `app/`, `app/layout.tsx` stub, `app/page.tsx` stub, `package.json`, `tsconfig.json`, `next.config.mjs`, `.gitignore` |
| Node CLI (TS) | `src/index.ts`, `src/lib/`, `package.json`, `tsconfig.json`, `bin/cli.mjs`, `.gitignore` |
| Python service (FastAPI) | `app/main.py`, `app/api/`, `app/models/`, `pyproject.toml`, `tests/`, `.gitignore` |
| Go service | `cmd/{name}/main.go`, `internal/`, `go.mod`, `Makefile`, `.gitignore` |
| Library (any lang) | `src/`, `tests/`, manifest, `.gitignore`, no entrypoint |

Rules:

- Stub files contain ONLY the smallest compilable / parseable placeholder + a `// TODO: <one line>` pointing at the first task. No business logic.
- `.gitignore` content is written inline based on stack (node_modules, .venv, dist, .DS_Store, .env*, .claude/handoff/ — never commit handoff state).
- `README.md` is a stub: project name (from profile), one-paragraph description, "Run /build-session to start the first task." Nothing else.
- If `adopt` mode: skip files that already exist. Diff and ask before touching any existing file.

Log: `[PHASE 4] Scaffold written: {file count} files, {dir count} dirs. Skipped existing: {N}.`

---

## PHASE 5 — Wire `.claude/`

Write `.claude/settings.json` from `templates/settings.json`, filled with:

- `permissions.allow` — entries for the wired skills' common Bash commands (e.g. `Bash(git status)`, `Bash(pnpm test:*)`)
- `hooks` — only if user opted in during Phase 2 (e.g., on-commit caveman-commit, on-stop reminder). Default empty.
- `model` — only if user picked a non-default
- `outputStyle` — only if user picked one

Create `.claude/agents/` directory. Empty by default. Phase 6 decides if any stub goes here.

Log: `[PHASE 5] settings.json written. Hooks: {count}. Allowlist entries: {count}.`

---

## PHASE 6 — Project-Specific Agent Stubs (gated)

Ask: "Do the installed skills + `build-session` cover your workflow, or do you need a project-specific orchestrator?" Default answer is "covered" — do NOT spawn a stub unless the user names a concrete responsibility the existing agents cannot handle.

Triggers that justify a stub:

- Domain expert review needed (e.g., trading-strategy reviewer, security-lead pre-check)
- Custom release flow (multi-env promotion, manual approval gates not in build-session)
- Non-standard quality gate (e.g., physical-device test runner, on-chain replay)

If yes, copy `templates/agent-stub.md` to `.claude/agents/{slug}.md`, fill name/description/triggers from the user's answer, leave the body as a `## TODO` outline. Do NOT write production logic in the stub — the user iterates on it later, possibly via build-session.

Log: `[PHASE 6] Project agents created: {list-or-none}.`

---

## PHASE 7 — Seed Backlog

Decompose the Phase 2 profile into 3–7 initial tasks. Examples:

- "Wire up CI (lint + test on PR)"
- "Add Dockerfile + docker-compose for local dev"
- "Define core data models — {entity list}"
- "Implement first end-to-end happy path: {primary user flow}"

For each task, delegate to `task-manager` Create mode with:

- `title`
- `priority` (high for the unblocker tasks, medium otherwise)
- `acceptance_criteria` (1–3 bullets derived from the profile)
- `context` (1 sentence linking to the relevant CLAUDE.md section)
- `source: setup-session`

Show the proposed task list to the user before creation. User edits/approves, then create.

Log: `[PHASE 7] Seeded {N} tasks: {list of IDs + titles}.`

---

## PHASE 8 — Git Init + First Commit

1. If `.git/` absent → `git init`.
2. `git add -A` is NOT used. Stage explicitly: every file the skill wrote this session (track the list as you write). Avoid committing pre-existing untracked files that the user did not authorize.
3. `git commit -m "chore: initial project setup via /setup-session"` with a body listing the major artifacts (CLAUDE.md, scaffold, N tasks seeded). No remote push.
4. If pre-commit hook fails → surface error, do NOT bypass with `--no-verify`. User fixes hook, reruns commit.

Log: `[PHASE 8] Initial commit: {short SHA}. Files: {count}.`

---

## PHASE 9 — Summary

Print to user (do not write to file):

```
## /setup-session complete

Project: {name}
Mode: {greenfield|adopt|rewire}

Written:
- CLAUDE.md ({line count} lines, MED-confidence items: {N})
- .claude/skill-memory/  ({skill count} skills wired)
- .claude/settings.json  ({allowlist count} allow rules)
- .claude/agents/        ({project-agent count} stubs)
- Scaffold: {dir tree, depth 2}
- README.md, .gitignore, {lang manifest}

Backlog seeded: {N} tasks. First task: {ID} {title}.

First commit: {short SHA}

Next:
  /build-session       → start the first task
  /task-manager next   → review the backlog
```

Stop. Do not auto-chain into build-session.

---

## HARD STOPS

Each fires `_SETUP_HARDSTOP.md` at project root with reason + needed input. Stop, surface, wait.

| # | Condition |
|---|---|
| 1 | [CRITICAL] question block unanswered after the user declined to answer twice. |
| 2 | `rune` returns a block (missing manifest, missing template, [CRITICAL] gap rune sees that setup-session missed). |
| 3 | Target dir is non-empty AND user did not approve overwrite/merge. |
| 4 | Git pre-commit hook fails. |
| 5 | Skill in user's selected list is not installed AND user declined to drop it. |
| 6 | `task-manager` Create mode errors on backlog seed (e.g., no task storage configured). |

---

## RESUME

Setup-session does NOT resume mid-flow. If interrupted:

- If `_SETUP_HARDSTOP.md` exists at project root → read it, restart from the named phase after the user resolves the input.
- If no hardstop and the user invokes again → ask whether to **redo** (overwrite-with-consent on each file) or **continue** (skip phases whose artifacts exist, run the rest).

Phase-completion detection (in continue mode):

| Phase | Detected complete when |
|---|---|
| 3 | `CLAUDE.md` exists with rune metadata header. |
| 4 | Scaffold dir tree matches the stack's expected entries. |
| 5 | `.claude/settings.json` exists. |
| 7 | task-manager has ≥1 task with `source: setup-session`. |
| 8 | `git log --oneline -1` matches `chore: initial project setup via /setup-session`. |

---

## ANTI-SCOPE

Setup-session does NOT:

- Install dependencies.
- Push to a remote, open a PR.
- Run tests, lint, or CI.
- Generate business logic, models, or APIs beyond stubs.
- Make architecture decisions for the user (it asks, records, and reflects — never invents).
- Auto-trigger build-session.
- Re-run after Phase 9 unless explicitly re-invoked.
- Modify global `~/.claude/settings.json` — project scope only.
