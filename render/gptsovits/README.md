# render/gptsovits — GPT-SoVITS peer renderer (#162)

A `render.Renderer` **peer** engine for GPT-SoVITS (GSO), alongside `render/sherpa`
(Kokoro). GPT-SoVITS speaks a block's words directly from a `.ckpt`/`.pth`
checkpoint pair plus a short reference clip (zero-shot clone) — so, unlike
`render/rvc` (a decorator that repaints an existing Kokoro render), there is nothing
to wrap. The renderer therefore takes **sherpa's shape** (owns the block loop,
derives spoken words from the plan, writes one WAV per block, builds Timeline +
manifest) driven by **rvc's warm-subprocess transport** (one warm worker per
document; AC2).

## Output format

**32 kHz mono PCM s16le, no resampling** — the GSO v2Pro native rate and a *third*
system rate (Kokoro 24 kHz, RVC 40 kHz, GSO 32 kHz). `OutputFormat()` is the single
source of truth; `bytesPerMs = 64`.

## Wire contract (frozen, #161)

Per-block request line — 5 positional shlex tokens, **no `<out>` token**:

```
<text> <ref_audio_path> <prompt_text> <text_split_method> <voice_id>
```

`text_split_method` is always `cut4`. The worker **mints** a content-addressed
output path and echoes it as `OK <out>`; the engine parses that payload as
`line[3:]` **literal** (never shlex-split — it may contain spaces). Per-block
failures are `ERR <category> <message>` over the closed set
`{bad-args|bad-voice|read-failed|infer-failed|write-failed}`. Startup fatal → exit
78, runtime fatal → exit 70, wrapper venv-missing → exit 2. The engine reads only
fd-1; the GPT-SoVITS stdout flood is shunted to fd-2 by the worker.

## Honesty rule (D5)

Any per-block worker `ERR`, a missing/broken worker, or a timeout is a **returned
error that stops the whole Render** — never a per-block skip, never a degrade, never
a Refusal. A refused *block* still speaks its `Refusal.Message` like any other block.

## Lifecycle

Each `Render` mints a private `GSO_OUT_DIR` (`os.MkdirTemp`) set on the child via
`cmd.Env = append(os.Environ(), "GSO_OUT_DIR=…")`, and `RemoveAll`s it on **every**
exit path (the worker never deletes its minted WAVs). The engine frame-aligns each
minted WAV into `OutDir/<blockID>.wav` **immediately after each `OK`** (before the
next request) — the buffer-before-next-request discipline that makes the worker's
content-addressed idempotent overwrite safe.

## Voices

`cool-jahns-gso` (phase one). The engine is **voice-neutral** (resolved from
`RenderOptions.Voice`, default `cool-jahns-gso`), matching sherpa's peer shape. The
`<ref_audio_path>` + `<prompt_text>` wire tokens are read from the packaged
source-of-record artifacts (`assets/gptsovits-models/<voice>/ref_audio.wav` +
`ref_transcript.txt`); the wire is authoritative and the worker owns drift.

## Testing

`go test ./render/gptsovits/` drives `testdata/fake-gptsovits.sh` — a torch-free fake
that speaks the real wire loop (no `.venv-gso`, no checkpoints, synthetic 32 kHz
silence). The cross-language shlex golden lives at
`scripts/testdata/gso-shlex-golden.json`. The real end-to-end smoke (`make
gso-sanity`) needs the GSO worker stack (`.venv-gso` + `make gso-fetch-base` + a
`GSO_REPO` clone) and is a manual `/verify` (#162 AC5 / #164) — the unit suite does
**not** depend on it.
