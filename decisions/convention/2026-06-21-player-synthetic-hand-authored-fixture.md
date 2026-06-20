# Player fixture is synthetic, committed, hand-authored (not real Kokoro output)

| Field    | Value            |
|----------|------------------|
| Date     | 2026-06-21       |
| Status   | accepted         |
| Category | convention       |
| Tags     | player, fixture, kokoro, demo, issue-18 |

## Context

The reference player needs a fixture under `player/public/fixtures/sample/` so `pnpm install && pnpm dev` produces a demo on first run (see sibling decision on dual data loading). Two fixture strategies:

- **A — Real Kokoro output.** Run `make run-persistent SAMPLE=docs/samples/sample.md OUT=player/public/fixtures/sample`. Plays real synthesized audio.
- **B — Synthetic hand-authored.** Hand-write `plan.json` + `manifest.json` to mirror the shape of `docs/samples/sample.md`'s blocks. Generate a 24 kHz mono PCM-16 silent `audio.wav` programmatically.

The fixture's job is to prove the player loads, renders all UI branches (every `Class` enum, every `Status` enum), and exercises the 8 ACs. It is *not* required to deliver demo audio worth listening to.

## Options considered

### Option A: Real Kokoro output
- **Pros**: Audio actually plays; demo feels alive.
- **Cons**: Regenerating the fixture requires a working Python venv + downloaded weights + working subprocess — gating fresh-checkout `pnpm install && pnpm dev` on a heavy non-JS toolchain that has nothing to do with the player.

### Option B: Synthetic hand-authored
- **Pros**: Regeneration is a single Python script (`make_silent_wav.py`) — no Kokoro, no weights. Fixture deterministically covers every `Class` + every `Status`. CI-friendly. Reviewer-friendly: each block is hand-tuned to a known shape.
- **Cons**: Audio is silent — the demo proves the UI, not the audio quality. Manual checking of real Kokoro output is one extra `make` target away.

## Decision

**Choose B.** Hand-author `plan.json` + `manifest.json` mirroring `docs/samples/sample.md`'s block layout (heading + prose + code + table + list + refused image). Cover every `Class` enum (`heading`, `prose`, `code`, `table`, `list`, `unknown`) and every `Status` enum (`voiced`, `degraded`, `refused`) so all UI branches render in the demo. Generate `audio.wav` via `player/public/fixtures/make_silent_wav.py` — 24 kHz mono PCM-16, ~2 s of zeros.

Add `make player-fixture-silent` to regenerate the synthetic WAV. Add `make player-fixture-kokoro` for the developer who wants real audio.

## Consequences

- The committed fixture is small (silent WAV ≈ 100 kB) and reviewable in source.
- A `player/public/fixtures/sample/README.md` documents the fixture contract and how to regenerate from real Kokoro.
- Fixture drifts from real `plan.json` shape if the Go schema bumps — mitigated by the TS type mirror in `player/src/types/plan.ts` and the smoke test loading the fixture under the same schema.

## Related decisions

- [[2026-06-21-player-dual-data-loading-fixture-and-picker]] — why the bundled fixture exists at all.
- [Single canonical demo doc at docs/samples/sample.md](./2026-06-18-single-canonical-demo-doc.md) — fixture mirrors this doc by design.
