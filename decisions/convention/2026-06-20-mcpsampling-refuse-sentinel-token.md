# mcpsampling refusal sentinel — literal __REFUSE__ as the leading token

- **Date:** 2026-06-20
- **Status:** experimental
- **Category:** convention
- **Tags:** intelligence, mcpsampling, refusal, honesty-rule, llm-contract, issue-13

## Context

The mcpsampling adapter calls the MCP client LLM via `sampling/createMessage` and must distinguish two non-error outcomes: the LLM produced a faithful summary, or the LLM refused because it could not summarize without inventing details (CLAUDE.md honesty rule). The MCP spec provides `StopReason` (values `endTurn`, `stopSequence`, `maxTokens`, `toolUse`), but stop-sequence honoring is documented as may-ignore for clients. The adapter needs a refusal signal that survives every reasonable MCP client implementation.

## Decision

The refusal sentinel is the literal ASCII token `__REFUSE__` as the **very first non-whitespace characters** of the assistant's reply, optionally followed by a short reason after one space. The adapter:

1. `strings.TrimLeftFunc(text, unicode.IsSpace)` — strip leading whitespace.
2. `strings.HasPrefix(trimmed, "__REFUSE__")` — must match exactly here.
3. If matched: strip the sentinel, trim the remainder, return `IntelligenceResult{Refused: true, RefusalNote: <remainder>}`.
4. If not matched (including the sentinel appearing mid-body): the entire reply is a successful summary, returned as `IntelligenceResult{Text: <reply>}`.

Every per-class system prompt restates the contract: *"If you cannot summarize without inventing details, respond with the literal token `__REFUSE__` as the very first non-whitespace characters of your reply, optionally followed by a short reason after one space. Do not produce a misleading summary. The token `__REFUSE__` must not appear anywhere else in your reply."*

## Justification

- **Robust across MCP clients.** Stop sequences are advisory in the MCP spec — a client may ignore them entirely or honor them inconsistently. A first-token check survives every conformant client.
- **Cheap to parse.** O(n) where n = leading whitespace count + 10 bytes; no JSON parsing, no regex.
- **Boundary-explicit.** The "very first non-whitespace characters" rule means a model that mentions `__REFUSE__` in the middle of a real summary (e.g., explaining a token-handling library) does not accidentally trip the refusal path. The test `refusal_sentinel_not_at_start_is_content` pins this.
- **Honesty rule respected.** Refusals are data (`Status: refused` upstream in the plan), errors stop. The sentinel is a strictly data-side signal; it never returns a Go error.

## Rejected alternatives

- **`stopReason == "stopSequence"` matching.** Fragile — the MCP spec marks stop-sequence honoring as may-ignore. Rejected.
- **JSON-mode reply (`{"refused": true, "note": "..."}`).** Adds parsing overhead for a one-byte signal; brittle when the LLM emits invalid JSON; doubles the prompt size for the "just say no" path. Rejected.
- **Heuristic refusal detection (e.g., "I cannot" / "I refuse" / "Sorry").** Locale-sensitive, model-sensitive, false-positive-prone. The honesty rule demands a sharp data-side signal. Rejected.

## Consequences

- Every prompt template `System` string contains the contract text. Test `TestRenderPrompt_SystemContainsRefusalContract` enforces this — accidental contract drift fires the test.
- The package doc carries the contract verbatim (`mcpsampling.go`).
- The adapter's `parseRefusal()` helper is the canonical parser. Tests cover: refusal at start, refusal with leading whitespace, sentinel mid-body is content, empty refusal note.
- A model that emits `__REFUSE__` accidentally as the first token (e.g., trying to explain refusal-handling) triggers a false refusal. Acceptable for phase one — the prompt explicitly forbids this, and the bare-image / opaque-region "refuse without calling" path covers most "model unsure" cases.

## Related decisions

- The Severity 2-value boundary decision (2026-06-20) explicitly excludes refusals from `Diagnostic.Severity` — they live as `Block.Status: refused`, not as a severity level.
- [2026-06-18-honesty-rule-baked-into-adapter-contract](../) — predecessor: the parent `intelligence/IntelligenceAdapter` docstring already mandates `Refused: true` over fabrication; this decision picks the wire-level signal.

## Revisit trigger

- If an MCP client materializes that strips leading whitespace differently from `unicode.IsSpace` (e.g., Unicode BOM markers passed through verbatim) — would require a more permissive trim. Currently `strings.TrimLeftFunc` covers all whitespace categories.
- If a future adapter (#15 Anthropic direct-API) wants a richer refusal signal (structured reason categories, retry-vs-give-up flag), revisit whether the token-prefix protocol generalizes or each adapter picks its own.

## Source

Inline mark `**Decision (v2) — convention: experimental.**` in `planner-task.md v2` for scope `intelligence-mcpsampling-issue-13`. Implemented in commit `2704618` (Phase 3 — `parseRefusal()` + `__REFUSE__` constant + system-prompt contract).
