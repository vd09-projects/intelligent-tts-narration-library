#!/usr/bin/env python3
"""RVC torch-free worker parity + contract gate (issue #144, plan Step 5).

Plain-python asserts (no pytest dep). Run under .venv-rvc via `make rvc-parity`:

    .venv-rvc/bin/python tests/rvc_parity/parity_test.py

Covers, in one always-on run (review B2 — no env-gated skips):
  * per-stage ONNX-vs-refio parity   (contentvec / rmvpe / net_g, corr ≥ 0.999)
  * full-pipeline log-mel parity     (corr ≥ 0.98 vs the FETCHED target, PARITY_VOICES;
                                       missing target -> FAIL, never skip)
  * parity-voice coverage (honesty)  (PARITY_VOICES + EXCLUDED_PARITY_VOICES partition
                                       the repo roster; no voice silently dropped)
  * torch-free subprocess banner     (`torch imported at inference?: False`)
  * protocol-loop contract           (count/framing/order/category/warm-load-once)
  * arg validation                   (table-driven, each -> the right ERR category)
  * single-line-ERR                  (newline/CR collapse + 300-char cap)
  * atomic write                     (a forced write failure leaves no partial WAV)
  * WAV format                       (40 kHz mono s16le, T*400-80000 samples)
  * npy unpickle-safety              (fetched .npy target loads with allow_pickle=False)
  * vacuous-test guard               (a perturbed feed MUST drop below the floor)

The fixtures (source.wav + the PARITY_VOICES ref WAV + log-mel target) are fetched
from a pinned GitHub Release by `make rvc-fixtures-fetch` (a prerequisite of
`make rvc-parity`), not committed — see tests/rvc_parity/fixtures/README.md.

Exit 0 iff every check passes; nonzero otherwise.
"""

from __future__ import annotations

import os
import subprocess
import sys

import numpy as np

_HERE = os.path.dirname(os.path.abspath(__file__))
_REPO = os.path.dirname(os.path.dirname(_HERE))
_SCRIPTS = os.path.join(_REPO, "scripts")
FIXTURES = os.path.join(_HERE, "fixtures")
MODELS_ROOT = os.path.join(_REPO, "assets", "rvc-models")
WORKER_PY = os.path.join(_SCRIPTS, "rvc_worker.py")
VOICES = {"cool-jahns": 0.75, "confident-neal": 0.5}

# Import the worker module + shared log-mel for unit-level checks.
sys.path.insert(0, _SCRIPTS)
sys.path.insert(0, _HERE)
import rvc_worker as worker            # noqa: E402
from logmel import log_mel, logmel_corr  # noqa: E402
from parity_voices import (            # noqa: E402
    ALL_RVC_VOICES,
    EXCLUDED_PARITY_VOICES,
    PARITY_VOICES,
    check_parity_coverage,
    excluded_reason,
)

# Per-stage floors (PILOT_REPORT §2). net_g uses the stored rnd so only the
# internal NSF-dither RandomNormalLike differs -> deterministic, high corr.
PERSTAGE = {
    "contentvec": {"npz": os.path.join(MODELS_ROOT, "_shared", "onnx", "contentvec_refio.npz"),
                   "onnx": os.path.join(MODELS_ROOT, "_shared", "onnx", "contentvec.onnx"),
                   "feeds": ["audio"], "ref": "ref", "out": "feats",
                   "corr": 0.999, "max_abs": 1.0e-3},          # pilot 6.4e-5
    "rmvpe": {"npz": os.path.join(MODELS_ROOT, "_shared", "onnx", "rmvpe_refio.npz"),
              "onnx": os.path.join(MODELS_ROOT, "_shared", "onnx", "rmvpe.onnx"),
              "feeds": ["mel"], "ref": "ref", "out": "hidden",
              "corr": 0.999, "max_abs": 1.0e-4},               # pilot 1.3e-7
    "net_g": {"npz": None, "onnx": None,                       # per-voice, filled below
              "feeds": ["phone", "phone_lengths", "pitch", "nsff0", "sid", "rnd"],
              "ref": "ref_audio", "out": "audio",
              "corr": 0.999, "max_abs": 5.0e-1},               # pilot real-feats 0.9993
}

