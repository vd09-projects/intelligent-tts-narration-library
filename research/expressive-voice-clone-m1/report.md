# Recommendation: no open model clones "sarcasm" — make it the clone's *default* via a persona fine-tune on Orpheus, add IndexTTS-2 Emo-Audio for on-demand modulation

**Version:** v1 · **Date:** 2026-07-25 · **Gear:** DEEP · **Intent:** PRIOR-ART + DECIDE
**Verification state:** 6 verified · 1 contested · 3 unsupported-and-cut · 6 pilots pending-approval (need your M1)
**Agents:** 19 (6 discovery · 4 fan-out · 6 verify · 3 reiterate) · within DEEP ceiling (20)

---

## 1. Recommendation (2–4 sentences)

Across ~20 open models, **no current TTS reliably captures or controls *sarcasm* as a dialable style** — it is an open research problem, and every candidate (StyleTTS2, Orpheus, Qwen3-TTS, CosyVoice, IndexTTS-2) demonstrates only generic emotion and/or speed. The winning move is a **reframe the field itself uses**: stop trying to *control* sarcasm per-line and instead **fine-tune on recordings of your person *being sarcastic* so a sarcastic+fast register becomes the clone's default** — this turns an unsolved control problem into a tractable data problem. Do that on **Orpheus TTS** (Llama-3B backbone, verified M1 fit at 2.36 GB Q4, ~$0 LoRA fine-tune), and if you later want *per-line* sarcasm on/off without retraining, layer **IndexTTS-2 Emo-Audio** (verified timbre/emotion disentanglement) on top — **pending a pilot to confirm it fits 16 GB, which is currently unverified.** This beats your rejected RVC (which copies source prosody) and the GPT-SoVITS baseline not by cloning *style* better, but by making sarcasm the speaker's baseline rather than a switch.

**You leaned toward "a model that learns from text where to be sarcastic."** That ideal (StyleTTS2's learned-from-text prosody) is the right *shape* but is **structurally biased against sarcasm** (it assumes tone matches text; sarcasm is tone contradicting text). So we recommend the persona-default reframe instead — same fine-tune effort, actually achievable.

---

## 2. Scorecard

**Weights** (proposed from your locked problem — *best expressive result* dominates, M1 is a hard floor, cost is minor since ≤$80 one-time & license irrelevant). **Adjust at Checkpoint 3.**

| Axis | Weight | Why |
|---|---|---|
| **fit** | 0.45 | expressive result + M1 + fine-tune-to-person is the whole ask |
| **limits** | 0.25 | blocking walls (16GB fit, no-sarcasm, forgetting) decide viability |
| **pilot** | 0.20 | how much is *verified* vs resting on an unrun synthesis |
| **cost** | 0.10 | all cheap; near-irrelevant |

| Model | fit | pilot | limits | cost | **total** | Note |
|---|---|---|---|---|---|---|
| **Orpheus TTS** | 4.0 | 2.5 | 3.5 | 5.0 | **3.68** | M1 fit + ~$0 FT *verified*; persona-sarcasm path; speed crude |
| **IndexTTS-2** | 3.5 | 2.0 | 2.0 | 5.0 | **2.98** | best sarcasm *mechanism* (disentangle verified); **16GB fit unverified**, speed disabled |
| **CosyVoice 2/3** | 3.0 | 2.0 | 2.5 | 3.5 | **2.73** | speed-instruct on zero-shot clone verified; FT causes catastrophic forgetting; Mac CPU-only |
| **StyleTTS2** | 2.5 | 2.0 | 2.5 | 4.0 | **2.55** | right mechanism, biased *against* sarcasm; no speed knob; tiny/fast |
| **Qwen3-TTS** | 2.5 | 1.5 | 2.0 | 4.5 | **2.38** | best MLX, but fine-tune ⊥ instruct-emotion (*verified disqualifier*) |

**Pick = Orpheus** as the single backbone (top total, only one with M1-fit + fine-tune both *verified*). **IndexTTS-2 = complementary layer**, not a competitor — high upside, gated on the 16 GB pilot. The recommendation is the **two-layer combination**, not either alone.

---

## 3. Frame (locked at Checkpoints 1–2)

- **[hard]** Clone a *specific person's* timbre + expressive delivery (sarcasm, speed, emotion).
- **[hard]** Inference on **M1 Pro / 16 GB**, local, offline.
- **[hard]** One-time fine-tune ≤ $80 on cloud GPU; **no recurring pay** (hosted APIs out).
- **[hard]** Model should ideally *learn* text→delivery mapping during fine-tune (self-learned preferred; in-text markers acceptable).
- **[soft]** Optimize best expressive result; setup effort secondary. License **irrelevant** (local-only, never redistributed).
- **Ruled out:** RVC (copies source prosody). **Baseline to beat:** GPT-SoVITS.

---

## 4. Reasoning — why sarcasm breaks the naive answer

**The universal finding:** sarcasm is carried by prosody that *contradicts* the words (flat pitch + specific loudness + timing — perception study arXiv 2606.09717). Every open TTS either (a) generates prosody to *match* text sentiment (StyleTTS2 — structurally anti-sarcasm), (b) exposes generic emotion labels that don't include sarcasm (CosyVoice's 8; IndexTTS-2's 8), or (c) offers instruct prompts where "be sarcastic" is an unproven string. Dedicated sarcasm-TTS research exists (arXiv 2510.07096, 2508.13028) but has **no open weights, non-cloning bases, no M1 story** — not reusable.

**Two things that *do* work, and compose:**

1. **Persona default (data, not control).** The field's actual method for sarcastic TTS is fine-tuning on a sarcastic corpus. Fine-tune your person's *sarcastic* recordings → the clone's baseline register is sarcastic+fast. You lose per-line switching (it's *always* sarcastic) but you gain a result that actually sounds sarcastic. Best host = **Orpheus** (cheapest + verified M1 fit; LLM backbone reads whole text so pace/emphasis emerge from the fine-tuned persona).

