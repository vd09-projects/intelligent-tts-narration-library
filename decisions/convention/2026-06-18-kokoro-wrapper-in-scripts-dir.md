# Kokoro wrapper script lives in `scripts/`, not `render/sherpa/`

- id: 2026-06-18-kokoro-wrapper-in-scripts-dir
- date: 2026-06-18
- status: accepted
- category: convention
- tags: [render, kokoro, subprocess, layout, phase-one]

## Decision

The Python venv launcher (`scripts/kokoro`) and runner (`scripts/kokoro_runner.py`) live under `scripts/` at project root. `render/sherpa/` (Go) calls into them via `os/exec`. The default binary path is `./scripts/kokoro` (cwd-relative), overridable via `sherpa.EngineConfig.BinaryPath`.

## Why

The wrapper is a developer-environment artifact: it activates the venv, calls the Python runner, manages model file paths. None of that is Go-language code, and none of it is `render/sherpa/`-specific (a future Piper renderer would put its own launcher under `scripts/` too). `scripts/` is the conventional Go-project home for cross-stack glue.

Co-locating the launcher inside `render/sherpa/` would imply the Go package owns the Python runtime, which it doesn't. It also conflicts with `go install ./...` semantics: tooling expects `scripts/` to be ignored by builds, but a Go package directory containing a shell script would confuse tools (Go test framework would try to copy it, linters would scan it, etc.).

## Rejected alternatives

- **`render/sherpa/scripts/kokoro`** — co-locates the Python runtime with the Go renderer, but breaks the convention that Go package dirs hold Go source. Tools would scan it.
- **`render/sherpa/kokoro.sh` (single file)** — same problem at smaller scope.
- **No wrapper; renderer activates venv inline via Go** — would require Go to know about Python venvs, sourcing bash init files, etc. Bash is the right tool for that job.
