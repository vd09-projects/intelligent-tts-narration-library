# pyright: reportMissingImports=false
# ^ Uses onnx / onnxruntime, which live in the Applio venv this runs under — not
#   necessarily the repo interpreter an editor points at. Resolves fine at runtime
#   under `make rvc-export`. See scripts/rvc-export/rvc-export.
"""Validate a voice's exported ONNX artifacts. Torch-free (onnx + onnxruntime + numpy).

For each of the three graphs (net_g, contentvec, rmvpe):
  - onnx.checker.check_model  (structural validity)
  - onnxruntime CPU InferenceSession load  (op coverage / real load)
  - numeric parity of the ONNX output vs the torch reference forward captured at
    export time (saved in *_refio.npz), reported as max-abs-diff + Pearson corr.

Exits non-zero if any graph is missing, fails to load, produces NaN/Inf, or drops
below its correlation floor. net_g's floor is lower because an internal NSF sine
dither (RandomNormalLike) is re-drawn per run by onnxruntime — the parity floor,
not an export defect (see pilot report §3/§5).
"""
import argparse
import sys

import numpy as np
import onnx
import onnxruntime as ort

import _common

# Correlation floors. net_g carries an internal per-run RandomNormalLike (sine
# dither, std ~0.003) so its floor is looser; contentvec/rmvpe are deterministic.
CORR_FLOOR = {"net_g": 0.99, "contentvec": 0.999, "rmvpe": 0.999}


def _parity(name, onnx_path, refio_path, feed_keys, ref_key, out_name):
    import os

    problems = []
    if not os.path.isfile(onnx_path):
        return [f"{name}: missing ONNX artifact {onnx_path}"]
    if not os.path.isfile(refio_path):
        return [f"{name}: missing reference IO {refio_path} (re-run export)"]

    print(f"\n=== {name} ({onnx_path}) ===")
    try:
        onnx.checker.check_model(onnx_path)  # by path → tolerates >2GB
        print("  onnx.checker: OK")
    except Exception as e:  # noqa: BLE001 - surface any checker failure
        return [f"{name}: onnx.checker failed: {e}"]

    so = ort.SessionOptions()
    so.intra_op_num_threads = 1
    try:
        sess = ort.InferenceSession(onnx_path, so, providers=["CPUExecutionProvider"])
        print("  onnxruntime session: OK")
    except Exception as e:  # noqa: BLE001
        return [f"{name}: onnxruntime failed to load: {e}"]

    d = np.load(refio_path)
    feeds = {k: d[k] for k in feed_keys}
    out = sess.run([out_name], feeds)[0]
    ref = d[ref_key]
    a, b = out.reshape(-1), ref.reshape(-1)
    n = min(len(a), len(b))
    a, b = a[:n], b[:n]

    if not np.all(np.isfinite(a)):
        problems.append(f"{name}: ONNX output has NaN/Inf")
    max_abs = float(np.max(np.abs(a - b)))
    corr = float(np.corrcoef(a, b)[0, 1])
    print(f"  onnx out shape: {out.shape}  torch ref shape: {ref.shape}")
    print(f"  parity  max_abs_diff={max_abs:.3e}  corr={corr:.6f}  (floor {CORR_FLOOR[name]})")

    if not np.isfinite(corr) or corr < CORR_FLOOR[name]:
        problems.append(f"{name}: parity corr {corr:.6f} below floor {CORR_FLOOR[name]}")
    return problems


def validate(slug):
    p = _common.voice_paths(slug)
    problems = []

    # net_g — the per-voice generator (parity is the load-bearing check).
    problems += _parity(
        "net_g",
        p["netg_onnx"],
        p["netg_refio"],
        feed_keys=["phone", "phone_lengths", "pitch", "nsff0", "sid", "rnd"],
        ref_key="ref_audio",
        out_name="audio",
    )
    # contentvec — shared embedder.
    problems += _parity(
        "contentvec",
        _common.CONTENTVEC_ONNX,
        _common.CONTENTVEC_REFIO,
        feed_keys=["audio"],
        ref_key="ref",
        out_name="feats",
    )
    # rmvpe — shared f0 E2E net.
    problems += _parity(
        "rmvpe",
        _common.RMVPE_ONNX,
        _common.RMVPE_REFIO,
        feed_keys=["mel"],
        ref_key="ref",
        out_name="hidden",
    )

    print()
    if problems:
        print(f"VALIDATION FAILED for '{slug}':")
        for pr in problems:
            print("  -", pr)
        sys.exit(1)
    print(f"VALIDATION OK for '{slug}': net_g + contentvec + rmvpe all checked, loaded, parity-clean.")


def main():
    ap = argparse.ArgumentParser(description="Validate an RVC voice's ONNX artifacts.")
    ap.add_argument("--voice", required=True, help="voice slug, e.g. cool-jahns")
    validate(ap.parse_args().voice)


if __name__ == "__main__":
    main()
