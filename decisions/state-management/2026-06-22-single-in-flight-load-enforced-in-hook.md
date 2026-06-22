# Single-in-flight load enforced in the loader hook so concurrent triggers cannot race the shared revokeRef

| Field    | Value            |
|----------|------------------|
| Date     | 2026-06-22       |
| Status   | accepted         |
| Category | state-management |
| Tags     | player, useDirectoryLoader, isLoading, abort-prior-load, revokeRef, blob-url, concurrency, double-click, in-flight-guard, issue-70 |

## Context

The #70 player gains a load-by-path entry point (`loadFromServerDir`) alongside the two existing folder pickers. All load paths share a single `revokeRef` for blob-URL lifetime management. Round-1 review (item 4) flagged that concurrent or double-click triggers could race the shared `revokeRef` — two loads in flight could revoke or overwrite each other's blob URL, leaking one or pointing `revokeRef` at an uncommitted URL.

## Decision

Enforce at most one load in flight at a time, in the hook (`useDirectoryLoader.ts`), not at the call sites:

- Reuse the picker path's existing in-flight handling if present; otherwise add an `isLoading` flag (or abort-prior-load) at build time, after reading the hook.
- All load paths (both pickers + load-by-path) go through the same `runLoad`-style revoke discipline: revoke `revokeRef.current` before committing, then set `revokeRef.current = data.audioUrl`.
- A rejected load never updates `revokeRef.current` to an uncommitted URL; `isLoading` clears in a `finally`.
- `isLoading` is surfaced so the UI disables the Load button and both pickers while a load runs.

Enforcing the guard in the hook (at the source of the shared `revokeRef`) rather than per-trigger means the single-in-flight guarantee holds regardless of which control fires, and concurrent triggers are *blocked*, not *raced*. UI disabling is a second layer, not the primary mechanism.

## Consequences

- Double-clicking Load, or firing a picker mid-load, cannot start a second concurrent load.
- The shared `revokeRef` is never observed by two in-flight loads at once.
- Hook test: two concurrent triggers -> only one load commits, `revokeRef` not raced, `isLoading` toggles.

## Related decisions

- [source.md failure modes pinned](../resilience/2026-06-22-source-md-failure-modes-pinned-null-or-warning-never-abort.md) — sibling #70 player-lib resilience decision; both govern `loadFromServerDir` behavior.

## Revisit trigger

If the player ever needs to support legitimately concurrent loads (e.g. multiple independent panes), the single shared `revokeRef` + single-in-flight model would need to become per-pane.
