// BlockRow.tsx — one transcript block (plan Step 3).
//
//   - refused  → delegates to RefusalBlock (no level control, no play). This is
//     the ORIGINALLY-refused case only: a block the document first rendered as
//     refused. An escalation that later RETURNS a refusal does NOT reach here —
//     the hook keeps the block at its prior voiced/degraded status and surfaces
//     the refusal inline via LevelControl's role=alert (#113, F4).
//   - degraded → renders the voiced text PLUS a NON-COLOR marker (icon + text
//     label "Degraded") so it is never silently shown as fully voiced (D5).
//   - voiced   → renders clean.
//
// voiced/degraded keep a real <button> play affordance (Space/Enter plays —
// seeks the shared <audio> to this block's start) and mount the LevelControl
// (#113). aria-current="true" marks the DERIVED active block; it is paired with
// a non-color visual marker.

import type { Block, Level } from "../api/types";
import { IDLE_LEVELING, type BlockLevelingState } from "../hooks/useNarrationSession";
import { sourceText, spokenText } from "../state/blockText";
import { LevelControl } from "./LevelControl";
import { RefusalBlock } from "./RefusalBlock";
import type { TranscriptView } from "./SegmentedToggle";

export function BlockRow({
  block,
  view,
  isActive,
  onPlay,
  leveling = IDLE_LEVELING,
  onCommitLevel,
}: {
  block: Block;
  view: TranscriptView;
  isActive: boolean;
  onPlay: (blockId: string) => void;
  /** Per-block leveling status (#113). Defaults to idle. */
  leveling?: BlockLevelingState;
  /** Commit a new level for this block. Absent → control disabled (no render_id). */
  onCommitLevel?: (level: Level) => void;
}) {
  // Hide the control ONLY on an originally-refused block (RefusalBlock path).
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
        aria-label={`Play from block ${block.order + 1}`}
      >
        <span className="block-row__play-affordance" aria-hidden="true">
          ⏯ from here
        </span>
        <span className="block-row__text">{text || <em>(no text)</em>}</span>
      </button>
      {isDegraded && view === "spoken" ? (
        <p className="block-row__degraded-note">Read verbatim — not summarised.</p>
      ) : null}
      <LevelControl
        current={block.level}
        leveling={leveling}
        blockLabel={`block ${block.order + 1}`}
        onCommit={onCommitLevel}
        disabled={!onCommitLevel}
      />
    </article>
  );
}
