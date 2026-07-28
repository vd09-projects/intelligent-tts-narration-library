#!/usr/bin/env python3
"""OFFICIAL GSO performance baseline recorder (issue #164, AC4 — perf-critical).

Records the #164 go/no-go performance baseline for the GPT-SoVITS (GSO) voice by
driving the REAL warm worker over a FIXED pre-existing document (docs/samples/sample.md)
and measuring three numbers, each against its ceiling:

  * cold load       — request 1's wall time (worker import + os.chdir(GSO_REPO) +
                      warm-load-once + the first block's synth). Ceiling 30 s.
                      INFORMATIONAL — latency is never a go/no-go gate (warm-proof
                      standing order).
  * warm per-block  — the mean wall time of requests 2..N (block 1, the cold block,
                      is EXCLUDED from the warm mean). Ceiling ~20 s. INFORMATIONAL.
  * peak RSS        — the peak resident set of the .venv-gso python worker child (the
                      torch-bearing process) sampled across the whole run. Ceiling
                      8 GB on a 16 GB M1 Pro. HARD go/no-go input — a run above 8 GB
                      is a NO-GO.

This is a SEPARATE script from scripts/gso_warmproof.py by deliberate decision (#164
S2): gso_warmproof.py is the warm-reuse CORRECTNESS oracle (A,B,A determinism /
warm-vs-cold) and stays byte-unchanged; this script is the RAM/latency MEASURER. It
imports nothing from gso_warmproof.py so the two stay independent.

PEAK-RSS SAMPLING METHOD (#164 S1 — stated in the output so the number is auditable):
  * WHICH pid: the worker child spawned via scripts/gso. scripts/gso `exec`s python3,
    so the spawned pid IS the torch-bearing python worker (no intermediate shell). The
    make/go parent process holds no model weights and is sampled once + reported as
    negligible.
  * HOW: poll `ps -o rss=` for the worker pid (plus any direct children, in case an MPS
    helper is spawned) every RSS_SAMPLE_INTERVAL_S seconds, keep the MAXIMUM. Cross-
    checked against resource.getrusage(RUSAGE_CHILDREN).ru_maxrss as an upper-bound
    sanity check after the worker exits.
  * APPLE-SILICON CAVEAT: on Apple Silicon, unified memory / MPS (Metal) allocations
    can be UNDER-counted by RSS (GPU-side buffers live outside the process RSS). So a
    near-8 GB reading is reported CONSERVATIVELY and is NEVER rounded down into a GO;
    the caveat is printed alongside the number.

Real mode needs `.venv-gso` + the real checkpoints + fetched base models + a GSO_REPO
clone. A missing worker stack STOPS non-zero (honesty rule) — #164 records AC4 as
UNSATISFIED in that case and does NOT substitute #165's smoke reading.

Usage:
    go run ./cmd/plandump --file docs/samples/sample.md > plan.json
    .venv-gso/bin/python scripts/gso_perf_baseline.py --plan plan.json
    make gso-perf-baseline                                        # the repeatable target
"""

from __future__ import annotations

import argparse
import json
import os
import platform
import resource
import subprocess
import sys
import tempfile
import threading
import time
from datetime import datetime, timezone

_HERE = os.path.dirname(os.path.abspath(__file__))
_ROOT = os.path.dirname(_HERE)

VOICE_ID = "cool-jahns-gso"
SPLIT_METHOD = "cut4"

# Ceilings (#164 Target SLO table).
COLD_CEILING_S = 30.0        # INFORMATIONAL
WARM_CEILING_S = 20.0        # INFORMATIONAL
PEAK_RSS_CEILING_GB = 8.0    # HARD go/no-go input

# RSS sampler cadence — recorded in the output so the number is reproducible.
RSS_SAMPLE_INTERVAL_S = 0.25

# Per-request stdout watchdog: a live-hung worker (running, never emitting a line —
# e.g. an MPS deadlock) must not wedge the run forever.
READ_TIMEOUT_S = 600.0

SEGMENT_KIND_SPEECH = "speech"
STATUS_REFUSED = "refused"


class BaselineError(RuntimeError):
    """Infrastructure failure — worker missing/crashed/hung, or an ERR response."""


# ── plan.json → per-block spoken text (faithful port of gptsovits.go spokenTextFor) ──
def spoken_text_for(block: dict) -> str:
    if block.get("status") == STATUS_REFUSED:
        refusal = block.get("refusal") or {}
        return (refusal.get("message") or "").strip()
    parts = [
        seg.get("text", "")
        for seg in block.get("segments", [])
        if seg.get("kind") == SEGMENT_KIND_SPEECH and seg.get("text")
    ]
    return " ".join(parts)


def block_texts(plan_path: str) -> list[str]:
    """Every non-empty block's spoken text, in plan order — exactly the blocks the GSO
    renderer would hand to the worker (empty/all-pause blocks are skipped, as in
    gptsovits.go)."""
    with open(plan_path, encoding="utf-8") as fh:
        plan = json.load(fh)
    out = []
    for b in plan.get("blocks", []):
        t = spoken_text_for(b)
        if t:
            out.append(t)
    return out


