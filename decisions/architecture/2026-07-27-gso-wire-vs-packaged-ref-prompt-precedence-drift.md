# GSO ref_audio/prompt_text has two homes (wire + packaged) — wire is authoritative, drift maps onto the closed taxonomy, single-source redesign deferred to an AC edit

| Field    | Value            |
|----------|------------------|
| Date     | 2026-07-27       |
| Status   | accepted         |
| Category | architecture     |
| Tags     | gso, gpt-sovits, wire-contract, source-of-truth, ref-audio, prompt-text, precedence, drift-validation, bad-args, bad-voice, closed-taxonomy, ac-lock, deferred-redesign, issue-161, issue-162 |

## Context

Issue #161's two locked acceptance criteria create two homes for the same reference data. **AC1 (locked)** packages a per-voice `ref_audio.wav` + `ref_transcript.txt` (alongside the `gpt.ckpt`/`sovits.pth` pair), resolvable from `<voice_id>`. **AC2 (locked)** puts `<ref_audio_path>` + `<prompt_text>` on the wire request line. Plan review round 1 flagged this as two sources of truth with no ERR category for wire-vs-packaged drift. Because both AC1 and AC2 are locked ticket text (only the ticket owner may edit them), the plan could not simply drop one home; it had to keep both and define how they relate — without inventing a new open-ended error category.

## Options considered

### Option A: Drop the wire tokens, resolve everything from `<voice_id>` (single source of truth)
- **Pros**: Eliminates the drift class entirely; wire value == packaged value by construction, so transcript enforcement becomes moot; cleanest long-term design.
- **Cons**: Requires editing AC1/AC2 (locked ticket text) and re-freezing the wire contract. Out of scope for #161; a ticket-owner decision, not something to do silently in a build plan.

### Option B: Keep both homes with no precedence rule
- **Pros**: No AC edit needed.
- **Cons**: Two sources of truth that can silently disagree; no defined behavior on drift; exactly the round-1 blocking finding.

### Option C (chosen): Keep both homes, define precedence, map drift onto the existing closed taxonomy, defer the single-source redesign to an AC edit
- **Pros**: Respects the AC lock, gives a deterministic answer on drift, invents no new error category, and records the cleaner redesign as an explicit ticket-owner recommendation rather than a silent change.
- **Cons**: Two homes persist; the best-effort drift compare is a coarse guardrail, not transcript enforcement, so subtle transcript edits still pass (the wire value wins).

## Decision

Both homes are kept (both are locked ticket text). Precedence and drift validation are defined as:

- **Precedence:** the **wire tokens (`<ref_audio_path>`, `<prompt_text>`) are authoritative** for what is fed to `.run()`. The packaged `ref_audio.wav` / `ref_transcript.txt` are the **source-of-record / DR backup** and the canonical values #162 SHOULD emit on the wire.
- **Drift validation, mapped onto the CLOSED ERR taxonomy (no new category invented):**
  - `<ref_audio_path>` resolves into a **different voice's** packaged dir than `<voice_id>` (wrong-voice identity drift) → **`bad-voice`**.
  - `<ref_audio_path>` / `<prompt_text>` are present but **inconsistent with THIS voice's** packaged artifacts (path outside the voice's packaged dir when a packaged ref exists, or `prompt_text` mismatches the packaged `ref_transcript.txt` on a best-effort compare) → **`bad-args`**.
- **Scope limit of the best-effort `prompt_text` compare:** it is a coarse guardrail against *gross* drift (wrong-voice references, out-of-dir paths), **NOT** a transcript-enforcement mechanism. Because the wire `prompt_text` is authoritative and is what's fed to inference, the compare cannot police subtle transcript edits — a reworded-but-plausible `prompt_text` still wins on the wire and is voiced as given. Byte-for-byte transcript verification is explicitly not mandated in phase one.
- **The cleaner single-source-of-truth redesign (Option A) is explicitly DEFERRED to a ticket-owner AC1/AC2 edit** — flagged as a recommendation in the plan and this record, not done silently in the build.

## Consequences

- Deterministic, taxonomy-compatible behavior on drift; #162 branches on the same closed category set (`bad-args`/`bad-voice`) it already handles.
- Two homes for the same data remain a latent smell; the drift rule contains it but does not remove it. The clean fix needs an AC edit and a wire re-freeze.
- A future engineer must not mistake the best-effort `prompt_text` compare for transcript enforcement.
- Build review round 2 confirmed the drift rule and its ERR mapping are implemented and torch-free-testable (the contract test exercises the drift → ERR path).

## Related decisions

- [GSO worker wire contract is RVC-shaped but NOT verbatim](../architecture/2026-07-27-gso-worker-wire-contract-rvc-shaped-not-verbatim.md) — the frozen wire contract whose `<ref_audio_path>`/`<prompt_text>` tokens create the two homes this decision governs.
- [RVC worker stdin/stdout wire contract — closed ERR taxonomy + startup/runtime FATAL exit-code split](../architecture/2026-07-22-rvc-worker-wire-contract-err-taxonomy-exit-codes.md) — the closed, append-only ERR taxonomy that drift is mapped onto instead of adding a new category.

## Revisit trigger

Revisit when the ticket owner decides whether to collapse to a single-source-of-truth wire (drop `<ref_audio_path>`/`<prompt_text>`, resolve from `<voice_id>`) — that edits AC1/AC2 and re-freezes the wire, superseding this precedence/drift rule. Also revisit if phase two mandates byte-for-byte transcript enforcement (the current best-effort compare explicitly does not provide it).
