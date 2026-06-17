# Solution-Phase Design — Intelligent TTS Narration Library

**Status:** Step 1 deliverable (design only). No code yet. Stop-and-review checkpoint
before the vertical slice (step 2).
**Stack:** Backend/core/interfaces in **Go**. Reference player defaults to **React**.
**Authority:** Requirements from `problem-statement.md`; stack/build-order from
`cowork-brief-tts-narration.md`. Where they conflict, the rule in the brief applies
(requirements → problem statement; implementation → brief).

This document defines, in order: the module layout, the **narration-plan schema**
(the centerpiece — everything hangs off it), the four edge interfaces plus the core,
and how **leveling**, **refusal**, and **graceful degradation** flow through the plan.
It ends with the two real decision points and every open question. Assumptions are
stated inline as **[A#]** and collected at the end.

---

## 0. Library currency (verified June 2026, before pinning)

The brief asked me to check currency before citing versions. Three findings change
small things in the brief; none change the architecture.

- **Official MCP Go SDK is now stable.** `github.com/modelcontextprotocol/go-sdk`
  shipped v1.0.0 and is at **v1.5.0** (June 2026), maintained with Google, with
  **stdio, SSE, and Streamable HTTP all official**. The brief's "use `mark3labs/mcp-go`
  only if HTTP/SSE is needed" fallback is now moot — the official SDK covers those.
  **Recommendation: official SDK only.** (Phase 4.)
- **One Go binding can host both candidate engines.** `github.com/k2-fsa/sherpa-onnx-go`
  (Apache-2.0, actively published May 2026) runs **Kokoro** (`OfflineTtsKokoroModelConfig`)
  **and** Piper/VITS voices (`OfflineTtsVitsModelConfig`) via onnxruntime, fully offline.
  This makes "swap the engine behind the `Renderer` interface" a *config/model-file*
  change rather than a rewrite — it strengthens the design and feeds Decision 1.
- **Piper's engine relicensed to GPL.** Active development moved
  `rhasspy/piper` → **`OHF-Voice/piper1-gpl`** (GPL). Linking that into our binary would
  make the library GPL. Running Piper's **VITS voice models** through Apache-licensed
  `sherpa-onnx` (or shelling out to a separate Piper process) avoids inheriting GPL.
  Kokoro-82M remains **Apache-2.0**. This is a constraint that feeds Decision 1, not a
  blocker.

Versions are recorded for context only; nothing is pinned in this phase.

---

## 1. Module layout

The plan is the hub. `plan/` imports nothing from the project; everything imports
`plan/`. The core (`planner/`) imports only `plan/` and the *intelligence interface* —
never a concrete adapter, never anything that does I/O. All files, sockets, and pixels
live behind edge packages that are injected at the composition root (`pipeline/`,
`cmd/`). This makes "the core never touches files/sockets/pixels" a structural property
the compiler enforces, not a guideline.

```
intelligent-tts-narration-library/
├── go.mod
├── plan/              # THE CONTRACT. Pure Go types + JSON + versioning. Zero deps.
│   ├── plan.go        #   NarrationPlan, Block, Segment, SourceMap, Refusal, Provenance
│   ├── enums.go       #   Level, Class, Status, RefusalReason, SourceKind
│   ├── timeline.go    #   BlockTiming, Timeline (renderer's block-level sync output)
│   └── version.go     #   SchemaVersion + compatibility helpers
├── planner/           # INTELLIGENCE-LIGHT CORE. Pure, deterministic where possible. No I/O.
│   ├── segment.go     #   split into blocks on clean structural seams
│   ├── classify.go    #   prose/code/config/table/diagram_as_text/example (heuristic)
│   ├── voice.go       #   symbols/identifiers/paths → spoken words (deterministic lexicon)
│   ├── level.go       #   per-class L1/L2/L3 leveling
│   ├── degrade.go     #   policy when no IntelligenceAdapter is plugged in
│   └── planner.go     #   orchestrates → NarrationPlan; calls the IntelligenceAdapter iface
├── intelligence/      # PLUGGABLE COMPREHENSION (edge). Interface + impls.
│   ├── intelligence.go#   IntelligenceAdapter interface + request/result types
│   ├── mcpsampling/    #   client-LLM-over-MCP (zero extra cost)          [phase 4]
│   └── anthropic/      #   direct-API, user's own key                     [phase 2/3, optional]
├── render/            # RENDERER (edge). Interface + swappable engine backends.
│   ├── render.go      #   Renderer interface + RenderOptions/RenderResult/AudioFormat
│   └── sherpa/        #   default backend: sherpa-onnx-go hosting Kokoro (Piper-swappable)
├── adapter/           # INPUT ADAPTERS (edge). Interface + impls. Own all source I/O.
│   ├── adapter.go     #   InputAdapter interface + RawDocument/SourceRef/SourceMap
│   ├── file/          #   file → text + line-range source map               [phase 2]
│   ├── mcptext/       #   MCP text-in                                        [phase 4]
│   └── ocr/           #   screenshot OCR                                     [later]
├── sink/              # OUTPUT SINKS (edge). Interface + impls. Own all output I/O.
│   ├── sink.go        #   OutputSink interface + SinkReceipt
│   ├── ephemeral/     #   speak & forget → system audio device
│   └── persistent/    #   save audio + plan.json + source link              [phase 3]
├── pipeline/          # COMPOSITION. Thin DI wiring: adapter→planner→renderer→sink. No logic.
│   └── pipeline.go
├── cmd/
│   ├── narrate/       #   CLI (cobra)                                        [phase 2/3]
│   └── narrate-mcp/   #   MCP server (official go-sdk)                       [phase 4]
└── player/            # React reference player; consumes plan + timeline JSON [phase 5]
```

**Dependency rule (one direction):**
`plan/` ← everything. `planner/` → `plan/` + `intelligence` interface only.
`adapter/ render/ sink/ intelligence/` → `plan/` + their own interface.
`pipeline/ cmd/` → the concrete impls (the only packages that know which engine,
which adapter, which key). Swapping any edge is a one-line change here.

---

## 2. The narration plan (centerpiece — define this first)

The narration plan is the single load-bearing artifact. **One format for every input
and every output.** It is Go types on the inside and **JSON on the wire** (versioned,
language-neutral, additive-compatible) so non-Go consumers — the React player, an MCP
client — read it directly. The plan is engine-neutral: it carries *what to say* and
*where it came from*, never audio offsets. Timing is laminated on later by the renderer
(see §2.5), keyed by block id, so the plan stays portable.

### 2.1 Top level

```go
// plan/plan.go
type NarrationPlan struct {
    SchemaVersion string       `json:"schema_version"` // semver, e.g. "1.0"
    PlanID        string       `json:"plan_id"`        // stable id (ULID)
    CreatedAt     string       `json:"created_at"`     // RFC3339
    Source        SourceRef    `json:"source"`         // what was narrated (doc level)
    Defaults      PlanDefaults `json:"defaults"`       // requested level, voice hint, locale
    Blocks        []Block      `json:"blocks"`         // ordered narration units
    Diagnostics   []Diagnostic `json:"diagnostics,omitempty"` // plan-level notes
}

type SourceRef struct {
    Kind        SourceKind `json:"kind"`         // file | mcp_text | ocr_screenshot | ...
    URI         string     `json:"uri,omitempty"`// opaque handle; the CORE never opens it
    ContentHash string     `json:"content_hash"` // hash of the raw text → staleness check
    Adapter     string     `json:"adapter"`      // adapter name@version that produced it
}

type PlanDefaults struct {
    Level  Level  `json:"level"`            // 1|2|3 requested for the doc as a whole
    Voice  string `json:"voice,omitempty"`  // engine-neutral voice hint (renderer may ignore)
    Locale string `json:"locale"`           // "en" for phase one
}
```

### 2.2 Block — the unit of classification, leveling, sync, and re-render

A block is independently regenerable: a stable id, its own source map, its own level,
its own provenance. That is what makes "escalate *this one* block to L3" a local
operation (§3) rather than a whole-document re-plan.

```go
type Block struct {
    ID        string     `json:"id"`             // stable within plan, e.g. "b007"
    Order     int        `json:"order"`          // reading-order index
    Class     Class      `json:"class"`          // prose|code|config|table|diagram_as_text|example|heading|list|unknown
    Level     Level      `json:"level"`          // the level THIS block was planned at
    Status    Status     `json:"status"`         // voiced | degraded | refused
    SourceMap SourceMap  `json:"source_map"`     // back-reference (block-level sync)
    Segments  []Segment  `json:"segments,omitempty"` // the spoken pieces (empty if refused)
    SubBlocks []Block    `json:"sub_blocks,omitempty"` // only on clean structural seams
    Refusal   *Refusal   `json:"refusal,omitempty"`  // present iff Status=refused
    Provenance Provenance `json:"provenance"`    // how this block's text was produced
}

type Provenance struct {
    VoicedBy      string `json:"voiced_by"`            // "planner" | "intelligence" | "verbatim"
    Deterministic bool   `json:"deterministic"`        // true if pure structural rules produced it
    Model         string `json:"model,omitempty"`      // model id, when intelligence was used
    LevelAsked    Level  `json:"level_asked,omitempty"`// level requested from intelligence
}
```

### 2.3 Segment — what the renderer actually speaks

The planner resolves all *meaning* into final spoken words here: `replicas: 3` becomes
the literal text `"replicas set to three"`. **VoicingDirectives are optional phonetic
hints over spans of that text** (say-as spell-out, phoneme override, emphasis, rate) —
the renderer may ignore every one of them and still speak correct words. This is how the
renderer stays "dumb and fast, no semantics": the semantics already happened in the
planner; directives only refine pronunciation.

```go
type Segment struct {
    ID       string             `json:"id"`
    Kind     SegmentKind        `json:"kind"`            // speech | pause | earcon
    Text     string             `json:"text,omitempty"` // literal words to speak (already voiced)
    Voicing  []VoicingDirective `json:"voicing,omitempty"` // OPTIONAL phonetic hints (renderer may ignore)
    PauseMs  int                `json:"pause_ms,omitempty"`// for Kind=pause
}

type VoicingDirective struct {
    Start  int    `json:"start"`           // rune offset into Text
    End    int    `json:"end"`
    SayAs  string `json:"say_as,omitempty"`// "characters" | "digits" | "verbatim"
    Phoneme string `json:"phoneme,omitempty"` // engine-neutral pronunciation hint
    Emphasis string `json:"emphasis,omitempty"` // none|moderate|strong
}
```

### 2.4 SourceMap — one shape for files, screenshots, and raw text

The same field set covers every input adapter; only `Kind` and which sub-fields are set
change. This is what lets one plan format serve file input *and* OCR input without
forking the schema.

```go
type SourceMap struct {
    Kind       SourceKind `json:"kind"`                // line_range | char_span | pixel_region
    StartLine  int        `json:"start_line,omitempty"`// files
    EndLine    int        `json:"end_line,omitempty"`
    StartChar  int        `json:"start_char,omitempty"`// raw text / MCP
    EndChar    int        `json:"end_char,omitempty"`
    Region     *Rect      `json:"region,omitempty"`    // OCR: pixel rect {x,y,w,h}
    RawExcerpt string     `json:"raw_excerpt,omitempty"`// verbatim snippet for "jump to source"
}
```

### 2.5 Refusal — the honesty rule as first-class data, not an error

A refusal is **not** an exception that aborts the plan. It is a block with
`Status = refused` carrying a short honest notice and a pointer to the source. The plan
still renders end-to-end; the listener hears *"there's a graph here that isn't voiced;
check the source"* and gets block-level sync to exactly that region. Refuse-and-surface,
never fabricate.

```go
type Refusal struct {
    Reason    RefusalReason `json:"reason"`       // see enum below
    Message   string        `json:"message"`      // human surface line (spoken by default)
    Spoken    bool          `json:"spoken"`       // speak the notice? default true [A12]
    SourceMap SourceMap     `json:"source_map"`   // where the un-voiceable thing is
}

// plan/enums.go
type RefusalReason string
const (
    RefuseBareImage       RefusalReason = "bare_image_no_description"   // image with no description
    RefuseTooLarge        RefusalReason = "too_large_for_level"         // can't faithfully fit the level
    RefuseTooRaw          RefusalReason = "too_raw_to_voice"            // unparseable / noise
    RefuseNoIntelligence  RefusalReason = "no_intelligence_available"   // prose gist needs an adapter
    RefuseUnsupported     RefusalReason = "unsupported_content"         // e.g. unknown diagram dialect
)
```

**Error vs refusal boundary [A11]:** an adapter failing to *read* a source (file
unreadable, socket closed) is an **error** returned up the pipeline. Content that is
readable but cannot be voiced faithfully at the level is a **refusal** inside the plan.
Errors stop; refusals are spoken and surfaced.

### 2.6 Timeline — block-level sync the renderer adds after audio exists

Audio offsets are not in the plan (it must stay engine-neutral and pre-render). The
renderer emits a `Timeline` keyed by block id; a consumer JOINs it onto
`plan.Blocks[i].SourceMap` to get the full sync picture: *spoken segment → source block
→ lines/region*. **Block-level only — no per-word timings** (word-level contradicts gist
mode, where spoken text ≠ source text).

```go
// plan/timeline.go
type Timeline struct {
    PlanID  string        `json:"plan_id"`
    Format  AudioFormat   `json:"format"`   // sample rate, encoding
    Blocks  []BlockTiming `json:"blocks"`
}
type BlockTiming struct {
    BlockID  string `json:"block_id"`
    StartMs  int    `json:"start_ms"`
    EndMs    int    `json:"end_ms"`
    AudioRef string `json:"audio_ref,omitempty"` // file/offset handle; written by the SINK, not the core
}
```

### 2.7 Two concrete JSON examples

A voiced config block (deterministic — no intelligence needed):

```json
{
  "id": "b004", "order": 4, "class": "config", "level": 1,
  "status": "voiced",
  "source_map": { "kind": "line_range", "start_line": 12, "end_line": 18,
                  "raw_excerpt": "spec:\n  replicas: 3\n  ..." },
  "segments": [
    { "id": "s1", "kind": "speech",
      "text": "A Kubernetes Deployment. Replicas set to three, one container image nginx version 1.27." }
  ],
  "provenance": { "voiced_by": "planner", "deterministic": true }
}
```

A refused bare-image block (honesty rule):

```json
{
  "id": "b009", "order": 9, "class": "unknown", "level": 1,
  "status": "refused",
  "source_map": { "kind": "line_range", "start_line": 40, "end_line": 40,
                  "raw_excerpt": "![throughput chart](bench.png)" },
  "refusal": {
    "reason": "bare_image_no_description",
    "message": "There's a chart here that isn't voiced — check the source, lines 40.",
    "spoken": true,
    "source_map": { "kind": "line_range", "start_line": 40, "end_line": 40 }
  },
  "provenance": { "voiced_by": "planner", "deterministic": true }
}
```

---

## 3. Interfaces (the four edges + the core)

Four narrow interfaces, each owning the I/O the core must not touch. All four speak only
in `plan/` types and their own request/result structs.

### 3.1 InputAdapter — source → text + source map

```go
// adapter/adapter.go
type InputAdapter interface {
    // Read performs ALL source I/O (files, sockets, OCR) and returns raw text plus a
    // source map from text offsets back to origin (line ranges, pixel regions).
    Read(ctx context.Context, ref SourceRef) (RawDocument, error)
}
type RawDocument struct {
    Text      string
    OffsetMap []OffsetSpan // text char-span → SourceMap (origin); planner derives per-block maps
    Source    SourceRef
}
```

The planner segments `Text` into blocks and, via `OffsetMap`, computes each block's
`SourceMap`. The file adapter (phase 2) produces line ranges; the OCR adapter (later)
produces pixel regions — same `RawDocument` shape either way.

### 3.2 IntelligenceAdapter — pluggable comprehension (the narrow contract)

```go
// intelligence/intelligence.go
type IntelligenceAdapter interface {
    // Voice returns voiceable text for one block at the requested level.
    // Implementations: MCP-sampling (client LLM, zero extra cost) or direct-API (user key).
    // It MUST refuse rather than fabricate: return Refused=true (honesty rule).
    Voice(ctx context.Context, req IntelligenceRequest) (IntelligenceResult, error)
}
type IntelligenceRequest struct {
    BlockText string   // the block's raw text
    Class     Class    // planner's classification (helps the model frame its answer)
    Facts     []string // deterministic structural facts the planner already extracted
    Level     Level    // 1|2|3
    Locale    string
}
type IntelligenceResult struct {
    Text        string
    Model       string // for provenance
    Refused     bool   // explicit honest refusal from the model side
    RefusalNote string
}
```

This is the brief's narrow contract verbatim: *"here is a block and the level I want;
return voiceable text."* A `nil` adapter triggers graceful degradation (§6). The honesty
rule is honored from both sides: the adapter can return `Refused`, or be absent entirely.

### 3.3 Renderer — narration plan → audio + block timing (swappable, no semantics)

```go
// render/render.go
type Renderer interface {
    Render(ctx context.Context, plan NarrationPlan, opts RenderOptions) (RenderResult, error)
    // RenderBlock re-renders ONE block (escalation / regenerate). Other blocks untouched.
    RenderBlock(ctx context.Context, plan NarrationPlan, blockID string, opts RenderOptions) (BlockRender, error)
}
type RenderResult struct {
    Audio    AudioStream // handle; bytes are written out by the SINK, not the core
    Timeline Timeline    // block-level sync (§2.6)
    Format   AudioFormat
}
```

The default backend is `render/sherpa` wrapping `sherpa-onnx-go` with a Kokoro model.
Swapping to Piper-VITS is a different model file behind the same backend; swapping to a
different engine entirely is a new `Renderer` impl. `RenderBlock` exists so escalating
one block re-renders only that block's audio (§3, §4).

### 3.4 OutputSink — where audio + sync go

```go
// sink/sink.go
type OutputSink interface {
    Consume(ctx context.Context, plan NarrationPlan, res RenderResult) (SinkReceipt, error)
}
```

- **Ephemeral** (`sink/ephemeral`): stream audio to the system device, speak, forget.
  The "read me this answer" case, no UI. Phase 2.
- **Persistent** (`sink/persistent`): write audio file + `plan.json` (sync data + source
  link) + a manifest; return paths. Storage layout is deferred [A9]. Phase 3.

### 3.5 Planner + Pipeline (the core and the wiring)

```go
// planner/planner.go — pure, no I/O. Intelligence is optional (nil ⇒ degrade).
func Plan(ctx context.Context, doc RawDocument, req Request, intel IntelligenceAdapter) (NarrationPlan, error)
type Request struct {
    Level     Level            // doc default
    Overrides map[string]Level // per-block level overrides (escalation)
}

// pipeline/pipeline.go — the only place that knows concrete edges (composition root).
type Pipeline struct {
    In    InputAdapter
    Intel IntelligenceAdapter // may be nil
    Rend  Renderer
    Out   OutputSink
}
func (p Pipeline) Narrate(ctx context.Context, ref SourceRef, req Request) (SinkReceipt, error)
```

`planner.Plan` imports `plan/` and the `IntelligenceAdapter` interface — nothing else.
It cannot open a file or touch a socket because it has no dependency that can. The
pipeline injects the real edges; the core stays pure.

---

## 4. How leveling flows through the plan

Leveling is **per block and re-requestable**, not a global setting. It lives in the data
as `Block.Level` and is driven by `Request.Level` (doc default) plus
`Request.Overrides[blockID]` (escalation).

What L1/L2/L3 *mean* is per class [A6]. The crucial property: for **structured** classes
the planner produces a meaningful L1 gist **deterministically**, with no intelligence
adapter at all. Only prose truly needs the adapter.

| Class | L1 — Gist (deterministic for structured) | L2 — Summary | L3 — Detail |
|---|---|---|---|
| prose | one-line gist *(needs intelligence)* | paragraph summary *(intelligence)* | full read / thorough explanation |
| code | signature + shape: "a 12-line Go func `parsePlan` returning a plan and an error" | declared symbols + key calls | line-by-line meaning *(intelligence may enrich)* |
| config | "a K8s Deployment, replicas three, one image" | key settings voiced by meaning | every field |
| table | "a 4-column, 12-row benchmark table" | headers + notable rows *(intel enriches "notable")* | read meaningfully row by row |
| diagram_as_text | "a flowchart, 5 nodes, request path" | nodes + edges in meaning | full traversal |
| example | treated as code or prose by content | — | — |
| heading / list | spoken structurally at all levels | — | — |

**Escalation = explicit regenerate of one segment.** It never re-plans the document:

1. Consumer (CLI/MCP/GUI) sends `Regenerate{plan_id, block_id, target_level}`.
2. `planner.Plan` runs for that block only (its raw text is recoverable via `SourceMap`
   + content hash), invoking the intelligence adapter if the new level needs it.
3. The returned `Block` (same `id`, bumped `Level`, new `Segments`, updated `Provenance`)
   is spliced back into the plan.
4. `Renderer.RenderBlock` re-renders just that block's audio and patches its
   `BlockTiming`. Every other block — audio and sync — is untouched.

This is exactly the brief's "escalating a block is just an explicit regenerate of that
segment," and it falls out of the schema because blocks are independently addressable.
Stale audio elsewhere is acceptable and intended.

---

## 5. How refusal flows through the plan

The system is built so I can *stop reading*; therefore it must never invent. Refusal is a
status a block can hold, decided at two points and rendered identically:

- **Planner-side:** the block is un-voiceable by structure — a bare image with no
  description (`bare_image_no_description`), an unknown diagram dialect
  (`unsupported_content`), content too raw to parse (`too_raw_to_voice`), or a prose
  block over threshold with no intelligence available (`no_intelligence_available`).
- **Intelligence-side:** the adapter returns `Refused=true` (it judged it could not
  faithfully summarize at the level). The planner converts that into a `Refusal` with
  `too_large_for_level` (or carries the model's note).

Either way the planner emits a block with `Status = refused`, empty `Segments`, and a
`Refusal{message, source_map, spoken:true}`. Downstream:

- the **renderer** speaks the short honest notice (because `spoken=true`),
- the **timeline** still gets a `BlockTiming` for it, so seek-by-block and jump-to-source
  work *to the un-voiced region*,
- the **plan stays valid** and renders to the end.

The listener's experience: *"…and there's a chart here that isn't voiced — check the
source around line 40,"* then narration continues. Never a confident fabricated summary.

---

## 6. How graceful degradation flows through the plan

When `IntelligenceAdapter` is `nil`, the planner takes the deterministic path. Behavior
is **per class**, so you still get the structured content voiced intelligently — only
prose loses its gist (exactly as the brief specifies):

- **Structured classes** (code, config, table, diagram_as_text, heading, list): voiced by
  the planner's own deterministic rules at **all** levels. Classification, structural
  parsing, and symbol voicing are the *planner's* job, not comprehension, so they need no
  adapter. `Status = voiced`, `Provenance.Deterministic = true`. Intelligence, when
  present, only *enriches* L2/L3 (e.g. "notable rows"); its absence caps richness, never
  blocks voicing.
- **Prose** [A4]: with intelligence → summarized to the level. Without intelligence →
  read **verbatim** if within a configurable length threshold (working default ~120 words
  / ~45 s of audio); **over** the threshold → refuse-and-surface
  (`no_intelligence_available`: "a long prose section I can't summarize without an
  intelligence adapter — check the source"). Never a silently invented gist.

Every block records what happened (`Status`, `Provenance.VoicedBy`,
`Provenance.Deterministic`), so any consumer can see precisely what was comprehended,
what was voiced structurally, and what was refused. Degradation is therefore visible and
honest, not a silent quality drop.

`Status = degraded` is used when a block was voiced but at lower fidelity than the level
requested because intelligence was absent (e.g. prose read verbatim instead of gisted) —
distinct from `voiced` (fully met) and `refused` (not voiced).

---

## 7. Decision points (the two real forks — your call)

Both are wired as clean seams, so neither blocks progress on the vertical slice. I need a
choice before they're *finalized*, with my recommendation below.

### Decision 1 — Final TTS engine **and** integration mechanism

Two sub-choices, behind the same `Renderer` interface:

*Integration mechanism*
- **Subprocess** (shell out to a local TTS CLI per block): simplest, no CGo, trivial to
  swap, clean process boundary, sidesteps Piper's GPL entirely; costs a process spawn per
  call and a bit of latency.
- **In-process CGo via `sherpa-onnx-go`**: one binding runs Kokoro *and* Piper/VITS,
  lower latency, in-memory; costs a CGo + onnxruntime build dependency.

*Default voice/model*
- **Kokoro-82M** (Apache-2.0): best naturalness for its size, ideal for narration.
- **Piper** (fast, light, broadest languages if multilingual returns): VITS voices,
  engine now GPL — prefer running its voices via sherpa-onnx, not linking `piper1-gpl`.

**Recommendation:** ship the vertical slice with the **subprocess** mechanism for
legibility (no CGo in the first slice), defaulting to **Kokoro-82M** for narration
quality, and document **`sherpa-onnx-go`** as the in-process upgrade path that can host
*both* engines when latency or packaging starts to matter. This keeps the engine genuinely
swappable, honors "free + local," and avoids GPL. **Your call to finalize.**

### Decision 2 — Reference player: **React** (default) vs Flutter

- **React (default):** lightest thing to wire to a local Go service over HTTP/socket;
  pairs naturally with the CLI; right for a desktop/terminal-side companion.
- **Flutter:** choose only if you want the player on **mobile / truly cross-platform**.

**Recommendation:** **React**, per the brief, unless you tell me you want mobile playback.
The GUI is a downstream consumer of the same plan + timeline the CLI already uses, so this
decision blocks nothing earlier than phase 5. **Your call.**

---

## 8. Open questions

Each carries my working assumption so nothing stalls (per "state assumptions inline").
Confirm or redirect; I'll proceed on the assumption otherwise.

1. **[A1] Design-doc home & format** — markdown at `docs/solution-phase-design.md` in the
   repo (this file). Assumed yes.
2. **[A2] Audio format** — 24 kHz mono PCM/WAV for phase one (Kokoro's native rate).
   Confirm sample rate.
3. **[A3] Plan encoding** — JSON only for phase one; additive-compatible within a major
   `schema_version`; consumers ignore unknown fields.
4. **[A4] Prose-without-intelligence policy** — verbatim under a length threshold, else
   refuse-and-surface. Confirm the threshold (default ~120 words / ~45 s).
5. **[A5] Segmentation source** — phase one uses a CommonMark-aware segmenter (e.g.
   `goldmark` AST) for clean seams: headings, fenced code, tables, lists, paragraphs;
   plaintext falls back to blank-line paragraphs. Confirm the markdown-first assumption.
6. **[A6] Level semantics per class** — the table in §4. Confirm it matches your mental
   model, especially "deterministic L1 for structured classes."
7. **[A7] Oversized-block splitting** — split only on clean seams per class (functions in
   code, top-level keys in YAML, rows in tables). Need thresholds for "oversized"
   (working default ~40 lines or ~1500 chars).
8. **[A8] Symbol/identifier/path lexicon** — ship a small default spoken-form lexicon
   (`->` "arrow", `==` "is equal to", `/etc/hosts` "etc hosts", common dev acronyms) plus
   a user-overridable map. Confirm scope; the starter lexicon will need your eye.
9. **[A9] Persistent-sink storage** (deferred per brief) — proposed: a directory per
   recording with `audio.wav` + `plan.json` + `manifest.json` storing `content_hash` and
   last-known source path; on hash/path mismatch, mark **stale** (don't auto-regenerate).
   Rename/move behavior still open.
10. **[A10] Intelligence result caching** — cache by `(block content hash, level, model)`
    so re-render / escalation doesn't re-bill. Assumed on.
11. **[A11] Error vs refusal boundary** — adapter I/O failure = error (stops); readable-
    but-unvoiceable = refusal (spoken + surfaced). Confirm (§2.5).
12. **[A12] Refusal audibility** — refusals are spoken by default (`spoken=true`), with a
    flag to emit data-only for GUI-only consumers. Confirm default.
13. **[A13] Classification method** — purely deterministic/heuristic for phase one (fenced
    blocks, YAML/JSON/TOML sniff, table pipes, Mermaid fences). No ML in the core. Confirm.
14. **[A14] Renderer segment sizing** — the planner pre-splits into sentence-ish segments
    sized to the engine's max input. Confirm the planner owns chunking (keeps renderer
    dumb).
15. **[A15] Voice selection** — single configured default voice for phase one; `voice` is
    a hint in `PlanDefaults`. Multi-voice deferred.
16. **[A16] MCP `speak` tool shape** (phase 4 preview) — args `{text|source, level, sink}`,
    summarization via MCP sampling. Flagged now, designed in phase 4.
17. **[A17] Concurrency** — block-parallel rendering is allowed later for long docs;
    phase-one slice renders sequentially. Note only.
18. **[A18] Secret-readout awareness** (deferred) — one line: local-only means a secret in
    a config block could be spoken on your machine; not a design driver in phase one.

---

## 9. What this sets up (not built now)

The vertical slice (step 2) instantiates exactly one path through these interfaces:
`adapter/file → planner (real classify + symbol voicing + per-block leveling) →
render/sherpa (one engine) → sink/ephemeral`, emitting block-level timing, demonstrating
the honesty rule (a refused block) and graceful degradation (nil intelligence adapter).
Nothing above forces an engine or GUI choice before you make the two decisions in §7.

**Checkpoint:** review this design — especially the §2 schema, the §4/§5/§6 flows, and the
two decisions in §7 — before I touch the vertical slice.
