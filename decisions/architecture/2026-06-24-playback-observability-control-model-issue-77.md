# ADR: Playback observability & control model (issue #77)

| Field    | Value            |
|----------|------------------|
| Date     | 2026-06-24       |
| Status   | accepted         |
| Category | architecture     |
| Tags     | observability, transport-control, receipt-envelope, transcript, oto, afplay, decoupled-observer, jsonl-tail, sse, mcp-tool-result, mcp-progress-notifications, block-level-sync, issue-77, issue-78, issue-79 |

> **Status:** accepted — re-researched + adversarially verified 2026-06-24.
> This version **supersedes the draft's single-channel-only D5**: the re-research FLIPS D5 to a
> complementary two-channel observability model. D1–D4 are AGREED + refined, no flip.

## Context

Issue #77 is the ADR analysis task for playback observability and transport control raised
after end-to-end testing of the #73 MCP "listen" path. Today the MCP `speak` path is
fire-and-forget: speak → ephemeral → afplay plays audio, returns a receipt-only envelope,
deletes temp dir. No on-screen transcript. No pause/resume/seek. `afplay` offers no
programmatic control.

Five accepted decisions from the #72/#73 ADR session constrain this analysis (all unchanged
and all **CONFIRMED** by the re-research):
- `terminal-listen-not-read-is-ephemeral-afplay-audio-only` — audio-only terminal path ships.
- `react-player-optional-reuses-existing-player-50` — React player is optional, not rebuilt.
- `player-playback-unit-stays-whole-audio-wav` — playback unit is the whole audio.wav blob.
- `player-raf-audio-sync-transition-only-rerender` — React player uses rAF, not timeupdate.
- `primary-listen-path-decoupled-from-durable-sink` — standing guardrail: no sink-lifetime coupling.

**This version is a re-research** that ran 9 load-bearing claims through blind adversarial
claim-verifier workers. Eight verified, one downgraded to single-source (the D4 wording). The
verification **REFUTED a sub-claim the draft's D3/D5 rested on** (the "inline, untruncated, no
collapsible panel" CC-render claim) and surfaced a **scope gap in the draft's D5** (it never
evaluated a user-launched second-terminal observer) — together producing a **FLIP on D5**.

## Decisions

### D1 — Playback surface: afplay phase one; oto v3.4.0 phase two. CONFIRMED + refined.

`oto/v3` v3.4.0 is Apache-2.0, no-CGo on macOS (purego), and `*Player` exposes
`Pause()`/`Play()`/`Seek()`. **REFINEMENT (verifier-flagged):** the full Seek signature is
`func (p *Player) Seek(offset int64, whence int) (int64, error)` and it returns an error unless
the underlying source implements `io.Seeker` — so the player's source reader **MUST be an
`io.Seeker`** for block-seek to work.

The existing `speak → ephemeral → afplay` path stays as-is for phase one. `afplay` is replaced
with `oto` v3.4.0 when transport control (pause/resume/seek) is implemented — a #78 concern,
not this ADR. SIGSTOP/SIGCONT over the existing afplay subprocess remains a zero-dependency
interim for pause/resume only (no seek), with an empirical glitch-risk caveat.

Evidence grade: **VERIFIED** (oto v3.4.0 LICENSE + player.go at the v3.4.0 tag; pkg.go.dev).
**Confirms** `terminal-listen-not-read-is-ephemeral-afplay-audio-only` — no flip.

### D2 — Seek/back = previous-block (block-granular). CONFIRMED + flag.

"Go back" = previous block. No word-level timing. Block-granular transport is well-precedented
(podcast chapter navigation, ID3v2 CHAP, Matroska chapters). A "back N seconds" time-skip is the
orthogonal mode (separate control), out of scope for phase one.

WAV seek byte offset = `dataChunkStart + floor(StartMs/1000 × 24000) × 2`. **FLAG
(verifier-flagged):** the literal `× 2` is **BlockAlign** and is valid ONLY for mono-16-bit PCM;
general code MUST use the file's actual BlockAlign (`NumChannels × BitsPerSample/8`).
`dataChunkStart` is **NOT reliably 44** — must parse RIFF chunk headers (a `LIST`/`fact`/`cue`
chunk can precede `data`).

