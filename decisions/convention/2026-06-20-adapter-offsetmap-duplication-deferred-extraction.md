# adapter offset-map line walker duplicated between file + mcptext; shared adapterutil deferred

- **Date:** 2026-06-20
- **Status:** accepted
- **Category:** convention
- **Tags:** [adapter, mcptext, file, offsetmap, duplication, speculative-abstraction, issue-17]
- **Owner:** vd
- **Scope:** issue-17

## Context

The CLAUDE.md adapter contract requires every concrete InputAdapter to emit a byte→line `OffsetMap` with identical shape: one `OffsetSpan` per source line, byte offsets (EndByte exclusive), CRLF preserved in the span, trailing-no-newline emitting a partial-line span, lines 1-indexed. `adapter/file` shipped this in #3; `adapter/mcptext` needs the same shape so the planner (which sees only `RawDocument`) cannot tell file from inline text.

The line-walking logic is ~30 lines of straightforward byte iteration: `buildOffsetMap` + `estimateLineCount`. With one adapter, the code lived in `adapter/file`. With two, the natural Go move is to factor it out — typically into `adapter/internal/adapterutil` or a sibling package — so both adapters call one implementation.

The Don't-Repeat-Yourself instinct fights the rule-of-three / two-consumers-is-too-thin rule. With only two adapters today and a planned third (`adapter/ocr`, deferred-phase per CLAUDE.md), lifting now would be speculative abstraction: we don't yet know what the OCR adapter's offset-emission needs are. OCR may not even produce a line-based offset map (it may produce a bounding-box-to-character-span map keyed on `SourceKindCharSpan` instead of `SourceKindLineRange`).

## Options considered

### Option A: duplicate `buildOffsetMap` + `estimateLineCount` byte-for-byte in `adapter/mcptext` (CHOSEN)
- **Pros**: Zero speculative abstraction. Both consumers see identical code in their own packages; reviewing parity is a side-by-side diff. Adapter/ocr's eventual shape informs the helper's signature when it actually exists.
- **Cons**: ~30 lines duplicated. Drift risk if one adapter changes the rule (e.g. swallow trailing newline) and the other does not. Mitigated by both files being tagged `# Decision: convention — duplicated from adapter/<other>` package-doc comments pointing at this decision.

### Option B: lift to `adapter/internal/adapterutil.BuildOffsetMap` now
- **Pros**: One source of truth; CRLF + trailing-newline rule lives in one file.
- **Cons**: Speculative — adapter/ocr may not need a line-based map at all (charspan instead). If it does, the helper's signature still has to be guessed. Two consumers is below the rule-of-three threshold the project has been honoring elsewhere (mcpsampling prompts, decision-journal templates). Adds a package layer with no current bug to fix.

### Option C: shared internal package via `internal/adapterio`
- **Pros**: Same as B but routes through `internal/` so it's not a public surface.
- **Cons**: Same speculative-abstraction problem as B; the `internal/` placement is moot when both callers are themselves siblings under `adapter/`.

## Decision

Duplicate the offset-map logic in `adapter/mcptext` rather than lifting. Mark the duplication explicitly in both files' source-level docstrings (top-of-package comment + on `buildOffsetMap`) so a future reader does not assume it's an oversight. Lift to a shared helper when the third byte-emitting adapter arrives — that adapter's actual shape will dictate the helper's signature instead of forcing it from a 2-consumer guess.

## Consequences

- ~30 lines of duplicated code in `adapter/file/file.go` and `adapter/mcptext/mcptext.go`. The duplication is byte-for-byte identical today; future readers should compare with `diff` if uncertain.
- Drift surface: if the CRLF or trailing-newline rule changes in one file but not the other, the planner sees inconsistent OffsetSpans depending on which adapter was wired. Test parity is the guard — `mcptext_test.go` is intentionally a mirror of `file_test.go` (same fixtures: empty, single partial line, multi-line LF, CRLF, UTF-8 multibyte). A change to the rule in one adapter must update both test files to match, which will flag the drift at PR review.
- When the third adapter lands, this decision is the revisit point. The follow-up is a refactor PR: extract `buildOffsetMap` + `estimateLineCount` to a shared package, update both `adapter/file` and `adapter/mcptext` to call into it, and supersede this decision.

## Related decisions

- [mcptext URI carries sha256(text); adapter cross-checks on Read](2026-06-20-mcptext-uri-sha256-cross-check.md) — companion decision from the same ticket; that one is about provenance, this one is about implementation duplication.
- [DefaultLexicon shipped frozen + user-overridable via WithLexicon](2026-06-18-default-lexicon-shipped-frozen-overridable.md) — similar precedent (lift when a real second consumer + shape appears).
- [mcpsampling prompt templates stay inside the package for #13](2026-06-20-mcpsampling-prompt-templates-stay-in-package-for-13.md) — same speculative-abstraction-deferred pattern, also pegged to "lift when the second/third real consumer lands".

## Revisit trigger

Adapter/ocr (or any third byte-emitting adapter) lands. At that point the helper's signature is informed by three concrete shapes rather than two. The follow-up PR is mechanical: move + import-rewrite + delete duplicates.
