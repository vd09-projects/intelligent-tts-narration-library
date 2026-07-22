#!/usr/bin/env python3
"""Generate the committed RVC parity gate assets (issue #144, plan Step 0a).

The full-pipeline log-mel gate compares the torch-free ONNX worker output against
a TORCH-reference target that is committed in-repo, so `make rvc-parity` always
runs the perceptual gate (never env-skipped — review blocker B2). This script
produces those committed assets ONCE from the Applio torch path:

  tests/rvc_parity/fixtures/
    source.wav                    the fixed source clip (input to both paths)
    <slug>_ref.wav                torch-reference output (Applio rvc-convert.sh)
    <slug>_logmel_target.npy      pinned log-mel of <slug>_ref.wav (the assertion target)

Run under the venv that has librosa (.venv-rvc). Torch reference generation shells
out to assets/rvc-models/rvc-convert.sh, which uses the SEPARATE Applio torch venv
(this script never imports torch).

Usage:
    .venv-rvc/bin/python tests/rvc_parity/gen_targets.py --source <clip.wav>
    # or, if source.wav is already committed:
    .venv-rvc/bin/python tests/rvc_parity/gen_targets.py

After running, `git add tests/rvc_parity/fixtures/` and commit — these are the
gate assets a fresh clone needs to run `make rvc-parity`.
"""

from __future__ import annotations

import argparse
import os
import shutil
import subprocess
import sys

import numpy as np
import soundfile as sf

from logmel import log_mel, LOGMEL_SR

_HERE = os.path.dirname(os.path.abspath(__file__))
_REPO = os.path.dirname(os.path.dirname(_HERE))
FIXTURES = os.path.join(_HERE, "fixtures")
RVC_CONVERT = os.path.join(_REPO, "assets", "rvc-models", "rvc-convert.sh")
VOICES = ("cool-jahns", "confident-neal")


def _ensure_source(source_arg: str | None) -> str:
    """Resolve + commit the fixed source clip into fixtures/source.wav."""
    dst = os.path.join(FIXTURES, "source.wav")
    if source_arg:
        os.makedirs(FIXTURES, exist_ok=True)
        if os.path.abspath(source_arg) != os.path.abspath(dst):
            shutil.copyfile(source_arg, dst)
    if not os.path.isfile(dst):
        sys.exit(
            "no source clip: pass --source <clip.wav> once so it is committed as "
            f"{dst} (a short spoken clip, e.g. a kokoro render or assets/huginn-samples/*.wav)"
        )
    return dst


def _torch_reference(slug: str, source: str) -> str:
    """Shell out to the Applio torch path to produce the reference WAV."""
    ref = os.path.join(FIXTURES, f"{slug}_ref.wav")
    if not os.path.isfile(RVC_CONVERT):
        sys.exit(f"missing {RVC_CONVERT} — cannot generate the torch reference")
    print(f"[{slug}] generating torch reference via rvc-convert.sh ...", file=sys.stderr)
    subprocess.run(["bash", RVC_CONVERT, slug, source, ref], check=True)
    return ref


def _logmel_target(slug: str, ref_wav: str) -> str:
    out = os.path.join(FIXTURES, f"{slug}_logmel_target.npy")
    audio, sr = sf.read(ref_wav, dtype="float32", always_2d=False)
    if audio.ndim > 1:
        audio = audio.mean(axis=1)
    if sr != LOGMEL_SR:
        sys.exit(f"[{slug}] reference WAV is {sr} Hz, expected {LOGMEL_SR}")
    target = log_mel(audio.astype(np.float32), sr)
    np.save(out, target)
    print(f"[{slug}] wrote {out}  shape={target.shape}", file=sys.stderr)
    return out


def main() -> int:
    ap = argparse.ArgumentParser(description="Generate committed RVC parity gate assets.")
    ap.add_argument("--source", help="source clip to commit as fixtures/source.wav (once)")
    ap.add_argument("--voice", choices=VOICES, help="only this voice (default: both)")
    args = ap.parse_args()

    source = _ensure_source(args.source)
    voices = (args.voice,) if args.voice else VOICES
    for slug in voices:
        ref = _torch_reference(slug, source)
        _logmel_target(slug, ref)
    print("done. git add tests/rvc_parity/fixtures/ and commit the gate assets.", file=sys.stderr)
    return 0


if __name__ == "__main__":
    sys.exit(main())
