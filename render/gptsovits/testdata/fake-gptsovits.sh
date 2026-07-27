#!/usr/bin/env bash
# Fake torch-free GPT-SoVITS (GSO) worker for render/gptsovits tests. Mirrors the
# fake-kokoro.sh idiom (a shell fake that shells to python for the WAV) but speaks
# the frozen #161 GSO stdin/stdout LINE LOOP — NO torch, NO .venv-gso, NO real
# checkpoints, synthetic 32 kHz silence only. Drift oracle: the real wire contract
# is pinned by scripts/gso_worker_contract_test.py; keep this fake in step with it.
#
# Protocol (frozen #161): reads stdin line by line, shlex-splits EXACTLY 5 tokens
#   <text> <ref_audio_path> <prompt_text> <text_split_method> <voice_id>
# There is NO <out> token — the worker MINTS a content-addressed path under
# GSO_OUT_DIR and echoes it verbatim. Replies exactly ONE line per request:
#   OK <out>                     (the worker-minted path, verbatim, incl. spaces)
#   ERR <category> <message>     (closed set: bad-args|bad-voice|read-failed|
#                                 infer-failed|write-failed)
# Emits `LOAD gso <voice_id>` (once, first request) + `OUT_BASE <dir>` to stderr,
# matching the real worker's warm-load-once proof. Reads GSO_OUT_DIR from the env the
# engine set on the child (cmd.Env = append(os.Environ(), "GSO_OUT_DIR=...")).
#
# Test-only env knobs (the real worker has NONE of these):
#   FAKE_GSO_FATAL_STARTUP=1  → FATAL + exit 78 before serving (EXIT_FATAL_STARTUP)
#   FAKE_GSO_EXIT2=1          → exit 2 before serving (wrapper venv-missing shape)
#   FAKE_GSO_FATAL_RUNTIME=1  → FATAL + exit 70 on the first request (EXIT_FATAL_RUNTIME)
#   FAKE_GSO_ERR=<category>   → reply `ERR <category> ...` to every request
#   FAKE_GSO_EMPTY_OK=1       → reply `OK ` with an EMPTY path (protocol violation)
#   FAKE_GSO_GARBAGE=1        → reply an unparseable line (protocol violation)
#   FAKE_GSO_EOF=1            → close stdout + exit 0 without replying (short count)
#   FAKE_GSO_NONALIGN=1       → write a NON-whole-ms sample count (exercises frame-align)
#   FAKE_GSO_DUP_REQUEST=1    → write a SHORTER clip on the 2nd+ identical request, so
#                              a mint-path overwrite is observable (D4 buffer proof)
#   FAKE_GSO_SLEEP_MS=<n>     → sleep n ms before replying (timeout / cancel test)
#   FAKE_GSO_ARGS_LOG=<path>  → append each raw request line to <path> (arg assertions)
#   GSO_INHERIT_PROBE=<v>     → echo `INHERIT <v>` to stderr at startup (env-inherit test)

set -euo pipefail

# Read the Python program from fd 3 (the heredoc) so stdin (fd 0) stays connected to
# the real request pipe. `exec` makes the worker BE the python process (mirrors
# scripts/gso), so a kill on cancel/timeout reaps it directly.
exec python3 /dev/fd/3 "$@" 3<<'PY'
import hashlib
import os
import shlex
import struct
import sys
import tempfile
import time

SR = 32000  # v2Pro native rate — 32 kHz mono s16le, never resampled.


def log(msg):
    sys.stderr.write(msg + "\n")
    sys.stderr.flush()


def emit(msg):
    sys.stdout.write(msg + "\n")
    sys.stdout.flush()


# --- startup arm (forced fatals fire before any request is serviced) ---
if os.environ.get("FAKE_GSO_EXIT2") == "1":
    log("gso: torch venv missing at .venv-gso — run 'make gso-worker-venv'")
    sys.exit(2)

if os.environ.get("FAKE_GSO_FATAL_STARTUP") == "1":
    log("FATAL torch is not importable — .venv-gso is broken")
    sys.exit(78)

out_base = os.environ.get("GSO_OUT_DIR")
if out_base:
    os.makedirs(out_base, exist_ok=True)
    out_base = os.path.abspath(out_base)
else:
    out_base = tempfile.mkdtemp(prefix="fake-gso-")
