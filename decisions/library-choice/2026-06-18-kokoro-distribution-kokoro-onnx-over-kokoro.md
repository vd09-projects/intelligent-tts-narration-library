# Kokoro distribution — kokoro-onnx over kokoro

| Field    | Value            |
|----------|------------------|
| Date     | 2026-06-18       |
| Status   | accepted         |
| Category | library-choice   |
| Tags     | tts, rendering, kokoro, onnx, dependency, phase-one |

## Context

Phase one of the intelligent narration library needs a Kokoro-82M TTS runtime
that the Go renderer can drive as a subprocess (CGo `sherpa-onnx-go` in-process
binding is deferred). Two Python packages distribute the model:

- `kokoro` 0.9.4 — the upstream PyTorch reference implementation, bundled with
  the full torch stack.
- `kokoro-onnx` 0.5.0 — an ONNX-runtime wrapper around the same model weights,
  designed for lightweight inference on CPU and Apple Silicon.

The project is local-only, hobby-scale, runs on an Apple Silicon Mac, and must
respect the honesty + license invariants in `CLAUDE.md` (no GPL linkage; weights
distributed under their declared licence).

## Options considered

### Option A: `kokoro-onnx` 0.5.0 (ONNX runtime via subprocess)
- **Pros**:
  - Install size ≈ 150 MB (kokoro-onnx + onnxruntime + numpy + phonemizer-fork).
  - Explicit Apple-Silicon optimization path documented by the package.
  - Active maintenance; releases keep pace with Kokoro-82M weight updates.
  - Package is MIT; weights remain Apache-2.0 (`hexgrad/Kokoro-82M`).
  - Clean Python API (`Kokoro(model_path, voices_path).create(text, voice=...)`).
- **Cons**:
  - Introduces a Python venv to the build (`.venv/` per machine).
  - Subprocess hop costs a few hundred ms per call vs an in-process binding.

### Option B: `kokoro` 0.9.4 (PyTorch reference)
- **Pros**:
  - Closest to upstream model authors; any new voices land here first.
- **Cons**:
  - Install size > 2 GB once torch + its CUDA/Metal extras land.
  - No explicit Apple-Silicon perf story; runs as generic torch.
  - Release cadence is slow; 0.9.4 has lagged.
  - Pulls in transitive dependencies we will never use.

### Option C: Precompiled Kokoro-82M binary
- **Pros**:
  - No Python at runtime.
- **Cons**:
  - No maintained binary distribution exists at the time of writing.
  - Would force the project to publish and sign its own builds — outside the
    hobby-scale charter.

## Decision

Use `kokoro-onnx` 0.5.0 as the phase-one TTS runtime. Drive it from Go via a
subprocess wrapper at `scripts/kokoro` that activates a project-local venv
(`.venv/`) and dispatches to `scripts/kokoro_runner.py`. The runner loads
`models/kokoro-v1.0.onnx` plus `models/voices-v1.0.bin` (both downloaded from
the kokoro-onnx model-files-v1.0 GitHub release) and emits 24 kHz mono PCM s16le
WAV to stdout — native rate, no resampling.

Phase-one voice pair: `af_bella` (female default) and `am_michael` (male).

The chosen package is MIT-licensed; the model weights bundled with the
referenced release are Apache-2.0. Both are compatible with the project's
no-GPL-linkage rule, and neither pulls in Piper's GPL code.

## Consequences

- Phase-one renderer is subprocess-only. CGo `sherpa-onnx-go` in-process
  binding remains a deferred upgrade path, taken when latency or packaging
  pain forces the switch.
- `.venv/` and `models/` are gitignored. Every machine runs the install steps
  in `render/sherpa/README.md` once.
- The wrapper refuses to start when the venv or model files are missing
  (exit codes 2 / 3 with distinct stderr fingerprints), so the failure mode is
  loud rather than silent.
- If `kokoro-onnx` ever returns a sample rate other than 24 000 Hz, the runner
  exits 4 rather than resampling — protecting the "no resampling" invariant.
- Any future Piper VITS voice support must go through Apache-2.0
  `sherpa-onnx-go` or run as a separate Piper process. Direct linkage with
  `piper1-gpl` remains forbidden.

## Related decisions

<!-- None yet — first entry in the journal. -->

## Experiments

Install + invocation verified on Apple Silicon (`darwin/arm64`) on 2026-06-18:

- `./scripts/kokoro --voice af_bella --text "Phase one verification of bella."`
  produced `/tmp/kokoro-af-bella.wav` (92 KB, 1.92 s).
- `./scripts/kokoro --voice am_michael --text "Phase one verification of michael."`
  produced `/tmp/kokoro-am-michael.wav` (112 KB, 2.33 s).
- `ffprobe` on both files reported
  `Stream #0:0: Audio: pcm_s16le ([1][0][0][0] / 0x0001), 24000 Hz, 1 channels, s16, 384 kb/s`,
  satisfying the AC.

Failure probes also captured (missing wrapper / bad voice / missing model file
/ empty text); see `render/sherpa/README.md` for the exit-code + stderr table.

## Revisit trigger

Reconsider if any of the following becomes true:

- Subprocess launch latency makes the renderer's end-to-end budget unmanageable
  (then evaluate `sherpa-onnx-go` in-process binding).
- `kokoro-onnx` releases stall or the package falls behind Kokoro-82M weight
  updates by more than two minor versions.
- A maintained, signed precompiled Kokoro-82M binary appears and would let the
  project drop the Python venv entirely.
- The project expands beyond hobby scope and a Python runtime in the install
  story becomes a real friction point.
