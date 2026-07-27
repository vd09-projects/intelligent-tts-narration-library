#!/usr/bin/env python3
"""GSO warm-load-across-blocks CORRECTNESS proof harness (issue #161, AC5).

Proves that ONE warm `TTS_infer_pack` pipeline object serves N sequential requests
CORRECTLY — not merely quickly. Latency is demoted to an informational note; the
pass/fail gate is two correctness oracles:

  (c) repeat-input determinism [PRIMARY]: feed A, B, A on the warm worker and assert
      bytes(A1) ~= bytes(A2) within an RMS tolerance (seeded; not byte-equality — MPS
      is not guaranteed bit-exact). Also assert bytes(B) != bytes(A) so a warm worker
      returning stale cached output for EVERY input is caught (an A-vs-A-only check
      would miss it — review suggestion 2).
  (d) warm-vs-cold equivalence [SECONDARY]: the genuinely-warm Nth output (A2 — the
      3rd request on the warm worker, NOT the cold-equivalent 1st) vs a fresh
      single-shot process on the same input, within the same tolerance, using
      DISTINCT GSO_OUT_DIRs so the two identical-input runs cannot clobber each other
      on a shared content-addressed path.

HARNESS DISCIPLINE (round-2 blocking fix — harness-only; the worker is unchanged):
the worker mints a CONTENT-ADDRESSED path, so two identical requests resolve to the
SAME file and the second .run() idempotently OVERWRITES the first. This harness
therefore READS-AND-BUFFERS each response's WAV bytes into memory the INSTANT its
`OK <out>` line arrives — BEFORE feeding the next request — so A1's bytes survive
A2's overwrite. It compares BUFFERED bytes and NEVER re-reads a path after a later
request. This relies on the worker's load-bearing happens-before invariant: `OK <out>`
is emitted only after the atomic write completes (see gptsovits_worker.py).

NEGATIVE DRY-CHECK (`--negative-dry-check`, torch-free — RUNS THIS SESSION):
runs the SAME oracle against a deliberately state-leaking fake worker stub
(scripts/testdata/fake-gso-worker.py) and asserts it goes RED on a SUBTLE
near-threshold divergence (not only gross divergence — review suggestion 1), GREEN on
sub-threshold jitter, and that the bytes(B) != bytes(A) guard fires against a
stale-cache stub. This proves the oracle CAN fail (and does not false-positive)
BEFORE it is trusted to pass green on real artifacts.

Real mode (default) needs `.venv-gso` + the real checkpoints + fetched base models;
it is machine-gated (see the build artifact: pending-machine-run). The negative
dry-check needs neither and is the in-session proof.

Usage:
    python3 scripts/gso_warmproof.py --negative-dry-check      # torch-free self-test
    scripts/gso --help                                         # (real worker)
    make gso-warmproof                                         # real mode (needs venv+artifacts)
"""

from __future__ import annotations

import argparse
import array
import math
import os
import shlex
import subprocess
import sys
import tempfile
import threading
import time
import wave

_HERE = os.path.dirname(os.path.abspath(__file__))
_ROOT = os.path.dirname(_HERE)

VOICE_ID = "cool-jahns-gso"
SPLIT_METHOD = "cut4"

# ── Oracle tolerances (normalized to int16 full-scale) ────────────────────────
DETERMINISM_RMS_TOL = 1e-3   # A1 vs A2 (warm) / warm vs cold must be within this
DISTINCT_MIN_RMS = 1e-2      # B must differ from A by at least this (anti-stale)
NONSILENT_RMS_FLOOR = 1e-4   # each output must carry at least this much energy

# ── Negative-dry-check leak levels (chosen relative to the tolerance) ─────────
SUBTLE_LEAK_RMS = 2e-3       # ~2x the tol — SUBTLE, must still be caught (RED)
SUBTHRESHOLD_JITTER_RMS = 2e-4  # below the tol — must be tolerated (GREEN)
GROSS_LEAK_RMS = 0.2         # obvious divergence — must be caught (RED)


