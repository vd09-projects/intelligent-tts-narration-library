#!/usr/bin/env python3
"""GPT-SoVITS (GSO) warm-load-once TTS worker (issue #161, P1 of the GSO chain).

Ephemeral per-job worker. Loads ONE `TTS_infer_pack` pipeline object per voice_id
the first time that voice appears (module-lived, warm across every request), then
services a stdin/stdout line loop, synthesizing one WAV per line until EOF, then
exits -> 0 idle RAM. This is the shared inference engine the future Go renderer
(#162) will drive as a subprocess; this file owns only the Python worker. Invoked
via `scripts/gso` (which activates .venv-gso). Consumes `.ckpt`/`.pth` DIRECTLY —
there is NO ONNX export step (a deliberate difference from the RVC worker).

Inverse of the RVC worker's torch-free invariant: this worker REQUIRES torch. The
torch import + the TTS pipeline load are LAZY and guarded (see `_assert_torch_present`
+ `_GsoPipeline`), so the arg-parsing, ERR taxonomy, minted-path derivation, drift
validation, and shlex framing are all exercisable WITHOUT torch installed — that is
what the Phase-4 torch-free contract test (`scripts/gso_worker_contract_test.py`)
depends on. Nothing above module scope imports torch, numpy, or GPT-SoVITS.

═══════════════════════════════════════════════════════════════════════════════
WIRE PROTOCOL (v1) — the load-bearing public surface #162 binds to
═══════════════════════════════════════════════════════════════════════════════
RVC-SHAPED, not RVC-verbatim. Reuses the RVC v1 SHAPE (decision
2026-07-22-rvc-worker-wire-contract-err-taxonomy-exit-codes): stdin line loop,
positional append-only tokens, exactly one response line per request, a CLOSED ERR
taxonomy, and a startup/runtime FATAL exit split. It DELIBERATELY differs from RVC
in three GSO-specific ways: (1) the output path is WORKER-MINTED (RVC's request
carried a caller-supplied <out>; GSO's line has none); (2) <text>/<prompt_text> are
free-text with spaces + punctuation, so shlex-quoting is LOAD-BEARING (RVC's tokens
were all space-free); (3) the engine emits 32 kHz (a third system rate; no resample).

Read stdin line by line. Each line is shlex-split into EXACTLY 5 positional tokens:

    <text> <ref_audio_path> <prompt_text> <text_split_method> <voice_id>

  <text>              target text to speak (final spoken words; voicing already done
                      Go-side). Free-text; MUST be shlex-quoted by the caller.
  <ref_audio_path>    path to the voice's reference clip. MANDATORY (gotcha 5).
                      empty -> bad-args; non-empty but unreadable -> read-failed.
  <prompt_text>       exact transcript of the ref clip. MANDATORY, never empty
                      (gotcha 6 — no ref-free mode). empty -> bad-args. Free-text.
  <text_split_method> phase one accepts ONLY `cut4` (gotcha 7). anything else ->
                      bad-args. The token exists for forward-compat.
  <voice_id>          roster slug; phase one only `cool-jahns-gso`. unknown /
                      malformed -> bad-voice.

SHLEX-QUOTING IS LOAD-BEARING (call this out to #162): <text> and <prompt_text> are
natural-language strings with spaces and punctuation, so the Go caller MUST
shlex-quote each positional token when emitting a line, and the worker `shlex.split`s
and requires exactly 5 tokens. GO HAS NO STDLIB SHLEX QUOTER, so #162 must
implement/vendor one and validate it against the shared golden fixture shipped at
`scripts/testdata/gso-shlex-golden.json`. Any token containing a newline or CR is
REJECTED up front as `bad-args` (must be single-line) so the `OK <out>` echo names
the real written path verbatim and each response stays one physical line.

OUTPUT PATH IS WORKER-MINTED (frozen for #162):
  * Base dir: resolved ONCE at startup — env GSO_OUT_DIR if set, else a per-process
    mkdtemp() under the OS temp dir (gso-worker-<pid>/). Printed to stderr once at
    startup so #162 can discover it.
  * Filename: CONTENT-ADDRESSED — gso-<voice_id>-<sha256[:16] of the canonical
    request tuple>.wav, canonical tuple = (voice_id, text, prompt_text,
    ref_audio_path, text_split_method) NUL-joined. Identical requests map to
    identical filenames; distinct requests won't collide in practice at this scale
    (16 hex = 64 bits, birthday-bounded ~2^32 — "won't in practice", not "cannot").
  * De-facto content-addressed CACHE: identical requests resolve to ONE physical
    file — a repeat does NOT produce a second file. Any consumer needing two
    identical-request outputs to coexist must buffer the first response's bytes
    before issuing the second (exactly the Phase-5 warmproof harness discipline).
    #162 must NOT assume one physical file per .run() call.
  * Collision handling: a content-address match means the request is byte-identical
    -> an idempotent atomic overwrite (temp-write + os.replace) is correct + intended.
  * Cleanup / TTL: the WORKER NEVER DELETES minted files and holds no TTL. #162 (the
    caller) owns the full lifecycle — read the WAV named by `OK <out>`, then
    delete/move it, and tear down GSO_OUT_DIR/mkdtemp at worker shutdown. Mirrors the
    ephemeral-sink ownership model.

★ LOAD-BEARING HAPPENS-BEFORE INVARIANT (do not weaken):
  The worker emits `OK <out>` ONLY AFTER the atomic write has fully completed
  (os.replace returned) — see the single `return f"OK {out_path}"` in `_Worker.handle`.
  The Phase-5 warmproof harness buffers each `OK` response's WAV bytes the instant
  the line arrives, so it RELIES on the file being fully written and named-verbatim
  by the time the `OK` line is visible. A worker that echoed `OK` before/around the
  write would let the harness read a partial/absent/other file. Pinned by test:
  `gso_worker_contract_test.py` asserts (a) a successful `OK <out>` implies the file
  exists at <out>, and (b) a write failure yields `ERR write-failed` and NO `OK`.

WIRE-VS-PACKAGED PRECEDENCE + DRIFT RULE (within the AC lock):
  The wire <ref_audio_path>/<prompt_text> tokens are AUTHORITATIVE for what is fed to
  .run(). The packaged per-voice ref_audio.wav/ref_transcript.txt are the
  source-of-record / DR backup and the canonical values #162 SHOULD emit. Drift is
  mapped onto the CLOSED taxonomy (no new category):
    * <ref_audio_path> inside a DIFFERENT roster voice's packaged dir -> bad-voice.
    * <ref_audio_path> outside THIS voice's packaged dir when a packaged ref exists,
      OR <prompt_text> mismatches the packaged ref_transcript.txt on a best-effort
      compare -> bad-args.
  The best-effort prompt_text compare is a COARSE anti-drift guardrail (catches
  wrong-voice / out-of-dir gross drift), NOT transcript enforcement: the wire
  prompt_text wins and is fed to inference, so a reworded-but-plausible prompt_text
  still voices as given. Byte-for-byte transcript verification is explicitly NOT
  mandated in phase one.

RESPONSES — exactly ONE physical line per request (response count == request count):

  OK <out>                      success; the WAV now exists at the worker-minted <out>
  ERR <category> <message>      recoverable per-line failure; the loop continues

  <category> is a CLOSED, append-only set (same tokens as RVC):
      bad-args | bad-voice | read-failed | infer-failed | write-failed
  <message> has all newlines/CRs collapsed to spaces and is capped at 300 chars, so
  an ERR is ALWAYS a single line. #162 branches retryable-vs-fatal on the <category>
  token alone; an unknown category (forward-compat) is treated as fatal.

OK-REPLY PARSING (call this out to #162): the `OK <out>` payload is the LITERAL
remainder of the line after the 3-char prefix `OK ` — parse it as `out_path = line[3:]`
verbatim. It is NOT shlex-quoted and NOT wrapped. The worker rejects any token
containing a newline/CR up front (bad-args), so a minted path is always a single
physical line and needs no quoting; #162 MUST NOT shlex-split or round-trip-quote the
payload (quoting would corrupt a legitimate path containing a space, e.g. a
GSO_OUT_DIR with a space). Pinned by
gso_worker_contract_test.py::test_ok_reply_is_literal_not_shlex_quoted.

FRAMING / FORWARD-COMPAT: the 5-token line is positional and append-only — never
reorder or repurpose a token. Future capability (per-request sample rate, non-cut4
splitting) is a trailing optional token with a proven default. The ERR category set
is append-only.

PROCESS SEMANTICS:
  EOF on stdin                  -> clean exit 0
  BrokenPipeError writing out   -> clean exit 0 (the parent closed the pipe)
  MemoryError / catchable fatal -> stderr FATAL + exit 70 (EX_SOFTWARE), NOT an ERR
  torch absent / base models    -> stderr FATAL + exit 78 (EX_CONFIG), startup fatal
    missing / checkpoint missing
  native fault (SIGSEGV/SIGABRT -> process dies with a signal/nonzero status and
    /MPS abort, uncatchable)        emits NO OK/ERR line for the in-flight request.
                                     #162 detects the unexpected exit + closed pipe
                                     and treats it as a fatal hard-stop (same
                                     disposition as exit 70), never a per-block ERR.

Pipeline loads once per voice. A `LOAD gso <voice_id>` line is printed to stderr
exactly once per voice — the warm-load-once proof `make gso-warmproof` asserts.

See assets/gptsovits-models/README.md for artifact layout, base-model fetch, and the
DR backup/restore runbook. All 7 known runbook gotchas are hardcoded (build-time
1-4 in the venv target; runtime request-validation 5-7 below), never re-discovered.
"""

