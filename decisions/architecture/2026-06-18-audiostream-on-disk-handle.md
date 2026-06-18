# AudioStream is an on-disk handle, not in-memory bytes

- id: 2026-06-18-audiostream-on-disk-handle
- date: 2026-06-18
- status: accepted
- category: architecture
- tags: [render, audiostream, memory, sink, phase-one]

## Decision

`render.AudioStream` carries `Dir`, `Files []string` (block-id-ordered relative names of non-empty rendered blocks), and `ManifestPath` — a handle pointing at WAVs on disk. It does NOT carry `[]byte` per-block audio.

## Why

Phase-one Kokoro produces ~48 000 bytes/sec at 24 kHz mono s16le. A long document with dozens of blocks could cost 50–200 MB of resident bytes if held in-RAM, with no benefit — the sink is the only consumer and it either streams to a device (ephemeral) or copies to a file (persistent). Both prefer to read straight from disk.

Keeping bytes on disk also makes `RenderBlock` (the escalation path) actually surgical: re-render one WAV, swap the file, sink picks it up — no in-memory state to invalidate.

## Rejected alternatives

- **`AudioStream{Bytes [][]byte}`** — would balloon memory, complicate `RenderBlock`'s "patch one block" story, and force the sink to choose between in-memory and on-disk paths. Rejected.
- **`AudioStream{Stream io.Reader}`** — single-reader handle would prevent the sink from reading blocks out of order or in parallel. Rejected for phase one; might be added as an additional handle type later if streaming sinks demand it.
