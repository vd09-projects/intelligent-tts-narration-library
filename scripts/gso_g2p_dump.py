#!/usr/bin/env python3
"""G2P coverage dump for the GSO voice (issue #164, AC3 — MACHINE halves only).

Reads a plan.json (produced by `go run ./cmd/plandump`, the pure-planner dump — no
renderer, no audio, no GSO worker) and, per block, surfaces the two MACHINE-checkable
halves of AC3:

  half A — Segment.Text correctness (NO worker, NO audio, NO listening):
     the block's spoken words, derived EXACTLY as render/gptsovits derives them
     (spokenTextFor: a refused block speaks its Refusal.Message; otherwise the
     concatenation of its `speech` segments' Text). This is engine-neutral — the same
     words Kokoro / RVC / GSO would each receive — so half A needs no `.venv-gso`.

  half B — g2p_en phoneme STRING (textual, NO listening):
     the ARPAbet phoneme string GPT-SoVITS's `g2p_en` produces for that exact
     Segment.Text. Half B needs `.venv-gso` (that is where g2p_en lives). It is a
     TEXTUAL inspection aid — we read the phoneme string and judge it against the
     expected ARPAbet by eye. It is NEVER a golden/fixture assertion (a #164 non-goal),
     and it NEVER plays or judges audio.

Whether the model actually REALISES those phonemes as the intended sound — prosody,
character likeness — is acoustic and is staged for the human by-ear session (AC3-ear),
never recorded here as a machine verdict (the B1 machine-vs-human boundary).

The block-text derivation is a faithful port of render/gptsovits/gptsovits.go's
`spokenTextFor` so the phoneme string is taken over the SAME words the worker would be
handed at render time.

Usage:
    go run ./cmd/plandump --file docs/samples/gso-g2p-coverage.md --level 3 > plan.json
    .venv-gso/bin/python scripts/gso_g2p_dump.py --plan plan.json          # half A + half B
    python3 scripts/gso_g2p_dump.py --plan plan.json --no-phonemes          # half A only (stock python)
"""

from __future__ import annotations

import argparse
import json
import sys

# render/gptsovits/gptsovits.go: SegmentKindSpeech + StatusRefused. Kept as literals
# (the plan schema is engine-neutral JSON; these strings are the wire values).
SEGMENT_KIND_SPEECH = "speech"
STATUS_REFUSED = "refused"


def spoken_text_for(block: dict) -> str:
    """Faithful port of render/gptsovits/gptsovits.go `spokenTextFor`.

    A refused block speaks its Refusal.Message; otherwise the block speaks the
    space-joined Text of its `speech` segments (non-empty only). An empty result
    means an all-pause / empty block the renderer would skip (no worker exchange).
    """
    if block.get("status") == STATUS_REFUSED:
        refusal = block.get("refusal") or {}
        return (refusal.get("message") or "").strip()
    parts = [
        seg.get("text", "")
        for seg in block.get("segments", [])
        if seg.get("kind") == SEGMENT_KIND_SPEECH and seg.get("text")
    ]
    return " ".join(parts)


def _load_g2p():
    """Import g2p_en lazily (it lives in .venv-gso). Returns a callable text->phones,
    or raises a clear error the caller turns into a graceful half-A-only fallback."""
    from g2p_en import G2p  # noqa: PLC0415 — deliberately lazy (only .venv-gso has it)

    g2p = G2p()

    def to_arpabet(text: str) -> str:
        # g2p_en returns a list of ARPAbet phones with ' ' entries as word breaks and
        # punctuation kept verbatim. Join to a single inspectable string.
        return " ".join(g2p(text))

    return to_arpabet


def main(argv: list[str] | None = None) -> int:
    ap = argparse.ArgumentParser(
        prog="gso_g2p_dump",
        description="Surface Segment.Text (half A) + g2p_en phoneme strings (half B) per plan block.",
    )
    ap.add_argument("--plan", required=True, help="path to a plan.json from `go run ./cmd/plandump`")
    ap.add_argument("--no-phonemes", action="store_true",
                    help="half A only — skip g2p_en (for stock python3 without .venv-gso)")
    ap.add_argument("--max-chars", type=int, default=0,
                    help="truncate printed Segment.Text/phonemes to N chars (0 = full)")
    args = ap.parse_args(sys.argv[1:] if argv is None else argv)

    try:
        with open(args.plan, encoding="utf-8") as fh:
            plan = json.load(fh)
    except (OSError, json.JSONDecodeError) as e:
        print(f"gso-g2p-check: cannot read plan.json at {args.plan!r}: {e}", file=sys.stderr)
        return 2

    to_arpabet = None
    if not args.no_phonemes:
        try:
            to_arpabet = _load_g2p()
        except Exception as e:  # noqa: BLE001 — any import failure = no .venv-gso
            print(f"gso-g2p-check: half B (g2p_en phoneme strings) UNAVAILABLE — {type(e).__name__}: {e}",
                  file=sys.stderr)
            print("gso-g2p-check: run under .venv-gso (make gso-worker-venv) for half B; "
                  "printing half A (Segment.Text) only.", file=sys.stderr)
            args.no_phonemes = True

    def clip(s: str) -> str:
        return s if args.max_chars <= 0 or len(s) <= args.max_chars else s[:args.max_chars] + "…"

    blocks = plan.get("blocks", [])
    print(f"# gso-g2p-check — {len(blocks)} blocks from {args.plan}")
    print(f"# half A = Segment.Text (planner, engine-neutral); "
          f"half B = g2p_en ARPAbet string {'(SKIPPED)' if args.no_phonemes else '(.venv-gso)'}")
    print("# machine-checkable only; acoustic realisation is AC3-ear (staged for human ears).")
    for b in blocks:
        text = spoken_text_for(b)
        header = f"[block {b.get('order')}] class={b.get('class')} level=L{b.get('level')} status={b.get('status')}"
        print("\n" + header)
        if not text:
            print("  Segment.Text : (empty — all-pause / skipped block, no worker exchange)")
            continue
        print(f"  Segment.Text : {clip(text)}")
        if not args.no_phonemes and to_arpabet is not None:
            phones = to_arpabet(text)
            print(f"  g2p_en ARPA  : {clip(phones)}")

    return 0


if __name__ == "__main__":
    sys.exit(main())
