# Ephemeral sink default `play` seam is stubbed; real `afplay` is opt-in behind `//go:build manual`

- id: 2026-06-19-ephemeral-stubbed-play-seam-build-tag
- date: 2026-06-19
- status: accepted
- category: convention
- tags: [sink, ephemeral, testing, build-tags, phase-one]

## Decision

`sink/ephemeral` exposes a package-level `play` function variable that the unit suite overrides with a no-op stub. The real `afplay` subprocess path is exercised only by `ephemeral_smoke_test.go`, gated by `//go:build manual` and run manually via:

```
go test -tags=manual ./sink/ephemeral/...
```

`go test ./...` runs the unit suite with the stubbed seam — no audio hardware required, no platform assumptions.

## Why

Unit tests must run on any machine. CI (when it eventually arrives), Linux dev boxes, and any contributor without macOS-flavored `afplay` available all need a green `go test ./...`. The seam pattern lets the same code path that production uses (`play(ctx, wavPath)`) be substituted with a synchronous no-op in tests, keeping the assertion surface honest (we test that `play` was called with the right args, not that audio actually emerged from speakers).

Mirrors the renderer's smoke-test pattern established in issue #5 — keeps "real-device verification" a separate, explicit invocation across the codebase.

## Rejected alternatives

- **Env-var gating (`EPHEMERAL_REAL_AFPLAY=1`).** Rejected because env-var control is invisible in `go test ./...` output and can silently change behavior between runs based on shell state. Build tags are explicit at compile time — the test either is or isn't in the binary, no hidden modes.
- **Always call real `afplay`; mock at the OS level in tests.** Rejected because it makes the test suite platform-dependent (no `afplay` on Linux) and hostile to fast iteration (tests would emit audio).
- **Make `play` an interface field on `EphemeralSink` rather than a package variable.** Considered — cleaner OO but adds a constructor parameter every caller has to pass or default. Package-level var is the minimum-ceremony seam for phase one. Revisit if a second sink backend wants the same seam shape (likely it will want its own).

## Related decisions

- [Phase-one subprocess timeouts: 60s per-block, 10min wall](2026-06-18-subprocess-timeouts-60s-10min.md) — renderer-side companion to this; both follow the same "real subprocess behind a tagged smoke test" pattern.
