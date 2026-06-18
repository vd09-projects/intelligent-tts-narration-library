# Two oversized-split thresholds: prose vs structured

| Field    | Value            |
|----------|------------------|
| Date     | 2026-06-18       |
| Status   | accepted         |
| Category | convention       |
| Tags     | planner, level, oversized-split, phase-one, heuristics |

## Context

`planner/level.go` splits oversized blocks before leveling so each chunk fits in audio-coherence limits and gets a meaningful per-class L1/L2/L3 rendering. CLAUDE.md `A7 nuance` (and the sindri/patterns.md amendment) calls out that prose and structured content need different thresholds — but `A7` in the design doc originally lumped them together as ~40 lines / ~1500 chars.

Picking the right thresholds is a judgement call with no field data yet. The decision is whether to encode one threshold or two.

## Options considered

### Option A: separate thresholds (chosen)
- prose: ~20 lines / ~800 chars
- structured (code / config / table / diagram_as_text): ~60–80 lines / ~2000–3000 chars
- **Pros**: prose audio-coherence is the binding constraint for prose; structured content tolerates longer runs because it has clean seams; matches CLAUDE.md `A7 nuance`.
- **Cons**: two sets of constants to defend, two test surfaces.

### Option B: one uniform threshold (e.g. 40 lines / 1500 chars)
- **Pros**: simpler, one constant.
- **Cons**: too-loose for prose (boring narration), too-tight for structured (over-splits valid functions/configs).

### Option C: dynamic threshold based on class L2/L3 leveling cost
- **Pros**: optimal per content.
- **Cons**: not deterministic; complex to test; over-engineered for phase one.

## Decision

Encode two distinct constants in `planner/level.go`:

```
const (
    proseMaxLines      = 20
    proseMaxChars      = 800
    structuredMaxLines = 70    // midpoint of 60-80 range
    structuredMaxChars = 2500  // midpoint of 2000-3000 range
)
```

Split rules per class:
- **prose** — split on sentence boundaries (`.` / `!` / `?` + whitespace) when the block exceeds either threshold; if no sentence boundary exists in a chunk that exceeds the threshold, fall through to leveling (degrade.go decides verbatim-vs-refuse).
- **code** — split at top-level decl keywords (`func`, `type`, `class`, `def`, `function`, `interface`, `struct`).
- **config** — split at top-level YAML keys.
- **table** — split on row boundaries with the header repeated per chunk.
- **diagram_as_text** — never split (diagrams have no clean seam).

Sub-blocks become first-class members of the output block list, with continuous `b001..b00N` IDs and their own source maps derived from the seam line.

## Consequences

- Threshold values are best-effort guesses. They are likely to be tuned once real narration usage produces audible feedback.
- Constants are named (not magic numbers) and discoverable via grep; tuning is a one-line edit.
- Diagrams stay whole — risk: an enormous Mermaid diagram with no node truncation produces a verbose L2/L3 voicing. Acceptable: diagrams that big are rare in narration source and a future "diagram is too dense" refusal can be added.
- The split logic is heuristic and intentionally conservative (`len(starts) < 2` → no split). Un-splittable cases fall through to normal leveling, never crash.

## Related decisions

- [Planner classifier sniff order](2026-06-18-planner-classifier-sniff-order.md) — class drives which split rule applies; both decisions form the leveling pipeline.

## Revisit trigger

- First real-world narration session produces audible "too long" or "over-split" complaints.
- Block-parallel rendering (CLAUDE.md `A17`) lands — split heuristics may need to consider per-block render cost.
- A multilingual phase requires per-locale threshold tuning (e.g. CJK languages have different audio-coherence math).
