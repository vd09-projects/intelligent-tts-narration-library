# Rune Configuration

**Last updated:** 2026-06-18

## Backend

```
backend: github
```

GitHub repo: `vd09-projects/intelligent-tts-narration-library`. Issues stored as `gh` issues; task ID = issue number.

## Default mode

```
default_mode: vibe
```

Most steady-state work in this project is atomic edits within already-defined interfaces (planner heuristics, lexicon entries, voicing directives, fixture additions). Multi-day interface/scaffold work overrides per task to `rune:dev`.

## Sizing rubric

| Rune | Solves | Output | Sizing | Forbidden |
|---|---|---|---|---|
| **dev** | Big chunk of the problem; an end-to-end feature slice. | Shipped code + tests + integration. | 3-4 days of focused work. | Splitting itself into vibe siblings to dodge review weight. |
| **vibe** | One subchunk of an already-understood problem. | Concrete code edit, atomic and reviewable. | Hours, one focused diff. | Interfaces, scaffolding, speculative abstractions, "set up the structure for". |
| **research** | Unknown — how does X work, what library to use, what does the API return. | Written findings or decision-journal entry. | Bounded timebox. | Shipping production code. |
| **analysis** | Best approach unclear — known problem, unknown solution. | Tradeoff comparison + recommendation. | Bounded timebox. | Shipping production code. |

## Exceptions

- path: `plan/*` → default_mode: dev (load-bearing JSON contract; schema bumps are dev-scoped, not vibe)
- path: `planner/{segment,classify}.go` → default_mode: dev (parser changes touch every downstream class)
- path: `intelligence/*` → default_mode: dev (new adapter = new external surface)

## Notes

- Rune set at task creation by Mode 8 of the task-manager skill.
- Reclassify any task with "rune for TASK-NNNN" or "is this vibe or dev?".
- Decompose (Mode 7) uses this rubric in both directions: split oversized tasks, merge undersized clusters.
- Decisions 1 & 2 from `docs/solution-phase-design.md` go to `decision-journal`, not here.

## Backlog ordering — three places, not prose

Dependency / unblock order MUST live in all three of these signals. Prose-only "Blocked by" inside Notes is invisible to Mode 3 (Next) logic.

1. **`status:blocked` label** on the blocked issue — filters it from `gh issue list --search "-label:status:blocked"` ("Up Next" query).
2. **`Blocked by: #N`** as a top-level body field (NOT inside `**Notes:**`) — Mode 3 parses this to surface newly-unblockable items when blocker #N closes.
3. **Priority label + `updatedAt`** — within the unblocked bucket: `critical` > `high` > `medium` > `low`, then most-recently-updated first.

Body template (per `~/.claude/skills/task-manager/references/backend-github.md`):

```
**Context:** <1-2 sentences>

**Acceptance criteria:**
- [ ] ...

**Blocked by:** #N, #M

**Notes:** optional
```

Caught the prose-only mistake on the 2026-06-18 backlog seed (issues #4–7) and re-edited. Do it right the first time.

## GitHub backend prerequisites

Before the first `gh issue create` against a fresh repo, these three steps must already be done. Without them, label-on-create behaviour varies across `gh` versions and Mode 3 ordering breaks.

1. **Git remote wired.** `git init -b main` if needed, then `git remote add origin https://github.com/<owner>/<repo>.git`. `gh repo view <owner>/<repo>` must succeed.
2. **15-label set provisioned.** `gh label create --force --color <hex> --repo <owner>/<repo>` for all four axes: `priority:{critical,high,medium,low}`, `rune:{dev,vibe,research,analysis}`, `source:{session,decision,user,discovery}`, `status:{in-progress,blocked,cancelled}`.
3. **This RUNE.md exists** with `backend: github` and a `default_mode:` value.

For this project, all three are already done (2026-06-18). Re-run only if the remote changes or the label set drifts (check via `gh label list --repo vd09-projects/intelligent-tts-narration-library`).

Pushing local commits is a SEPARATE step — `gh issue create` works against the remote regardless of whether local commits have been pushed.
