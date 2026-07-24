#!/usr/bin/env python3
"""Shared stdlib-hashlib checksum I/O for the RVC parity fixture bundle (#151).

ONE implementation, used by BOTH sides:
  * the publish flow (`make rvc-fixtures-publish`) GENERATES the pin, and
  * the fetch flow (`make rvc-fixtures-fetch` -> fetch_fixtures.py) VERIFIES every
    downloaded asset against it.

Platform-neutral: pure `hashlib`, run under `python3`. It does NOT shell out to
GNU `sha256sum` (absent on the macOS dev/verify platform) and has no CWD-relative
`-c` semantics. The committed pin file `tests/rvc_parity/fixtures.sha256` — NOT the
owner-mutable GitHub Release URL — is the trust root.

Pin format: one `<hex-sha256>␠␠<name>` line per asset (two spaces, the portable
`shasum -a 256` / `sha256sum` layout), plus optional `#`-comment and blank lines.
"""

from __future__ import annotations

import argparse
import hashlib
import os
import sys
from typing import Dict, Iterable, Tuple

_CHUNK = 1 << 20  # 1 MiB streaming reads — never load a whole asset into memory
_HEX = set("0123456789abcdef")


def sha256_file(path: str) -> str:
    """Return the lowercase hex SHA-256 of a file, read in bounded chunks."""
    h = hashlib.sha256()
    with open(path, "rb") as f:
        for chunk in iter(lambda: f.read(_CHUNK), b""):
            h.update(chunk)
    return h.hexdigest()


def format_pin_line(digest: str, name: str) -> str:
    """Render one portable `<hex>␠␠<name>` pin line."""
    return f"{digest}  {name}"


def parse_pins(text: str) -> Dict[str, str]:
    """Parse pin-file text into `{name: hex_digest}`.

    Blank lines and `#`-comment lines are ignored. A malformed non-comment line
    (wrong field count, or a token that is not a 64-char hex digest) raises
    ValueError — a corrupt pin file must fail loud, never be silently skipped.
    """
    pins: Dict[str, str] = {}
    for lineno, raw in enumerate(text.splitlines(), 1):
        line = raw.strip()
        if not line or line.startswith("#"):
            continue
        parts = line.split(None, 1)
        if len(parts) != 2:
            raise ValueError(
                f"fixtures.sha256 line {lineno}: malformed pin {raw!r} "
                f"(want '<sha256hex>  <name>')"
            )
        digest, name = parts[0].lower(), parts[1].strip()
        if len(digest) != 64 or any(c not in _HEX for c in digest):
            raise ValueError(
                f"fixtures.sha256 line {lineno}: not a 64-char sha256 hex digest: "
                f"{parts[0]!r}"
            )
        if not name:
            raise ValueError(f"fixtures.sha256 line {lineno}: empty asset name")
        pins[name] = digest
    return pins


def load_pins(path: str) -> Dict[str, str]:
    """Read + parse a pin file into `{name: hex_digest}`."""
    with open(path, "r", encoding="utf-8") as f:
        return parse_pins(f.read())


def write_pins(path: str, entries: Dict[str, str], header_lines: Iterable[str] = ()) -> None:
    """Write a portable pin file for `{name: hex_digest}`, sorted by name.

    `header_lines` are emitted as leading `#` comments (each rendered `# <line>`).
    """
    lines = []
    for hl in header_lines:
        lines.append(f"# {hl}" if hl.strip() else "#")
    for name, digest in sorted(entries.items()):
        lines.append(format_pin_line(digest, name))
    with open(path, "w", encoding="utf-8") as f:
        f.write("\n".join(lines) + "\n")


def verify_file(path: str, expected_hex: str) -> Tuple[bool, str]:
    """Return `(matches, actual_hex)` comparing a file's SHA-256 to `expected_hex`."""
    actual = sha256_file(path)
    return actual.lower() == expected_hex.lower(), actual


# ── GENERATE CLI (publish flow) ────────────────────────────────────────────────
# `make rvc-fixtures-publish` calls this to pin the freshly-regenerated bundle. The
# same functions above verify it on fetch — one implementation, both directions.

_PIN_HEADER = (
    "RVC parity fixture bundle — integrity pins (issue #151). TRUST ROOT.",
    "",
    "Generated + verified by the ONE shared hashlib impl in "
    "tests/rvc_parity/fixtures_io.py (never GNU sha256sum). These committed hashes,",
    "NOT the owner-mutable release URL, are the integrity guarantee. Pin the release",
    "tag/base URL (Makefile) + this file in the SAME commit, and do not merge that",
    "commit until the release is live + fetchable (plan Step 6 + Step 8 merge gate).",
)


def _generate_cli(argv=None) -> int:
    here = os.path.dirname(os.path.abspath(__file__))
    ap = argparse.ArgumentParser(
        description="Generate tests/rvc_parity/fixtures.sha256 over the hosted bundle.")
    ap.add_argument("--fixtures-dir", default=os.path.join(here, "fixtures"))
    ap.add_argument("--pin-file", default=os.path.join(here, "fixtures.sha256"))
    ap.add_argument("names", nargs="+",
                    help="asset filenames (relative to --fixtures-dir) to pin")
    args = ap.parse_args(argv)

    entries: Dict[str, str] = {}
    for name in args.names:
        path = os.path.join(args.fixtures_dir, name)
        if not os.path.isfile(path):
            print(f"fixtures_io: ERROR: no such asset to pin: {path}", file=sys.stderr)
            return 1
        entries[name] = sha256_file(path)
    write_pins(args.pin_file, entries, header_lines=_PIN_HEADER)
    print(f"fixtures_io: wrote {args.pin_file} pinning {sorted(entries)}", file=sys.stderr)
    return 0


if __name__ == "__main__":
    sys.exit(_generate_cli())
