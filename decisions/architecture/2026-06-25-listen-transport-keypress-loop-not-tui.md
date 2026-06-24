# LISTEN-path terminal transport: minimal raw-mode keypress loop, not a full-screen TUI; honest "Stop / Replay block", not "Pause"

| Field    | Value            |
|----------|------------------|
| Date     | 2026-06-25       |
| Status   | accepted         |
| Category | architecture     |
| Tags     | listen-path, cmd-narrate, transport-ui, keypress-loop, tui, bubbletea, tview, tcell, x-term, raw-mode, afplay, honesty-rule, sigstop-sigcont, pseudo-pause, phase-one-weight-discipline, issue-79, issue-77 |

## Context

Issue #79 (rune `analysis` — a design, not code) designs the UI/UX of the terminal playback
transport for the LISTEN path's standalone `cmd/narrate` **controller** — the process that
owns its own tty, drives a serial sequence of `afplay` subprocesses directly, and holds the
segment-file temp dir for the duration of an interactive session. It is built strictly against
the ADR #77 Data Contract. This is NOT the ADR #80 Channel-2 passive observer
(`cmd/narrate-observe`), and `cmd/narrate-mcp` must not gain cross-call playback state or a
background daemon — the transport is a `cmd/narrate` terminal feature only.

Three coupled UX choices were load-bearing: the **presentation form**, the **input mechanism**,
and the **pause semantics**. Constraints in play:

- Phase-one weight discipline (CLAUDE.md: "CGo deferred", local-only, no recurring spend) — new
  dependency weight is a first-class cost.
- The honesty rule (CLAUDE.md, non-negotiable): never imply a capability the system cannot honor.
- The block-level-sync invariant (CLAUDE.md): no sub-block / word / seconds seeking is ever exposed.
- The surface is intrinsically simple: a block list, a current-block cursor, a status line, a
  transcript pane.

The evidence-graded source of record is `research/listen-transport-ui-issue-79/report.md` (v1),
where every load-bearing claim carries a grade and a citation; the design deliverable is
`docs/design/listen-transport-ui.md`.

## Options considered

### Fork A — Presentation form

#### Option A1: minimal keypress loop (stdlib + `golang.org/x/term`)
- **Pros**: adds exactly **one** genuinely new direct dependency (`golang.org/x/term`;
  `golang.org/x/sys` is already in `go.mod` indirect) — ~1 build-relevant module. No CGo. The
  control set the ADR #77 contract requires needs nothing a TUI buys.
- **Cons**: hand-rolled rendering; would need rework if the surface later grows
  (search / filter / scrollback).

#### Option A2: `rivo/tview` + `gdamore/tcell/v2` full-screen TUI
- **Pros**: rich full-screen layout out of the box; pure-Go, no CGo.
- **Cons**: **~8 new build-relevant modules** for a surface that is intrinsically simple — weight
  with no payoff against today's control set.

