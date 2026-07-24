# `make rvc-parity` is a single-voice FLOW gate, not a per-voice correctness oracle

| Field    | Value            |
|----------|------------------|
| Date     | 2026-07-24       |
| Status   | accepted         |
| Category | convention       |
| Tags     | rvc, parity, testing, flow-gate, single-voice, honesty, issue-151 |

## Context

Task #151 builds the RVC parity fixtures and the `make rvc-parity` gate. The open question was scope: does the parity gate prove the conversion PIPELINE reproduces on a fresh clone, or does it try to prove every RVC voice converts correctly? If every future voice earned its own parity fixture, the fixture set and the gate's runtime grow with the roster, and the "what does green mean" contract blurs.

## Options considered

### Option A: Single-voice flow gate (chosen)
- **Pros**: Proves the whole conversion path reproduces byte-for-byte on a fresh clone using ONE voice that exercises source -> `rvc-convert.sh` torch reference -> ONNX worker -> log-mel compare. One fixture, one source of truth (`PARITY_VOICES`), stable runtime. Future voices do NOT each earn a parity fixture.
- **Cons**: A voice-specific conversion regression that does NOT affect the parity voice will not trip the gate.

### Option B: Per-voice parity oracle (rejected)
- **Pros**: Would catch a regression isolated to any single voice.
- **Cons**: Fixture set and gate runtime grow with every added voice; conflates "the pipeline reproduces" with "each voice is correct"; heavy hosting/maintenance cost for marginal coverage over by-ear `/verify`.

## Decision

The RVC parity gate proves the conversion PIPELINE reproduces byte-for-byte on a fresh clone, using exactly ONE voice — `PARITY_VOICES`, a single source of truth — that exercises the whole path (source -> `rvc-convert.sh` torch reference -> ONNX worker -> log-mel compare). Other voices are documented-excluded via `EXCLUDED_PARITY_VOICES`, guarded by a disjoint/coverage meta-assert cross-checked against an INDEPENDENT roster, plus a negative test — so no voice can be silently absent from both sets and the parity matrix cannot silently re-widen. Future voices do NOT each earn a parity fixture.

## Consequences

- Accepted tradeoff: a voice-specific conversion regression that does NOT affect the parity voice will NOT trip the gate. It surfaces instead in the by-ear `/verify` of that voice. This is a deliberate, documented boundary (the honesty rule — name the blind spot rather than imply full coverage), not an accidental gap.
- Sets the scope contract for every future RVC voice and fixes the shape of the parity test: one flow-exercising voice, an excluded set, a disjoint/coverage meta-assert, and a negative re-widen guard.
- Keeps fixture hosting and gate runtime constant as the voice roster grows.

## Related decisions

- [RVC cloned-voice output not publicly redistributable — swap the parity voice](2026-07-24-rvc-cloned-voice-output-not-redistributable.md) — determines WHICH voice is the parity voice; this decision fixes how many.
- [Torch-free ONNX RVC via an ephemeral per-job worker](../architecture/2026-07-22-torch-free-onnx-rvc-ephemeral-worker.md) — the conversion pipeline the gate reproduces.
- [RVC index-blend reconstructs big_npy in-worker](../algorithm/2026-07-22-rvc-index-blend-reconstruct-big-npy-in-worker.md) — parity judged by full-pipeline log-mel correlation, the same measure this gate applies.

## Revisit trigger

If a voice-specific conversion regression escaping the parity voice ever ships a broken render that by-ear `/verify` also misses, reconsider whether the single-voice flow-gate scope needs per-voice coverage.
