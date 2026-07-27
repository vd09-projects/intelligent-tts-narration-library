#!/usr/bin/env python3
"""Torch-free wire-contract + ERR-taxonomy test for the GSO worker (issue #161, AC6).

Runs under a STOCK `python3` — NO torch, NO numpy, NO .venv-gso, NO models — because
everything it exercises (shlex parse, token/framing validation, the CLOSED ERR
taxonomy, the wire-vs-packaged drift rule, content-addressed path minting, the atomic
WAV write, and the OK-only-after-write happens-before invariant) is stdlib-only. The
torch-touching inference is the single `_Worker._synthesize` seam, which this test
monkeypatches. This is the in-session proof of AC2/AC6; the warm-load CORRECTNESS
proof (AC5, real artifacts) is machine-gated.

Plain-python asserts + a main() runner (pytest is not assumed present), matching the
repo's tests/rvc_parity/*_test.py style.

    python3 scripts/gso_worker_contract_test.py

Covers:
  * torch-free import        — importing the worker pulls in NO torch/numpy
  * wire framing             — exactly 1 response line per request; blank line -> ERR
  * ERR taxonomy             — every category round-trips as a single typed line
  * gotchas 5/6/7            — empty ref -> bad-args, empty prompt -> bad-args,
                               non-cut4 -> bad-args, missing ref file -> read-failed
  * drift rule               — wrong-voice-dir -> bad-voice; out-of-dir + transcript
                               mismatch -> bad-args; wire value stays authoritative
  * minted path              — content-addressed shape, deterministic, single-line
  * OK-after-write invariant — a successful OK implies the file exists; a write
                               failure yields ERR write-failed and NO OK
  * OK reply is literal      — the `OK <out>` payload is the LITERAL path (line[3:]),
                               NOT shlex-quoted (proved with an out_base containing a
                               space so literal vs quoted are distinguishable)
  * honesty boundary         — a MemoryError in the per-request handler PROPAGATES
                               as a FATAL runtime exit (70), never a per-block ERR;
                               and a missing-torch/artifacts startup is a clean
                               FATAL exit (78), never an OK/ERR line
  * no raw traceback         — every ERR case is one physical line, no 'Traceback'
  * shlex golden fixture     — the cross-language round-trip contract for #162
"""

from __future__ import annotations

import json
import os
import re
import shlex
import struct
import subprocess
import sys
import tempfile
import wave

_HERE = os.path.dirname(os.path.abspath(__file__))
sys.path.insert(0, _HERE)

import gptsovits_worker as gw  # noqa: E402

VOICE = "cool-jahns-gso"
PACKAGED_PROMPT = "This is the reference clip transcript for the packaged voice."
_STUB_PCM = struct.pack("<8h", 1000, -1000, 2000, -2000, 1500, -1500, 500, -500)

_failures: list[str] = []


def check(cond: bool, msg: str) -> None:
    if not cond:
        _failures.append(msg)
        print(f"  FAIL: {msg}")


def _err_cat(resp: str) -> str | None:
    parts = resp.split(" ", 2)
    if len(parts) >= 2 and parts[0] == "ERR":
        return parts[1]
    return None


def _assert_single_typed_err(resp: str, expected_cat: str, label: str) -> None:
    check("\n" not in resp and "\r" not in resp, f"{label}: response not single physical line: {resp!r}")
    check(resp.startswith("ERR "), f"{label}: expected ERR line, got {resp!r}")
    cat = _err_cat(resp)
    check(cat in gw.ERR_CATEGORIES, f"{label}: category {cat!r} not in closed taxonomy")
    check(cat == expected_cat, f"{label}: expected category '{expected_cat}', got '{cat}' ({resp!r})")
    check("Traceback" not in resp, f"{label}: raw traceback leaked into response: {resp!r}")