Evidence grade: **VERIFIED** (wavefilegem + soundfile.sapp.org WAVE refs; confirmed by a
sandboxed probe that shifted `data` to offset 70 with a `LIST` chunk and confirmed the
time→byte formula lands on the exact sample frame). **Confirms** the block-level-only sync
invariant — no flip.

### D3 — Transcript placement: structuredContent + duplicate TextContent. CONFIRMED + CORRECTION.

The transcript (per-block `spoken_text` + `level` + `status` + `duration_ms`) goes in
`structuredContent`; a serialized-JSON copy rides in **one** `TextContent` block.

**CORRECTION to the draft:** the draft claimed the CC CLI renders tool results "inline,
untruncated, no collapsible panel" — that is **REFUTED**. The CC CLI **COLLAPSES** MCP tool
results to a single line by default and the user expands with **ctrl+o** (official
interactive-mode docs). So the inline `TextContent` is collapsed-by-default, not
always-visible — it is the model's after-the-fact record, **not a live display**.

Also confirmed: in current CC (~v2.1.x) text content blocks are **DROPPED** from model context
when `structuredContent` is present (authoritative agent-SDK doc; #55677 at v2.1.126); the
inverse held at v1.0.60 (#4427). So the duplicate `TextContent` is for backward-compat / older
clients, and the `structuredContent` is what current CC forwards to the model. go-sdk v1.5.0
`ToolHandlerFor` auto-mirrors structured output into `Content` as JSON.

A compact per-block transcript shape:
```json
{
  "receipt": { "blocks_played": N, "total_duration_ms": M },
  "transcript": [
    { "block_id": "b1", "level": 1, "status": "voiced", "spoken_text": "...", "duration_ms": 4200 }
  ]
}
```

The persistent sink is NOT required for observability — transcript is carried in-band. This is
additive under the `schema_version` rule (new fields, no removals); no `plan/` schema change
(spoken_text already lives in `Block.Segment[].Text`).

Evidence grades: **VERIFIED** (CC collapse + ctrl+o; text-dropped-when-structured).
**Confirms** the receipt-only-envelope decision as still additive-compatible.

### D4 — Transport control vs no-streaming. CONFIRMED but DOWNGRADED wording.

Adding pause/resume/seek over a fully-rendered audio buffer does **NOT** make the batch pipeline
"streaming"; a whole-input pipeline can offer full transport control without incremental
generation.

Evidence grade: **SINGLE-SOURCE** (downgraded from the draft's confident "orthogonal / opposite").
The verifier flagged: GStreamer's `GstBaseSrc` shows a push/streaming source CAN also be
seekable, so seek is NOT the strict "opposite" of streaming, and consumer "streaming services"
colloquially include transport control. The **defensible one-directional claim stands**
(transport control over rendered audio ≠ streaming generation); the strong "orthogonal axes /
opposite of streaming" framing is softened. GStreamer does name push = "streaming mode" and
pull = random-access/seekable (VERIFIED span), which grounds the one-directional claim.

**Confirms** the phase-one no-streaming constraint — no flip, softer wording.

### D5 — Terminal observability surface. **FLIPPED** from the draft.

The draft concluded the **ONLY** observability surface is the MCP receipt's `TextContent` (no
live during-playback view), having rejected a TUI. The re-research **FLIPS this to a
complementary TWO-CHANNEL model**:

- **CHANNEL 1 — Inline MCP receipt** (`structuredContent` + duplicate `TextContent`, per D3).
  The model's after-the-fact record. **KEPT from the draft.**
- **CHANNEL 2 — Opt-in DECOUPLED OBSERVER** the **USER** launches in a second terminal / browser,
  reading an ephemeral side channel the `speak` server writes. This is the **ONLY** surface that
  shows **LIVE during-playback progress**.

**Why the flip is load-bearing and verifier-grounded:**

- **CRUX (VERIFIED):** MCP / go-sdk v1.5.0 tool calls are **SYNCHRONOUS** — the
  `CallToolResult` is delivered only after the handler returns. **CONFIRMED against THIS repo:**
  `sink/ephemeral/ephemeral.go` `Consume` plays every block serially and `playWithAfplay`
  blocks on `cmd.Wait()` per block, so the `speak` handler returns only AFTER all playback
  finishes. The receipt therefore **CANNOT carry live progress** — it arrives after playback
  ends. This is exactly the live-progress gap the draft never named.
- **CRUX (VERIFIED):** the MCP stdio reservation constrains only the **SERVER's STDOUT** for
  JSON-RPC. A side channel (unix socket / FIFO / file / HTTP) violates **NO** MCP rule — it is
  not the reserved stream.
- **SCOPE GAP (confirmed against the draft text):** the draft only ever evaluated a
  same-terminal TUI and a **server-spawned** osascript window — it NEVER evaluated a
  **user-launched** observer in a second terminal. Raw mode is per-tty (VERIFIED via termios
  docs + a pty probe: raw mode on one tty leaves a second tty fully canonical), so a
  user-launched 2nd-terminal observer has **NO raw-mode conflict** with Claude Code's tty. The
  draft's TUI rejection does not apply to this option.

**DEFAULT side channel:** an append-only JSONL the `speak` server writes + `tail -f` to read it
— zero deps, no raw-mode, ephemeral (dodges the durable-sink guardrail). A localhost SSE web
view is the richer opt-in (`http.Flusher` pattern). A unix socket works but carries the macOS
104-byte `sun_path` caveat (VERIFIED: macOS `sun_path`=104 vs Linux 108; use a short `/tmp`
path, not a long `$TMPDIR` path); a FIFO is the most fragile.

**NEW ALTERNATIVE surfaced in verification** (note, do not adopt as default): MCP has a NATIVE
`notifications/progress` channel (`Session.NotifyProgress`, VERIFIED in go-sdk v1.5.0) callable
from inside the blocking handler — a lighter-weight live-progress option than a decoupled
observer, but it surfaces progress to the MCP **CLIENT**, not a user-facing live view. Worth a
spike comparison, but the decoupled observer remains the recommended default for a true
user-visible during-playback surface.

The React player (`player/`) remains the optional visual companion — unchanged.

## Rejected options

Carrying over the draft's rejections, plus the new ones from re-research:

- **Single-channel receipt-only as the COMPLETE observability story**: REJECTED — cannot show
  live progress because the call is synchronous (the receipt arrives after playback ends).
- **Same-terminal TUI** (k9s-style, tview/tcell): REJECTED — MCP stdio reserves the server's
  stdout for JSON-RPC, and Claude Code holds the current tty in raw mode (Ink/React TUI).
- **Server-spawned osascript window**: REJECTED — macOS/iTerm2-specific, fire-and-forget detach,
  unverified for long-lived TUIs.
- **React player as primary observability**: REJECTED as primary — remains the optional visual
  companion per existing accepted decisions.
- **Persistent sink for transcript / live progress**: REJECTED — transcript is carry-able
  in-band in the receipt; the persistent sink is a separate `--sink persistent` invocation per
  the standing guardrail.
- **VLC or ffplay as player**: REJECTED — ffplay has no IPC/programmatic control surface
  (keyboard-only); VLC is a heavy dep with unconfirmed rc-module availability in VLC 4.x.
- **Streaming / real-time generation**: out of scope (phase-one non-goal, unaffected here).

> **NOTE:** these rejections do **NOT** extend to the **user-launched decoupled observer**,
> which is the accepted Channel 2. The TUI rejection rested on same-terminal raw-mode + MCP
> stdio conflicts that a user-launched second-terminal observer does not have.

## Consequences / open pins

- **#78 (blocked on this)**: implement the `transcript[]` additive receipt field and oto v3.4.0
  transport control. Planner already produces `Block.Segment[].Text`; the MCP handler collects
  these into the response.
- **#79 (references "#77 v3 Data Contract")**: the data contract = the D3 shape —
  `structuredContent: { receipt: {...}, transcript: [{block_id, level, status, spoken_text,
  duration_ms}] }` + one serialized-JSON `TextContent` block.
- **NEW: `cmd/narrate-observe` decoupled-observer spike (Channel 2)** — the default JSONL +
  `tail -f` surface, plus an optional localhost SSE observer (`http.Flusher`). Worth a spike
  comparison against the native `notifications/progress` alternative.
- `outputSchema` on the `speak` tool is additive — land it with #78 so clients validate
  `structuredContent` against it (servers MUST conform; clients SHOULD validate).
- Receipt must stay well under 10,000 tokens; for a typical 5–15 block response each block's
  `spoken_text` is a short gist, so this is easily satisfied.

## Evidence summary

9 load-bearing claims adversarially verified — **8 VERIFIED**, **1 SINGLE-SOURCE**:

| # | Claim | Grade |
|---|-------|-------|
| 1 | CRUX — MCP stdio reserves only the server's stdout; a side channel is legal | VERIFIED |
| 2 | CRUX — MCP/go-sdk v1.5.0 tool calls are synchronous (receipt after handler returns) | VERIFIED |
| 3 | oto v3.4.0 API (Pause/Play/Seek; Seek needs io.Seeker source) | VERIFIED |
| 4 | CC drops text content when structuredContent present (~v2.1.x; inverse at v1.0.60) | VERIFIED |
| 5 | CC CLI collapses tool results to one line; ctrl+o expands (**refutes the draft**) | VERIFIED |
| 6 | WAV RIFF-parse for dataChunkStart + time→byte formula (BlockAlign caveat) | VERIFIED |
| 7 | macOS `sun_path` = 104 vs Linux 108 | VERIFIED |
| 8 | Raw mode is per-tty (user-launched 2nd terminal has no raw-mode conflict) | VERIFIED |
| 9 | transport control ⊥ streaming | SINGLE-SOURCE (wording softened) |

**Final answer DISAGREES with the draft on D5** (flip to the two-channel model); **AGREES +
refines on D1–D4.**

## Sources

Draft sources (preserved):

- oto v3.4.0: https://pkg.go.dev/github.com/ebitengine/oto/v3
- gopxl/beep v2.1.1: https://github.com/gopxl/beep
- ID3v2 CHAP spec: https://id3.org/id3v2-chapters-1.0
- Matroska RFC 9559: https://datatracker.ietf.org/doc/rfc9559/
- Apple Podcasts iOS chapter nav: https://support.apple.com/guide/iphone/watch-and-listen-to-podcasts-iph3a22707a5/ios
- WAVE format (McGill MMSP): https://www.mmsp.ece.mcgill.ca/Documents/AudioFormats/WAVE/WAVE.html
- MCP spec 2025-06-18 — Tools: https://modelcontextprotocol.io/specification/2025-06-18/server/tools
- go-sdk v1.5.0 protocol.go: https://raw.githubusercontent.com/modelcontextprotocol/go-sdk/v1.5.0/mcp/protocol.go
- Claude Code custom-tools docs: https://code.claude.com/docs/en/agent-sdk/custom-tools
- Claude Code MCP output limits: https://code.claude.com/docs/en/mcp
- claude-code issue #55677 (text dropped when structuredContent present): https://github.com/anthropics/claude-code/issues/55677
- claude-code issue #4427 (structuredContent ignored v1.0.60): https://github.com/anthropics/claude-code/issues/4427
- claude-code issue #1072 (raw mode / Ink): https://github.com/anthropics/claude-code/issues/1072
- bubbletea pkg docs: https://pkg.go.dev/github.com/charmbracelet/bubbletea
- GStreamer pull vs push mode: https://discourse.gstreamer.org/t/segment-seeking-on-various-sources-streaming-mode-random-access/2296
- Azure batch synthesis: https://learn.microsoft.com/en-us/azure/ai-services/speech-service/batch-synthesis
- MCP spec discussion #1563: https://github.com/modelcontextprotocol/modelcontextprotocol/discussions/1563

Added verifier sources (re-research 2026-06-24):

- go-sdk v1.5.0 server.go / tool.go / protocol.go (synchronous CallTool + `Session.NotifyProgress`): https://github.com/modelcontextprotocol/go-sdk/tree/v1.5.0/mcp
- oto v3.4.0 player.go + LICENSE (Seek signature + Apache-2.0): https://github.com/ebitengine/oto/tree/v3.4.0
- Claude Code interactive-mode docs (ctrl+o collapse/expand of tool results): https://code.claude.com/docs/en/interactive-mode
- Claude Code agent-sdk custom-tools doc (text dropped when structuredContent set): https://code.claude.com/docs/en/agent-sdk/custom-tools
- wavefilegem WAVE chunk reference: https://wavefilegem.com/how_wave_files_work.html
- soundfile.sapp.org WAVE format: http://soundfile.sapp.org/doc/WaveFormat/
- macOS sys/un.h (`sun_path` = 104): https://opensource.apple.com/source/xnu/xnu-7195.141.2/bsd/sys/un.h
- termios man pages (per-tty raw mode): https://man7.org/linux/man-pages/man3/termios.3.html
- GStreamer scheduling / pull-vs-push docs (GstBaseSrc seekable streaming source): https://gstreamer.freedesktop.org/documentation/plugin-development/advanced/scheduling.html

## Related decisions

- **SUPERSEDES the draft's single-channel-only D5** — the re-researched two-channel
  (inline receipt + decoupled observer) model is now the accepted decision; the OLD
  receipt-`TextContent`-only-as-the-complete-story conclusion is superseded by this same file.
- [Terminal listen-not-read is ephemeral-afplay audio-only](2026-06-23-terminal-listen-not-read-is-ephemeral-afplay-audio-only.md) — CONFIRMED, no flip
- [React player is optional (#50)](2026-06-23-react-player-optional-reuses-existing-player-50.md) — CONFIRMED
- [Player playback unit stays whole audio.wav](2026-06-22-player-playback-unit-stays-whole-audio-wav.md) — CONFIRMED
- [Player rAF audio sync](../convention/2026-06-21-player-raf-audio-sync-transition-only-rerender.md) — CONFIRMED (React optional path)
- [Primary listen path decoupled from durable sink](../tradeoff/2026-06-23-primary-listen-path-decoupled-from-durable-sink.md) — CONFIRMED

## Revisit trigger

- If MCP gains a first-class async/streaming tool-result mechanism, re-evaluate whether the
  decoupled observer (Channel 2) can collapse back into the receipt.
- If a spike shows `notifications/progress` gives an adequate user-visible live surface, revisit
  the decoupled-observer-as-default choice.
- If `structuredContent` behavior stabilizes across Claude Code versions and the text-dropped
  behavior is documented normatively, remove the duplicate `TextContent` block recommendation.
- If the project gains CI or moves off macOS-only, re-evaluate the server-spawned osascript
  window as a platform-appropriate option.
- If streaming / real-time narration is added (phase-two non-goal), transport control
  architecture needs a full re-analysis.

## Addendum — 2026-06-25 (from the #79 design session)

> **bubbletea v2 dependency-weight + import-path correction.** This ADR's same-terminal-TUI
> rejection (see *Rejected options*) and the Revisit triggers reference a pure-Go TUI
> (bubbletea/tview/tcell) as the heavier alternative. The #79 transport-UI research
> (`research/listen-transport-ui-issue-79/report.md`, claim 1) surfaced two corrections for
> any future TUI reconsideration: (1) bubbletea **v2's import path moved** — it no longer
> resolves at `github.com/charmbracelet/bubbletea/v2` (that path fails with *"module declares
> its path as: charm.land/bubbletea/v2"*); the real v2 module path is **`charm.land/bubbletea/v2`**.
> (2) Its dependency weight is **~16 build-relevant modules / 21 in `go list -m all`** — the
> heaviest of the candidates (tview+tcell ≈ 8; a stdlib + `golang.org/x/term` keypress loop ≈ 1).
> This does not change any decision in this ADR; it only corrects the figures so a later TUI
> reconsideration starts from the right path and weight.