from __future__ import annotations

import argparse
import hashlib
import os
import shlex
import sys
import tempfile
import wave

# ── Process exit codes (sysexits.h) — the Go caller (#162) branches on these ───
# Startup/construction fatal (torch absent, base models / checkpoints missing, bad
# venv): the worker never serviced a request. Runtime fatal (MemoryError mid-loop):
# a catchable fatal that is NOT a per-block ERR. Both are stable + nonzero.
EXIT_FATAL_STARTUP = 78   # EX_CONFIG   — environment/construction is wrong
EXIT_FATAL_RUNTIME = 70   # EX_SOFTWARE — catchable runtime fatal (e.g. MemoryError)

# ── Roster (phase one: one voice) ─────────────────────────────────────────────
# Anything not here -> ERR bad-voice. Append-only.
ROSTER = ("cool-jahns-gso",)

# ── Per-voice packaged artifact filenames (AC1 source-of-record / DR backup) ──
PACKAGED_REF_AUDIO = "ref_audio.wav"
PACKAGED_REF_TRANSCRIPT = "ref_transcript.txt"
CHECKPOINT_GPT = "gpt.ckpt"       # t2s weights (.ckpt) — consumed directly, no ONNX
CHECKPOINT_SOVITS = "sovits.pth"  # vits weights (.pth) — consumed directly, no ONNX