FULLPIPE_CORR_FLOOR = 0.98
SEED_ENV = {"RVC_SEED": "1234"}


# ── helpers ───────────────────────────────────────────────────────────────────

def _run_worker(lines, extra_env=None, timeout=600):
    """Spawn the worker (same venv interpreter), feed stdin lines, capture IO."""
    env = dict(os.environ)
    if extra_env:
        env.update(extra_env)
    stdin = "".join(l + "\n" for l in lines)
    proc = subprocess.run(
        [sys.executable, WORKER_PY, "--models-root", MODELS_ROOT],
        input=stdin, capture_output=True, text=True, env=env, timeout=timeout,
    )
    return proc


def _corr(a, b):
    a, b = a.reshape(-1).astype(np.float64), b.reshape(-1).astype(np.float64)
    n = min(len(a), len(b))
    return float(np.corrcoef(a[:n], b[:n])[0, 1])


# ── tests ─────────────────────────────────────────────────────────────────────

# GAP: the rmvpe per-stage test covers only the ONNX net (mel->salience). The numpy
# f0 front-end (_rmvpe_mel/_decode_f0/_coarse_f0) + frame alignment are validated
# only end-to-end by the full-pipeline gate. A focused DSP-glue fixture (like the
# pilot's torch_probe.py "pitch 100% exact bucket match, pitchf max|Δ| 7.6e-5")
# committed here would isolate that stage — worth a dedicated test-coverage ticket. (tracked: #150)
def test_perstage_parity():
    """Per-stage ONNX vs stored torch refio (contentvec, rmvpe, net_g/both voices)."""
    cases = []
    for name in ("contentvec", "rmvpe"):
        cases.append((name, PERSTAGE[name]["npz"], PERSTAGE[name]["onnx"], PERSTAGE[name]))
    for slug in VOICES:
        onnx = os.path.join(MODELS_ROOT, slug, "onnx", "net_g.onnx")
        npz = os.path.join(MODELS_ROOT, slug, "onnx", "net_g_refio.npz")
        cases.append((f"net_g/{slug}", npz, onnx, PERSTAGE["net_g"]))

    for name, npz, onnx, cfg in cases:
        assert os.path.isfile(npz), f"{name}: missing refio {npz}"
        assert os.path.isfile(onnx), f"{name}: missing onnx {onnx}"
        sess = worker._load_session(onnx)
        d = np.load(npz)
        feeds = {k: d[k] for k in cfg["feeds"]}
        out = sess.run([cfg["out"]], feeds)[0]
        ref = d[cfg["ref"]]
        assert np.all(np.isfinite(out)), f"{name}: NaN/Inf in ONNX output"
        corr = _corr(out, ref)
        n = min(out.size, ref.size)
        max_abs = float(np.max(np.abs(out.reshape(-1)[:n] - ref.reshape(-1)[:n])))
        assert corr >= cfg["corr"], f"{name}: corr {corr:.6f} < {cfg['corr']}"
        assert max_abs <= cfg["max_abs"], f"{name}: max|Δ| {max_abs:.3e} > {cfg['max_abs']:.1e}"
        print(f"  [perstage] {name}: corr={corr:.6f} max|Δ|={max_abs:.3e}  OK")


