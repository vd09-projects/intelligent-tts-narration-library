# Per-block WAVs stay separate; renderer does not concatenate

- id: 2026-06-18-per-block-wavs-no-concat-in-renderer
- date: 2026-06-18
- status: accepted
- category: architecture
- tags: [render, sink, audiostream, escalation, phase-one]

## Decision

`render/sherpa/Engine.Render` writes one WAV per block (`<blockID>.wav`) plus a `manifest.txt` listing them in plan order. It does NOT concatenate WAVs into a single output file. Concatenation — when and how — is a sink concern.

## Why

1. **`RenderBlock` is actually surgical.** Escalation re-renders one block. With per-block files, the sink swaps one WAV; with a monolithic file it would have to re-cat all blocks. The interface promise ("re-renders ONE block") only works if the artifact granularity matches.
2. **Sinks differ.** The ephemeral sink (phase two) streams blocks to the audio device sequentially with optional pauses between — it never needs a concatenated file. The persistent sink writes a single output bundle — it cats blocks at write-time. Pre-cat in the renderer would force the ephemeral sink to split the concatenation back apart.
3. **Manifest decouples ordering from filesystem listing.** Sinks don't need to glob the dir or parse filenames; they read `manifest.txt` and get `(order, block_id, audio_ref)` rows in plan order.

## Rejected alternatives

- **Concatenated single WAV** — breaks `RenderBlock`'s surgical patch story; forces the ephemeral sink to split.
- **In-memory `[]byte` per block** — separately rejected in `2026-06-18-audiostream-on-disk-handle`.
- **Renderer-managed virtual filesystem (e.g. a `fs.FS`)** — overkill phase one; on-disk files are the simplest sink contract.