class HarnessError(RuntimeError):
    """Infrastructure failure (worker crashed, no response, unreadable WAV)."""


class OracleFailure(AssertionError):
    """A correctness oracle judged the outputs divergent/stale — a RED result."""


# ══════════════════════════════════════════════════════════════════════════════
# Oracle primitives — stdlib only (array + math + wave). Work identically under the
# .venv-gso python (3.11) and a stock python3 (incl. 3.13+, where audioop is gone).
# These are SHARED by real mode and the negative dry-check — that sharing is the
# whole point: the dry-check exercises the exact code real mode trusts.
# ══════════════════════════════════════════════════════════════════════════════

def read_wav_pcm(path: str) -> tuple[int, bytes]:
    """Read a mono 16-bit WAV -> (sample_rate, s16le PCM bytes)."""
    try:
        with wave.open(path, "rb") as w:
            if w.getsampwidth() != 2:
                raise HarnessError(f"{path}: expected 16-bit PCM, got {w.getsampwidth()*8}-bit")
            if w.getnchannels() != 1:
                raise HarnessError(f"{path}: expected mono, got {w.getnchannels()} channels")
            frames = w.readframes(w.getnframes())
            return w.getframerate(), frames
    except wave.Error as e:
        raise HarnessError(f"{path}: not a readable WAV: {e}") from e
    except FileNotFoundError as e:
        raise HarnessError(f"{path}: WAV not found (was OK emitted before the write?): {e}") from e


def _samples(pcm: bytes) -> array.array:
    a = array.array("h")
    a.frombytes(pcm)
    return a


def rms_norm(pcm: bytes) -> float:
    """RMS of a signal, normalized to int16 full-scale (0..~1)."""
    a = _samples(pcm)
    if not a:
        return 0.0
    sq = 0.0
    for v in a:
        sq += float(v) * float(v)
    return math.sqrt(sq / len(a)) / 32768.0


def rms_diff_norm(pcm_a: bytes, pcm_b: bytes) -> float:
    """Normalized RMS of the sample-wise difference. A length mismatch is itself a
    divergence -> +inf (never silently truncate away real corruption)."""
    a = _samples(pcm_a)
    b = _samples(pcm_b)
    if len(a) != len(b):
        return float("inf")
    if not a:
        return 0.0
    sq = 0.0
    for i in range(len(a)):
        d = float(a[i]) - float(b[i])
        sq += d * d
    return math.sqrt(sq / len(a)) / 32768.0


def assert_determinism(pcm_a1: bytes, pcm_a2: bytes, tol: float = DETERMINISM_RMS_TOL) -> float:
    """THE determinism oracle. Raise OracleFailure if the two identical-input
    outputs diverge beyond `tol`. Returns the measured diff on success."""
    diff = rms_diff_norm(pcm_a1, pcm_a2)
    if diff > tol:
        raise OracleFailure(
            f"determinism FAILED: identical requests diverged by RMS {diff:.2e} > tol {tol:.2e} "
            f"(warm-state corruption of a repeat request)")
    return diff


def assert_distinct(pcm_a: bytes, pcm_b: bytes, min_diff: float = DISTINCT_MIN_RMS) -> float:
    """Anti-stale oracle (review suggestion 2). Raise OracleFailure if a DIFFERENT
    input produced ~the same output (a warm worker returning stale cached bytes for
    every request). Returns the measured diff on success."""
    diff = rms_diff_norm(pcm_a, pcm_b)
    if diff < min_diff:
        raise OracleFailure(
            f"distinctness FAILED: distinct requests A and B differ by only RMS {diff:.2e} "
            f"< min {min_diff:.2e} (stale cached output for every input?)")
    return diff


def assert_nonsilent(pcm: bytes, floor: float = NONSILENT_RMS_FLOOR) -> float:
    r = rms_norm(pcm)
    if r < floor:
        raise OracleFailure(f"non-silent FAILED: output RMS {r:.2e} < floor {floor:.2e} (silence)")
    return r


# ══════════════════════════════════════════════════════════════════════════════
# Worker session driver — buffer-each-OK-before-next-request discipline.
# ══════════════════════════════════════════════════════════════════════════════

