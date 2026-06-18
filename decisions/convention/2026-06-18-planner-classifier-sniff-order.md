# Planner classifier — deterministic priority sniff order

| Field    | Value            |
|----------|------------------|
| Date     | 2026-06-18       |
| Status   | accepted         |
| Category | convention       |
| Tags     | planner, classify, segmentation, determinism, phase-one |

## Context

`planner/classify.go` must turn a `rawBlock` produced by the segmenter into a `plan.Class` (one of `prose | code | config | table | diagram_as_text | example | heading | list | unknown`). Two kinds of evidence are available:

- **Structural hints** from the goldmark AST (or from the plaintext-fallback shape): `hintHeading`, `hintList`, `hintImage`, `hintTable`, `hintCode` (with fence info string), `hintProse`, `hintBlockHTML`.
- **Content sniffing** on the block text: pipe-table shape, YAML/JSON/TOML/INI key:value shape, Mermaid/DOT/PlantUML keywords, ASCII box-drawing characters.

If the order of these checks is left unspecified, two refactors will produce different classifications for the same input. The classifier sits in the path of every downstream voicing decision; subtle reordering changes which level rules apply and what the listener hears.

## Options considered

### Option A: priority chain — structural hints first, then content sniffs (chosen)
- **Pros**: AST-derived evidence is stronger than content matching; deterministic; easy to test rule-by-rule.
- **Cons**: priority order is subjective and must be defended in code.

### Option B: weighted scoring across all rules
- **Pros**: handles ambiguous blocks gracefully.
- **Cons**: harder to test, harder to reason about, behavior changes whenever a rule's weight changes.

### Option C: ML classifier
- **Pros**: handles edge cases naturally.
- **Cons**: forbidden by CLAUDE.md `A13` — phase one is deterministic heuristic only.

## Decision

Apply rules in this fixed order — first match wins:

1. AST hint `headingHint` → `ClassHeading`
2. AST hint `listHint` → `ClassList`
3. AST hint `imageHint` → `ClassUnknown` (refused downstream by honesty rule)
4. AST hint `tableHint` → `ClassTable`
5. AST hint `codeHint`: inspect fence info string
   - `mermaid` / `dot` / `plantuml` → `ClassDiagramAsText`
   - `yaml` / `yml` / `toml` / `ini` / `json` / `properties` / `env` → `ClassConfig`
   - Else → `ClassCode`
6. Plaintext paragraph pipe-table sniff → `ClassTable`
7. Plaintext paragraph YAML/JSON/TOML/INI sniff → `ClassConfig`
8. Plaintext paragraph ASCII / box-drawing diagram sniff → `ClassDiagramAsText`
9. Default → `ClassProse`

`ClassExample` is reserved for future intelligence-driven re-classification (the segmenter / classifier never emits it in phase one). `ClassUnknown` is emitted only for bare-image blocks.

The rule order is documented in `planner/classify.go`'s doc comment and each rule has a matching named test case in `classify_test.go`.

## Consequences

- A future reorder requires updating both the doc comment and the test case names; intentional drift is loud, accidental drift is caught at PR review.
- Plaintext sniffs only run when no AST hint exists — keeps markdown parsing fast and predictable.
- The classifier never throws or refuses by itself; refusal is downstream of `ClassUnknown` in `degrade.go`. The classifier is a pure function of rawBlock → Class.

## Related decisions

- [Plan testdata fixtures committed verbatim from design doc](../schema/2026-06-18-plan-testdata-verbatim-from-design-doc.md) — same anti-drift discipline applied at the schema level.

## Revisit trigger

- A new block class is added to `plan.Class` (e.g. `math`, `quote`).
- The segmenter is swapped away from goldmark (a different AST shape changes the strength of structural hints).
- Intelligence-driven re-classification lands and `ClassExample` starts being emitted.
