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
