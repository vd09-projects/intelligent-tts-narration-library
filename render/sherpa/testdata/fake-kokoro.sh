#!/usr/bin/env bash
# Fake Kokoro wrapper for sherpa_test.go.
#
# Mirrors the real scripts/kokoro contract closely enough for renderer tests:
#   --voice VOICE   required; checked against {af_bella, am_michael}
#   --text  TEXT    required; must be non-empty
# stdout: a 44-byte standard WAV header + 4800 frames of silence
#         (200 ms at 24 kHz mono s16le → 9600 data bytes → 200 ms exactly).
# stderr: diagnostic messages on failure.
#
# Env knobs the real wrapper does NOT have — used only by tests:
#   FAKE_KOKORO_FAIL=1            → exit 1 with "fake: forced fail" stderr
#   FAKE_KOKORO_SLEEP_MS=NUM      → sleep before emitting (timeout test)
#   FAKE_KOKORO_ARGS_LOG=PATH     → append "voice|text" to PATH (assertions)

set -euo pipefail

VOICE=""
TEXT=""
while [[ $# -gt 0 ]]; do
  case "$1" in
    --voice) VOICE="$2"; shift 2 ;;
    --text)  TEXT="$2";  shift 2 ;;
    *) echo "fake: unknown arg $1" >&2; exit 2 ;;
  esac
done

if [[ -n "${FAKE_KOKORO_ARGS_LOG:-}" ]]; then
  printf '%s|%s\n' "$VOICE" "$TEXT" >> "$FAKE_KOKORO_ARGS_LOG"
fi

case "$VOICE" in
  af_bella|am_michael) ;;
  "")     echo "fake: --voice required" >&2; exit 2 ;;
  *)      echo "kokoro: unsupported voice '${VOICE}'." >&2; exit 2 ;;
esac

if [[ -z "$TEXT" ]]; then
  echo "kokoro: --text must be non-empty." >&2
  exit 2
fi

if [[ "${FAKE_KOKORO_FAIL:-}" == "1" ]]; then
  echo "fake: forced fail" >&2
  exit 1
fi

if [[ -n "${FAKE_KOKORO_SLEEP_MS:-}" ]]; then
  # bash sleep accepts floats on most coreutils; convert ms→s
  python3 -c "import time,sys; time.sleep(int(sys.argv[1])/1000.0)" "$FAKE_KOKORO_SLEEP_MS"
fi

# Emit a minimal valid 44-byte WAV header + 9600 zero data bytes
# (200 ms at 24 kHz mono s16le).
#
# RIFF header (12 bytes) + fmt chunk (24 bytes) + data header (8 bytes) = 44.
# Total file size = 44 + 9600 = 9644 bytes.

python3 - <<'PY'
import struct, sys

sample_rate = 24000
channels = 1
bits = 16
data_bytes = 9600  # 4800 frames * 2 bytes = 200 ms
byte_rate = sample_rate * channels * bits // 8
block_align = channels * bits // 8

riff = b'RIFF' + struct.pack('<I', 36 + data_bytes) + b'WAVE'
fmt  = b'fmt ' + struct.pack('<IHHIIHH', 16, 1, channels, sample_rate, byte_rate, block_align, bits)
data = b'data' + struct.pack('<I', data_bytes) + (b'\x00' * data_bytes)

sys.stdout.buffer.write(riff + fmt + data)
sys.stdout.buffer.flush()
PY
