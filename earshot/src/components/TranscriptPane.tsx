// TranscriptPane.tsx — center <main>. Shows loading state when the selected
// entry is in flight, then renders ordered blocks with Spoken/Source toggle.

import { useEffect, useRef, useState } from "react";
import { useNarration } from "../state/NarrationContext";
import { BlockRow } from "./BlockRow";
import { SegmentedToggle, type TranscriptView } from "./SegmentedToggle";

export function TranscriptPane() {
  const {
    currentTranscript,
    activeBlockId,
    playFromBlock,
    selectedEntry,
    selectedEntryId,
    blockLeveling,
    setBlockLevel,
  } = useNarration();
  const [view, setView] = useState<TranscriptView>("spoken");
  const firstBlockRef = useRef<HTMLDivElement>(null);

  const blocks = currentTranscript?.blocks ?? [];
  const isLoading = selectedEntry?.status === "loading";
  // Leveling needs render_id to drive /narrate/block; without it the control
  // renders disabled (older server / pre-#113).
  const canLevel = Boolean(currentTranscript?.render_id) && selectedEntryId != null;

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

      {isLoading && (
        <div className="transcript-pane__loading" role="status" aria-live="polite" data-testid="transcript-loading">
          <span className="transcript-pane__spinner" aria-hidden="true" />
          Narrating…
        </div>
      )}

      {blocks.length === 0 && !isLoading ? (
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
              leveling={blockLeveling.get(`${selectedEntryId}::${block.id}`)}
              onCommitLevel={
                canLevel
                  ? (level) => setBlockLevel(selectedEntryId as string, block.id, level)
                  : undefined
              }
            />
          ))}
        </div>
      )}
    </main>
  );
}