def test_full_pipeline_logmel():
    """Full pipeline vs the FETCHED per-voice log-mel target. ALWAYS runs; a
    missing target is a FAILURE, never a skip (review B2).

    Scoped to PARITY_VOICES (single voice): the gate is a pipeline FLOW gate, not a
    per-voice correctness matrix — one voice exercises the whole path. The dropped
    voice is matrix-narrowed (its case is not generated), documented in
    EXCLUDED_PARITY_VOICES, and covered by test_parity_voice_coverage — never a
    silent skip. See tests/rvc_parity/fixtures/README.md."""
    source = os.path.join(FIXTURES, "source.wav")
    assert os.path.isfile(source), (
        f"missing source clip {source} — run `make rvc-fixtures-fetch` (a "
        "prerequisite of `make rvc-parity`) to fetch the hosted bundle. The gate "
        "does NOT skip.")
    for slug in PARITY_VOICES:
        index_rate = VOICES[slug]
        target_path = os.path.join(FIXTURES, f"{slug}_logmel_target.npy")
        assert os.path.isfile(target_path), (
            f"missing log-mel target {target_path} — run `make rvc-fixtures-fetch`. "
            "Missing target FAILS the gate (never skips).")
        # allow_pickle=False: a tampered .npy target cannot smuggle+execute a pickle.
        target = np.load(target_path, allow_pickle=False)

        out_wav = os.path.join(FIXTURES, f"_out_{slug}.wav")
        if os.path.exists(out_wav):
            os.remove(out_wav)
        proc = _run_worker([f'"{source}" "{out_wav}" {slug} {index_rate} 0'], extra_env=SEED_ENV)
        assert proc.returncode == 0, f"{slug}: worker exit {proc.returncode}\n{proc.stderr}"
        resp = [l for l in proc.stdout.splitlines() if l.strip()]
        assert resp and resp[0].startswith("OK "), f"{slug}: expected OK, got {resp}\n{proc.stderr}"
        assert os.path.isfile(out_wav), f"{slug}: no output WAV at {out_wav}"

        import soundfile as sf
        audio, sr = sf.read(out_wav, dtype="float32", always_2d=False)
        assert sr == 40000 and audio.ndim == 1, f"{slug}: not 40k mono"
        assert np.all(np.isfinite(audio)), f"{slug}: NaN/Inf in output"
        mel = log_mel(audio, sr)
        corr = logmel_corr(mel, target)
        assert corr >= FULLPIPE_CORR_FLOOR, f"{slug}: log-mel corr {corr:.4f} < {FULLPIPE_CORR_FLOOR}"

        # matched peak/RMS against the reference WAV. The verified fetch GUARANTEES
        # the ref WAV is present before the gate runs, so a missing one is a FAIL,
        # not a silent no-op (D6 guard-hardening — honesty rule).
        ref_wav = os.path.join(FIXTURES, f"{slug}_ref.wav")
        assert os.path.isfile(ref_wav), (
            f"missing reference WAV {ref_wav} — it ships in the fetched bundle; run "
            "`make rvc-fixtures-fetch`. The peak/RMS sub-check FAILS (never skips).")
        ref_audio, _ = sf.read(ref_wav, dtype="float32", always_2d=False)
        if ref_audio.ndim > 1:
            ref_audio = ref_audio.mean(axis=1)
        pk_o, pk_r = float(np.abs(audio).max()), float(np.abs(ref_audio).max())
        rms_o = float(np.sqrt(np.mean(audio ** 2)))
        rms_r = float(np.sqrt(np.mean(ref_audio ** 2)))
        assert abs(pk_o - pk_r) <= 0.05, f"{slug}: peak mismatch {pk_o:.3f} vs {pk_r:.3f}"
        assert abs(rms_o - rms_r) <= 0.01, f"{slug}: rms mismatch {rms_o:.4f} vs {rms_r:.4f}"
        os.remove(out_wav)
        print(f"  [fullpipe] {slug}: log-mel corr={corr:.4f}  OK")


