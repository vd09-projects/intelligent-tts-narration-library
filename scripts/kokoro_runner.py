#!/usr/bin/env python3
"""Kokoro TTS runner — generates a 24 kHz mono PCM s16le WAV on stdout.

Invoked by scripts/kokoro (which activates the venv). Loads the local
kokoro-onnx model + voices bundle, synthesizes the requested text with the
requested voice, and writes a standards-compliant WAV to stdout.

Native sample rate for Kokoro-82M is 24000 Hz — we do not resample.
"""

from __future__ import annotations

import argparse
import io
import sys
import wave
from pathlib import Path

import numpy as np
from kokoro_onnx import Kokoro

# Voice catalogue wired for phase one. Keep in sync with render/sherpa/README.md.
SUPPORTED_VOICES = {"af_bella", "am_michael"}
NATIVE_SAMPLE_RATE = 24000


def _project_root() -> Path:
    return Path(__file__).resolve().parent.parent


def _model_paths() -> tuple[Path, Path]:
    root = _project_root()
    return root / "models" / "kokoro-v1.0.onnx", root / "models" / "voices-v1.0.bin"


def _parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(
        prog="kokoro",
        description="Synthesize text to a 24 kHz mono PCM s16le WAV on stdout.",
    )
    parser.add_argument(
        "--voice",
        required=True,
        help=f"Voice id. One of: {', '.join(sorted(SUPPORTED_VOICES))}.",
    )
    parser.add_argument(
        "--text",
        required=True,
        help="UTF-8 text to synthesize. Must be non-empty.",
    )
    return parser.parse_args()


def _write_wav(samples: np.ndarray, sample_rate: int) -> bytes:
    """Pack float32 samples in [-1, 1] as 16-bit PCM mono WAV bytes."""
    if samples.ndim != 1:
        samples = samples.reshape(-1)
    # Clip then convert; kokoro-onnx already returns roughly normalized floats.
    clipped = np.clip(samples, -1.0, 1.0)
    pcm = (clipped * 32767.0).astype(np.int16)
    buf = io.BytesIO()
    with wave.open(buf, "wb") as wav:
        wav.setnchannels(1)
        wav.setsampwidth(2)  # 16-bit
        wav.setframerate(sample_rate)
        wav.writeframes(pcm.tobytes())
    return buf.getvalue()


def main() -> int:
    args = _parse_args()

    if args.voice not in SUPPORTED_VOICES:
        print(
            f"kokoro: unsupported voice '{args.voice}'. "
            f"Supported: {', '.join(sorted(SUPPORTED_VOICES))}.",
            file=sys.stderr,
        )
        return 2

    if not args.text.strip():
        print("kokoro: --text must be non-empty.", file=sys.stderr)
        return 2

    model_path, voices_path = _model_paths()
    if not model_path.exists() or not voices_path.exists():
        print(
            f"kokoro: model files missing. Expected:\n  {model_path}\n  {voices_path}\n"
            "See render/sherpa/README.md for download URLs.",
            file=sys.stderr,
        )
        return 3

    kokoro = Kokoro(str(model_path), str(voices_path))
    samples, sample_rate = kokoro.create(
        args.text, voice=args.voice, speed=1.0, lang="en-us"
    )

    if sample_rate != NATIVE_SAMPLE_RATE:
        # Invariant breach — surface loudly instead of resampling silently.
        print(
            f"kokoro: unexpected sample rate {sample_rate}, expected "
            f"{NATIVE_SAMPLE_RATE}. Refusing to resample.",
            file=sys.stderr,
        )
        return 4

    sys.stdout.buffer.write(_write_wav(samples, sample_rate))
    sys.stdout.buffer.flush()
    return 0


if __name__ == "__main__":
    sys.exit(main())
