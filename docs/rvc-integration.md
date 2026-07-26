# RVC Integration — how a trained voice threads through render / CLI / MCP

> **Scope.** This is the *integration* doc: how a finished RVC voice model plugs into the
> narration library and reaches the user through the CLI, the MCP `speak` tool, and the
> HTTP server. It is the sibling of two other docs — do not duplicate them here:
> - **`docs/rvc-voice-creation-runbook.md`** — how to *train* a `.pth`/`.index` voice from audio (upstream of this doc).
> - **`assets/rvc-models/README.md`** — the model artifacts on disk, export, and HF backup/restore.
>
> Design of record: `research/rvc-voice-conversion-integration/report.md` (Decisions 1–5).
> Delivered across #143 (export) · #144 (worker) · #145 (decorator) · #146 (CLI/MCP/server wiring) · #156 (unified roster) · **#147 (this: verify-by-ear + docs)**.

---

## 0. The one-liner

```
Kokoro writes the words (24 kHz)  →  RVC repaints the timbre (40 kHz)  →  the user hears a character voice
```

Kokoro controls *what* is said and the pacing; the RVC model controls *who* it sounds like. RVC is a
**decorator** over the existing Kokoro/sherpa renderer — turned on per job by naming an RVC voice, off
otherwise. Nothing in `planner/` or `plan/` knows RVC exists; the seam is entirely inside the `render/`
edge and the composition roots.

---

## 1. Architecture — a `render.Renderer` decorator

```
                     render.Renderer (interface, unchanged)
                               ▲
          ┌────────────────────┴────────────────────┐
   plain  │ sherpa.Engine                    RVC on  │ rvc.Renderer ── wraps ──► sherpa.Engine
          │ Kokoro 24 kHz WAV/block                  │ per block: 24 kHz WAV → worker → 40 kHz WAV
          │ Format = 24 kHz                          │ rebuild Timeline, Format = 40 kHz
          └──────────────────────────────────────────┘         │
                                                                ▼
                                        scripts/rvc_worker.py (ephemeral, one per Render)
                                        contentvec.onnx → faiss index-blend → rmvpe f0
                                        → net_g.onnx → 40 kHz mono s16le WAV. torch-free venv.
                                        spawned at Render start · warm across blocks · EXITS at job end.
```

- **`Render(plan)`** — inner sherpa renders every block to 24 kHz → spawn **one** worker → stream every
  non-empty block WAV through it (cold load paid once on block 1, warm thereafter) → frame-align each
  repainted 40 kHz WAV into `OutDir` → rebuild `Timeline` with `Format = 40 kHz` → worker exits. Load
  once per document, ~0 idle RAM.
- **`RenderBlock(plan, id)`** — the escalation path: inner renders one block → one cold-load worker
  exchange → exit. Other blocks' audio + sync untouched (per-block leveling invariant).
- **Empty-text blocks** (all-pause / no speech) are passed through with zero duration and empty
  `AudioRef`, mirroring sherpa — no worker exchange.

Code: `render/rvc/rvc.go` (decorator), `render/rvc/worker.go` (subprocess protocol),
`render/rvc/voice.go` (the target→source map).

### Worker wire protocol (v1, frozen)

The Go decorator drives the Python worker over a stdin/stdout line loop — the same idiom as
`scripts/kokoro`:

```
Go →  <in> <out> <voice> <index_rate> <pitch>\n
py ←  OK <out>              (success; 40 kHz WAV now exists at <out>)
py ←  ERR <category> <msg>  (recoverable; category ∈ bad-args|bad-voice|read-failed|infer-failed|write-failed)
EOF → worker exits.
```

`<pitch>` MUST be `0` in phase one (no transpose). Full contract:
`scripts/rvc-README`. The decorator classifies a missing/broken worker as a **returned error that stops
the pipeline** — never a silent degrade back to plain Kokoro (honesty rule, Decision 3).

---

## 2. How a voice reaches the renderer — the shared seam

There is **one** engine factory, `pipeline.BuildRenderer(voice)`, shared by every composition root.
It looks the voice up in the unified roster (`pipeline/voices.go`, the single source of truth) and
either returns plain sherpa (24 kHz) or wraps it in the RVC decorator (40 kHz):

```
CLI   narrate --voice <slug>          MCP  speak {…, voice?}          HTTP  narrate-server
   │                                     │  (gender = deprecated alias)   │
   └──────────────► pipeline.BuildRenderer(slug) ◄────────────────────────┘   (ONE shared helper)
                              │
     slug ∈ {af-bella, am-michael}  ──┤──  slug ∈ {cool-jahns, confident-neal}
       Kokoro, 24 kHz                 │        RVC, 40 kHz
       sherpa.New(EngineConfig{})     │        rvc.New(sherpa.New(...), rvc.Config{Voice: slug})
```