def test_parity_voice_coverage():
    """Honesty gate for the single-voice narrowing (issue #151).

    PARITY_VOICES (the fetched-fixture matrix) and EXCLUDED_PARITY_VOICES (the
    documented drop) must be disjoint AND together cover every voice the repo
    defines, so a future voice cannot be silently absent from both. The
    authoritative roster is ALL_RVC_VOICES (independent of PARITY_VOICES); it is
    cross-checked against the per-stage VOICES matrix so the two cannot drift."""
    assert set(VOICES) == set(ALL_RVC_VOICES), (
        f"per-stage VOICES {sorted(VOICES)} and the authoritative roster "
        f"ALL_RVC_VOICES {sorted(ALL_RVC_VOICES)} disagree — a voice was added to "
        f"one but not the other; reconcile so coverage stays non-vacuous.")
    check_parity_coverage(ALL_RVC_VOICES)  # raises if not an honest partition
    assert "confident-neal" in EXCLUDED_PARITY_VOICES, "confident-neal drop undocumented"
    # read the documented reason through the canonical accessor, not the raw dict.
    assert excluded_reason("confident-neal").strip(), "empty exclusion reason"
    print(f"  [parity-voices] PARITY={list(PARITY_VOICES)} "
          f"EXCLUDED={sorted(EXCLUDED_PARITY_VOICES)} — honest partition of "
          f"{sorted(ALL_RVC_VOICES)}  OK")


def test_npy_allow_pickle_guard():
    """A fetched .npy target must load with allow_pickle=False so a tampered file
    cannot execute a pickle payload. Prove the flag actually blocks object arrays."""
    import tempfile
    with tempfile.TemporaryDirectory() as d:
        evil = os.path.join(d, "evil.npy")
        # An object-dtype array can only be persisted/loaded via pickle.
        np.save(evil, np.array([{"x": 1}], dtype=object), allow_pickle=True)
        raised = False
        try:
            np.load(evil, allow_pickle=False)
        except ValueError:
            raised = True
        assert raised, "np.load(allow_pickle=False) did NOT reject a pickled .npy"
    print("  [npy-safety] allow_pickle=False rejects a pickled target  OK")


def test_torch_free_banner():
    """Subprocess the worker and assert the torch-free banner (immediate EOF)."""
    proc = _run_worker([])
    assert proc.returncode == 0, f"worker exit {proc.returncode}\n{proc.stderr}"
    assert "torch imported at inference?: False" in proc.stderr, \
        f"missing torch-free banner in stderr:\n{proc.stderr}"
    print("  [torch-free] banner reports False  OK")


def test_protocol_loop_contract():
    """Multi-line batch: count/framing/order/category + warm-load-once."""
    source = os.path.join(FIXTURES, "source.wav")
    assert os.path.isfile(source), f"missing {source} (gen_targets.py Step 0a)"
    out = os.path.join(FIXTURES, "_loop_out.wav")
    lines = [
        f'"{source}" "{out}" cool-jahns 0.75 0',       # [0] OK
        f'"{source}" "{out}" confident-neal 0.5 0',     # [1] OK
        "",                                             # [2] blank -> ERR bad-args (round-3 #2)
        f'"{source}" "{out}" not-a-voice 0.5 0',        # [3] ERR bad-voice
        f'"{source}" "{out}" cool-jahns 0.5 2',         # [4] ERR bad-args (non-zero pitch)
    ]
    proc = _run_worker(lines, extra_env=SEED_ENV)
    if os.path.exists(out):
        os.remove(out)
    assert proc.returncode == 0, f"exit {proc.returncode}\n{proc.stderr}"
    resp = proc.stdout.splitlines()
    # every physical stdout line is a complete response; count == request count. A
    # blank input line is NOT skipped — it yields exactly one ERR bad-args, so the
    # count invariant holds AND the loop keeps servicing the lines after it.
    assert len(resp) == len(lines), f"response count {len(resp)} != {len(lines)}\n{proc.stdout}"
    for r in resp:
        assert "\n" not in r and "\r" not in r, f"multi-line response: {r!r}"
    assert resp[0].startswith("OK "), resp[0]
    assert resp[1].startswith("OK "), resp[1]
    assert resp[2].startswith("ERR bad-args "), resp[2]      # blank line -> bad-args
    assert resp[3].startswith("ERR bad-voice "), resp[3]
    assert resp[4].startswith("ERR bad-args "), resp[4]
    for r in resp:
        if r.startswith("ERR "):
            cat = r.split(" ", 2)[1]
            assert cat in worker.ERR_CATEGORIES, f"invalid category token: {cat}"
    # warm-load-once: exactly one LOAD per (kind, voice).
    for token in ("LOAD net_g cool-jahns", "LOAD index cool-jahns",
                  "LOAD net_g confident-neal", "LOAD index confident-neal"):
        c = proc.stderr.count(token)
        assert c == 1, f"warm-load-once broken: '{token}' appeared {c}x"
    print("  [protocol] count/framing/order/category + warm-load-once  OK")


