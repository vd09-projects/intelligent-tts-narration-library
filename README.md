# Intelligent TTS Narration Library

Go library for shape-aware TTS narration. Voices text by *meaning*, not character — per-block leveling (gist / summary / detail), pluggable edges (input adapter, intelligence adapter, renderer, output sink), honest refusal over fabrication.

See `problem-statement.md` for framing and `docs/solution-phase-design.md` for module layout and the narration-plan schema.

## Quickstart

Prerequisites:

- Go 1.25+. The MCP SDK (`github.com/modelcontextprotocol/go-sdk` v1.5.0) requires it; this bumped the project's minimum Go from 1.22 to 1.25.0. See the "Go toolchain" note below.
- macOS (phase one uses `afplay` for the ephemeral sink).
- A working `scripts/kokoro` wrapper — see `render/sherpa/README.md` for the Kokoro-onnx setup (Python venv + ONNX model files).

Run the vertical-slice CLI against the canonical sample document:

```sh
go run ./cmd/narrate --file docs/samples/sample.md
```

Expected: speakers play the document one block at a time, including an honest spoken refusal for the bare-image block. Exit code `0` on success.

The ephemeral sink cleans up its temp WAV directory at the end of the run. The path is printed to stdout while the run completes; the dir is gone by the time the process exits. Persistent sink (phase 2) will own its own retention policy.

## CLI flags

| Flag | Default | Choices | Meaning |
|---|---|---|---|
| `--file` | — (required) | path | Markdown document to narrate. |
| `--level` | `1` | `1` / `2` / `3` | Per-block leveling target: 1 = gist, 2 = summary, 3 = detail. With `--block`, this is the absolute target level for that one block — downgrade L3→L1 supported. |
| `--sink` | `ephemeral` | `ephemeral` / `persistent` | Output sink. `persistent` writes `audio.wav`, `plan.json`, and `manifest.json` to the `--out` directory (required with `--sink=persistent`). |
| `--out` | empty | path | Destination directory for `--sink=persistent` — receives `audio.wav`, `plan.json`, and `manifest.json`. Required with `--sink=persistent`; rejected with `--sink=ephemeral` (which owns its own temp-dir lifecycle). |
| `--gender` | `female` | `female` / `male` | Voice gender. `female` → `af_bella`, `male` → `am_michael`. Ignored when `--voice` is set (a one-line stderr notice says so). |
| `--voice` | empty | `cool-jahns` / `confident-neal` | RVC character voice (see "RVC character voices" below). Empty = plain Kokoro. Requires the RVC worker; an unknown slug or a missing worker exits non-zero (no silent fallback). Rejected with `--listen`. |
| `--block` | empty | block id | Re-render a single block by id (from the roster printed at the end of every whole-doc run). Empty preserves whole-document narration. |
| `--expected-content-hash` | empty | hex string | Only meaningful with `--block`. If the document's `content_hash` has changed since you obtained the id, a warning prints to stderr; the re-render still runs (exit `0`). |

Exit codes: `0` success (including refused blocks and hash-mismatch warnings); `1` adapter / planner / renderer / sink error; `2` flag error (e.g. `--sink=persistent` without `--out`), unknown `--block` id, or a persistent `--block` patch refusal.

## RVC character voices

