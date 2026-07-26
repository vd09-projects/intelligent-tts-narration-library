# RVC pipeline is objectively faithful to torch, but Kokoro-synthetic source ≠ recognizable target voice by ear

| Field    | Value            |
|----------|------------------|
| Date     | 2026-07-25       |
| Status   | accepted         |
| Category | tradeoff         |
| Tags     | rvc, voice, by-ear, parity, kokoro, source-mismatch, index-rate, cool-jahns, issue-147 |

## Context

#147 (RVC P4) is the verify-by-ear + docs close-out of the RVC integration. During
verification the end-to-end CLI render `narrate --voice cool-jahns` produced a 40 kHz
file whose objective signals all passed (manifest `voice: cool-jahns`, 40 kHz mono
s16le, 191 s non-silent) — but by ear it **did not sound like Cool Jahns at all**.

The question was where the break is: Go decorator, torch-free ONNX worker, or upstream.

## Options considered

Diagnosis was empirical, not a design vote — but the candidate root causes were:

### A: Go `render/rvc` decorator distorts the voice
- Ruled out: `frameAlignWAV` only trims/pads sub-millisecond (<80 bytes, <1 ms); it
  drives the same worker with the same index_rate. Cannot alter timbre.

### B: torch-free ONNX worker (#144) diverges from the Applio torch path
- Ruled out objectively: full-pipeline log-mel **corr 0.9824** (≥0.98 floor); per-stage
  ONNX-vs-torch-refio **net_g 0.999993, contentvec 1.0, rmvpe 1.0**. On the exact test
  clip, worker-vs-torch log-mel **corr 0.9824**, median f0 **110 Hz (worker) vs 109 Hz
  (torch)** — same voice.

### C: upstream — the `.pth` on a Kokoro-**synthetic** source doesn't transfer to the target
- Confirmed by ear: the Applio **torch** reference (the training-approved path) on a
  Kokoro `am_michael` source **also** does not sound like Cool Jahns. Every layer we
  ship reproduces that reference — so the reference itself is the ceiling.

## Decision

The RVC integration **code** (worker + decorator + CLI/MCP/server wiring) is verified
**faithful** and ships as-is. The by-ear character failure is an **upstream** model/source
issue, not a #147 code bug:

- cool-jahns was trained on ~59 min of **real** Jeremy Jahns speech; a Kokoro-synthesized
  `am_michael` source is out-of-distribution for it, so the timbre transfer is weak.
- **index_rate stays at 0.75.** A by-ear sweep at 0.75 / 0.90 / 1.00 did not recover the
  character (0.75 preferred, others no closer). So the fix is NOT a roster index_rate bump;
  the 0.75 default in `render/rvc/voice.go` is unchanged.

The character-quality fix (retrain/validate on synthetic sources, try a real-speech
source, or a different source Kokoro voice / model) is spun out to a follow-up ticket.

## Consequences

- #147's code + docs deliverables are complete and correct; its **by-ear character
  sign-off** is deferred to the follow-up (the acceptance criterion "hear it in the Cool
  Jahns voice" is blocked upstream, not by this work).
- The always-on `make rvc-parity` gate keeps proving pipeline fidelity (0.98) regardless —
  it is a flow gate, not a per-voice character oracle (see the sibling decision).
- Anyone re-hitting "RVC voice sounds wrong" should first check `make rvc-parity` (is the
  pipeline still faithful?) before touching worker/decorator code — the likely cause is
  source/model, not the Go/ONNX path.

## Related decisions

- [make rvc-parity is a single-voice FLOW gate, not a per-voice correctness oracle](../convention/2026-07-24-rvc-parity-single-voice-flow-gate.md) — the gate proves pipeline fidelity, and explicitly does NOT catch per-voice character regressions; this finding is exactly that boundary in action.
- [RVC cloned-voice output not redistributable](../convention/2026-07-24-rvc-cloned-voice-output-not-redistributable.md) — a synthetic-source-friendly replacement voice would also help clear the D0 redistribution gate.

## Experiments

- **Source clip**: `narrate --voice am-michael --level 3` on a short doc → 7.76 s, 24 kHz
  Kokoro `am_michael` WAV (the RVC source for cool-jahns).
- **Torch-free worker** (`make rvc-convert VOICE=cool-jahns`, index_rate 0.75 / 0.90 /
  1.00) vs **Applio torch** (`assets/rvc-models/rvc-convert.sh cool-jahns`, index_rate 0.75).
- **Objective** (`tests/rvc_parity/logmel.py`): worker-vs-torch log-mel **corr 0.9824**;
  median f0 worker **110 Hz** / torch **109 Hz** (README characterizes cool-jahns ~134 Hz).
- **Full parity gate**: `[fullpipe] cool-jahns log-mel corr=0.9824 OK`; per-stage
  `net_g 0.999993 / contentvec 1.0 / rmvpe 1.0`.
- **By ear** (user): torch reference + all three worker index_rates all sound the same and
  **none** sound like Cool Jahns → confirms the ceiling is upstream of the shipped code.

## Revisit trigger

When a redistributable / synthetic-source-validated RVC voice replaces (or augments)
cool-jahns — re-run the by-ear verify; if that voice is recognizable through the Kokoro→RVC
path, close the #147 by-ear sign-off and this tradeoff no longer binds.