def test_arg_validation_table():
    """Table-driven parser validation — each bad line -> the right ERR category."""
    source = os.path.join(FIXTURES, "source.wav")
    src = source if os.path.isfile(source) else "/does/not/matter.wav"
    out = os.path.join(FIXTURES, "_argval_out.wav")
    cases = [
        ("too few tokens",        "a b c",                                   "ERR bad-args "),
        ("unknown voice",         f'"{src}" "{out}" nope 0.5 0',             "ERR bad-voice "),
        ("index_rate NaN",        f'"{src}" "{out}" cool-jahns abc 0',       "ERR bad-args "),
        ("index_rate out of range", f'"{src}" "{out}" cool-jahns 1.5 0',     "ERR bad-args "),
        ("pitch non-int",         f'"{src}" "{out}" cool-jahns 0.5 x',       "ERR bad-args "),
        ("pitch non-zero",        f'"{src}" "{out}" cool-jahns 0.5 3',       "ERR bad-args "),
        ("missing input file",    f'"/no/such/file.wav" "{out}" cool-jahns 0.5 0', "ERR read-failed "),
    ]
    proc = _run_worker([c[1] for c in cases])
    if os.path.exists(out):
        os.remove(out)
    resp = proc.stdout.splitlines()
    assert len(resp) == len(cases), f"count {len(resp)} != {len(cases)}\n{proc.stdout}\n{proc.stderr}"
    for (label, _line, expect), got in zip(cases, resp):
        assert got.startswith(expect), f"{label}: expected {expect!r}, got {got!r}"
    print("  [argval] all 7 validation rows -> correct ERR category  OK")


def test_single_line_err_unit():
    """_one_line collapses newlines/CRs and caps length; _err_line is one line."""
    assert worker._one_line("a\nb\r\nc\td") == "a b c d"
    long = worker._err_line("read-failed", "boom\nsecond line\n" + "x" * 500)
    assert "\n" not in long and "\r" not in long, "ERR not single-line"
    assert long.startswith("ERR read-failed "), long
    assert len(long.split(" ", 2)[2]) <= worker._MSG_CAP, "message not capped at 300"
    print("  [single-line-ERR] collapse + 300-char cap  OK")


def test_atomic_write():
    """A forced write failure leaves NO partial file at <out> (temp cleaned)."""
    import tempfile
    tone = (0.1 * np.sin(np.linspace(0, 3.14 * 200, 40000))).astype(np.float32)
    with tempfile.TemporaryDirectory() as d:
        # success path -> valid 40k mono s16le WAV.
        good = os.path.join(d, "good.wav")
        worker._atomic_write_wav(tone, good)
        import wave
        with wave.open(good, "rb") as w:
            assert w.getframerate() == 40000 and w.getnchannels() == 1 and w.getsampwidth() == 2

        # failure path -> os.replace raises; assert no partial + temp cleaned.
        # Patch via unittest.mock so the real os.replace is restored by the context
        # manager even if an assertion below raises — no manual process-global
        # save/restore of os.replace (round-3 #5).
        bad = os.path.join(d, "bad.wav")
        from unittest.mock import patch
        with patch("rvc_worker.os.replace", side_effect=OSError("disk full")):
            raised = False
            try:
                worker._atomic_write_wav(tone, bad)
            except OSError:
                raised = True
            assert raised, "forced write failure did not raise"
            assert not os.path.exists(bad), "partial WAV left at <out>"
            leftovers = [f for f in os.listdir(d) if f.startswith("bad.wav.tmp")]
            assert not leftovers, f"temp not cleaned: {leftovers}"
    print("  [atomic-write] failure leaves no partial WAV  OK")


