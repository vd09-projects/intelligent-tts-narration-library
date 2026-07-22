# RVC parity gate fixtures (issue #144)

The always-on full-pipeline log-mel gate compares against these assets
(review blocker B2 — the headline gate can never env-skip because its target
must be present when the gate runs).

**These fixtures are LOCAL / generated — they are NOT committed to git.** The
source clip (`source.wav`), the torch-reference outputs (`*_ref.wav`), and the
pinned log-mel targets (`*.npy`) are all gitignored: the repo forbids committed
`.wav` audio binaries and the local `source.wav` is personal audio. Generate
them on your own machine with `make rvc-parity-gen` (see "Regenerating" below).

Fresh-clone reproducibility of the full-pipeline gate — a public-download
fixture flow so anyone cloning the repo can run the gate without the local
Applio torch path — is **DEFERRED to issue #151**.

| file | what | how produced |
|---|---|---|
| `source.wav` | the fixed source clip fed to BOTH the torch reference and the ONNX worker | placed once locally (a short spoken clip, e.g. a kokoro render or `assets/huginn-samples/*.wav`); gitignored |
| `cool-jahns_ref.wav` | torch-reference output for cool-jahns | `assets/rvc-models/rvc-convert.sh cool-jahns source.wav …` (Applio torch venv) |
| `confident-neal_ref.wav` | torch-reference output for confident-neal | same, confident-neal |
| `cool-jahns_logmel_target.npy` | pinned log-mel of `cool-jahns_ref.wav` — the actual assertion target | `gen_targets.py` |
| `confident-neal_logmel_target.npy` | pinned log-mel of `confident-neal_ref.wav` | `gen_targets.py` |

## Regenerating

These fixtures are generated locally and gitignored — there is nothing to
`git add`; they simply need to exist on your machine before `make rvc-parity`.

```
# once, from the repo root, with the Applio torch venv available:
make rvc-parity-gen SOURCE=<a short clip.wav>
# (equivalently: .venv-rvc/bin/python tests/rvc_parity/gen_targets.py --source <clip.wav>)
```

The log-mel extraction params are PINNED in `tests/rvc_parity/logmel.py`. If you
change them you MUST regenerate every `*_logmel_target.npy` here, or the gate
compares two different transforms.

> **PENDING-REAL-ASSETS (build note, issue #144):** these binaries are generated
> from the Applio torch path (`rvc-convert.sh`, ~10 s/voice) and are NOT produced
> by the worker build itself. Until `gen_targets.py` has been run locally, `make
> rvc-parity`'s full-pipeline test **fails (does not skip)** with a clear
> "missing target" message — which is the designed fail-not-skip behavior, not a
> bug. (Because the fixtures are local/gitignored, a fresh clone cannot yet run
> this gate; the public-download fixture flow is deferred to issue #151.)
