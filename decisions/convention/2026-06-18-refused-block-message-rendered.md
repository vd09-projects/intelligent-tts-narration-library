# Refused blocks render Refusal.Message through the same Kokoro path

- id: 2026-06-18-refused-block-message-rendered
- date: 2026-06-18
- status: accepted
- category: convention
- tags: [render, refusal, honesty-rule, phase-one]

## Decision

In `render/sherpa/`, blocks with `Status == StatusRefused` are rendered by feeding `Block.Refusal.Message` to the same Kokoro subprocess call used for voiced blocks. No marker tone, no second audio channel, no special voice. A `BlockTiming` is emitted with `AudioRef = "<blockID>.wav"` exactly as if the block were voiced.

If `Block.Refusal.Message` is empty (after trimming whitespace), the renderer returns `ErrMalformedPlan` — that is an upstream bug (planner emitted a refused block without a message), not a refusal, so it stops the pipeline.

## Why

CLAUDE.md honesty rule: "refused blocks still rendered — speak `Refusal.Message`, emit `BlockTiming`." The listener gets a short spoken notice ("Image at line 7 has no description; skipping.") and a block-level sync point back to source. Anything else — silence, a tone, dropping the block — would violate the rule's core promise: refusal is *data*, surfaced through the same channel as voiced content.

Treating empty `Refusal.Message` as an error rather than data is the contrapositive: the planner's job is to always populate `Refusal.Message`; an empty one is a contract violation, not a refusal in itself.

## Rejected alternatives

- **Pre-canned refusal earcon** — would require shipping audio assets and a `SegmentKindEarcon` renderer path. Out of scope phase one. Also: a generic earcon does not tell the listener *why* the block was skipped, defeating the purpose.
- **Skip refused blocks entirely** — flatly violates the honesty rule.
- **Render refusals through a separate Refusal-only renderer** — duplication for no clear gain. The Kokoro path is already engine-neutral via the `Renderer` interface.
