# Typed-enum pattern wins for all enum-shaped string fields in plan/ (Severity, SayAs, Emphasis)

- **Date:** 2026-06-20
- **Status:** accepted
- **Category:** schema
- **Tags:** plan, enum, severity, sayas, emphasis, ssml, additive-compatible, refactor, issue-10, issue-13

## Context

`plan/` historically used two patterns for enum-shaped string fields. Six enums — `Status`, `Class`, `RefusalReason`, `SourceKind`, `SegmentKind`, `Level` — were typed string aliases in `plan/enums.go` with `IsValid()` methods. Three fields — `Diagnostic.Severity`, `VoicingDirective.SayAs`, `VoicingDirective.Emphasis` — were bare `string` with allowed values listed in line comments. The inconsistency was flagged as a deferred-from-#2 review finding (lens 3 + lens 4) and tracked as GitHub issue #10.

## Decision

Adopt the typed-enum pattern for the three remaining freeform fields. Three new types in `plan/enums.go`, each carrying the same shape as the existing six:

- `type Severity string` with `SeverityInfo`, `SeverityWarning` + `IsValid()`.
- `type SayAs string` with `SayAsCharacters`, `SayAsDigits`, `SayAsVerbatim` + `IsValid()`.
- `type Emphasis string` with `EmphasisNone`, `EmphasisModerate`, `EmphasisStrong` + `IsValid()`.

The pattern in code (shape shared across all nine enums in `plan/enums.go`):

```go
type Severity string
const ( SeverityInfo Severity = "info"; SeverityWarning Severity = "warning" )
func (s Severity) IsValid() bool { /* switch over known values */ }
```

Field types in `Diagnostic` and `VoicingDirective` (in `plan/plan.go`) changed from bare `string` to the new alias types. Inline value-listing comments dropped — the typed-enum + named constants replace them as documentation.

**Severity is intentionally 2-valued.** `Diagnostic.Severity` stays at `Info` + `Warning`. Pipeline-stopping conditions surface as Go `error` (per CLAUDE.md: *"adapter I/O failure = error returned up the pipeline (stops). Readable-but-unvoiceable content = Refusal inside the plan. Never both."*), never as a `SeverityError` diagnostic. `Diagnostic` lives on the data side of that boundary; introducing `SeverityError` would re-import a concept the architecture deliberately exiled.

**Validation hook point.** Producers SHOULD call `IsValid()` before emitting a value into a `Diagnostic` / `VoicingDirective`. Consumers MAY check on ingest. The JSON layer never enforces — round-trip preserves unknown values per [[2026-06-18-plan-testdata-verbatim-from-design-doc]].

## Justification

- Go's `encoding/json` treats `type X string` identically to bare `string` at the wire level — testdata fixtures (verbatim from design doc §2.7) round-trip byte-for-byte.
- `IsValid()` is advisory (not a JSON gate), so the additive-compatibility invariant from CLAUDE.md is preserved.
- The existing six typed enums already tolerate unknown values via this same mechanism — the "open enum" property does not actually distinguish freeform from typed.
- `TestForwardCompat_UnknownEnumValuesRoundTrip` (in `plan/plan_test.go`) extended with three new probes (`diagnostic_severity`, `voicing_say_as`, `voicing_emphasis`) to prove behavior parity. All 9 sub-cases now confirm uniform additive-compat behavior across all 9 enums.

## Rejected alternatives

- **Option B — document the freeform choice.** Add a package-doc paragraph saying these three fields intentionally stay freeform because `SayAs` and `Emphasis` overlap with SSML, and `Severity` may grow. Rejected because the "open enum" property doesn't actually distinguish the three freeform fields from the six typed ones — the existing typed enums already round-trip unknown values. Option B defends a split that the open-enum property does not justify.
- **Option C — hybrid (typed `Severity`, freeform `SayAs`/`Emphasis`).** Recognize SSML as a real external constraint and clean up only the project-internal field. Rejected because it preserves a two-pattern split for a cosmetic SSML-tracking reason that doesn't outweigh the consistency win. If a future SSML revision adds a value, the cost is one new named constant per value — minor maintenance, not a contract break.

