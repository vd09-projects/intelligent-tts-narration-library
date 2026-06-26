# Table L2/L3 degrade gist and levelTable share one parseTable — byte-identity is structural, not test-dependent

| Field    | Value            |
|----------|------------------|
| Date     | 2026-06-27       |
| Status   | accepted         |
| Category | convention       |
| Tags     | planner, levelTable, deterministicTableGist, parseTable, degrade, byte-identical, honesty-rule, dry, issue-47, issue-48 |

## Context

Issue #47 wired intelligence into table voicing at L2/L3 (see
[the table meaning-summary tradeoff](../tradeoff/2026-06-22-table-meaning-summary-via-intelligence-l2-l3.md)),
with a deterministic degrade when no intelligence adapter is wired. That created
the same dual-path byte-identity hazard already seen for code at L2: the table
block must voice **the same deterministic header/row reading** from two places —

1. **`levelTable`** in `planner/level.go`, which reads table *facts* (the
   headers fact, shape, rows) directly; and
2. **The no-adapter degrade path** (`planner/degrade.go`), which falls back to
   the deterministic header/row reading with `Status = degraded` when L2/L3
   requested an AI meaning-summary but no adapter exists.

If these two builders parse the table independently, they can drift — different
cell trimming, header detection, or column counting — and a reader (or golden
fixture) sees a "requested-but-degraded" reading that disagrees with the
deterministic reading for the same table body. The honesty rule depends on the
degraded reading being an exact, non-fabricated rendering of the source cells.

The already-recorded code analog,
[Code L2 deterministic gist shared helper (#48)](2026-06-22-code-l2-deterministic-gist-shared-helper.md),
solved its version by sharing a `deterministicCodeGist` builder — but its
byte-identity rests on a **call-site contract** ("langPhrase MUST be
`codeLangPhrase(lang)` at every call site") that the compiler cannot enforce,
backed only by doc comments + a pinning test. #48's revisit trigger explicitly
anticipated a *second* structured class needing the same dual-path fallback.
Tables are that second class — and the open question here was whether to copy
#48's helper-plus-contract shape or guarantee identity more strongly.

## Options considered

### Option A: Mandatory shared `parseTable` under both paths + a thin `deterministicTableGist` (chosen)
Extract one `parseTable` that produces the canonical parsed table (headers,
columns, rows). **Both** `levelTable` (for the headers fact) and the degrade
gist consume that same `parseTable` output; `deterministicTableGist` is a thin
formatter over it. Byte-identity between the requested-but-degraded reading and
the deterministic reading is then **structural** — they read from the same
parse, so divergence is impossible without changing the shared parser.

- **Pros**: Byte-identity is guaranteed by construction at the *parse* layer,
  not by a per-call-site convention the compiler can't check (strictly stronger
  than #48). The honesty guarantee on the degraded reading is structural. One
  parser is the single source of truth for table structure.
- **Cons**: `parseTable` becomes a mandatory shared dependency — a hot path both
  the fact reader and the gist must route through; a future contributor adding a
  third reader must remember to go through it (mitigated because the parse, not
  just a phrase, is what's shared — there's little reason to re-parse).

### Option B: Copy #48's shape — a shared `deterministicTableGist` builder with a doc-comment call-site contract
Mirror code exactly: one gist builder, both paths call it, identity backed by
doc comment + test.

- **Pros**: Consistent with the established #48 pattern; minimal new surface.
- **Cons**: Inherits #48's weakness — identity rests on a contract the type
  system can't enforce, so a future caller that re-parses the table by hand
  breaks byte-identity silently. Tables had a clean place to make the guarantee
  structural (a shared parse step), so settling for the weaker contract was
  unnecessary here.

## Decision

**Option A — a single mandatory `parseTable` consumed by BOTH `levelTable` (the
headers fact) and the degrade gist, with `deterministicTableGist` as a thin
formatter over that shared parse.** Byte-identity between the no-adapter
degraded reading and the deterministic reading is therefore **structural rather
than test-dependent**.

Rationale: byte-identity is a *requirement* (the honesty rule says the degraded
reading must be an exact rendering of the source cells), and the strongest way
to guarantee a requirement is to make divergence structurally impossible. Tables
offered a cleaner seam than code did: the shared unit is the *parser*, not just
a language phrase, so both readers necessarily agree on structure. This is the
table analog of #48 but a deliberately stronger guarantee — it resolves #48's
"second structured class" revisit trigger by sharing the parse step rather than
lifting a generic `levelResult.deterministicFallback` field (still YAGNI). A
golden-fixture test still pins the behavior, but it now *confirms* a structural
property rather than *holding up* a convention.

## Consequences

- `parseTable` is the single source of truth for table structure; `levelTable`
  and `deterministicTableGist` are thin consumers of it.
- The degraded reading and the deterministic reading cannot drift without
  changing the shared parser — the honesty guarantee is enforced at the parse
  layer, not by a call-site doc comment.
- Stronger than the code (#48) equivalent: where #48 relies on a doc-comment
  contract the compiler can't enforce, the table case shares the parse itself.
  The two classes now use *different* mechanisms for the same goal — acceptable,
  but a future lift to a generic per-class deterministic-fallback mechanism
  should reconcile both.
- `levelResult` stays lean — no per-class `deterministicFallback` field added
  (Option B from #48 still rejected as YAGNI).

## Related decisions

- [Code L2 deterministic gist shared helper (#48)](2026-06-22-code-l2-deterministic-gist-shared-helper.md) — the code analog; this is the table version with a structurally stronger byte-identity guarantee (shared parser vs shared builder + call-site contract). Resolves #48's "second structured class" revisit trigger.
- [Wire intelligence into table meaning-summary at L2/L3 (#47)](../tradeoff/2026-06-22-table-meaning-summary-via-intelligence-l2-l3.md) — the parent cost-model decision; named "degrade.go fallback" as scope. This entry specifies the structural mechanism that fallback uses.

## Revisit trigger

If a third structured class needs the same dual-path deterministic fallback, or
if code (#48) and table diverge enough to cause confusion, reconsider lifting to
a single generic `levelResult.deterministicFallback` mechanism that both the
shared-parser and shared-builder cases can collapse into.
