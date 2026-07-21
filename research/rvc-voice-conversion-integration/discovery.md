# Discovery — solution space (Stage 1)

50 candidates surfaced, 42 viable, 8 ruled out. 5 scouts (3 reframe + 2 lateral), grounded on the local Applio install + repo. Clustered below into runtime classes × integration boundary, plus cross-cutting levers.

Two orthogonal axes emerged:
- **Runtime:** torch · MLX · ONNX · CoreML · TorchScript/ExecuTorch/ggml
- **Boundary:** in-process Go · warm Python worker (socket/HTTP) · subprocess-per-call · per-doc batch

## Viable clusters (fan-out candidates)

### A — Warm torch worker (SAME math → quality parity by construction)
Collapses ~13 candidates: Applio-imported daemon, Applio's own in-tree `rvc/realtime` worker (worker.py+client.py, RealtimeVoiceConverter), rvc-python `api` mode, infer-rvc-python, CircuitCM/inferrvc, upstream RVC-WebUI api, w-okada server, UDS/gRPC/HTTP/stdin variants.
- **Why:** load torch+faiss+models ONCE, Go is client. Kills per-call cold start. Exact current inference → highest-confidence by-ear parity.
- **Cost:** keeps torch/Python. Least tech-consistent. Lowest risk.
- Sleeper detail: **Applio already ships a resident-model realtime worker in-tree** → least new code.

### B — MLX-native RVC (Apple-Silicon runtime, NO torch) ⭐ sleeper
`lextoumbourou/mlx-rvc` (MIT): auto-detects v2 768-dim .pth, **FULL faiss .index support via index-rate**, RMVPE + ContentVec. `Acelogic/RVC-MLX`: reports **8.71× faster than torch-MPS**.
- **Why:** drops torch → dissolves the faiss+torch OpenMP segfault; runs on Apple GPU (MLX Metal, not torch-MPS, so the MPS-crash constraint doesn't apply); loads far lighter than torch.
- **Cost:** still Python; reimplementation → parity must be ear-checked; mlx-rvc ~25 stars, license on Acelogic unconfirmed.

### C — Torch-free ONNX in a warm Python worker
`tts-with-rvc-onnx` (MIT, **keeps index_path+index_rate** — rare), `codename0og/RVC_Onnx_Infer` (v2-only), self-built net_g.onnx+contentvec.onnx+rmvpe.onnx+faiss, w-okada onnx.
- **Why:** torch-free runtime, kills segfault, onnxruntime matches the repo's kokoro-onnx stack. Warm worker kills cold start.
- **Cost:** needs the export (Cluster E); rmvpe may still pull torch (swap for rmvpe.onnx). Python stays.

### D — Fully in-process Go (the dream / CGo-plan alignment)
`yalue/onnxruntime_go` (ships arm64-darwin CPU libs) for net_g/contentvec/rmvpe + `.index` via **go-faiss** (DataIntelligenceCrew/blevesearch cgo) OR **pure-Go brute kNN** (retrieval math is ~6 lines over reconstructed vectors — no cgo).
- **Why:** zero Python, zero torch, no segfault, one Go process. Maximal tech-consistency, matches the deferred sherpa-onnx-go CGo path.
- **Cost:** highest build effort — must reimplement RVC pre/post DSP in Go (butter filter, RMS envelope, feature interp, pitch-protect blend) + wire 3 ONNX sessions. Depends on E.

### E — The EXPORT gate (enabler for C **and** D) 🔑
How to get a correct `net_g.onnx` from our **Applio HiFi-GAN** .pth. Options: **DIY `torch.onnx.export` via Applio's own Synthesizer class** (scout: state_dict keys match 1:1 enc_p/dec/flow/enc_q/emb_g; HiFiGAN convs export clean after `remove_weight_norm()`; gotchas = dynamic phone_lengths slice + internal randn, both solved by passing `rnd` as external input), RVC-Project official exporter, `visgotti/rvc-onnx-web` (pure-TS, torch-free, claims 100% corr), CoreML mlpackage, TorchScript, ExecuTorch.
- **KEY resolution:** faiss `.index` blends contentvec features **UPSTREAM of net_g** → a model-only net_g.onnx export does **NOT** lose the index. (Settles the earlier worry definitively.)

### F — Cross-cutting cheap levers (adopt regardless of runtime)
Per-document batching (1 RVC call/doc not per-block — fits the non-streaming design), content-hash cache (mirrors existing intelligence cache), convert-only-prose. Not rivals — optimizations layered on any winner.

## Ruled out (8, hard-constraint violations)
- **sherpa-onnx native RVC** — feature doesn't exist (issue #1128). The dream tech-fit, but can't load our .pth at all.
- **RVC→ONNX drop-index** — violates keep-index (the model-only regression).
- **voiceclonnx** — bakes speaker into .onnx at export, no index seam.
- **Bake timbre into TTS (fine-tune Piper/VITS)** — discards trained voices + already rejected (runbook §0).
- **knn-vc / seed-vc / FreeVC / so-vits-svc** — discard the trained .pth+.index; seed-vc also GPL (repo is GPL-averse).
