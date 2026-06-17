<!-- rune-generated: 2026-06-18 | git: unknown | rune: 1.0 -->

# task-manager — skill memory

**Source of truth for task-manager config + workflow rules lives in `tasks/RUNE.md`, not here.**

task-manager (per its `SKILL.md` Backend Detection step 1) reads `tasks/RUNE.md` on every invocation. It does NOT scan `.claude/skill-memory/task-manager/`. Any rule put here would be unloaded — human-only documentation with drift risk.

This directory exists as a discovery aid for cross-skill readers (sindri, mimir, skald) that scan `.claude/skill-memory/` for project conventions. When you need task-manager behavior info, follow the pointer:

## Where to look

| Question | File |
|---|---|
| Backend (github vs file) | `tasks/RUNE.md` → `backend:` |
| Default coding mode (dev/vibe/research/analysis/mixed) | `tasks/RUNE.md` → `default_mode:` |
| Per-area mode exceptions | `tasks/RUNE.md` → `Exceptions` |
| Backlog ordering rules (status:blocked label + Blocked by body field + priority) | `tasks/RUNE.md` → `Backlog ordering — three places, not prose` |
| GitHub backend prereqs (git remote + 15-label set) | `tasks/RUNE.md` → `GitHub backend prerequisites` |
| Label scheme, issue body template, gh cheatsheet | `~/.claude/skills/task-manager/references/backend-github.md` |
| File backend layout, BACKLOG.md format | `~/.claude/skills/task-manager/references/backend-file.md` |
| Rune classification rubric | `~/.claude/skills/task-manager/references/RUBRIC.md` |
| Mode flows (harvest, review, decompose) | `~/.claude/skills/task-manager/references/mode-*.md` |

## Why this split exists

`tasks/` is task-manager's home (it owns `BACKLOG.md`, `TASK-LOG.md`, `archive/`, and `RUNE.md`). Keeping config + rules there means a single read per invocation and zero drift between "what's documented" and "what runs."

`.claude/skill-memory/` is the convention rune-skill follows for skills WITH a `rune.md` manifest. task-manager doesn't ship a `rune.md` (it self-bootstraps via its own `references/setup.md`), so rune-skill never created this directory automatically. It exists here only as a redirect for searchers.

## Don't add rules here

Adding rules in this file directly = drift. If you find yourself wanting to document a task-manager behavior, edit `tasks/RUNE.md` instead and (if useful) add a pointer row in the table above.
