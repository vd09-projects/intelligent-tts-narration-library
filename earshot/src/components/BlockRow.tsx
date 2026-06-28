// BlockRow.tsx — one transcript block (plan Step 3).
//
//   - refused  → delegates to RefusalBlock (no level control, no play).
//   - degraded → renders the voiced text PLUS a NON-COLOR marker (icon + text
//     label "Degraded") so it is never silently shown as fully voiced (D5).
//   - voiced   → renders clean.
//
// voiced/degraded keep a real <button> play affordance (Space/Enter plays —
// seeks the shared <audio> to this block's start). aria-current="true" marks the
// DERIVED active block; it is paired with a non-color visual marker.

import type { Block } from "../api/types";
import { sourceText, spokenText } from "../state/blockText";
import { RefusalBlock } from "./RefusalBlock";
import type { TranscriptView } from "./SegmentedToggle";

export function BlockRow({
  block,
  view,
  isActive,
  onPlay,
}: {
  block: Block;
  view: TranscriptView;
  isActive: boolean;
  onPlay: (blockId: string) => void;
}) {
  if (block.status === "refused") {
    return <RefusalBlock block={block} />;
  }

  const text = view === "spoken" ? spokenText(block) : sourceText(block);
  const isDegraded = block.status === "degraded";

  return (
    <article
      className={"block-row block-row--" + block.status + (isActive ? " is-active" : "")}
      data-status={block.status}
      data-block-id={block.id}
      tabIndex={-1}
      aria-current={isActive ? "true" : undefined}
    >
      <h3 className="block-row__heading">
        <span className="visually-hidden">Block {block.order + 1}, </span>
        {block.class}
        {isActive ? (
          <span className="block-row__marker block-row__marker--active"> ▸ now playing</span>
        ) : null}
        {isDegraded ? (
          <span className="block-row__marker block-row__marker--degraded" data-testid="degraded-marker">
            <span aria-hidden="true">△ </span>Degraded
          </span>
        ) : null}
      </h3>
      <button
        type="button"
        className="block-row__play"
        onClick={() => onPlay(block.id)}
        aria-label={`Play block ${block.order + 1}`}
      >
        <span className="block-row__text">{text || <em>(no text)</em>}</span>
      </button>
      {isDegraded && view === "spoken" ? (
        <p className="block-row__degraded-note">Read verbatim — not summarised.</p>
      ) : null}
    </article>
  );
}