def _write_wav(path: str) -> None:
    os.makedirs(os.path.dirname(path), exist_ok=True)
    with wave.open(path, "wb") as w:
        w.setnchannels(1)
        w.setsampwidth(2)
        w.setframerate(32000)
        w.writeframes(_STUB_PCM)


def _make_worker(tmp: str):
    """Build a torch-free worker over a temp models_root with a packaged voice dir."""
    models_root = os.path.join(tmp, "models")
    voice_dir = os.path.join(models_root, VOICE)
    _write_wav(os.path.join(voice_dir, gw.PACKAGED_REF_AUDIO))
    with open(os.path.join(voice_dir, gw.PACKAGED_REF_TRANSCRIPT), "w", encoding="utf-8") as fh:
        fh.write(PACKAGED_PROMPT + "\n")
    out_base = os.path.join(tmp, "out")
    os.makedirs(out_base, exist_ok=True)
    worker = gw._Worker(models_root, os.path.join(models_root, "_base"), out_base)
    # Monkeypatch the ONLY torch-touching seam: return a fixed non-silent PCM buffer.
    worker._synthesize = lambda voice_id, text, prompt_text, ref_audio_path: (32000, _STUB_PCM)
    return worker, models_root, voice_dir, out_base


def q(*tokens: str) -> str:
    return " ".join(shlex.quote(t) for t in tokens)


# ══════════════════════════════════════════════════════════════════════════════

def test_import_is_torch_free() -> None:
    print("test_import_is_torch_free")
    check("torch" not in sys.modules, "importing gptsovits_worker pulled in torch")
    check("numpy" not in sys.modules, "importing gptsovits_worker pulled in numpy")


def test_happy_path_and_minted_path() -> None:
    print("test_happy_path_and_minted_path")
    with tempfile.TemporaryDirectory() as tmp:
        worker, _mr, voice_dir, out_base = _make_worker(tmp)
        ref = os.path.join(voice_dir, gw.PACKAGED_REF_AUDIO)
        resp = worker.handle(q("replicas set to three", ref, PACKAGED_PROMPT, "cut4", VOICE))
        check("\n" not in resp, f"OK not single line: {resp!r}")
        check(resp.startswith("OK "), f"expected OK, got {resp!r}")
        out_path = resp[3:]
        base = os.path.basename(out_path)
        check(re.fullmatch(rf"gso-{re.escape(VOICE)}-[0-9a-f]{{16}}\.wav", base) is not None,
              f"minted filename shape wrong: {base!r}")
        check(os.path.dirname(os.path.abspath(out_path)) == os.path.abspath(out_base),
              f"minted path not under out_base: {out_path!r}")
        # OK-after-write invariant (b): a successful OK implies the file exists.
        check(os.path.isfile(out_path), f"OK returned but file missing at {out_path!r} (write-before-OK invariant)")


def test_minted_path_deterministic_and_distinct() -> None:
    print("test_minted_path_deterministic_and_distinct")
    p1 = gw.mint_out_path("/o", VOICE, "hello", "prompt", "/r.wav", "cut4")
    p2 = gw.mint_out_path("/o", VOICE, "hello", "prompt", "/r.wav", "cut4")
    p3 = gw.mint_out_path("/o", VOICE, "HELLO", "prompt", "/r.wav", "cut4")
    check(p1 == p2, "identical requests must mint identical paths")
    check(p1 != p3, "distinct requests must mint distinct paths")


