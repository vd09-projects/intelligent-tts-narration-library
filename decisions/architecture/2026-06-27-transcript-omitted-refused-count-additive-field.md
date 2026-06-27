# Report dropped refused blocks via additive transcript_omitted_refused_count on MCP speak receipt

| Field    | Value            |
|----------|------------------|
| Date     | 2026-06-27       |
| Status   | accepted         |
| Category | architecture     |
| Tags     | transcript-cap, mcp, speak, refusal-aware, omission-accounting, additive-compatible, omitempty, byte-identical, channel-2-no-text-leak, honesty-rule, issue-98, issue-86 |

## Context

Issue #98 is the follow-on to the accepted #86 decision (head-keep / tail-truncate cap on the per-block `transcript[]` of the MCP `speak` receipt). #86 bounds the transcript at `transcriptMaxEntries = 200` and explicitly named "refusal-aware omission accounting" — what happens when a >200-block document drops refused blocks in the truncated tail — as its deferred follow-on (a count, not a retention change).

The question for #98: when the dropped tail of the transcript contains refused blocks, how should the receipt surface that, without violating the constraints that shaped #86 (plan-order contiguity, no text on the wire, pure/allocation-free cap helper, byte-identical-under-cap)?

File: `cmd/narrate-mcp/main.go`.

## Options considered

### Option A: additive omitempty count field
Add `transcript_omitted_refused_count` (omitempty) to the speak response, reporting how many refused blocks fell in the dropped transcript tail. Count is enum-sourced (`entry.Status == string(plan.StatusRefused)`).
- **Pros**: Honors #86's named follow-on (a count, not a retention change). Additive-compatible within schema_version; consumers ignore unknown fields. Byte-identical wire response under cap (field elides via omitempty). Keeps `capTranscript` pure and allocation-free. No spoken/source text re-added to the wire.
- **Cons**: Count-only, not an exhaustive refusal ledger (accepted limitation, documented).

### Option B: refusal-preferring retention
Retain refused blocks preferentially when truncating, instead of strict head-keep.
- **Cons**: Breaks plan-order contiguity (the same property that got elide-middle rejected in #86 — consumers walk `transcript[]` from `Order=0` expecting a contiguous prefix). Makes `capTranscript` non-pure and allocation-bearing. Re-adds spoken/source text to the wire, violating the Channel-2 no-text-leak constraint.

## Decision

Chose **Option A**: add an additive `omitempty` count field `transcript_omitted_refused_count` reporting how many refused blocks fell in the dropped transcript tail.

Rationale: Option A honors the accepted #86 head-keep/tail-truncate decision, which explicitly named "refusal-aware omission accounting" as its follow-on — a count, not a retention change. The count is enum-sourced (`entry.Status == string(plan.StatusRefused)`), additive-compatible, and byte-identical under cap.

Option B was rejected because it breaks plan-order contiguity (the same property that got elide-middle rejected in #86), makes `capTranscript` non-pure and allocation-bearing, and re-adds spoken/source text to the wire, violating the Channel-2 no-text-leak constraint.

## Consequences

- The receipt now reports *how many* refused blocks were dropped past the cap, but not *which* ones — accepted limitation: count-only, not an exhaustive refusal ledger.
- Under cap the field elides (omitempty), so the wire response stays byte-identical to before.
- `capTranscript` remains pure and allocation-free.
- Honesty rule untouched: audio + plan unaffected; refused blocks are still spoken at playback and still counted in `receipt.blocks_played`.

## Related decisions

- [Cap the MCP speak per-block transcript via head-keep tail-truncate by entry count](2026-06-27-cap-mcp-speak-transcript-head-keep-tail-truncate.md) — parent decision (#86); this is its named refusal-aware-omission-accounting follow-on (#98).

## Revisit trigger

If consumers need to know *which* tail blocks were refused (not just how many), revisit toward an exhaustive refusal ledger — bearing in mind the Channel-2 no-text-leak constraint.