import shlex  # noqa: E402 — grouped with the wire helper it serves


def quote_request(text: str, ref_audio_path: str, prompt_text: str) -> str:
    """Build one wire line with the SAME shlex-quoting the Go caller uses."""
    return " ".join(shlex.quote(t) for t in
                    (text, ref_audio_path, prompt_text, SPLIT_METHOD, VOICE_ID))


# ── peak-RSS sampler (worker pid + direct children; keep max) ──────────────────────
def _rss_kb(pid: int) -> int:
    """RSS (KB) of pid + its DIRECT children, via ps. 0 if the pid is gone. Children
    are summed so an MPS helper the worker spawns is not missed (best-effort; the
    unified-memory caveat still applies to GPU-side memory RSS cannot see)."""
    pids = [pid]
    try:
        kids = subprocess.run(["pgrep", "-P", str(pid)], capture_output=True, text=True, timeout=5)
        pids += [int(x) for x in kids.stdout.split()]
    except (subprocess.SubprocessError, ValueError):
        pass
    total = 0
    for p in pids:
        try:
            r = subprocess.run(["ps", "-o", "rss=", "-p", str(p)],
                               capture_output=True, text=True, timeout=5)
            v = r.stdout.strip()
            if v:
                total += int(v)
        except (subprocess.SubprocessError, ValueError):
            pass
    return total


class RSSSampler(threading.Thread):
    def __init__(self, pid: int, interval_s: float) -> None:
        super().__init__(daemon=True)
        self.pid = pid
        self.interval_s = interval_s
        self.peak_kb = 0
        self._stop = threading.Event()

    def run(self) -> None:
        while not self._stop.is_set():
            self.peak_kb = max(self.peak_kb, _rss_kb(self.pid))
            self._stop.wait(self.interval_s)

    def stop(self) -> None:
        self._stop.set()
        # one final sample so a late-run peak is not missed
        self.peak_kb = max(self.peak_kb, _rss_kb(self.pid))


def _readline_with_watchdog(stream, timeout_s: float):
    """readline() bounded by a watchdog thread so a live-hung worker cannot wedge the
    run forever. Returns the line ("" on EOF) or None on timeout."""
    box: list[str] = []
    got = threading.Event()

    def _reader() -> None:
        try:
            box.append(stream.readline())
        except (ValueError, OSError):
            box.append("")
        finally:
            got.set()

    threading.Thread(target=_reader, daemon=True).start()
    if not got.wait(timeout_s):
        return None
    return box[0]


