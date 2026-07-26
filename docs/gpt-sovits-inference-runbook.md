# GPT-SoVITS Local Inference Runbook (Apple Silicon M1 Pro) — run an already fine-tuned clone

> **What this produces:** one WAV file from a text prompt, spoken in a voice that was **already fine-tuned
> elsewhere** (this run used a Jeremy Jahns v2Pro clone trained on Colab: SoVITS 8 epochs/batch 4,
> GPT 15 epochs/batch 4, top-3 acc ~0.62). **This doc does not train anything** — it's pure inference setup.
> Training instructions live in `research/expressive-voice-clone-m1/finetune/gpt_sovits_finetune_colab.ipynb`.
>
> **Proven once on this machine** (M1 Pro, 16GB, macOS, no NVIDIA GPU). Every gotcha below was actually hit.

---

## 0. The concept

GPT-SoVITS is a two-stage clone: a GPT-style text→semantic-token model + a SoVITS (VITS-family) semantic→audio
decoder, both fine-tuned per-speaker. Inference needs the fine-tuned pair **plus** a short reference audio clip
of the target speaker (voice identity anchor) **plus** a small set of base feature-extraction models (BERT,
HuBERT, speaker-verification embedding) that are *not* speaker-specific and don't change per voice.

```
target text  +  reference clip (+ its transcript)  →  GPT (text→semantic)  →  SoVITS (semantic→wav)
```

---

## 1. Environment
- **Repo clone (outside this project, third-party ML tooling, never git-tracked here):**
  `~/repos/GPT-SoVITS-local` (`git clone https://github.com/RVC-Boss/GPT-SoVITS`)
- **Python:** `/opt/homebrew/bin/python3.11` → fresh venv at `~/repos/GPT-SoVITS-local/.venv`. **Do not use
  system Python 3.14** — the dependency stack (funasr, pyopenjtalk, jieba_fast, etc.) targets 3.10–3.12.
- **Fine-tuned weights** (already trained on Colab, not produced by this doc):
  - GPT: `GPT_weights_v2Pro/<name>-e<N>.ckpt`
  - SoVITS: `SoVITS_weights_v2Pro/<name>_e<N>_s<STEP>.pth`
  - Use the highest epoch unless a specific earlier checkpoint sounds better by ear (see §8 — earlier
    checkpoints can generalize better on a small fine-tune).
- **Device:** tried MPS first with `PYTORCH_ENABLE_MPS_FALLBACK=1`; fell back to CPU only on error. In practice
  MPS worked for every run once the torchcodec gotcha (below) was fixed — no CPU fallback needed.

---

## 2. Reference clip — required even in "no ref text" mode ⚠️ GOTCHA 1
v2Pro needs a reference **audio** clip regardless of whether you supply ref text. Omitting it entirely →
`RuntimeError: Failed to create AudioDecoder for None`.
- Cut **3–10s** of clean, uninterrupted speech (no music/intro) from a source clip:
  ```bash
  ffmpeg -i source.wav -ss 115 -t 9 ref_clip.wav
  ```
- Sanity-check it's not silence before using it: `ffmpeg -i ref_clip.wav -af volumedetect -f null -` and confirm
  mean volume is well above ~-40dB.
- **Longer (8–10s) + more dynamic/expressive clips anchor identity better than short flat ones** — see §8.

---

## 3. Dependencies ⚠️ GOTCHAs 2–5
```bash
cd ~/repos/GPT-SoVITS-local
/opt/homebrew/bin/python3.11 -m venv .venv && source .venv/bin/activate

# GOTCHA 2 — requirements.txt has no CUDA pin, but on a Mac you still want to install
# torch/torchaudio FIRST so pip resolves the mac wheel cleanly:
pip install torch torchaudio

# GOTCHA 3 — the real `opencc` PyPI package can fail to build from source. Filter it out
# and use the pure-Python reimplementation instead (same import name, same API):
grep -v -i '^opencc' requirements.txt | grep -v -i 'no-binary=opencc' > /tmp/reqs.txt
pip install -r /tmp/reqs.txt
pip install opencc-python-reimplemented

# NLTK data (needed by g2p_en for English phonemization):
python -c "import nltk; [nltk.download(p) for p in ['averaged_perceptron_tagger_eng','averaged_perceptron_tagger','cmudict','punkt','punkt_tab']]"
```
⚠️ **GOTCHA 4 — new torchaudio hard-requires `torchcodec` for `torchaudio.load()`.** Installing bare
`pip install torch torchaudio` on this machine resolved to a version where loading the reference WAV fails with
`ModuleNotFoundError: No module named 'torchcodec'` (not the same as gotcha 1's "no ref audio" error — this one
happens even *with* a valid ref clip path). **Fix:** `pip install torchcodec` (arm64 wheel installed cleanly,
backed by the Homebrew `ffmpeg`/`ffmpeg-full` already on this machine).