# ── Phase-one hardcoded inference constants ───────────────────────────────────
TEXT_SPLIT_METHOD = "cut4"   # gotcha 7 — only cut4 accepted / passed to .run()
TEXT_LANG = "en"             # English only, phase one (CLAUDE.md)
PROMPT_LANG = "en"
SR_EXPECTED = 32000          # v2Pro emits 32 kHz directly; never resampled (CLAUDE.md)

# Deterministic inference params (#165 AC5 machine-run finding): with NO seed passed,
# GPT-SoVITS picks a RANDOM one ("Set seed to <rand>") — every request diverges, which
# breaks the warm-load determinism oracle (A1 must ~= A2) and warm-vs-cold equivalence.
# Pin a FIXED seed + the runbook's proven-good sampling params so repeat requests are
# reproducible within the harness RMS tolerance (and quality matches the runbook).
INFER_SEED = 42
INFER_TOP_K = 15
INFER_TOP_P = 1.0
INFER_TEMPERATURE = 1.0
INFER_REPETITION_PENALTY = 1.35

# ── ERR taxonomy — CLOSED, append-only (same tokens as the RVC worker) ────────
ERR_CATEGORIES = ("bad-args", "bad-voice", "read-failed", "infer-failed", "write-failed")
_MSG_CAP = 300

# Catchable fatals: NOT a per-block ERR — propagate to a nonzero process exit.
# Native segfaults/MPS aborts are uncatchable and kill the process with a nonzero
# signal exit; that is relied on (documented), not intercepted.
FATAL_EXC = (MemoryError,)


# ══════════════════════════════════════════════════════════════════════════════
# Pure helpers — stdlib only, NO torch/numpy. Exercised by the torch-free contract
# test. These form the whole wire/ERR/mint/drift surface #162 binds to.
# ══════════════════════════════════════════════════════════════════════════════

def _one_line(text: str) -> str:
    """Collapse newlines/CRs to spaces and cap length — guarantees a single
    physical output line."""
    flat = " ".join(str(text).split())
    return flat[:_MSG_CAP]


def _err_line(category: str, message: str) -> str:
    # Every call site passes a literal from the closed ERR set; a miss is a worker
    # bug, not user input. Coerce to a generic in-set category rather than emit an
    # off-contract token — an EXPLICIT guard, not an `assert` that `python -O` strips.
    if category not in ERR_CATEGORIES:
        message = f"[bug: off-contract category {category!r}] {message}"
        category = "infer-failed"
    return f"ERR {category} {_one_line(message)}"


def _norm(text: str) -> str:
    """Coarse normalization for the best-effort prompt_text drift compare:
    collapse whitespace + case-fold. Deliberately NOT byte-exact — this is an
    anti-drift guardrail, not transcript enforcement (the wire value wins)."""
    return " ".join(text.split()).casefold()


def _is_within(path: str, root: str) -> bool:
    """True if `path` is `root` itself or lives under it. Both should be realpath'd
    by the caller so symlink escapes are resolved before the containment test."""
    if path == root:
        return True
    return path.startswith(root + os.sep)


def mint_out_path(out_base: str, voice_id: str, text: str, prompt_text: str,
                  ref_audio_path: str, text_split_method: str) -> str:
    """Content-addressed output path (frozen contract for #162).

    gso-<voice_id>-<sha256[:16] of the NUL-joined canonical tuple>.wav

    Deterministic: identical requests -> identical path (idempotent overwrite);
    distinct requests won't collide in practice at this scale. The NUL separator
    cannot appear in a valid token (newline/CR are rejected up front and shlex
    tokens do not carry NUL), so the join is unambiguous.
    """
    canonical = "\x00".join(
        (voice_id, text, prompt_text, ref_audio_path, text_split_method)
    )
    digest = hashlib.sha256(canonical.encode("utf-8")).hexdigest()[:16]
    return os.path.join(out_base, f"gso-{voice_id}-{digest}.wav")


