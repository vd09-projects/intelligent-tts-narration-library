import { afterEach, describe, expect, it, vi } from "vitest";
import { ClientError, getMessages, postNarrate, postNarrateFile } from "./client";
import { createMockFetch } from "../mocks/server";
import narrateMixed from "../fixtures/narrate.mixed.json";

afterEach(() => {
  vi.unstubAllGlobals();
});

describe("api client — shape mapping", () => {
  it("GET /sessions/{id}/messages hits the right path and maps the shape", async () => {
    const seen: string[] = [];
    vi.stubGlobal(
      "fetch",
      createMockFetch({ onRequest: (r) => seen.push(`${r.method} ${r.url}`) }),
    );

    const resp = await getMessages("demo-session-001");
    expect(seen[0]).toBe("GET /sessions/demo-session-001/messages");
    expect(resp.messages).toHaveLength(3);
    expect(resp.messages[0].role).toBe("user");
  });

  it("POST /narrate sends JSON and passes audio_url through OPAQUE (no render_id parse)", async () => {
    let body: unknown;
    vi.stubGlobal(
      "fetch",
      createMockFetch({
        onRequest: (r) => {
          if (r.method === "POST") {
            body = JSON.parse(String(r.init?.body));
          }
        },
      }),
    );

    const resp = await postNarrate({ text: "hello", level: 1, gender: "male" });
    expect(body).toEqual({ text: "hello", level: 1, gender: "male" });
    // Opaque: the client returns audio_url verbatim, identical to the fixture.
    expect(resp.audio_url).toBe(narrateMixed.audio_url);
  });

  it("POST /narrate/file sends MULTIPART FormData (D1), not JSON", async () => {
    let init: RequestInit | undefined;
    vi.stubGlobal(
      "fetch",
      createMockFetch({
        onRequest: (r) => {
          if (r.url.endsWith("/narrate/file")) {
            init = r.init;
          }
        },
      }),
    );

    const file = new File(["# Title\nbody"], "doc.md", { type: "text/markdown" });
    await postNarrateFile({ file, level: 2 });

    expect(init?.body).toBeInstanceOf(FormData);
    const form = init?.body as FormData;
    expect(form.get("file")).toBeInstanceOf(File);
    expect((form.get("file") as File).name).toBe("doc.md");
    expect(form.get("level")).toBe("2");
    // No Content-Type header — browser sets the multipart boundary.
    expect(init?.headers).toBeUndefined();
  });

  it("normalizes an HTTP error body into a ClientError with reason + status", async () => {
    vi.stubGlobal("fetch", createMockFetch({ messages: "error" }));
    await expect(getMessages("nope")).rejects.toMatchObject({
      name: "ClientError",
      reason: "source_not_found",
      status: 404,
    });
  });

  it("normalizes a network-level reject into a ClientError (status 0)", async () => {
    vi.stubGlobal("fetch", createMockFetch({ reject: true }));
    const err = await getMessages("x").catch((e) => e);
    expect(err).toBeInstanceOf(ClientError);
    expect(err.status).toBe(0);
    expect(err.reason).toBe("network_error");
  });
});
