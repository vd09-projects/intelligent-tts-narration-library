# Player dev actions drive through Makefile targets, not raw pnpm commands

| Field    | Value            |
|----------|------------------|
| Date     | 2026-06-21       |
| Status   | accepted         |
| Category | convention       |
| Tags     | player, makefile, dev-workflow, issue-18 |

## Context

`CLAUDE.md` mandates: "drive all repeatable dev actions through `Makefile` targets. `make help` lists them." The Go half of the project is fully Makefile-driven (`make test`, `make sanity`, `make run`, `make run-persistent`, etc.). The new React reference player under `player/` has its own toolchain — `pnpm install`, `pnpm dev`, `pnpm build`, `pnpm test`. Two choices for how the player surfaces these:

- **A — Document raw `pnpm` commands** in `player/README.md`. Developers cd into `player/` and run pnpm directly.
- **B — Wrap as Makefile targets** at the repo root: `make player-dev`, `make player-test`, etc.

## Options considered

### Option A: Raw pnpm in README
- **Pros**: Less ceremony — pnpm commands are themselves the contract.
- **Cons**: Splits the project's dev workflow into two muscle-memory patterns (Go via Makefile, TS via pnpm). Forgetting to `cd player/` first is a regular paper cut. Breaks the CLAUDE.md mandate.

### Option B: Makefile wrappers
- **Pros**: One muscle-memory pattern across the whole project. `make help` lists every action. Future CI / pre-commit / docs all reference one source of truth. Honors CLAUDE.md directly.
- **Cons**: Tiny duplication — the Makefile targets are one-liners that cd into `player/` and run pnpm.

## Decision

**Choose B.** Add five targets to the root `Makefile`:

- `player-dev` — `cd player && pnpm install && pnpm dev`
- `player-build` — `cd player && pnpm install && pnpm build`
- `player-test` — `cd player && pnpm install && pnpm test`
- `player-fixture-silent` — regenerate the synthetic silent WAV via `make_silent_wav.py`
- `player-fixture-kokoro` — regenerate the fixture from real Kokoro output

Update the `make help` block so all five appear in the documented surface.

## Consequences

- Players's `pnpm-lock.yaml` becomes the canonical lockfile; `pnpm install` is idempotent so the wrapper targets don't bloat repeat runs much.
- A developer who prefers raw pnpm can still `cd player && pnpm <cmd>` — the Makefile wraps, it doesn't restrict.
- New player dev actions added in future tickets must be added as Makefile targets too, to stay consistent with the rule.

## Related decisions

- `CLAUDE.md`'s "drive all repeatable dev actions through Makefile" rule is the parent constraint; this is its application to the player.
