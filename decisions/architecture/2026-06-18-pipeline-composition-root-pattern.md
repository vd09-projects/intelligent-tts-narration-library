# Pipeline composition root pattern

- **Date:** 2026-06-18
- **Status:** accepted
- **Category:** architecture
- **Tags:** [pipeline, composition-root, cmd, mcp, phase-one, issue-7]
- **Source:** harvested from cmd-narrate-issue-7 build summary v1, decision mark v1

## Context

Phase one needed a place to wire the four edges (adapter → planner → renderer → sink) into a runnable program. Two CLI binaries are already on the roadmap: `cmd/narrate` (this slice, issue #7) and `cmd/narrate-mcp` (phase 4 — MCP server). The React reference player at `player/` does not run a Go pipeline directly but consumes the JSON plan + timeline the pipeline produces.

The composition rule from CLAUDE.md is that only `pipeline/` and `cmd/` may import concrete edge implementations. `planner/` stays I/O-free and edge-agnostic. The decision was where exactly the composition logic lives — inside each `cmd/*/main.go` (per-binary wiring), in a global var (shared singleton), or in a dedicated struct in `pipeline/`.

## Decision

`pipeline.Pipeline` is the only struct in the project that holds concrete edge instances. Its single public method is `Narrate(ctx context.Context, ref plan.SourceRef, req NarrateRequest) (sink.SinkReceipt, error)`. The constructor `New(adapter, intelligence, renderer, sink, defaults)` takes interfaces, not concrete types. Each `cmd/*/main.go` constructs concrete edges and hands them to `pipeline.New`. `cmd/narrate-mcp` (phase 4) reuses the same struct unchanged.

## Rejected alternatives

1. **Per-cmd wiring code in each `cmd/*/main.go`.** The composition logic would duplicate across `cmd/narrate/main.go` and `cmd/narrate-mcp/main.go`. Any change to the pipeline shape (e.g., adding an intelligence-bypass flag, or a per-call OutDir) would need to land in two places and could silently drift.
2. **Global wiring var (singleton).** A package-level `var Default = pipeline.New(...)` shared across binaries is test hostile (hidden state, init ordering), and forces all binaries to use the same edges — defeating the point of having edge interfaces.

## Consequences

- Any future `cmd/*` binary that wants narration goes through `pipeline.Pipeline`. If a binary needs a different shape (e.g., streaming, partial reads), it should propose a new method on the same struct or a separate `pipeline.StreamingPipeline` — but not a parallel composition root.
- The pipeline struct is the contract the MCP server depends on. Renaming `Narrate` or changing its signature after this lands is breaking, even pre-1.0.
- `pipeline.New` panics on nil required edges. This is a programmer error, not a runtime condition. A future `NewE` error-returning variant may be added if the constructor needs typed-config validation (round-1 review S3).

## Related decisions

- Per-block WAVs decision (`architecture/2026-06-18-per-block-wavs-no-concat-in-renderer.md`) — informs why the pipeline does not concat audio.
- Voice resolution order decision (`convention/2026-06-18-voice-resolution-order.md`) — pipeline forwards opts through to the renderer.
