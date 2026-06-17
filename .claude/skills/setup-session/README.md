# setup-session

Cold-start project bootstrap skill. Opposite end of the lifecycle from `build-session`:

- **setup-session** — produces the project (CLAUDE.md, scaffold, skills wired, backlog seeded, first commit).
- **build-session** — ships one task end-to-end on an existing project.

## What it does

1. **Pre-flight** — classify dir as greenfield / adopt / rewire. Check which skills are installed.
2. **Intake** — accept the problem statement + named skills. Echo back understanding.
3. **Hard grill** — batched questions (3–5 at a time) covering project shape, stack, constraints, integrations, quality bar, ops, ownership, conventions. Refuses to proceed if any [CRITICAL] block is open.
4. **Run rune** — delegate CLAUDE.md + per-skill memory to the `rune` skill.
5. **Scaffold** — minimal stack-appropriate source tree, manifest, `.gitignore`, README stub.
6. **Wire `.claude/`** — `settings.json` with allowlist + optional hooks. Empty `agents/` dir.
7. **Project-agent stubs** — only if defaults can't cover the workflow. Default = none.
8. **Seed backlog** — 3–7 tasks via `task-manager`, user approves before creation.
9. **Git init + first commit** — local only, no push.
10. **Summary** — what was written, what's next.

## Pipeline

```
pre-flight
  → intake
    → hard grill ([CRITICAL] gates here)
      → rune (CLAUDE.md + skill memory)
        → scaffold
          → wire .claude/
            → project agents (gated, usually skipped)
              → seed backlog via task-manager
                → git init + first commit
                  → summary
```

## Required skills

Installed at `~/.claude/skills/`:

- `rune` (Phase 3 delegates here for CLAUDE.md + per-skill memory)
- `task-manager` (Phase 7 seeds the backlog)

Recommended (offered during pre-flight, optional):

- `skald`, `mimir`, `sindri`, `multi-perspective-review`, `decision-journal`, `conventional-commits`

## Install

One-time symlink:

```bash
ln -s ~/repos/skills/agents/setup-session ~/.claude/skills/setup-session
```

Verify:

```bash
ls -la ~/.claude/skills/setup-session
```

## Usage

Triggers — any of:

- "set up new project"
- "bootstrap this repo"
- "/setup-session"
- "scaffold project with skills X, Y, Z"
- "kick off new codebase for {problem statement}"

The user provides a problem statement (paragraph) + optionally names which installed skills to wire. The skill fills the rest by grilling.

## Modes

| Mode | When | Behavior |
|---|---|---|
| greenfield | Empty dir | All phases run, no prompts about overwrite. |
| adopt | Files exist, no `.claude/` or `CLAUDE.md` | Scaffold phase skips conflicting files (diffs and asks). |
| rewire | `.claude/` or `CLAUDE.md` already present | Requires `--force` or per-file consent. Rune runs in merge mode. |

## Hard stops

| # | Condition |
|---|---|
| 1 | [CRITICAL] question unanswered after user declined twice. |
| 2 | Rune blocks (missing manifest/template/[CRITICAL] gap). |
| 3 | Non-empty target dir without overwrite/merge consent. |
| 4 | Git pre-commit hook failure. |
| 5 | Required skill not installed AND user declined to drop it. |
| 6 | `task-manager` create errors on backlog seed. |

Hard stops write `_SETUP_HARDSTOP.md` at project root. User resolves, re-invokes — skill restarts from named phase.

## Anti-scope

This skill does NOT:

- Install dependencies (`npm install`, `pip install`, `go mod tidy`).
- Push to a remote or open a PR.
- Run tests, lint, or CI.
- Generate business logic — only stubs.
- Make architecture decisions silently — it asks, records, reflects.
- Auto-trigger `build-session` after completing.
- Touch global `~/.claude/settings.json` — project scope only.

## Tuning per project

Open `SKILL.md` Phase tables and:

- **Skip a phase** — copy SKILL.md to `.claude/skills/setup-session/SKILL.md` (project-local override) and delete the phase block.
- **Add a question block** — append to `references/grill-questions.md`.
- **Change stack scaffold** — edit the table under Phase 4.
- **Change default allowlist** — edit `templates/settings.json`.

## Files

```
~/repos/skills/agents/setup-session/
├── SKILL.md                         # main spec
├── README.md                        # this file
├── references/
│   └── grill-questions.md           # question bank (extends rune's)
└── templates/
    ├── settings.json                # default .claude/settings.json
    └── agent-stub.md                # project-specific agent stub
```
