// RefusalBlock.tsx — honesty-rule rendering for a refused block (counter-metric).
//
// A refused block renders its spoken refusal MESSAGE + its source map and
// exposes NO level control and NO play affordance beyond the spoken refusal
// (plan Keyboard Interaction Map). It is still a heading-bearing, focusable
// first block so an on-load focus move lands (refusal-first case). Refusal is
// DATA, never an error.

import type { Block } from "../api/types";

function sourceMapLabel(block: Block): string {
  const sm = block.refusal?.source_map ?? block.source_map;
  if (sm.start_line != null && sm.end_line != null) {
    return `lines ${sm.start_line}–${sm.end_line}`;
  }
  if (sm.start_char != null && sm.end_char != null) {
    return `chars ${sm.start_char}–${sm.end_char}`;
  }
  return sm.kind;
}

export function RefusalBlock({ block }: { block: Block }) {
  const refusal = block.refusal;
  return (
    <article
      className="block-row block-row--refused"
      data-status="refused"
      data-block-id={block.id}
      tabIndex={-1}
    >
      <h3 className="block-row__heading">
        <span className="block-row__marker block-row__marker--refused" aria-hidden="true">
          ⃠
        </span>
        Refused
      </h3>
      <p className="block-row__refusal-message">
        {refusal?.message ?? "This block could not be voiced."}
      </p>
      <p className="block-row__source-map">
        <span className="visually-hidden">Source: </span>
        {sourceMapLabel(block)}
        {refusal?.reason ? ` · ${refusal.reason}` : ""}
      </p>
      {block.source_map.raw_excerpt ? (
        <pre className="block-row__excerpt">{block.source_map.raw_excerpt}</pre>
      ) : null}
    </article>
  );
}