def test_wav_format():
    """Full-pipeline output is 40 kHz mono s16le with a T*400-80000 length."""
    source = os.path.join(FIXTURES, "source.wav")
    assert os.path.isfile(source), f"missing {source} (gen_targets.py Step 0a)"
    out = os.path.join(FIXTURES, "_fmt_out.wav")
    if os.path.exists(out):
        os.remove(out)
    proc = _run_worker([f'"{source}" "{out}" cool-jahns 0.75 0'], extra_env=SEED_ENV)
    assert proc.returncode == 0, proc.stderr
    import wave
    with wave.open(out, "rb") as w:
        assert w.getframerate() == 40000, "not 40 kHz"
        assert w.getnchannels() == 1, "not mono"
        assert w.getsampwidth() == 2, "not 16-bit"
        n = w.getnframes()
    os.remove(out)
    assert n > 0, "empty output"
    # pre-trim length = T*400; after [40000:-40000] trim, n = T*400 - 80000.
    assert (n + 80000) % 400 == 0, f"length {n} not consistent with T*400-80000"
    print("  [wav-format] 40k mono s16le, length invariant  OK")


def test_vacuous_guard():
    """A deliberately WRONG feed MUST drop below the corr floor (guards against a
    test that passes on anything)."""
    npz = PERSTAGE["contentvec"]["npz"]
    onnx = PERSTAGE["contentvec"]["onnx"]
    assert os.path.isfile(npz) and os.path.isfile(onnx), "contentvec assets missing"
    sess = worker._load_session(onnx)
    d = np.load(npz)
    good = sess.run(["feats"], {"audio": d["audio"]})[0]
    assert _corr(good, d["ref"]) >= 0.999, "sanity: correct feed should match"
    rng = np.random.default_rng(0)
    perturbed = (d["audio"] + rng.standard_normal(d["audio"].shape).astype(np.float32) * 5.0)
    bad = sess.run(["feats"], {"audio": perturbed.astype(np.float32)})[0]
    assert _corr(bad, d["ref"]) < 0.999, "vacuous: a wrong feed still passed the floor"
    print("  [vacuous-guard] perturbed feed drops below floor  OK")


TESTS = [
    test_single_line_err_unit,     # pure-unit first (no models)
    test_atomic_write,             # pure-unit (no models)
    test_parity_voice_coverage,    # pure-unit honesty gate (no models)
    test_npy_allow_pickle_guard,   # pure-numpy (no models)
    test_torch_free_banner,
    test_vacuous_guard,
    test_perstage_parity,
    test_arg_validation_table,
    test_protocol_loop_contract,
    test_wav_format,
    test_full_pipeline_logmel,     # headline gate last
]


def main() -> int:
    print("== RVC parity + contract gate (issue #144) ==")
    failures = []
    for t in TESTS:
        try:
            t()
        except Exception as e:  # noqa: BLE001 - collect + report, fail at the end
            failures.append((t.__name__, e))
            print(f"  FAIL {t.__name__}: {e}")
    print()
    if failures:
        print(f"FAILED {len(failures)}/{len(TESTS)}:")
        for name, e in failures:
            print(f"  - {name}: {e}")
        return 1
    print(f"PASSED {len(TESTS)}/{len(TESTS)} — parity + contract gate green.")
    return 0


if __name__ == "__main__":
    sys.exit(main())
