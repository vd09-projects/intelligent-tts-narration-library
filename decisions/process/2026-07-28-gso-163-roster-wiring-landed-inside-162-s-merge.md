# GSO #163 roster wiring landed inside #162's merge; #163 pivoted to docs+tests-only

| Field    | Value            |
|----------|------------------|
| Date     | 2026-07-28       |
| Status   | accepted         |
| Category | process          |
| Tags     | gso, gpt-sovits, scope, process, 163, 162, roster |

## Context

The nominal deliverables of #163 (GSO P3 — roster wiring) were the GPT-SoVITS
roster row, the `pipeline.BuildRenderer` `EngineGSO` arm, and their pipeline unit
tests. On picking up #163, those were already authored and merged as part of #162
(commit `c0d98c3`, the GSO peer render engine) — so AC1/AC2/AC3-seam/AC4/AC5 were
already green on HEAD. Engine wiring bled forward from the render-engine ticket
(#162) into the roster ticket (#163).

## Decision

#163 was re-scoped to close only the gaps #162 left: the AC6 `narrate-mcp`
rebuild+restart gotcha note (added to the existing
`docs/gpt-sovits-inference-runbook.md`), GSO-named cmd-level voice assertions at
both roots, and a cosmetic `--listen` rejection-message generalization. No
`pipeline/` production code changed in #163 — the PR is docs+tests-only. This
entry exists so a reviewer expecting the roster wiring diff in #163 knows why it
isn't there, and to flag the #162→#163 scope bleed on the #161–#164 GSO chain
trail. Not an architecture decision — the load-bearing GSO architecture decisions
were already recorded during #161/#162.