def _atomic_write_wav(sample_rate: int, pcm_s16le: bytes, out_path: str) -> None:
    """Write a mono s16le WAV atomically: temp in the same dir + os.replace. A
    failed write never leaves a partial WAV at <out>. stdlib-only (no numpy) so the
    write path is torch-free and testable.

    NOTE: this function returning is the happens-before that `OK <out>` relies on —
    the caller emits `OK` only after os.replace has swapped the finished file in.
    """
    out_dir = os.path.dirname(os.path.abspath(out_path))
    os.makedirs(out_dir, exist_ok=True)
    tmp_path = f"{out_path}.tmp.{os.getpid()}"
    try:
        with wave.open(tmp_path, "wb") as wav:
            wav.setnchannels(1)
            wav.setsampwidth(2)              # 16-bit
            wav.setframerate(sample_rate)    # 32 kHz for v2Pro; never resampled
            wav.writeframes(pcm_s16le)
        os.replace(tmp_path, out_path)       # atomic swap
    except Exception:
        try:
            if os.path.exists(tmp_path):
                os.remove(tmp_path)
        except OSError:
            pass
        raise


# ══════════════════════════════════════════════════════════════════════════════
# Torch bridge — the ONLY torch-touching code. Lazily imported so the wire/ERR/mint
# surface above stays exercisable without torch. Runtime-gated: real inference needs
# torch + the base models + the voice checkpoints (M1 Pro machine run).
# ══════════════════════════════════════════════════════════════════════════════

# ── External GPT-SoVITS code clone (never vendored — subprocess/licensing boundary) ─
# The worker imports GPT_SoVITS.* from an EXTERNAL clone resolved at build time via env
# GSO_REPO, defaulting to ~/repos/GPT-SoVITS-local (the runbook's location). Only the
# CODE is reached from here; the model WEIGHTS come from assets/gptsovits-models. This
# is the same subprocess/licensing boundary as torch itself — GPT-SoVITS's mixed-license
# transitive stack is only ever spoken to across the process boundary, never linked.
DEFAULT_GSO_REPO = os.path.expanduser("~/repos/GPT-SoVITS-local")


def _resolve_gso_repo() -> str:
    """Resolve the external GPT-SoVITS clone (env GSO_REPO, else the runbook default).
    Raised here (not at module scope) so the torch-free wire/ERR/mint surface never
    needs the clone present."""
    repo = os.environ.get("GSO_REPO", DEFAULT_GSO_REPO)
    if not os.path.isdir(os.path.join(repo, "GPT_SoVITS", "TTS_infer_pack")):
        raise FileNotFoundError(
            f"GPT-SoVITS code clone not found at {repo!r} — set env GSO_REPO or clone "
            "https://github.com/RVC-Boss/GPT-SoVITS there (needed for warm load)")
    return repo


def _assert_torch_present() -> None:
    """Startup guard (inverse of the RVC worker). Torch MUST be importable — the
    GSO worker infers with torch. Absent torch -> FATAL 78 (EX_CONFIG, "venv
    broken"). An EXPLICIT check + exit, NOT `assert` (which `python -O` strips)."""
    try:
        import torch  # noqa: F401  (presence probe only)
    except Exception as e:  # noqa: BLE001 — any import failure = broken venv
        print(f"FATAL torch is not importable — .venv-gso is broken: "
              f"{type(e).__name__}: {_one_line(str(e))}",
              file=sys.stderr, flush=True)
        raise SystemExit(EXIT_FATAL_STARTUP)


