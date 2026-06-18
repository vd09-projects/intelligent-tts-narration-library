# Empty-text blocks emit zero-duration timing with empty AudioRef

- id: 2026-06-18-empty-text-blocks-zero-ms-no-audioref
- date: 2026-06-18
- status: accepted
- category: convention
- tags: [render, timeline, audioref, pause, phase-one]

## Decision

When a voiced block has no speakable text (all-pause segments, or no speech segments at all), `render/sherpa/Engine.Render`:

- Skips the Kokoro subprocess call.
- Writes no WAV file.
- Omits the block from `AudioStream.Files`.
- Emits a `BlockTiming{StartMs: cursor, EndMs: cursor, AudioRef: ""}` so the timeline still has one row per block in plan order.

The empty `AudioRef` (zero-value string, `omitempty` in JSON) signals "no audio for this block."

## Why

Round-1 review (B2) found that emitting `AudioRef = "<blockID>.wav"` for a block that wrote no file would make downstream sinks ENOENT when they tried to open the WAV. Worse: the manifest would list a file that doesn't exist.

The alternative — writing a 44-byte empty WAV — would force the sink into the opposite confusion (a "real" file with zero audio content). Empty `AudioRef` is the honest signal: "this block was acknowledged in the timeline; nothing was written for it; deal with it according to your sink's policy (skip, pause, etc.)."

Phase-one note: pauses live entirely sink-side; the renderer treats them as zero-duration. If a future phase wants the renderer to synthesize silent WAVs for pause segments, this decision is replaced.

## Rejected alternatives

- **Set `AudioRef = "<blockID>.wav"` and write an empty file** — sink sees a file, opens it, plays nothing, hides the data gap. Honest-rule-adjacent: better to surface the absence.
- **Omit empty-text blocks from `Timeline.Blocks` entirely** — breaks 1:1 alignment between `plan.Blocks` and `Timeline.Blocks`, which is the load-bearing sync invariant.
- **Error on empty-text blocks** — would make perfectly valid plans (all-pause blocks for emphasis) unrenderable.
