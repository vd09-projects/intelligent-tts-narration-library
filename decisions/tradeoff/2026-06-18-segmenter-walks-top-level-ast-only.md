# Segmenter walks top-level AST children only, not the full tree

| Field    | Value            |
|----------|------------------|
| Date     | 2026-06-18       |
| Status   | accepted         |
| Category | tradeoff         |
| Tags     | planner, segmentation, goldmark, source-map, phase-one |

## Context

`planner/segment.go` walks the goldmark AST and converts top-level nodes into `rawBlock` values. Each rawBlock carries text + line range + AST kind hint. The line range is then materialised into a `plan.SourceMap` that drives block-level sync (CLAUDE.md invariant — "sync is block-level only").

Goldmark's AST has nodes at multiple depths. A paragraph may contain inline code spans; a list item contains paragraphs; a blockquote may wrap fenced code. The segmenter has to decide whether to walk all the way down or stop at the first layer of block children.

## Options considered

### Option A: walk top-level children only (chosen)
- **Pros**: `plan.SourceMap` line ranges always correspond to verbatim source slices; a paragraph-with-inline-code stays one block (correct — inline code is still prose); easy to reason about.
- **Cons**: a fenced code block nested inside a blockquote folds into the blockquote's text — its inner fence info is invisible to the classifier.

### Option B: flatten the AST into a single ordered list of leaf blocks
- **Pros**: every fenced code block is independently classifiable regardless of nesting.
- **Cons**: source-map line ranges may overlap (parent + children claim the same lines); ordering across siblings would have to be re-derived; block-level sync — the load-bearing invariant — becomes harder to guarantee.

### Option C: hybrid — walk top-level for normal nodes, descend into containers (blockquote, list-item) that wrap block children
- **Pros**: catches the nested-fenced-code case.
- **Cons**: container detection is goldmark-specific; the rule "what to descend into" multiplies test cases and edge cases; bug surface grows.

## Decision

Walk top-level AST children only. Use `ast.Walk` (goldmark's built-in DFS) inside container nodes whose `Lines()` are empty (lists, tables) just to compute their byte span — never to emit additional rawBlocks. The descent is bounded and only collects line-span data, never produces classification.

This means:
- A blockquote with a nested fenced code block voices the whole quote as prose at L1 (its dominant content), refusing only if it tips into the bare-image / oversized branches.
- Lists and tables (container nodes goldmark exposes with empty top-level `Lines()`) still produce a single rawBlock with correct line ranges.

## Consequences

- Pro: source-map line ranges are always contiguous and non-overlapping; block-level sync is a structural property, not a runtime invariant to defend.
- Con: blockquote-wrapped fenced code goes un-voiced as code. Acceptable because blockquotes are rare in narration source; the alternative would risk fabricating offsets.
- The decision is documented inline in `planner/segment.go` doc comments so future maintainers can find this trail.

## Related decisions

- [Planner classifier sniff order](../convention/2026-06-18-planner-classifier-sniff-order.md) — relies on AST hints being strong; only fires when a top-level node carries a recognisable hint.

## Revisit trigger

- A new common content shape (e.g. tabbed code blocks, container fenced regions) becomes prevalent in narration sources.
- The segmenter is swapped away from goldmark.
- A user reports that blockquote-wrapped code is being voiced incorrectly.
