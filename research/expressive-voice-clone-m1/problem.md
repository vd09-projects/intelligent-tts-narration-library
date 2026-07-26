# Problem-lock — Expressive voice-clone model (better than GPT-SoVITS / RVC)

**Gear:** DEEP · **Intent:** PRIOR-ART (survey field for a reusable voice box) + DECIDE (vs GPT-SoVITS baseline)
**Ceiling:** ≤20 agents · ≤8 pilots · ≤3 discovery rounds

## Restatement

Find a TTS/voice-clone model that clones a **specific real person's** voice *and*
their **expressive delivery** (sarcasm, speaking speed, emotional emphasis), where
the model **learns from fine-tune data when/where to apply that delivery from the
text itself** — not just timbre, and not by copying prosody from a source clip.
Must run inference on an **M1 Pro / 16 GB RAM** machine. Optimize for **best
expressive result**; setup effort is secondary. Beats RVC (copies source prosody,
can't inject style) and GPT-SoVITS (generates prosody but sarcasm intent not
automatic).

## Intent

Primary **PRIOR-ART** — is there an existing model to reuse. Secondary **DECIDE** —
if yes, does it beat the GPT-SoVITS baseline for this job.

## Frame — KNOWNS / UNKNOWNS / ASSUMPTIONS

### KNOWNS
| Fact | Tag |
|---|---|
| Must clone a **specific person's** timbre + expressive style (have their recordings) | **[hard]** |
| Inference must run on **M1 Pro / 16 GB RAM**, local, offline | **[hard]** |
| Model should **learn text→delivery mapping during fine-tune** (understands where to be sarcastic/emotional), optionally aided by in-text markers the user supplies | **[hard]** |
| Optimize for **best expressive result**; setup/data effort secondary | **[soft]** |
| Prefer permissive license (Apache/MIT); local-only hobby project | **[soft]** |
| Voice box slots behind a Go library (subprocess acceptable); Kokoro is current default | **[soft]** |
| RVC rejected — copies source-clip prosody, can't inject trained voice's delivery | **[hard]** (ruled out) |
| GPT-SoVITS is the baseline to beat, not a floor | **[soft]** |

### UNKNOWNS (drive discovery)
1. Which clone-capable TTS models learn **speaker-specific expressive style** (not just timbre) via fine-tune?
2. Of those, which can **place emphasis/sarcasm/emotion conditioned on text** (learned or via in-text control tokens)?
3. Which fit **M1 Pro / 16 GB inference** (params, quant, RAM, Apple Silicon / Metal / MLX support)?
4. Training-data volume + labeling needed to capture the style (secondary, but a viability floor).
5. License of each.

### RESOLVED at Checkpoint 1
- **A1 → confirmed:** Fine-tune on cloud/rented GPU is OK as a **one-time** spend. Only *inference* must fit M1/16 GB.
- **A2 → confirmed:** Text-driven sarcasm/emotion placement is a **ranked capability, not a gate**. Both work: (a) in-text markers the user supplies, and (b) model learns it itself. **Prefer self-learned if it gives the best/most-efficient result.**
- **NEW [hard] — Budget:** **One-time spend ≤ $50–$80 total** (GPU fine-tune rental). **No recurring payments** → rules out subscription/hosted TTS APIs (ElevenLabs, Play.ht, hosted GPT-SoVITS, etc.). Confirms: open-source model + at most one cloud fine-tune job under budget.

### STANDING ASSUMPTIONS
- **A3:** Batch/offline synthesis is fine (narration library plans whole input); no real-time latency requirement.
- **A4:** English-only phase-one holds.