## Consequences

- All nine enum-shaped string fields in `plan/` now share the same shape: `type X string` + named constants + receiver `IsValid()` switch.
- The pattern is uniform from a reviewer's perspective — no "these three are special" exception needed.
- Consumers gain `IsValid()` they can use during plan validation if they want; it is not enforced at JSON decode time.
- A small Go-source-level compatibility note: any external code assigning a runtime `string` variable into one of these fields (`d.Severity = mystr` rather than `d.Severity = plan.Severity(mystr)`) would now fail to compile. String literals (`d.Severity = "info"`) still compile via Go's untyped-constant rule; only typed `string` variables need explicit conversion. Phase-one is local-only with no external Go consumers, so the blast radius is contained.
- Fixtures under `plan/testdata/` are pinned to the current enum set per [[2026-06-18-plan-testdata-verbatim-from-design-doc]]. Adding a value later is additive at the wire level but requires a fixture append to keep coverage honest.

## Related decisions

- [2026-06-18-plan-zero-deps-via-go-list-subprocess](2026-06-18-plan-zero-deps-via-go-list-subprocess.md) — the zero-deps invariant that constrained this refactor (no MarshalJSON helper packages allowed).
- [2026-06-18-plan-id-ulid-stdlib-only](2026-06-18-plan-id-ulid-stdlib-only.md) — same stdlib-only discipline applied to a different `plan/` concern.
- [2026-06-18-plan-testdata-verbatim-from-design-doc](2026-06-18-plan-testdata-verbatim-from-design-doc.md) — the testdata-verbatim policy that proves the wire format didn't change.

## Revisit trigger

If a future SSML revision publishes new `SayAs` or `Emphasis` values, adopt them as additive new named constants. No contract break required — unknown values from clients on newer SSML versions continue to round-trip in the meantime.

Also revisit if a new producer (intelligence adapter, render edge, sink) wants to emit a value not in the current set — particularly a `Severity` level the 2-value scheme cannot express. Issue #13 (intelligence/mcpsampling) is the first non-planner producer of `Diagnostic`; its cases map cleanly under the 2-value scheme (transport timeout = Go `error`; malformed-JSON-recovered-to-verbatim = `SeverityWarning`; cache-miss-on-escalation = no diagnostic, internal accounting only). Future adapters that want a third level should propose it here first.

## Related issues

- #10 — the refactor that introduced the typed-enum pattern (planner-side validation).
- #13 — the first non-planner producer of `Diagnostic`; validating use case for the 2-value `Severity` boundary.

## Source

Inline mark `**Decision (1) — schema: accepted.**` emitted in the planner-task.md for scope `plan-enum-consistency-issue-10`, harvested at session close via decision-journal Harvest mode. Implemented in PR https://github.com/vd09-projects/intelligent-tts-narration-library/pull/22, commits `98cfa52` + `dca64d1`.

## Amendment 2026-06-20 (post-merge, pre-#13)

Reviewer surfaced coverage gap during #13 plan review: the Decision did not name the boundary between `Diagnostic.Severity` and Go `error`, and did not name #13 as the validating use case. Amendment adds:
- Boundary made explicit (Severity stays 2-valued; pipeline-stopping uses Go `error`).
- Validation hook point named (producers SHOULD call `IsValid()`; JSON never enforces).
- Untyped-constant clarification (string literals still compile).
- Test file pointer named (`plan/plan_test.go`).
- 3-line code pattern snippet added to Decision body.
- Consequence on `plan/testdata/` fixture pinning added.
- Revisit trigger broadened to cover new producers.
- Cross-link to #13 (first non-planner producer of `Diagnostic`).
- Tags include `issue-13`.

No change to the substantive Decision: `Severity` remains 2-valued (Option A). Reasoning preserved verbatim above.
