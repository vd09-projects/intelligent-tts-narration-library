# errclass.Class deliberately omits IsValid() (closed internal return type), keeps only String()

| Field    | Value            |
|----------|------------------|
| Date     | 2026-06-21       |
| Status   | accepted         |
| Category | convention       |
| Tags     | internal/errclass, typed-enum, isvalid, string-method, error-classification, issue-51 |

## Context

Task #51 extracted the shared caller-vs-internal-vs-cancel error classification out of `cmd/narrate-mcp` and `cmd/narrate-server` into a new `internal/errclass` package, introducing a `Class int` typed enum (`ClassInternal`, `ClassCaller`, `ClassCancelled`).

The established project convention (the #10 / #23 enum sweep, `plan/enums.go`) is typed-enum-**with-`IsValid()`**: every enum-shaped field carries an `IsValid()` method. That sweep made all ten enum-shaped fields uniform (see [Adopt typed enum + IsValid() pattern for all enum-shaped fields](../schema/2026-06-20-typed-enum-pattern-wins-for-all-enum-shaped.md)). A new typed enum that silently drops `IsValid()` would read as an oversight rather than an intentional departure.

## Options considered

### Option A: Follow the convention verbatim — add IsValid()
- **Pros**: Surface-uniform with every other project enum; no explanation needed.
- **Cons**: `Class` is produced ONLY by `Classify` and never crosses a trust boundary — there is no untrusted input path that could yield an invalid `Class`. `IsValid()` would be unreachable dead code carried purely for symmetry.

### Option B: Omit IsValid(), add only String(), document the departure (chosen)
- **Pros**: No dead code. `String()` earns its place via debuggability and readable test failures. A doc comment on the type explains WHY the convention is broken, so the absence is legibly intentional.
- **Cons**: One enum in the codebase that does not match the others' method set; relies on the doc comment to prevent a future "you forgot IsValid()" change.

## Decision

`errclass.Class` **deliberately departs** from the typed-enum-with-`IsValid()` convention: **no `IsValid()`**. The rationale is the convention's own rationale turned around — the #10/#23 enums carry `IsValid()` because they are parsed from wire, deserialized, or user-supplied and therefore need input validation. `Class` is none of those: it is a **closed INTERNAL return type produced only by `Classify`**, never parsed from wire, never deserialized, never user-supplied. There is no untrusted input to validate, so `IsValid()` would be dead code.

`String()` **is** provided — it is the only method on `Class` — purely for debuggability and readable test failures. The departure is documented in a doc comment on the `Class` type (`internal/errclass/errclass.go`) so the absence of `IsValid()` reads as intentional, not an oversight.

## Consequences

- One project enum intentionally diverges in method set. The doc comment is load-bearing: it is the guard against a well-meaning "add the missing IsValid()" change.
- The convention's true trigger is clarified: `IsValid()` exists for enums that cross a trust/parse boundary, not for every typed enum. Future internal-only enums may follow `Class`'s lead.

## Related decisions

- [Adopt typed enum + IsValid() pattern for all enum-shaped fields](../schema/2026-06-20-typed-enum-pattern-wins-for-all-enum-shaped.md) — the convention this decision deliberately departs from; clarifies the convention applies to wire/parsed enums, not closed internal return types.

## Revisit trigger

If `Class` ever becomes serialized, parsed from wire, or otherwise constructed from untrusted input, the no-trust-boundary premise breaks and `IsValid()` should be added.