The default voice is plain Kokoro (24 kHz `af_bella` / `am_michael`). Opting into a **character voice** repaints each Kokoro block through the Apache-2.0 ONNX RVC decorator (`render/rvc`, issue #145) into **40 kHz mono** audio. Two voices are wired in phase one: `cool-jahns` (male source) and `confident-neal` (female source). The same knob is exposed on all three composition roots — routed through one shared `pipeline.BuildRenderer` factory:

| Root | Surface | Scope |
|---|---|---|
| `cmd/narrate` (CLI) | `--voice <slug>` | Per invocation. |
| `cmd/narrate-mcp` (MCP) | `voice` arg on `speak` / `speak_last` / `speak_to_file` | Per tool call. |
| `cmd/narrate-server` (HTTP) | `--voice <slug>` **launch flag** | One character voice per server process (per-request voice is a deferred follow-up). |

Contract:

- **Worker required.** A character voice needs the torch-free RVC worker on disk: `make rvc-worker-venv` then `make rvc-export VOICE=<slug>`. Without it the render **stops** — no silent fallback to Kokoro.
- **Honesty rule.** An unknown slug or an unavailable worker is a hard **error**, not a refusal: the CLI/server exit non-zero and the MCP tool returns a `caller-error`. Errors stop the pipeline; refusals are spoken. An unknown slug is caught up front (before any render work).
- **`--voice` overrides `--gender`.** The decorator picks the Kokoro source voice its model was trained against, so `--gender` has no effect; the CLI and server print a one-line stderr notice when both are set. The MCP tools stay silent (structured receipt).
- **40 kHz end-to-end.** The persistent sinks validate the container at 40 kHz when a voice is set; the plain path stays byte-identical at 24 kHz.
- **`manifest.voice` records the character slug** (Decision D6). A persistent RVC render stamps `manifest.json` `"voice"` with the character voice you asked to hear (`cool-jahns` / `confident-neal`) — the honest provenance — not its hidden Kokoro source. Staleness keys only on `content_hash`, so this never affects caching or stale detection.
- **Listen mode** (`--listen`) rejects `--voice` for now — the interactive transport plays through a fixed 24 kHz context; a 40 kHz character voice there is a deferred follow-up.

By-ear check (the exhaustive audio /verify is issue #147): `make rvc-sanity` renders the sample doc at both voices to a temp dir (40 kHz `audio.wav` + a `manifest.json` whose `"voice"` is the slug) for inspection.

The plan stays engine-neutral: no RVC slug appears in `plan.json`, and `planner/` / `plan/` gain no dependency on the decorator.

## Escalate one block

Real workflow: listen at gist (`--level=1`), spot a block you want more detail on, re-run that block at `--level=2` or `--level=3` without re-narrating the whole document. The renderer patches just that block's audio + timing — every other block is untouched.

After every ephemeral whole-doc run, `narrate` prints a tab-separated block roster to stderr so you can grab an id, and the document's `content_hash` is appended to the stdout summary so you can capture it for a later staleness guard:

```sh
go run ./cmd/narrate --file docs/samples/sample.md
# stdout (last line):
#   blocks_played=4 total_duration_ms=9215 out_dir=/tmp/narrate-XXXXXX content_hash=8a3f…
# stderr (after audio finishes):
#   # 4 blocks — escalate one with: narrate --file docs/samples/sample.md --block <id> --level {2|3}
#   b001	heading	1	voiced	1
#   b002	prose	1	degraded	3-9
#   b003	code	1	voiced	11-15
#   b004	unknown	1	refused	17
```

Roster columns: `id`, `class`, `level`, `status`, `lines`. Status `refused` blocks are still voiced — they speak a short honest notice (refusal-is-data per `CLAUDE.md`).

Escalate one of them:

```sh
go run ./cmd/narrate --file docs/samples/sample.md --block b002 --level 3
```

The roster is suppressed on `--block` runs (you already know which block you're targeting). Downgrade is symmetric — `--level=1` is allowed on a block that voiced at L3.

### What each level does, per class

Escalation changes the voiced text only for some classes. A `list` or `heading` voices **identically at L1, L2, and L3** — L1 already speaks the full content, so escalating it is a no-op on the text (only the level label changes). The classes below that *do* grow with the level are where escalation earns its keep:

| Class | L1 | L2 | L3 |
|-------|----|----|----|
| `heading` | `Section: {text}` | *same as L1* | *same as L1* |
| `list` | all items, with ordinals | *same as L1* | *same as L1* |
| `config` | `N top-level keys` | enumerate keys | read every key = value |
| `code` | `N-line {lang} block` | one-line semantic gist¹ | line-by-line meaning¹ |
| `table` | `C-column, R-row table` | meaning summary¹ | per-row reading¹ |
| `prose` | comprehension¹ | comprehension¹ | comprehension¹ |

¹ Needs an `IntelligenceAdapter`. Without one: structured classes (`code`, `table`) fall back to a deterministic reading; `prose` ≤ ~120 words is read verbatim (`degraded`), longer prose is `refused` (honesty rule — never an invented gist).

So if you escalate a `list` and the text doesn't change, that's expected — try a `config`, `code`, or `table` block to see the level actually do work.

Want to be sure the document hasn't changed under you between the roster and the re-render? Capture the `content_hash` from the stdout summary above and pass it back:

```sh
# In a shell pipeline, grab the hash with awk / sed / cut as a key=value parse:
HASH=$(go run ./cmd/narrate --file docs/samples/sample.md \
       | tail -1 | tr ' ' '\n' | grep '^content_hash=' | cut -d= -f2)

go run ./cmd/narrate --file docs/samples/sample.md \
        --block b002 --level 3 \
        --expected-content-hash "$HASH"
```

If the document's hash differs, you'll see a stderr warning — `warning: content_hash mismatch (expected …, got …) — block content has changed since you got that id` — and the re-render still runs (exit `0`). An unknown `--block` id exits `2` with `block not found: <id>`. Passing `--expected-content-hash` without `--block` is a flag error (exit `2`) — without `--block` the pipeline does not check the hash and the guard would be silently ineffective.

Per-block re-render works against both sinks. With `--sink=persistent` (issue #16), `--block --sink=persistent` patches just the targeted block's WAV into the existing `--out` directory (issue #28, `sink/persistent.PatchBlock`) — every other block is byte-preserved and `manifest.json` stays consistent. A missing directory, absent manifest, stale content, cross-document hash, or container mismatch is refused at runtime (exit `2`).

## MCP server (`cmd/narrate-mcp`)

`cmd/narrate-mcp` is a sibling composition root that exposes the same pipeline over the Model Context Protocol's stdio transport. MCP clients (Claude Desktop, Claude Code, the `mcp` CLI) can call a single `speak` tool to narrate a markdown document.

Tool family: `narrate.*` — `speak` is the canonical entry point. Future tools belong under the same server.

### Listen to a Claude Code response (whole-response buffering convention)

To *listen* to a large assistant response instead of reading it, the MCP host must **buffer the entire response and hand it to `speak` as one `text` input**. The planner needs the whole document to decide voicing — there is no streaming (a phase-one non-goal). Partial, streamed, or chunk-per-call invocations are **out of contract**: each call is planned as an independent fragment, not one continuous narration. "As it arrives" is reconciled by buffering to completion, then calling `speak` once.

In listen-mode, **code blocks are voiced at a Level-2 floor** — a structural gist (count + declarations, or a one-line semantic gist when an intelligence adapter is wired) rather than the bare L1 line count. It is a *floor*, not a hard set: a code block still rises to L3 if you request `level=3`. This floor is composition-root policy on `cmd/narrate-mcp` (a planner-read declarative field via `pipeline.PipelineDefaults`); it is not a `speak` argument, and it does not change the document-wide `level` arg's meaning. Prose and other classes are voiced at the requested `level` unchanged.

Start the server (mostly useful for the `mcp` CLI or local development):

```sh
make run-mcp
```

The server logs to stderr and runs until stdin EOF or Ctrl-C (both clean shutdowns, exit `0`).

### `speak` tool arguments

| Field | Required | Default | Choices | Meaning |
|---|---|---|---|---|
| `source` | one of source/text | — | path | File path to the markdown document. |
| `text` | one of source/text | — | string | Inline markdown text. Routed through the in-memory `adapter/mcptext` (ticket #17); the composition root assembles the URI as `mcp://inline/<sha256-hex-of-text>` and the adapter cross-checks the hash on read. |
| `level` | no | `1` | `1` / `2` / `3` | Document-wide per-block leveling target: 1 = gist, 2 = summary, 3 = detail. Code blocks observe a Level-2 floor in listen-mode (see above); raising `level` to `3` lifts code (and everything else) to L3. |
| `sink` | no | `ephemeral` | `ephemeral` / `persistent` | Output sink. `persistent` returns a tool error in phase one. |
| `gender` | no | `female` | `female` / `male` | Voice gender. `female` → `af_bella`, `male` → `am_michael`. Moot when `voice` is set. |
| `voice` | no | empty | `cool-jahns` / `confident-neal` | RVC character voice (see the "RVC character voices" section). Empty = plain Kokoro. An unknown slug is a `caller-error: invalid_argument`. Also accepted by `speak_last` and `speak_to_file`. Additive-compat: older clients that omit it are unaffected. |

### Tool response

```json
{
  "receipt": {
    "blocks_played": 7,
    "total_duration_ms": 12345,
    "out_dir": "/tmp/narrate-mcp-XXXXXX"
  }
}
```

`out_dir` is the renderer's per-call temp directory and is deleted after the call returns. A `plan` envelope may be added additively in future releases under the schema-versioning rule.

### Tool errors

The `speak` handler returns errors via `CallToolResult.IsError = true`. The error text uses one of these prefixes so callers can self-correct:

- `caller-error: invalid_argument: …` — bad request (missing/conflicting args, unknown enum, file not found, permission denied, `sink=persistent`).
- `internal_error: pipeline failure: …` — renderer/sink failure unrelated to the request shape.
- `cancelled: …` — context cancelled by the client.

Refusals (e.g. bare images, oversized prose without an intelligence adapter) are not errors. The block is voiced with a spoken notice per the honesty rule, and the call still returns a normal receipt.

### Claude Desktop config (canonical)

Add to `~/Library/Application Support/Claude/claude_desktop_config.json`:

```json
{
  "mcpServers": {
    "narrate": {
      "command": "/absolute/path/to/intelligent-tts-narration-library/bin/narrate-mcp",
      "args": []
    }
  }
}
```

Build the binary first (the `bin/` path matches the snippet above; `make build-mcp` only compile-checks — emit the binary explicitly):

```sh
mkdir -p bin && go build -o bin/narrate-mcp ./cmd/narrate-mcp
```

Restart Claude Desktop. The `narrate.speak` tool will appear in the model's tool list.

For local development, you can also point `command` at `go` and `args` at `["run", "./cmd/narrate-mcp"]` from the repo root — startup is slower but no build step.

### `mcp` CLI (secondary, for power-user smoke)

If you have the [`mcp` CLI](https://github.com/modelcontextprotocol) installed:

```sh
mcp tool call --server "go run ./cmd/narrate-mcp" speak \
  --arg source=docs/samples/sample.md --arg level=1 --arg gender=female
```

(The exact `mcp` CLI invocation may vary with the CLI's release version; see upstream docs.)

### Manual smoke

```sh
make test-mcp-manual
```

This runs `runSpeak` against `docs/samples/sample.md` in-process (bypassing the stdio transport), plays audio via afplay, and asserts the receipt shape. Listener confirms the bare-image refusal by ear, same as `make test-manual`.

### Live observer (Channel 2)

The `transcript` in the tool response is the *after-the-fact* per-block record (ADR #77 Channel 1) — it is only assembled once the last block has finished playing, because `speak` blocks on afplay per block. On very large documents the `transcript` array is capped at 200 entries (`transcriptMaxEntries`, issue #86), head-keep / tail-truncate: when the document exceeds the cap the response carries `transcript_truncated=true` and `transcript_omitted_count=<n>` (how many trailing entries were dropped). The under-cap case (≤200 blocks) is byte-identical to before — both signal fields are `omitempty` and stay absent. The dropped tail blocks were still spoken and are still counted in `receipt.blocks_played`, so a truncated transcript is **not** an exhaustive refusal ledger. To watch progress **while audio is still playing**, launch the decoupled, read-only `cmd/narrate-observe` binary in a **second terminal** (ADR #77 Channel 2). The `speak` handler appends one JSONL line per block to an ephemeral scratch file before each blocking play; the observer tails it and renders `[3/9] L2 voiced 4.2s > b3` lines live.

Opt in on the **writer** (the `speak` server) via environment, in precedence order:

| Variable | Effect |
| --- | --- |
| `NARRATE_OBSERVE_FILE=/path/x.jsonl` | Emit to this exact path. Highest precedence. |
| `NARRATE_OBSERVE=1` (`true`/`yes`/`on`) | Emit to an auto-created `/tmp/narrate-observe-*.jsonl` temp file. |
| *(unset, or `0`/`false`)* | Off — the speak response is byte-for-byte unchanged. |

The **observer** (reader) discovers its target by the same precedence: `-f <path>` flag > `NARRATE_OBSERVE_FILE` > newest matching `/tmp/narrate-observe-*.jsonl`. It runs until **Ctrl-C** — it keeps tailing for the next `speak` run and does not self-exit when one ends.

Two-terminal manual flow:

```sh
# Terminal 1 — start tailing (defaults to -f /tmp/narrate-observe-manual.jsonl)
make run-observe

# Terminal 2 — speak the sample doc, emitting live to the same file
make run-observe-manual
```

Design notes:

- **Decoupled by construction.** Enabling the observer only writes the side file; the `speak` response (receipt + transcript, both channels) stays `bytes.Equal` to the observer-off baseline — byte-identical only when the transcript is *not* truncated (≤200 blocks, the common case; see the transcript cap above). On a truncated response the two `omitempty` signal fields appear, but that change is driven by the document size, not by the observer. A scratch open/write failure prints **one** line to STDERR and goes silent — it never errors the `speak` call (observability must not break playback).
- **No secrets on the wire.** Each JSONL line carries only structural metadata — `block_id`, `order`/`total`, `level`, `status`, `planned_duration_ms`, `playing` — never source or spoken text (CLAUDE.md "local-only means secrets get read aloud"). The scratch file is created `0600` (owner-only).
- **Ephemeral.** The scratch file lives under `/tmp` (deliberately, not `$TMPDIR`, so the observer's newest-file glob works), is left for the OS to reap, and is never under the renderer's `out_dir` or teed into a durable sink.

### Known limitations

- `sink=persistent` is not implemented (phase two).
- No intelligence adapter wired in this release — the planner uses the deterministic + degraded path; prose under ~120 words is read verbatim, larger prose is refused honestly. With the listen-mode Level-2 code floor and no adapter, code blocks are voiced at a deterministic count + declarations gist with `status=degraded` (honest, not refused) — quieter than an AI gist but never fabricated.
- Local-only means secrets get read aloud — and with listen-mode this now extends to secrets embedded in LLM-output code blocks voiced at the L2 floor. Deliberate phase-one deferral, awareness only, not a design driver.

## Running the tests

Default test pass — no audio, no subprocess to Kokoro / afplay:

```sh
go test ./...
```

Benchmarks — planner-only and end-to-end with stub edges:

```sh
go test -bench=BenchmarkNarrate -benchmem ./pipeline/...
```

Manual end-to-end smoke (real binary, real audio, listener confirms refusal by ear):

```sh
go test -tags manual ./pipeline/...
```

The manual smoke must run from the repo root so `./scripts/kokoro` resolves.

MCP server manual smoke (same audio path as above, in-process through the `speak` handler):

```sh
make test-mcp-manual
```

## Go toolchain

The MCP SDK pulls the minimum Go version to `1.25.0` (transitive requirement from `github.com/modelcontextprotocol/go-sdk` v1.5.0). This is intentional — the SDK is a first-class dependency in CLAUDE.md's stack list, and pinning the SDK lower than 1.5.0 would forfeit the speak-tool wiring. Consumers on Go 1.22–1.24 cannot build the library until they upgrade their toolchain.

## Architecture, briefly

`pipeline.Pipeline` is the composition root — the only struct that holds concrete edge instances. It wires four edges around the intelligence-light `planner/` core: `adapter/` for input, `intelligence/` for optional comprehension, `render/` for audio, `sink/` for delivery. Plans flow through the pipeline as a single `plan.NarrationPlan` JSON contract; the renderer attaches a `plan.Timeline` keyed by `block_id` (block-level sync only — no word timings).

Phase one runs with no intelligence adapter wired. The planner takes the deterministic + degraded path: structured classes (code, config, table, heading, list) voice at every level; prose under ~120 words is read verbatim; prose over that limit and bare images are refused honestly with a spoken notice. Refusal is data, not error.

## Skills

This project is developed with a set of Claude Code skills:

- `rune` — project onboarding.
- `task-manager` — backlog + status, GitHub Issues backend.
- `mimir` — planning (architecture or task breakdown).
- `sindri` — implementation (plan / build / iterate).
- `skald` — handoff persistence for mimir / sindri / multi-perspective-review output.
- `multi-perspective-review` — multi-lens review.
- `decision-journal` — record load-bearing decisions.
- `conventional-commits` — commit message discipline.

Skill artifacts live under `.claude/handoff/{scope}/` (audit trail) and `decisions/` (journal).
