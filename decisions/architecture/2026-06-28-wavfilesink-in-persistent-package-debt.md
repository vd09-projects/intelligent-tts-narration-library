# WAVFileSink lives in package sink/persistent, accepted as debt

| Field    | Value            |
|----------|------------------|
| Date     | 2026-06-28       |
| Status   | accepted         |
| Category | architecture     |
| Tags     | speak-to-file, tech-debt, package-naming, wavfile-sink, wavconcat, accepted-debt |

## Context

The new single-wav `WAVFileSink` (backing `speak_to_file`) reuses the extracted
`buildSegments` wav-concat core. That core lives in package `sink/persistent`,
so the new sink was placed there too. The package name `persistent` now
**overstates its contents**: it holds both the 3-file directory sink and the
single-wav sink.

## Options considered

### Option A: Hoist the shared core into `internal/wavconcat`, split the package
- **Pros**: package names match contents; the single-wav sink is not misfiled
  under `persistent`.
- **Cons**: premature package split to satisfy naming before any objection has
  surfaced; more moving parts for one new sink.

### Option B: Keep WAVFileSink in `sink/persistent` (accept the debt)
- **Pros**: reuse the extracted `buildSegments` core directly, no premature
  split; ships the priority tool fastest.
- **Cons**: the `persistent` package name overstates its contents.

## Decision

Chose **Option B**, accepted as deliberate **debt**. `WAVFileSink` lives in
package `sink/persistent` to reuse the extracted `buildSegments` core without a
premature package split. A self-describing code marker records this in
`wavfile.go` (no issue number).

## Consequences

- The `sink/persistent` package name no longer precisely describes its contents.
- The debt is tracked by a code marker rather than a ticket.

## Revisit trigger

Trigger-on-objection: hoist the shared wav-concat core into
`internal/wavconcat` (and split the package) the first time the naming mismatch
causes confusion or someone objects.

## Related decisions

- [WAVFileSink reuses persistent-sink wav-concat math, no sidecars](2026-06-28-wavfilesink-reuses-wav-concat-no-sidecars.md) — the `buildSegments` extraction this placement reuses.
- [Ship speak_to_file as a separate MCP tool](2026-06-28-speak-to-file-separate-mcp-tool.md) — the tool this sink backs.
