# RVC phase-one rejects non-zero pitch with a clean ERR (no transpose ships); the request line's index_rate is authoritative

| Field    | Value            |
|----------|------------------|
| Date     | 2026-07-22       |
| Status   | accepted         |
| Category | tradeoff         |
| Tags     | rvc, voice-conversion, pitch, transpose, index-rate, phase-one-scope, reject-not-ignore, honesty-rule, wire-contract, issue-144, issue-145 |

## Context

Two of the worker's five request tokens (`<pitch>`, `<index_rate>`) had unresolved semantics at plan time. Both phase-one voices (`cool-jahns` at index_rate 0.75, `confident-neal` at 0.5) run at pitch 0; no semitone-transpose path was piloted or tested. And `index_rate` exists in two places — a per-voice value #145 might bake in, and the value on the request line — so "which wins" (OQ#1) had to be settled before #145 could bind. Shipping an untested transpose path, or silently ignoring a non-zero pitch, would both violate the honesty rule (fabricating a capability / silently doing something other than asked).

## Options considered

### Option A: ship a transpose path so non-zero pitch works
- **Cons**: untested DSP shipped under time pressure; the pilot never exercised transpose; a wrong pitch shift is a confident mis-render. Out of scope for #144.

### Option B: accept non-zero pitch but silently clamp/ignore it to 0
- **Cons**: the caller asked for a transpose and got none with no signal — a silent lie. Violates reject-not-fabricate.

### Option C: reject non-zero pitch with a clean single-line ERR; make the line's index_rate authoritative — CHOSEN
- **Pros**: no untested path ships; the refusal is explicit and machine-parseable (`ERR bad-args pitch must be 0 in phase one`); the loop survives; #145 has one unambiguous rule for each token.
- **Cons**: #145 cannot request a transpose until a future ticket adds the path (as a trailing optional token).

## Decision

Phase one **rejects any non-zero `<pitch>`** with `ERR bad-args pitch must be 0 in phase one` — no transpose/semitone-shift DSP is shipped, and pitch is not silently ignored. The transpose path is a future trailing-token capability, engine-faithful-defaulted so a v1 caller stays valid. Separately, **the request line's `<index_rate>` is authoritative** (resolves OQ#1): the per-voice 0.75 / 0.5 values are merely what #145 is expected to pass on the line, not a baked-in worker constant that overrides it. `<index_rate>` is validated as a float in [0.0, 1.0]; out-of-range → `ERR bad-args`.

## Consequences

- #145 binds to: pitch must be 0 (non-zero is a `bad-args` refusal, not an error-exit and not a silent no-op), and whatever index_rate it puts on the line is the value used — the worker holds no overriding per-voice index_rate.
- Aligns with the project honesty rule at the subprocess edge: the worker refuses cleanly and keeps serving rather than fabricating a transpose or silently dropping the request.
- Adding transpose later must be additive (trailing optional token) to avoid breaking a v1-built #145.

## Related decisions

- [RVC worker stdin/stdout wire contract — closed ERR taxonomy + startup/runtime FATAL exit-code split](../architecture/2026-07-22-rvc-worker-wire-contract-err-taxonomy-exit-codes.md) — this decision fixes the semantic scope of two of that contract's five request tokens.
- [Torch-free ONNX RVC via an ephemeral per-job worker, wrapped as a render decorator](../architecture/2026-07-22-torch-free-onnx-rvc-ephemeral-worker.md) — the parent decision; #145 (the render decorator) is the consumer that binds to these token semantics.

## Revisit trigger

Reconsider when a use case needs pitch/semitone transpose — add it as a trailing optional token with an engine-faithful default and a tested DSP path, keeping non-zero-pitch-rejection only for callers that omit the new token. Also revisit if a per-voice default index_rate is ever wanted server-side (would change "line is authoritative").
