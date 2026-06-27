# Cap the MCP speak per-block transcript via head-keep tail-truncate by entry count

| Field    | Value            |
|----------|------------------|
| Date     | 2026-06-27       |
| Status   | accepted         |
| Category | architecture     |
| Tags     | transcript-cap, mcp, speak, head-keep, tail-truncate, observability, additive-compatible, omitempty, byte-identical, honesty-rule, issue-86, adr-77 |

## Context

The MCP `speak` tool returns a per-block `transcript[]` on its receipt (ADR #77 Channel 1, issue #78). The array was **unbounded** — one entry per block — and a very large `speak` response (a long document with hundreds of blocks) shipped that whole array *twice*: once in `structuredContent` and again in the duplicate serialized-JSON `TextContent` block (ADR #77 D3). A number-less `TODO(#86)` sat at the `runSpeak` cap site where `transcriptFromResult(receipt)` is assigned into `speakResponse.Transcript`.

Issue #86 required bounding that transcript while honoring hard constraints: additive-compatible only within the current `schema_version` (new fields `omitempty`, nothing removed/renamed/retyped); byte-identical output for the common under-cap case; no new source/spoken-text leak; `receipt` shape untouched; and no `Refusal` involved (this caps an observability receipt, not narration content).

## Options considered

### Option A: Head-keep, tail-truncate by entry count (chosen)
- **Pros**: deterministic and trivially testable; the kept slice is a contiguous `Order 0..N-1` prefix — exactly what a front-to-back listener reaches first, preserving the listen transport's plan-order contiguity assumption; whole-entry drop only (never mutates a kept entry, never surfaces new text); under-cap case returns the input slice header unchanged (same backing array, no copy) so both `omitempty` signal fields elide and the wire response stays byte-identical.
- **Cons**: can drop *refused* tail entries from the transcript view (refusals are high-signal rows); mitigated because refused blocks were still spoken and remain counted in `receipt.blocks_played`.

### Option B: Elide-middle
- **Cons**: breaks the listen transport's plan-order contiguity assumption — consumers walk `transcript[]` from `Order=0`; a hole in the middle violates that contiguous-prefix contract.

### Option C: Byte-budget cap
- **Cons**: non-deterministic; would force mid-entry `SpokenText` truncation, leaking a partial-spoken-text variant — barred by the Channel-2 no-text-leak decision.

### Option D: Caller-tunable `speakArgs` field
- **Cons**: deferred — the v1 envelope stays minimal. Promote to configurable only if a real need appears.

## Decision

Cap the per-block `transcript[]` using **head-keep, tail-truncate by entry count**, governed by a single named `const transcriptMaxEntries = 200`. The cap is a pure, total, allocation-free helper (`capTranscript`) applied to `transcriptFromResult`'s output at the single `runSpeak`/`runSpeakWithCache` success-path site — projection and bounding stay independently testable, and the dual-channel handler is untouched (capping the one `speakResponse` before it marshals once bounds both channels).

Truncation is signaled by **two additive `omitempty` fields** on the `speakResponse` struct: `transcript_truncated` (bool) and `transcript_omitted_count` (int). Under the cap, `capTranscript` returns its input unchanged with `false, 0`, so both fields elide and the serialized response is byte-identical to before. Over the cap it returns `entries[:transcriptMaxEntries]` with `truncated=true` and `omitted=len-cap`. The `cap <= 0` branch is a documented defensive no-op. The error path is untouched (nil transcript, no signal fields).

Bounding an observability receipt is **not fabrication** — the honesty rule is untouched: audio and plan are unaffected, refusals are still spoken at playback time and still counted in `receipt.blocks_played` (sourced from `SinkReceipt`, independent of the transcript array). A truncated transcript is documented (in both the cap-site comment and the client-visible tool `Description`) as **not an exhaustive refusal ledger**.

## Consequences

- The common (≤200-block) `speak` response is byte-identical to pre-change output; the ADR #77 / observer byte-equal invariant is now cap-conditional (holds only under the cap).
- Pathological (>200-block) responses get a bounded, self-describing payload across both channels.
- The cap is one named, justified constant — re-tuning is a one-line change, and the truncation signal lets a consumer detect when it bites.
- Known follow-on (a deferred *task*, not a decision): refusal-aware omission accounting when a >200-block document drops tail refusals (e.g. `transcript_omitted_refused_count`, or a refusal-preferring retention strategy).

## Related decisions

- [ADR: Playback observability & control model (issue #77)](2026-06-24-playback-observability-control-model-issue-77.md) — this caps the Channel-1 transcript that ADR #77 D3 defines (double-shipped in structuredContent + serialized TextContent).
- [Channel-2 live observer mechanism: append-only JSONL + tail -f](2026-06-26-channel2-mechanism-jsonl-tail-over-mcp-progress.md) — the no-source/spoken-text-on-the-wire (Channel-2 no-text-leak) constraint that rules out the byte-budget alternative.
- [MCP speak response is a receipt-only envelope](../convention/2026-06-19-mcp-speak-response-receipt-only-envelope.md) — the receipt-only-envelope decision the transcript sibling rides on; the cap lives on the `transcript` sibling, never the `receipt`.

## Revisit trigger

If a >200-block document with tail refusals becomes a real usage (file the refusal-aware omission accounting task), or if a consumer needs the cap configurable per-call (promote the deferred caller-tunable `speakArgs` field).
