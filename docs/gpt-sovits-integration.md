# GPT-SoVITS Integration — how a cloned voice threads through render / CLI / MCP

> **Scope.** This is the *integration* doc: how the GPT-SoVITS (GSO) voice engine plugs
> into the narration library and reaches the user through the CLI, the MCP `speak` tool,
> and the HTTP server. It is the sibling of two other docs — do not duplicate them here:
> - **`docs/gpt-sovits-inference-runbook.md`** — how to stand up the local GPT-SoVITS
>   inference environment (venv, base models, gotchas) and run one WAV by hand (upstream of this doc).
> - **`assets/gptsovits-models/README.md`** — the model artifacts on disk + DR backup/restore runbook.
>
> Delivered across #161 (worker) · #162 (peer renderer) · #163 (roster/CLI/MCP wiring) ·
> #165 (warm-load machine proof) · **#164 (this: perf baseline + G2P coverage + verify-by-ear staging + docs)**.

---

## 0. The one-liner

```
GPT-SoVITS speaks the block's words directly (32 kHz)  →  a real Jahns reference clip anchors WHO it sounds like  →  the user hears a character voice
```

The planner controls *what* is said and the pacing (`Segment.Text`); the GPT-SoVITS
checkpoint pair plus a short reference clip control *who* it sounds like. Nothing in
`planner/` or `plan/` knows GSO exists; the seam is entirely inside the `render/` edge and
the composition roots.

---

## 1. Architecture — a `render.Renderer` PEER, **not** an RVC-style decorator

> **This is the load-bearing distinction (do not unify it into the RVC arm).**
> `render/gptsovits` is a **peer** `render.Renderer` alongside `render/sherpa` (Kokoro) —
> it is **NOT** a decorator like `render/rvc`. GPT-SoVITS is a text→audio zero-shot clone:
> it synthesizes a block's words *from scratch* from a `.ckpt`/`.pth` pair plus a real
> reference clip. There is **no base audio to repaint**, so there is nothing to wrap —
> wrapping Kokoro would synthesize every block **twice**. RVC repaints Kokoro's 24 kHz
> output into 40 kHz; GSO produces its own 32 kHz audio directly. A future engineer must
> not fold GSO into the RVC decorator arm.

```
                     render.Renderer (interface, unchanged)
                               ▲
        ┌──────────────────────┼───────────────────────────┐
 plain  │ sherpa.Engine   RVC  │ rvc.Renderer (decorator)   │ gptsovits.Engine (PEER)
 Kokoro │ 24 kHz/block   wraps │ 24 kHz → worker → 40 kHz    │ text → worker → 32 kHz/block
        │                sherpa│ (repaints Kokoro)           │ (own block loop; wraps NOTHING)
        └──────────────────────┴────────────────────────────┘              │
                                                                            ▼
                              scripts/gptsovits_worker.py (ONE warm process per document)
                              .ckpt/.pth loaded once (~17-22 s cold) · warm across blocks
                              · os.chdir(GSO_REPO) + seed=42 · 32 kHz mono s16le · torch venv.
```