#### Option A3: `charm.land/bubbletea/v2` full-screen TUI
- **Cons**: the **heaviest** option by every measure — **~16 build-relevant modules / 21 in
  `go list -m all`** (verification corrected the draft's "~17–18" figure). Its v2 **import path
  churned across majors**: `github.com/charmbracelet/bubbletea/v2` no longer resolves (the module
  declares its path as `charm.land/bubbletea/v2`) — exactly the phase-one liability weight
  discipline exists to avoid.

### Fork B — Input mechanism

#### Option B1: raw-mode single-key via `golang.org/x/term` (`n`/`b`/`space`/`g`/`q`)
- **Pros**: true single-keypress transport control (press `n`, advance — no Enter). `x/term`
  v0.44.0 supplies exactly `MakeRaw`/`Restore`/`IsTerminal`, pure-Go via `x/sys` (no CGo).
- **Cons**: one hazard — if the process dies on an uncaught signal, the deferred `Restore` never
  runs and the terminal is left in no-echo/raw state. **Fully mitigated** by the same signal
  handler the temp-dir cleanup already requires (two correctness needs collapse into one handler).

#### Option B2: line-prompt input (Enter per command)
- **Cons**: Enter-per-command is worse UX for a transport surface; offers no upside that offsets
  the loss of single-key immediacy.

### Honesty call — pause semantics

#### Option H1: ship "Stop / Replay block" (stop `afplay` + replay current block from start)
- **Pros**: honest. `afplay` has **no runtime pause / seek / position IPC** (verified: its entire
  option set is start-time parameters) — so a true mid-block resume cannot be delivered. Replaying
  the current block from its start is the truthful coarse block-granular pseudo-pause and is exactly
  the ADR #77 AC2 control.
- **Cons**: a listener cannot resume mid-block; restarting the block is the only "pause" semantic.

#### Option H2: ship a bare "Pause" (with a mid-block progress bar)
- **Cons**: implies a mid-block resume the system cannot honor — a **CLAUDE.md honesty-rule
  violation**. Rejected.

#### Option H3 (alternative, gated): true OS pause via `SIGSTOP`/`SIGCONT` on the `afplay` PID
- **Pros**: a genuine OS-level mid-block pause on Darwin (`SIGSTOP` "cannot be caught or ignored";
  `S → T → S` observed empirically). Would earn a *true* "Pause" label.
- **Cons**: the **audible cleanliness of the `SIGCONT` resume seam** (CoreAudio buffer state on
  resume) is unverified by any source and can only be checked by listening. Not adopted as default;
  carried forward as a by-ear open item.

## Decision

**Three coupled picks:**

1. **Presentation form → Option A1, the minimal raw-mode keypress loop** (stdlib +
   `golang.org/x/term`), NOT a full-screen TUI. The surface is intrinsically simple, all three
   candidates are pure-Go/no-CGo, so the deciding axis is new-dependency weight under phase-one
   discipline — and the keypress loop adds one module versus ~8 (tview+tcell) or ~16/21
   (`charm.land/bubbletea/v2`). The TUI buys nothing the ADR #77 control set needs today.
   **Reconsider the TUI only if the surface gains search / filter / scrollback.**

2. **Input mechanism → Option B1, raw-mode single-key via `x/term`** (`n`/`b`/`space`/`g`/`q`),
   NOT line-prompt. A transport wants single-keypress control. The one raw-mode hazard (terminal
   left raw on an uncaught signal) collapses into the **same signal handler** the temp-dir cleanup
   already mandates — which is itself an argument *for* raw mode over line-prompt, not against it.

3. **Pause semantics → Option H1, the honest "Stop / Replay block"**, NOT a bare "Pause".
   `afplay` cannot resume mid-block, so labelling the control "Pause" with a progress bar would
   imply a capability the system cannot honor — a direct honesty-rule violation. The surface ships
   "Stop / Replay block" (stop + replay-current-block-from-start). The `SIGSTOP`/`SIGCONT` true-OS-
   pause (Option H3) is the alternative, **gated on a by-ear test** of the resume seam; until that
   passes, the surface must not present a true "Pause" affordance.

## Consequences

- The new controller takes on exactly one new direct dependency (`golang.org/x/term`); the on-wire
  `plan.json` schema and the planner's no-I/O purity are untouched (this is a `cmd/`-layer surface).
- A single SIGINT/SIGTERM handler does three jobs in order — `term.Restore` the tty, `Kill` + `Wait`
  the live `afplay` child (so two players never overlap; `Process.Kill` is async and must be reaped),
  then `os.RemoveAll` the session temp dir (idempotent, safe to also run on normal exit). Go's default
  signal disposition skips deferred cleanup, so the handler is mandatory, not optional (verified by
  probe). Prefer stdlib `exec.Cmd.WaitDelay` to bound the kill-then-reap window over a hand-rolled grace.
- The view-model binds to the real `plan/` field names: `Block.ID`, `Block.Segments[]` (plural)
  `.Text`, `Block.Level`, `Block.Status` (`voiced`/`degraded`/`refused`), and
  `BlockTiming{BlockID, StartMs, EndMs, AudioRef}`. `StartMs`/`EndMs` are display/accounting only,
  never sub-block seek targets. Refused (and zero-duration) blocks are SHOWN in the transcript but
  SKIPPED for navigation.
- The honest "Stop / Replay block" semantics mean a listener restarts the current block rather than
  resuming it. If the by-ear `SIGCONT` seam test later passes, a follow-up ticket may promote this to
  a true OS-level pause on the current block.
- If the surface ever grows beyond the simple block-list/cursor/status/transcript shape, the keypress
  loop is the thing to revisit (the rejected tview/tcell or bubbletea options become live again).

## Related decisions

- [ADR: Playback observability & control model (issue #77)](2026-06-24-playback-observability-control-model-issue-77.md) — this design implements the #79 transport surface against that ADR's Data Contract; the ADR's 2026-06-25 addendum carries the corrected bubbletea v2 path + weight surfaced by this same research.
- [Terminal listen-not-read is ephemeral-afplay audio-only](2026-06-23-terminal-listen-not-read-is-ephemeral-afplay-audio-only.md) — the LISTEN path this transport drives; `afplay`'s lack of runtime IPC is why the pause is a pseudo-pause.
- [Player playback unit stays whole audio.wav](2026-06-22-player-playback-unit-stays-whole-audio-wav.md) — same block-level-sync invariant: navigation plays whole segment files, never sub-block seeks.

## Revisit trigger

- If the transport surface gains search / filter / scrollback, re-evaluate the keypress loop versus a
  pure-Go TUI (tview+tcell ≈ 8 modules; `charm.land/bubbletea/v2` ≈ 16/21).
- If a by-ear `/verify` test proves the `afplay` `SIGCONT` resume seam is audibly clean, promote the
  "Stop / Replay block" control to a true OS-level "Pause" on the current block.