class Response:
    __slots__ = ("line", "buffered", "elapsed_s")

    def __init__(self, line: str, buffered: tuple[int, bytes] | None, elapsed_s: float) -> None:
        self.line = line
        self.buffered = buffered          # (sample_rate, pcm_bytes) for an OK, else None
        self.elapsed_s = elapsed_s


def _readline_with_watchdog(stream, read_timeout_s: float) -> str | None:
    """`stream.readline()` bounded by a watchdog so a LIVE-HUNG worker (running but
    never emitting a line, never closing stdout — e.g. an MPS deadlock) cannot wedge
    the harness forever.

    The blocking readline runs on a daemon reader thread; we join with a timeout.
    Returns the line ("" on EOF) if it arrived within `read_timeout_s`, else None —
    the caller treats None as a hang, kills the worker, and fails the session. After
    the caller kills the proc on a None, the abandoned readline returns "" (EOF) and
    the daemon thread exits on its own; being a daemon it never blocks interpreter
    shutdown either way.
    """
    box: list[str] = []
    got = threading.Event()

    def _reader() -> None:
        try:
            box.append(stream.readline())
        except (ValueError, OSError):
            box.append("")   # stream closed under us (proc killed) -> treat as EOF
        finally:
            got.set()

    threading.Thread(target=_reader, daemon=True).start()
    if not got.wait(read_timeout_s):
        return None
    return box[0]


def run_worker_session(worker_cmd: list[str], requests: list[str], out_dir: str,
                       extra_env: dict[str, str] | None = None,
                       timeout_s: float = 600.0,
                       read_timeout_s: float = 600.0) -> tuple[list[Response], str, int]:
    """Spawn ONE worker process, feed `requests` one at a time, and BUFFER each
    `OK <out>`'s WAV bytes the instant the response line arrives — before the next
    request is written. Returns (responses, stderr_text, returncode).

    stderr is redirected to a file (not a pipe) so a full stderr pipe can never
    deadlock the interactive stdout read loop. Each per-request stdout read is
    additionally bounded by a `read_timeout_s` watchdog so a LIVE-HUNG worker (still
    running, never emitting a line) cannot wedge the machine-run harness forever: on
    timeout the worker is killed and the session fails with a clear HarnessError.
    """
    env = os.environ.copy()
    env["GSO_OUT_DIR"] = out_dir
    if extra_env:
        env.update(extra_env)
    os.makedirs(out_dir, exist_ok=True)
    stderr_path = os.path.join(out_dir, "_worker.stderr")

    responses: list[Response] = []
    returncode = -1
    with open(stderr_path, "wb") as errf:
        proc = subprocess.Popen(
            worker_cmd, stdin=subprocess.PIPE, stdout=subprocess.PIPE, stderr=errf,
            text=True, bufsize=1, env=env,
        )
        try:
            assert proc.stdin is not None and proc.stdout is not None
            for req in requests:
                t0 = time.monotonic()
                proc.stdin.write(req + "\n")
                proc.stdin.flush()
                line = _readline_with_watchdog(proc.stdout, read_timeout_s)
                elapsed = time.monotonic() - t0
                if line is None:
                    # LIVE-HUNG: the worker is alive but emitted no line within the
                    # budget (not a crash — that returns "" below). Kill it so the
                    # abandoned reader thread unblocks, and fail loudly.
                    proc.kill()
                    proc.wait(timeout=10)
                    raise HarnessError(
                        f"worker produced no response within {read_timeout_s:.0f}s — "
                        f"LIVE-HUNG (not dead); killed. see {stderr_path}")
                if not line:
                    raise HarnessError(
                        "worker produced no response line (crashed / FATAL exit?) — "
                        f"see {stderr_path}")
                line = line.rstrip("\n")
                buffered = None
                if line.startswith("OK "):
                    # BUFFER NOW — before the next request can overwrite a shared
                    # content-addressed path. Compare buffered bytes only, never a
                    # re-read of the path after a later request.
                    buffered = read_wav_pcm(line[3:])
                responses.append(Response(line, buffered, elapsed))
            proc.stdin.close()
            returncode = proc.wait(timeout=timeout_s)
        finally:
            if proc.poll() is None:
                proc.kill()
                proc.wait(timeout=10)

    with open(stderr_path, encoding="utf-8", errors="replace") as fh:
        stderr_text = fh.read()
    return responses, stderr_text, returncode


