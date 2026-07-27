# GPT-SoVITS Voices — zero-shot TTS clone artifacts (backup + DR runbook)

GPT-SoVITS (GSO) speaks the words directly from a `.ckpt`/`.pth` checkpoint pair +
a short reference clip — no separate TTS front-end, and **no ONNX export** (a
deliberate difference from the RVC path, which converts an existing Kokoro render).
Local inference on Apple Silicon via the torch subprocess worker
`scripts/gptsovits_worker.py` (driven by `scripts/gso`).

> This README is the ONE tracked file under `assets/gptsovits-models/` — a deliberate
> improvement over the RVC dir, whose README is untracked. Everything else here
> (weights, reference audio, the `_base/` set) is gitignored: large + non-redistributable.
> Because the wholesale `assets` ignore excludes the parent dir, git cannot re-include
> this file by a bare negation alone — `.gitignore` re-includes the parent-dir chain
> (see the `#161` block there), and a fresh checkout still needs `git add -f` if the
> negation is ever narrowed.

## Voices
| Voice | Base | Source | Sample rate | text_split_method |
|---|---|---|---|---|
| **cool-jahns-gso** | GPT-SoVITS v2Pro | Jeremy Jahns v2Pro clone (same non-redistributable real person as RVC `cool-jahns`) | **32 kHz** | `cut4` |

32 kHz is a **third** system output rate (Kokoro 24 kHz, RVC 40 kHz, GSO 32 kHz).
CLAUDE.md forbids resampling, so the roster/format metadata #162/#163 defines MUST
carry a per-voice/per-engine sample rate — 32 kHz cannot be assumed away.

## Artifact map

Per-voice dir — the four files the worker resolves from `<voice_id>`:
```
cool-jahns-gso/gpt.ckpt            t2s (GPT) weights (.ckpt) — consumed DIRECTLY, no ONNX
cool-jahns-gso/sovits.pth          vits (SoVITS) weights (.pth) — consumed DIRECTLY, no ONNX
cool-jahns-gso/ref_audio.wav       reference clip (source-of-record / DR backup)
cool-jahns-gso/ref_transcript.txt  exact transcript of ref_audio.wav (source-of-record)
```

Shared base-model set (voice-independent), fetched by `make gso-fetch-base` (~950MB):
```
_base/chinese-hubert-base/                        content encoder   (~189 MB)
_base/chinese-roberta-wwm-ext-large/              text BERT         (~651 MB)
_base/sv/pretrained_eres2netv2w24s4ep4.ckpt       v2Pro SV embed    (~108 MB)
```

