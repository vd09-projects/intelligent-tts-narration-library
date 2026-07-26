# Session Handoff — Expressive Voice Clone (Jeremy Jahns / "cool-jahns")

**Purpose of this doc:** everything needed to resume in a fresh session without
re-deriving research or re-discovering fixes. Read this file first; it supersedes
prose scattered across the prior conversation.

**Status as of last check: fine-tune NOT confirmed complete on either account.
No trained weights confirmed saved anywhere yet.** See §5.

---

## 1. Original question

> Is there a TTS/voice-clone model better than GPT-SoVITS/RVC that can capture a
> target speaker's **expressive delivery** — sarcasm, speaking speed, emotion —
> not just timbre? Must run inference on **M1 Pro / 16GB**. Fine-tune may cost a
> one-time ≤ $50–80 on a rented cloud GPU. No recurring/hosted-API cost.

This was run through the **huginn** research skill (DEEP gear), full pipeline:
problem-lock → discovery → fan-out+pilot → blind verify → rank/report.

---

## 2. Research pipeline — what was done, class by class

### Stage 0 — Problem lock
File: `research/expressive-voice-clone-m1/problem.md`

Locked constraints:
- **[hard]** Clone a *specific* person's timbre + expressive delivery.
- **[hard]** Inference on M1 Pro / 16GB, local, offline.
- **[hard]** One-time fine-tune ≤ $80 on rented cloud GPU; no recurring pay (rules
  out ElevenLabs/Play.ht/hosted APIs).
- **[hard]** Ideally the model *learns* text→delivery mapping during fine-tune
  (self-learned preferred over manual per-line tags).
- **[soft]** License irrelevant (local-only, never redistributed).
- Ruled out: **RVC** (voice conversion — copies source clip's prosody, can't
  inject a trained voice's own delivery). Baseline to beat: **GPT-SoVITS**.

### Stage 1 — Discovery (6 scouts: 4 reframes + 2 lateral)
File: `research/expressive-voice-clone-m1/candidates.md`

Surfaced ~20 real candidates across 4 classes:

- **Class 1 — learns emotion from text (the ideal):** StyleTTS2, Orpheus TTS,
  Sesame CSM, VibeVoice
- **Class 2 — explicit emotion control (instruct/vector/tags):** Qwen3-TTS,
  CosyVoice 2/3, IndexTTS-2, Zonos, Step-Audio-EditX, Chatterbox, Spark-TTS
- **Class 3 — reference-clip prosody transfer:** F5-TTS, Vevo, Llasa, OuteTTS, Dia
- **Class 4 — adjacent/pipeline approaches:** two-stage expressive-base→seed-vc
  (successor to the old Kokoro+RVC pattern), EmoSteer-TTS (activation-steering
  emotion layer), VoiceCraft (speech editing)

Pruned as hard-fails: **Higgs Audio v2/v3** (5.8B, ~18-20GB VRAM, no Metal/quant
path — busts the 16GB constraint), Parler-TTS/Bark/EmotiVoice (can't clone a
*specific* person — preset/described voices only), GLM-TTS.

**User's clarifying rule that shaped fan-out:** Class 2 models fine-tune on the
*target person's own audio* directly (no reference clip needed for timbre) — this
made Class 3 (reference-clip) unnecessary as a requirement, so fan-out was
trimmed to top 4 (all Class 1/2) rather than also including Class 3/4.

### Stage 2 — Fan-out + pilot (top 4, deep-researched + piloted)
Candidates researched in depth: **StyleTTS2, Orpheus TTS, Qwen3-TTS, CosyVoice
2/3**. Each doc resolved: M1/16GB inference fit, fine-tune-to-person feasibility
+ cost, and — the differentiator — whether it does SARCASM+SPEED specifically
(not just generic emotion).

### Stage 3 — Verify (completeness critic + blind adversarial refuters)
Verify **materially changed the answer** (this is the point of the stage):
- **Qwen3-TTS disqualified**: verified — official docs state *"Only Base models
  support fine-tuning"* and Base *"does not support instruction-based emotion
  control."* Fine-tune and instruct-emotion live on different, non-combinable
  model variants. Can't have both.
- **CosyVoice's advantage weakened**: `inference_instruct2` (instruct + clone in
  one call) verified at **zero-shot** inference, but "survives fine-tuning" was
  **unsupported** — evidence of *catastrophic forgetting* after fine-tuning
  found instead. So CosyVoice's real strength is zero-shot-clone + instruct, not
  fine-tune + instruct.
