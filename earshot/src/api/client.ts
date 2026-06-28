// client.ts — typed fetch wrappers for the narrate-server (#109) routes.
//
// One job: hit the right method+path, parse the documented shape, and normalize
// any failure into a single ClientError. `audio_url` is passed through OPAQUE
// (D4) — never parsed, never reconstructed. All shape knowledge is imported from
// types.ts; this file adds no new shape.

import type {
  Gender,
  Level,
  MessagesResponse,
  NarrateResponse,
  ServerError,
} from "./types";

/**
 * The single normalized client failure. Every transport fault (network reject,
 * non-2xx status, unparseable body) becomes a ClientError so callers branch on
 * one type. Honesty boundary: this is the ERROR path (stops the flow). A refusal
 * is NOT an error — it arrives as a 200 NarrateResponse with refused blocks.
 */
export class ClientError extends Error {
  /** Server reason token when available (ServerError.reason), else a synthetic. */
  readonly reason: string;
  /** HTTP status when there was a response, else 0 for a network-level reject. */
  readonly status: number;

  constructor(message: string, reason: string, status: number) {
    super(message);
    this.name = "ClientError";
    this.reason = reason;
    this.status = status;
  }
}

/** Parse a non-2xx response body into a ClientError (ServerError when shaped). */
async function toClientError(res: Response): Promise<ClientError> {
  let reason = "request_failed";
  let message = `request failed with status ${res.status}`;
  try {
    const body = (await res.json()) as Partial<ServerError>;
    if (body && typeof body.message === "string") {
      message = body.message;
    }
    if (body && typeof body.reason === "string") {
      reason = body.reason;
    }
  } catch {
    // Non-JSON error body — keep the synthetic status message.
  }
  return new ClientError(message, reason, res.status);
}

/** Run fetch, normalizing a network-level reject into a ClientError. */
async function safeFetch(input: string, init?: RequestInit): Promise<Response> {
  try {
    return await fetch(input, init);
  } catch (err) {
    const message = err instanceof Error ? err.message : "network request failed";
    throw new ClientError(message, "network_error", 0);
  }
}

/** GET /sessions/{id}/messages → ordered message list. */
export async function getMessages(sessionId: string): Promise<MessagesResponse> {
  const res = await safeFetch(`/sessions/${encodeURIComponent(sessionId)}/messages`);
  if (!res.ok) {
    throw await toClientError(res);
  }
  return (await res.json()) as MessagesResponse;
}

/** POST /narrate {text, level, gender} → {audio_url (opaque), blocks, timeline}. */
export async function postNarrate(args: {
  text: string;
  level: Level;
  gender?: Gender;
}): Promise<NarrateResponse> {
  const res = await safeFetch("/narrate", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({
      text: args.text,
      level: args.level,
      ...(args.gender ? { gender: args.gender } : {}),
    }),
  });
  if (!res.ok) {
    throw await toClientError(res);
  }
  return (await res.json()) as NarrateResponse;
}

/**
 * POST /narrate/file — MULTIPART upload (D1). The picked/dropped File goes under
 * a `file` field with `level`/`gender` form fields. No `{path}` JSON variant.
 * Same response shape as /narrate.
 */
export async function postNarrateFile(args: {
  file: File;
  level: Level;
  gender?: Gender;
}): Promise<NarrateResponse> {
  const form = new FormData();
  form.append("file", args.file);
  form.append("level", String(args.level));
  if (args.gender) {
    form.append("gender", args.gender);
  }
  // NB: do not set Content-Type — the browser sets the multipart boundary.
  const res = await safeFetch("/narrate/file", { method: "POST", body: form });
  if (!res.ok) {
    throw await toClientError(res);
  }
  return (await res.json()) as NarrateResponse;
}