⚠️ **GOTCHA 5 — ffmpeg binary.** Already installed via Homebrew (`ffmpeg` + `ffmpeg-full` formulas) — just make
sure it's on `PATH` inside the venv's subprocess calls. No reinstall needed.

---

## 4. Base pretrained models — minimal set, not the full ~5GB snapshot
The README's "download the whole pretrained snapshot" advice is for training or for using the *stock* base
voice. **For inference with a fully fine-tuned (non-LoRA) checkpoint pair, only 3 files are load-bearing** —
confirmed by reading `GPT_SoVITS/TTS_infer_pack/TTS.py`'s `init_t2s_weights` / `init_vits_weights`: they load
straight from whatever path you pass, and only fall back to the stock base files if that path is missing.
The stock `s1v3.ckpt` / `v2Pro/s2Gv2Pro.pth` are never touched when you supply your own fine-tune.

```bash
BASE=https://huggingface.co/lj1995/GPT-SoVITS/resolve/main
DEST=GPT_SoVITS/pretrained_models
mkdir -p "$DEST/chinese-hubert-base" "$DEST/chinese-roberta-wwm-ext-large" "$DEST/sv"

curl -L -o "$DEST/chinese-hubert-base/config.json"             "$BASE/chinese-hubert-base/config.json"
curl -L -o "$DEST/chinese-hubert-base/preprocessor_config.json" "$BASE/chinese-hubert-base/preprocessor_config.json"
curl -L -o "$DEST/chinese-hubert-base/pytorch_model.bin"        "$BASE/chinese-hubert-base/pytorch_model.bin"       # ~189MB
curl -L -o "$DEST/chinese-roberta-wwm-ext-large/config.json"    "$BASE/chinese-roberta-wwm-ext-large/config.json"
curl -L -o "$DEST/chinese-roberta-wwm-ext-large/pytorch_model.bin" "$BASE/chinese-roberta-wwm-ext-large/pytorch_model.bin"  # ~651MB
curl -L -o "$DEST/chinese-roberta-wwm-ext-large/tokenizer.json" "$BASE/chinese-roberta-wwm-ext-large/tokenizer.json"
curl -L -o "$DEST/sv/pretrained_eres2netv2w24s4ep4.ckpt"        "$BASE/sv/pretrained_eres2netv2w24s4ep4.ckpt"        # ~108MB
```
Total ~950MB (vs. the ~5GB full snapshot). `sv/pretrained_eres2netv2w24s4ep4.ckpt` is specifically required for
v2Pro/v2ProPlus's speaker-verification embedding (`GPT_SoVITS/sv.py`) — v1/v2/v3/v4 don't need it.

---

## 5. Inference — drive `TTS_infer_pack` directly, skip the Gradio WebUI
```python
import os, sys
os.environ.setdefault("PYTORCH_ENABLE_MPS_FALLBACK", "1")
REPO = os.path.expanduser("~/repos/GPT-SoVITS-local")
os.chdir(REPO); sys.path.insert(0, REPO); sys.path.insert(0, os.path.join(REPO, "GPT_SoVITS"))

import soundfile as sf
from TTS_infer_pack.TTS import TTS, TTS_Config

cfg = TTS_Config({"custom": {
    "device": "mps",              # falls back to "cpu" on error — see gotcha 4, rarely needed once fixed
    "is_half": False,
    "version": "v2Pro",
    "t2s_weights_path": "/path/to/<name>-e<N>.ckpt",
    "vits_weights_path": "/path/to/<name>_e<N>_s<STEP>.pth",
    "bert_base_path": f"{REPO}/GPT_SoVITS/pretrained_models/chinese-roberta-wwm-ext-large",
    "cnhuhbert_base_path": f"{REPO}/GPT_SoVITS/pretrained_models/chinese-hubert-base",
}})
pipeline = TTS(cfg)

sr, audio = list(pipeline.run({
    "text": "Your target text here.",
    "text_lang": "en",
    "ref_audio_path": "/path/to/ref_clip.wav",
    "prompt_text": "Exact transcript of ref_clip.wav",   # see gotcha 6 — don't leave this empty
    "prompt_lang": "en",
    "text_split_method": "cut4",                          # see gotcha 7 — not cut5
    "top_k": 15, "top_p": 1.0, "temperature": 1.0,
    "batch_size": 4, "parallel_infer": True, "split_bucket": True,
    "seed": 42, "repetition_penalty": 1.35,
}))[-1]
sf.write("/path/to/output.wav", audio, sr)
```
`pipeline.run()` is a generator; for non-streaming requests it yields exactly one `(sr, audio_ndarray)` tuple.

