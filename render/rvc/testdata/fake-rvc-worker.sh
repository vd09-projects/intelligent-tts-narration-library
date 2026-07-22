#!/usr/bin/env bash
# Fake torch-free RVC worker for render/rvc tests. Mirrors the real
# scripts/rvc_worker.py wire contract closely enough for the decorator tests —
# NO torch, NO .venv-rvc, NO exported models, synthetic silence only.
#
# Protocol (frozen v1): reads stdin line by line, shlex-splits EXACTLY 5 tokens
#   <in> <out> <voice> <index_rate> <pitch>
# and replies with exactly ONE line per request:
#   OK <out>                     (echoes the real out path VERBATIM, incl. spaces)
#   ERR <category> <message>     (closed set: bad-args|bad-voice|read-failed|
#                                 infer-failed|write-failed)
# Emits the three warm-load-once lines to stderr, in the REAL worker's format
# `LOAD <kind> <slug>` (LOAD shared _shared at spawn; LOAD net_g <slug> / LOAD
# index <slug> on first use of a voice), each exactly once.
#
# Test-only env knobs (the real worker has NONE of these):
#   FAKE_RVC_FATAL_STARTUP=1   → FATAL + exit 78 before serving (EXIT_FATAL_STARTUP)
#   FAKE_RVC_EXIT2=1           → exit 2 before serving (wrapper venv-missing shape)
#   FAKE_RVC_FATAL_RUNTIME=1   → FATAL + exit 70 on the first request (EXIT_FATAL_RUNTIME)
#   FAKE_RVC_ERR=<category>    → reply `ERR <category> ...` to every request
#   FAKE_RVC_BAD_OK=1          → reply `OK /wrong/path.wav` (OK path mismatch)
#   FAKE_RVC_GARBAGE=1         → reply an unparseable line (protocol violation)
#   FAKE_RVC_EOF=1             → close stdout + exit 0 without replying (short count)
#   FAKE_RVC_NONALIGN=1        → write a NON-whole-ms sample count (exercises frame-align)
#   FAKE_RVC_SLEEP_MS=<n>      → sleep n ms before replying (timeout / cancel test)
#   FAKE_RVC_ARGS_LOG=<path>   → append each raw request line to <path> (arg assertions)

set -euo pipefail

# Read the Python program from fd 3 (the heredoc) so stdin (fd 0) stays connected
# to the real request pipe — `python3 - <<HEREDOC` would consume stdin as the
# program source and leave sys.stdin empty. `exec` makes the worker BE the python
# process (mirrors scripts/rvc), so a kill on cancel/timeout reaps it directly.
exec python3 /dev/fd/3 "$@" 3<<'PY'
import os, sys, shlex, struct, time

def log(msg):
    sys.stderr.write(msg + "\n")
    sys.stderr.flush()

def emit(msg):
    sys.stdout.write(msg + "\n")
    sys.stdout.flush()

# --- startup arm (banner + shared load, or a forced fatal) ---
log("torch imported at inference?: False")

if os.environ.get("FAKE_RVC_EXIT2") == "1":
    log("rvc: torch-free venv missing — run 'make rvc-worker-venv'")
    sys.exit(2)

if os.environ.get("FAKE_RVC_FATAL_STARTUP") == "1":
    log("FATAL RuntimeError: fake startup failure (missing exported artifacts)")
    sys.exit(78)

log("LOAD shared _shared")

ROSTER = {"cool-jahns", "confident-neal"}
args_log = os.environ.get("FAKE_RVC_ARGS_LOG")
forced_err = os.environ.get("FAKE_RVC_ERR")
bad_ok = os.environ.get("FAKE_RVC_BAD_OK") == "1"
garbage = os.environ.get("FAKE_RVC_GARBAGE") == "1"
eof = os.environ.get("FAKE_RVC_EOF") == "1"
nonalign = os.environ.get("FAKE_RVC_NONALIGN") == "1"
fatal_runtime = os.environ.get("FAKE_RVC_FATAL_RUNTIME") == "1"
sleep_ms = os.environ.get("FAKE_RVC_SLEEP_MS")

loaded_netg = set()
loaded_index = set()

def write_wav(path, samples):
    sample_rate, channels, bits = 40000, 1, 16
    data_bytes = samples * 2
    byte_rate = sample_rate * channels * bits // 8
    block_align = channels * bits // 8
    riff = b"RIFF" + struct.pack("<I", 36 + data_bytes) + b"WAVE"
    fmt = b"fmt " + struct.pack("<IHHIIHH", 16, 1, channels, sample_rate, byte_rate, block_align, bits)
    data = b"data" + struct.pack("<I", data_bytes) + (b"\x00" * data_bytes)
    with open(path, "wb") as f:
        f.write(riff + fmt + data)

for line in sys.stdin:
    line = line.rstrip("\n")

    if args_log:
        with open(args_log, "a") as f:
            f.write(line + "\n")

    if fatal_runtime:
        log("FATAL MemoryError: fake runtime fatal")
        sys.exit(70)

    if eof:
        # Simulate a worker that dies/closes stdout mid-batch without replying.
        sys.exit(0)

    try:
        parts = shlex.split(line)
    except ValueError as e:
        emit(f"ERR bad-args unparseable line: {e}")
        continue
    if len(parts) != 5:
        emit(f"ERR bad-args expected 5 tokens, got {len(parts)}")
        continue
    in_path, out_path, voice, index_rate_s, pitch_s = parts

    if garbage:
        emit("GARBAGE not-a-valid-response")
        continue
    if forced_err:
        emit(f"ERR {forced_err} forced by test")
        continue

    # --- validation, mirroring the real worker's closed ERR set ---
    if voice not in ROSTER:
        emit(f"ERR bad-voice unknown voice '{voice}'")
        continue
    try:
        index_rate = float(index_rate_s)
    except ValueError:
        emit(f"ERR bad-args index_rate '{index_rate_s}' is not a float")
        continue
    if not (0.0 <= index_rate <= 1.0):
        emit(f"ERR bad-args index_rate {index_rate} out of range [0,1]")
        continue
    try:
        pitch = int(pitch_s)
    except ValueError:
        emit(f"ERR bad-args pitch '{pitch_s}' is not an int")
        continue
    if pitch != 0:
        emit("ERR bad-args pitch must be 0 in phase one")
        continue
    if not os.path.exists(in_path):
        emit(f"ERR read-failed {in_path}: no such file")
        continue

    # warm-load-once per voice (net_g + index), like the real worker's lazy load.
    if voice not in loaded_netg:
        log(f"LOAD net_g {voice}")
        loaded_netg.add(voice)
    if voice not in loaded_index:
        log(f"LOAD index {voice}")
        loaded_index.add(voice)

    if sleep_ms:
        time.sleep(int(sleep_ms) / 1000.0)

    # 8000 samples = 200 ms at 40 kHz (whole-ms). NONALIGN uses 8017 samples so
    # the data chunk is NOT a whole-ms multiple, exercising the frame-aligner.
    samples = 8017 if nonalign else 8000
    try:
        write_wav(out_path, samples)
    except Exception as e:  # noqa: BLE001
        emit(f"ERR write-failed {out_path}: {e}")
        continue

    if bad_ok:
        emit("OK /wrong/path.wav")
    else:
        emit(f"OK {out_path}")
PY
