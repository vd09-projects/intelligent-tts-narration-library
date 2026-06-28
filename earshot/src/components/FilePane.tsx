// FilePane.tsx — drop/pick a file → multipart POST /narrate/file via the shared
// owner. Key = file.name; uploading the same name again re-narrates it.

import { useCallback, useRef, useState } from "react";
import { useNarration } from "../state/NarrationContext";
import { useAnnouncer } from "../state/Announcer";
import { ErrorBanner } from "./ErrorBanner";

export function FilePane() {
  const { narrateFile, entries, selectedEntryId } = useNarration();
  const { announce } = useAnnouncer();
  const inputRef = useRef<HTMLInputElement>(null);
  const [dragging, setDragging] = useState(false);
  const [fileName, setFileName] = useState<string | null>(null);

  const send = useCallback(
    (file: File) => {
      setFileName(file.name);
      announce(`Preparing audio for ${file.name}`);
      void narrateFile(file.name, file, 1);
    },
    [narrateFile, announce],
  );

  const onPick = useCallback(
    (e: React.ChangeEvent<HTMLInputElement>) => {
      const file = e.target.files?.[0];
      if (file) send(file);
    },
    [send],
  );

  const onDrop = useCallback(
    (e: React.DragEvent) => {
      e.preventDefault();
      setDragging(false);
      const file = e.dataTransfer.files?.[0];
      if (file) send(file);
    },
    [send],
  );

  // Show error for the currently selected file entry (if any).
  const fileEntry = fileName ? entries.get(fileName) : null;
  const fileError =
    fileEntry?.status === "error" && selectedEntryId === fileName ? fileEntry.error : null;

  return (
    <section className="file-pane" aria-label="Read out a file">
      <h2 className="file-pane__title">Read out a file</h2>

      <div
        className={"file-pane__dropzone" + (dragging ? " is-dragging" : "")}
        onDragOver={(e) => {
          e.preventDefault();
          setDragging(true);
        }}
        onDragLeave={() => setDragging(false)}
        onDrop={onDrop}
        data-testid="file-dropzone"
      >
        <p className="file-pane__drop-hint">Drop a text or markdown file here</p>
        <span className="file-pane__or">or</span>
        <button
          type="button"
          className="file-pane__pick"
          onClick={() => inputRef.current?.click()}
        >
          Choose a file
        </button>
        <input
          ref={inputRef}
          type="file"
          className="visually-hidden"
          accept=".txt,.md,text/plain,text/markdown"
          onChange={onPick}
          aria-label="Upload a text or markdown file to read out"
          data-testid="file-input"
        />
      </div>

      {fileName ? (
        <p className="file-pane__selected" data-testid="file-selected">
          Selected: {fileName}
          {fileEntry?.status === "loading" && (
            <span className="file-pane__loading-badge" aria-label="Narrating">
              {" "}<span className="transcript-pane__spinner" aria-hidden="true" style={{display:"inline-block",verticalAlign:"middle"}} />
            </span>
          )}
          {fileEntry?.status === "ready" && (
            <span className="file-pane__ready-badge" aria-label="Ready"> ✓</span>
          )}
        </p>
      ) : null}

      {fileError ? <ErrorBanner message={fileError} /> : null}
    </section>
  );
}