2. **On-demand modulation (reference clip, disentangled).** **IndexTTS-2** clones timbre from one clip and takes emotion/prosody from a *separate* clip, with a gradient-reversal layer that keeps the emotion clip's timbre out (verified). Feed a genuinely sarcastic reference clip → its sarcastic prosody rides onto your cloned voice, per line, no retraining. This is the clean version of "supply a sarcastic sample." **Caveat: its 16 GB fit is unverified and may need 32 GB — pilot first.**

**Speed is separable and solved:** sarcasm and speed are independent dimensions (arXiv 2606.09717), so "sarcastic *and* fast" is coherent. Speed comes free from the persona fine-tune's pace, or from CosyVoice's verified rate-instruct, or an ffmpeg `atempo` post-step.

---

## 5. Evidence & load-bearing claims (grade inline)

- **[verified]** Orpheus = Llama-3.2-3B; fine-tune ~300 samples via Unsloth LoRA on free Colab T4; Q4_K_M GGUF 2.36 GB runs on Apple-Silicon Metal (LM Studio) — fits 16 GB. — HF `canopylabs/orpheus-3b-0.1-pretrained` + Unsloth docs — *"base_model: meta-llama/Llama-3.2-3B-Instruct"*, *"best results, aim for 300 examples"* (blind-verified).
- **[verified]** Qwen3-TTS disqualifier: *"Only Base models support fine-tuning"*; Base *"does not support instruction-based emotion control… purely voice cloning."* Instruct lives only in CustomVoice on preset/designed voices. — QwenLM README + HF cards (blind-verified). ⇒ can't put instruct-emotion on your fine-tuned speaker.
- **[verified]** IndexTTS-2 disentanglement: GRL forces the emotion embedding to be timbre-invariant; timbre from timbre-prompt, emotion from a separate style-prompt; `emo_alpha` 0–1 blend. — arXiv 2506.21619v2 + repo README (blind-verified).
- **[verified]** IndexTTS-2 speed control **disabled**: *"This functionality is not yet enabled in this release."* — index-tts README (blind-verified).
- **[verified]** StyleTTS2 prosody is *generated from text* (CFG `embedding_scale`), not copied — the right mechanism, but its unsupervised assumption is *tone matches text* (anti-sarcasm). — StyleTTS2 demo notebook + discussion #181 (quote-pinned primary source).
- **[verified]** CosyVoice2 `inference_instruct2(tts_text, instruct_text, prompt_wav, …)` applies an instruction + clone prompt in one call (speed-instruct on a zero-shot clone works). — `cosyvoice/cli/cosyvoice.py` main (blind-verified).
- **[contested]** "Sarcasm absent from all four": a real **"sarcastic" instruct preset** exists (ComfyUI-Qwen3TTS-Emotional, 80+ presets incl. sarcastic/mocking; repo resolves 200) — but *quality undemonstrated* and only on Qwen3's non-fine-tunable instruct variant. Sarcasm-as-*label* exists; sarcasm-as-*reliable-result-on-your-clone* does not. — both sides attributed.
- **[unsupported — CUT]** "Qwen3 fine-tune flattens expressiveness / monotone" — the 'monotone' source was about *reference-audio* quality in zero-shot cloning, not fine-tune; one source says the opposite. Not used against Qwen3.
- **[unsupported — CUT]** "CosyVoice instruct survives single-speaker fine-tune" — papers use *multi-speaker* FT to avoid catastrophic forgetting; a guide reports forgetting after epoch 1. CosyVoice's strength is zero-shot-clone + instruct, **not** fine-tune + instruct.
- **[unsupported — RISK]** "IndexTTS-2 fits 16 GB" — no source confirms; MLX benched only on 128 GB M3 Max; a note says the MLX backend targets 32 GB+ and falls back on 16 GB. Support stack (gpt.pth 3.48 GB + s2mel 1.2 GB + w2v-bert + BigVGAN + Qwen-0.6B) is heavy. **Gates IndexTTS-2 on a pilot.**

---

## 6. Pilots (all synthesis is `pending-approval` — needs your M1 Pro)

