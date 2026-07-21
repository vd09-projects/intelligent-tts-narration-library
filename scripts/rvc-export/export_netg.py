# pyright: reportMissingImports=false
# ^ This tool runs under the Applio venv (torch + rvc.*), NOT the repo's torch-free
#   venv, so an editor pointed at the repo interpreter can't resolve those imports.
#   They resolve fine at runtime under `make rvc-export`. See scripts/rvc-export/rvc-export.
"""Export a voice's net_g (RVC infer path) to ONNX with an EXTERNAL `rnd` input.

Reads  assets/rvc-models/<slug>/<Name>.pth
Writes assets/rvc-models/<slug>/onnx/net_g.onnx
       assets/rvc-models/<slug>/onnx/net_g_refio.npz   (torch reference IO for parity)

Torch is used ONCE here (offline). Keeps every pilot-d gotcha fix:
  - remove_parametrizations weight-norm bake (new parametrizations.weight_norm API)
  - LayerNorm monkeypatch (static normalized_shape so ONNX export accepts it)
  - external `rnd` flow-noise input (deterministic parity vs torch)
  - dynamic time axis; TorchScript exporter (dynamo=False)
"""
import argparse
import os

import numpy as np
import torch

import _common


def _patch_layernorm():
    # Applio's custom LayerNorm passes a dynamic x.size(-1) as normalized_shape,
    # which ONNX export rejects as non-constant. gamma has fixed shape (channels,),
    # so use that static python int. In-memory only — no Applio source edit.
    from rvc.lib.algorithm.normalization import LayerNorm as _LN

    def _ln_forward_static(self, x):
        x = x.transpose(1, -1)
        n = self.gamma.shape[0]  # static python int
        x = torch.nn.functional.layer_norm(x, (n,), self.gamma, self.beta, self.eps)
        return x.transpose(1, -1)

    _LN.forward = _ln_forward_static


class NetGInferONNX(torch.nn.Module):
    """Wraps Synthesizer.infer with rnd as an external input (matches
    RVC-Project models_onnx.py)."""

    def __init__(self, synth):
        super().__init__()
        self.synth = synth

    def forward(self, phone, phone_lengths, pitch, nsff0, sid, rnd):
        g = self.synth.emb_g(sid).unsqueeze(-1)
        m_p, logs_p, x_mask = self.synth.enc_p(phone, pitch, phone_lengths)
        z_p = (m_p + torch.exp(logs_p) * rnd * 0.66666) * x_mask
        z = self.synth.flow(z_p, x_mask, g=g, reverse=True)
        o = self.synth.dec(z * x_mask, nsff0, g=g)
        return o  # [1, 1, T*400]


def export(slug):
    p = _common.require_voice(slug)
    _common.setup_applio()
    _patch_layernorm()
    from rvc.lib.algorithm.synthesizers import Synthesizer

    cpt = torch.load(p["pth"], map_location="cpu", weights_only=True)
    config = cpt["config"]
    # mirror Applio setup_network exactly
    config[-3] = cpt["weight"]["emb_g.weight"].shape[0]  # spk_embed_dim
    use_f0 = cpt.get("f0", 1)
    version = cpt.get("version", "v1")
    text_enc_hidden_dim = 768 if version == "v2" else 256
    vocoder = cpt.get("vocoder", "HiFi-GAN")
    print(f"[{slug}] config: {config} use_f0: {use_f0} version: {version} vocoder: {vocoder}")

    net_g = Synthesizer(
        *config, use_f0=use_f0, text_enc_hidden_dim=text_enc_hidden_dim, vocoder=vocoder
    )
    del net_g.enc_q  # infer path never uses the posterior encoder
    net_g.load_state_dict(cpt["weight"], strict=False)
    print("load_state_dict done (strict=False)")

    # Model uses new parametrizations.weight_norm; the built-in remover crashes
    # (iterates the deleted enc_q + old-hook API). Bake it into plain weights.
    from torch.nn.utils import parametrize

    nbaked = 0
    for m in net_g.modules():
        if parametrize.is_parametrized(m, "weight"):
            parametrize.remove_parametrizations(m, "weight", leave_parametrized=True)
            nbaked += 1
    print(f"baked weight-norm parametrizations on {nbaked} modules")
    net_g.eval().float()

    wrapper = NetGInferONNX(net_g).eval()

    T = 120
    inter_channels = config[2]  # 192
    phone = torch.randn(1, T, text_enc_hidden_dim, dtype=torch.float32)
    phone_lengths = torch.tensor([T], dtype=torch.int64)
    pitch = torch.randint(1, 255, (1, T), dtype=torch.int64)
    nsff0 = torch.rand(1, T, dtype=torch.float32) * 200 + 50
    sid = torch.tensor([0], dtype=torch.int64)
    rnd = torch.randn(1, inter_channels, T, dtype=torch.float32)

    with torch.no_grad():
        ref_o = wrapper(phone, phone_lengths, pitch, nsff0, sid, rnd)
    print("torch wrapper output shape:", tuple(ref_o.shape))

    dynamic_axes = {
        "phone": {1: "T"},
        "pitch": {1: "T"},
        "nsff0": {1: "T"},
        "rnd": {2: "T"},
        "audio": {2: "T_audio"},
    }

    out = p["netg_onnx"]
    torch.onnx.export(
        wrapper,
        (phone, phone_lengths, pitch, nsff0, sid, rnd),
        out,
        input_names=["phone", "phone_lengths", "pitch", "nsff0", "sid", "rnd"],
        output_names=["audio"],
        dynamic_axes=dynamic_axes,
        opset_version=17,
        do_constant_folding=True,
        dynamo=False,
    )
    print("exported", out, "size MB:", round(os.path.getsize(out) / 1e6, 1))

    np.savez(
        p["netg_refio"],
        phone=phone.numpy(),
        phone_lengths=phone_lengths.numpy(),
        pitch=pitch.numpy(),
        nsff0=nsff0.numpy(),
        sid=sid.numpy(),
        rnd=rnd.numpy(),
        ref_audio=ref_o.numpy(),
    )
    print("saved ref io npz:", p["netg_refio"])


def main():
    ap = argparse.ArgumentParser(description="Export an RVC voice's net_g to ONNX.")
    ap.add_argument("--voice", required=True, help="voice slug, e.g. cool-jahns")
    export(ap.parse_args().voice)


if __name__ == "__main__":
    main()