def test_bad_args_cases() -> None:
    print("test_bad_args_cases")
    with tempfile.TemporaryDirectory() as tmp:
        worker, _mr, voice_dir, _out = _make_worker(tmp)
        ref = os.path.join(voice_dir, gw.PACKAGED_REF_AUDIO)
        cases = {
            "blank line -> 0 tokens": "",
            "4 tokens": q("text", ref, PACKAGED_PROMPT, "cut4"),
            "6 tokens": q("text", ref, PACKAGED_PROMPT, "cut4", VOICE, "extra"),
            "unbalanced quote": '"unterminated ' + q(ref, PACKAGED_PROMPT, "cut4", VOICE),
            "empty text": q("", ref, PACKAGED_PROMPT, "cut4", VOICE),
            "empty ref (gotcha 5)": q("text", "", PACKAGED_PROMPT, "cut4", VOICE),
            "empty prompt (gotcha 6)": q("text", ref, "", "cut4", VOICE),
            "non-cut4 (gotcha 7)": q("text", ref, PACKAGED_PROMPT, "cut5", VOICE),
            "newline/CR in a token": q("bad\rtext", ref, PACKAGED_PROMPT, "cut4", VOICE),
        }
        for label, line in cases.items():
            _assert_single_typed_err(worker.handle(line), "bad-args", label)


def test_bad_voice() -> None:
    print("test_bad_voice")
    with tempfile.TemporaryDirectory() as tmp:
        worker, _mr, voice_dir, _out = _make_worker(tmp)
        ref = os.path.join(voice_dir, gw.PACKAGED_REF_AUDIO)
        _assert_single_typed_err(
            worker.handle(q("text", ref, PACKAGED_PROMPT, "cut4", "no-such-voice")),
            "bad-voice", "unknown voice")


def test_read_failed_missing_ref() -> None:
    print("test_read_failed_missing_ref")
    with tempfile.TemporaryDirectory() as tmp:
        worker, _mr, voice_dir, _out = _make_worker(tmp)
        # Inside this voice's dir (passes drift out-of-dir), transcript matches, but
        # the file does not exist -> read-failed (gotcha 5 missing-file path).
        missing = os.path.join(voice_dir, "nonexistent.wav")
        _assert_single_typed_err(
            worker.handle(q("text", missing, PACKAGED_PROMPT, "cut4", VOICE)),
            "read-failed", "missing ref file")


def test_drift_out_of_dir_and_transcript_mismatch() -> None:
    print("test_drift_out_of_dir_and_transcript_mismatch")
    with tempfile.TemporaryDirectory() as tmp:
        worker, _mr, voice_dir, _out = _make_worker(tmp)
        ref = os.path.join(voice_dir, gw.PACKAGED_REF_AUDIO)
        # out-of-dir ref while a packaged ref exists -> bad-args
        outside = os.path.join(tmp, "elsewhere-ref.wav")
        _write_wav(outside)
        _assert_single_typed_err(
            worker.handle(q("text", outside, PACKAGED_PROMPT, "cut4", VOICE)),
            "bad-args", "ref outside packaged dir")
        # in-dir ref but prompt_text drifts from the packaged transcript -> bad-args
        _assert_single_typed_err(
            worker.handle(q("text", ref, "a completely different transcript", "cut4", VOICE)),
            "bad-args", "prompt_text drift")


def test_drift_wrong_voice_dir_bad_voice() -> None:
    print("test_drift_wrong_voice_dir_bad_voice")
    # Phase-one roster has one voice, so the wrong-voice-dir branch is dormant; extend
    # the roster in-test to prove the forward-compat branch classifies as bad-voice.
    original = gw.ROSTER
    gw.ROSTER = (VOICE, "other-voice")
    try:
        with tempfile.TemporaryDirectory() as tmp:
            worker, models_root, _vd, _out = _make_worker(tmp)
            other_dir = os.path.join(models_root, "other-voice")
            other_ref = os.path.join(other_dir, gw.PACKAGED_REF_AUDIO)
            _write_wav(other_ref)
            # ref lives in ANOTHER voice's packaged dir -> bad-voice
            _assert_single_typed_err(
                worker.handle(q("text", other_ref, PACKAGED_PROMPT, "cut4", VOICE)),
                "bad-voice", "ref in a different voice's dir")
    finally:
        gw.ROSTER = original