**Wire-vs-packaged precedence (the worker's drift rule).** The wire
`<ref_audio_path>` + `<prompt_text>` tokens are **authoritative** for what is fed to
inference. The packaged `ref_audio.wav` + `ref_transcript.txt` are the
**source-of-record / DR backup** and the canonical values #162 SHOULD emit on the
wire. The worker maps drift onto the closed ERR taxonomy: a ref path inside a
*different* voice's dir → `bad-voice`; a ref path outside this voice's dir (when a
packaged ref exists) or a `prompt_text` that mismatches the packaged transcript on a
best-effort compare → `bad-args`. The transcript compare is a **coarse anti-drift
guardrail, not transcript enforcement** — the wire value still wins.

## Prerequisites

- **Homebrew ffmpeg on PATH** — `torchaudio.load()` needs `torchcodec`, which needs
  ffmpeg (`brew install ffmpeg`). Environmental prereq (gotcha), not a pip dep.
- **Python 3.10–3.12 only** (never system 3.14) — `funasr`/`pyopenjtalk`/`jieba_fast`
  target 3.10–3.12. The venv is built from `$(PYTHON311)`.
- **GPT-SoVITS code clone on disk** — the worker imports `GPT_SoVITS.*` from an
  EXTERNAL clone (never vendored here; subprocess/licensing boundary), resolved via env
  `GSO_REPO`, default `~/repos/GPT-SoVITS-local` (`git clone
  https://github.com/RVC-Boss/GPT-SoVITS`). Only the CODE comes from the clone; all
  WEIGHTS come from this dir. See `docs/gpt-sovits-inference-runbook.md`.

### Machine-run findings (#165 — how warm load actually resolves the base models)
The M1-Pro acceptance run surfaced that the v2Pro config keys the runbook passes are
partly inert and are masked only by its `os.chdir(REPO)`; the worker now handles all of
this explicitly (see `scripts/gptsovits_worker.py::_GsoPipeline._build`):
- **chdir + sys.path is load-bearing** — `GPT_SoVITS/sv.py` hardcodes CWD-relative
  paths (`sys.path.append(f"{os.getcwd()}/GPT_SoVITS/eres2net")` + a module-global
  `sv_path`), so the worker `os.chdir(GSO_REPO)` + inserts it on `sys.path` before
  importing GPT-SoVITS.
- **`cnhubert_base_path` is the wrong key** — TTS.py reads `cnhuhbert_base_path` (extra
  `h`); the worker uses the correct spelling so CNHuBERT loads from `_base/`.
- **TTS_Config `sv_path` is INERT** — v2Pro's `SV(device, is_half)` takes no path arg;
  the SV embedding loads from `sv.py`'s module-global `sv_path`. The worker overrides
  `sv.sv_path` to `_base/sv/pretrained_eres2netv2w24s4ep4.ckpt` so the fetched embedding
  is used deterministically.
- **Fixed inference seed** — GPT-SoVITS randomizes the seed when none is passed; the
  worker pins `seed=42` + the runbook sampling params so warm output is reproducible.
- **stdout is a noise sink** — GPT-SoVITS prints copiously to stdout; the worker
  preserves the real stdout for the OK/ERR wire and shunts fd 1 → stderr.
- **Offline-provable** — `scripts/gso` pins `HF_HUB_OFFLINE=1` + `TRANSFORMERS_OFFLINE=1`;
  warm load succeeds with an empty HF cache (nothing is fetched).

## Setup
```bash
make gso-fetch-base       # fetch the ~950MB shared base models into _base/ (idempotent)
make gso-worker-venv      # build .venv-gso (python3.11) from scripts/gso-requirements.txt
                          #   — asserts torch PRESENT (inverse of the RVC worker) + prints freeze
```
Then place the two `cool-jahns-gso` checkpoints + the reference clip/transcript in
`cool-jahns-gso/` (regenerate from training, or restore from HF — below).

## Restore after a machine wipe
```bash
# whole voice tree (weights + ref) — restores everything under assets/gptsovits-models/
hf download vd09-projects/gptsovits-voices --repo-type model --local-dir assets/gptsovits-models

# just the shared base set (or use `make gso-fetch-base`, which pulls the same files)
hf download lj1995/GPT-SoVITS \
  --include "chinese-hubert-base/*" "chinese-roberta-wwm-ext-large/*" "sv/*" \
  --repo-type model --local-dir assets/gptsovits-models/_base
```

## Push a new / updated voice (backup)
`hf` lives in a torch venv (not global PATH). A write token must be logged in
(`hf auth login --token hf_...`).
```bash
HF=/Users/vikrantdhawan/repos/TTS/Applio/.venv/bin/hf   # or wherever hf is installed

# one voice folder (weights + ref) — format: hf upload <repo> <local-folder> <path-in-repo>
$HF upload vd09-projects/gptsovits-voices assets/gptsovits-models/cool-jahns-gso cool-jahns-gso \
   --repo-type model --commit-message "Add cool-jahns-gso"

# sync everything, incl. this README (LFS is automatic for the large .ckpt/.pth/.wav)
$HF upload vd09-projects/gptsovits-voices assets/gptsovits-models . \
   --repo-type model --commit-message "Sync GSO voices"
```

## Regenerate vs restore
- **Base models (`_base/`)** — always `make gso-fetch-base` (public, deterministic).
  No need to back these up to the private voices repo.
- **Voice checkpoints (`gpt.ckpt`/`sovits.pth`)** — training outputs; restore from the
  `vd09-projects/gptsovits-voices` HF repo (retraining is expensive; prefer restore).
- **Reference clip/transcript** — small; ride the voice folder backup.

## Use
```bash
make gso-warmproof                       # AC5 warm-load correctness smoke (needs venv + real artifacts)
scripts/gso                              # raw worker: feed the stdin wire on your own
```
Wire protocol + failure modes: the module docstring at
`scripts/gptsovits_worker.py`. Cross-language shlex contract for the Go renderer (#162):
`scripts/testdata/gso-shlex-golden.json`.

## License (the subprocess boundary is load-bearing)

GPT-SoVITS itself is **MIT**, but its transitive dependency set — `funasr`,
`pyopenjtalk`, `jieba_fast`, and others — carries **mixed and in places
restrictive/copyleft-adjacent** licenses. The **subprocess boundary is what keeps all
of it out of the Go binary**: torch and the GPT-SoVITS stack are only ever *spoken to*
over stdin/stdout across a process boundary, never imported or linked. This is the
same reasoning as the Piper-GPL gotcha in `CLAUDE.md` (and the RVC worker's LGPL
native libs) — a **load-bearing licensing reason for the subprocess architecture**,
not just a packaging convenience.

**Voice-model provenance:** `cool-jahns-gso` is a clone of a real public figure with
**no redistribution license on record** (the same person as RVC `cool-jahns`, D0 not
cleared). Weights + reference audio are **never committed** and **never published**;
verify by ear locally only. See the standing order in the RVC dir + `#151`.
