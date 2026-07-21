# Problem-lock — Efficient RVC voice-conversion in the Go TTS pipeline

**Gear:** DEEP · **Intent:** FEASIBILITY (primary) + APPROACH/DECIDE (secondary)
**Date:** 2026-07-21 · **Status:** Checkpoint 1 (awaiting approval)

## Restatement (one sentence)

Find the best long-term way to run our two already-trained RVC voices (Applio v2 / 40k / HiFi-GAN / contentvec `.pth` + faiss `.index`) as an **efficient, reliable, tech-stack-consistent** repaint step inside the Go narration pipeline — where "torch-free ONNX + onnxruntime-go" is the user's current lean, **not** a fixed requirement.

## XY resolution

- **Stated Y:** "implement torch-free ONNX + onnxruntime inference."
- **Real X (surfaced at Checkpoint 1):** *efficiency + a consistent technology language + best long-term solution.* User is OK with Python if it's genuinely best; leans ONNX/Go for speed + personal comfort; **explicitly open to a better solution.**
- **Consequence:** discovery does NOT decompose from the ONNX prior. It explores the real space (ONNX-in-Go, ONNX-in-Python, load-once Python worker, Applio-as-daemon, other VC engines) and lets the rubric decide.

## Frame

| KNOWNS | tag | note |
|---|---|---|
| M1 Pro / 16GB / macOS, no CUDA, CPU-only (MPS segfaults in RVC) | **[hard]** | machine reality |
| Local + free only (hobby project, no recurring spend) | **[hard]** | |
| Must reuse the 2 trained voices' timbre (cool-jahns, confident-neal) | **[hard]** | ~8h training each; a solution that discards them must justify a full retrain |
| Models are Applio-flavored: v2, 40k, f0, vocoder='HiFi-GAN', contentvec | **[hard]** | net_g config dumped; not classic NSF net_g |
| The faiss `.index` must stay usable (index_rate 0.5–0.75 = quality) | **[hard]** | faiss ≠ torch; works torch-free (our faiss-fix report) |
| Quality parity with current torch inference, judged by ear | **[hard]** | acceptance test; user's bar is high |
| Go as the integration language | **[soft]** | "fast + I'm comfortable" — a preference, "doesn't need to be this" |
| ONNX as the mechanism | **[soft]** | current lean, "open to change my mind" |
| torch-free end state | **[soft]** | wants efficiency, not torch-elimination per se |
| Integrate as a `render.Renderer` decorator over sherpa | **[soft]** | our proposed architecture; the seam, not the goal |

## UNKNOWNS (research targets)

1. Can Applio's **HiFi-GAN** net_g be exported to ONNX cleanly (`torch.onnx.export` via Applio's own Synthesizer class) — unsupported ops? Dynamic axes? Vocoder graph self-contained?
2. Do existing **torch-free ONNX RVC runtimes** (voiceclonnx/TigreGotico, tts-with-rvc-onnx, rvc-onnx-web, w-okada, others) support v2/40k **+ faiss index_rate + the Applio HiFi-GAN vocoder**, or only classic NSF net_g?
3. **contentvec + rmvpe as ONNX** on M1 (CoreML/CPU) — availability, quality, licensing.
4. **onnxruntime-go** maturity for this graph on macOS/M1 (CoreML EP? CPU only?) — is true in-process Go realistic, or does "Go" really mean "Go orchestrates a Python worker"?
5. **rvc-python load-once worker** (daswer123) as the guaranteed fallback — real per-clip latency after warm load, memory, reliability.
6. **Applio-as-daemon** (keep the working stack, just stop cold-starting) — cheapest reliable win? What's the effort vs a rewrite?
7. Any **other voice-conversion engine** (e.g. seed-vc, so-vits derivatives) that is more tech-consistent / higher-quality AND worth a retrain — or firmly not worth abandoning the trained voices.

## Interaction map (seams that can break)

```
Go pipeline ──(render.Renderer)── sherpa Engine (Kokoro subprocess, 24kHz WAV/block)
                                        │
                                        ▼  NEW repaint step, per block
                          [ RVC runtime: contentvec → rmvpe f0 → faiss retrieve → net_g → 40kHz WAV ]
                                        │
   seams: (a) 24k↔40k format vs sink/timeline assumptions
          (b) Go↔runtime boundary (in-process CGo? subprocess? worker socket?)
          (c) faiss+torch OpenMP clash (gone iff torch not co-loaded)
          (d) runtime availability on a machine without Applio/torch → error vs degrade-to-Kokoro
```

## Acceptance

A graded, cited **decision** (go-ONNX-in-Go · ONNX-in-Python · rvc-python worker · Applio-daemon · other) **+ a spike plan for the finalist** (huginn deep-dive on one). Quality-parity-by-ear is the gate; efficiency + tech-consistency are the rubric's heavy axes.
