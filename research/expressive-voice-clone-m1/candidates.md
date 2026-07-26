# Discovery — solution space (Stage 1 output)

6 scouts (4 reframes + 2 lateral), web-grounded 2026-07. 67 raw → ~20 real models +
5 adjacent approaches after dedup. Grouped by **how expressive delivery is produced**
(the user's core axis: ideal = model *learns from text* where to be sarcastic/emotional).

Viability = HARD constraints only (open/no-recurring-pay · M1 16GB inference ·
fine-tune-to-person · clones a specific person). License/quality = recorded, ranked later.

## Class 1 — Learned-from-text delivery (the ideal: model infers emotion from text)
| Model | License | M1 fit | Fine-tune-to-person | Notes |
|---|---|---|---|---|
| **StyleTTS2** | MIT (code) | ★ ~300MB, CoreML port | ★ documented recipe (~1-3h audio) | style-diffusion predictor *generates* prosody from text. Best structural fit for self-learned. |
| **Orpheus TTS** (Llama-3B) | Apache-2.0 | Q4 GGUF ~2-4GB via Metal/LM Studio | ★ recipe (~300 samples/speaker) | LM backbone infers emotion from context after fine-tune + in-text emotion tags. |
| **Sesame CSM-1B** | Apache-2.0 | ★ native MLX (csm-mlx) | community LoRA | infers prosody from text + preceding audio; conversational strength. |
| VibeVoice (0.5B) | MIT (community fork) | 0.5B light, PyTorch | ✗ no clone-finetune recipe | emergent expressiveness, no control lever, upstream repo pulled by MS. |

## Class 2 — Explicit emotion control (instruct / emotion-vector / tags; decoupled from speaker)
| Model | License | M1 fit | Fine-tune-to-person | Notes |
|---|---|---|---|---|
| **Qwen3-TTS** (0.6B/1.7B) | Apache-2.0 | ★ native mlx-audio, dedicated M1 repos | ★ official Base-model SFT | instruct CustomVoice (tone/pace/emotion) + fine-tune. Freshest (Jan 2026). |
| **CosyVoice 2/3** (0.5B) | Apache-2.0 | Mac CPU today (MPS open req) | ★ official SFT recipe | natural-language instruct ("speak sarcastically/faster") + emphasis/laugh tags + speaker SFT. |
| **IndexTTS-2** | code Apache-2.0; **weights research-only** | borderline: 16GB min, MPS works | ✗ not yet (LoRA announced Q4'25, unconfirmed) | strongest emotion-decoupling: emotion-ref clip OR NL emotion instruction. License + finetune gaps. |
| Zonos-v0.1 (1.6B) | Apache-2.0 | runs on M1 (pure-PyTorch) | ✗ zero-shot only, no recipe | 8-emotion vector + rate/pitch dials. |
| Step-Audio-EditX (3B) | Apache-2.0 | ~6-7GB fp16 thin, no MLX yet | ✗ editing not speaker-adapt | instruct + iterative emotion EDITING of a take. |
| Chatterbox (Resemble) | **MIT** | ★ MLX ~1.3GB | community LoRA/full FT | **global** exaggeration knob (not per-span) + paralinguistic tags. Timbre-only clone. |
| Spark-TTS (0.5B) | CC-BY-NC-SA | MLX port | community LoRA | pitch/rate control, NOT affect/sarcasm. |
| ~~Higgs Audio v2/v3~~ | Community (capped) | ✗ **fails H2**: 5.8B, ~18-20GB VRAM, no Metal | marketed zero-shot | prompt-directed sarcasm — but won't fit 16GB. PRUNED. |

## Class 3 — Reference-clip prosody (supply an emotive sample per line)
| Model | License | M1 fit | Fine-tune | Notes |
|---|---|---|---|---|
| F5-TTS | MIT code (weights Emilia CC-BY-NC) | ★ native MLX ~4s/gen | ★ simple recipe | reference-driven; strong **host** for steering (EmoSteer). |
| Vevo (Amphion) | MIT | unknown | ✗ | style-ref + timbre-ref separation. |
| Llasa-3B | **CC-BY-NC-ND** (bad) | Q4 fits, MLX/GGUF | ★ LoRA | reproduces prompt-audio emotion. |
| OuteTTS-1B | CC-BY-NC-SA | ★ official GGUF/Metal | LoRA | timbre only, no emotion lever. |
| Dia-1.6B | Apache-2.0 | MLX ~4GB | ✗ | emergent + nonverbal tags only. |

## Class 4 — Adjacent / pipeline & control-layer (from lateral agents)
| Approach | License | Notes |
|---|---|---|
| **Two-stage: expressive base TTS → seed-vc / knn-vc timbre swap** | seed-vc GPL (shell out) / knn-vc MIT | direct successor to your current Kokoro+RVC. Stage-2 *preserves* base prosody, swaps timbre to the person. Only as expressive as the base. |
| EmoSteer-TTS | unknown (paper Aug'25, no code repo yet) | training-free activation-steering emotion LAYER over a cloned base (F5/CosyVoice). Per-token sarcasm dial without retraining. Highest-risk (no code). |
| VoiceCraft | CC-BY-NC | prosody editing via token infilling. |

## Pruned (fail a hard constraint)
- **Higgs Audio v2/v3** — fails H2 (5.8B, ~18-20GB VRAM, no quant/Metal path).
- **Parler-TTS, Bark, EmotiVoice** — can't clone a *specific target person* (described/preset voices, no fine-tune-to-person).
- **GLM-TTS** — flagged not-viable by scout (license/size).
