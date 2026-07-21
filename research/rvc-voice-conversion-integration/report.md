# RVC voice-conversion integration — decision + build plan

**Status:** decision locked (by-ear parity confirmed 2026-07-22). Gear DEEP · Intent FEASIBILITY+APPROACH.

## Decision

Run our trained RVC voices as a **torch-free ONNX** repaint step, driven by an **ephemeral per-job Python worker**, wrapped as a Go `render.Renderer` **decorator** over the Kokoro (sherpa) renderer. Shared construction is the "common code" both CLI and MCP call.

### Why (evidence-graded, all piloted)
- **Parity:** per-stage corr ≥0.9999, DSP-glue 1.0, perceptual log-mel 0.982, **user-confirmed by ear** = torch quality. `pilot-d/cooljahns_{TORCH,ONNX}.wav`.
- **Faster than torch on every axis:** cold 9.0s (mmap) vs 15.4s; warm 6.9s vs 7.8s/clip; RAM 2.75GB vs 4.5–5.9GB. Torch's tax is the ~5.5s import + faiss/libomp single-thread pin, both gone torch-free.
- **Ephemeral wins:** keep-alive saves only ~2s/clip but costs ~3GB idle → exit-after-job, **0 idle RAM** (user's constraint).
- **mmap = optional tuning flag, NOT architecture.** Its RSS win (2.75 vs 3.70GB) was a single-convert measurement; a per-job worker touches the whole index+weights across blocks → pages fault in → RSS climbs back ~3.9GB. Residual value: faster time-to-first-block + file-backed pages the OS can reclaim under memory pressure (no swap). Ship full-load first; add mmap in P1 only if RAM pressure bites.
- **D (in-process Go) rejected for now:** same onnxruntime native lib → same speed as C, but costs un-deferring CGo + a Go DSP port for zero gain. D = endgame, folded into the future `sherpa-onnx-go` CGo migration (shares the lib).

## Architecture — RVC as a decorator Renderer

```
                    render.Renderer (interface, unchanged)
                              ▲
          ┌───────────────────┴───────────────────┐
   plain: sherpa.Engine            RVC on: rvc.Renderer  ── wraps ──►  sherpa.Engine
          (Kokoro 24kHz WAV/block)         │ per block: 24kHz WAV → RVC worker → 40kHz WAV
                                           │ rebuild Timeline, Format=40kHz
                                           ▼
                                  scripts/rvc_worker.py (ephemeral, per Render call)
                                  contentvec.onnx → index-blend(faiss) → rmvpe f0
                                  → net_g.onnx → 40kHz WAV.  torch-free venv.
                                  spawned at Render start, warm across blocks, EXITS at end.
```

- `Render(plan)`: inner.Render → all block WAVs (24k) → spawn ONE worker → stream every block through it (load paid once, warm across blocks) → collect repainted 40k WAVs → rebuild Timeline + set Format 40kHz → worker exits. **Load once/doc, 0 idle RAM.**
- `RenderBlock(plan,id)`: inner.RenderBlock → one-shot convert → exit (escalation path; cold, but one block).
- Worker protocol: **stdin/stdout line loop** (matches the existing `scripts/kokoro` subprocess idiom): Go writes `<in> <out> <voice> <index_rate> <pitch>\n`, worker replies `OK <out>` / `ERR ...`, EOF → worker exits.

## CLI + MCP flow (the shared seam)

RVC adds a **character-voice axis** orthogonal to `gender`. New optional arg selects an RVC voice; selecting one = opt into the repaint. Absent = plain Kokoro (no RVC, unchanged).

```
MCP  speak {text, level, sink, gender, voice?}          CLI  narrate --gender --voice?
        │                                                       │
        └────────────► buildRenderer(rvcVoice) ◄────────────────┘   (ONE shared helper)
                              │
             rvcVoice==""  ───┤───  rvcVoice set (cool-jahns|confident-neal)
                    │                        │
             sherpa.New(...)        rvc.New(sherpa.New(...), cfg)
                                    + Kokoro source voice = the one matching the RVC target:
                                      cool-jahns → am_michael, pitch 0, index_rate 0.75
                                      confident-neal → af_bella, pitch 0, index_rate 0.5
```

- One `buildRenderer` helper in the composition layer, called by `cmd/narrate`, `cmd/narrate-mcp`, `cmd/narrate-server`, `cmd/narrate/listen_run` — replaces the 5 bare `sherpa.New(EngineConfig{})` sites. **This is the common code.**
- `gender` still picks the plain Kokoro voice when no RVC voice is set. When an RVC voice IS set, the decorator picks the Kokoro *source* voice appropriate to that target (gender becomes implied by the character).

## Build phases

- **Phase 0 — Export tooling (one-time).** Productionize the pilot export scripts into `make rvc-export VOICE=<v>` (Applio venv / torch, offline) → `net_g.onnx` per voice + shared `contentvec.onnx`/`rmvpe.onnx` once. Store under `assets/rvc-models/<voice>/onnx/` + `_shared/`; add to the HF backup (big, gitignored).
- **Phase 1 — Python RVC worker (`scripts/rvc_worker.py` + `scripts/rvc`).** Evolve `onnx_infer.py`: stdin/stdout loop, load-once-per-process, torch-free lean venv (onnxruntime+numpy+faiss+scipy+librosa, NO torch). Standalone parity test. (Full-load; mmap is an optional flag added only if RAM pressure bites — see decision note.)
- **Phase 2 — Go `render/rvc` decorator.** `rvc.New(inner, cfg)`; Render/RenderBlock; voice→source+pitch map; Timeline rebuild; Format=40kHz; worker-missing/crash → error (honesty). Table-driven tests + a fake worker (like `fake-kokoro.sh`) for CI.
- **Phase 3 — CLI + MCP + server wiring.** Shared `buildRenderer`; `--voice` flag; MCP `voice` arg on `speak`/`speak_last`; verify Format propagates through sink/timeline at 40kHz.
- **Phase 4 — verify + docs.** End-to-end narrate `--voice cool-jahns` → hear it; `/verify` by ear; Makefile targets; integration doc; decision-journal entry.

## Locked decisions (2026-07-22)
1. **Arg name = `voice`** — MCP `voice: cool-jahns`, CLI `--voice cool-jahns`. NOTE the layer distinction: this user-facing `voice` = RVC *character*; it is NOT the internal `RenderOptions.Voice` (Kokoro engine id). When `voice=cool-jahns`, the composition sets RVC target=cool-jahns AND internal Kokoro source `RenderOptions.Voice=am_michael`. User never sets the Kokoro id directly under RVC.
2. **Format = 40kHz end-to-end** when RVC on. Decorator sets `RenderResult.Format` = 40kHz mono s16le; every block WAV is 40kHz; Timeline rebuilt from repainted WAVs. Build check: confirm `sink/persistent` (patch/frame-align, ms↔byte) reads `Format.SampleRate` and doesn't assume 24000.
3. **Unavailable = error + stop.** RVC worker/artifacts missing or worker crash → hard error with a fix hint (`make rvc-export`). No silent degrade — honesty invariant. (`Status=degraded` reserved for fidelity drops, not voice substitution.)
4. **Scope = every block.** All classes (prose/code/heading/refusal) repainted → consistent character voice. Cost ~7s/block CPU accepted.