def _quote_request(text: str, ref_audio_path: str, prompt_text: str) -> str:
    """Build a wire line with the same shlex-quoting the Go caller (#162) must do."""
    return " ".join(shlex.quote(t) for t in
                    (text, ref_audio_path, prompt_text, SPLIT_METHOD, VOICE_ID))


def _count_load_lines(stderr_text: str) -> int:
    return sum(1 for ln in stderr_text.splitlines() if ln.startswith(f"LOAD gso {VOICE_ID}"))


# ══════════════════════════════════════════════════════════════════════════════
# Real mode (machine-gated — needs .venv-gso + real artifacts)
# ══════════════════════════════════════════════════════════════════════════════

def run_real_warmproof(models_root: str) -> int:
    worker_cmd = [os.path.join(_ROOT, "scripts", "gso")]
    voice_dir = os.path.join(models_root, VOICE_ID)
    ref_audio = os.path.join(voice_dir, "ref_audio.wav")
    transcript_path = os.path.join(voice_dir, "ref_transcript.txt")
    if not os.path.isfile(transcript_path):
        print(f"gso-warmproof: packaged transcript missing at {transcript_path} — "
              f"place the real voice artifacts first (pending-machine-run).", file=sys.stderr)
        return 2
    with open(transcript_path, encoding="utf-8") as fh:
        prompt_text = " ".join(fh.read().split())

    text_a = "Warm load proof, sentence A: the reference voice speaks the first line."
    text_b = "A different sentence B, so its audio must differ from A."
    req_a = _quote_request(text_a, ref_audio, prompt_text)
    req_b = _quote_request(text_b, ref_audio, prompt_text)

    warm_dir = tempfile.mkdtemp(prefix="gso-warmproof-warm-")
    cold_dir = tempfile.mkdtemp(prefix="gso-warmproof-cold-")  # DISTINCT from warm (suggestion 2 / round-2)

    print("gso-warmproof: feeding A, B, A to ONE warm worker (buffering each OK before the next request)...")
    responses, stderr_text, rc = run_worker_session(worker_cmd, [req_a, req_b, req_a], warm_dir)

    # Framing: 3 requests -> 3 response lines, all OK.
    if len(responses) != 3:
        raise HarnessError(f"expected 3 responses, got {len(responses)}")
    for i, r in enumerate(responses):
        if not r.line.startswith("OK ") or r.buffered is None:
            raise HarnessError(f"request {i} did not succeed: {r.line!r} (stderr: {stderr_text[:400]})")

    (sr_a1, pcm_a1) = responses[0].buffered
    (sr_b, pcm_b) = responses[1].buffered
    (sr_a2, pcm_a2) = responses[2].buffered

    # (a) LOAD once.
    n_load = _count_load_lines(stderr_text)
    if n_load != 1:
        raise OracleFailure(f"LOAD-once FAILED: saw {n_load} 'LOAD gso {VOICE_ID}' lines (expected 1)")

    # (b) non-silent + 32 kHz.
    for label, sr, pcm in (("A1", sr_a1, pcm_a1), ("B", sr_b, pcm_b), ("A2", sr_a2, pcm_a2)):
        assert_nonsilent(pcm)
        if sr != 32000:
            raise OracleFailure(f"sample-rate FAILED: {label} is {sr} Hz, expected 32000 Hz")

    # (c) determinism + distinctness.
    d_det = assert_determinism(pcm_a1, pcm_a2)
    d_dist = assert_distinct(pcm_a1, pcm_b)

    # (d) warm-vs-cold equivalence, DISTINCT out dir. Compare the genuinely-warm Nth
    #     call (pcm_a2 — the 3rd request on the warm worker), NOT pcm_a1 (which is the
    #     cold-equivalent first request), against the fresh single-shot cold process,
    #     so this oracle exercises the genuinely-warm output directly (AC5(d) / review
    #     suggestion). Determinism already proved pcm_a1 ~= pcm_a2, so this stays sound.
    cold_responses, cold_stderr, _ = run_worker_session(worker_cmd, [req_a], cold_dir)
    if len(cold_responses) != 1 or cold_responses[0].buffered is None:
        raise HarnessError(f"cold single-shot failed: {cold_responses and cold_responses[0].line!r} "
                           f"(stderr: {cold_stderr[:400]})")
    (_, pcm_cold) = cold_responses[0].buffered
    d_wvc = assert_determinism(pcm_a2, pcm_cold)

    # (e) latency — INFORMATIONAL only (NOT a gate). Warm amortization is expected;
    # request-1 pays the one-time model load.
    t1, t3 = responses[0].elapsed_s, responses[2].elapsed_s
    print(f"  [perf note, informational] request-1 {t1:.2f}s (incl. one-time LOAD) vs "
          f"warm request-3 {t3:.2f}s")

    print(f"  (a) LOAD once: OK")
    print(f"  (b) non-silent + 32 kHz: OK")
    print(f"  (c) determinism A1~=A2: OK (RMS {d_det:.2e} <= {DETERMINISM_RMS_TOL:.0e}); "
          f"distinct B!=A: OK (RMS {d_dist:.2e} >= {DISTINCT_MIN_RMS:.0e})")
    print(f"  (d) warm-vs-cold (warm A2 vs fresh cold): OK (RMS {d_wvc:.2e} <= {DETERMINISM_RMS_TOL:.0e})")
    if rc != 0:
        print(f"  [warn] worker exited {rc} after EOF (expected 0)", file=sys.stderr)
    print("gso-warmproof: PASS (warm-load correctness proven on real artifacts)")
    return 0


