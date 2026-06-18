# Zero-deps invariant enforced via `go list -deps` subprocess

| Field    | Value            |
|----------|------------------|
| Date     | 2026-06-18       |
| Status   | accepted         |
| Category | schema           |
| Tags     | plan, zero-deps, testing, invariant, go-tooling |

## Context

`plan/` must import nothing from this project and nothing externally beyond
the Go standard library. This is a load-bearing architectural invariant (see
CLAUDE.md "Invariants" + the design doc): if `plan/` gains an internal
dependency, the dependency direction inverts and the schema contract becomes
coupled to its consumers.

The invariant needs a runnable check that fails loudly the moment a future
commit accidentally adds `import "github.com/vd09-projects/intelligent-tts-narration-library/something"`
into `plan/`. Two approaches were considered.

## Options considered

### Option A: In-process AST traversal via `go/build` or `golang.org/x/tools/go/packages`
- **Pros**: no subprocess; pure Go; faster on cold caches; can be more
  expressive (filter by import-set, parse build tags, etc.).
- **Cons**: `golang.org/x/tools/go/packages` is a non-stdlib dep that would
  itself be a `plan/` import — recursive paradox. `go/build` is stdlib but
  more verbose to use correctly; build-tag and vendor-dir handling are
  subtle.

### Option B (chosen): Shell `go list -deps <pkg>` from a `*_test.go`
- **Pros**: mirrors the human invariant check (the AC literally says
  "`go list -deps ./plan/...` shows zero internal-project imports"). Drift
  produces stderr a developer can copy-paste-investigate. Uses only stdlib
  `os/exec`. `_test.go` files don't affect package imports, so no zero-deps
  paradox.
- **Cons**: requires `go` on PATH; slightly slower than in-process; skipped
  when `go` isn't available (test calls `t.Skip` rather than failing).

## Decision

**Option B.** `plan/deps_test.go` shells `go list -deps github.com/vd09-projects/intelligent-tts-narration-library/plan`,
scans the output for any line beginning with the module path, and fails if
any line other than the self-package appears.

Scope the check by the **module-qualified import path**, not the relative
pattern `./plan/...`. Reason: `go test` runs with the package directory as
cwd, so the relative pattern `./plan/...` would resolve to `plan/plan/...`
inside the test process — wrong directory. The module-qualified path is
cwd-independent.

`t.Skip` when `go` is not on PATH (minimal container environments) rather
than failing — the invariant matters on developer machines and standard CI
where `go` is present.

## Consequences

- A future commit that accidentally adds an internal import to `plan/` fails
  CI with a copy-pasteable list of the offending dependencies.
- The test depends on the `go` tool's output format remaining stable. Go has
  promised this for `go list` for many releases; risk is low.
- `t.Skip` means a minimal Docker image that runs `go test` from a prebuilt
  binary cache might miss the invariant. The fallback for that environment
  is reading the module graph during the build step (not yet implemented;
  out of scope for phase one).

## Related decisions

- [PlanID — ULID stdlib only](2026-06-18-plan-id-ulid-stdlib-only.md) — the
  ULID decision is the other place where the zero-deps invariant constrains
  implementation choice.
- [Testdata fixtures verbatim from §2.7](2026-06-18-plan-testdata-verbatim-from-design-doc.md) — sister invariant; both are tests
  that fail at PR review on drift.

## Experiments

`TestInvariant_ZeroInternalDeps` in `plan/deps_test.go`. Verified manually:
the test fails (as expected) when a temporary import line is added to
`plan/plan.go`; passes after revert.

## Revisit trigger

Revisit if:
- The Go team changes `go list -deps` output format in a way that affects
  parsing.
- The project gains a build system other than vanilla `go build` (Bazel,
  Pants) — at that point the subprocess approach may not see the full
  dependency graph, and an in-process traversal becomes necessary.
- `plan/` legitimately needs to depend on a sibling first-party module
  (e.g. a shared schema-version helper) — the invariant should be loosened
  to allow that specific edge, not abandoned.