---

## 6. Output convention ⚠️
**Never write generated WAVs into this project's git tree** (`intelligent-tts-narration-library-manual/`), even
though `.gitignore` already blocks `*.wav` as a backstop — same rule as the RVC pipeline
(`docs/rvc-voice-creation-runbook.md`): no personal/cloned-voice audio in git. Write outputs to a scratch/output
dir outside the repo. Whether the fine-tuned `.ckpt`/`.pth` weights themselves get copied into
`assets/` (as the RVC `.pth`/`.index` pairs are) is a separate decision to make deliberately, not a byproduct of
running inference.

---

## 7. Quality issues actually hit + fixes

⚠️ **GOTCHA 6 — no-ref-text ("ref_free") mode drifts toward generic/inconsistent identity mid-clip.** Leaving
`prompt_text` empty is a valid fallback (v2Pro doesn't hard-require it, unlike v3/v4), but it's noticeably less
stable than giving the model the reference clip's real transcript to anchor on. **Fix:** transcribe the ref clip
(a local Whisper pass via `transformers.pipeline("automatic-speech-recognition", model="openai/whisper-base.en")`
works fine, no extra download friction) and pass the exact transcript as `prompt_text`. Combined with a longer
(8–10s), more dynamic reference clip (vs. a short/flat one), this measurably fixed "sometimes generic in
between" on this voice.

⚠️ **GOTCHA 7 — `cut5` (the commonly-copied default in examples) splits on every punctuation mark, including
commas.** For a 76-word / 6-sentence story this produced **12 fragments**, each independently vocoder-decoded
then hard-concatenated with a fixed silence gap and no crossfade — audible as random small clicks/blips roughly
every ~2.5s. **Fix:** use `cut4` (splits only on sentence-ending periods) — same story became 6 fragments
(matching actual sentence count), and the artifacts went away. Not something baked into the trained voice; a
pure inference-time chunking choice.

**Remaining ceiling — crispness / "not that crisp":** inference-time tuning (ref clip, prompt text, split
method, sampling params) fixes *consistency* problems but cannot add missing high-frequency detail. Two real
options if this isn't good enough as-is:
- **Cheap, partial:** a gentle post-process high-shelf boost, e.g.
  `ffmpeg -i in.wav -af "highpass=f=80,treble=g=5:f=6500:width_type=o:width=1,alimiter=limit=0.95" out.wav`
  — genuinely helps a little (boosts existing high-frequency content) but plateaus fast; push it further and it
  turns harsh/sibilant instead of clearer.
- **Real fix, requires retraining:** fine-tune on a **v2ProPlus or v4 base** instead of v2Pro. v4 uses a 48kHz
  BigVGAN vocoder — materially higher-fidelity than v2Pro's 32kHz decoder, not just reshaped after the fact. The
  dataset and Colab recipe are already proven (this exact 8/15-epoch run finished clean), so this is a bounded
  amount of extra Colab time, not new unknowns. Source audio quality (compression artifacts in the original
  YouTube clips) is also a hard floor either way.

---

## Gotchas quick-reference
| # | Symptom | Fix |
|---|---|---|
| 1 | `RuntimeError: Failed to create AudioDecoder for None` | Always pass `ref_audio_path`, even in no-ref-text mode |
| 2 | pip resolves a CUDA torch build | `pip install torch torchaudio` first, before `requirements.txt` |
| 3 | `opencc` fails to build from source | filter it from requirements, `pip install opencc-python-reimplemented` |
| 4 | `ModuleNotFoundError: No module named 'torchcodec'` on ref-audio load | `pip install torchcodec` |
| 5 | (n/a on this machine) | Homebrew `ffmpeg`/`ffmpeg-full` already on PATH, no reinstall needed |
| 6 | voice sounds "generic" mid-clip | stop using no-ref-text mode; pass the ref clip's real transcript as `prompt_text` |
| 7 | random clicks/blips every ~2–3s | switch `text_split_method` from `cut5` to `cut4` |
| — | output not crisp enough | ffmpeg high-shelf EQ (partial) or retrain on v2ProPlus/v4 base (real fix) |

## Pinned versions (this machine)
Python 3.11.13 (Homebrew) · torch 2.13.0 (mac wheel, MPS-capable) · torchaudio 2.11.0 · torchcodec 0.15.0 ·
opencc-python-reimplemented · GPT-SoVITS main branch (2026-07) · v2Pro config, 32kHz output.
