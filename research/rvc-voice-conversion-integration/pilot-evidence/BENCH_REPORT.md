# Bench Report — Cold vs Warm load/convert for the two RVC inference paths ("Cool Jahns")

**Date:** 2026-07-22  **Machine:** MacBookPro18,1 (M1 Pro, 10 cores, 16 GB RAM), macOS arm64, **CPU only**.
**Env:** Applio `.venv` (Python 3.12.13), onnxruntime 1.27.0, faiss-cpu 1.14.3, torch 2.11.
**Fixed source clip:** `assets/huginn-samples/rvc/long_male_src.wav` — 24 kHz mono, **15.488 s** (resampled to 16 kHz internally). Params: index_rate 0.75, pitch 0, protect 0.33, f0 rmvpe, volume_envelope 1. Identical for every measurement.
**Disk state:** "warm-disk-cache cold" (fresh process, files already in OS page cache). No `sudo purge` (would block on a password prompt). True disk-cold estimated analytically below.
**No production code in the library repo was modified.** Bench scripts live in `pilot-d/`: `bench_onnx.py`, `bench_torch_warm.py`, `run_onnx_battery.sh`, `run_torch_battery.sh`. Raw logs: `onnx_battery.log`, `torch_battery.log`.

Method: `time.perf_counter()` for in-process splits; `/usr/bin/time -l` for whole-process wall (`real`) + peak RSS (`maximum resident set size`, bytes). COLD = fresh process (interpreter + imports + all loads + 1 convert), 3× → median. WARM = load once, convert the same clip 5× in-process → median convert-only. ORT ran **multithread** (all 10 cores; safe torch-free). Torch ran with the required `OMP_NUM_THREADS=1` + MPS-fallback env (single-thread — faiss/libiomp clash forbids ORT-style multithreading there).

---

## Headline table (medians)

| path | COLD total (load+1 convert, wall) | ├ imports | ├ model/index load | └ first convert | WARM convert (per clip) | peak RSS |
|---|---|---|---|---|---|---|
| **ONNX (full-load)** | **9.55 s** | 0.81 s | 0.87 s* | 7.12 s | **6.93 s** | **3.70 GB** |
| **ONNX (mmap)** | **8.90 s** | 0.70 s | 0.57 s* | 7.02 s | 6.91 s | **2.75 GB** |
| **Torch (Applio)** | **15.36 s** | **~5.5 s** (torch/Applio chain) | ~1.0 s | ~7.8 s | **8.35 s** naive / 7.76 s cached | **4.48 GB** |

\* ONNX model/index load excludes source decode (soxr 24→16 kHz), reported separately: 0.52 s (full) / 0.44 s (mmap). Columns don't sum exactly to COLD total: add interpreter start (~0.25 s) + source decode + WAV write. Peak RSS = `maximum resident set size` for the cold single-convert process.

### ONNX per-component load split (median of 3, seconds)
| | contentvec.onnx sess | rmvpe.onnx sess | net_g.onnx sess | melbasis | big_npy (557 MB) | faiss index (573 MB) |
|---|---|---|---|---|---|---|
| **full** | 0.31 | 0.10 | 0.22 | 0.001 | 0.133 | 0.110 |
| **mmap** | 0.31 | 0.10 | 0.22 | 0.001 | **0.009** | **0.003** |

The three ONNX session creates (~0.63 s total) dominate ONNX load. The 1.13 GB retrieval data (big_npy + faiss index) costs only ~0.24 s to read fully, and **~0 s under mmap** (deferred to page-fault-on-use).

---

## Raw medians (with min/max)

