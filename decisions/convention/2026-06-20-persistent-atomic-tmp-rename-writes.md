# Persistent sink uses atomic tmp+rename writes for all three output files

- **Date:** 2026-06-20
- **Status:** accepted
- **Category:** convention
- **Tags:** [sink, persistent, atomic-write, honesty-rule, partial-state, ctx-cancel, issue-16]
- **Owner:** vd
- **Scope:** issue-16

## Context

`sink/persistent` writes three files: `audio.wav`, `plan.json`, `manifest.json`. The honesty rule (CLAUDE.md) is load-bearing: refusal is data; partial state is not honest. A context-cancel mid-write, a disk-full mid-write, or a process kill mid-write must not leave behind a partial file that `CheckStale` would later mis-parse as authoritative.

POSIX guarantees that `rename(tmp, target)` is atomic on the same filesystem. Writing to a sibling tmp file and renaming over the target is the standard pattern.

## Options considered

### Option A: Atomic tmp+rename for all three files (CHOSEN)
- **Pros**: A cancel/crash/full-disk at any point leaves the previous output (or no output) on disk — never a corrupted file. `CheckStale` and downstream consumers see only finalized state. Matches the no-partial-state-on-disk honesty-rule extension.
- **Cons**: Slightly more code per write site (the tmp-create + rename ceremony). Atomic only on same-filesystem renames (cross-FS would silently fall back to copy on some OSes).

### Option B: Direct overwrite with `os.Create`
- **Pros**: Simpler code.
- **Cons**: A mid-write failure leaves a truncated file on disk that downstream tooling will dutifully parse and report as authoritative. Violates the honesty rule on bytes.

### Option C: Write-to-buffer-then-WriteFile
- **Pros**: Atomicity via `os.WriteFile` is not actually guaranteed across all platforms; this is a half-measure.
- **Cons**: Same as B — `os.WriteFile` is `os.Create` + `Write` + `Close` under the hood, not atomic against concurrent readers / mid-write failure.

## Decision

All three output writes go through a helper that:

1. Creates `.persistent-<tag>-*.tmp` in the same directory as the target (so rename is same-FS).
2. Writes the bytes to the tmp file.
3. Closes the tmp file.
4. `os.Rename(tmpPath, target)`.
5. On any failure, removes the tmp file best-effort and returns the error.

`atomicWriteFile(path, data, perm)` covers `plan.json` and `manifest.json` (data-in-memory writes). `writeAudio(path, format, segments)` covers `audio.wav` (streamed via `writeCombinedWAV` into the tmp file).

The tmp filename prefix (`.persistent-`) is chosen so a partial-cleanup leftover is greppable.

## Consequences

- A ctx-cancel mid-`Consume` returns `ctx.Err()` with a partial `SinkReceipt` and leaves NO files on disk for blocks not yet processed. Tests assert this directly (`TestConsume_CtxCancelBeforeBlock`, `TestConsume_MissingBlockAudio`, `TestConsume_FormatMismatch`).
- `CheckStale` never sees a half-written `manifest.json` — either the manifest exists in its complete form, or it doesn't.
- Concurrent readers (a user `cat`-ing `manifest.json` while a render is in flight) always see a consistent file, never a half-written one.

## Related decisions

- [Persistent-sink manifest carries no build timestamp](2026-06-20-persistent-manifest-no-build-timestamps.md) — companion idempotency guarantee.
- [refused-block message rendered](2026-06-18-refused-block-message-rendered.md) — refusal-is-data honesty rule, here extended to bytes-on-disk.

## Revisit trigger

If the output directory ever needs to live on a cross-filesystem mount (e.g. an NFS export, a tmpfs separate from the rest of `/tmp`), revisit. `os.Rename` is not atomic across filesystems on all platforms.
