# Earshot file input is multipart upload (FormData POST /narrate/file), not the design-doc `{path}` JSON variant

| Field    | Value            |
|----------|------------------|
| Date     | 2026-06-28       |
| Status   | accepted         |
| Category | architecture     |
| Tags     | issue-111, earshot, narrate-server, http-bridge, file-input, multipart, formdata, decide-not-discover, csrf, file-read-vector, issue-109 |

## Context

Earshot (#111) needs a file pane: drop or pick a file, hear it read out. The `narrate-server` design doc described a `{path}` JSON variant for file input. The #111 client shell was built against committed mock fixtures (no running #109 server to probe), so the input mode had to be **decided, not discovered** — the multi-perspective plan review flagged the path-vs-upload question as the single blocking item (Risk 5's "implement whichever the server supports" is unreachable under the mock-only build), because File-upload (FormData) vs `{path}` (JSON) is non-abstractable client code: you pick one shape and write it.

## Options considered

### Option A: `{path}` JSON variant (design-doc shape)
- **Pros**: matches the original design-doc wording; symmetric with a server-local file read.
- **Cons**: a browser drop/pick yields a `File` object the browser can read in-process, but `{path}` only addresses **server-local** paths the browser cannot read — wrong trust model for a browser client; `{path}` is also the dropped **server-side file-read vector** that the related #109 security decision removed (a CSRF / DNS-rebinding-reachable arbitrary-local-file read + render side effect that pinned CORS does not prevent).

### Option B: multipart upload (FormData → `POST /narrate/file`)
- **Pros**: a browser drop/pick already produces a `File`; FormData multipart is the native browser idiom; the bytes travel in the request body, so no server-local path is addressed and the #109 file-read vector is not reintroduced client-side; keeps the client honest about what it can actually read.
- **Cons**: diverges from the design-doc `{path}` wording (resolved by dropping `{path}` everywhere in the client + mock).

## Decision

Earshot's file-input mode is **multipart upload**: the picked/dropped `File` is packed into `FormData` (a `file` field plus `level`/`gender` form fields) and sent as multipart `POST /narrate/file`, returning the same shape as `POST /narrate`. The `{path}` JSON variant exists nowhere in the client or the mock. This was settled at the human gate (D1) as a decide-not-discover call because Phases 4–5 build against mocks with no server to probe. The build sends and a test asserts the request body is an actual `FormData` (multipart), and the path-vs-upload risk was retired.

## Consequences

- The client can only narrate files the browser hands it as bytes; it can never ask the server to read a server-local path (by construction, not by filtering).
- This is a distinct **client-side** decision from the #109 server-side "source path dropped" security decision, but it is the same trust-boundary reasoning carried to the browser edge.
- If a future server feature legitimately needs server-local file addressing, it must re-open the security analysis, not just the client shape.

## Related decisions

- [POST /narrate accepts inline text only — server-side source path dropped](../security/2026-06-28-narrate-inline-text-only-source-path-dropped.md) — RELATED: the server-side counterpart; this is the distinct client-side input-mode call that keeps the same file-read vector out of the browser path.
- [Earshot narrate-server contract pinned in types.ts; audio_url opaque](2026-06-28-earshot-narrate-contract-pinned-audio-url-opaque.md) — sibling #111 contract decision.

## Revisit trigger

Revisit if narrate-server ever needs to address server-local files for a legitimate use case (would require re-running the #109 file-read security analysis), or if the live #109 `/narrate/file` contract at `/verify` differs from the assumed multipart shape.
