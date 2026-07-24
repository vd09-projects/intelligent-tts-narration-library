#!/usr/bin/env python3
"""Stdlib-only smokes for the #151 fixture fetch/verify + single-voice honesty flow.

These run under a STOCK `python3` — no `.venv-rvc`, no torch/numpy/onnxruntime, no
network, no real fixtures — because everything they exercise (parity_voices,
fixtures_io, fetch_fixtures) is stdlib-only. The heavier full-pipeline gate lives in
parity_test.py (needs the venv). Plain-python asserts + a main() runner, matching
the repo's parity_test.py style (pytest is not assumed present).

    python3 tests/rvc_parity/fixtures_flow_test.py

Covers:
  * single-voice honesty     — PARITY_VOICES + EXCLUDED_PARITY_VOICES partition the
                               repo roster (positive) AND an unclassified voice trips
                               the meta-assert (negative), plus a stale-classification
                               case
  * portable verify          — hashlib pin round-trip + verify with an EMPTIED PATH
                               (proves no GNU sha256sum / coreutils dependency)
  * unpinned local fallback  — unpinned tag + MISSING local fixtures fails loud;
                               unpinned tag + PRESENT local fixtures proceeds with a
                               LOUD "unverified" notice (never a silent green)
  * fail-loud fetch          — empty pin file, present-but-divergent, corrupted
                               download, and 404/unreachable all exit non-zero and
                               leave no .part
  * happy download           — a correct download verifies + atomically installs
  * npy unpickle-safety       — parity_test.py loads the .npy target with allow_pickle=False
"""

from __future__ import annotations

import contextlib
import io
import os
import sys
import tempfile

_HERE = os.path.dirname(os.path.abspath(__file__))
sys.path.insert(0, _HERE)

import fetch_fixtures  # noqa: E402
import fixtures_io  # noqa: E402
from parity_voices import (  # noqa: E402
    ALL_RVC_VOICES,
    EXCLUDED_PARITY_VOICES,
    PARITY_VOICES,
    ParityCoverageError,
    bundle_assets,
    check_parity_coverage,
    excluded_reason,
)


# ── helpers ─────────────────────────────────────────────────────────────────────

@contextlib.contextmanager
def _patched_fixtures(assets: dict, pins: dict):
    """Temp fixtures dir + pin file, with fetch_fixtures globals patched to them.

    assets: {name: bytes} pre-placed in the dir. pins: {name: hex} written to the pin
    file. Restores the globals afterwards.
    """
    saved_dir, saved_pin = fetch_fixtures.FIXTURES_DIR, fetch_fixtures.PIN_FILE
    with tempfile.TemporaryDirectory() as d:
        fixtures_dir = os.path.join(d, "fixtures")
        os.makedirs(fixtures_dir)
        for name, data in assets.items():
            with open(os.path.join(fixtures_dir, name), "wb") as f:
                f.write(data)
        pin_path = os.path.join(d, "fixtures.sha256")
        fixtures_io.write_pins(pin_path, pins) if pins else _write_empty_pin(pin_path)
        fetch_fixtures.FIXTURES_DIR = fixtures_dir
        fetch_fixtures.PIN_FILE = pin_path
        try:
            yield fixtures_dir, pin_path
        finally:
            fetch_fixtures.FIXTURES_DIR, fetch_fixtures.PIN_FILE = saved_dir, saved_pin


def _write_empty_pin(path):
    with open(path, "w", encoding="utf-8") as f:
        f.write("# no assets pinned yet (template)\n")


def _parts(fixtures_dir):
    return [f for f in os.listdir(fixtures_dir) if f.endswith(".part")]


# ── single-voice honesty ─────────────────────────────────────────────────────────

def test_parity_coverage_positive():
    """The real partition is honest: disjoint + covers the roster; drop documented."""
    check_parity_coverage(ALL_RVC_VOICES)  # must not raise
    assert set(PARITY_VOICES).isdisjoint(EXCLUDED_PARITY_VOICES)
    assert "confident-neal" in EXCLUDED_PARITY_VOICES
    # read the reason through the canonical accessor, not by indexing the dict.
    reason = excluded_reason("confident-neal")
    assert reason.strip(), "exclusion reason is empty"
    # single canonical rationale: the reason points at the README, not a restatement.
    assert "README" in reason, "exclusion reason must point at fixtures/README.md"
    print("  [honesty+] PARITY+EXCLUDED honestly partition the roster  OK")


def test_parity_coverage_negative_unclassified():
    """A voice in NEITHER set MUST trip the meta-assert (the core honesty guarantee)."""
    roster = list(ALL_RVC_VOICES) + ["ghost-voice"]
    tripped = False
    try:
        check_parity_coverage(roster)
    except ParityCoverageError as e:
        tripped = True
        assert "ghost-voice" in str(e)
    assert tripped, "an unclassified voice did NOT trip the coverage meta-assert"
    print("  [honesty-] unclassified voice trips the meta-assert  OK")


