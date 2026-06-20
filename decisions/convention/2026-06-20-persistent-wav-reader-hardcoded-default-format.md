# Persistent sink WAV reader hard-coded to render.DefaultFormat()

- **Date:** 2026-06-20
- **Status:** accepted
- **Category:** convention
- **Tags:** [sink, persistent, wav, format-validation, kokoro, phase-one, issue-16]
- **Owner:** vd
- **Scope:** issue-16

## Context

`sink/persistent` reads per-block WAVs the renderer produced and concatenates them into a single `audio.wav`. The WAV reader needs to know what format to accept. Two extremes: hard-code the phase-one Kokoro format (24 kHz mono PCM s16le) or implement a format-agnostic reader that accepts whatever it parses.

## Options considered

### Option A: Hard-code to render.DefaultFormat() (CHOSEN)
- **Pros**: Cross-engine format negotiation is a phase-two concern. Tight constraint catches upstream renderer regressions early — a Kokoro upgrade that silently changes sample rate becomes a loud test failure. Reader code path stays minimal and fully testable. Single source of truth (`render.DefaultFormat()`) means no drift between renderer and sink.
- **Cons**: A second renderer with a different format would require a code change before the sink could consume its WAVs.

### Option B: Format-agnostic reader (accepts whatever the WAV header reports)
- **Pros**: New renderers drop in without code change.
- **Cons**: Untestable branches (each format permutation needs coverage). No regression alarm when the renderer drifts. Adds complexity for a phase-two need.

## Decision

Persistent sink validates each per-block WAV against `render.DefaultFormat()` (or the override passed via `WithExpectedFormat`). Format mismatches return a `formatMismatchError` naming the divergent field (sample_rate, channels, bits_per_sample, format_tag) plus the block's source path. The error wraps `ErrInvalidWAV` only for true container corruption (missing magic, truncated subchunks); format mismatches are a distinct error type.

The `WithExpectedFormat(plan.AudioFormat)` Option exists so a future engine doesn't require library forking — callers can wire a different expected format at composition time.

## Consequences

- Upstream renderer regressions surface as test failures and runtime errors naming the divergent field, not as silent audio corruption.
- The reader's branch count stays small; tests cover the full happy path + every documented failure mode.
- Adding a second renderer with a different native format takes one composition-root line (`persistent.New(out, persistent.WithExpectedFormat(otherFmt))`) plus a `bitDepthFromEncoding` entry in `wav.go`.

## Related decisions

- [Sink receipt totals planned duration not wall time](2026-06-19-sink-receipt-planned-duration-not-wall-time.md) — same "trust the plan, not the device" principle.
- [Per-block WAVs no concat in renderer](../architecture/2026-06-18-per-block-wavs-no-concat-in-renderer.md) — establishes the per-block WAV contract this sink reads.

## Revisit trigger

When a second renderer with a different native format ships (e.g. Piper voices via sherpa-onnx-go), promote `WithExpectedFormat` to a required argument, or document the composition pattern for callers to pin per-renderer formats.
