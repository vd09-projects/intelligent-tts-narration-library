#!/usr/bin/env python3
"""Fetch + verify the hosted RVC parity fixture bundle (issue #151). Stdlib-only.

Run under a stock `python3` (no venv, no torch, no numpy) — it is a prerequisite
of `make rvc-parity`, so a fresh clone can obtain the fixtures before the gate.

Behaviour (plan Decisions D3/D4):
  * The committed pin file `tests/rvc_parity/fixtures.sha256` is the source of truth
    for WHICH assets to fetch and their SHA-256s (NOT the owner-mutable URL).
  * Idempotent: an asset already present AND checksum-valid is skipped — no network.
  * A present-but-DIVERGENT asset (bytes != pin) is a HARD FAIL (never silently
    overwritten, never silently accepted) telling the dev to rm + re-fetch.
  * Otherwise the asset is downloaded with a bounded `curl -fSL --connect-timeout
    --max-time --retry` into a `mkstemp` `.part` staged INSIDE fixtures/ (same
    filesystem), hashlib-verified, then `os.replace`d atomically into place.
  * ANY failure (empty pin file, 404/unreachable, timeout/hang, checksum mismatch)
    exits NON-ZERO with a named reason and removes the partial. Never a silent green.
  * UNPINNED tag/base URL (the D0-not-cleared default — no release exists yet): fall
    back to LOCAL fixtures. If every required bundle asset is present, run the gate
    against them but print a LOUD "unverified — not checked against a published
    checksum" notice (never a silent green); if any is missing, fail loud and point
    at `make rvc-parity-gen` / the D0 pivot. This keeps the interim gate runnable for
    a dev with locally-regenerated fixtures without weakening fail-loud honesty — the
    parity assertions themselves still gate correctness.

`.npy` assets are treated as opaque bytes here (only hashed, never unpickled). The
consumer (parity_test.py) loads them with `numpy.load(..., allow_pickle=False)`.
"""

from __future__ import annotations

import argparse
import os
import subprocess
import sys
import tempfile

_HERE = os.path.dirname(os.path.abspath(__file__))
sys.path.insert(0, _HERE)
import fixtures_io  # noqa: E402  (stdlib-only sibling)
import parity_voices  # noqa: E402  (stdlib-only sibling — single source of the bundle set)

FIXTURES_DIR = os.path.join(_HERE, "fixtures")
PIN_FILE = os.path.join(_HERE, "fixtures.sha256")

# Belt-and-suspenders slack (seconds) added to the Python-side wall ceiling on top of
# curl's own per-attempt bounds — covers curl's inter-retry backoff so a genuine
# slow-but-progressing fetch is not killed while a truly stuck curl still dies.
_WALL_SLACK_S = 60

# Placeholder sentinel documented in the Makefile + fixtures/README.md: until the
# release is published (blocked by the D0 voice-model license gate), the tag/base
# URL are unset and fetching fails loud rather than 404-ing confusingly.
_UNSET = ""


class FetchError(Exception):
    """A named, fail-loud fetch failure. Message is printed; exit code is non-zero."""


def _fail(msg: str) -> "FetchError":
    return FetchError(msg)


def _asset_url(base_url: str, name: str) -> str:
    return base_url.rstrip("/") + "/" + name