def run_baseline(plan_path: str, models_root: str) -> int:
    texts = block_texts(plan_path)
    if len(texts) < 2:
        raise BaselineError(
            f"need >=2 non-empty blocks to separate cold from warm; {plan_path} yielded {len(texts)}")

    voice_dir = os.path.join(models_root, VOICE_ID)
    ref_audio = os.path.abspath(os.path.join(voice_dir, "ref_audio.wav"))
    transcript_path = os.path.join(voice_dir, "ref_transcript.txt")
    if not os.path.isfile(transcript_path):
        raise BaselineError(
            f"packaged transcript missing at {transcript_path} — restore the real voice "
            f"artifacts (assets/gptsovits-models/README.md) before measuring AC4.")
    with open(transcript_path, encoding="utf-8") as fh:
        prompt_text = " ".join(fh.read().split())

    requests = [quote_request(t, ref_audio, prompt_text) for t in texts]

    out_dir = tempfile.mkdtemp(prefix="gso-perf-baseline-")
    stderr_path = os.path.join(out_dir, "_worker.stderr")
    worker_cmd = [os.path.join(_ROOT, "scripts", "gso")]

    env = os.environ.copy()
    env["GSO_OUT_DIR"] = out_dir

    machine = f"{platform.platform()} / {platform.machine()} / python {platform.python_version()}"
    when = datetime.now(timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")

    print(f"gso-perf-baseline: driving {len(requests)} blocks from {os.path.relpath(plan_path, _ROOT)} "
          f"through ONE warm worker")
    print(f"  machine: {machine}")
    print(f"  when:    {when}")
    print(f"  RSS sampling: poll worker pid (+direct children) every {RSS_SAMPLE_INTERVAL_S}s, keep max")

    elapsed: list[float] = []
    sampler = None
    with open(stderr_path, "wb") as errf:
        proc = subprocess.Popen(
            worker_cmd, stdin=subprocess.PIPE, stdout=subprocess.PIPE, stderr=errf,
            text=True, bufsize=1, env=env,
        )
        # scripts/gso exec's python3, so proc.pid is the torch-bearing worker pid.
        sampler = RSSSampler(proc.pid, RSS_SAMPLE_INTERVAL_S)
        sampler.start()
        try:
            assert proc.stdin is not None and proc.stdout is not None
            for idx, req in enumerate(requests):
                t0 = time.monotonic()
                proc.stdin.write(req + "\n")
                proc.stdin.flush()
                line = _readline_with_watchdog(proc.stdout, READ_TIMEOUT_S)
                dt = time.monotonic() - t0
                if line is None:
                    proc.kill()
                    proc.wait(timeout=10)
                    raise BaselineError(
                        f"worker LIVE-HUNG on block {idx} (no response in {READ_TIMEOUT_S:.0f}s); "
                        f"killed. see {stderr_path}")
                if not line:
                    raise BaselineError(
                        f"worker produced no response line on block {idx} (crashed / FATAL exit?) — "
                        f"see {stderr_path}")
                line = line.rstrip("\n")
                if not line.startswith("OK "):
                    raise BaselineError(f"block {idx} did not synthesize: {line!r} (see {stderr_path})")
                elapsed.append(dt)
                tag = "cold (incl. one-time LOAD)" if idx == 0 else "warm"
                print(f"  block {idx:>2} [{tag:<26}] {dt:6.2f}s")
            proc.stdin.close()
            rc = proc.wait(timeout=60)
        finally:
            if sampler is not None:
                sampler.stop()
            if proc.poll() is None:
                proc.kill()
                proc.wait(timeout=10)

    if rc != 0:
        print(f"  [warn] worker exited {rc} after EOF (expected 0)", file=sys.stderr)

    # ── metrics ────────────────────────────────────────────────────────────────────
    cold_s = elapsed[0]
    warm = elapsed[1:]
    warm_mean = sum(warm) / len(warm)
    warm_min, warm_max = min(warm), max(warm)

    peak_ps_gb = sampler.peak_kb / (1024.0 * 1024.0)  # ps rss is KB
    ru = resource.getrusage(resource.RUSAGE_CHILDREN).ru_maxrss
    # macOS ru_maxrss is BYTES; Linux is KB.
    ru_gb = (ru / (1024.0 ** 3)) if sys.platform == "darwin" else (ru / (1024.0 ** 2))

    def verdict(v: float, ceil: float) -> str:
        return "OK" if v <= ceil else "OVER"

    print("\n── OFFICIAL AC4 baseline (record verbatim into the AC6 entry + PR body) ──")
    print(f"  cold load        : {cold_s:6.2f}s   ceiling {COLD_CEILING_S:.0f}s   "
          f"[{verdict(cold_s, COLD_CEILING_S)}] INFORMATIONAL (not a gate)")
    print(f"  warm per-block   : {warm_mean:6.2f}s   ceiling ~{WARM_CEILING_S:.0f}s  "
          f"[{verdict(warm_mean, WARM_CEILING_S)}] INFORMATIONAL (not a gate)")
    print(f"                     (n={len(warm)} warm blocks; spread {warm_min:.2f}s..{warm_max:.2f}s; "
          f"block 1 excluded)")
    peak_gate = "GO" if peak_ps_gb <= PEAK_RSS_CEILING_GB else "NO-GO"
    print(f"  peak RSS (worker): {peak_ps_gb:6.2f}GB  ceiling {PEAK_RSS_CEILING_GB:.0f}GB  "
          f"[{peak_gate}] HARD go/no-go input")
    print(f"                     cross-check ru_maxrss(children) = {ru_gb:.2f}GB")
    print(f"  RSS method       : worker pid {proc.pid} (+children) polled every "
          f"{RSS_SAMPLE_INTERVAL_S}s, max kept")
    print("  APPLE-SILICON CAVEAT: unified-memory / MPS (GPU-side) allocations can be "
          "UNDER-counted by RSS;")
    print("                     a near-8 GB reading is reported conservatively and is "
          "NEVER rounded down into a GO.")
    if peak_ps_gb > PEAK_RSS_CEILING_GB * 0.85:
        print(f"  [attention] peak RSS {peak_ps_gb:.2f}GB is within 15% of the {PEAK_RSS_CEILING_GB:.0f}GB "
              f"ceiling — treat conservatively per the caveat.")
    print("\ngso-perf-baseline: DONE (numbers above are the OFFICIAL #164 baseline; "
          "latency informational, peak-RSS is the go/no-go input).")
    return 0


def main(argv: list[str] | None = None) -> int:
    ap = argparse.ArgumentParser(
        prog="gso_perf_baseline",
        description=__doc__,
        formatter_class=argparse.RawDescriptionHelpFormatter,
    )
    ap.add_argument("--plan", required=True,
                    help="plan.json from `go run ./cmd/plandump --file docs/samples/sample.md`")
    ap.add_argument("--models-root", default=os.path.join(_ROOT, "assets", "gptsovits-models"),
                    help="root holding <voice_id>/ + _base/")
    args = ap.parse_args(sys.argv[1:] if argv is None else argv)

    try:
        return run_baseline(args.plan, args.models_root)
    except BaselineError as e:
        print(f"gso-perf-baseline: AC4 UNSATISFIED — {e}", file=sys.stderr)
        return 2


if __name__ == "__main__":
    sys.exit(main())
