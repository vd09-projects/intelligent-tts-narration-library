"""Shared paths + setup for the RVC → ONNX export tooling.

Productionized from pilot-d (see scripts/rvc-export/README references). Torch is
used ONLY during export (offline); the runtime consumes the emitted ONNX/npy.

Layout produced under assets/rvc-models/:
  <slug>/<Name>.pth               (input, hand-trained in Applio)
  <slug>/<Name>.index             (input, faiss retrieval)
  <slug>/onnx/net_g.onnx          (per-voice generator)
  <slug>/onnx/net_g_refio.npz     (torch reference IO for parity validation)
  <slug>/onnx/index_vectors.npy   (faiss reconstruct_n dump for a Go kNN)
  _shared/onnx/contentvec.onnx    (voice-independent embedder)
  _shared/onnx/rmvpe.onnx         (voice-independent f0 E2E net)
  _shared/onnx/rmvpe_melbasis.npy (baked mel filterbank constant)
"""
import os
import sys

# Repo root derived from this file: <repo>/scripts/rvc-export/_common.py
_HERE = os.path.dirname(os.path.abspath(__file__))
REPO_ROOT = os.path.dirname(os.path.dirname(_HERE))
MODELS_DIR = os.path.join(REPO_ROOT, "assets", "rvc-models")
SHARED_DIR = os.path.join(MODELS_DIR, "_shared")
SHARED_ONNX_DIR = os.path.join(SHARED_DIR, "onnx")

# Applio (external checkout — holds the rvc.* python packages + embedder/predictor
# weights). Default: the sibling of this repo (…/TTS/Applio, right next to it) —
# no home/username assumption. Override with APPLIO_DIR if it ever moves.
APPLIO = os.environ.get("APPLIO_DIR") or os.path.join(os.path.dirname(REPO_ROOT), "Applio")

# Voice-independent shared artifacts.
CONTENTVEC_ONNX = os.path.join(SHARED_ONNX_DIR, "contentvec.onnx")
CONTENTVEC_REFIO = os.path.join(SHARED_ONNX_DIR, "contentvec_refio.npz")
RMVPE_ONNX = os.path.join(SHARED_ONNX_DIR, "rmvpe.onnx")
RMVPE_REFIO = os.path.join(SHARED_ONNX_DIR, "rmvpe_refio.npz")
RMVPE_MELBASIS = os.path.join(SHARED_ONNX_DIR, "rmvpe_melbasis.npy")

# Applio's embedder + predictor weights (torch, export-time only).
CONTENTVEC_DIR = os.path.join(APPLIO, "rvc", "models", "embedders", "contentvec")
RMVPE_PT = os.path.join(APPLIO, "rvc", "models", "predictors", "rmvpe.pt")


def voice_name(slug):
    """Slug → trained-model basename. cool-jahns → Cool_Jahns."""
    return "_".join(part.capitalize() for part in slug.split("-"))


def voice_paths(slug):
    name = voice_name(slug)
    vdir = os.path.join(MODELS_DIR, slug)
    onnx_dir = os.path.join(vdir, "onnx")
    return {
        "name": name,
        "dir": vdir,
        "pth": os.path.join(vdir, f"{name}.pth"),
        "index": os.path.join(vdir, f"{name}.index"),
        "onnx_dir": onnx_dir,
        "netg_onnx": os.path.join(onnx_dir, "net_g.onnx"),
        "netg_refio": os.path.join(onnx_dir, "net_g_refio.npz"),
        "index_vectors": os.path.join(onnx_dir, "index_vectors.npy"),
    }


def setup_applio():
    """Make Applio's `rvc.*` packages importable. Applio imports resolve relative
    to its own root, so cd there (mirrors the pilot + rvc-convert.sh)."""
    if not os.path.isdir(APPLIO):
        sys.exit(f"Applio not found at {APPLIO} (set APPLIO_DIR)")
    os.chdir(APPLIO)
    if APPLIO not in sys.path:
        sys.path.insert(0, APPLIO)


def require_voice(slug):
    """Validate the voice slug + its .pth input exist; return its path dict."""
    p = voice_paths(slug)
    if not os.path.isfile(p["pth"]):
        sys.exit(
            f"missing model for voice '{slug}': expected {p['pth']}\n"
            f"(voice dir must contain {p['name']}.pth + {p['name']}.index)"
        )
    os.makedirs(p["onnx_dir"], exist_ok=True)
    return p