`BuildRenderer` returns the renderer **and** its `AudioFormat` as one object; each root hands that same
format to the format-validating persistent sink (`WithExpectedFormat`), so renderer-rate and
sink-expected-rate are coupled by construction — the sink never guesses 24000 vs 40000 (Decision 2).

### The roster (single source of truth)

`pipeline/voices.go`:

| Slug | Engine | Rate | Kokoro source | index_rate | Needs worker |
|---|---|---|---|---|---|
| `af-bella` | Kokoro | 24 kHz | `af_bella` | — | no |
| `am-michael` | Kokoro | 24 kHz | `am_michael` | — | no |
| `cool-jahns` | RVC | 40 kHz | `am_michael` (male ~134 Hz) | 0.75 | **yes** |
| `confident-neal` | RVC | 40 kHz | `af_bella` (female ~199 Hz) | 0.5 | **yes** |

User-facing slugs are **hyphenated** (`cool-jahns`); internal Kokoro engine ids are **underscored**
(`am_michael`). The hyphen↔underscore reconciliation lives only in the roster.

### The layer distinction (Decision 1) — the one thing to keep straight

The user-facing `voice` is the RVC **character**, *not* the internal `RenderOptions.Voice` (a Kokoro
engine id). When `--voice cool-jahns`:

1. the composition root selects RVC **target** `cool-jahns`, and
2. the decorator overrides `RenderOptions.Voice` to the Kokoro **source** `am_michael` before calling
   inner sherpa — **exactly once**, and it is the *only* place a target slug is translated
   (Locked Decision 5). sherpa's `resolveVoice` therefore only ever sees `{af_bella, am_michael}`.

The user never sets a Kokoro id directly under RVC. `gender` becomes implied by the character (and is a
deprecated alias only meaningful on the Kokoro path: `female→af-bella`, `male→am-michael`).

### MCP `speak` shape

```
speak { text | source, level, sink, gender?, voice? }
```

`voice` is the primary selector over the full roster; `gender` is the deprecated alias. Precedence is in
`speakArgs.effectiveVoice` (explicit `voice` wins). The arg is additive-compatible — `schema_version`
unchanged; older clients that omit `voice` are unaffected. Same threading on `speak_last` and
`speak_to_file`.

