# Manual absolute-path dir field in TopBar is the server-mode escalate enabler

| Field    | Value            |
|----------|------------------|
| Date     | 2026-06-22       |
| Status   | accepted         |
| Category | architecture     |
| Tags     | player, escalate, server-mode, topbar, absolute-path, browser-fs, go-server, issue-50 |

## Context

Task #50, React player server mode. The Go escalate server needs a real absolute filesystem path for the persistent-sink directory: it calls `filepath.Abs` then `readManifest` against that dir. The question was how the browser player supplies that absolute path.

Browsers deliberately do not expose real absolute filesystem paths through any file/directory loader (File API, directory picker, drag-drop all hide the true path for security). So the existing player loaders (bundled fixture, runtime directory picker) cannot hand the Go server the absolute path it requires.

## Options considered

### Option A: Derive the dir from the existing loaders
- **Pros**: No new UI; reuse the directory the user already picked.
- **Cons**: Impossible — no browser file loader exposes a real absolute FS path. The picker gives sandboxed handles / fake paths, not something `filepath.Abs` can resolve on the server's filesystem.

### Option B: Manual absolute-path dir text field in TopBar (server mode)
- **Pros**: Gives the Go server exactly the absolute path it needs. It is the literal go/no-go enabler for server-mode escalation — without it the feature cannot function.
- **Cons**: Manual entry; user must type/paste an absolute path. Less polished than picking a folder.

## Decision

Chose Option B. Server mode exposes a manual absolute-path directory text field in the TopBar; its value is what the Go server resolves via `filepath.Abs` → `readManifest`. This is the go/no-go enabler for server-mode escalate. Rejected deriving the dir from the existing loaders because no browser file loader exposes a real absolute FS path.

## Consequences

- Server-mode escalation is functional: the server gets a resolvable absolute path.
- Manual UX cost: the user types/pastes an absolute path rather than picking a folder.
- The field is server-mode-specific; fixture/picker loaders remain for non-server browsing.

## Related decisions

- [Player dual data loading — bundled fixture AND runtime directory picker](../convention/2026-06-21-player-dual-data-loading-fixture-and-picker.md) — the browser loaders that cannot supply an absolute path, motivating this field.
