# Debug Report: faiss segfault in RVC inference (SOLVED)

**Date:** 2026-07-21 · **Machine:** M1 Pro, macOS 26, Applio venv (torch 2.11, faiss-cpu 1.14.3, numpy 2.4.6)

## Symptom
RVC voice conversion **with the `.index` on** (`--index_rate > 0`) crashed with **exit 139 (segfault)**. Worked around it for days by running `--index_rate 0.0` (index disabled = lower timbre fidelity).

## Wrong first hypothesis
"faiss-cpu 1.14.3 + numpy 2.x ABI mismatch." Would have meant a risky numpy downgrade (torch 2.11 needs numpy 2.x → could break inference). **Did not touch versions until proven.**

## Diagnosis (3 steps)
| # | Test | Result | Meaning |
|---|---|---|---|
| 1 | faiss `.search()` **in isolation** (load index, query — no torch) | ✅ exit 0, 190k vectors searched | faiss+numpy are **fine** alone → hypothesis wrong |
| 2 | Real inference, `--index_rate 0.5` | ❌ exit 139 | crash only in the **full pipeline** |
| 3 | Re-run with `python -X faulthandler` | segfault at `faiss swigfaiss.py:7126 search` ← `pipeline.py:380 _retrieve_speaker_embeddings`; **196 native ext modules loaded** (torch + faiss + scipy + numba…) | crash is faiss search **only when torch is co-loaded** |

### Commands run (reproducible)
```bash
cd /Users/vikrantdhawan/repos/TTS/Applio

# versions
.venv/bin/python -c "import faiss, numpy, torch; print('faiss',faiss.__version__,'numpy',numpy.__version__,'torch',torch.__version__)"

# STEP 1 — faiss search in ISOLATION (no torch) -> worked, exit 0
.venv/bin/python -c "
import faiss, numpy as np
idx = faiss.read_index('logs/cool_jahns/cool_jahns.index')
print('ntotal', idx.ntotal, 'dim', idx.d)
D,I = idx.search(np.random.rand(1, idx.d).astype('float32'), 8)
print('SEARCH OK', I.shape)"

# STEP 2 — real inference WITH index -> segfault, exit 139
export PYTORCH_ENABLE_MPS_FALLBACK=1; export PYTORCH_MPS_HIGH_WATERMARK_RATIO=0.0
.venv/bin/python core.py infer \
  --input_path SRC.wav --output_path OUT.wav \
  --pth_path   .../cool-jahns/Cool_Jahns.pth \
  --index_path .../cool-jahns/Cool_Jahns.index \
  --pitch 0 --f0_method rmvpe --index_rate 0.5
echo "exit $?"    # -> 139

# STEP 3 — faulthandler to locate the crash
.venv/bin/python -X faulthandler core.py infer ... --index_rate 0.5 > /tmp/idx.log 2>&1
grep -nE "Segmentation|faiss|pipeline.py|Extension modules" /tmp/idx.log
#   -> faiss swigfaiss.py:7126 search  <-  pipeline.py:380 _retrieve_speaker_embeddings
#   -> "Extension modules: ... torch._C ... faiss._swigfaiss (total: 196)"
```

## Root cause
**OpenMP runtime conflict.** faiss-cpu and torch each bundle their **own** OpenMP library (`libomp`). On macOS, loading both into one process and then running faiss's **multi-threaded** search corrupts state → segfault. Isolation worked because torch wasn't loaded; the pipeline crashes because it loads torch *and* faiss.

## Fix + verification (commands)
```bash
cd /Users/vikrantdhawan/repos/TTS/Applio
export PYTORCH_ENABLE_MPS_FALLBACK=1; export PYTORCH_MPS_HIGH_WATERMARK_RATIO=0.0
export KMP_DUPLICATE_LIB_OK=TRUE    # tolerate the duplicate OpenMP runtime  <-- THE FIX
export OMP_NUM_THREADS=1            # single-thread faiss -> avoids the crashing threaded path

# same command as STEP 2, now with the fix -> exit 0, valid WAV
.venv/bin/python core.py infer \
  --input_path SRC.wav --output_path OUT.wav \
  --pth_path   .../cool-jahns/Cool_Jahns.pth \
  --index_path .../cool-jahns/Cool_Jahns.index \
  --pitch 0 --f0_method rmvpe --index_rate 0.75    # 0.5 and 0.75 both verified

# swept 0.0 / 0.5 / 0.75 on Cool Jahns, and 0.0 / 0.75 on Confident Neal -> all exit 0
```
- `--index_rate 0.5` and `0.75` → **exit 0**, valid WAV, on **both** voices (Cool Jahns + Confident Neal).
- No numpy/faiss/torch downgrade needed; inference unaffected.

## Takeaways
- **A segfault in library X co-loaded with library Y is often an OpenMP/native-runtime clash, not a version-ABI bug.** Isolate first (X alone) before repinning — it saved a torch-breaking numpy downgrade.
- Env vars baked into the convert path (`assets/rvc-models/rvc-convert.sh`) + the runbook (`docs/rvc-voice-creation-runbook.md`, gotcha 5), so it won't resurface.
