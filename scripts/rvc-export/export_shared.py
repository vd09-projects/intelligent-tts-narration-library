# pyright: reportMissingImports=false
# ^ This tool runs under the Applio venv (torch + rvc.*), NOT the repo's torch-free
#   venv, so an editor pointed at the repo interpreter can't resolve those imports.
#   They resolve fine at runtime under `make rvc-export`. See scripts/rvc-export/rvc-export.
"""Export the voice-INDEPENDENT ONNX artifacts (once, shared across all voices).

Writes assets/rvc-models/_shared/onnx/
  contentvec.onnx        (HubertModelWithFinalProj → last_hidden_state, 768-dim)
  contentvec_refio.npz   (torch reference IO for parity validation)
  rmvpe.onnx             (RMVPE E2E net: mel → salience; STFT/mel/decode stay numpy)
  rmvpe_refio.npz        (torch reference IO)
  rmvpe_melbasis.npy     (exact 128×513 mel filterbank, baked constant for the runtime)

Skips if all artifacts already exist, unless --force. Torch is used ONCE here.
"""
import argparse
import os

import numpy as np

import _common


def _all_present():
    return all(
        os.path.isfile(f)
        for f in (
            _common.CONTENTVEC_ONNX,
            _common.CONTENTVEC_REFIO,
            _common.RMVPE_ONNX,
            _common.RMVPE_REFIO,
            _common.RMVPE_MELBASIS,
        )
    )


def _export_contentvec():
    import torch
    from rvc.lib.utils import HubertModelWithFinalProj

    model = HubertModelWithFinalProj.from_pretrained(_common.CONTENTVEC_DIR).float().eval()

    class ContentVecONNX(torch.nn.Module):
        """In: 16k mono audio [1, T_samples]. Out: last_hidden_state [1, T_frames, 768]."""

        def __init__(self, m):
            super().__init__()
            self.m = m

        def forward(self, audio):
            return self.m(audio)["last_hidden_state"]

    wrapper = ContentVecONNX(model).eval()
    T = 16000 * 3  # 3s dummy
    audio = torch.randn(1, T, dtype=torch.float32)
    with torch.no_grad():
        ref = wrapper(audio)
    print("torch contentvec out shape:", tuple(ref.shape))  # ~[1, 149, 768]

    out = _common.CONTENTVEC_ONNX
    torch.onnx.export(
        wrapper,
        (audio,),
        out,
        input_names=["audio"],
        output_names=["feats"],
        dynamic_axes={"audio": {1: "T"}, "feats": {1: "T_frames"}},
        opset_version=17,
        do_constant_folding=True,
        dynamo=False,
    )
    print("exported", out, "size MB:", round(os.path.getsize(out) / 1e6, 1))
    np.savez(_common.CONTENTVEC_REFIO, audio=audio.numpy(), ref=ref.numpy())
    print("saved ref io:", _common.CONTENTVEC_REFIO)


def _export_rmvpe():
    import torch
    from rvc.lib.predictors.RMVPE import RMVPE0Predictor

    pred = RMVPE0Predictor(_common.RMVPE_PT, device="cpu")
    e2e = pred.model.eval()  # DeepUnet+GRU: mel[1,128,F] → hidden[1,F,360]

    F = 32 * 5  # 160 frames, multiple of 32
    mel = torch.randn(1, 128, F, dtype=torch.float32)
    with torch.no_grad():
        ref = e2e(mel)
    print("torch rmvpe-e2e out shape:", tuple(ref.shape))  # [1, 160, 360]

    out = _common.RMVPE_ONNX
    torch.onnx.export(
        e2e,
        (mel,),
        out,
        input_names=["mel"],
        output_names=["hidden"],
        dynamic_axes={"mel": {2: "F"}, "hidden": {1: "F"}},
        opset_version=17,
        do_constant_folding=True,
        dynamo=False,
    )
    print("exported", out, "size MB:", round(os.path.getsize(out) / 1e6, 1))
    np.savez(_common.RMVPE_REFIO, mel=mel.numpy(), ref=ref.numpy())

    # Dump the exact mel-filter so a numpy/Go mel front-end matches bit-for-bit.
    mb = pred.mel_extractor.mel_basis.cpu().numpy()
    np.save(_common.RMVPE_MELBASIS, mb)
    print("mel_basis shape:", mb.shape, "-> saved", _common.RMVPE_MELBASIS)
    print("cents_mapping[:3]:", pred.cents_mapping[:3], "len:", len(pred.cents_mapping))


def export(force=False):
    os.makedirs(_common.SHARED_ONNX_DIR, exist_ok=True)
    if not force and _all_present():
        print("shared ONNX artifacts already present — skipping (use --force to rebuild)")
        return
    _common.setup_applio()
    _export_contentvec()
    _export_rmvpe()


def main():
    ap = argparse.ArgumentParser(description="Export voice-independent shared ONNX artifacts.")
    ap.add_argument("--force", action="store_true", help="rebuild even if artifacts exist")
    export(ap.parse_args().force)


if __name__ == "__main__":
    main()
