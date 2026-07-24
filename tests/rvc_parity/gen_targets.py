#!/usr/bin/env python3
"""Generate the RVC parity gate assets from the Applio torch path (issue #144/#151).

The full-pipeline log-mel gate compares the torch-free ONNX worker output against
a TORCH-reference target. This script produces those assets from the Applio torch
path (it never imports torch — it shells out to rvc-convert.sh):

  tests/rvc_parity/fixtures/
    source.wav                    the fixed source clip (input to both paths)
    <slug>_ref.wav                torch-reference output (Applio rvc-convert.sh)
    <slug>_logmel_target.npy      pinned log-mel of <slug>_ref.wav (the assertion target)

Run under the venv that has librosa (.venv-rvc).

Two roles (issue #151):
  * LOCAL regen (ungated) — `--voice <slug>` or default (all VOICES). Produces
    fixtures on your machine; they are gitignored (the repo forbids .wav/.npy
    binaries and the source may be personal audio). This is NOT the hosted bundle.
  * BUNDLE regen (`--bundle`) — emits exactly PARITY_VOICES (the single-voice
    hosted set). `make rvc-fixtures-publish` calls this, then pins + uploads the
    result. Fresh-clone reproducibility runs `make rvc-fixtures-fetch` to pull the
    hosted bundle — nothing is committed to git.

Usage:
    .venv-rvc/bin/python tests/rvc_parity/gen_targets.py --source <clip.wav>            # local, all voices
    .venv-rvc/bin/python tests/rvc_parity/gen_targets.py --source <clip.wav> --bundle   # hosted bundle set
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
from parity_voices import PARITY_VOICES  # single source of truth for the hosted set

_HERE = os.path.dirname(os.path.abspath(__file__))
_REPO = os.path.dirname(os.path.dirname(_HERE))
FIXTURES = os.path.join(_HERE, "fixtures")
RVC_CONVERT = os.path.join(_REPO, "assets", "rvc-models", "rvc-convert.sh")
# Every voice this generator can produce LOCALLY (ungated). The hosted bundle set
# is the narrower PARITY_VOICES (imported above) — see fixtures/README.md.
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
    ap = argparse.ArgumentParser(
        description="Generate RVC parity gate assets from the Applio torch path.")
    ap.add_argument("--source", help="source clip to place as fixtures/source.wav (once)")
    ap.add_argument("--voice", choices=VOICES,
                    help="only this voice (local regen; default: all VOICES)")
    ap.add_argument("--bundle", action="store_true",
                    help="regenerate exactly the hosted PARITY_VOICES set "
                         "(used by `make rvc-fixtures-publish`)")
    args = ap.parse_args()

    if args.bundle and args.voice:
        sys.exit("--bundle and --voice are mutually exclusive "
                 "(--bundle emits exactly PARITY_VOICES)")

    source = _ensure_source(args.source)
    if args.bundle:
        voices = tuple(PARITY_VOICES)
    elif args.voice:
        voices = (args.voice,)
    else:
        voices = VOICES
    for slug in voices:
        ref = _torch_reference(slug, source)
        _logmel_target(slug, ref)

    names = ", ".join(voices)
    if args.bundle:
        print(f"done (bundle set: {names}). Next: `make rvc-fixtures-publish` pins "
              "fixtures.sha256 + uploads the release; fixtures stay OUT of git.",
              file=sys.stderr)
    else:
        print(f"done (local: {names}). Fixtures are gitignored (not committed); they "
              "just need to exist before `make rvc-parity`. The hosted bundle is "
              "produced by `make rvc-fixtures-publish` (--bundle).", file=sys.stderr)
    return 0


if __name__ == "__main__":
    sys.exit(main())
