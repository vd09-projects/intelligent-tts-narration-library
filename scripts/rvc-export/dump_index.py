# pyright: reportMissingImports=false
# ^ This tool runs under the Applio venv (faiss), NOT the repo's torch-free venv,
#   so an editor pointed at the repo interpreter can't resolve faiss. It resolves
#   fine at runtime under `make rvc-export`. See scripts/rvc-export/rvc-export.
"""Dump a voice's FAISS retrieval vectors torch-free (what a Go kNN would read).

Reads  assets/rvc-models/<slug>/<Name>.index
Writes assets/rvc-models/<slug>/onnx/index_vectors.npy   (reconstruct_n, float32)

The .index itself is kept (the runtime may use faiss IVF search directly); this
npy is the portable brute-force-kNN fallback.
"""
import argparse

import faiss
import numpy as np

import _common


def dump(slug):
    p = _common.require_voice(slug)
    idx_path = p["index"]
    import os

    if not os.path.isfile(idx_path):
        raise SystemExit(f"missing faiss index: {idx_path}")

    index = faiss.read_index(idx_path)
    print(f"[{slug}] ntotal: {index.ntotal} dim: {index.d} metric: {index.metric_type}")
    big_npy = index.reconstruct_n(0, index.ntotal).astype(np.float32)
    print("reconstructed big_npy:", big_npy.shape, big_npy.dtype)
    out = p["index_vectors"]
    np.save(out, big_npy)
    print("saved", out, "size MB:", round(big_npy.nbytes / 1e6, 1))


def main():
    ap = argparse.ArgumentParser(description="Dump an RVC voice's faiss index vectors to npy.")
    ap.add_argument("--voice", required=True, help="voice slug, e.g. cool-jahns")
    dump(ap.parse_args().voice)


if __name__ == "__main__":
    main()