- **Sarcasm universal-negative → contested**: a real "sarcastic" instruct preset
  was found (community Qwen3 tooling, 80+ emotion presets) — so sarcasm exists
  as an instruct *label* somewhere, but quality/reliability on a *fine-tuned
  clone* remains undemonstrated.
- **Completeness critic surfaced 2 real misses**, triggering one reiteration
  round:
  - **IndexTTS-2** was wrongly excluded from fan-out — it has genuine
    timbre/emotion **disentanglement** (gradient-reversal-layer training; a
    *separate* emotion-reference clip drives prosody while a *different* clip
    supplies timbre, verified via arXiv 2506.21619 + repo). Caveats: zero-shot
    only (no official fine-tune path), speed control shipped-but-disabled in
    current release, and its **16GB fit is unverified** (only benchmarked on a
    128GB M3 Max; a community note suggests the MLX backend targets 32GB+).
  - **Persona-default reframe**: the field's actual method for sarcastic TTS is
    to fine-tune on a corpus that is *itself* sarcastic, so sarcasm becomes the
    clone's default register — turning an unsolved per-line *control* problem
    into a tractable *data* problem.

### Stage 4/5 — Rank + Report
File: `research/expressive-voice-clone-m1/report.md` (v1, dated 2026-07-25)

**Scorecard** (weights: fit .45 · limits .25 · pilot .20 · cost .10):

| Model | Total | Verdict |
|---|---|---|
| **Orpheus TTS** | **3.68** (top) | M1 fit + ~$0 fine-tune both *verified*; best backbone |
| IndexTTS-2 | 2.98 | best sarcasm *mechanism*, but 16GB fit unverified, speed disabled |
| CosyVoice 2/3 | 2.73 | speed-instruct verified on zero-shot clone; fine-tune → forgetting |
| StyleTTS2 | 2.55 | right "learn-from-text" mechanism but structurally *biased against* sarcasm (assumes tone matches text; sarcasm = tone contradicts text) |
| Qwen3-TTS | 2.38 | fine-tune ⊥ instruct-emotion (verified disqualifier) |

**Final recommendation:** no open model reliably does sarcasm as a controllable
style. The winning strategy is **two-layer**:
1. **Persona-default fine-tune on Orpheus** — fine-tune on the target person's
   *sarcastic* recordings so sarcasm becomes the clone's baseline register
   (verified: M1 fit at 2.36GB Q4 GGUF; ~$0 LoRA fine-tune on free Colab T4;
   ~300 samples).
2. **Optional on-demand modulation via IndexTTS-2 Emo-Audio** — feed a separate
   sarcastic reference clip to ride sarcastic prosody onto the cloned timbre
   without retraining (gated on confirming it fits 16GB — untested).

