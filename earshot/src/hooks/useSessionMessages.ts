// useSessionMessages.ts — GET /sessions/{id}/messages (plan Step 4). Owns the
// message-list request lifecycle for the session pane. Distinct idle / loading /
// loaded / empty / error states so the pane can render each one differently
// (loading + empty must be visibly distinct from each other and from idle).

import { useCallback, useState } from "react";
import { ClientError, getMessages } from "../api/client";
import type { SessionMessage } from "../api/types";

export type MessagesStatus = "idle" | "loading" | "loaded" | "error";

export interface UseSessionMessages {
  status: MessagesStatus;
  messages: SessionMessage[];
  /** True when the fetch succeeded but the session had zero messages. */
  isEmpty: boolean;
  error: string | null;
  load: (sessionId: string) => Promise<void>;
}

export function useSessionMessages(): UseSessionMessages {
  const [status, setStatus] = useState<MessagesStatus>("idle");
  const [messages, setMessages] = useState<SessionMessage[]>([]);
  const [error, setError] = useState<string | null>(null);

  const load = useCallback(async (sessionId: string) => {
    setStatus("loading");
    setError(null);
    setMessages([]);
    try {
      const resp = await getMessages(sessionId);
      setMessages(resp.messages);
      setStatus("loaded");
    } catch (err) {
      const message =
        err instanceof ClientError
          ? err.message
          : err instanceof Error
            ? err.message
            : "Couldn't load messages";
      setError(message);
      setStatus("error");
    }
  }, []);

  return {
    status,
    messages,
    isEmpty: status === "loaded" && messages.length === 0,
    error,
    load,
  };
}
