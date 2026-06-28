# GET /audio/{render_id}.wav holds the store read-lock only across resolve+open, then streams lock-free

| Field    | Value            |
|----------|------------------|
| Date     | 2026-06-28       |
| Status   | accepted         |
| Category | concurrency      |
| Tags     | narrate-server, http-bridge, rwmutex, writer-starvation, liveness, servecontent, posix-unlink, render-store, reaper, wall-clock-test, issue-109 |

## Context

`GET /audio/{render_id}.wav` serves rendered WAV files out of an in-memory render store guarded by an `RWMutex`. The store is also mutated by `POST /narrate` (mints new render ids under the write-lock) and by a TTL reaper (evicts expired entries). The naive implementation would take the store read-lock for the whole handler — resolve the id, open the file, and `http.ServeContent` the file while still holding the lock.

The trap: Go's `sync.RWMutex` blocks *new* readers once a writer is waiting (to prevent writer starvation). So a single slow or large download holding the read-lock across `ServeContent`, plus one waiting reaper write-lock, causes every subsequent `/narrate` mint and `/audio` serve to block behind the in-flight download. That is a liveness bug — the race detector cannot see it because there is no data race, just a starvation chain.

## Options considered

### Option A: Hold the read-lock across the entire ServeContent
- **Pros**: Simplest; file cannot be reaped mid-serve while the lock is held.
- **Cons**: A slow/large download holds the store read-lock for the whole transfer. A waiting reaper write-lock then starves all new readers (RWMutex waiting-writer semantics) — new mints and serves block. Liveness bug invisible to `-race`.

### Option B: Lock only across resolve+open; stream from the held *os.File lock-free (CHOSEN)
- **Pros**: The store lock is held for microseconds. `ServeContent` streams from the already-open `*os.File` with no lock. On POSIX the open fd survives `unlink`, so the reaper can `os.Remove` the path concurrently without corrupting the in-flight serve.
- **Cons**: Slightly more careful handler structure; relies on POSIX open-fd-survives-unlink semantics (fine for the local-only macOS/Linux target).

## Decision

`GET /audio/{render_id}.wav` takes the render-store read-lock ONLY across resolve + `os.Open`, releases it, then `http.ServeContent` streams from the held `*os.File`.

Reasoning: a read-lock spanning the whole `ServeContent` lets a slow/large download hold the store read-lock; a waiting reaper write-lock then starves all new `/narrate` mints and `/audio` serves, because Go's `RWMutex` blocks new readers once a writer is waiting — a liveness bug `-race` cannot detect. On POSIX the open fd survives `unlink`, so the TTL reaper can `os.Remove` the file lock-free while a serve is in flight. A wall-clock test asserts that a slow in-flight serve does not block a concurrent mint.

## Consequences

- Download duration is fully decoupled from store-lock hold time; mints and serves stay responsive under slow clients.
- Correctness depends on POSIX open-fd-survives-unlink — acceptable for the local-only target; would need revisiting on a non-POSIX platform.
- A wall-clock (not `-race`) test guards the property, since the failure mode is starvation, not a data race.

## Related decisions

- [render_id wav lifecycle: TTL reaper + orphan-scan, deletes outside the store write-lock](../architecture/2026-06-28-render-id-wav-ttl-reaper-orphan-scan-lifecycle.md) — the reaper whose lock-free `os.Remove` this serve path is designed to tolerate.
- [Read-side per-dir mutex in /artifact](../concurrency/2026-06-22-artifact-read-side-per-dir-mutex.md) — sibling read-path concurrency decision on the other server route.

## Revisit trigger

If the server is ever targeted at a non-POSIX platform where an open fd does not survive unlink, the reaper-vs-serve interaction must be redesigned.
