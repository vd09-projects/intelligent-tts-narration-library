# WAVFileSink reuses persistent-sink wav-concat math but writes only the combined wav, no JSON sidecars

| Field    | Value            |
|----------|------------------|
| Date     | 2026-06-28       |
| Status   | accepted         |
| Category | architecture     |
| Tags     | speak-to-file, wav-sink, buildSegments, persistent-sink, no-sidecars, dry |

## Context

`speak_to_file`'s contract is a single `.wav` written at a caller-given path.
The existing persistent sink writes a **3-file directory** —
`audio.wav` + `plan.json` + `manifest.json` — and its per-block
wav-concatenation walk is entangled with building that directory receipt. The
new `WAVFileSink` needs the same per-block wav-concat math but must emit only
the combined wav, none of the JSON sidecars.

## Options considered

### Option A: Duplicate the concat walk in WAVFileSink
- **Pros**: full independence; no refactor of the persistent sink.
- **Cons**: copies the per-block wav-concatenation math (drift risk between the
  two sinks; two places to fix a WAV-framing bug).

### Option B: Extract a shared `buildSegments` helper
- **Pros**: the concat walk lives once; each sink builds its own lean receipt
  on top of the shared segments + counts; the single-wav contract does not drag
  in the directory-manifest contract.
- **Cons**: a new unexported seam to maintain in `sink/persistent`.

## Decision

Chose **Option B**. Extract an unexported `buildSegments` helper out of
`sink/persistent.Consume` that returns **segments + counts only** (not a
receipt). `WAVFileSink` reuses `buildSegments` for the per-block
wav-concatenation math, then writes **only** the combined wav — no `plan.json`,
no `manifest.json`. Rationale: avoid duplicating the concat walk; each sink
builds its own lean receipt; the single-wav contract must not drag in the
directory-manifest contract.

## Consequences

- `Consume` and `WAVFileSink` share one concat path; a WAV-framing fix lands in
  one place.
- `buildSegments` returns segments + counts, deliberately not a receipt, so each
  sink owns its own receipt shape.
- `speak_to_file` output carries no `plan.json`/`manifest.json`.

## Related decisions

- [Ship speak_to_file as a separate MCP tool](2026-06-28-speak-to-file-separate-mcp-tool.md) — the tool this sink backs.
- [WAVFileSink lives in package sink/persistent (accepted debt)](2026-06-28-wavfilesink-in-persistent-package-debt.md) — placement of the new sink.

## Revisit trigger

If a caller needs the plan/manifest alongside the wav, reconsider whether
`WAVFileSink` should optionally emit the directory form.