**However:** the user redirected from "trust the research" to "prove it
empirically" — hence the actual fine-tuning attempts below, on **GPT-SoVITS**
(not Orpheus) as the first empirical test, because the `cool-jahns` dataset was
already prepared for GPT-SoVITS specifically (see `assets/.../DATASET.md`).
Orpheus fine-tune has **not** been attempted yet — it remains the
research-backed pick once GPT-SoVITS proves out (or doesn't) the empirical
question "does tone/delivery survive cloning at all."

---

## 3. Data — exact paths

### Source (canonical, lives in the repo, in git)
```
assets/voice-clips/male/cool-jahns/
├── DATASET.md                        <- describes the dataset, edits, method
├── 01-death-of-robin-hood.wav  (27M)
├── 02-disclosure-day.wav       (32M)
├── 03-am-i-racist.wav          (24M)
├── 04-captain-america-bnw.wav  (47M)
├── 05-dune-part-2.wav          (47M)
├── 06-joker-folie-a-deux.wav   (33M)
├── 07-la-la-land.wav           (17M)
├── 08-the-drama.wav            (40M)
└── 09-the-odyssey.wav          (73M)
```
Speaker: Jeremy Jahns (YouTube movie reviewer). Single-speaker, English,
sponsor-reads/end-cards already cut. Format: WAV mono 48kHz PCM s16le.
**Content note:** general movie-review commentary (animated, opinionated, some
sarcasm mixed with earnest hype) — **not** a sarcasm-curated corpus. First
fine-tune tests whether his general expressive register survives cloning at
all; a sarcasm-weighted subset would be a follow-up if this run flattens.

### Locally prepped (session scratchpad — EPHEMERAL, may not exist in a new session)
```
/private/tmp/claude-501/-Users-vikrantdhawan-repos-TTS-intelligent-tts-narration-library-manual/e0f94dd5-a731-4182-aecd-328144594523/scratchpad/jahns-prep/
├── wav24k/                    <- 9 files resampled to 24kHz mono (ffmpeg)
├── segments/                  <- 408 pre-sliced 3-10s clips (avg 7.3s)
├── manifest.json              <- segment metadata (file, src, start, len)
├── jahns-full-24k.zip  (145M)  <- wav24k/*.wav zipped, for GPT-SoVITS (self-slices)
└── jahns-segments-24k.zip (128M) <- segments/*.wav zipped, for Orpheus (needs pre-cut clips)
```
**Empirically verified sufficiency:** 59.1 min raw → 408 segments, 49.7 min
usable after pause-trim. This is well past every model's stated floor
(StyleTTS2 ~1hr, Orpheus ~300 samples, GPT-SoVITS a few min). **No more source
data is needed** for a first run on any of the 4 fan-out candidates.

If this scratchpad is gone in a new session: re-run the same ffmpeg
resample+silence-slice recipe against `assets/voice-clips/male/cool-jahns/` (24kHz
mono, 3-10s windows, -32dB silence threshold) — described in
`research/expressive-voice-clone-m1/finetune/README.md`.

### Google Drive — Account 1 (original account used for research/first attempt)
Folder: `finetune-data-repo/TTS/` (path as mounted: `/content/drive/MyDrive/finetune-data-repo/TTS/`)
- `jahns-full-24k.zip`, `jahns-segments-24k.zip` — uploaded manually by user
- `gpt_sovits_finetune_colab.ipynb` — was replaced with the hardened version partway through (see §4)
- `orpheus_finetune_colab.ipynb` — uploaded, never run

**Fine-tune attempted here first. VM recycled overnight before weights were
saved — weights from this account's run are LOST.** (`/content` wiped, fresh
`sample_data` only, ~10hr idle gap detected via container-ID + clock jump.)
Also: free-tier GPU became unavailable on this account afterward ("GPU
assignment failure").

### Google Drive — Account 2 (`vdhawan.projects`, paid-compute-willing account)
Colab URL pattern: `https://colab.research.google.com/?authuser=3`
Drive folder: `https://drive.google.com/drive/u/3/folders/1jCvk8jI3iWhsgQfTtH1DXU27_4Bv9Te3`
(same relative path: `finetune-data-repo/TTS/`)

Contents confirmed present:
- `jahns-full-24k.zip` (135.2 MB), `jahns-segments-24k.zip` (113.8 MB) — same
  data as account 1, re-uploaded/copied here
- `gpt_sovits_finetune_colab.ipynb` — **hardened notebook** (user replaced the
  old 5KB original with the debugged ~790KB version mid-session; see §4 for
  what's baked into it)
- `orpheus_finetune_colab.ipynb` — present, unrun

**Target save location for trained weights (not yet confirmed populated):**
`/content/drive/MyDrive/finetune-data-repo/TTS/cool-jahns-weights/`
— expected to contain a `.pth` (SoVITS) and a `.ckpt` (GPT), each tens-to-low-hundreds of MB.

**Open item — unresolved, needs user input:** user mentioned a **different**
Drive folder, `https://drive.google.com/drive/.../folders/1nbDvSBbRdUCS9FzOELONZ6IzDaQg8xbv`,
described as containing "v2pro files," with an ask to "download + create auto on
text locally." This was never clarified or acted on — **get context on this
before assuming what it is.**

### Notebook files (in the git repo, in THIS conversation's working directory)
```
research/expressive-voice-clone-m1/finetune/
├── README.md                          <- data sufficiency + run-order notes
├── gpt_sovits_finetune_colab.ipynb     <- HARDENED version (see §4). Also copied
│                                          to Drive (both accounts) and used to
│                                          drive the actual training runs.
└── orpheus_finetune_colab.ipynb        <- ready, never run
```

---

## 4. Fine-tune attempt log — what was actually done, and the fixes discovered

### Attempt 1 (Account 1) — GPT-SoVITS, lost to VM recycling

Ran the full pipeline successfully end-to-end through training:
1. Cloned `RVC-Boss/GPT-SoVITS`, installed requirements.
2. Downloaded pretrained models (`lj1995/GPT-SoVITS` snapshot, ~5GB).
3. Got data onto the VM via `gdown` (direct Drive file-ID download, since
   `drive.mount()` OAuth kept failing under browser automation — see below) —
   note account-2 approach instead unzips straight from a *mounted* Drive since
   mount worked there.
4. Ran WebUI (`webui.py`, `is_share=True` for a public gradio link).
5. **Hit 7 missing-dependency crashes in sequence**, each fixed by installing
   and re-running the failed step:
   `ffmpeg-python` → `wordsegment` → `g2p_en` (+ NLTK data:
   `averaged_perceptron_tagger_eng`, `averaged_perceptron_tagger`, `cmudict`,
   `punkt`, `punkt_tab`) → `x_transformers` → `pytorch_lightning` →
   `fast_langdetect` → `split_lang`.
6. Slicing (CPU threads must be **2**, not default 4 — Colab free tier has 2
   vCPUs and errors on the default) → 427 segments from the 9 files.
7. ASR (Faster-Whisper, `large-v3-turbo`, language `en`) → 427/427 labeled.
8. Dataset formatting (experiment name `cool-jahns`) → 1Aa/1Ab/1Ac all finished.
9. **SoVITS training**: batch size **4** (not default 7 — OOM risk on T4),
   8 epochs → finished clean.
10. **GPT training**: batch size 4, 15 epochs → finished clean (top-3 acc ~0.62).
11. Inference test attempted: hit `RuntimeError: Failed to create AudioDecoder
    for None` — **v2Pro requires a reference audio clip even in "no reference
    text" mode** (no-ref mode only skips the ref *text*, not the ref *audio*).
    A 3-10s clip from the person is still mandatory for timbre.
12. **Before a reference clip could be supplied and weights saved, the Colab VM
    recycled** (idled ~10hrs overnight). `/content` wiped. **Both trained
    weight files (`cool-jahns_e8_s904.pth`, `cool-jahns-e15.ckpt`) are gone,
    unrecoverable.**

**Root cause of the loss:** weights were never copied off the ephemeral VM.
`drive.mount()` was attempted post-hoc to save them but failed 3× with
`MessageError: credential propagation was unsuccessful` — an OAuth/cookie
handshake issue specific to driving the mount flow via browser automation.
Manual user clicks on the *same* consent dialog also failed once. A
`files.download()` test (push straight to the user's Downloads, no Drive/OAuth
involved) was in progress as an alternative save path when the browser session
itself froze/disconnected.

**Verified (by the user, manually, outside automation): a Drive-mounted
notebook CAN write a test file successfully** when the user completes the
`drive.mount()` OAuth click themselves — the failure is specific to automation
completing that click, not Drive access in general.

### Hardened notebook produced (in response to the above)

`research/expressive-voice-clone-m1/finetune/gpt_sovits_finetune_colab.ipynb`
was rewritten to bake in every fix from Attempt 1:
- All missing deps pre-installed in the install cell (no crash-loop):
  `ffmpeg-python wordsegment g2p_en x_transformers pytorch_lightning
  fast_langdetect split_lang cn2an pypinyin jieba jieba_fast` + the NLTK
  downloads.
- Data loads straight from mounted Drive (`/content/drive/MyDrive/finetune-data-repo/TTS/jahns-full-24k.zip`)
  instead of `gdown`.
- **A dedicated "save weights to Drive" cell (cell 7)** — copies both
  `SoVITS_weights*/*cool-jahns*.pth` and `GPT_weights*/*cool-jahns*.ckpt` into
  `.../TTS/cool-jahns-weights/` — meant to be run **immediately** when training
  finishes, before anything else, specifically to prevent the Attempt-1 loss
  from recurring.
- Inline notes: slicer CPU threads = 2, training batch size = 4.

This is the version the user then uploaded to replace the old notebook in
**both** Drive accounts (account 1's copy and account 2's copy).

### Attempt 2 (Account 2, `vdhawan.projects`) — in progress, NOT confirmed complete

Account 2 was chosen because the user is willing to pay for compute there if
needed, and it has its own copy of the same data + hardened notebook.

Setup was redone **via the Colab terminal** (not notebook cells) for
reliability, hitting two *new* dependency issues not seen in Attempt 1:

1. **`opencc` wheel build failure** — the real `opencc` package fails to
   compile from source on this Colab image (`Building wheel for opencc
   (pyproject.toml) ... error`). **Fix:** install `opencc-python-reimplemented`
   (pure-Python, same import name/API) instead, and filter `opencc` out of
   `requirements.txt` before installing the rest:
   ```bash
   pip install -q opencc-python-reimplemented
   grep -v -i '^opencc' requirements.txt > /tmp/reqs.txt
   pip install -q -r /tmp/reqs.txt
   ```
2. **Gradio 4.44.1 template-engine crash** — after installing all the extra
   deps together, the WebUI returned `Internal Server Error` on every load,
   with `TypeError: unhashable type: 'dict'` in Starlette/Jinja2's template
   cache. Root cause: the batch dep-install pulled in a too-new
   `fastapi`/`starlette` incompatible with gradio 4.44. **Fix:** pin explicitly
   and relaunch:
   ```bash
   pip install -q 'fastapi==0.112.4' 'starlette==0.38.6'
   pkill -f webui.py
   cd /content/GPT-SoVITS && is_share=True PYTHONUNBUFFERED=1 nohup python -u webui.py > /content/webui.log 2>&1 &
   grep -o 'https://[a-z0-9]*\.gradio.live' /content/webui.log   # get the new link
   ```
   (Launch with `python -u` / `PYTHONUNBUFFERED=1` — plain `nohup python
   webui.py` buffers stdout and the gradio.live link never appears in the log
   until the process exits.)

**Confirmed progress on Account 2 before visibility was lost:**
- T4 GPU connected, stale sessions cleared ("too many sessions" error resolved
  via Colab's session manager).
- Drive mounted successfully (`Mounted at /content/drive`) — this account's
  mount does NOT hit the credential-propagation error Account 1 did.
- All deps installed clean (opencc fix applied).
- Pretrained models downloaded, data unzipped from Drive into `/content/jahns`.
- WebUI launched, gradio-bug fixed, page renders.
- Speech Slicing: done (CPU threads=2).
- ASR: done, 427/427 segments (Faster Whisper large-v3-turbo, language=`auto`
  — correctly auto-detects 'en' per segment; this is fine to use, doesn't need
  to be forced to `en` explicitly).
- Dataset formatting (`cool-jahns` experiment name): 1Aa/1Ab/1Ac all confirmed
  "Finished", no errors.
- SoVITS training: started clean (batch=4, 8 epochs — pretrained G/D loaded,
  "All keys matched successfully", epoch 1 begun, no OOM).

**NOT confirmed:**
- Whether SoVITS training actually reached "Finished".
- Whether GPT training was ever started.
- Whether the weight-save-to-Drive step ran.
- Chrome browser extension disconnected partway through (reason: the user took
  manual control of the Chrome tab directly at some point — "I already
  triggered run myself" — and separately the automated session hit an API
  usage-limit pause). **The disconnection means nothing after "SoVITS training
  started clean" is verified. Do not assume it finished.**

### Orpheus path — not attempted at all
Notebook exists (`orpheus_finetune_colab.ipynb`, both locally and in both Drive
accounts) but has never been run. Per the research report, Orpheus is the
top-scoring candidate (3.68) and the intended primary bet — GPT-SoVITS was run
first only because the data was already prepped for it and it gave the fastest
empirical readout on "does his delivery survive cloning at all."

---

## 5. First thing to check in a new session

**Before doing anything else**, verify current real state — don't trust this
document's "in progress" claims blindly, they were last confirmed before a
disconnect:

```bash
# On Account 2 Colab terminal, if the VM is still alive:
ls -la /content/drive/MyDrive/finetune-data-repo/TTS/cool-jahns-weights/ 2>&1
ls -la /content/GPT-SoVITS/SoVITS_weights_v2Pro/ /content/GPT-SoVITS/GPT_weights_v2Pro/ 2>&1
```

- If `cool-jahns-weights/` has both a real-sized `.pth` and `.ckpt` → **done**,
  proceed to inference testing (needs a 3-10s reference clip of the target
  speaker — one can be cut server-side from `/content/jahns/*.wav` via ffmpeg,
  no upload needed).
- If the VM is gone / weights absent → training must be redone. Everything in
  §4's "hardened notebook" + the two new Attempt-2 fixes (opencc,
  fastapi/starlette pin) should make the redo fast and mostly hands-off; expect
  ~30-40 min of actual compute (mostly waiting).
- Either way: **run the Drive-save step the instant training finishes**, before
  doing anything else, before testing inference, before idling the tab. This
  is the one lesson this whole log exists to capture.

## 6. Open items needing user input

1. What's in Drive folder `1nbDvSBbRdUCS9FzOELONZ6IzDaQg8xbv` ("v2pro files"),
   and what does "create auto on text locally" mean as a follow-up ask?
2. Does account 1 still have free-tier GPU access, or is it still blocked
   ("GPU assignment failure")? Determines whether it's usable as a fallback.
3. Orpheus fine-tune — run it after GPT-SoVITS succeeds (for comparison), or
   only if GPT-SoVITS's expressive result disappoints?
4. If GPT-SoVITS's cloned Jahns voice sounds flat/non-sarcastic on inference —
   per the research report's §9 MVP plan, next step is either (a) a
   sarcasm-curated data subset, or (b) layering IndexTTS-2 Emo-Audio on top
   (pending its own 16GB-fit verification). Revisit `report.md` §9 for the
   exact spike plan.
