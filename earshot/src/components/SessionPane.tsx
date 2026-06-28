// SessionPane.tsx — session-ID input → message listbox → click/Enter to hear
// (plan Step 4). APG listbox: one tab stop on the container, ArrowUp/Down move
// the active option (aria-activedescendant), Enter activates. Activating a
// message assembles its block text and dispatches POST /narrate into the SHARED
// owner (useNarration) — the pane holds no transcript/audio state of its own.
//
// Distinct states: idle (no id), loading, empty (zero messages), error.

import { useCallback, useEffect, useRef, useState } from "react";
import { useNarration } from "../state/NarrationContext";
import { useAnnouncer } from "../state/Announcer";
import { useSessionMessages } from "../hooks/useSessionMessages";
import type { SessionMessage } from "../api/types";
import { ErrorBanner } from "./ErrorBanner";
import { SessionRow } from "./SessionRow";

const LISTBOX_ID = "session-listbox";
const optionId = (i: number) => `session-option-${i}`;

function messageToText(message: SessionMessage): string {
  return message.blocks.map((b) => b.text).join("\n\n");
}

export function SessionPane() {
  const { narrateText } = useNarration();
  const { announce } = useAnnouncer();
  const { status, messages, isEmpty, error, load } = useSessionMessages();

  const [sessionId, setSessionId] = useState("");
  const [activeIndex, setActiveIndex] = useState(0);
  const [selectedIndex, setSelectedIndex] = useState<number | null>(null);
  const listRef = useRef<HTMLDivElement>(null);

  // Announce + manage focus as the message list resolves.
  useEffect(() => {
    if (status === "loading") {
      announce("Loading messages");
    } else if (status === "loaded") {
      if (messages.length === 0) {
        announce("No messages in this session");
      } else {
        announce(`${messages.length} messages loaded`);
        setActiveIndex(0);
        listRef.current?.focus();
      }
    }
  }, [status, messages.length, announce]);

  const onSubmit = useCallback(
    (e: React.FormEvent) => {
      e.preventDefault();
      const id = sessionId.trim();
      if (id) {
        setSelectedIndex(null);
        void load(id);
      }
    },
    [sessionId, load],
  );

  const activate = useCallback(
    (index: number) => {
      const message = messages[index];
      if (!message) {
        return;
      }
      setSelectedIndex(index);
      setActiveIndex(index);
      void narrateText(messageToText(message), 1);
    },
    [messages, narrateText],
  );

  const onKeyDown = useCallback(
    (e: React.KeyboardEvent) => {
      if (messages.length === 0) {
        return;
      }
      if (e.key === "ArrowDown") {
        e.preventDefault();
        setActiveIndex((i) => Math.min(i + 1, messages.length - 1));
      } else if (e.key === "ArrowUp") {
        e.preventDefault();
        setActiveIndex((i) => Math.max(i - 1, 0));
      } else if (e.key === "Home") {
        e.preventDefault();
        setActiveIndex(0);
      } else if (e.key === "End") {
        e.preventDefault();
        setActiveIndex(messages.length - 1);
      } else if (e.key === "Enter" || e.key === " ") {
        e.preventDefault();
        activate(activeIndex);
      }
    },
    [messages.length, activeIndex, activate],
  );

  return (
    <div className="session-pane">
      <h2 className="session-pane__title">Sessions</h2>

      <form className="session-pane__form" onSubmit={onSubmit}>
        <label htmlFor="session-id-input" className="session-pane__label">
          Session ID
        </label>
        <div className="session-pane__input-row">
          <input
            id="session-id-input"
            className="session-pane__input"
            type="text"
            value={sessionId}
            onChange={(e) => setSessionId(e.target.value)}
            placeholder="paste a session id"
            autoComplete="off"
            spellCheck={false}
          />
          <button type="submit" className="session-pane__load">
            Load
          </button>
        </div>
      </form>

      {status === "error" ? <ErrorBanner message={error} /> : null}

      {status === "idle" ? (
        <p className="session-pane__hint" data-testid="session-idle">
          Enter a session ID to list its messages.
        </p>
      ) : null}

      {status === "loading" ? (
        <p className="session-pane__loading" data-testid="session-loading" aria-busy="true">
          Loading messages…
        </p>
      ) : null}

      {isEmpty ? (
        <p className="session-pane__empty" data-testid="session-empty">
          No messages in this session.
        </p>
      ) : null}

      {status === "loaded" && messages.length > 0 ? (
        <div
          ref={listRef}
          role="listbox"
          id={LISTBOX_ID}
          aria-label="Session messages"
          tabIndex={0}
          aria-activedescendant={optionId(activeIndex)}
          className="session-pane__listbox"
          onKeyDown={onKeyDown}
        >
          {messages.map((message, i) => (
            <SessionRow
              key={message.id}
              message={message}
              optionId={optionId(i)}
              selected={selectedIndex === i}
              active={activeIndex === i}
              onActivate={() => activate(i)}
            />
          ))}
        </div>
      ) : null}
    </div>
  );
}
