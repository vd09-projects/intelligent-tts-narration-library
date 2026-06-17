# Problem Statement — Intelligent TTS Narration Library

## The problem

I process text faster by ear than by eye. Long LLM responses and dense files
cost me disproportionate time and energy to read, and skimming a wall of text to
find the part I actually need is slow. Listening is faster, and listening *while*
skimming is faster still — but the tools that read text aloud are dumb screen
readers. They pronounce symbols literally, read code character by character, and
have no idea that a YAML block or a chart should be *explained* rather than
recited.

I want a TTS layer that understands the *shape* of what it is voicing and renders
each part in the way that is actually useful to hear — so I can listen to a long
explanation or document first, get the gist, and only drop into reading the source
when I sense I have missed something.

This is explicitly a hobby project. It must not require recurring dedicated
spending.

## Who and when

A single primary user (me), a developer working mostly in a terminal, frequently
interacting with LLMs (Claude Code, Claude Desktop, Codex, and similar) and
reading markdown-ish documents — approach write-ups, explanations, configs,
examples — that are too long to want to read in full up front.

## What "useful to hear" means

The core insight is that good narration is not transcription. Prose can be read
fairly faithfully, but structured content (code, config, tables, diagrams,
examples) must be voiced by *meaning*, not by character. Reading `replicas: 3`
as "replicas colon three" with indentation announced is useless; "replicas set to
three" is what a listener needs.

Crucially, *how deep* that explanation goes must be on demand and per block, not a
single setting applied once to a whole document:

- **Level 1 — Gist (default).** A high-level pass. Enough to know what a block is
  about so I can decide whether I need more.
- **Level 2 — Summary.** A decent summary with some context per block.
- **Level 3 — Detail.** Explain the block thoroughly.

My real workflow is: listen at gist level, pause when a block sounds important,
glance at the source or escalate *that one block* to level 2 or 3, then resume.
So leveling is per block and re-requestable, not global.

## Architecture as agreed (the backbone of the solution)

This is included because the framing turns on it. The system is deliberately small
at the center with heavy or specialized concerns pushed to pluggable edges.

```
Input adapters  →  raw text + source map  →  Planner  →  Narration plan  →  Renderer  →  Output sink
(file / MCP /                              (structure,                    (pure local
 screenshot-OCR)                            classify, level,               TTS; audio +
                                            symbol voicing,                block timing)
                                            honest refusal)
```

**Input adapters** turn some source into *text plus a source map* (a "where did
this block come from" reference — line ranges for files, pixel regions for
screenshots). The core never sees a file, a socket, or pixels directly.

**Planner** (the intelligence-light core) segments input into blocks, classifies
each (prose / code / config / table / diagram-as-text / example), applies the
requested level, and decides how each block should be voiced — including symbol,
identifier, and path pronunciation. It splits an oversized block into sub-blocks
*only* on clean, natural structural seams, never by forcing arbitrary cuts.

**Narration plan** is the load-bearing artifact every part of the system produces
or consumes: leveled, classified segments with voicing directives and
back-references to source positions. One plan format serves every input and every
output.

**Renderer** is a pure, local, near-human TTS engine with no semantics. It turns a
narration plan into audio plus block-level timing data. Dumb and fast.

**Output sinks** are either *ephemeral* (speak it and forget — the "read me this
answer" case, no UI) or *persistent* (save the recording with its sync data and a
link back to the source, for things worth revisiting).

**Interfaces** — an MCP server (so any LLM client can call a `speak` tool) and a
CLI — are just consumers of this pipeline. MCP is a first-class requirement: the
capability to link with Claude Code / Desktop / Codex must exist, even though the
specific wiring is decided later.

## Intelligence model (summarization, image description)

Semantic work — summarizing prose to a gist, describing an undescribed image — is
*comprehension*, not rendering, and lives outside the core as a **pluggable,
caller-supplied, model-agnostic** adapter. The contract is narrow: "here is a
block and the level I want; return voiceable text."

The cost asymmetry is deliberate and worth remembering:

- **TTS is free and local**, because good free local TTS exists and a dedicated
  recurring TTS subscription for a hobby toy is not acceptable.
- **Intelligence rides on an LLM key I already own.** I always have at least one
  membership, so requiring *some* intelligence endpoint does not violate the cost
  constraint — that constraint was always about *new recurring dedicated* spend.

Cost is controlled by matching model to difficulty: cheap models (Haiku/Sonnet
tier) for basic summarization and graph description; the caller chooses the model
because the caller owns the budget and keys. Over MCP, the summarizer simply *is*
the client LLM already in the conversation, at zero extra cost. With no
intelligence plugged in, the planner degrades gracefully to faithful structural
voicing (no prose-gist, but structured blocks are still voiced intelligently).

The leveling system is therefore both the comprehension control and the cost
control — the same mechanism that protects comprehension protects the token bill.

## Honesty rule (non-negotiable)

Because the whole point is that I am *not* reading, the system must never
fabricate. If input is too large or too raw to voice faithfully at the requested
level, or a block is a bare image with no description available, the planner
**refuses and surfaces it** ("there is a graph here that isn't voiced; check the
source") rather than inventing a confident summary I would never know was wrong.

## Sync

The library *emits* block-level sync data (this spoken segment maps to source
block N, lines/region X). It does not render UI; a CLI, an MCP client, or a future
GUI consumes the sync data and displays it however it wants. Sync is **block-level,
not word-level karaoke** — word-level only works for verbatim narration, which
contradicts gist mode. Block-level is exactly enough for the real need: when I
sense a miss, jump to the right region of the source. It also gives seek-by-block
("jump to the config section") for free.

## Non-goals (phase one)

- **No streaming / real-time narration.** Good narration needs the whole answer,
  because the planner cannot decide how to voice a block until it sees the block's
  full shape. The real-time speech-to-text → intelligence → text-to-speech
  companion (e.g. a Whisper-driven conversational assistant) is a *different
  product* with its own hard problems and is explicitly out of scope.
- **No comprehension inside the core.** Digesting a 2000-line source file, or
  interpreting a chart, is the caller's job via the pluggable intelligence adapter.
  The library voices descriptions it is given; it does not author them.
- **No image/graph interpretation in the library.** Diagrams expressed *as text*
  (Mermaid, ASCII, chart-as-YAML) are the planner's job. Bare images require an
  upstream vision adapter; undescribed visual blocks are flagged, not invented.
  (Revisit after phase one.)
- **No follow-up Q&A.** This system speaks; it does not answer questions about what
  it spoke. Interactive Q&A belongs to a separate STT→intelligence→TTS system.
- **No OCR or vision in the core.** Both are edge input adapters with their own
  heavy, platform-specific dependencies; most invocations never need them.

## Deferred to the solution phase (real, but not framing)

- Specific local TTS engine selection (local, near-human, free or one-time cost —
  known to be achievable today).
- Recording storage format, and behavior when a linked source file is
  renamed/moved.
- Multi-column / scattered-layout reading order for OCR'd screenshots — phase one
  assumes mostly-linear single-flow text.
- English only for phase one; multilingual deferred.
- Local-only means a secret could be read aloud on my own machine — worth one line
  of awareness, not a design driver.

## Stale recordings

Out-of-date is acceptable and is the chosen behavior. Recordings are only
(re)generated on explicit request; the system does not re-render on every source
change. Escalating a block to a deeper level is just an explicit regenerate of that
segment, which fits this rule cleanly.
