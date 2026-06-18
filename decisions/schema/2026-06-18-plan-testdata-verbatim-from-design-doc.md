# Testdata fixtures committed verbatim from design doc §2.7

| Field    | Value            |
|----------|------------------|
| Date     | 2026-06-18       |
| Status   | accepted         |
| Category | schema           |
| Tags     | plan, testdata, fixtures, schema, documentation-drift |

## Context

The design doc (`docs/solution-phase-design.md` §2.7) shows two canonical JSON
examples for the narration plan: a voiced config block and a refused
bare-image block. These examples are referenced from the problem statement and
are how a non-Go consumer (React player, MCP client author) learns the schema.

We need round-trip JSON tests in `plan/` that decode → re-encode → re-decode
→ reflect-equal. The question was: do the fixtures used in those tests live
inline in Go test files (string literals), get generated from the Go types, or
get committed verbatim from the design doc?

## Options considered

### Option A: Inline Go string literals in `plan_test.go`
- **Pros**: tests are self-contained; no separate testdata/ tree.
- **Cons**: the canonical JSON in the design doc and the JSON in the test
  drift the moment one of them is edited. No machine-detectable link.

### Option B: Generate fixtures from Go types in a `TestMain` setup
- **Pros**: always in sync with the Go types.
- **Cons**: defeats the purpose. Tests would prove `Go encodes Go correctly`,
  not `the schema we publish matches our consumers' contracts`. Round-trip
  passes trivially even if the wire format silently changes shape.

### Option C (chosen): Verbatim files under `plan/testdata/`, copied from §2.7
- **Pros**: design-doc drift is caught by `go test ./plan/...` failing at
  PR review time. The fixtures are the contract; the Go types must conform
  to them, not the other way around. Non-Go consumers can `curl` the raw
  files to learn the schema.
- **Cons**: when the design doc legitimately changes (new schema feature),
  fixtures and doc must be updated in lockstep. CLAUDE.md already calls this
  out as a `quality-overrides` rule for `plan/`.

## Decision

**Option C.** Commit three fixtures under `plan/testdata/`:

- `example_voiced_config.json` — verbatim from §2.7 voiced-block example.
- `example_refused_image.json` — verbatim from §2.7 refused-block example.
- `example_full_plan.json` — composes both blocks into a `NarrationPlan`
  with `schema_version`, `plan_id`, `source`, `defaults`, etc. Provides
  end-to-end round-trip coverage that §2.7's per-block fragments cannot.

Round-trip tests decode → re-encode → re-decode → `reflect.DeepEqual`, plus
spot-checks on load-bearing field values (status, refusal reason, segment
text contains "Replicas set to three") to catch silent string-format drift.

## Consequences

- Any future change to `plan/` types that affects JSON-on-wire shape will
  fail the round-trip test until either (a) the fixture is updated to match
  the new shape (and §2.7 is updated in the same PR), or (b) the Go change
  is reverted.
- Sindri's `quality-overrides` for `plan/` (per CLAUDE.md) require fixture
  + schema-version review on every schema change — this Decision is the
  mechanism that makes that policy enforceable.
- The fixtures are the publishable schema sample. Non-Go consumers should
  pin to commit hashes of these files, not to docs that may be paraphrased.

## Related decisions

- [PlanID — ULID stdlib only](2026-06-18-plan-id-ulid-stdlib-only.md) — the
  full-plan fixture's `plan_id` field is one place where ULID character class
  is implicitly pinned.
- [Zero-deps invariant via `go list -deps` subprocess](2026-06-18-plan-zero-deps-via-go-list-subprocess.md) — sister invariant; both are tests that catch drift the moment it lands.

## Experiments

`TestRoundTrip_VoicedConfigBlock`, `TestRoundTrip_RefusedImageBlock`,
`TestRoundTrip_FullPlan` in `plan/plan_test.go`. Run on every commit.

## Revisit trigger

Revisit if:
- The schema majors (e.g. `1.0` → `2.0`). At that point the §2.7 examples
  may need to fork into v1 and v2 fixtures for compatibility tests.
- Non-Go consumers (React player, MCP client) start consuming the fixtures
  directly from this repo — at that point we may want to publish them under
  a stable URL or copy them into a public docs path.
