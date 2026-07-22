# RVC parity gate fixtures (issue #144)

Committed assets the always-on full-pipeline log-mel gate compares against
(review blocker B2 — the headline gate can never env-skip because its target
lives in-repo).

| file | what | how produced |
|---|---|---|
| `source.wav` | the fixed source clip fed to BOTH the torch reference and the ONNX worker | committed once (a short spoken clip, e.g. a kokoro render or `assets/huginn-samples/*.wav`) |
| `cool-jahns_ref.wav` | torch-reference output for cool-jahns | `assets/rvc-models/rvc-convert.sh cool-jahns source.wav …` (Applio torch venv) |
| `confident-neal_ref.wav` | torch-reference output for confident-neal | same, confident-neal |
| `cool-jahns_logmel_target.npy` | pinned log-mel of `cool-jahns_ref.wav` — the actual assertion target | `gen_targets.py` |
| `confident-neal_logmel_target.npy` | pinned log-mel of `confident-neal_ref.wav` | `gen_targets.py` |

## Regenerating

```
# once, from the repo root, with the Applio torch venv available:
.venv-rvc/bin/python tests/rvc_parity/gen_targets.py --source <a short clip.wav>
git add tests/rvc_parity/fixtures/
```

The log-mel extraction params are PINNED in `tests/rvc_parity/logmel.py`. If you
change them you MUST regenerate every `*_logmel_target.npy` here, or the gate
compares two different transforms.

> **PENDING-REAL-ASSETS (build note, issue #144):** these binaries are generated
> from the Applio torch path (`rvc-convert.sh`, ~10 s/voice) and are NOT produced
> by the worker build itself. Until `gen_targets.py` has been run and the outputs
> committed, `make rvc-parity`'s full-pipeline test **fails (does not skip)** with
> a clear "missing committed target" message — which is the designed fail-not-skip
> behavior, not a bug.
