# sink/ephemeral testdata stays a smoke-test skip message, no committed WAV fixture

| Field    | Value            |
|----------|------------------|
| Date     | 2026-06-21       |
| Status   | accepted         |
| Category | convention       |
| Tags     | sink/ephemeral, testdata, golden-audio, smoke-test, build-tags, issue-11 |

## Context

Issue #11 AC#7 offered a choice: "replace `.gitkeep` with a small real WAV fixture OR a skip message in the smoke test." The `//go:build manual` smoke test (`ephemeral_smoke_test.go`) plays `testdata/5s.wav` through real afplay; that file is not committed and `testdata/.gitkeep` is the placeholder.

## Options considered

### Option A: keep the skip message, no committed WAV
- **Pros**: aligns with CLAUDE.md "no golden audio (audio validated by ear during /verify)"; no binary in the repo; manual-tag run stays green on a fresh checkout because the test `t.Skip`s with a generate-via-`scripts/kokoro` hint when the file is absent.
- **Cons**: the real-afplay smoke path only runs after a contributor generates the fixture locally.

### Option B: commit a real or synthetic WAV fixture
- **Pros**: smoke test runs out of the box under `-tags=manual`.
- **Cons**: contradicts the no-golden-audio rule; lands a binary in a local-only hobby repo.

## Decision

Keep the skip-message branch and `testdata/.gitkeep`. The smoke test already `t.Skip`s cleanly when `testdata/5s.wav` is missing. No WAV committed. Recorded explicitly so a future contributor does not "fix" the missing fixture by committing a binary.

## Consequences

- The `-tags=manual` smoke test is a no-op on a fresh checkout until a fixture is generated locally — acceptable, since manual audio validation happens via `/verify`, not golden files.

## Related decisions

- [Ephemeral default play seam is stubbed; real afplay behind //go:build manual](../convention/2026-06-19-ephemeral-stubbed-play-seam-build-tag.md) — same testing posture: real audio is opt-in, never default.

## Revisit trigger

If the project ever adopts golden-audio validation or a CI lane with audio hardware, revisit committing a canonical fixture.
