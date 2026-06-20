#!/usr/bin/env python3
"""Generate a 24 kHz mono PCM-16 silent WAV file.

The bundled fixture audio.wav must be a valid 24 kHz mono WAV so the player's
<audio> element plays it without resampling (CLAUDE.md "24 kHz mono PCM/WAV").
We synthesize ~2 s of silence rather than committing recorded audio — the
fixture is exercising plan + manifest + UI plumbing, not voice quality.

Default output:
    player/public/fixtures/sample/audio.wav

Usage:
    python3 player/public/fixtures/make_silent_wav.py [DURATION_SECONDS]
                                                     [OUTPUT_PATH]

Used by `make player-fixture-silent`. The Makefile target is the entry point;
this script is the implementation.
"""
import struct
import sys
import wave
from pathlib import Path

SAMPLE_RATE = 24_000
CHANNELS = 1
SAMPLE_WIDTH_BYTES = 2  # PCM-16


def main(argv: list[str]) -> int:
    duration_s = float(argv[1]) if len(argv) > 1 else 2.0
    out = Path(argv[2]) if len(argv) > 2 else (
        Path(__file__).parent / "sample" / "audio.wav"
    )
    out.parent.mkdir(parents=True, exist_ok=True)

    n_samples = int(SAMPLE_RATE * duration_s)
    silence = struct.pack("<" + "h" * n_samples, *([0] * n_samples))

    with wave.open(str(out), "wb") as wf:
        wf.setnchannels(CHANNELS)
        wf.setsampwidth(SAMPLE_WIDTH_BYTES)
        wf.setframerate(SAMPLE_RATE)
        wf.writeframes(silence)

    print(f"wrote {out} ({duration_s:g}s, {SAMPLE_RATE} Hz mono PCM-16)")
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv))
