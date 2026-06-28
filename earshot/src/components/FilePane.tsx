// FilePane.tsx — drop/pick a file → read it out (plan Step 5). On drop or pick,
// the File is sent as MULTIPART POST /narrate/file (D1) via the SHARED owner
// (narrateFile), so the result flows into the same TranscriptPane + audio path
// as the session flow. Keyboard path is a real <input type=file> + <button>
// (no <div onClick> picker). The drop zone is a mouse affordance ON TOP of that.

import { useCallback, useRef, useState } from "react";
import { useNarration } from "../state/NarrationContext";
import { useAnnouncer } from "../state/Announcer";
import { ErrorBanner } from "./ErrorBanner";

export function FilePane() {
  const { narrateFile, request } = useNarration();
  const { announce } = useAnnouncer();
  const inputRef = useRef<HTMLInputElement>(null);
  const [dragging, setDragging] = useState(false);
  const [fileName, setFileName] = useState<string | null>(null);

  const send = useCallback(
    (file: File) => {
      setFileName(file.name);
      announce(`Preparing audio for ${file.name}`);
      void narrateFile(file, 1);
    },
    [narrateFile, announce],
  );

  const onPick = useCallback(
    (e: React.ChangeEvent<HTMLInputElement>) => {
      const file = e.target.files?.[0];
      if (file) {
        send(file);
      }
    },
    [send],
  );

  const onDrop = useCallback(
    (e: React.DragEvent) => {
      e.preventDefault();
      setDragging(false);
      const file = e.dataTransfer.files?.[0];
      if (file) {
        send(file);
      }
    },
    [send],
  );

  const fileError = request.source === "file" && request.status === "error" ? request.error : null;

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
        </p>
      ) : null}

      {fileError ? <ErrorBanner message={fileError} /> : null}
    </section>
  );
}
