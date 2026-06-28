// TranscriptPane.tsx — the center <main> (plan Step 3). Reads the shared owner;
// renders ordered blocks with the Spoken/Source toggle (Spoken default). The
// view state is local to the pane (a pure display choice), not shared narration
// state. The first block is focusable so the on-load / file-loaded focus move
// lands — including the refusal-first case (RefusalBlock is tabIndex -1).

import { useEffect, useRef, useState } from "react";
import { useNarration } from "../state/NarrationContext";
import { BlockRow } from "./BlockRow";
import { SegmentedToggle, type TranscriptView } from "./SegmentedToggle";

export function TranscriptPane() {
  const { currentTranscript, activeBlockId, playFromBlock } = useNarration();
  const [view, setView] = useState<TranscriptView>("spoken");
  const firstBlockRef = useRef<HTMLDivElement>(null);

  const blocks = currentTranscript?.blocks ?? [];

  // Move focus to the first block when a new transcript arrives (Focus Mgmt).
  const transcriptKey = currentTranscript?.timeline.plan_id ?? "";
  useEffect(() => {
    if (transcriptKey) {
      const first = firstBlockRef.current?.querySelector<HTMLElement>("[data-block-id]");
      first?.focus();
    }
  }, [transcriptKey]);

  return (
    <main className="transcript-pane" aria-label="Transcript">
      <div className="transcript-pane__bar">
        <h2 className="transcript-pane__title">Transcript</h2>
        {blocks.length > 0 ? (
          <SegmentedToggle value={view} onChange={setView} />
        ) : null}
      </div>

      {blocks.length === 0 ? (
        <p className="transcript-pane__empty" data-testid="transcript-empty">
          Select a message or drop a file to hear it read out.
        </p>
      ) : (
        <div className="transcript-pane__blocks" ref={firstBlockRef}>
          {blocks.map((block) => (
            <BlockRow
              key={block.id}
              block={block}
              view={view}
              isActive={block.id === activeBlockId}
              onPlay={playFromBlock}
            />
          ))}
        </div>
      )}
    </main>
  );
}
