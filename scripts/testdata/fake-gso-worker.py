#!/usr/bin/env python3
"""Torch-free FAKE GSO worker stub — for the warmproof harness self-test only.

Mirrors the render/sherpa/testdata/fake-kokoro.sh idiom (a fake that speaks the real
wire contract) but for the GSO stdin/stdout line loop. It reimplements JUST enough of
the wire to be a controllable stand-in worker for `scripts/gso_warmproof.py`'s
NEGATIVE DRY-CHECK — it is NOT a GPT-SoVITS implementation and never imports torch.

It deliberately reuses the REAL worker's `mint_out_path` + `_atomic_write_wav` +
`_resolve_out_base` (all torch-free), so the harness exercises the genuine
content-addressed path minting and atomic WAV write — the same code paths the real
worker uses — while the "audio" is a deterministic pseudo-waveform derived from the
request hash (identical request -> identical audio; distinct request -> distinct
audio, so the harness's bytes(B) != bytes(A) check is meaningful).

WARM-STATE-LEAK SIMULATION (the whole point of the self-test):
  FAKE_GSO_LEAK_RMS=<float>   On the SECOND (and later) occurrence of an IDENTICAL
                             request, perturb the pseudo-audio so its normalized RMS
                             difference from the first occurrence is ~this value.
                             0 (default) = perfectly deterministic warm reuse (clean
                             worker). >0 = a warm worker whose cached/leaked state
                             silently corrupts repeat requests — what AC5 must catch.

  FAKE_GSO_STALE=1           A warm worker returning the SAME cached output for EVERY
                             request regardless of input. Distinct requests then yield
                             identical bytes — the failure mode review-suggestion 2's
                             bytes(B) != bytes(A) check exists to catch. Lets the
                             self-test prove that guard can actually fire.

  Because the worker mints a CONTENT-ADDRESSED path, the first and second identical
  requests write to the SAME file (the second atomically overwrites the first). The
  harness must buffer the first response's bytes before issuing the second, or it
  self-compares and false-greens — this stub is how that discipline is proven.

Env is read fresh each request so a single warm process can leak deterministically.
Prints `LOAD gso <voice>` to stderr exactly once (first request) — the warm-load
proof the harness also asserts.
"""

from __future__ import annotations

import array
import hashlib
import os
import random
import shlex
import sys

_HERE = os.path.dirname(os.path.abspath(__file__))
_SCRIPTS = os.path.dirname(_HERE)
sys.path.insert(0, _SCRIPTS)

# Reuse the REAL torch-free helpers so the self-test exercises genuine minting/write.
from gptsovits_worker import (  # noqa: E402
    _atomic_write_wav,
    _resolve_out_base,
    mint_out_path,
)

SR = 32000            # match the real worker's v2Pro rate
N_SAMPLES = 6400      # 0.2 s — short but non-silent
BASE_SCALE = 0.30     # base amplitude (full-scale fraction); leaves headroom for perturb
FULL = 32768.0


def _canonical(voice_id: str, text: str, prompt_text: str,
               ref_audio_path: str, split_method: str) -> str:
    return "\x00".join((voice_id, text, prompt_text, ref_audio_path, split_method))


def _pseudo_audio(canonical: str) -> array.array:
    """Deterministic non-silent int16 waveform seeded by the request hash."""
    seed = int(hashlib.sha256(canonical.encode("utf-8")).hexdigest()[:12], 16)
    rng = random.Random(seed)
    amp = BASE_SCALE * FULL
    return array.array("h", (int(amp * (rng.random() * 2.0 - 1.0)) for _ in range(N_SAMPLES)))


def _perturb(base: array.array, target_rms_norm: float, seed: int) -> array.array:
    """Return a copy of `base` perturbed so the normalized RMS of the difference is
    ~target_rms_norm. Each delta is +/- (target_rms_norm * FULL); constant magnitude
    means RMS(delta)/FULL == target_rms_norm exactly (before clipping)."""
    rng = random.Random(seed ^ 0x5EED)
    mag = int(round(target_rms_norm * FULL))
    out = array.array("h", base)
    for i in range(len(out)):
        delta = mag if rng.random() < 0.5 else -mag
        v = out[i] + delta
        out[i] = max(-32768, min(32767, v))
    return out


def main() -> int:
    seen: dict[str, int] = {}
    out_base = _resolve_out_base()
    print(f"OUT_BASE {out_base}", file=sys.stderr, flush=True)
    loaded = False

    for line in sys.stdin:
        line = line.rstrip("\n")
        parts = shlex.split(line)
        if len(parts) != 5:
            sys.stdout.write("ERR bad-args fake: expected 5 tokens\n")
            sys.stdout.flush()
            continue
        text, ref_audio_path, prompt_text, split_method, voice_id = parts

        if not loaded:
            print(f"LOAD gso {voice_id}", file=sys.stderr, flush=True)
            loaded = True

        canonical = _canonical(voice_id, text, prompt_text, ref_audio_path, split_method)
        occurrence = seen.get(canonical, 0)
        seen[canonical] = occurrence + 1

        # Stale-cache worker: identical output for every input (input ignored).
        audio_key = "\x00STALE\x00" if os.environ.get("FAKE_GSO_STALE") == "1" else canonical
        audio = _pseudo_audio(audio_key)
        leak_rms = float(os.environ.get("FAKE_GSO_LEAK_RMS", "0") or "0")
        if occurrence >= 1 and leak_rms > 0.0:
            # A warm worker corrupting request 2+ from request-1's leaked state.
            audio = _perturb(audio, leak_rms, seed=occurrence)

        out_path = mint_out_path(out_base, voice_id, text, prompt_text,
                                 ref_audio_path, split_method)
        _atomic_write_wav(SR, audio.tobytes(), out_path)  # real atomic write
        # OK only AFTER the write (same happens-before the real worker guarantees).
        sys.stdout.write(f"OK {out_path}\n")
        sys.stdout.flush()

    return 0


if __name__ == "__main__":
    try:
        sys.exit(main())
    except BrokenPipeError:
        sys.exit(0)
    except KeyboardInterrupt:
        sys.exit(130)
