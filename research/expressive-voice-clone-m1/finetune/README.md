# Fine-tune handoff — cool-jahns (Jeremy Jahns) voice

Empirical prep + runnable fine-tunes for the `cool-jahns` dataset. Produced during the
huginn research run (`../report.md`). **Both pipelines; run GPT-SoVITS first.**

## The honest constraint

Training was **not** run here — this session is on an **M1 Pro (no CUDA GPU)**, and
Orpheus/GPT-SoVITS training are CUDA jobs. Local python is 3.14 (too new for the ML
wheels). So: **all local prep is done + verified; the GPU training step you run on a
free Colab T4.** These notebooks are ready-to-run but *unrun by me* — execute a cell,
paste any error back, I fix it live. This is the opposite of blind approval: we get a
real clone you can hear.

## Data sufficiency — VERIFIED (not estimated)

Ran a real resample + silence-slice locally over all 9 files:

```
59.1 min clean speech  →  408 segments  (avg 7.3s, 49.7 min usable after pause-trim)
```

**408 ≫ Orpheus's ~300 target and ≫ GPT-SoVITS's need.** Data is sufficient for a
robust clone. You do **not** need to provide more for a first run.

- Content note: this is Jahns doing **movie reviews** — animated, opinionated, *some*
  sarcasm mixed with earnest hype. This clones his **general expressive register**,
  not "sarcasm-by-default." Per `../report.md` §4, sarcasm-default needs a
  sarcasm-weighted subset — a later pass once you've heard the base clone.

## Packages (in session scratchpad — personal audio, NOT committed to git)

| File | What | For |
|---|---|---|
| `jahns-full-24k.zip` (145 MB) | 9 full files, resampled 24 kHz mono | GPT-SoVITS (its pipeline slices) |
| `jahns-segments-24k.zip` (128 MB) | 408 pre-sliced 3–10 s segments + `manifest.json` | Orpheus (needs short clips) |

Path: `…/scratchpad/jahns-prep/`. Upload to Google Drive for the Colab runs.

## Run order

1. **GPT-SoVITS** (`gpt_sovits_finetune_colab.ipynb`) — fastest listen, and it can also
   run inference on your M1 after. Directly tests your original question: does Jahns's
   tone/sarcasm survive cloning? ASR + slicing happen inside its pipeline.
2. **Orpheus** (`orpheus_finetune_colab.ipynb`) — the research pick. ~$0 LoRA on T4,
   exports a Q4 GGUF that runs on your M1 (verified 2.36 GB). Compare head-to-head.

## After both

Play both on the M1, judge by ear:
- Timbre match (does it sound like Jahns)?
- Does his expressive/sarcastic delivery carry, or flatten to neutral?

That verdict decides the next move (sarcasm-curated subset, IndexTTS-2 Emo-Audio layer
per `../report.md` §9, or ship the winner as the library's voice box).