**ONNX full — COLD (n=3):** wall 9.55 s (9.41–9.93) · imports 0.81 · model_load 0.87 · first_convert 7.12 (7.04–7.13) · peak RSS 3.70 GB (3.58–3.90) · peak footprint 3.81 GB.
**ONNX mmap — COLD (n=3):** wall 8.90 s (8.90–9.09) · imports 0.70 · model_load 0.57 · first_convert 7.02 · peak RSS 2.75 GB (2.75–2.76) · peak footprint 2.47 GB.
**ONNX full — WARM (n=5):** convert median 6.93 s (6.88–7.17), RTF 0.45. peak RSS over 6 converts 3.63 GB, footprint 4.98 GB.
**ONNX mmap — WARM (n=5):** convert median 6.91 s (6.89–6.98), RTF 0.45. peak RSS over 6 converts **3.91 GB** (climbs — mmap pages fault in), footprint 3.63 GB.

**Torch import tax (isolated, /usr/bin/time real):** `import torch` 1.18 s · `import torch,faiss,librosa,numpy,soundfile` 0.78 s · **full Applio chain `from rvc.infer.infer import VoiceConverter` 6.11 s** (in-process, warm cache: 4.26 s). torch itself is ~1 s; the extra ~4–5 s is torchcrepe / pedalboard / noisereduce / transformers / faiss / librosa pulled in by Applio.
**Torch — COLD (n=3, rvc-convert.sh):** wall 15.36 s (15.24–15.53) · Applio's own "conversion" timer (excludes net_g load) 9.63 s (9.56–10.42) · peak RSS 4.48 GB (4.09–5.21) · peak footprint 5.38 GB.
**Torch — WARM (n=5):** naive median 8.35 s (8.07–9.18), RTF 0.54 · **compute-only estimate 7.76 s** (subtracting the 0.59 s/call Applio re-pays). Load split: torch/import 4.26 s, get_vc (net_g) 0.32 s, load_hubert 0.08 s. Per-call reload Applio bakes into every `pipeline()`: faiss read 0.11 s + reconstruct_n 0.13 s + RMVPE(rmvpe.pt) init 0.36 s = 0.59 s. peak RSS over 5 converts 5.89 GB, footprint 6.09 GB.

> **Gotcha found in Applio's code:** `Pipeline.pipeline()` **re-reads the 573 MB faiss index (+reconstruct_n) and re-instantiates the RMVPE model (172 MB rmvpe.pt) on every call** (`pipeline.py` L432-433 and L236-239). So a naive "keep VoiceConverter alive" worker still pays ~0.59 s/clip of avoidable reload. Hoisting those into a cache gets torch warm from 8.35 s → 7.76 s/clip. The ONNX path has no such wart — its sessions + index + big_npy are process globals, so warm converts are pure compute.

---

## Answers

### 1. ONNX cold vs torch cold — which reaches first-audio faster?
**ONNX wins decisively.** ONNX cold = **9.55 s** (full) / **8.90 s** (mmap); torch cold = **15.36 s**. ONNX is **~5.8 s faster (full) / ~6.5 s faster (mmap) — ~38–42 % less time to first audio.** **Hypothesis CONFIRMED:** the gap is dominated by imports — ONNX imports in **0.8 s** vs Applio's **~5.5 s** torch chain (a ~4.7 s swing). Bonus: the convert itself is also faster on ONNX (7.1 s multithread vs 7.8 s single-thread torch), because torch-free ORT is free to use all 10 cores while the torch path is pinned to `OMP_NUM_THREADS=1` by the faiss/libiomp clash. So ONNX beats torch on both the import tax *and* the convert.

### 2. Warm vs cold gap — how much does staying warm save per clip?
- **ONNX:** cold-first-audio 9.55 s → warm 6.93 s ⇒ **keep-alive saves ≈ 2.1–2.6 s/clip.** That saving is entirely the one-time interpreter+imports+model-load (~0.25 + 0.8 + 0.9 ≈ 2 s); the ~7 s convert is unavoidable either way (first-inference warmup is negligible, ~0.2 s). **Keep-alive buys little.**
- **Torch:** cold-first-audio 15.36 s → warm 8.35 s (naive) / 7.76 s (cached) ⇒ **keep-alive saves ≈ 7.0–7.6 s/clip** — almost all of it the ~5.5 s Applio import tax plus ~1 s model load. **Keep-alive buys a lot** here, purely because the cold overhead is ~7× larger than ONNX's.

