# RVC Voice Creation Runbook (local, Apple Silicon M1) — reproduce a `.pth` voice from audio

> **What this produces:** a custom RVC voice model (`.pth` + `.index`) trained on a target speaker's audio,
> usable to convert **any** Kokoro (or other) TTS output into that voice. **Fully local, free, no CUDA.**
>
> **Proven twice on this machine** (M1 Pro, 16GB, macOS 26, no GPU): produced **Confident Neal** (female-range)
> and **Cool Jahns** (male). This doc is the exact, battle-tested procedure — every gotcha below was actually hit
> and fixed. Follow it verbatim; do **not** re-research.

---

## 0. The concept (why RVC, not TTS)
Off-the-shelf local TTS (Kokoro/Chatterbox/XTTS/Indic) sounded machine-like and failed the naturalness bar.
**RVC (Retrieval-based Voice Conversion) won:** it takes a *source* audio and repaints its timbre as a *target* voice
you trained. Pipeline:
```
Kokoro (writes the words + prosody)  →  RVC (repaints into the trained voice)  →  final audio
```
Kokoro controls WHAT is said + pacing; the RVC `.pth` controls WHO it sounds like. One narration → any character.

---

## 1. Environment (already set up — verify, don't reinstall)
- **Applio** (RVC framework, IAHispano/Applio) installed at: **`/Users/vikrantdhawan/repos/TTS/Applio`**
- **Python/venv:** `Applio/.venv/bin/python` (Python 3.12, provisioned by `uv` — ignores system Python 3.14)
- **Runs on CPU.** MPS (Apple GPU) **segfaults** during RVC inference/training — everything uses CPU. Slow but stable.
- **Always export these before any Applio command:**
  ```bash
  export PYTORCH_ENABLE_MPS_FALLBACK=1
  export PYTORCH_MPS_HIGH_WATERMARK_RATIO=0.0
  # REQUIRED for inference WITH the index on (fixes the faiss/torch OpenMP segfault — see gotcha 5):
  export KMP_DUPLICATE_LIB_OK=TRUE
  export OMP_NUM_THREADS=1
  ```
- **Kokoro** (for making source clips) is the repo venv: `intelligent-tts-narration-library-manual/.venv/bin/python3` + `models/kokoro-v1.0.onnx`.
- Prerequisite models (rmvpe, contentvec, pretrained f0G40k/f0D40k) are already downloaded under `Applio/rvc/models/`. If missing: `cd Applio && .venv/bin/python core.py prerequisites`.

**Fixed constants used for both voices (keep these):** `sample_rate 40000` · `f0_method rmvpe` · `embedder contentvec` · `batch_size 4` · `vocoder HiFi-GAN` · `version v2`.

---

## 2. Prepare the dataset  ⚠️ 3 gotchas here
- **Audio:** clean single-speaker speech, **WAV mono** (48kHz or any — preprocess resamples to 40k). **~10 min floor, 30–60 min ideal.**
- **Clean it:** remove music/sponsors/other-speakers. (For Cool Jahns, `DATASET.md` documented cutting sponsor reads + end-card music — that prep matters; BGM hurts timbre.)
- ⚠️ **GOTCHA 1 — FLATTEN into ONE directory.** Applio treats sub-folders of `dataset_path` as **integer speaker IDs**. Named subdirs (e.g. `01-scifi/`, `02-substance/`) → error `Speaker ID folder is expected to be integer` → **0 files found**. Put all `.wav`s directly in one flat dir (a non-`.wav` like `DATASET.md` is ignored — fine).
  ```bash
  # if clips are in subdirs, flatten (prefix names to avoid collisions):
  FLAT=/tmp/mydataset; mkdir -p "$FLAT"
  for f in /path/to/clips/*/*.wav; do sub=$(basename "$(dirname "$f")"); cp "$f" "$FLAT/${sub}__$(basename "$f")"; done
  ```