`render/gptsovits` takes `render/sherpa`'s **shape** (it owns the block loop, derives
spoken words from the plan via `spokenTextFor`, writes one WAV per block keyed by block id,
builds `Timeline` + manifest from scratch) and drives it with `render/rvc`'s
**warm-subprocess-worker transport** (one warm process per document — sherpa's per-block
spawn would repay GSO's ~17-22 s cold load *every block*):

- **`Render(plan)`** — resolve the voice, spawn **one** warm worker, stream every non-empty
  block's `Segment.Text` through it (cold load paid once on block 1, warm thereafter), frame-
  align each 32 kHz WAV into `OutDir`, build `Timeline` with `Format = 32 kHz`, worker exits.
- **`RenderBlock(plan, id)`** — the escalation path: a single cold-load worker exchange
  (start → one exchange → close). Other blocks' audio + sync untouched (per-block leveling).
- **Empty-text blocks** (all-pause / no speech) are passed through with zero duration and
  empty `AudioRef` — no worker exchange, mirroring sherpa/rvc.
- **Refused blocks are still rendered:** their `Refusal.Message` goes through the same worker
  (honesty rule — a refusal is spoken, not skipped).
- **Honesty rule / D5:** a missing/broken worker OR any per-block `ERR` is a returned error
  that **stops** the whole `Render` — never a silent degrade back to Kokoro, never a Refusal,
  never a per-block skip.

Code: `render/gptsovits/gptsovits.go` (peer block loop, Timeline/manifest, 32 kHz),
`render/gptsovits/worker.go` (warm-subprocess transport), `render/gptsovits/voice.go`
(packaged ref-clip → wire tokens), `config.go` / `wav.go` / `errors.go` / `shlexquote.go`.
`OutputFormat()` is the single source of truth for the 32 kHz rate (`bytesPerMs = 64`).

### Worker wire protocol (v1, frozen — #161)

The Go peer drives the Python worker over a stdin/stdout line loop. It is **RVC-shaped, not
RVC-verbatim** — same idiom (positional tokens, one response line per request, a closed ERR
taxonomy) with three GSO-specific differences: the output path is **worker-minted**
(content-addressed), `<text>`/`<prompt_text>` are **free-text so shlex-quoting is
load-bearing**, and the engine emits **32 kHz**:

```
Go →  <text> <ref_audio_path> <prompt_text> <text_split_method> <voice_id>\n   (each token shlex-quoted)
py ←  OK <out>              (success; 32 kHz WAV now exists at the worker-minted <out>)
py ←  ERR <category> <msg>  (recoverable; category ∈ bad-args|bad-voice|read-failed|infer-failed|write-failed)
EOF → worker exits.
```

`<text_split_method>` MUST be `cut4` (gotcha 7); `<voice_id>` is `cool-jahns-gso`. Full
contract: the `scripts/gptsovits_worker.py` module docstring + `render/gptsovits/README.md`.

---

## 2. How a voice reaches the renderer — the shared seam

There is **one** engine factory, `pipeline.BuildRenderer(voice)`, shared by every
composition root. It looks the voice up in the unified roster (`pipeline/voices.go`, the
single source of truth) and returns the matching engine + its `AudioFormat`:

```
CLI   narrate --voice <slug>          MCP  speak {…, voice?}          HTTP  narrate-server
   │                                     │  (gender = deprecated alias)   │
   └──────────────► pipeline.BuildRenderer(slug) ◄────────────────────────┘   (ONE shared helper)
                              │
   slug ∈ {af-bella, am-michael}  ─┤─  {cool-jahns, confident-neal}  ─┤─  {cool-jahns-gso}
     Kokoro, 24 kHz                │      RVC decorator, 40 kHz        │     GSO peer, 32 kHz
     sherpa.New(EngineConfig{})    │      rvc.New(sherpa.New(...))     │     gptsovits.New(EngineConfig{})
```

`BuildRenderer` returns the renderer **and** its `AudioFormat` as one object; each root hands
that same format to the format-validating persistent sink (`WithExpectedFormat`), so
renderer-rate and sink-expected-rate are coupled by construction — the sink never guesses
24000 vs 40000 vs 32000. For a GSO voice the branch is a **bare peer**:
`gptsovits.New(gptsovits.EngineConfig{})` — no Kokoro source is wrapped.

### The roster (single source of truth)

`pipeline/voices.go`:

| Slug | Engine | Rate | Backing | Needs worker |
|---|---|---|---|---|
| `af-bella` | Kokoro | 24 kHz | `af_bella` | no |
| `am-michael` | Kokoro | 24 kHz | `am_michael` | no |
| `cool-jahns` | RVC (decorator) | 40 kHz | repaints `am_michael` | **yes** |
| `confident-neal` | RVC (decorator) | 40 kHz | repaints `af_bella` | **yes** |
| `cool-jahns-gso` | **GSO (peer)** | **32 kHz** | `.ckpt`/`.pth` + ref clip, wraps nothing | **yes** |

User-facing slugs are **hyphenated** (`cool-jahns-gso`). Unlike the RVC decorator (which
overrides `RenderOptions.Voice` to a Kokoro *source* id), the GSO peer never maps to a Kokoro
id — the voice is resolved per call from `RenderOptions.Voice` against the GSO roster
(`render/gptsovits/voice.go`), defaulting to `cool-jahns-gso`.

### MCP `speak` shape

```
speak { text | source, level, sink, gender?, voice? }
```

`voice` is the primary selector over the full roster (`voice: "cool-jahns-gso"`); `gender` is
the deprecated Kokoro-only alias. The arg is additive-compatible — older clients that omit
`voice` are unaffected.

> **Gotcha — a running MCP server is a *binary*, not source.** A live `narrate-mcp` connected
> before the `cool-jahns-gso` roster wiring (#163) advertises only the old roster and will not
> offer the GSO voice. After pulling this work, rebuild + restart it (`make build-mcp-bin`,
> then restart the client's MCP connection) or `cool-jahns-gso` is invisible to the client
> even though the code supports it. Same gotcha the RVC roster hit.

---

## 3. Operations — worker, base models, external clone

GSO needs three local prerequisites before any GSO voice can render. All are **gitignored +
regenerable** (large binaries + non-redistributable audio never in git).

### 3.1 The shared base models (~950 MB, once)

```bash
make gso-fetch-base                     # chinese-hubert-base + chinese-roberta-wwm-ext-large
                                        # + the v2Pro sv embedding → assets/gptsovits-models/_base/
```

Speaker-independent feature extractors — fetched once, shared by every GSO voice. Size-sanity
checked + fail-loud. See `assets/gptsovits-models/README.md` for the file map + HF restore.

### 3.2 The torch inference worker venv

```bash
make gso-worker-venv                    # builds .venv-gso (python3.11) from scripts/gso-requirements.txt;
                                        # ASSERTS torch PRESENT (inverse of the RVC worker), bakes gotchas 1-4
```

The GSO worker (`scripts/gptsovits_worker.py`, driven via the `scripts/gso` wrapper) **infers
with torch** — the inverse of the torch-free RVC worker. Torch is reached ONLY across the
stdin/stdout subprocess boundary, never linked into any Go binary (the same licensing boundary
as the RVC worker; keeps the Go library Apache-2.0-clean).

### 3.3 The external GPT-SoVITS code clone

The worker imports `GPT_SoVITS.*` from an **external clone**, resolved at run time via env
`GSO_REPO`, default `~/repos/GPT-SoVITS-local`:

```bash
git clone https://github.com/RVC-Boss/GPT-SoVITS ~/repos/GPT-SoVITS-local   # or export GSO_REPO=<path>
```

Only the **code** comes from the clone; all **weights** live under `assets/gptsovits-models/`.
The worker `os.chdir(GSO_REPO)` + `sys.path`-inserts it before importing (load-bearing —
`GPT_SoVITS/sv.py` uses CWD-relative imports + a module-global `sv_path`; proven in #165). It
consumes the `.ckpt`/`.pth` **directly** — there is **no ONNX export step** (a deliberate
difference from the RVC pipeline). It pins `seed=42` + the runbook sampling params so warm and
cold output are reproducible, and it mints a **content-addressed** output path (a de-facto
cache — identical requests resolve to one physical file, not one file per `.run()`).
Free-text `<text>`/`<prompt_text>` tokens are **shlex-quoted** on the wire.

> Details on the inert `TTS_Config` `sv_path`/`cnhubert` keys, the `os.chdir` requirement, and
> the fd-1→stderr wire isolation are in `docs/gpt-sovits-inference-runbook.md` and
> `decisions/architecture/2026-07-27-gso-warm-load-chdir-repo-inert-config-keys.md`.

---

## 4. Verify by ear (#164 staging)

GSO output is validated **by ear** during the human `/verify` session — there is **no golden
audio in git** (`cool-jahns-gso` is the same non-redistributable Jeremy Jahns clone as RVC's
`cool-jahns`; audio is non-deterministic across runs and would bloat/leak the repo). The
objective signals below **gate** the ear check (they are scripted, never an ear check
themselves); **the ear check is the final word.** These are *staged* for the human — the agent
produces the artifact + records the exact repro command, and never marks the by-ear criterion
"verified."

| What to prove | Command | Objective pass signals (scripted, gate the ear) | Ear check (human, final word) |
|---|---|---|---|
| **CLI** `--voice cool-jahns-gso` end-to-end | `make gso-sanity` (or `make voice-sanity` for the full matrix) | `audio.wav` is **32 kHz mono s16le**; `manifest.json "voice" == cool-jahns-gso`; exactly one `BlockTiming` per non-empty block; **non-silent** via a scripted RMS/peak probe on `audio.wav` | `afplay $OUT/cool-jahns-gso/audio.wav` — sounds like Jeremy Jahns |
| **MCP** `speak` with `voice` | `make mcp-voice-sanity MCP_VOICE=cool-jahns-gso` | `TestSpeakManualSmoke` green; speak-receipt `blocks_played > 0`; **non-silent** via the scripted probe on the ephemeral temp wav *if reachable* (the ephemeral path writes **no** `manifest.json` — do **not** assert a manifest voice for the MCP path) | the afplay'd audio is the same character voice via MCP |
| **G2P coverage** (pronunciation, textual) | `make gso-g2p-check` | half A: `Segment.Text` correct per structured class (no worker); half B: `g2p_en` ARPAbet string per `Segment.Text` (needs `.venv-gso`) | AC3-ear residue only — see `docs/samples/gso-g2p-coverage.md` |
| **Perf baseline** (go/no-go input) | `make gso-perf-baseline` | cold / warm-per-block (INFORMATIONAL) + **peak RSS ≤ 8 GB** (hard go/no-go), sampled off the `.venv-gso` worker pid | n/a (machine number) |

> `make gso-sanity` / `voice-sanity` / `mcp-voice-sanity` / `gso-perf-baseline` and half B of
> `gso-g2p-check` all require the worker prerequisites from §3 (`.venv-gso` + `gso-fetch-base`
> + a `GSO_REPO` clone). Without them the render errors loudly with a fix hint (honesty rule) —
> it does **not** fall back to Kokoro. Half A of `gso-g2p-check` needs no worker.

> **Known limitation — long verbatim-prose blocks exceed the 30 s per-block timeout.** With no
> intelligence adapter, long prose is read *in full* (degraded verbatim). GPT-SoVITS synthesis
> of such a block can take far longer than the renderer's `defaultPerBlockTimeout` of **30 s**
> (`render/gptsovits/config.go`) — the #164 perf baseline measured `docs/samples/sample.md`
> prose blocks at **60–161 s/block**. A block that overruns hard-stops the whole render (D5:
> `worker timed out: context deadline exceeded`). So `make gso-sanity` / `make mcp-voice-sanity`
> **cannot render `sample.md` end-to-end through GSO** today. For by-ear staging of the GSO
> voice, point them at a **short** document whose blocks each synthesize under 30 s
> (`make gso-sanity SAMPLE=<short.md>`); `mcp-voice-sanity`'s source is currently hardcoded to
> `sample.md`, so its GSO leg is blocked until that timeout is raised/scaled for long blocks
> (tracked as a follow-up, not fixed in #164 — the renderer engine is out of scope this batch).

**Known machine-checked ceilings (documented, not blockers):** `g2p_en` mis-phonemizes
hyphenated spelled-out cardinals ("twenty-four", "forty-two") and technical tokens ("L1",
"32 kHz"); see `docs/samples/gso-g2p-coverage.md` CEILINGS 1-2. The perf baseline + the
character likeness are recorded per-run into the PR body and the AC6 decision entry.

---

## 5. Adding a new GSO voice

1. **Fine-tune** a GPT-SoVITS clone elsewhere (Colab) → a `.ckpt` (GPT) + `.pth` (SoVITS)
   pair, plus a 3-10 s reference clip + its exact transcript (see
   `docs/gpt-sovits-inference-runbook.md`).
2. **Drop the artifacts** under `assets/gptsovits-models/<new-slug>/`:
   `gpt.ckpt`, `sovits.pth`, `ref_audio.wav`, `ref_transcript.txt` (the last two are the
   authoritative on-the-wire conditioning; `voice.go` reads them at resolve time).
3. **Register** two entries, kept in sync:
   - `pipeline/voices.go` — roster row `{Slug, Engine: EngineGSO, RequiresWorker: true}`.
   - `render/gptsovits/voice.go` — add the slug to `gsoVoices` (membership set).
4. **Verify** → `make voice-sanity` + `make mcp-voice-sanity MCP_VOICE=<new-slug>`, listen.

Both edits are the *only* places a GSO voice is named; `BuildRenderer` and every root pick it
up for free.

---

## 6. File map

| Path | Role |
|---|---|
| `render/gptsovits/gptsovits.go` | the **peer** engine — Render/RenderBlock, `spokenTextFor`, Timeline, 32 kHz |
| `render/gptsovits/voice.go` | roster slug → packaged `{ref_audio.wav, ref_transcript.txt}` wire tokens |
| `render/gptsovits/worker.go`, `config.go`, `wav.go`, `errors.go`, `shlexquote.go` | warm-subprocess driver, timeouts, frame-align, error classes, wire quoting |
| `pipeline/voices.go` | the unified voice roster (single source of truth) |
| `pipeline/build_renderer.go` | `BuildRenderer` — the one shared engine factory (bare GSO peer branch) |
| `cmd/narrate` · `cmd/narrate-mcp` · `cmd/narrate-server` | composition roots, each calls `BuildRenderer` |
| `cmd/plandump` | plan-only dump (no worker/audio) backing `gso-g2p-check` half A + `gso-perf-baseline` block feed (#164) |
| `scripts/gptsovits_worker.py` · `scripts/gso` · `scripts/gso-requirements.txt` | torch worker + venv wrapper + pinned deps |
| `scripts/gso_warmproof.py` | warm-load-across-blocks CORRECTNESS oracle (`make gso-warmproof`) |
| `scripts/gso_perf_baseline.py` | OFFICIAL cold/warm/peak-RSS baseline recorder (`make gso-perf-baseline`, #164) |
| `scripts/gso_g2p_dump.py` | `Segment.Text` + `g2p_en` phoneme-string dump (`make gso-g2p-check`, #164) |
| `assets/gptsovits-models/` | model artifacts (gitignored) + `README.md` (fetch/backup/restore) |
| `docs/samples/gso-g2p-coverage.md` | the G2P coverage input + findings (#164 AC3) |

## Related docs

- `docs/gpt-sovits-inference-runbook.md` — stand up the local inference env + gotchas (upstream).
- `assets/gptsovits-models/README.md` — artifacts, base-model fetch, HF backup/restore.
- `docs/samples/gso-g2p-coverage.md` — G2P pronunciation coverage + documented ceilings.
- `decisions/architecture/2026-07-27-gso-warm-load-chdir-repo-inert-config-keys.md` — the warm-load design.
- `docs/rvc-integration.md` — the sibling decorator-engine integration (contrast: peer vs decorator).