# ══════════════════════════════════════════════════════════════════════════════
# Negative dry-check (torch-free — runs this session; proves the oracle can fail)
# ══════════════════════════════════════════════════════════════════════════════

def _fake_session(leak_rms: float = 0.0, stale: bool = False):
    """Run A, B, A against the fake stub with a given warm-state behavior. Returns
    (pcm_a1, pcm_b, pcm_a2)."""
    stub = os.path.join(_HERE, "testdata", "fake-gso-worker.py")
    worker_cmd = [sys.executable, stub]
    req_a = _quote_request("sentence A for the oracle self-test", "/fake/ref.wav", "prompt A")
    req_b = _quote_request("a DIFFERENT sentence B for the oracle self-test", "/fake/ref.wav", "prompt B")
    out_dir = tempfile.mkdtemp(prefix="gso-warmproof-neg-")
    extra_env = {"FAKE_GSO_LEAK_RMS": repr(leak_rms)}
    if stale:
        extra_env["FAKE_GSO_STALE"] = "1"
    responses, stderr_text, _ = run_worker_session(worker_cmd, [req_a, req_b, req_a], out_dir, extra_env)
    if len(responses) != 3 or any(r.buffered is None for r in responses):
        raise HarnessError(f"fake stub did not return 3 OKs: {[r.line for r in responses]} "
                           f"(stderr: {stderr_text[:400]})")
    if _count_load_lines(stderr_text) != 1:
        raise HarnessError(f"fake stub LOAD-once broken: {stderr_text[:200]}")
    return (responses[0].buffered[1], responses[1].buffered[1], responses[2].buffered[1])


def _expect_green(name: str, fn) -> None:
    try:
        fn()
    except OracleFailure as e:
        raise OracleFailure(f"[{name}] expected GREEN but oracle went RED: {e}") from e
    print(f"  GREEN as expected: {name}")


def _expect_red(name: str, fn) -> None:
    try:
        fn()
    except OracleFailure:
        print(f"  RED as expected:   {name}")
        return
    raise OracleFailure(f"[{name}] expected RED but the oracle passed — it cannot detect this "
                        f"divergence, so a green result would be meaningless")


