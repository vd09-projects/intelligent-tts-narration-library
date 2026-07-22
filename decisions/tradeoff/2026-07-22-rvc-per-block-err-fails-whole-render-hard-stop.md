# A per-block RVC worker ERR fails the whole Render in phase one (no per-block skip or degrade)

| Field    | Value            |
|----------|------------------|
| Date     | 2026-07-22       |
| Status   | accepted         |
| Category | tradeoff         |
| Tags     | rvc, voice-conversion, render-decorator, error-handling, honesty-rule, all-or-nothing, per-block-err, no-skip, no-degrade, uniform-format, timeline, 40khz, error-not-refusal, issue-145 |

## Context

The RVC worker returns exactly one line per block: `OK <out>` or `ERR <category> <message>`, where category comes from a closed v1 set `{ bad-args | bad-voice | read-failed | infer-failed | write-failed }`. The `render/rvc` decorator (#145) reads those lines. The open question was **how the decorator reacts to a per-block `ERR`** — abort the whole render, skip just that block, or fall back to the block's plain 24 kHz Kokoro audio.

The stakes: when RVC is on, the whole timeline is uniform 40 kHz. A single unconverted block would either leave a gap or reintroduce 24 kHz audio into a 40 kHz timeline. The project honesty rule draws a hard line — errors stop the pipeline; refusals are spoken, surfaced data — and forbids fabrication. A worker `ERR` is an **error**, not a readable-but-unvoiceable refusal, so it must not be dressed up as a refusal or silently repaired.

## Options considered

### Option A: skip the failed block
- **Cons**: produces a non-uniform timeline with a silent gap where the block should be — the `Format`/`Timeline` contract (block-keyed, uniform sample rate) breaks, and the listener silently loses content. Rejected.

### Option B: degrade the failed block to its plain 24 kHz Kokoro audio
- **Cons**: mixes a 24 kHz block into an otherwise-40 kHz timeline (format inconsistency), and silently substitutes a different-quality/timbre rendering for the one that was asked for — a fabricated-quality signal. Both violate the honesty rule (never fabricate; a substituted repaint is a lie about what was voiced). Rejected.

### Option C: any per-block ERR is a hard error that stops the entire Render — CHOSEN
- **Pros**: the 40 kHz `Format`/`Timeline` stays uniform (all blocks converted or none shipped); no fabricated substitution; `bad-args` / `bad-voice` — which signal a *decorator bug* constructing the request line — surface loudly instead of being swallowed; consistent with "errors stop the pipeline, refusals are spoken data."
- **Cons**: one failed block fails the whole job — an RVC render is all-or-nothing. Accepted as the honest behavior for phase one.

## Decision

**ANY per-block `ERR` from the worker is a hard error that stops the entire `Render`**, returned up the pipeline. It is **not** a per-block skip and **not** a degrade back to the block's 24 kHz Kokoro audio.

Rationale, tied to the invariants:

- Mixing a 24 kHz Kokoro block into an otherwise-40 kHz timeline breaks the uniform `Format`/`Timeline` contract.
- Silently substituting or fabricating a repaint violates the honesty rule — never fabricate; **errors stop the pipeline, refusals are spoken data, and this is an error, not a refusal.**
- `bad-args` and `bad-voice` specifically indicate the decorator built a malformed request line (a code bug), and must fail loudly rather than degrade — a degrade would hide the bug behind plausible-sounding audio.

## Consequences

- An RVC job is all-or-nothing at 40 kHz: every block converts, or the whole `Render` fails.
- Failures stop the pipeline loudly, carrying the worker's `ERR <category> <message>` up as the error (a fix hint for the caller — e.g. `bad-voice` points at an unmapped target, `bad-args` at a request-line bug).
- No partial/quality-inconsistent timeline can ever ship; consumers of the plan + timeline never see a 24 kHz island inside a 40 kHz render.
- Aligns with the honesty-rule-at-the-subprocess-edge theme established by the #144 worker decisions: the worker refuses/reports cleanly; the decorator translates a report into a pipeline-stopping error rather than a silent repair.

## Related decisions

- [RVC worker stdin/stdout wire contract — closed ERR taxonomy + startup/runtime FATAL exit-code split](../architecture/2026-07-22-rvc-worker-wire-contract-err-taxonomy-exit-codes.md) — defines the `ERR <category> <message>` grammar this decision decides how to *react* to; the closed category set is what lets `bad-args`/`bad-voice` be recognized as decorator bugs.
- [Torch-free ONNX RVC via an ephemeral per-job worker, wrapped as a render decorator](../architecture/2026-07-22-torch-free-onnx-rvc-ephemeral-worker.md) — the parent decision establishing 40 kHz-end-to-end-when-RVC-on and worker-unavailable → hard error; this extends that hard-error stance from "worker missing" to "worker reports a per-block ERR."
- [RVC decorator owns the target->{Kokoro source, index_rate, pitch} map; translation happens exactly once](../architecture/2026-07-22-rvc-decorator-owns-voice-map-single-translation.md) — a `bad-args`/`bad-voice` ERR most often means the decorator's map/request-line construction is wrong, so this hard-stop makes that bug surface immediately.

## Revisit trigger

Reconsider if phase two introduces a genuine per-block refusal path for RVC (readable-but-unrepaintable content that should be *spoken* as a refusal rather than error out), or if a mixed-rate timeline ever becomes a supported contract (would allow a documented, non-fabricated degrade instead of an all-or-nothing failure).