def test_infer_failed() -> None:
    print("test_infer_failed")
    with tempfile.TemporaryDirectory() as tmp:
        worker, _mr, voice_dir, _out = _make_worker(tmp)
        ref = os.path.join(voice_dir, gw.PACKAGED_REF_AUDIO)

        def boom(*a, **k):
            raise RuntimeError("Failed to create AudioDecoder for None")  # the gotcha-5 torch error
        worker._synthesize = boom
        _assert_single_typed_err(
            worker.handle(q("text", ref, PACKAGED_PROMPT, "cut4", VOICE)),
            "infer-failed", "inference raised")


def test_write_failed_and_no_ok() -> None:
    print("test_write_failed_and_no_ok")
    with tempfile.TemporaryDirectory() as tmp:
        worker, _mr, voice_dir, _out = _make_worker(tmp)
        ref = os.path.join(voice_dir, gw.PACKAGED_REF_AUDIO)
        original = gw._atomic_write_wav

        def boom(sample_rate, pcm, out_path):
            raise OSError("disk full")
        gw._atomic_write_wav = boom
        try:
            resp = worker.handle(q("text", ref, PACKAGED_PROMPT, "cut4", VOICE))
        finally:
            gw._atomic_write_wav = original
        _assert_single_typed_err(resp, "write-failed", "write raised")
        # Load-bearing invariant: a write failure must NOT emit OK.
        check(not resp.startswith("OK"), f"write failure still returned OK: {resp!r}")


def test_one_response_per_request_framing() -> None:
    print("test_one_response_per_request_framing")
    with tempfile.TemporaryDirectory() as tmp:
        worker, _mr, voice_dir, _out = _make_worker(tmp)
        ref = os.path.join(voice_dir, gw.PACKAGED_REF_AUDIO)
        lines = [
            q("first", ref, PACKAGED_PROMPT, "cut4", VOICE),
            "",  # blank -> one ERR, not skipped
            q("third", ref, PACKAGED_PROMPT, "cut5", VOICE),  # bad-args
        ]
        responses = [worker.handle(ln) for ln in lines]
        check(len(responses) == len(lines), "response count must equal request count")
        for r in responses:
            check(r.count("\n") == 0, f"multi-line response: {r!r}")
        check(responses[0].startswith("OK "), f"line 0 should be OK: {responses[0]!r}")
        check(responses[1].startswith("ERR bad-args"), f"blank line should be ERR bad-args: {responses[1]!r}")
        check(responses[2].startswith("ERR bad-args"), f"cut5 should be ERR bad-args: {responses[2]!r}")


def test_shlex_golden_fixture() -> None:
    print("test_shlex_golden_fixture")
    fixture = os.path.join(_HERE, "testdata", "gso-shlex-golden.json")
    with open(fixture, encoding="utf-8") as fh:
        doc = json.load(fh)
    check(doc.get("wire_field_order") ==
          ["text", "ref_audio_path", "prompt_text", "text_split_method", "voice_id"],
          "golden fixture wire_field_order drifted from the wire contract")
    cases = doc["cases"]
    check(len(cases) >= 5, "golden fixture should carry a meaningful number of cases")
    for i, case in enumerate(cases):
        tokens = case["tokens"]
        wire = case["wire"]
        check(len(tokens) == 5, f"case {i}: golden tuple must have 5 tokens")
        # The cross-language contract: the worker's shlex.split(wire) reproduces tokens.
        split = shlex.split(wire)
        check(split == tokens, f"case {i}: shlex.split(wire) != tokens\n  wire={wire!r}\n  got={split!r}\n  want={tokens!r}")
        # And the worker's 5-token framing requirement holds for every golden line.
        check(len(split) == 5, f"case {i}: worker requires exactly 5 tokens; golden line splits to {len(split)}")