def _curl(url: str, dest: str, connect_timeout: int, max_time: int, retry: int) -> None:
    """Bounded download. Raises FetchError (named) on any curl failure."""
    cmd = [
        "curl", "-fSL",
        "--connect-timeout", str(connect_timeout),
        "--max-time", str(max_time),
        "--retry", str(retry),
        "-o", dest,
        url,
    ]
    # Python-side wall ceiling (belt-and-suspenders beyond curl's own
    # --connect-timeout/--max-time): if a curl build ignores its own bounds and
    # hangs, subprocess.run's timeout still kills it so the gate can never block
    # forever. Sized generously (per-attempt ceiling x every retry + backoff slack)
    # so it only ever trips on a truly stuck curl, never a slow-but-valid download.
    wall_ceiling = (connect_timeout + max_time) * (retry + 1) + _WALL_SLACK_S
    try:
        proc = subprocess.run(cmd, capture_output=True, text=True, timeout=wall_ceiling)
    except FileNotFoundError as exc:  # curl itself missing
        raise _fail(f"curl not found on PATH — cannot fetch {url}") from exc
    except subprocess.TimeoutExpired as exc:
        raise _fail(
            f"fetch of {url} FAILED: exceeded the {wall_ceiling}s Python-side wall "
            f"ceiling — curl ignored its own --max-time/--connect-timeout and hung."
        ) from exc
    if proc.returncode != 0:
        # curl exit 22 = HTTP >= 400 (e.g. 404); 28 = timeout/hang tripped the
        # --max-time/--connect-timeout bound; 6/7 = DNS/connection failure.
        reason = {
            22: "HTTP error (asset unreachable / 404 — release not live at this tag?)",
            28: "timed out (bounded --max-time/--connect-timeout tripped — network hang)",
            6: "could not resolve host",
            7: "could not connect to host",
        }.get(proc.returncode, "download failed")
        raise _fail(
            f"fetch of {url} FAILED: {reason} (curl exit {proc.returncode}). "
            f"{proc.stderr.strip()}"
        )


def _fetch_one(name: str, expected_hex: str, base_url: str,
               connect_timeout: int, max_time: int, retry: int) -> str:
    """Fetch+verify a single asset. Returns a short status word for logging."""
    dest = os.path.join(FIXTURES_DIR, name)

    if os.path.isfile(dest):
        ok, actual = fixtures_io.verify_file(dest, expected_hex)
        if ok:
            return "skip (present + valid)"
        raise _fail(
            f"present-but-divergent fixture {name}: on-disk sha256 {actual} != "
            f"pinned {expected_hex}. Refusing to overwrite. `rm "
            f"{os.path.join('tests/rvc_parity/fixtures', name)}` and re-run "
            f"`make rvc-fixtures-fetch`."
        )

    os.makedirs(FIXTURES_DIR, exist_ok=True)
    fd, part = tempfile.mkstemp(dir=FIXTURES_DIR, prefix=name + ".", suffix=".part")
    os.close(fd)  # curl opens/writes the path itself; we only needed a unique name
    try:
        _curl(_asset_url(base_url, name), part, connect_timeout, max_time, retry)
        ok, actual = fixtures_io.verify_file(part, expected_hex)
        if not ok:
            raise _fail(
                f"checksum mismatch for downloaded {name}: got {actual}, "
                f"pinned {expected_hex}. Corrupt or wrong asset — not installed."
            )
        os.replace(part, dest)  # atomic on the same filesystem
        return "fetched + verified"
    finally:
        if os.path.exists(part):
            os.unlink(part)  # never leave a .part behind on failure


def _local_fallback() -> int:
    """Unpinned release tag (the D0-not-cleared default): there is no published
    release to download from and no trust-root checksum to verify against.

    Rather than hard-blocking the interim workflow, run the gate against LOCAL
    fixtures IF all are present (a dev who ran `make rvc-parity-gen` has valid ones)
    — LOUDLY flagged as UNVERIFIED so a green gate is never mistaken for
    reproducible-from-a-published-bundle. If any required fixture is missing, fail
    loud (never a silent green) and point at the local-regen / D0-pivot paths.

    Honesty is preserved two ways: (1) a MISSING fixture still fails loud here; and
    (2) this only skips the download+checksum step (there is nothing to download) —
    the parity assertions in parity_test.py still run against these fixtures, so a
    corrupt/stale local fixture fails the gate on its own merits.
    """
    required = parity_voices.bundle_assets()  # derived from PARITY_VOICES, single source
    missing = [n for n in required
               if not os.path.isfile(os.path.join(FIXTURES_DIR, n))]
    if missing:
        raise _fail(
            "RVC parity fixtures are NOT published yet — the release tag / asset "
            "base URL are unpinned (blocked on the #151 D0 voice-model license "
            "gate, see tests/rvc_parity/fixtures/README.md) AND these required "
            f"LOCAL fixtures are missing: {missing}. Regenerate them locally with "
            "`make rvc-parity-gen SOURCE=<clip.wav>` (ungated, gitignored), OR "
            "complete the D0 pivot to a redistributable voice and publish "
            "(`make rvc-fixtures-publish`). This is a fail-loud placeholder, not a "
            "bug — the gate will NOT run without its fixtures."
        )
    print(
        "[fetch] WARNING: unpinned release tag (D0 not cleared) — running against "
        "LOCAL fixtures, NOT verified against a published checksum. No download or "
        "trust-root integrity check happened; these are whatever "
        "`make rvc-parity-gen` produced on this machine. Do NOT treat a green gate "
        "as reproducible from a published bundle.",
        file=sys.stderr,
    )
    for name in required:
        print(f"[fetch] {name}: present (LOCAL, unverified)", file=sys.stderr)
    print(f"[fetch] all {len(required)} LOCAL fixture(s) present (unverified) — "
          "running the gate against them.", file=sys.stderr)
    return 0