class _GsoPipeline:
    """Wraps one warm `TTS_infer_pack` pipeline object for a single voice_id.

    Constructed lazily on the first request for a voice; reused warm afterwards.
    All torch / GPT-SoVITS imports happen HERE (inside methods), never at module
    scope — the torch-free surface must not pull them in transitively.
    """

    def __init__(self, voice_id: str, models_root: str, base_root: str) -> None:
        self.voice_id = voice_id
        self.models_root = models_root
        self.base_root = base_root
        self._pipeline = None  # the TTS object, built on first synthesize()

    def _build(self):
        """Import torch + GPT-SoVITS and construct the TTS pipeline ONCE. Faithful to
        the v2Pro runbook — INCLUDING its os.chdir(REPO) + sys.path preamble, which the
        machine run (#165) proved is LOAD-BEARING, not optional: without it neither
        `GPT_SoVITS.*` nor the CWD-relative loads inside GPT_SoVITS/sv.py resolve.
        Runtime-gated (needs torch + the GPT-SoVITS clone + real artifacts)."""
        # MPS fallback for the few ops without an MPS kernel (runbook §1).
        os.environ.setdefault("PYTORCH_ENABLE_MPS_FALLBACK", "1")

        # Put the external GPT-SoVITS clone on sys.path AND chdir into it. sv.py does
        # `sys.path.append(f"{os.getcwd()}/GPT_SoVITS/eres2net")` + hardcodes a
        # CWD-relative module-global `sv_path`, so the runbook's os.chdir(REPO) is
        # required (#165 machine-run finding). Only the CODE comes from the clone; all
        # WEIGHTS are absolute paths under <models_root>/<base_root>, so chdir is safe
        # (out_base was already abspath'd at startup, before this runs).
        repo = _resolve_gso_repo()
        os.chdir(repo)
        for p in (repo, os.path.join(repo, "GPT_SoVITS")):
            if p not in sys.path:
                sys.path.insert(0, p)

        import torch  # noqa: F401
        from GPT_SoVITS.TTS_infer_pack.TTS import TTS, TTS_Config

        voice_dir = os.path.join(self.models_root, self.voice_id)
        gpt_ckpt = os.path.join(voice_dir, CHECKPOINT_GPT)
        sovits_pth = os.path.join(voice_dir, CHECKPOINT_SOVITS)
        for label, path in (("gpt.ckpt", gpt_ckpt), ("sovits.pth", sovits_pth)):
            if not os.path.isfile(path):
                # Startup-shaped construction fault surfaced on first use -> the loop
                # maps it to infer-failed (recoverable per-line); a globally-missing
                # checkpoint is caught eagerly by _Worker validation at startup.
                raise FileNotFoundError(f"missing {label} for {self.voice_id}: {path}")

        # Base-model set fetched by `make gso-fetch-base` into <base_root>.
        cnhubert = os.path.join(self.base_root, "chinese-hubert-base")
        bert = os.path.join(self.base_root, "chinese-roberta-wwm-ext-large")
        # v2Pro speaker-verification embedding (sv/pretrained_eres2netv2w24s4ep4.ckpt).
        sv_ckpt = os.path.join(self.base_root, "sv", "pretrained_eres2netv2w24s4ep4.ckpt")

        # sv_path RESOLUTION (#165 "sv_path confirm" finding): v2Pro's TTS_Config has NO
        # functional `sv_path` key — GPT_SoVITS/sv.py loads the SV embedding from a
        # MODULE-GLOBAL `sv_path` (CWD-relative default) and TTS.py calls
        # `SV(device, is_half)` with no path arg. To load the FETCHED _base embedding
        # deterministically (not the clone's own copy), override that module global
        # here — after importing TTS (which did `from sv import SV`), before TTS(config)
        # constructs it. `import sv` finds the same already-loaded module.
        import sv as _sv  # noqa: E402 — same module TTS.py imported
        _sv.sv_path = sv_ckpt

        device = "mps" if torch.backends.mps.is_available() else "cpu"
        config = TTS_Config({
            "custom": {
                "device": device,
                "is_half": False,                 # fp16 is unstable on MPS (runbook)
                "version": "v2Pro",
                "t2s_weights_path": gpt_ckpt,
                "vits_weights_path": sovits_pth,
                # TTS.py reads `cnhuhbert_base_path` (that spelling). The runbook's
                # `cnhubert_base_path` was an inert key masked by chdir; use the real
                # one so cnhubert loads from _base, not the CWD-relative default (#165).
                "cnhuhbert_base_path": cnhubert,
                "bert_base_path": bert,
                # Kept for intent/forward-compat though inert in this version — the
                # sv.sv_path override above is what actually loads the embedding.
                "sv_path": sv_ckpt,
            }
        })
        self._pipeline = TTS(config)
        # LOAD-once proof: printed exactly once per voice (first build only).
        print(f"LOAD gso {self.voice_id}", file=sys.stderr, flush=True)

    def synthesize(self, text: str, prompt_text: str,
                   ref_audio_path: str) -> tuple[int, bytes]:
        """Run one request against the warm pipeline. Returns (sample_rate,
        s16le PCM bytes). The wire tokens are authoritative (drift already
        validated by the caller). cut4 is hardcoded (gotcha 7); prompt_text +
        ref_audio_path are mandatory + real (gotchas 5/6) and never ref-free."""
        import numpy as np

        if self._pipeline is None:
            self._build()

        gen = self._pipeline.run({
            "text": text,
            "text_lang": TEXT_LANG,
            "ref_audio_path": ref_audio_path,   # mandatory (gotcha 5)
            "prompt_text": prompt_text,         # mandatory, real (gotcha 6)
            "prompt_lang": PROMPT_LANG,
            "text_split_method": TEXT_SPLIT_METHOD,  # cut4 (gotcha 7)
            "return_fragment": False,
            # FIXED seed + runbook sampling params -> reproducible warm/cold output
            # (#165 AC5). Without an explicit seed GPT-SoVITS randomizes per request.
            "seed": INFER_SEED,
            "top_k": INFER_TOP_K,
            "top_p": INFER_TOP_P,
            "temperature": INFER_TEMPERATURE,
            "repetition_penalty": INFER_REPETITION_PENALTY,
            "batch_size": 1,
            "parallel_infer": True,
            "split_bucket": False,
        })
        sample_rate, audio = next(gen)  # single yield (return_fragment=False)

        audio = np.asarray(audio)
        if audio.dtype != np.int16:
            audio = np.clip(audio, -1.0, 1.0)
            audio = (audio * 32767.0).astype(np.int16)
        return int(sample_rate), audio.tobytes()


