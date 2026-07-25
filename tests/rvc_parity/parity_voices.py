#!/usr/bin/env python3
"""Single source of truth for the RVC parity voice matrix (issue #151).

Why this module exists
----------------------
The full-pipeline log-mel gate (`make rvc-parity` -> parity_test.py) is a PIPELINE
FLOW gate, not a per-voice correctness matrix: one voice exercises the entire
source -> rvc-convert.sh (Applio torch ref) -> ONNX worker -> log-mel path. So the
hosted fixture bundle ships ONE voice (`cool-jahns`); `confident-neal` is
deliberately excluded from the bundle.

To keep that drop HONEST (intentional + discoverable, never a silent skip / silent
pass), the narrowed set and the documented exclusion live here as constants, next
to a pure `check_parity_coverage` that trips if any voice the repo defines is
absent from BOTH the parity set and the documented-exclusion set.

Kept stdlib-only (no numpy / onnxruntime / soundfile / librosa) so the honesty
meta-assert + its negative test run under a stock `python3` with no venv, no
network, and no fixtures. parity_test.py and gen_targets.py both import from here,
so PARITY_VOICES has exactly one definition.
"""

from __future__ import annotations

# Authoritative roster of every RVC voice the repo defines. This is the
# INDEPENDENT coverage source for check_parity_coverage: it is a hand-maintained
# mirror of the voice-model dirs under assets/rvc-models/ (cool-jahns,
# confident-neal) and the #157 unified voice roster — NOT PARITY_VOICES and NOT
# gen_targets.VOICES — so the coverage guarantee below is not vacuously true.
#
# It cannot be derived from the filesystem at test time: `.gitignore` blanket-
# ignores `assets`, so a fresh clone has no assets/rvc-models/ dirs to scan. It is
# therefore pinned here as a committed constant. parity_test.py cross-checks it
# against its per-stage `VOICES` matrix (`set(VOICES) == set(ALL_RVC_VOICES)`), so
# a new voice cannot be added to the actually-tested per-stage gate without also
# appearing here — which then forces it into PARITY_VOICES or EXCLUDED_PARITY_VOICES.
ALL_RVC_VOICES = ("cool-jahns", "confident-neal")

# The narrowed full-pipeline hosted-fixture set: ONE voice exercises the whole path.
PARITY_VOICES = ("cool-jahns",)

# Voices deliberately dropped from the parity bundle, each mapped to a reason.
# The canonical "why" lives in tests/rvc_parity/fixtures/README.md; the reason
# string points there rather than restating the rationale in three places
# (single canonical rationale).
EXCLUDED_PARITY_VOICES = {
    "confident-neal": (
        "dropped from the parity bundle in #151 — the gate is a pipeline flow "
        "gate; one voice exercises the whole path. See fixtures/README.md."
    ),
}


class ParityCoverageError(AssertionError):
    """Raised when the parity/excluded partition does not honestly cover the roster."""


def check_parity_coverage(roster) -> None:
    """Assert PARITY_VOICES and EXCLUDED_PARITY_VOICES honestly partition `roster`.

    `roster` (the authoritative independent set of every voice the repo defines)
    is passed in by the caller — kept a parameter, not read from a module global,
    so the negative test can drive this pure function with a synthetic roster that
    adds an unclassified voice and prove it trips.

    Raises ParityCoverageError if:
      * a voice is in BOTH sets (not disjoint), or
      * a roster voice is in NEITHER set (silently dropped — honesty violation), or
      * a set names a voice the roster does not define (stale classification).
    """
    roster_set = set(roster)
    parity = set(PARITY_VOICES)
    excluded = set(EXCLUDED_PARITY_VOICES)

    overlap = parity & excluded
    if overlap:
        raise ParityCoverageError(
            f"voice(s) in BOTH PARITY_VOICES and EXCLUDED_PARITY_VOICES: "
            f"{sorted(overlap)} — a voice is either tested or documented-excluded, "
            f"never both."
        )

    covered = parity | excluded
    missing = roster_set - covered
    if missing:
        raise ParityCoverageError(
            f"voice(s) defined by the repo roster but absent from BOTH "
            f"PARITY_VOICES and EXCLUDED_PARITY_VOICES: {sorted(missing)} — "
            f"classify each as a parity voice or a documented exclusion "
            f"(honesty rule: no silently-dropped voice)."
        )

    stale = covered - roster_set
    if stale:
        raise ParityCoverageError(
            f"voice(s) in PARITY_VOICES/EXCLUDED_PARITY_VOICES not defined by the "
            f"repo roster {sorted(roster_set)}: {sorted(stale)} — remove the stale "
            f"entry or add the voice to the roster."
        )


def excluded_reason(voice: str) -> str:
    """Return the documented-exclusion reason for `voice` (KeyError if not excluded).

    The single canonical accessor for the exclusion rationale: callers read the
    reason through here rather than indexing EXCLUDED_PARITY_VOICES directly, so the
    "one canonical rationale" discipline has exactly one read path.
    """
    return EXCLUDED_PARITY_VOICES[voice]


def bundle_assets():
    """Hosted-bundle asset filenames, DERIVED from PARITY_VOICES (single source).

    The bundle is: the one fixed source clip plus, per parity voice, its
    torch-reference WAV and the pinned log-mel target. Deriving the list here — not
    hardcoding it in the Makefile — means a D0 pivot (swapping cool-jahns for a
    redistributable voice in PARITY_VOICES) updates the publish upload AND the
    unpinned-local fetch check in lockstep instead of letting them desync. Order is
    stable (source first, then each voice's ref + target) so callers can
    print/compare deterministically.
    """
    assets = ["source.wav"]
    for slug in PARITY_VOICES:
        assets.append(f"{slug}_ref.wav")
        assets.append(f"{slug}_logmel_target.npy")
    return tuple(assets)


if __name__ == "__main__":
    # `make rvc-fixtures-publish` derives RVC_BUNDLE_ASSETS from this line so the
    # published/pinned asset list cannot desync from PARITY_VOICES on a D0 pivot.
    print(" ".join(bundle_assets()))