- Check durations with Python (the repo's `ffprobe` has a broken `libvpx` dylib — use `wave` instead):
  ```bash
  python3 -c "import wave,glob; t=sum(wave.open(f).getnframes()/wave.open(f).getframerate() for f in glob.glob('DIR/*.wav')); print(round(t/60,1),'min')"
  ```

---

## 3. Preprocess  ⚠️ GOTCHA 2
```bash
cd /Users/vikrantdhawan/repos/TTS/Applio
export PYTORCH_ENABLE_MPS_FALLBACK=1; export PYTORCH_MPS_HIGH_WATERMARK_RATIO=0.0
.venv/bin/python core.py preprocess \
  --model_name MYVOICE \
  --dataset_path /path/to/FLAT_dataset \
  --sample_rate 40000 \
  --cpu_cores 8 \
  --cut_preprocess Automatic \
  --noise_reduction False        # True + --noise_reduction_strength 0.5 if BGM present
```
⚠️ **`--cut_preprocess` is REQUIRED** (`Skip|Simple|Automatic`) — omitting it errors. Use `Automatic` (silence-slices into 3–15s clips). Fast (~10–15s). Verify: `ls logs/MYVOICE/sliced_audios | wc -l` (32 min → ~720 clips; 59 min → ~1370).

---

## 4. Extract features  ⚠️ GOTCHA 3
```bash
.venv/bin/python core.py extract \
  --model_name MYVOICE --sample_rate 40000 \
  --f0_method rmvpe --embedder_model contentvec \
  --include_mutes 2 --cpu_cores 8
```
⚠️ **`--include_mutes` is REQUIRED** (0–10; use `2`) — omitting it errors. Produces `logs/MYVOICE/{f0,f0_voiced,extracted}` (one per clip). A few minutes on CPU.

---

## 5. Train  ⚠️ GOTCHA 4 + epoch guidance
```bash
export PYTORCH_ENABLE_MPS_FALLBACK=1; export PYTORCH_MPS_HIGH_WATERMARK_RATIO=0.0
# assets/config.json MUST exist (see gotcha) — create once if missing:
[ -f assets/config.json ] || cp assets/config_template.json assets/config.json

nohup .venv/bin/python core.py train \
  --model_name MYVOICE --sample_rate 40000 \
  --total_epoch 60 --batch_size 4 \
  --save_every_epoch 5 --save_every_weights True --save_only_latest False \
  --pretrained True --overtraining_detector True --overtraining_threshold 50 \
  > /tmp/MYVOICE_train.log 2>&1 &
```
⚠️ **GOTCHA 4 — `assets/config.json` missing on fresh install** → auto-export of the inference `.pth` fails every epoch (`No such file or directory: assets/config.json`). **Fix once:** `cp assets/config_template.json assets/config.json` (already done on this machine). With it present, training auto-exports `logs/MYVOICE/MYVOICE_<epoch>e_<step>s.pth` + a `*_best_epoch.pth`.

**Timing (CPU):** ~**5 min/epoch** for 32 min data, ~**9–11 min/epoch** for 59 min data. So full runs are **overnight (8–11 hrs)**. Run it detached (`nohup ... &`) and check back.

**⭐ EPOCH COUNT — the important decision (not a fixed number):**
- The best model is where **validation loss bottoms** (`lowest_value` in the log), then STOP — past it is overtraining (validation flat, training loss keeps dropping = memorizing → artifacts on unseen words).
- **More data → bottoms EARLIER.** Confident Neal (32 min) bottomed @**epoch 69**. Cool Jahns (59 min) bottomed @**epoch 25**.
- Rule of thumb: ~1 hr data → expect the bottom around **epoch 25–40**; less data → later. Set `--total_epoch` generously (60–100), keep `--save_every_epoch 5`, then **pick the checkpoint at/near the loss bottom** (step 6). Watch the log:
  ```bash
  grep -aoE "epoch=[0-9]+ .*lowest_value=[0-9.]+ \(epoch [0-9]+" /tmp/MYVOICE_train.log | tail
  ```

---

## 6. Pick the best epoch + get the inference `.pth`
Find the loss bottom, then use that checkpoint. Applio saves periodic inference weights (`MYVOICE_25e_8625s.pth`, etc.) **if** auto-export worked (gotcha 4 fixed).

**If auto-export FAILED (only `G_*.pth`/`D_*.pth` training checkpoints exist)** — extract the inference weight manually from the generator checkpoint. `G_<step>.pth` where `step = epoch × steps_per_epoch` (steps_per_epoch = clips/batch ≈ shown in the tqdm `/NNN`). Script:
```python
# .venv/bin/python this, cwd = Applio, assets/config.json must exist
import torch, json, os, sys
os.chdir("/Users/vikrantdhawan/repos/TTS/Applio"); sys.path.insert(0, os.getcwd())
from rvc.train.utils import HParams
from rvc.train.process.extract_model import extract_model
hps  = HParams(**json.load(open("logs/MYVOICE/config.json")))
ckpt = torch.load("logs/MYVOICE/G_<STEP>.pth", map_location="cpu", weights_only=False)["model"]
extract_model(ckpt=ckpt, sr=40000, name="MYVOICE", model_path="logs/MYVOICE/MYVOICE.pth",
              epoch=<EPOCH>, step=<STEP>, hps=hps, overtrain_info="manual", vocoder="HiFi-GAN",
              pitch_guidance=True, version="v2")
```
Result: `logs/MYVOICE/MYVOICE.pth` (~55MB). Validate: `.venv/bin/python core.py model_information --pth_path logs/MYVOICE/MYVOICE.pth` and check 0 NaN/inf.

---

## 7. Name + back up (protect the 8-hr training!)  ⚠️ path gotcha
Set the display name in metadata + copy out of the ephemeral Applio dir into the repo.
```python
# .venv/bin/python, cwd = Applio. Write to logs dir first (direct write to the backup dir has failed).
import torch
c = torch.load("logs/MYVOICE/MYVOICE.pth", map_location="cpu", weights_only=False)
c["model_name"] = "Display Name"; c["author"] = "Display Name"
torch.save(c, "logs/MYVOICE/Display_Name.pth")
```
```bash
DEST=/Users/vikrantdhawan/repos/TTS/intelligent-tts-narration-library-manual/assets/rvc-models/<slug>
mkdir -p "$DEST"
cp logs/MYVOICE/Display_Name.pth "$DEST/"
cp logs/MYVOICE/MYVOICE.index    "$DEST/Display_Name.index"   # the .index is generated at TRAIN END
```
⚠️ **Extraction/torch.save can fail writing directly to the backup dir** — always write to `logs/MYVOICE/` first, then `cp`.

---

## 8. Use the voice (inference)  ⚠️ GOTCHA 5 (faiss) + pitch
```bash
# 1) make a source clip with Kokoro (repo venv):
cd /Users/vikrantdhawan/repos/TTS/intelligent-tts-narration-library-manual
./.venv/bin/python3 -c "
import numpy as np, wave; from kokoro_onnx import Kokoro
k=Kokoro('models/kokoro-v1.0.onnx','models/voices-v1.0.bin')
s,sr=k.create('Your narration text here.', voice='am_michael', speed=1.0, lang='en-us')  # am_michael male / af_heart female
p=(np.clip(s,-1,1)*32767).astype(np.int16)
w=wave.open('src.wav','wb'); w.setnchannels(1); w.setsampwidth(2); w.setframerate(sr); w.writeframes(p.tobytes())"

# 2) convert -> your voice (index ON):
cd /Users/vikrantdhawan/repos/TTS/Applio
export PYTORCH_ENABLE_MPS_FALLBACK=1; export PYTORCH_MPS_HIGH_WATERMARK_RATIO=0.0
export KMP_DUPLICATE_LIB_OK=TRUE; export OMP_NUM_THREADS=1     # <-- REQUIRED or faiss segfaults
.venv/bin/python core.py infer \
  --input_path /path/src.wav --output_path /path/out.wav \
  --pth_path   .../assets/rvc-models/<slug>/Display_Name.pth \
  --index_path .../assets/rvc-models/<slug>/Display_Name.index \
  --pitch 0 --f0_method rmvpe --index_rate 0.5
```
✅ **GOTCHA 5 — faiss segfault (SOLVED).** The crash (exit 139 in `faiss ... search` → `pipeline.py _retrieve_speaker_embeddings`) is **NOT** a numpy version issue — it's an **OpenMP runtime conflict**: faiss-cpu and torch each bundle their own libomp, and on macOS loading both crashes faiss's threaded search. **Fix = two env vars: `KMP_DUPLICATE_LIB_OK=TRUE` + `OMP_NUM_THREADS=1`.** With those set, `--index_rate 0.5`–`0.75` works and sharpens timbre toward the real speaker; `0.0` = model-only. (Verified: faiss search alone works fine — only the faiss+torch combo needed the fix. No package repin.) `--index_path` is required regardless.
- **`--pitch`:** match source→target gender. Female source → female target = `0`. Male source → female target = `+12`. Female source → male target = `-12`. Same gender = `0`. (Check output pitch with the analyzer; ~120–150Hz = male, ~180–255Hz = female.)
- ⚠️ **Don't build paths with `$(ls ...)`** — color codes corrupt the path → `NoneType has no attribute 'pipeline'`. Use `python -c "import glob; print(glob.glob(...)[0])"` or literal paths.
- Speed: ~5–8s per clip on M1 CPU. New engines clip hot → normalize levels if mixing.

---

## Gotchas quick-reference (all 5, in order hit)
| # | Symptom | Fix |
|---|---|---|
| 1 | `Speaker ID folder expected integer` / 0 files | Flatten dataset into one dir (no named subdirs) |
| 2 | preprocess: `--cut_preprocess required` | add `--cut_preprocess Automatic` |
| 3 | extract: `--include_mutes required` | add `--include_mutes 2` |
| 4 | `No such file: assets/config.json`, no inference `.pth` | `cp assets/config_template.json assets/config.json` (once) |
| 5 | inference exit 139 / segfault (faiss+torch OpenMP clash) | `export KMP_DUPLICATE_LIB_OK=TRUE; export OMP_NUM_THREADS=1` → index works at `--index_rate 0.5`+ |
| — | `NoneType ... 'pipeline'` | path corrupted by `ls` color or missing file — use clean glob/literal path |
| — | MPS crash | everything runs on CPU (config auto-selects it); keep the MPS-fallback env vars |

---

## The two voices already made (reference examples)
| Voice | Data | Chosen epoch | Why | Files |
|---|---|---|---|---|
| **Confident Neal** | 32 min narration | 100 (loss bottom ~69) | user liked ep100 by ear | `assets/rvc-models/confident-neal/Confident_Neal.pth`(+`.index`), plus `Confident_Neal_e70.pth` |
| **Cool Jahns** | 59 min (Jeremy Jahns, cleaned) | **25** (loss bottom) | generalizes best for unseen text; user liked 25 & 60 | `assets/rvc-models/cool-jahns/Cool_Jahns.pth`(+`.index`) |

## Pinned versions (this machine)
Applio main branch (2026-07) · torch 2.11 (Applio venv, MPS build) · faiss-cpu 1.14.3 · numpy 2.4.6 · kokoro-onnx 0.5.0 (repo venv). Model format: v2, 40kHz, HiFi-GAN, contentvec embedder.

## Speed note / alternative
Local CPU training is an overnight job. For a **~10–15× faster** run, train on a **free Kaggle T4** (Applio ships `assets/Applio_NoUI.ipynb` / `Applio_Kaggle.ipynb`): upload the prepped dataset, train (~30–60 min), download the `.pth`+`.index`, then infer locally. Same steps 2–4 apply.