# ══════════════════════════════════════════════════════════════════════════════
# Worker — holds warm pipelines; processes one request line -> one response string.
# ══════════════════════════════════════════════════════════════════════════════

class _Worker:
    """Warm-pipeline registry + per-request handler. Construction is torch-free
    (no pipeline is built until the first synthesize); the contract test constructs
    a _Worker directly and monkeypatches `_synthesize`."""

    def __init__(self, models_root: str, base_root: str, out_base: str) -> None:
        self.models_root = models_root
        self.base_root = base_root
        self.out_base = out_base
        self._pipelines: dict[str, _GsoPipeline] = {}

    # -- inference seam (monkeypatched by the torch-free contract test) ----------
    def _synthesize(self, voice_id: str, text: str, prompt_text: str,
                    ref_audio_path: str) -> tuple[int, bytes]:
        pipe = self._pipelines.get(voice_id)
        if pipe is None:
            pipe = _GsoPipeline(voice_id, self.models_root, self.base_root)
            self._pipelines[voice_id] = pipe  # warm-load-once
        return pipe.synthesize(text, prompt_text, ref_audio_path)

    # -- drift validation (wire-vs-packaged, mapped onto the CLOSED taxonomy) -----
    def _drift_err(self, voice_id: str, ref_audio_path: str,
                   prompt_text: str) -> str | None:
        """Return an ERR line if the wire ref/prompt drift from the packaged
        source-of-record for this voice, else None. See the module docstring's
        wire-vs-packaged rule. Coarse guardrail, NOT transcript enforcement."""
        voice_dir = os.path.realpath(os.path.join(self.models_root, voice_id))
        ref_real = os.path.realpath(ref_audio_path)

        # Wrong-voice identity: ref path inside a DIFFERENT roster voice's dir.
        for other in ROSTER:
            if other == voice_id:
                continue
            other_dir = os.path.realpath(os.path.join(self.models_root, other))
            if _is_within(ref_real, other_dir):
                return _err_line(
                    "bad-voice",
                    f"ref_audio_path is inside a different voice's packaged dir "
                    f"('{other}'), not '{voice_id}'")

        packaged_ref = os.path.join(voice_dir, PACKAGED_REF_AUDIO)
        packaged_txt = os.path.join(voice_dir, PACKAGED_REF_TRANSCRIPT)

        # Out-of-dir: ref outside THIS voice's dir while a packaged ref exists.
        if os.path.isfile(packaged_ref) and not _is_within(ref_real, voice_dir):
            return _err_line(
                "bad-args",
                f"ref_audio_path is outside the packaged dir for '{voice_id}' "
                f"while a packaged ref exists (source-of-record drift)")

        # Best-effort transcript compare (coarse; NOT enforcement).
        if os.path.isfile(packaged_txt):
            try:
                with open(packaged_txt, encoding="utf-8") as fh:
                    packaged_prompt = fh.read()
            except OSError:
                packaged_prompt = None
            if packaged_prompt is not None and _norm(prompt_text) != _norm(packaged_prompt):
                return _err_line(
                    "bad-args",
                    f"prompt_text drifts from the packaged ref_transcript.txt for "
                    f"'{voice_id}' (best-effort anti-drift guardrail)")
        return None

    def handle(self, line: str) -> str:
        """Parse + run one request line. Returns EXACTLY one response line.

        Raises only FATAL_EXC (MemoryError) / BrokenPipeError to the caller; every
        other failure becomes a single-line typed ERR (the loop survives). A raw
        traceback is NEVER printed to stdout.
        """
        try:
            parts = shlex.split(line)
        except ValueError as e:
            return _err_line("bad-args", f"unparseable line: {e}")

        if len(parts) != 5:
            return _err_line(
                "bad-args",
                f"expected 5 tokens <text> <ref_audio_path> <prompt_text> "
                f"<text_split_method> <voice_id>, got {len(parts)}")
        text, ref_audio_path, prompt_text, split_method, voice_id = parts

        # --- single-line invariant: reject newline/CR in ANY token up front ---
        # A CR can survive CRLF stdin; collapsing it would make the OK echo name a
        # different path than was written. Rejecting keeps OK free to echo verbatim.
        labels = ("text", "ref_audio_path", "prompt_text", "text_split_method", "voice_id")
        for label, val in zip(labels, parts):
            if "\n" in val or "\r" in val:
                return _err_line("bad-args", f"<{label}> contains a newline/CR — tokens must be single-line")

        # --- validation (bad-args / bad-voice) ---
        if not text:
            return _err_line("bad-args", "<text> must be non-empty")
        if split_method != TEXT_SPLIT_METHOD:                      # gotcha 7
            return _err_line("bad-args", f"text_split_method must be '{TEXT_SPLIT_METHOD}', got '{split_method}'")
        if not ref_audio_path:                                     # gotcha 5 (empty)
            return _err_line("bad-args", "<ref_audio_path> is mandatory and must be non-empty")
        if not prompt_text:                                        # gotcha 6 (never ref-free)
            return _err_line("bad-args", "<prompt_text> is mandatory and must be non-empty (no ref-free mode)")
        if voice_id not in ROSTER:
            return _err_line("bad-voice", f"unknown voice '{voice_id}' (roster: {', '.join(ROSTER)})")

        # --- wire-vs-packaged drift (bad-voice / bad-args) ---
        drift = self._drift_err(voice_id, ref_audio_path, prompt_text)
        if drift is not None:
            return drift

        # --- read (read-failed): non-empty ref that is missing/undecodable ---
        # gotcha 5 (missing): reject before inference so a torch "Failed to create
        # AudioDecoder for None"/decode error can never reach stdout.
        if not os.path.isfile(ref_audio_path):
            return _err_line("read-failed", f"ref_audio_path is not a readable file: {ref_audio_path}")

        # --- mint the worker-owned output path (pure; content-addressed) ---
        out_path = mint_out_path(self.out_base, voice_id, text, prompt_text,
                                 ref_audio_path, TEXT_SPLIT_METHOD)

        # --- infer (infer-failed): any mid-generation exception is recoverable;
        #     only FATAL_EXC (MemoryError) escapes to a nonzero process exit. ---
        try:
            sample_rate, pcm = self._synthesize(voice_id, text, prompt_text, ref_audio_path)
        except FATAL_EXC:
            raise
        except Exception as e:  # noqa: BLE001
            return _err_line("infer-failed", f"{voice_id}: {type(e).__name__}: {e}")

        # --- write (write-failed): atomic temp + os.replace ---
        try:
            _atomic_write_wav(sample_rate, pcm, out_path)
        except FATAL_EXC:
            raise
        except Exception as e:  # noqa: BLE001
            return _err_line("write-failed", f"{out_path}: {type(e).__name__}: {e}")

        # ★ LOAD-BEARING INVARIANT: OK is returned ONLY here — AFTER _atomic_write_wav
        # returned (os.replace done). The file at out_path is fully written and named
        # verbatim, so the Phase-5 harness can safely buffer its bytes the instant
        # this line arrives. Do not move this above the write.
        #
        # WIRE NOTE (#162): the payload after "OK " is the LITERAL out_path — NOT
        # shlex-quoted, NOT wrapped. #162 parses it as line[3:] verbatim and must not
        # round-trip-quote it (that would corrupt a path containing a space). The
        # up-front newline/CR -> bad-args rejection keeps it one physical line.
        return f"OK {out_path}"