| Solution | Pilot | Predicate | Status | Evidence / why not |
|---|---|---|---|---|
| Orpheus (feasibility) | backbone + FT recipe + GGUF size | verified from sources | **passed (doc)** | Llama-3.2-3B, 300-sample LoRA free Colab, Q4 2.36 GB — all confirmed |
| **Orpheus persona-FT** | fine-tune on sarcastic clips → sarcastic default clone, on M1 | audibly sarcastic + target timbre, runs <16 GB | **pending-approval** | needs ~300 sarcastic (text,audio) pairs + one cloud LoRA (~$0–few$) + local M1 run |
| **IndexTTS-2 Emo-Audio** | sarcastic ref clip → sarcasm onto cloned voice, no timbre bleed, <16 GB | timbre preserved + sarcasm audible + peak RAM <16 GB + tolerable RTF | **pending-approval** | ~2 GB fp16 + support weights (~5–7 GB); sweep `emo_alpha` {0.6,0.8,1.0}; **decisive for the 16 GB question** |
| CosyVoice speed | zero-shot clone + `inference_instruct2` "speak fast" | fast delivery on cloned voice | pending-approval | API verified; run on M1 CPU (~20 s/utt) to confirm |
| StyleTTS2 | FT on sarcastic person → does it hold sarcasm or average to neutral | sarcasm survives the tone-matches-text bias | pending-approval | mechanism predicts it averages out — worth one cheap test if you have data |

---

## 7. Contradictions left standing

- **Sarcasm capability:** "no open model does sarcasm" (papers, emotion taxonomies) **vs** "a sarcastic instruct preset ships" (Qwen3 community tooling). Axis: *label existence* vs *demonstrated quality on a cloned, fine-tuned voice*. Not averaged — both true at different definitions.

## 8. Open unknowns (what would settle them)

1. **Does a sarcastic reference clip actually transfer sarcasm onto the IndexTTS-2 clone without timbre bleed?** — inferred from verified disentanglement, never demonstrated for sarcasm. → the Emo-Audio pilot.
2. **Does IndexTTS-2 fit 16 GB on an M1 Pro?** — unverified, leans risky. → the pilot; if it needs 32 GB, IndexTTS-2 drops out and Orpheus persona-FT stands alone.
3. **Does a persona fine-tune on ~300 sarcastic clips yield convincing default sarcasm** (vs a flattened average)? — the field's method suggests yes; unproven for your data. → the Orpheus persona-FT pilot.

## 9. Follow-up — MVP / spike plan (smallest test of the hypothesis, not a build)

**Spike A — Orpheus persona-default (the primary bet):**
1. Collect 30–60 min (~300 clips) of the target person's *sarcastic/deadpan* delivery, transcribed.
2. One Unsloth LoRA fine-tune (free Colab T4, ~1–2 h, ~$0).
3. Pull Q4 GGUF to the M1, run in LM Studio (Metal). Generate neutral *and* sarcasm-friendly lines.
4. **PASS if** the clone sounds like the person *and* carries an audibly sarcastic register by default, under 16 GB.

**Spike B — IndexTTS-2 modulation (the upside, gated on 16 GB):**
1. `pip install` the `vanch007/mlx-indextts2-standard-fp16` port; download (~5–7 GB total).
2. `spk_audio_prompt` = person's clip; `emo_audio_prompt` = any genuinely sarcastic clip; sweep `emo_alpha` {0.6, 0.8, 1.0}; try 8-bit for speed.
3. **PASS if** timbre preserved + sarcasm audible + **peak RAM < 16 GB** + RTF tolerable offline.

Run A first (safe, ~$0). Run B only if you want per-line sarcasm control and it clears 16 GB. If B fits, the two compose into the full answer.

---

## 10. Sources (deduped)

- Orpheus: https://github.com/canopyai/Orpheus-TTS · https://huggingface.co/canopyai/orpheus-3b-0.1-pretrained
- Qwen3-TTS: https://github.com/QwenLM/Qwen3-TTS · https://huggingface.co/Qwen/Qwen3-TTS-12Hz-1.7B-CustomVoice · https://github.com/Dawizzer/ComfyUI-Qwen3TTS-Emotional
- IndexTTS-2: https://arxiv.org/abs/2506.21619 · https://github.com/index-tts/index-tts · https://index-tts.github.io/index-tts2.github.io/ · https://huggingface.co/vanch007/mlx-indextts2-standard-fp16
- CosyVoice: https://github.com/FunAudioLLM/CosyVoice · https://arxiv.org/abs/2412.10117 · https://funaudiollm.github.io/cosyvoice2/
- StyleTTS2: https://github.com/yl4579/StyleTTS2 · https://styletts2.github.io/ · https://github.com/yl4579/StyleTTS2/discussions/181
- Sarcasm research: https://arxiv.org/abs/2510.07096 · https://arxiv.org/abs/2508.13028 · https://arxiv.org/abs/2606.09717 (sarcasm prosodic cues; speed separable)
- Emotion layer (footnote): https://arxiv.org/abs/2508.03543 (EmoSteer-TTS — 6 basic emotions, no sarcasm)