def test_parity_coverage_negative_stale():
    """A classified voice absent from the roster MUST trip (stale classification)."""
    tripped = False
    try:
        check_parity_coverage(("cool-jahns",))  # confident-neal excluded but not in roster
    except ParityCoverageError:
        tripped = True
    assert tripped, "a stale (roster-absent) classified voice did NOT trip"
    print("  [honesty-] stale classification trips the meta-assert  OK")


# ── portable verify (no GNU coreutils) ────────────────────────────────────────────

def test_portable_verify_no_coreutils():
    """Pin round-trip + verify runs on pure hashlib with an EMPTIED PATH — proving no
    GNU sha256sum / external coreutils dependency (BLOCKING-2)."""
    payload = b"portable-verify-smoke\n" * 100
    with _patched_fixtures({"source.wav": payload},
                           {"source.wav": _sha(payload)}):
        saved_path = os.environ.get("PATH", "")
        os.environ["PATH"] = ""  # no sha256sum, no curl reachable
        try:
            # present + valid -> idempotent skip, pure hashlib, no subprocess at all.
            rc = fetch_fixtures.main(["--tag", "t", "--base-url", "http://example/x"])
        finally:
            os.environ["PATH"] = saved_path
    assert rc == 0, "present+valid verify failed under an emptied PATH"
    print("  [portable-verify] hashlib verify works with PATH='' (no coreutils)  OK")


# ── fail-loud fetch ───────────────────────────────────────────────────────────────

def test_fetch_unpinned_missing_local_fails_loud():
    """Unpinned tag AND missing local fixtures -> non-zero, pointing at regen/pivot.

    The escape hatch must NOT paper over absent fixtures: with no published release
    and no local bundle on disk, there is nothing to run the gate against, so this
    still fails loud (never a silent green)."""
    with _patched_fixtures({}, {}):  # empty fixtures dir — no bundle assets present
        err = io.StringIO()
        with contextlib.redirect_stderr(err):
            rc = fetch_fixtures.main(["--tag", "", "--base-url", ""])
    assert rc != 0, "unpinned tag + missing local fixtures did not fail loud"
    assert "missing" in err.getvalue().lower(), "no missing-fixtures message"
    print("  [fail-loud] unpinned tag + missing local fixtures exits non-zero  OK")


def test_fetch_unpinned_present_local_proceeds_loud():
    """Unpinned tag but ALL local fixtures present -> proceed (rc 0) with a LOUD
    unverified notice, so the interim local-fixtures gate is runnable without a
    silent green. The notice makes clear these are not checksum-verified."""
    assets = {name: b"local-fixture-bytes\n" for name in bundle_assets()}
    with _patched_fixtures(assets, {}):  # unpinned; pins irrelevant on this path
        err = io.StringIO()
        with contextlib.redirect_stderr(err):
            rc = fetch_fixtures.main(["--tag", "", "--base-url", ""])
    msg = err.getvalue()
    assert rc == 0, f"unpinned tag + present local fixtures did not proceed\n{msg}"
    assert "NOT verified against a published checksum" in msg, \
        "loud unverified notice missing — a silent green would be dishonest"
    print("  [escape-hatch] unpinned + present local fixtures proceeds, loudly  OK")


def test_fetch_empty_pinfile_fails_loud():
    """A pinned tag but an empty pin file -> non-zero (nothing to fetch, no silent green)."""
    with _patched_fixtures({}, {}):  # empty pin file
        rc = fetch_fixtures.main(["--tag", "v1", "--base-url", "http://example/x"])
    assert rc != 0, "empty pin file did not fail loud"
    print("  [fail-loud] empty pin file exits non-zero  OK")


def test_fetch_present_but_divergent_hard_fail():
    """A present file whose bytes != pin -> hard fail, never silently overwritten."""
    good, bad = b"real-bytes\n", b"tampered-bytes\n"
    with _patched_fixtures({"source.wav": bad}, {"source.wav": _sha(good)}) as (fx, _):
        rc = fetch_fixtures.main(["--tag", "v1", "--base-url", "http://example/x"])
        # the divergent file is left in place (not overwritten); dev is told to rm it.
        with open(os.path.join(fx, "source.wav"), "rb") as f:
            assert f.read() == bad, "divergent file was silently overwritten"
        assert not _parts(fx), "left a .part behind"
    assert rc != 0, "present-but-divergent did not fail loud"
    print("  [fail-loud] present-but-divergent hard-fails, no overwrite  OK")


