# GET /sessions/{id}/messages pre-chunking is server-side structural-split-only, not a planner.Plan call

| Field    | Value            |
|----------|------------------|
| Date     | 2026-06-28       |
| Status   | accepted         |
| Category | architecture     |
| Tags     | narrate-server, http-bridge, structural-split, planner-boundary, no-io-in-planner, read-endpoint, segment-unexported, refusal-boundary, step-0-spike, issue-109 |

## Context

Issue #109's `GET /sessions/{id}/messages` endpoint must return messages broken into reasonably-sized chunks so the Earshot UI can render and selectively narrate them. The obvious reuse candidate was `planner.Plan` — it already segments documents. A Step 0 spike investigated whether the planner exposes a render-free / voice-free segmentation entry point.

The spike found it does not. `planner.Plan` is the only exported entry, and it voices + levels + refuses as a unit; the internal `segment()` function is unexported. Routing large messages through `Plan` would (1) REFUSE large prose (>120 words) per the honesty rule instead of structurally splitting it — exactly the wrong behavior for a read endpoint that just needs chunks — and (2) couple I/O / voicing concerns into a read path, violating the "no I/O in planner" posture and the error-vs-refusal boundary.

## Options considered

### Option A: Call planner.Plan from the messages endpoint
- **Pros**: Reuses existing segmentation; one code path.
- **Cons**: `Plan` voices/levels/refuses — large prose comes back REFUSED, not chunked. Pulls voicing + the refusal boundary into a plain read endpoint. `segment()` is unexported, so there is no clean voice-free seam to call.

### Option B: Server-side structural splitter in package main, structural-split-only (CHOSEN)
- **Pros**: Returns plain ordered blocks with no voicing, no leveling, no refusal. Leaves `planner/` and `plan/` imports untouched. Read endpoint stays a read endpoint.
- **Cons**: A second, simpler splitter exists alongside the planner's segmentation; some conceptual duplication of "where do I cut text."

## Decision

`GET /sessions/{id}/messages` pre-chunking is STRUCTURAL-SPLIT-ONLY, performed by a server-side splitter living in `package main`, NOT by a `planner.Plan` call.

Reasoning: the Step 0 spike confirmed the planner exposes no render-free / voice-free segmentation entry — `planner.Plan` is the only exported entry and it voices + levels + refuses (`segment()` is unexported). Sending big messages through it would REFUSE large prose (>120 words) instead of structurally splitting it, and would couple I/O / voicing into a read endpoint. The server-side structural splitter returns plain ordered blocks and keeps `planner/` and `plan/` imports untouched, preserving the planner's purity and the read endpoint's simplicity.

## Consequences

- The messages endpoint never refuses content — it always returns chunks, which is the correct contract for a UI read path.
- `planner/` and `plan/` keep their existing import surface; the server does not become a planner consumer for this endpoint.
- A lightweight structural splitter now lives in `package main`. If a render-free segmentation entry is ever extracted from the planner, this splitter is the candidate to retire against it.

## Related decisions

- [Shared transcript parser lives in internal/transcript](../architecture/2026-06-28-shared-transcript-parser-lives-in-internal.md) — the #106 parser feeds the message list this endpoint chunks; that decision's forward contract for #109 governs message identity/pagination.

## Revisit trigger

If the planner later exports a voice-free / refusal-free segmentation entry point, reconsider whether this server-side splitter should delegate to it.
