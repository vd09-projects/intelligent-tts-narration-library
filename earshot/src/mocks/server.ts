// server.ts — the test-only mock layer (plan Fixtures & Mock Strategy). A small
// fetch shim that serves the SAME committed fixtures the Phase 3 client tests
// assert against — one source, two readers, so the mock can never desync from
// the tests. This is NOT used at dev runtime: `make earshot-dev` proxies to the
// real narrate-server. It exists only under Vitest.
//
// No external mock dependency (MSW): a fetch shim keeps the dependency surface
// small (CLAUDE.md "no heavy infra") and lets a test assert the exact request —
// crucially the multipart FormData on POST /narrate/file (D1).

import messages from "../fixtures/messages.json";
import messagesEmpty from "../fixtures/messages.empty.json";
import narrateMixed from "../fixtures/narrate.mixed.json";
import narrateRefusalFirst from "../fixtures/narrate.refusalFirst.json";
import errorBody from "../fixtures/error.json";
import type { NarrateBlockResponse } from "../api/types";

export interface MockOptions {
  /** Which messages fixture GET /sessions returns. */
  messages?: "default" | "empty" | "error";
  /** Which narrate fixture POST /narrate and /narrate/file return. */
  narrate?: "mixed" | "refusalFirst" | "error";
  /**
   * POST /narrate/block (#113). A full custom response (test controls the
   * post-patch timeline), or a canned outcome. Defaults to a no-op echo error
   * if a test posts /narrate/block without configuring it.
   */
  narrateBlock?: NarrateBlockResponse | "error";
  /** Force a network-level reject (TypeError) instead of a Response. */
  reject?: boolean;
  /** Called with each request so a test can assert method/path/body. */
  onRequest?: (req: { url: string; method: string; init?: RequestInit }) => void;
}

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

/** Build a fetch implementation honoring the given options. */
export function createMockFetch(options: MockOptions = {}) {
  return async (input: RequestInfo | URL, init?: RequestInit): Promise<Response> => {
    const url = typeof input === "string" ? input : input.toString();
    const method = (init?.method ?? "GET").toUpperCase();
    options.onRequest?.({ url, method, init });

    if (options.reject) {
      throw new TypeError("Failed to fetch");
    }

    if (method === "GET" && /\/sessions\/[^/]+\/messages$/.test(url)) {
      if (options.messages === "error") {
        return json(errorBody, 404);
      }
      if (options.messages === "empty") {
        return json(messagesEmpty);
      }
      return json(messages);
    }

    if (method === "POST" && url.endsWith("/narrate/block")) {
      if (options.narrateBlock === "error" || options.narrateBlock == null) {
        return json({ reason: "render_failed", message: "block re-render failed: boom" }, 500);
      }
      return json(options.narrateBlock);
    }

    if (method === "POST" && (url.endsWith("/narrate") || url.endsWith("/narrate/file"))) {
      if (options.narrate === "error") {
        return json({ reason: "render_failed", message: "render failed: boom" }, 500);
      }
      if (options.narrate === "refusalFirst") {
        return json(narrateRefusalFirst);
      }
      return json(narrateMixed);
    }

    return json({ reason: "not_found", message: "no mock route: " + method + " " + url }, 404);
  };
}