def fetch_all(tag: str, base_url: str, connect_timeout: int, max_time: int,
              retry: int) -> int:
    if not tag or tag == _UNSET or not base_url or base_url == _UNSET:
        # Unpinned placeholder (D0 not cleared): fall back to LOCAL fixtures if all
        # are present (loud + unverified), else fail loud. See _local_fallback.
        return _local_fallback()

    # Pinned (post-pivot): strict download + checksum-verify + atomic move; fail
    # loud on 404 / timeout / mismatch / present-but-divergent. Unchanged behavior.
    if not os.path.isfile(PIN_FILE):
        raise _fail(f"missing pin file {PIN_FILE} — cannot verify any fetched asset.")
    pins = fixtures_io.load_pins(PIN_FILE)
    if not pins:
        raise _fail(
            f"{PIN_FILE} pins no assets yet — run `make rvc-fixtures-publish` to "
            f"regenerate the bundle + pin it (blocked on the #151 D0 license gate). "
            f"Fetch cannot proceed against an empty pin."
        )

    print(f"[fetch] tag={tag} base={base_url} assets={sorted(pins)}", file=sys.stderr)
    for name, expected_hex in sorted(pins.items()):
        status = _fetch_one(name, expected_hex, base_url, connect_timeout, max_time, retry)
        print(f"[fetch] {name}: {status}", file=sys.stderr)
    print(f"[fetch] all {len(pins)} fixture(s) present + verified.", file=sys.stderr)
    return 0


def main(argv=None) -> int:
    ap = argparse.ArgumentParser(description="Fetch + verify hosted RVC parity fixtures.")
    ap.add_argument("--tag", default=os.environ.get("RVC_FIXTURES_TAG", _UNSET),
                    help="pinned GitHub Release tag (fail-loud if unset)")
    ap.add_argument("--base-url", default=os.environ.get("RVC_FIXTURES_BASEURL", _UNSET),
                    help="release asset base URL (fail-loud if unset)")
    ap.add_argument("--connect-timeout", type=int,
                    default=int(os.environ.get("RVC_FIXTURES_CONNECT_TIMEOUT", "10")))
    ap.add_argument("--max-time", type=int,
                    default=int(os.environ.get("RVC_FIXTURES_MAX_TIME", "120")))
    ap.add_argument("--retry", type=int,
                    default=int(os.environ.get("RVC_FIXTURES_RETRY", "3")))
    args = ap.parse_args(argv)

    try:
        return fetch_all(args.tag, args.base_url, args.connect_timeout,
                         args.max_time, args.retry)
    except FetchError as exc:
        print(f"rvc-fixtures-fetch: ERROR: {exc}", file=sys.stderr)
        return 1
    except ValueError as exc:  # malformed pin file
        print(f"rvc-fixtures-fetch: ERROR: {exc}", file=sys.stderr)
        return 1


if __name__ == "__main__":
    sys.exit(main())
