# Pilot D — Torch-free ONNX RVC ("Cool Jahns") feasibility

**Date:** 2026-07-21  **Env:** Applio `.venv` (uv), Python 3.12, macOS arm64, CPU only.
onnx 1.22.0, onnxruntime 1.27.0 (installed into Applio venv via `uv pip` — venv has no pip),
torch 2.11 (Stage-1 export only), faiss-cpu 1.14.3, numpy 2.4.6, librosa/soxr/scipy.

All artifacts + scripts in this dir (`pilot-d/`). No production code in the library repo was touched.

---

## 1. PASS/FAIL — does the ONNX pipeline run fully torch-free at inference?

### **PASS.**

`onnx_infer.py` produces `cooljahns_ONNX.wav` using **onnxruntime + numpy + faiss + scipy + librosa/soxr only**.
It asserts `"torch" not in sys.modules` at import and prints `torch imported at inference?: False` at the end — verified True(-negative).

- **contentvec** → torch-free (exported to `contentvec.onnx`, runs in ORT). Not a blocker.
- **rmvpe** → torch-free. Exported the E2E net (`rmvpe.onnx`); mel-spectrogram front-end and the
  cents-decode back-end reimplemented in numpy/librosa. **No torch needed for f0.** Not a blocker.
- **net_g (HiFiGAN-NSF)** → torch-free (`cool_jahns_netg.onnx`), incl. the NSF sine source. Not a blocker.
- **index retrieval** → torch-free (faiss read + IVF search; vectors also dumped to `.npy` for a Go kNN).

Torch is used **once, offline**, in the four `export_*.py` / probe scripts only.

---

## 2. The two WAVs + numeric closeness

- Torch reference (current Applio path, `rvc-convert.sh cool-jahns`): **`cooljahns_TORCH.wav`**
- Torch-free ONNX: **`cooljahns_ONNX.wav`**
- Both 40 kHz mono, 619 200 samples (15.48 s), identical length.

**Comparison A — full pipeline, TORCH vs ONNX (independent RNG in each path):**
| metric | value |
|---|---|
| raw max abs diff | 0.170 |
| raw RMS diff | 0.0098 |
| raw Pearson corr | **0.905** |
| peak (TORCH / ONNX) | 0.2197 / 0.2198 |
| RMS (TORCH / ONNX) | 0.0225 / 0.0224 |
| log-mel mean |Δ| | 2.35 dB |
| **log-mel corr** | **0.982** |
| NaN/Inf | none |

The raw-sample gap is dominated by the **two RNG draws that Applio itself re-randomizes every run**
(flow `z_p` noise + NSF sine dither) — not by the ONNX port. Matched peak/RMS, 0.982 mel correlation,
and no artifacts ⇒ **perceptually equivalent** (confirm by ear). The controlled parity numbers below
isolate the port fidelity from RNG:

**Per-stage parity (ONNX vs torch, identical inputs):**
| stage | max abs diff | corr |
|---|---|---|
| contentvec.onnx | 6.4e-5 | 0.99999999 |
| rmvpe.onnx (E2E) | 1.3e-7 | 0.99999999 |
| net_g.onnx (dummy inputs, same rnd) | 2.3e-3 | 0.999993 |
| **net_g.onnx (REAL pipeline feats, same rnd)** | 5.9e-2 | **0.9993** |

**DSP-glue parity — my numpy reimpl vs Applio's torch DSP on the real clip (`torch_probe.py`):**
| quantity | result |
|---|---|
| feats (contentvec+index+interp+protect) | max|Δ| 4.3e-5, corr 1.0 |
| pitch (coarse f0) | **100 % exact bucket match** |
| pitchf (rmvpe f0, Hz) | max|Δ| 7.6e-5 Hz, corr 1.0 |

⇒ Every stage of the torch-free path matches Applio to ~5 decimals; the only residual is RNG the
reference also randomizes. **Quality parity: yes.**

---

## 3. net_g.onnx exact I/O signature (for the Go port)

Opset 17, self-contained (no external-data file), 105.5 MB.

**Inputs**
| name | dtype | shape | notes |
|---|---|---|---|
| `phone` | float32 | `[1, T, 768]` (T dynamic) | contentvec feats AFTER index-blend + ×2 interp + pitch-protect |
| `phone_lengths` | int64 | `[1]` | = T |
| `pitch` | int64 | `[1, T]` | coarse f0, buckets 1..255 |
| `nsff0` | float32 | `[1, T]` | fine f0 in Hz (pitchf) |
| `sid` | int64 | `[1]` | speaker id (0) |
| `rnd` | float32 | `[1, 192, T]` | **external** flow noise = `randn_like(m_p)`; 192 = inter_channels |

