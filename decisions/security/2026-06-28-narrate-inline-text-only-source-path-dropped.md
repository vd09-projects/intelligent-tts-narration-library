# POST /narrate accepts inline text only — server-side source path dropped as a CSRF/file-read vector

| Field    | Value            |
|----------|------------------|
| Date     | 2026-06-28       |
| Status   | accepted         |
| Category | security         |
| Tags     | narrate-server, http-bridge, csrf, dns-rebinding, ssrf, loopback, attack-surface, source-deferred, issue-109 |

## Context

Issue #109 (`feat/109-narrate-server-http-bridge`) adds an HTTP bridge so the Earshot UI can drive narration. The original acceptance criteria gave `POST /narrate` two input modes: inline `text`, or `source` — a server-side file path the server reads and renders.

The server is loopback-bound (127.0.0.1) but, being HTTP, it is reachable from any browser running on the same machine, including pages a user merely visits. A `source` path argument means a remote page can ask the local server to open and render an arbitrary local file. Pinned/locked-down CORS blocks the attacker from *reading* the HTTP response, but it does not block the *side effect* of the request being processed — the file is still read off disk and rendered to audio (which on a local-only build is spoken aloud). This is a classic CSRF / DNS-rebinding shape: the cross-origin request fires, the side effect lands, and the response-read block is irrelevant because the damage is the side effect, not the exfiltration.

The `cmd/narrate --in` CLI flag has the same file-read capability, but its trust boundary is the shell — only a local user with shell access invokes it. The HTTP `source` argument widens that to "any web page the user visits," a materially larger surface.

The Earshot UI already obtains message text from `GET /sessions/{id}/messages`, so it can POST inline `text` directly. It never needs the server to resolve a path on its behalf.

## Options considered

### Option A: Inline `text` only; drop `source` for phase one (CHOSEN)
- **Pros**: Removes the arbitrary-local-file read + render side effect entirely. No path resolution, no allowlist to get wrong. UI already has the text, so no functionality lost.
- **Cons**: Diverges from the original AC. Any future server-side-path use case must re-introduce the input deliberately and securely.

### Option B: Keep `source`, constrain it via `resolveWithin` against an allowlisted base dir
- **Pros**: Preserves the original AC shape; path-traversal contained by the allowlist.
- **Cons**: Still a CSRF-reachable file-read+render side effect for anything inside the allowlist. The allowlist becomes a security-critical config that is easy to widen by accident. More code, more to audit, for a capability the UI does not need.

## Decision

`POST /narrate` accepts inline `text` ONLY for phase one. The `source` (server-side file path) input from the original AC is dropped/deferred. A request carrying `source` (or both `text` and `source`) is rejected with HTTP 400; a regression test asserts `{text, source}` -> 400.

Reasoning: the loopback-but-browser-reachable server turns a file-path argument into a CSRF / DNS-rebinding vector — an arbitrary-local-file read + render side effect that pinned CORS does not prevent (it blocks reading the response, not the side effect). This is a strictly wider surface than the `cmd/narrate --in` CLI flag, whose trust boundary is the shell. Since the Earshot UI already has the message text from `GET /sessions`, inline text is sufficient and the path input buys nothing but risk. Option B (allowlisted `resolveWithin`) was rejected/deferred because it still leaves a CSRF-reachable file-read side effect and adds a security-critical config surface for no functional gain.

## Consequences

- Any future need to render a server-side file must re-introduce the input behind an explicit, separately-reasoned auth/origin control — not as a quiet AC restoration.
- The 400-on-`source` behavior is locked by a regression test, so the deferral cannot silently regress.
- Mirrors the project's existing loopback-refuse-to-start posture: the bridge errs toward refusing capability rather than filtering it after the fact.

## Related decisions

- [Loopback enforcement — refuse to start on any non-loopback bind](../security/2026-06-21-loopback-enforcement-refuse-to-start-on-non-loopback-bind.md) — same threat model (browser-reachable loopback server); both choose refusal over post-hoc filtering.

## Revisit trigger

If a genuine server-side-file narration use case appears (e.g. a non-browser trusted client), revisit with an explicit origin/auth control rather than restoring the raw `source` argument.
