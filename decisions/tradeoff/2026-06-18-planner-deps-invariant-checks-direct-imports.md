# Planner deps invariant checks direct .Imports, not transitive -deps

| Field    | Value            |
|----------|------------------|
| Date     | 2026-06-18       |
| Status   | accepted         |
| Category | tradeoff         |
| Tags     | planner, invariant, dependencies, goldmark, testing, phase-one |

## Context

CLAUDE.md declares a load-bearing invariant: `planner/` must not perform I/O. Concretely: no imports of `os`, `net`, `net/http`, `io/ioutil`, `syscall`, and no concrete adapter / render / sink / intelligence-impl package. The mirror tests in `plan/deps_test.go`, `adapter/deps_test.go`, and `intelligence/deps_test.go` use `go list -deps` (transitive) to enforce their own zero-internal-deps invariants.

When `planner/` adopted `goldmark` as the CommonMark segmenter (design doc §1 explicitly sanctions this), the transitive-deps check began failing because goldmark itself imports `net/url`, `regexp` (which pulls in `syscall` on most platforms), and `os`. None of these are reachable by planner source code; they're entirely inside goldmark's tree.

## Options considered

### Option A: check direct `.Imports` only (chosen)
- **Pros**: enforces what the invariant actually says — "planner source files must not reach for I/O"; allows goldmark to exist without leakage; symmetric with `intelligence/deps_test.go`'s logic conceptually.
- **Cons**: a future maintainer who imports `os` indirectly via a sister package wrapper would slip past the test. Mitigation: forbidden internal prefixes still detect that case.

### Option B: keep `-deps` (transitive) and add an allowlist for goldmark's transitive deps
- **Pros**: still catches accidental indirect leakage.
- **Cons**: every minor goldmark bump may add a new transitive dep that fails the test for unrelated reasons; the allowlist becomes a maintenance tax.

### Option C: drop the deps invariant for planner/ entirely
- **Pros**: simplest.
- **Cons**: loses the load-bearing CLAUDE.md guarantee; future PRs could silently introduce `os.ReadFile` in `planner.go`.

## Decision

`planner/deps_test.go` shells `go list -f "{{range .Imports}}{{.}}\n{{end}}"` on the planner package and checks the resulting direct-imports list against the forbidden stdlib set + the forbidden internal-prefix list. Transitive deps are not inspected.

The test is named `TestInvariant_NoDirectIOImports` (renamed from the original `_NoForbiddenDeps` to make the scope explicit).

Other packages' deps tests keep their `-deps` (transitive) scope unchanged because they don't import goldmark and their invariant is "this package has no internal-project deps at all" — a stricter and different rule.

## Consequences

- The check matches the CLAUDE.md invariant's intent exactly: source-file reaching for I/O.
- The check accepts that the sanctioned segmenter library uses stdlib in ways planner code doesn't directly observe.
- Naming is explicit (`_NoDirectIOImports`) so the scope is documented in the test name.
- If a future maintainer needs to import a different stdlib I/O package indirectly via planner code, they will fail this test and be forced to either justify the change in code review or refactor.

## Related decisions

- [Plan zero-deps invariant enforced via `go list -deps` subprocess](../schema/2026-06-18-plan-zero-deps-via-go-list-subprocess.md) — same enforcement mechanism, different scope (transitive vs direct).

## Revisit trigger

- A non-goldmark library is added to `planner/` that the segmenter doesn't need (e.g. a regex engine, a YAML parser). Re-evaluate whether direct-only is still safe.
- Goldmark is replaced with a different segmenter — re-tighten to `-deps` if the replacement has no transitive I/O.
- A bug surfaces where indirect I/O via a planner-internal wrapper slipped past the test.