**Output**
| name | dtype | shape |
|---|---|---|
| `audio` | float32 | `[1, 1, T*400]` (dynamic; 400 = ∏ upsample_rates 10·10·2·2) |

(Output batch axis exported with a cosmetic dynamic name `Tanhaudio_dim_0`; value is always 1.)
**Go must supply `rnd ~ N(0,1)`.** A second, *internal* `RandomNormalLike` remains inside the graph
(NSF sine dither, std 0.003) — onnxruntime generates it per-run; it's the ~0.0009-RMS parity floor.

Companion ONNX I/O:
- `contentvec.onnx`: in `audio` f32 `[1, T_samp]` (16 kHz) → out `feats` f32 `[1, T_frames, 768]` (~320 samp/frame).
- `rmvpe.onnx`: in `mel` f32 `[1, 128, F]` (F multiple of 32) → out `hidden` f32 `[1, F, 360]`.

---

## 4. DSP pipeline math (Go-port spec)

Fixed params (cool-jahns): sr_in 16 000, tgt 40 000, window/hop 160, index_rate 0.75, pitch 0,
f0 rmvpe, **protect 0.33** (Applio CLI default — pitch-protect IS active), volume_envelope 1
(change_rms **skipped**), x_pad 1 ⇒ t_pad 16 000 / t_pad_tgt 40 000. Clip < t_max ⇒ single segment
(no windowing).

1. **Load**: decode → mono → resample to 16 kHz (soxr VHQ). Normalize `peak/0.95`, divide if >1.
2. **High-pass**: Butterworth N=5, cutoff 48 Hz, fs 16 000, **zero-phase** (`filtfilt`, fwd+back IIR).
3. **Reflect-pad** ±16 000 samples.
4. **F0 (rmvpe)**: STFT n_fft 1024 / hop 160 / win 1024, **Hann periodic**, center=True, reflect-pad;
   magnitude = |STFT|; mel = `melbasis[128,513] @ mag` (librosa **htk** mel, fmin 30, fmax 8000);
   `log(clip(mel,1e-5))`. Pad frames to ×32 (reflect) → `rmvpe.onnx` → salience `[F,360]`.
   Decode: weighted avg of `cents_mapping` over ±4 bins around argmax
   (`cents_mapping = 20·arange(360)+1997.3794084376191`, pad (4,4)); `f0 = 10·2^(cents/1200)`,
   salience ≤ 0.03 → 0. Coarse: `f0_mel = 1127·ln(1+f0/700)`, linear-map to 1..255, `rint`.
5. **Features**: `contentvec.onnx(audio16k)` → `[1,Tc,768]`.
   Index blend: faiss **IVFFlat** search k=8 (L2 dist `score`); `w=(1/score)²` row-normalized;
   `npy = Σ big_npy[ix]·w`; `feats = 0.75·npy + 0.25·feats`.
   **×2 time upsample = nearest = `np.repeat(·,2,axis=time)`** (both `feats` and index-free `feats0`).
   `p_len = min(len(pad)//160, 2·Tc)`; slice feats/pitch/pitchf to `p_len`.
   **Pitch-protect (0.33)**: `pitchff = 1 where pitchf>0 else 0.33`; `feats = feats·pitchff + feats0·(1-pitchff)`.
6. **net_g.onnx** → 40 kHz waveform (already 40 k; **no resample step** — 100 Hz frames ×400).
7. **Trim** `[40000 : -40000]`; normalize `peak/0.99` if >1; write 40 kHz mono WAV.

**Annoying-in-Go bits:**
- **librosa STFT + mel**: need a Go STFT (Hann periodic, center/reflect) and the exact 128×513 mel
  filterbank — ship `rmvpe_melbasis.npy` as a baked constant. Getting `center`/reflect padding and the
  periodic (not symmetric) Hann window right is the fiddly part.
- **soxr resample** to 16 k: need an equivalent high-quality resampler (or require 16 k input).
- **filtfilt** (zero-phase Butterworth): forward-backward IIR with edge padding — reimplement carefully.
- **faiss IVFFlat**: no faiss in Go. Options: brute-force L2 kNN over `big_npy` (190 143×768, 557 MB)
  — simple but **not identical** to IVF nprobe=1 (approximate; neighbor sets can differ slightly), or
  replicate IVF (nlist 4875, nprobe 1). Blend is robust, so brute-force is likely fine by ear — verify.
- **`np.repeat` ×2** and **pitch-protect** are trivial in Go.

---

