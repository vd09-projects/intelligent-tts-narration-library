# Code L2 deterministic gist shared by degrade and size-gate paths (single helper, byte-identical)

| Field    | Value            |
|----------|------------------|
| Date     | 2026-06-22       |
| Status   | accepted         |
| Category | convention       |
| Tags     | planner, levelCode, degrade, size-gate, deterministicCodeGist, codeLangPhrase, honesty-rule, dry, yagni, issue-48 |

## Context

Issue #48's code-at-L2 work (see
[the L2-only tradeoff](../tradeoff/2026-06-22-code-semantic-gist-l2-only.md))
created **two** distinct paths that must voice the *same* deterministic
count+decls gist for a code block at L2:

1. **No-adapter degrade path** (`planner/degrade.go`): L2 asked for an AI
   gist (`needsIntelligence=true`) but no intelligence adapter is wired, so
   the planner degrades to a deterministic gist with `Status = degraded`.
2. **Size-gate path** (`planner/level.go` `levelCode`): the block is over
   `codeGistMaxLines`, so the AI call is skipped and the deterministic gist is
   voiced directly with `Status = voiced`.

Both must produce **the same spoken words** for the same code body — and the
size-gate decision depends on byte-identical output: a gated block is only
indistinguishable from a normal deterministic block if its text matches
exactly. The risk is drift: two independently-maintained gist builders slowly
diverge (different language phrasing, different decl formatting), and a reader
or a golden fixture sees two "deterministic gists" that disagree.

## Options considered

### Option A: One shared `deterministicCodeGist` + `codeLangPhrase` helper (chosen)
Both paths call a single `deterministicCodeGist(body, langPhrase)` that
returns the raw gist text and decl count, where `langPhrase` is *always*
computed via the shared `codeLangPhrase(lang)`. The degrade path and the
size-gate path each construct their segment from that one helper, so output is
byte-identical by construction. Doc comments at both call sites and on the
helper state the contract ("langPhrase MUST be `codeLangPhrase(lang)` at every
call site"). A test asserts the degrade-path segment text equals
`deterministicCodeGist` for the same body.

- **Pros**: Byte-identical output is structural, not hoped-for. Drift between
  the two paths is impossible without changing the shared helper. Makes the
  size-gate's behavioral observability decision sound. Minimal surface — one
  helper, one language-phrase function.
- **Cons**: A shared helper with a stated call-site contract ("always pass
  `codeLangPhrase(lang)`") that the compiler can't fully enforce; relies on
  the doc comment + test to keep both call sites honest.

### Option B: A per-class `levelResult.deterministicFallback` field
Carry the deterministic fallback text on the `levelResult` so the degrade
machinery reads it back generically across classes.

- **Pros**: Generic — degrade could pull a precomputed fallback for any
  structured class uniformly.
- **Cons**: YAGNI. Only code needs this today; adding a field to
  `levelResult` for one class is speculative generality. Bigger struct
  surface, and the field would be dead for every other class. The two code
  paths sharing one helper is the smaller, sufficient fix.

## Decision

**Option A — a single `deterministicCodeGist` helper plus a single
`codeLangPhrase` language-phrase function, shared by both the no-adapter
degrade path and the size-gate path, producing byte-identical voiced text.**
The `levelResult.deterministicFallback` field (Option B) was explicitly
rejected as YAGNI.

Rationale: byte-identical output across the two paths is a *requirement*
(the size-gate observability decision rests on it), and the cleanest way to
guarantee a requirement is to make divergence impossible — one code path that
both call. A struct field would generalize to classes that don't need it and
add permanent surface for a one-class need. The shared-helper contract is
backed by doc comments at both call sites and a test pinning the degrade
segment to `deterministicCodeGist`.

## Consequences

- `deterministicCodeGist` + `codeLangPhrase` are the single source of truth
  for code's deterministic L2 gist; the degrade path and size-gate path are
  thin callers.
- The contract "langPhrase MUST be `codeLangPhrase(lang)` at every call site"
  is enforced by convention (doc comments) + test, not the type system — a
  future caller that hand-rolls the language phrase would break byte-identity
  silently, so the comments are load-bearing.
- `levelResult` stays lean — no per-class fallback field.
- If a second structured class later needs the same dual-path
  (degrade + gate) deterministic fallback, that's the trigger to reconsider a
  generic mechanism (Option B) with two real consumers in hand.

## Related decisions

- [Code L2 size-gate is observed behaviorally](2026-06-22-code-l2-size-gate-observed-behaviorally-no-schema-field.md) — depends on this helper: the gated block's byte-identical deterministic text is what makes the gate invisible in the plan.
- [AI semantic gist for code at L2 only](../tradeoff/2026-06-22-code-semantic-gist-l2-only.md) — the parent decision; `degrade.go` deterministic fallback was named as scope there.

## Revisit trigger

If a second structured class needs the same degrade+size-gate deterministic
fallback, reconsider lifting to a generic `levelResult.deterministicFallback`
(Option B) — with two real consumers, the speculative-generality objection no
longer applies.
