# Kokoro Renderer — Phase One

Subprocess-based Kokoro-82M TTS for the intelligent narration library. CGo
in-process binding (`sherpa-onnx-go`) is deferred; phase one shells out to a
Python venv that runs [`kokoro-onnx`](https://github.com/thewh1teagle/kokoro-onnx).

The Go renderer (when wired) will invoke `scripts/kokoro` with `--voice` and
`--text` and read a 24 kHz mono PCM s16le WAV from stdout. This document
covers the local install, the wrapper contract, supported voices, verification,
failure modes, and licensing.

## Install

Phase one runtime is a project-local Python 3.12 venv with `kokoro-onnx` 0.5.0
plus the two model files distributed with that release.

```bash
# 1. Python 3.12 from Homebrew (one-time, idempotent)
brew install python@3.12

# 2. Create the project-local venv from that interpreter
/opt/homebrew/opt/python@3.12/bin/python3.12 -m venv .venv

# 3. Install kokoro-onnx into the venv
.venv/bin/pip install --upgrade pip
.venv/bin/pip install kokoro-onnx

# 4. Download the model weights + voice bundle into models/
mkdir -p models
curl -fL -o models/kokoro-v1.0.onnx \
  https://github.com/thewh1teagle/kokoro-onnx/releases/download/model-files-v1.0/kokoro-v1.0.onnx
curl -fL -o models/voices-v1.0.bin \
  https://github.com/thewh1teagle/kokoro-onnx/releases/download/model-files-v1.0/voices-v1.0.bin
```

Both `.venv/` and `models/` are gitignored — every machine runs these steps
once. The wrapper script refuses to start if either is missing.

## Invocation

```bash
./scripts/kokoro --voice af_bella  --text "Hello, world." > out-bella.wav
./scripts/kokoro --voice am_michael --text "Hello, world." > out-michael.wav
```

Contract:

- `--voice` selects one of the supported voice ids (see below). Required.
- `--text`  is a UTF-8 string; must be non-empty. Required.
- WAV bytes go to **stdout**. Redirect or pipe; do not expect a file on disk.
- Any diagnostic output goes to **stderr**.
- Exit `0` on success, non-zero on any failure (codes documented under
  **Failure modes**).

The wrapper activates the project-local `.venv` before exec; no global state
required, no shell init files read. Go's `os/exec` calls it the same way the
shell does.

## Voices

Phase one wires two Kokoro-82M voices:

| Voice id     | Gender | Role in phase one    | Sample rate |
| ------------ | ------ | -------------------- | ----------- |
| `af_bella`   | female | Default voice        | 24 000 Hz   |
| `am_michael` | male   | Alternate voice      | 24 000 Hz   |

Default selection is `af_bella` when the MCP `speak` tool's `gender` arg is
unset or `female`; `am_michael` is selected when `gender=male`. Both voices
are native 24 kHz mono — the wrapper performs no resampling.

## Verification

Generate one sample per voice and probe with `ffprobe`:

```bash
./scripts/kokoro --voice af_bella  --text "verify bella"   > /tmp/kokoro-af-bella.wav
./scripts/kokoro --voice am_michael --text "verify michael" > /tmp/kokoro-am-michael.wav

ffprobe /tmp/kokoro-af-bella.wav   2>&1 | grep "Stream #0:0"
ffprobe /tmp/kokoro-am-michael.wav 2>&1 | grep "Stream #0:0"
```

Both files must show:

```
Stream #0:0: Audio: pcm_s16le ([1][0][0][0] / 0x0001), 24000 Hz, 1 channels, s16, 384 kb/s
```

If the codec, sample rate, or channel count differs, the invariant
("24 kHz mono PCM/WAV native, no resampling") is broken — do not paper over it
in the wrapper, fix the source.

## Failure modes

Every failure mode keeps stdout silent (no half-written WAV bytes) and emits a
distinct stderr fingerprint with a unique exit code so callers can branch.

| Probe              | Trigger                                | Exit | Stderr fingerprint                                        |
| ------------------ | -------------------------------------- | ---- | --------------------------------------------------------- |
| Missing wrapper    | `scripts/kokoro` not on disk           | 127  | shell: `no such file or directory: ./scripts/kokoro`      |
| Bad voice          | `--voice zz_invalid`                   | 2    | `kokoro: unsupported voice '...'. Supported: ...`         |
| Missing model file | `models/kokoro-v1.0.onnx` absent       | 3    | `kokoro: model files missing. Expected: ...`              |
| Empty text         | `--text ""`                            | 2    | `kokoro: --text must be non-empty.`                       |

A missing venv (e.g. step 2 of install skipped) exits `2` with
`kokoro: venv missing at ... — run install steps in render/sherpa/README.md`.
If `kokoro-onnx` ever returns an unexpected sample rate, the runner exits `4`
rather than resampling silently.

## License notes

- `kokoro-onnx` (the Python package, the wrapper API): **MIT**.
- Kokoro-82M (the model weights distributed via `kokoro-v1.0.onnx` and
  `voices-v1.0.bin`): **Apache-2.0** (`hexgrad/Kokoro-82M`).
- This wrapper (`scripts/kokoro`, `scripts/kokoro_runner.py`): project license.

The Apache-2.0 licence applies to the weights and the runtime use thereof.
Piper (`OHF-Voice/piper1-gpl`) is **not** used — its GPL linkage would taint
the library. If a future renderer adopts Piper VITS voice models, route them
through Apache-2.0 `sherpa-onnx-go` or shell out to a separate Piper process;
do not link directly.