def test_ok_reply_is_literal_not_shlex_quoted() -> None:
    print("test_ok_reply_is_literal_not_shlex_quoted")
    with tempfile.TemporaryDirectory() as tmp:
        # An out_base WITH A SPACE makes the minted path contain a space, so a LITERAL
        # reply and a shlex-quoted reply are DISTINGUISHABLE (they are byte-identical
        # for a space-free hash path, which would make the assertion vacuous). Proves
        # #162 must parse the reply as line[3:] verbatim and must not round-trip-quote.
        models_root = os.path.join(tmp, "models")
        voice_dir = os.path.join(models_root, VOICE)
        _write_wav(os.path.join(voice_dir, gw.PACKAGED_REF_AUDIO))
        with open(os.path.join(voice_dir, gw.PACKAGED_REF_TRANSCRIPT), "w", encoding="utf-8") as fh:
            fh.write(PACKAGED_PROMPT + "\n")
        out_base = os.path.join(tmp, "out dir with spaces")
        os.makedirs(out_base, exist_ok=True)
        worker = gw._Worker(models_root, os.path.join(models_root, "_base"), out_base)
        worker._synthesize = lambda voice_id, text, prompt_text, ref_audio_path: (32000, _STUB_PCM)
        ref = os.path.join(voice_dir, gw.PACKAGED_REF_AUDIO)
        resp = worker.handle(q("replicas set to three", ref, PACKAGED_PROMPT, "cut4", VOICE))
        check(resp.startswith("OK "), f"expected OK, got {resp!r}")
        expected = gw.mint_out_path(out_base, VOICE, "replicas set to three",
                                    PACKAGED_PROMPT, ref, "cut4")
        check(" " in expected, f"test setup: minted path should contain a space: {expected!r}")
        # The reply payload is the LITERAL minted path (line[3:]) — verbatim.
        check(resp == "OK " + expected,
              f"OK reply is not the literal minted path: {resp!r} != {'OK ' + expected!r}")
        check(resp[3:] == expected, f"line[3:] must be the literal path: {resp[3:]!r}")
        # ...and it is NOT shlex-quoted (quoting a space-bearing path would differ).
        check(resp != "OK " + shlex.quote(expected),
              f"OK reply appears shlex-quoted; must be literal: {resp!r}")
        check(os.path.isfile(resp[3:]),
              f"the literal parsed path must be the real written file: {resp[3:]!r}")


def test_memory_error_propagates_as_fatal_not_err() -> None:
    print("test_memory_error_propagates_as_fatal_not_err")
    with tempfile.TemporaryDirectory() as tmp:
        worker, _mr, voice_dir, _out = _make_worker(tmp)
        ref = os.path.join(voice_dir, gw.PACKAGED_REF_AUDIO)

        def oom(*a, **k):
            raise MemoryError("simulated OOM inside the inference seam")
        worker._synthesize = oom
        # Honesty boundary: a fatal in the per-request handler must PROPAGATE (never a
        # per-block ERR). MemoryError is in FATAL_EXC, so `handle` re-raises rather than
        # degrading to `ERR infer-failed`.
        raised = False
        resp = None
        try:
            resp = worker.handle(q("text", ref, PACKAGED_PROMPT, "cut4", VOICE))
        except MemoryError:
            raised = True
        check(raised, f"MemoryError must propagate out of handle(), not be swallowed; got resp={resp!r}")
        check(resp is None, f"handle() must not return an ERR line for a fatal; got {resp!r}")
        check(MemoryError in gw.FATAL_EXC, "MemoryError must be in the FATAL_EXC set")


def _run_worker_proc(pysrc: str, stdin_text: str, env_extra: dict | None = None):
    """Run a short driver against the worker in a subprocess (stock python3, torch-free)
    and return (returncode, stdout, stderr)."""
    env = os.environ.copy()
    if env_extra:
        env.update(env_extra)
    proc = subprocess.run(
        [sys.executable, "-c", pysrc],
        input=stdin_text, capture_output=True, text=True, env=env, timeout=120,
    )
    return proc.returncode, proc.stdout, proc.stderr