def run_negative_dry_check() -> int:
    print("gso-warmproof --negative-dry-check: proving the determinism/distinctness oracle "
          "can FAIL (and won't false-positive) before it is trusted...")
    print(f"  tolerances: determinism <= {DETERMINISM_RMS_TOL:.0e}, distinct >= {DISTINCT_MIN_RMS:.0e}")

    # 1. Clean worker: determinism GREEN, non-silent GREEN, distinct GREEN.
    a1, b, a2 = _fake_session(leak_rms=0.0)
    _expect_green("clean worker -> determinism (A1==A2)", lambda: assert_determinism(a1, a2))
    _expect_green("clean worker -> non-silent (A1)", lambda: assert_nonsilent(a1))
    _expect_green("clean worker -> distinct (B!=A)", lambda: assert_distinct(a1, b))

    # 2. SUBTLE near-threshold leak (~2x tol): determinism RED (review suggestion 1 —
    #    the tolerance is proven tight enough to catch subtle warm-state corruption,
    #    not only gross divergence).
    a1, _b, a2 = _fake_session(leak_rms=SUBTLE_LEAK_RMS)
    measured = rms_diff_norm(a1, a2)
    print(f"    (subtle leak injected: measured A1-vs-A2 RMS {measured:.2e}, "
          f"tol {DETERMINISM_RMS_TOL:.0e})")
    _expect_red("subtle near-threshold leak -> determinism", lambda: assert_determinism(a1, a2))

    # 3. Sub-threshold jitter (below tol): determinism GREEN — proves the tolerance
    #    is calibrated (tolerates MPS-jitter-scale noise), not absurdly tight / a
    #    guaranteed-RED that would prove nothing.
    a1, _b, a2 = _fake_session(leak_rms=SUBTHRESHOLD_JITTER_RMS)
    measured = rms_diff_norm(a1, a2)
    print(f"    (sub-threshold jitter: measured A1-vs-A2 RMS {measured:.2e}, "
          f"tol {DETERMINISM_RMS_TOL:.0e})")
    _expect_green("sub-threshold jitter -> determinism", lambda: assert_determinism(a1, a2))

    # 4. Gross leak: determinism RED.
    a1, _b, a2 = _fake_session(leak_rms=GROSS_LEAK_RMS)
    _expect_red("gross leak -> determinism", lambda: assert_determinism(a1, a2))

    # 5. Stale-cache worker (same output for every input): distinct RED (proves the
    #    bytes(B) != bytes(A) guard can actually fire — review suggestion 2), while
    #    determinism stays GREEN (a stale worker is trivially self-consistent).
    a1, b, a2 = _fake_session(stale=True)
    _expect_green("stale worker -> determinism (A1==A2)", lambda: assert_determinism(a1, a2))
    _expect_red("stale worker -> distinct (B==A)", lambda: assert_distinct(a1, b))

    print("gso-warmproof --negative-dry-check: PASS — the oracle detects subtle + gross "
          "warm-state corruption and stale-cache reuse, and tolerates sub-threshold jitter.")
    return 0


def main(argv: list[str] | None = None) -> int:
    ap = argparse.ArgumentParser(prog="gso_warmproof", description=__doc__,
                                 formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("--negative-dry-check", action="store_true",
                    help="torch-free oracle self-test (proves the oracle can fail); runs this session")
    ap.add_argument("--models-root", default=os.path.join(_ROOT, "assets", "gptsovits-models"),
                    help="root holding <voice_id>/ + _base/ (real mode only)")
    args = ap.parse_args(sys.argv[1:] if argv is None else argv)

    try:
        if args.negative_dry_check:
            return run_negative_dry_check()
        return run_real_warmproof(args.models_root)
    except OracleFailure as e:
        print(f"gso-warmproof: RED — {e}", file=sys.stderr)
        return 1
    except HarnessError as e:
        print(f"gso-warmproof: HARNESS ERROR — {e}", file=sys.stderr)
        return 2


if __name__ == "__main__":
    sys.exit(main())