### 3. Peak RSS, and how much mmap reduces it
- **ONNX full:** 3.70 GB (cold, one convert). **ONNX mmap: 2.75 GB** ⇒ mmap of the faiss index + big_npy cuts **~0.95 GB (~26 %)** off resident (peak footprint drops even more, 3.81→2.47 GB, ~1.34 GB). **But the win is only for ephemeral single-job workers:** over 6 converts the mmap'd pages fault in and warm-mmap RSS climbed back to 3.9 GB (≈ full). mmap lowers *first-job* footprint, not a long-lived worker's steady state.
- **Torch:** 4.48 GB cold single-convert; **5.89 GB after 5 warm converts** (memory grows). Heaviest of the three. peak footprint 5.4–6.1 GB.
- Ranking (resident): **ONNX-mmap 2.75 GB < ONNX-full 3.70 GB < Torch 4.48–5.89 GB.**

### 4. Verdict — is exit-after-job (ephemeral, zero idle RAM) comfortable?
**Yes for the ONNX path; not really for the torch path — so run the ONNX path ephemerally.**

- **ONNX ephemeral (load → convert → exit): comfortable.** First-call is 9.5 s, of which ~7 s is the convert you'd pay warm anyway; the *extra* cost of being ephemeral vs keeping warm is only **~2 s/clip**. Zero idle RAM. The cold penalty is small and acceptable. Add faiss+big_npy **mmap** to hold peak RSS at ~2.75 GB for single jobs.
- **Torch ephemeral: not comfortable.** Exit-after-job costs ~7 s/clip in reload tax — as much as the convert itself, roughly *doubling* per-clip latency (8.4 s → 15.4 s). If torch must be used, keep it warm for the job's duration; but the better answer is to not use torch for narration converts at all.

**Recommended idle policy (numbers behind it):**
1. **Use the torch-free ONNX path**, launched as an **ephemeral per-job worker** (load, convert, exit). Cold ≈ 9.5 s, only ~2 s of which is avoidable overhead; zero idle RAM.
2. **Within a single narration job that renders many blocks**, keep the *one* ONNX worker warm for the duration (each subsequent block ≈ 6.9 s instead of 9.5 s), then exit at job end. A short idle keep-alive (≈ 30–60 s) can bridge bursty back-to-back jobs, but a hard exit-after-job is fine given the ~2 s penalty.
3. **Enable faiss/big_npy mmap** for the ephemeral case to keep peak RSS ~2.75 GB (vs 3.70 GB full). For a long-lived warm worker mmap gives no steady-state RSS benefit, so it's optional there.
4. **Do not keep a torch worker idle** — its 4.5–5.9 GB resident footprint is the worst and its only advantage (avoiding the 5.5 s import) is moot once you've switched to ONNX.

---

## Disk-cold note (analytical, not measured)
All numbers above are warm-disk-cache. True first-ever disk-cold adds the time to read the model/index bytes from SSD:
- **ONNX reads ~1.90 GB** (contentvec 360 + rmvpe 345 + net_g 105 + big_npy 557 + faiss index 573 MB). mmap defers big_npy+index but they still fault in during the convert, so bytes touched ≈ same.
- **Torch reads ~1.13 GB** (pth 53 + faiss index 573 + rmvpe.pt 173 + contentvec 361 MB).

Additive upper bound at an assumed sustained SSD read of **3 GB/s** (Apple NVMe typically 3–5 GB/s): **+~0.6 s (ONNX) / +~0.4 s (torch)**. At a conservative **1.5 GB/s**: +~1.3 s / +~0.8 s. This is an additive bound on the load portion only; it does not change the verdict (ONNX true-cold ≈ 10.1 s, torch true-cold ≈ 15.8 s at 3 GB/s).
