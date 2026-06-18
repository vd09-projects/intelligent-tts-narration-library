# Pipeline manual smoke test gated by //go:build manual

- **Date:** 2026-06-18
- **Status:** accepted
- **Category:** convention
- **Tags:** [pipeline, testing, build-tags, manual-smoke, phase-one, issue-7]
- **Source:** harvested from cmd-narrate-issue-7 build summary v1, decision mark v5

## Context

The vertical-slice cmd/narrate produces real audio via the ephemeral sink + the Kokoro subprocess via `render/sherpa`. A useful end-to-end smoke test exists — build the binary, run it against `docs/samples/sample.md`, listen for the bare-image refusal — but that test is expensive (real subprocess, real audio) and depends on the developer's environment (`scripts/kokoro` venv set up, speakers on). Default `go test ./...` runs must not invoke it.

A similar problem was already solved on the sink side: `sink/ephemeral` real-afplay is opt-in behind `//go:build manual` (`decisions/convention/2026-06-19-ephemeral-stubbed-play-seam-build-tag.md`). The question was whether to follow that pattern at the pipeline layer or use an env var or a separate `test/` directory.

## Decision

`pipeline/pipeline_manual_smoke_test.go` declares `//go:build manual` at the top. Default test runs (`go test ./...`) skip it; manual runs (`go test -tags manual ./pipeline/...`) include it. The test builds `cmd/narrate` into a temp dir, runs it against `docs/samples/sample.md`, and asserts exit 0 + a parseable `blocks_played=N` summary on stdout. The listener confirms by ear that the bare-image block is refused honestly.

## Rejected alternative

**Env-var gating** (e.g., `if os.Getenv("RUN_MANUAL_SMOKE") != "1" { t.Skip(...) }`). The test would compile and link into every test binary, and would silently skip without a clear signal. `go test ./pipeline/... -v` would print a skip line that the developer either ignores or has to remember. Build tags are explicit, discoverable via `grep -r "//go:build manual"`, and consistent with the sink-side pattern already established.

## Consequences

- Any future end-to-end test that touches real audio, real network, or real subprocess time goes behind `//go:build manual`.
- The manual smoke test runs from the **repo root** (required because `render/sherpa` resolves `./scripts/kokoro` relatively). The test walks up from its package dir to find `go.mod` to make this work whether invoked from the package or the repo root.
- The orchestrator's `/verify` slash command is the natural trigger for `go test -tags manual ./pipeline/...`.

## Related decisions

- Ephemeral sink stubbed play seam build tag (`convention/2026-06-19-ephemeral-stubbed-play-seam-build-tag.md`) — the established sink-side pattern this decision continues.
- Single canonical demo doc (`convention/2026-06-18-single-canonical-demo-doc.md`) — the smoke test runs against `docs/samples/sample.md`.