def test_fetch_corruption_mismatch():
    """A download whose bytes don't match the pin -> hard fail + no .part left."""
    good, corrupt = b"the-real-asset\n", b"corrupted-download\n"
    saved_curl = fetch_fixtures._curl

    def fake_curl(url, dest, *a, **k):
        with open(dest, "wb") as f:
            f.write(corrupt)  # simulate a corrupted/truncated download

    with _patched_fixtures({}, {"source.wav": _sha(good)}) as (fx, _):
        fetch_fixtures._curl = fake_curl
        try:
            rc = fetch_fixtures.main(["--tag", "v1", "--base-url", "http://example/x"])
        finally:
            fetch_fixtures._curl = saved_curl
        assert not os.path.exists(os.path.join(fx, "source.wav")), "installed corrupt asset"
        assert not _parts(fx), "left a .part behind on checksum mismatch"
    assert rc != 0, "checksum mismatch did not fail loud"
    print("  [fail-loud] corrupt download hard-fails, no partial installed  OK")


def test_fetch_happy_download_installs():
    """A correct download verifies + atomically installs; no .part remains."""
    payload = b"a-correct-asset-payload\n" * 10
    saved_curl = fetch_fixtures._curl

    def fake_curl(url, dest, *a, **k):
        with open(dest, "wb") as f:
            f.write(payload)

    with _patched_fixtures({}, {"cool-jahns_ref.wav": _sha(payload)}) as (fx, _):
        fetch_fixtures._curl = fake_curl
        try:
            rc = fetch_fixtures.main(["--tag", "v1", "--base-url", "http://example/x"])
        finally:
            fetch_fixtures._curl = saved_curl
        dest = os.path.join(fx, "cool-jahns_ref.wav")
        assert os.path.isfile(dest), "correct download not installed"
        with open(dest, "rb") as f:
            assert f.read() == payload
        assert not _parts(fx), "left a .part behind after success"
    assert rc == 0, "happy-path download did not succeed"
    print("  [happy] correct download verifies + atomically installs  OK")


def test_fetch_unreachable_fails_loud():
    """A real (bounded) curl against an unroutable host -> non-zero + no .part.

    Uses localhost:1 (connection refused, fast, offline-deterministic on macOS). If
    curl is absent the helper still fails loud ('curl not found')."""
    with _patched_fixtures({}, {"source.wav": _sha(b"whatever")}) as (fx, _):
        rc = fetch_fixtures.main([
            "--tag", "v1", "--base-url", "http://127.0.0.1:1",
            "--connect-timeout", "1", "--max-time", "2", "--retry", "0",
        ])
        assert not _parts(fx), "left a .part behind on unreachable host"
    assert rc != 0, "unreachable host did not fail loud"
    print("  [fail-loud] unreachable host exits non-zero, no partial  OK")


# ── npy unpickle-safety (source guard, stock-python3 runnable) ────────────────────

def test_npy_allow_pickle_source_guard():
    """parity_test.py must load the fetched .npy target with allow_pickle=False (the
    runtime proof lives in parity_test.test_npy_allow_pickle_guard, which needs numpy;
    this cheap source guard runs under stock python3)."""
    with open(os.path.join(_HERE, "parity_test.py"), encoding="utf-8") as f:
        src = f.read()
    assert "np.load(target_path, allow_pickle=False)" in src, \
        "parity_test.py no longer loads the .npy target with allow_pickle=False"
    print("  [npy-safety] target load pins allow_pickle=False  OK")


def _sha(data: bytes) -> str:
    import hashlib
    return hashlib.sha256(data).hexdigest()


TESTS = [
    test_parity_coverage_positive,
    test_parity_coverage_negative_unclassified,
    test_parity_coverage_negative_stale,
    test_portable_verify_no_coreutils,
    test_fetch_unpinned_missing_local_fails_loud,
    test_fetch_unpinned_present_local_proceeds_loud,
    test_fetch_empty_pinfile_fails_loud,
    test_fetch_present_but_divergent_hard_fail,
    test_fetch_corruption_mismatch,
    test_fetch_happy_download_installs,
    test_fetch_unreachable_fails_loud,
    test_npy_allow_pickle_source_guard,
]


def main() -> int:
    print("== #151 fixture fetch/verify + single-voice honesty smokes (stdlib-only) ==")
    failures = []
    for t in TESTS:
        try:
            t()
        except Exception as e:  # noqa: BLE001 - collect + report, fail at the end
            failures.append((t.__name__, e))
            print(f"  FAIL {t.__name__}: {e}")
    print()
    if failures:
        print(f"FAILED {len(failures)}/{len(TESTS)}:")
        for name, e in failures:
            print(f"  - {name}: {e}")
        return 1
    print(f"PASSED {len(TESTS)}/{len(TESTS)} — fetch/verify + honesty smokes green.")
    return 0


if __name__ == "__main__":
    sys.exit(main())
