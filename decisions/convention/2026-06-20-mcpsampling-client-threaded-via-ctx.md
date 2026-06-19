# mcpsampling client threaded via ctx, not adapter constructor

- **Date:** 2026-06-20
- **Status:** experimental
- **Category:** convention
- **Tags:** intelligence, mcpsampling, mcp, server-session, ctx-threading, composition-root, pipeline, issue-13

## Context

The mcpsampling adapter needs a `*mcp.ServerSession` to call `CreateMessage`. The session is a per-request resource — it lives for the duration of one MCP tool call, then ends. Where does the adapter get it?

Options:

- **A. Construct the adapter per-call.** `pipeline.New(... mcpsampling.New(session) ...)` runs once per call. Works, but commits `pipeline.New` to know about MCP sessions — a layer crossing.
- **B. Widen the interface.** Add `VoiceWithClient(ctx, req, client)` to `IntelligenceAdapter`. Pollutes the contract for every adapter that doesn't need a per-call client.
- **C. Thread the client via `context.Context`.** Adapter exposes `WithSamplingClient(ctx, client)` and reads via an unexported ctx key. The pipeline stays engine-neutral.

## Decision

Option C — ctx-threading. The mcpsampling package defines:

```go
type SamplingClient interface {
    CreateMessage(ctx context.Context, params *mcp.CreateMessageParams) (*mcp.CreateMessageResult, error)
}
func WithSamplingClient(ctx context.Context, c SamplingClient) context.Context
// internal: samplingClientFromCtx(ctx) SamplingClient
```

`*mcp.ServerSession` satisfies `SamplingClient` as-is — its method signature matches verbatim. The speak handler in `cmd/narrate-mcp` calls `WithSamplingClient(ctx, req.Session)` before invoking the pipeline; the adapter extracts via the unexported `samplingClientFromCtx(ctx)` helper. If absent, `Voice()` returns `ErrNoSamplingClient` (routed to `internal_error:` by the classifier — operator bug, not caller).

## Justification

- **Pipeline stays engine-neutral.** `pipeline.New` does not need to know about MCP sessions. The single composition root in `cmd/narrate-mcp` is the only place that touches both the session AND the pipeline.
- **No interface widening.** `IntelligenceAdapter.Voice(ctx, req) (result, error)` stays the same for every adapter — direct-API Anthropic (#15) does not need a per-call client and does not pay the ctx-key cost.
- **Standard Go pattern.** Per-request context-bound resources (DB transactions, request IDs, traces) are routinely threaded via ctx. This is idiomatic, not exotic.
- **Testability.** Tests inject a `fakeSamplingClient` via the same `WithSamplingClient(ctx, fake)` path the production code uses — no per-call constructor variance to mock.

## Rejected alternatives

- **Option A (per-call pipeline.New).** Cleaner from a "values are explicit" perspective, but bleeds MCP types into `pipeline.New`. Rejected because the pipeline's composition surface is load-bearing; widening it for one adapter sets a precedent for every future adapter.
- **Option B (interface widening).** Pollutes the contract for every future adapter. Rejected.
- **Option D (global registry).** `mcpsampling.SetActiveClient(session)`. Rejected without serious consideration — global state for per-request resources is the classic reason for ctx-threading patterns.

## Consequences

- `intelligence/mcpsampling/client.go` exports `SamplingClient` and `WithSamplingClient`. Both are stable surface.
- `samplingClientFromCtx` is unexported — callers cannot bypass `WithSamplingClient`.
- `ErrNoSamplingClient` is a sentinel returned by `Voice()` when ctx is bare; classifier in `cmd/narrate-mcp` routes it to `internal_error:`.
- The speak handler in `cmd/narrate-mcp` has the only production call to `WithSamplingClient`. Diff: one line + a nil-Session guard.
- Test `TestVoice_NoClientInCtx_ReturnsSentinelError` pins the ErrNoSamplingClient path. Test `TestVoice_ParallelCallsShareCtxSession` pins that multiple goroutines using the same ctx see the same client (no per-adapter state).
- Tradeoff acknowledged: ctx-as-bag-of-values is sometimes criticized as smuggling. Here the smuggled value IS request-scoped, which is exactly what ctx is for — accepted with eyes open.

## Related decisions

- [2026-06-19-runspeak-newpipeline-composition-seam](./2026-06-19-runspeak-newpipeline-composition-seam.md) — `cmd/narrate-mcp` already has a `newPipeline` factory hook; Phase 5 widens its signature to accept the optional adapter. The two seams compose cleanly.
- [2026-06-19-mcp-error-classifier-caller-vs-internal-split](./2026-06-19-mcp-error-classifier-caller-vs-internal-split.md) — `ErrNoSamplingClient` routing to `internal_error:` follows this established split.

## Revisit trigger

- If a future intelligence adapter needs MULTIPLE per-call resources (e.g., a sampling client AND a separate tracing handle), this works — they each ride their own ctx key. Revisit only if the ctx-bag grows to >3 keys; at that point a typed "per-call deps" struct might read cleaner.
- If `IntelligenceAdapter` gains other per-call resource needs that all adapters share, lift to interface widening.

## Source

Inline mark `**Decision (v3) — convention: experimental.**` in `planner-task.md v2` for scope `intelligence-mcpsampling-issue-13`. Implemented in commit `2704618` (Phase 3 — `client.go`).