> **Gotcha — a running MCP server is a *binary*, not source.** A live `narrate-mcp` connected before
> #156 advertises only `gender` (no `voice`). After pulling this work, rebuild + restart it
> (`make build-mcp-bin`, then restart the client's MCP connection) or the `voice` arg is invisible to
> the client even though the code supports it.

---

## 3. Operations — export, worker, backup

RVC needs two local prerequisites before any RVC voice can render. Both are **gitignored + regenerable**
(large binaries; personal audio never in git).

### 3.1 One-time export — `.pth` → ONNX (torch, offline)

Turns the *training* outputs (`.pth`/`.index`) into the *runtime* artifacts the torch-free worker
consumes. Runs once in the Applio venv (torch used here only, offline):

```bash
make rvc-export-shared                 # voice-independent: _shared/onnx/{contentvec,rmvpe}.onnx + mel basis (once)
make rvc-export VOICE=cool-jahns       # per-voice: <slug>/onnx/net_g.onnx + index_vectors.npy (+ refio parity fixture)
make rvc-export VOICE=confident-neal
```

Artifacts land under `assets/rvc-models/` — see that dir's README for the full file map. Missing artifacts
make the worker fail cleanly at startup.

### 3.2 The torch-free inference worker

```bash
make rvc-worker-venv                    # builds .venv-rvc (python3.12) from scripts/rvc-requirements.txt;
                                        # ASSERTS torch is absent (Apache-2.0 posture — no GPL/torch linked)
```

The worker (`scripts/rvc_worker.py`, driven via the `scripts/rvc` wrapper) is onnxruntime + numpy +
faiss + scipy + librosa — **no torch**. It is a separate process, so no GPL linking (Apache-2.0
invariant). Run as an ephemeral per-job subprocess by the decorator; also usable standalone for a single
by-ear smoke:

```bash
make rvc-convert VOICE=cool-jahns IN=in.wav OUT=out.wav   # baked index_rate (0.75 / 0.5); INDEX_RATE=… to override
```

### 3.3 Backup / restore (HF)

`.pth`/`.index` and the ONNX artifacts all ride the `vd09-projects/rvc-voices` HF repo. Restore after a
machine wipe (or just re-run `make rvc-export`, a few minutes):

```bash
hf download vd09-projects/rvc-voices --repo-type model --local-dir assets/rvc-models
```

Full push/pull commands: `assets/rvc-models/README.md`.

---

## 4. Verify by ear (#147 acceptance)

RVC output is validated **by ear** during `/verify` — there is no golden audio in git (audio is
non-deterministic across runs and would bloat the repo). The objective signals below gate the ear check;
the ear check is the final word.

| What to prove | Command | Objective pass signals | Ear check |
|---|---|---|---|
| **CLI** `--voice cool-jahns` end-to-end | `make rvc-sanity` (or `make voice-sanity` for the full matrix) | per-voice `audio.wav` is **40 kHz mono s16le**; `manifest.json "voice"` == the slug (D6); non-silent | `afplay $OUT/cool-jahns/audio.wav` — sounds like Cool Jahns |
| **MCP** `speak` with `voice` | `make mcp-voice-sanity` (default `VOICE=cool-jahns`; `MCP_VOICE=<slug>` to change) | runSpeak returns `blocks_played > 0`, `total_duration_ms > 0`; drives `BuildRenderer → RVC decorator → afplay` | the afplay'd audio is the same character voice via MCP |
| single clip smoke | `make rvc-convert VOICE=cool-jahns IN=… OUT=…` | `OUT` written, 40 kHz | `afplay OUT` |

**What "objectively good" looks like** (from a real `rvc-sanity` run on `docs/samples/sample.md`):
`sample_rate 40000`, `channels 1`, `encoding pcm_s16le`, `"voice": "cool-jahns"`, ~191 s of non-silent
audio (RMS ≫ 0, peak ≈ −10 dBFS). If the manifest voice mismatches the slug, or the rate is 24000, or the
audio is silent → the RVC path is broken; do **not** sign off on ear alone.

> `make rvc-sanity` / `voice-sanity` / `mcp-voice-sanity` all require the worker prerequisites from §3
> (`make rvc-worker-venv` + `make rvc-export`). Without them the render errors loudly with a fix hint
> (honesty rule) — it does **not** fall back to Kokoro.

---

## 5. Adding a new RVC voice

1. **Train** it → `.pth` + `.index` (`docs/rvc-voice-creation-runbook.md`), drop under
   `assets/rvc-models/<new-slug>/`.
2. **Export** → `make rvc-export VOICE=<new-slug>`.
3. **Register** two entries, kept in sync:
   - `pipeline/voices.go` — roster row `{Slug, Engine: EngineRVC, RequiresWorker: true}`.
   - `render/rvc/voice.go` — `rvcVoices["<new-slug>"] = {source: <kokoro-id>, indexRate: …, pitch: 0}`
     (the `index_rate` must match the `rvc-convert` per-voice default).
4. **Verify** → `make voice-sanity` + `make mcp-voice-sanity MCP_VOICE=<new-slug>`, listen.

Both edits are the *only* places a voice is named; `BuildRenderer` and every root pick it up for free.

---

## 6. File map

| Path | Role |
|---|---|
| `render/rvc/rvc.go` | the decorator — Render/RenderBlock, Timeline rebuild, 40 kHz format |
| `render/rvc/voice.go` | target slug → {Kokoro source, index_rate, pitch}; the *only* translation point |
| `render/rvc/worker.go`, `config.go`, `wav.go`, `errors.go` | subprocess driver, timeouts, frame-align, error classes |
| `pipeline/voices.go` | the unified voice roster (single source of truth) |
| `pipeline/build_renderer.go` | `BuildRenderer` — the one shared engine factory |
| `cmd/narrate` · `cmd/narrate-mcp` · `cmd/narrate-server` | composition roots, each calls `BuildRenderer` |
| `scripts/rvc_worker.py` · `scripts/rvc` · `scripts/rvc-requirements.txt` | torch-free worker + wrapper + pinned deps |
| `scripts/rvc-export/` | `.pth`→ONNX export tooling (torch, offline) |
| `assets/rvc-models/` | model artifacts (gitignored) + `README.md` (export/backup) |
| `tests/rvc_parity/` | standalone parity + contract gate (`make rvc-parity`) |

## Related docs

- `docs/rvc-voice-creation-runbook.md` — train a voice (upstream).
- `assets/rvc-models/README.md` — artifacts, export, HF backup/restore.
- `docs/faiss-fix-debug-report.md` — the faiss/torch OpenMP segfault fix.
- `research/rvc-voice-conversion-integration/report.md` — the design + locked decisions.