log(f"OUT_BASE {out_base}")

probe = os.environ.get("GSO_INHERIT_PROBE")
if probe:
    log(f"INHERIT {probe}")

args_log = os.environ.get("FAKE_GSO_ARGS_LOG")
forced_err = os.environ.get("FAKE_GSO_ERR")
empty_ok = os.environ.get("FAKE_GSO_EMPTY_OK") == "1"
garbage = os.environ.get("FAKE_GSO_GARBAGE") == "1"
eof = os.environ.get("FAKE_GSO_EOF") == "1"
nonalign = os.environ.get("FAKE_GSO_NONALIGN") == "1"
dup_request = os.environ.get("FAKE_GSO_DUP_REQUEST") == "1"
fatal_runtime = os.environ.get("FAKE_GSO_FATAL_RUNTIME") == "1"
sleep_ms = os.environ.get("FAKE_GSO_SLEEP_MS")

loaded = False
seen = {}  # canonical -> occurrence count (D4 overwrite simulation)


def mint_out_path(voice_id, text, prompt_text, ref_audio_path, split_method):
    canonical = "\x00".join((voice_id, text, prompt_text, ref_audio_path, split_method))
    digest = hashlib.sha256(canonical.encode("utf-8")).hexdigest()[:16]
    return os.path.join(out_base, f"gso-{voice_id}-{digest}.wav")


def write_wav(path, samples):
    data_bytes = samples * 2
    byte_rate = SR * 1 * 16 // 8
    riff = b"RIFF" + struct.pack("<I", 36 + data_bytes) + b"WAVE"
    fmt = b"fmt " + struct.pack("<IHHIIHH", 16, 1, 1, SR, byte_rate, 2, 16)
    data = b"data" + struct.pack("<I", data_bytes) + (b"\x00" * data_bytes)
    tmp = f"{path}.tmp.{os.getpid()}"
    with open(tmp, "wb") as f:
        f.write(riff + fmt + data)
    os.replace(tmp, path)  # atomic — OK is emitted only after this returns.


for line in sys.stdin:
    line = line.rstrip("\n")

    if fatal_runtime:
        log("FATAL MemoryError: fake runtime fatal")
        sys.exit(70)

    if eof:
        # Worker dies / closes stdout mid-batch without replying.
        sys.exit(0)

    if args_log:
        with open(args_log, "a") as f:
            f.write(line + "\n")

    try:
        parts = shlex.split(line)
    except ValueError as e:
        emit(f"ERR bad-args unparseable line: {e}")
        continue
    if len(parts) != 5:
        emit(f"ERR bad-args expected 5 tokens, got {len(parts)}")
        continue
    text, ref_audio_path, prompt_text, split_method, voice_id = parts

    if not loaded:
        log(f"LOAD gso {voice_id}")
        loaded = True

    if garbage:
        emit("GARBAGE not-a-valid-response")
        continue
    if empty_ok:
        emit("OK ")
        continue
    if forced_err:
        emit(f"ERR {forced_err} forced by test")
        continue

    canonical = "\x00".join((voice_id, text, prompt_text, ref_audio_path, split_method))
    occurrence = seen.get(canonical, 0)
    seen[canonical] = occurrence + 1

    # 6400 samples = 200 ms at 32 kHz (whole-ms). NONALIGN uses 6417 so the data
    # chunk is NOT a whole-ms multiple (exercises the frame-aligner). DUP_REQUEST
    # writes a shorter 150 ms clip on the 2nd+ identical request so a mint-path
    # overwrite changes the bytes — the engine must have buffered the first (D4).
    samples = 6400
    if nonalign:
        samples = 6417
    if dup_request and occurrence >= 1:
        samples = 4800  # 150 ms — distinct from the first occurrence's 200 ms.

    out_path = mint_out_path(voice_id, text, prompt_text, ref_audio_path, split_method)

    if sleep_ms:
        time.sleep(int(sleep_ms) / 1000.0)

    try:
        write_wav(out_path, samples)
    except Exception as e:  # noqa: BLE001
        emit(f"ERR write-failed {out_path}: {e}")
        continue

    # OK carries the LITERAL minted path (line[3:] for #162) — never shlex-quoted.
    emit(f"OK {out_path}")
PY
