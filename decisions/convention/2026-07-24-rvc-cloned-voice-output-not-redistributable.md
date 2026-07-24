# RVC voice cloned from a real person without a license: its converted output is not publicly redistributable — swap the parity voice

| Field    | Value            |
|----------|------------------|
| Date     | 2026-07-24       |
| Status   | accepted         |
| Category | convention       |
| Tags     | rvc, license, voice-clone, fixtures, redistribution, honesty, d0, parity, cool-jahns, issue-151 |

## Context

Task #151 (RVC parity fixtures, single-voice; branch `feat/rvc-parity-fixtures-151`) needs hosted fixtures so a fresh clone can run `make rvc-parity` with no local setup. The natural parity voice was `cool-jahns`, which was cloned from ~59 minutes of Jeremy Jahns — a real, named person — WITHOUT any documented consent or license. There is no license / consent / model-card under `assets/rvc-models/`; the models live only in a private Hugging Face backup. The parity fixtures that would be hosted are derived from that voice's *output*: the `*_ref.wav` is converted output and the `*_logmel_target.npy` is a derivative of that converted output. `fixtures.sha256` was left empty because nothing could be legitimately hosted, which is the symptom this decision explains and prevents a future contributor from "fixing" by publishing the non-redistributable assets.

## Options considered

### Option A: Swap the parity voice to a licensed / self-trained voice (chosen)
- **Pros**: The converted output's public redistribution is clearable, so the fixture bundle can actually be hosted. Keeps the zero-setup property — a fresh clone runs `make rvc-parity` with no local regeneration step.
- **Cons**: Requires sourcing / training a voice whose redistribution is clear; the parity voice is then not one of the phase-one character voices.

### Option B: Documented regenerate-locally-only fresh-clone path (rejected as primary; kept as fallback)
- **Pros**: Avoids hosting any redistributable-restricted asset by never hosting converted output at all.
- **Cons**: Loses the zero-setup property — a fresh clone cannot run parity without first regenerating fixtures locally against private models. Kept only as a fallback, not the primary path.

## Decision

Establish a hard pre-publish gate (a **D0** gate): the voice-model license MUST be vetted BEFORE hosting any converted-output-derived asset. An RVC voice model cloned from a real, named person without documented consent/license may NOT have its CONVERTED OUTPUT publicly redistributed. Because the parity/test fixtures ARE converted-output-derived (`*_ref.wav` = converted output; `*_logmel_target.npy` = a derivative of it), fixtures cannot host `cool-jahns`-derived assets.

Chosen resolution: **swap the parity voice** to a licensed or self-trained voice whose converted-output public redistribution is clearable, then host that bundle — preserving the zero-setup fresh-clone experience. The regenerate-locally-only fresh-clone path is retained only as a fallback, not the primary, because it loses zero-setup.

## Consequences

- Governs ALL future RVC parity / redistribution / hosting choices: no converted-output-derived asset ships publicly until its source voice model's license clears.
- Prevents a contributor from "fixing" the empty `fixtures.sha256` by publishing non-redistributable converted output.
- The parity voice is decoupled from whether a given character voice (e.g. `cool-jahns`) is redistributable — parity uses a clearable voice; character voices can remain private-backup-only.
- Aligns with the honesty rule at the redistribution edge: refuse to host what cannot be cleared rather than quietly shipping it.

## Related decisions

- [Torch-free ONNX RVC via an ephemeral per-job worker](../architecture/2026-07-22-torch-free-onnx-rvc-ephemeral-worker.md) — the RVC integration whose voices this gate governs.
- [Voice-selection namespace: one unified named roster](../architecture/2026-07-23-unified-voice-roster-namespace.md) — the roster (`cool-jahns` / `confident-neal`) whose redistributability this gate constrains.
- [manifest.voice records the RVC character slug](../tradeoff/2026-07-23-rvc-manifest-voice-records-character-slug.md) — provenance sibling; honest-provenance theme.

## Revisit trigger

If a cloned voice gains a documented consent/license/model-card under `assets/rvc-models/`, or if a licensed/self-trained voice with clearable converted-output redistribution becomes the parity voice, revisit which voice backs the hosted fixtures.