def test_memory_error_exit_70_subprocess() -> None:
    print("test_memory_error_exit_70_subprocess")
    with tempfile.TemporaryDirectory() as tmp:
        out_dir = os.path.join(tmp, "out")
        # Torch-free: stub the startup torch gate + artifact check so construction
        # succeeds, then force the per-request handler to raise MemoryError. main()
        # must map the propagated FATAL_EXC to exit 70 and emit NO per-block OK/ERR.
        driver = (
            "import sys\n"
            "sys.path.insert(0, %r)\n"
            "import gptsovits_worker as gw\n"
            "gw._assert_torch_present = lambda: None\n"
            "gw._startup_check_artifacts = lambda models_root, base_root: None\n"
            "def _boom(self, line):\n"
            "    raise MemoryError('simulated OOM inside per-request handler')\n"
            "gw._Worker.handle = _boom\n"
            "sys.exit(gw.main(['--models-root', %r]))\n"
        ) % (_HERE, os.path.join(tmp, "models"))
        rc, out, err = _run_worker_proc(driver, "one request line\n", {"GSO_OUT_DIR": out_dir})
        check(rc == gw.EXIT_FATAL_RUNTIME,
              f"MemoryError in handler must exit {gw.EXIT_FATAL_RUNTIME}, got {rc}; stderr={err[:300]!r}")
        check("OK " not in out and "ERR " not in out,
              f"fatal must NOT degrade to a per-block OK/ERR line; stdout={out!r}")
        check("FATAL" in err and "MemoryError" in err,
              f"expected a FATAL MemoryError line on stderr; got {err[:300]!r}")


def test_startup_fatal_78_subprocess() -> None:
    print("test_startup_fatal_78_subprocess")
    with tempfile.TemporaryDirectory() as tmp:
        out_dir = os.path.join(tmp, "out")
        # Real worker (no stubs), empty models-root: whether torch is absent (this
        # session -> exit 78 from the torch gate) OR present-but-artifacts-missing
        # (-> FileNotFoundError -> exit 78), startup is a construction fault that
        # yields a clean FATAL exit 78, never a per-block ERR/OK line.
        driver = (
            "import sys\n"
            "sys.path.insert(0, %r)\n"
            "import gptsovits_worker as gw\n"
            "sys.exit(gw.main(['--models-root', %r]))\n"
        ) % (_HERE, os.path.join(tmp, "models"))
        rc, out, err = _run_worker_proc(driver, "one request line\n", {"GSO_OUT_DIR": out_dir})
        check(rc == gw.EXIT_FATAL_STARTUP,
              f"missing torch/artifacts must exit {gw.EXIT_FATAL_STARTUP} at startup, got {rc}; stderr={err[:300]!r}")
        check("FATAL" in err, f"expected a FATAL line on stderr; got {err[:300]!r}")
        check("OK " not in out and "ERR " not in out,
              f"startup fatal must NOT emit a per-block OK/ERR; stdout={out!r}")


def main() -> int:
    tests = [
        test_import_is_torch_free,
        test_happy_path_and_minted_path,
        test_minted_path_deterministic_and_distinct,
        test_ok_reply_is_literal_not_shlex_quoted,
        test_bad_args_cases,
        test_bad_voice,
        test_read_failed_missing_ref,
        test_drift_out_of_dir_and_transcript_mismatch,
        test_drift_wrong_voice_dir_bad_voice,
        test_infer_failed,
        test_memory_error_propagates_as_fatal_not_err,
        test_memory_error_exit_70_subprocess,
        test_startup_fatal_78_subprocess,
        test_write_failed_and_no_ok,
        test_one_response_per_request_framing,
        test_shlex_golden_fixture,
    ]
    for t in tests:
        t()
    if _failures:
        print(f"\nFAILED: {len(_failures)} check(s)")
        for f in _failures:
            print(f"  - {f}")
        return 1
    print(f"\nOK: all {len(tests)} contract tests passed (torch-free)")
    return 0


if __name__ == "__main__":
    sys.exit(main())