# ══════════════════════════════════════════════════════════════════════════════
# Protocol loop + startup
# ══════════════════════════════════════════════════════════════════════════════

def _isolate_wire_stdout():
    """Preserve the REAL stdout (fd 1) for the OK/ERR wire, then point fd 1 at stderr
    so the chatty GPT-SoVITS stack — which prints load/inference progress COPIOUSLY to
    stdout ("Loading VITS weights…", "Set seed to…", tqdm bars) — cannot corrupt the
    wire. Returns a line-buffered text stream bound to the preserved fd; it is the ONLY
    thing that may ever write an OK/ERR line.

    #165 machine-run finding: without this the OK line is buried under ~5KB of library
    chatter (or lost to block-buffering), so a reader taking the FIRST stdout line gets
    "Loading Text2Semantic weights…" instead of "OK <path>". Set up BEFORE any heavy
    import. Torch-free-safe: with no chatty library loaded it is a harmless fd shuffle
    and the wire still carries exactly the OK/ERR lines the contract test reads."""
    sys.stdout.flush()
    wire_fd = os.dup(sys.stdout.fileno())               # private handle to real stdout
    os.dup2(sys.stderr.fileno(), sys.stdout.fileno())   # fd 1 -> stderr (noise sink)
    return os.fdopen(wire_fd, "w", buffering=1)         # line-buffered wire


def _emit(wire, response: str) -> None:
    """Write one response line to the PRESERVED wire + flush. BrokenPipe propagates
    (clean exit 0)."""
    wire.write(response + "\n")
    wire.flush()


def run_loop(worker: "_Worker", wire) -> int:
    """Service stdin line by line until EOF. Uses readline() (NOT `for line in
    sys.stdin`) so an INTERACTIVE caller — the warmproof harness / #162 — that writes
    one request then blocks for its response never deadlocks on text-iterator
    read-ahead. One response per input line: a blank line is NOT skipped — it
    shlex-splits to 0 tokens and yields a natural `ERR bad-args`, preserving
    response-count == request-count. Returns the process exit code (0 on EOF)."""
    while True:
        line = sys.stdin.readline()
        if not line:                     # '' == EOF (a blank line is '\n', not '')
            return 0
        response = worker.handle(line.rstrip("\n"))   # exactly one line back
        _emit(wire, response)


