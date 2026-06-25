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
| `--sink` | `ephemeral` | `ephemeral` / `persistent` | Output sink. `persistent` is not implemented in this slice and exits non-zero. |
| `--gender` | `female` | `female` / `male` | Voice gender. `female` → `af_bella`, `male` → `am_michael`. |
| `--block` | empty | block id | Re-render a single block by id (from the roster printed at the end of every whole-doc run). Empty preserves whole-document narration. |
| `--expected-content-hash` | empty | hex string | Only meaningful with `--block`. If the document's `content_hash` has changed since you obtained the id, a warning prints to stderr; the re-render still runs (exit `0`). |

Exit codes: `0` success (including refused blocks and hash-mismatch warnings); `1` adapter / planner / renderer / sink error; `2` flag error, `--sink=persistent`, or unknown `--block` id.

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

Phase-one caveats: per-block re-render works against the ephemeral sink only. The persistent sink (issue #16) will keep `manifest.json` consistent and rewrite just the patched block's WAV in place — until then, `--block --sink=persistent` returns the same `errPersistentNotImplemented` fast-error as any other persistent-sink call.

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
| `gender` | no | `female` | `female` / `male` | Voice gender. `female` → `af_bella`, `male` → `am_michael`. |

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

The `transcript` in the tool response is the *after-the-fact* per-block record (ADR #77 Channel 1) — it is only assembled once the last block has finished playing, because `speak` blocks on afplay per block. To watch progress **while audio is still playing**, launch the decoupled, read-only `cmd/narrate-observe` binary in a **second terminal** (ADR #77 Channel 2). The `speak` handler appends one JSONL line per block to an ephemeral scratch file before each blocking play; the observer tails it and renders `[3/9] L2 voiced 4.2s > b3` lines live.

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

- **Decoupled by construction.** Enabling the observer only writes the side file; the `speak` response (receipt + transcript, both channels) stays `bytes.Equal` to the observer-off baseline. A scratch open/write failure prints **one** line to STDERR and goes silent — it never errors the `speak` call (observability must not break playback).
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
