# render_id wav lifecycle — TTL reaper plus orphan-scan, with deletes outside the store write-lock

| Field    | Value            |
|----------|------------------|
| Date     | 2026-06-28       |
| Status   | accepted         |
| Category | architecture     |
| Tags     | narrate-server, http-bridge, render-store, ttl-reaper, orphan-scan, crash-window, wavfilesink, single-wav, os-remove-outside-lock, snapshot-under-lock, issue-109 |

## Context

`POST /narrate` renders inline text to a WAV and mints a `render_id` that `GET /audio/{render_id}.wav` later serves. These files need a lifecycle: they must be cleaned up, and the cleanup must not stall the hot path or leak files on crash.

Two failure modes shaped the design. First, a render can finish writing its WAV to disk and then the process crashes *before* the in-memory store records the mint — leaving an orphan file no store entry points at. Second, naive cleanup (`os.Remove` while holding the store write-lock) would do disk I/O under the lock that `/narrate` mints also need, coupling eviction latency into the mint path.

The render itself uses the single-wav `WAVFileSink` variant (combined WAV, no `plan.json` / `manifest.json` sidecars) — distinct from the 3-file persistent sink — because the audio endpoint serves one streamable file, not a plan bundle. The single-wav-no-sidecars choice is recorded separately.

## Options considered

### Option A: TTL-only eviction, os.Remove under the store write-lock
- **Pros**: Simple; one mechanism.
- **Cons**: Leaks files written in the crash-between-write-and-mint window (no store entry, so TTL never sees them). `os.Remove` disk I/O under the write-lock couples eviction into the `/narrate` mint path.

### Option B: TTL reaper + orphan-scan; snapshot expired under lock, delete after release (CHOSEN)
- **Pros**: TTL reaps normal entries; the orphan-scan sweeps files with no store entry (covers the crash window). All `os.Remove` I/O happens OUTSIDE the write-lock — expired entries are snapshotted under the lock, then deleted after release — so eviction never blocks a mint.
- **Cons**: Two cleanup mechanisms to reason about; the orphan-scan must distinguish in-flight-but-not-yet-minted files from true orphans (handled by age/heuristic).

## Decision

The `render_id` WAV lifecycle uses a TTL reaper PLUS an orphan-scan for the crash-between-write-and-mint window. The render uses the single-wav `WAVFileSink` variant (combined WAV, no plan/manifest sidecars), distinct from the 3-file persistent sink. `os.Remove` I/O happens OUTSIDE the store write-lock: expired entries are snapshotted under the lock, the lock is released, then the files are deleted.

Reasoning: TTL alone leaks files written in the window where a render completes on disk but the process dies before the mint is recorded — the orphan-scan catches those. Doing the `os.Remove` under the write-lock would couple eviction disk-I/O latency into the `/narrate` mint path; snapshotting-then-deleting-after-release keeps the lock hold short and the mint path fast, and lets the reaper delete lock-free (on POSIX the serve path's open fd survives the unlink).

## Consequences

- No file leak across a crash between WAV write and store mint.
- Eviction never blocks a mint — the write-lock is held only to snapshot the expiry set, never across disk I/O.
- Lock-free `os.Remove` pairs with the audio-serve path, which is designed to tolerate the file being unlinked mid-stream (POSIX open-fd survival).

## Related decisions

- [WAVFileSink reuses persistent-sink wav-concat math but writes only the combined wav, no JSON sidecars](../architecture/2026-06-28-wavfilesink-reuses-wav-concat-no-sidecars.md) — the single-wav sink variant this lifecycle manages; this decision records the server-side reaper/orphan-scan layer on top of it.
- [GET /audio holds the store read-lock only across resolve+open, then streams lock-free](../concurrency/2026-06-28-audio-serve-releases-store-lock-before-streaming.md) — the serve path whose POSIX open-fd survival makes the lock-free reaper safe.

## Revisit trigger

If the store is ever backed by non-POSIX storage, or if the crash window is closed by writing the mint before/atomically-with the file, re-evaluate whether the orphan-scan is still needed.