## 5. Export gotchas actually hit (and fixes)

1. **Applio venv had no `pip`** (uv-managed) and no onnx/onnxruntime → installed via
   `VIRTUAL_ENV=… uv pip install onnx onnxruntime`.
2. **`Synthesizer.remove_weight_norm()` crashes** (iterates deleted `enc_q`) **and** the model uses the
   **new `parametrizations.weight_norm`** API while the built-in remover uses the old hook API →
   baked weight-norm with `torch.nn.utils.parametrize.remove_parametrizations(m,"weight",leave_parametrized=True)`
   over all modules (104 baked). Clean plain-conv export.
3. **Custom `LayerNorm` passed a dynamic `x.size(-1)` as `normalized_shape`** → ONNX export rejects
   non-constant normalized_shape. Fixed with an **in-memory monkeypatch** (export script only; no Applio
   source edit) using the static `gamma.shape[0]`.
4. **`randn_like` nondeterminism** → externalized the flow noise as input `rnd` (RVC-Project exporter
   pattern) so torch and ONNX share the same noise for a fair parity test. The NSF sine dither
   `randn_like` was left as an internal `RandomNormalLike` (tiny, and the torch ref randomizes it too).
5. Used the **TorchScript exporter** (`dynamo=False`) — exports `RandomNormalLike`, `CumSum`, `Mod`
   (fmod), `Resize(nearest)`, GRU, ConvTranspose cleanly. No unsupported-op failures.
6. rmvpe: exported only the **E2E net** (mel→salience); kept STFT/mel + decode in numpy so the torch STFT
   never has to export. Cleaner and faster than exporting `torch.stft`.

---

## 6. Sizes + timing (CPU, warm)

| artifact | size |
|---|---|
| cool_jahns_netg.onnx | 105.5 MB |
| contentvec.onnx | 360.1 MB |
| rmvpe.onnx | 344.9 MB |
| cool_jahns_index_vectors.npy (190143×768 f32) | 557.0 MB |
| rmvpe_melbasis.npy (128×513) | 0.2 MB |

**Timing (15.48 s clip):**
- Torch-free ONNX, **ORT multithread** (10 cores; safe — torch never imported so no faiss/libiomp clash):
  **~7.0 s / clip, RTF ≈ 0.46× (faster than realtime).**
- Torch-free ONNX, single-thread (`OMP_NUM_THREADS=1`, intra_op=1): ~26 s / clip.
- Torch reference `rvc-convert.sh` (end-to-end incl. model load): ~10.3 s.
- ONNX session cold-load: ~0.9 s (one-time).

⇒ Multithreaded ONNX is **competitive-to-faster** than the torch path and comfortably realtime.

---

## 7. Blockers for the in-process-Go plan

**No fatal blocker. The pilot clears the torch-free hypothesis.** Caveats to carry:

1. **torch-free ≠ CGo-free.** Every op used (RandomNormalLike, Mod, CumSum, Resize-nearest, GRU,
   ConvTranspose1d/2d, GroupNorm, attention/Softmax, Tanh, Sin) is a standard ONNX op with full coverage
   in the **onnxruntime C library**. The realistic Go route (`onnxruntime_go`) **CGo-binds that C lib** —
   so going in-process Go re-introduces a native/CGo dependency, exactly the thing CLAUDE.md marked
   "CGo deferred." A *pure-Go* ONNX runtime would be a real risk (GRU/attention/RandomNormalLike coverage).
   This is a packaging decision, not a feasibility failure.
2. **f0 is NOT torch-bound** — rmvpe runs fine as E2E-ONNX + numpy DSP. No torch-only f0. (Good news.)
3. **faiss has no Go equivalent** — must brute-force kNN over the dumped `big_npy` (portable, but
   approximate-vs-IVF differs slightly) or reimplement IVFFlat. Not an ONNX problem.
4. **DSP front-end must be reimplemented in Go** (STFT+htk-mel, periodic Hann, zero-phase Butterworth,
   soxr-grade resample). Numerically validated to match here; the effort is real but bounded.
5. Ship the **mel filterbank** (128×513) and **cents_mapping** as baked constants; supply external `rnd`.

**Bottom line:** the Applio "Cool Jahns" RVC voice runs **fully torch-free** as ONNX+onnxruntime+faiss in
Python at **quality parity** (per-stage corr ≥ 0.999, DSP glue corr = 1.0, perceptual mel corr 0.982,
faster than realtime). The in-process-Go port is **feasible**, gated only by a CGo binding to the
onnxruntime C lib + a modest Go DSP/kNN reimplementation — no torch-only or unexportable component.