def _resolve_out_base() -> str:
    """Resolve the worker-owned output base dir ONCE at startup: env GSO_OUT_DIR if
    set, else a per-process mkdtemp under the OS temp dir. The worker never deletes
    inside it — #162 owns cleanup."""
    env = os.environ.get("GSO_OUT_DIR")
    if env:
        os.makedirs(env, exist_ok=True)
        return os.path.abspath(env)
    return tempfile.mkdtemp(prefix=f"gso-worker-{os.getpid()}-")


def _default_models_root() -> str:
    return os.path.join(
        os.path.dirname(os.path.dirname(os.path.abspath(__file__))),
        "assets", "gptsovits-models",
    )


def _parse_args(argv: list[str]) -> argparse.Namespace:
    ap = argparse.ArgumentParser(
        prog="gptsovits_worker",
        description="GPT-SoVITS warm-load-once TTS worker (stdin/stdout wire v1).",
    )
    ap.add_argument(
        "--models-root",
        default=_default_models_root(),
        help="root holding <voice_id>/{gpt.ckpt,sovits.pth,ref_audio.wav,"
             "ref_transcript.txt} + _base/ (default: <repo>/assets/gptsovits-models).",
    )
    return ap.parse_args(argv)


def _startup_check_artifacts(models_root: str, base_root: str) -> None:
    """Eager startup validation of the shared + per-voice artifacts. A globally
    missing base model or checkpoint is a construction fault -> FATAL 78 (the worker
    never serviced a request), not a per-block ERR. Verifies the base-model weight
    FILES exist (not merely their dirs), so a partial/interrupted fetch fails clean
    at startup rather than as an infer-failed on the first request."""
    # Base-model WEIGHT FILES (not just their dirs): a partial/interrupted fetch
    # leaves the component dir present but the weight blob absent. Check the blob (and
    # use os.path.isfile) so that yields a clean FATAL-78 here, not an infer-failed on
    # the first request. These are exactly the files scripts/gso-fetch-base pulls.
    required = [
        os.path.join(base_root, "chinese-hubert-base", "pytorch_model.bin"),
        os.path.join(base_root, "chinese-roberta-wwm-ext-large", "pytorch_model.bin"),
        os.path.join(base_root, "sv", "pretrained_eres2netv2w24s4ep4.ckpt"),
    ]
    for voice_id in ROSTER:
        voice_dir = os.path.join(models_root, voice_id)
        required.append(os.path.join(voice_dir, CHECKPOINT_GPT))
        required.append(os.path.join(voice_dir, CHECKPOINT_SOVITS))
    missing = [p for p in required if not os.path.isfile(p)]
    if missing:
        raise FileNotFoundError(
            "missing required artifact file(s) — run `make gso-fetch-base` + place the "
            "voice checkpoints: " + "; ".join(missing))


def main(argv: list[str] | None = None) -> int:
    args = _parse_args(sys.argv[1:] if argv is None else argv)
    base_root = os.path.join(args.models_root, "_base")

    # Preserve the real stdout for the wire and shunt fd 1 -> stderr BEFORE any heavy
    # import can pollute it (#165). All OK/ERR lines go through `wire`; FATAL/OUT_BASE/
    # LOAD + all library chatter go to stderr.
    wire = _isolate_wire_stdout()

    # Startup (torch presence + artifact resolve + out-base) is fatal-if-it-raises:
    # emit ONE clean FATAL line + a stable nonzero exit, never a raw traceback. #162
    # reads the nonzero exit.
    try:
        _assert_torch_present()               # inverse of RVC — torch MUST be present
        _startup_check_artifacts(args.models_root, base_root)
        out_base = _resolve_out_base()
        worker = _Worker(args.models_root, base_root, out_base)
    except SystemExit:
        raise                                 # _assert_torch_present already exited 78
    except FATAL_EXC as e:
        print(f"FATAL {type(e).__name__}: {_one_line(str(e))}", file=sys.stderr, flush=True)
        return EXIT_FATAL_RUNTIME
    except Exception as e:  # noqa: BLE001 — missing artifact / bad venv / construction
        print(f"FATAL {type(e).__name__}: {_one_line(str(e))}", file=sys.stderr, flush=True)
        return EXIT_FATAL_STARTUP

    # Announce the resolved output base once so #162 can discover minted paths.
    print(f"OUT_BASE {out_base}", file=sys.stderr, flush=True)

    try:
        return run_loop(worker, wire)
    except BrokenPipeError:
        # Parent closed the pipe — clean exit 0. Silence the final flush-on-exit
        # BrokenPipe by redirecting the WIRE fd (the one that broke) to devnull.
        try:
            devnull = os.open(os.devnull, os.O_WRONLY)
            os.dup2(devnull, wire.fileno())
        except OSError:
            pass
        return 0
    except FATAL_EXC as e:
        # Catchable fatal (e.g. MemoryError) -> stderr + nonzero exit, NOT ERR.
        print(f"FATAL {type(e).__name__}: {_one_line(str(e))}", file=sys.stderr, flush=True)
        return EXIT_FATAL_RUNTIME


if __name__ == "__main__":
    try:
        sys.exit(main())
    except KeyboardInterrupt:
        sys.exit(130)
