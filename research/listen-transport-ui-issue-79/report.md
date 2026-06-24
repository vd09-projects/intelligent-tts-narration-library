---
slug: listen-transport-ui-issue-79
ticket: "#79"
intent: UI
version: 1
status: verified-draft
created: 2026-06-25
huginn_stage: 5 (report) — stopped at Checkpoint 3 (pre-PR)
linked_task: "#79"
relates_to: ["#77 (ADR)", "#80 (Channel-2 observer, separate)", "#78 (transcript-derivation fn, downstream)"]
---

# Design: terminal playback transport for the LISTEN path — ship a minimal raw-mode keypress loop, not a full-screen TUI

**Recommendation (one line):** Build the `cmd/narrate` listen-path transport as a
**minimal single-key raw-mode keypress loop** (stdlib + `golang.org/x/term`), expose an
**honest block-granular control set** ("Stop / Replay block", not a true "Pause"), and
collapse raw-mode restore and temp-dir cleanup into the **one signal handler** that
correctness already requires. Defer the full-screen TUI until the surface grows.

This report is the evidence-graded, versioned source for the design deliverable at
`docs/design/listen-transport-ui.md`. Every load-bearing claim below carries a grade and a
citation; claims settled by a throwaway verification probe are marked `verified (probe)`
and the probe sources are archived under `_history/probes/`.

---

## 1. Scope (X / Y / assumptions)

**What this designs (X — given):** the UI/UX of the standalone `cmd/narrate` **controller**
on the LISTEN path — the process that owns its own tty, drives a serial sequence of `afplay`
subprocesses directly, and holds the segment-file temp dir for the duration of an interactive
session. Output is a **design**, not code (issue #79 AC6; rune `analysis`).

**What this is NOT:** the ADR #80 Channel-2 **passive observer** (`cmd/narrate-observe`) — a
separate ticket. The transport is a `cmd/narrate` terminal feature only; `cmd/narrate-mcp`
MUST NOT gain cross-call playback state or a background daemon (issue #79 AC4).

**Unknowns resolved here (Y):** presentation form (TUI vs keypress loop), input mechanism
(raw-mode single-key vs line-prompt), the honest pause semantics, the resilience/cleanup
model, and the binding to the real data contract.

**Assumptions that, if wrong, change the answer:**
- A1: phase-one **weight discipline** ("CGo deferred", local-only, no recurring spend —
  CLAUDE.md) dominates the form choice. If the surface later grows (search / filter /
  scrollback), reconsider the TUI.
- A2: the transport surface is **intrinsically simple** — a block list, a current-block
  cursor, a status line, a transcript pane. (Holds for the ADR #77 control set.)
- A3: block-level sync is a hard invariant (CLAUDE.md): no sub-block / word / seconds
  seeking is ever exposed.

---

## 2. Decisions

### Fork A — Presentation form: **minimal keypress loop**, not full-screen TUI

The surface is a block list + cursor + status line + transcript pane — intrinsically simple
(A2). The deciding axis is **new-dependency weight** under phase-one discipline (A1):

| Option | New modules (build-relevant) | New modules (`go list -m all`) | CGo? | Grade |
|---|---|---|---|---|
| stdlib loop + `golang.org/x/term` | **1 beyond stdlib** (`x/sys`; already in tree) | 2 | no | `verified` |
| `rivo/tview` + `gdamore/tcell/v2` | **8** | 18 | no | `verified` (count) / `verified` (no-CGo) |
| `charm.land/bubbletea/v2` v2.0.7 | **16** | 21 | no | `contested` (count band + path) |

> **Correction surfaced in verification (claim 1, contested).** The draft's "bubbletea
> ≈17–18 modules" does not match either standard measure (16 build-relevant / 21 full
> graph), and — more importantly — **bubbletea v2 no longer lives at
> `github.com/charmbracelet/bubbletea/v2`**: that path fails to resolve
> (`module declares its path as: charm.land/bubbletea/v2`). The correct v2 path is
> `charm.land/bubbletea/v2`. This does not flip the pick — it *strengthens* it: bubbletea is
> the heaviest option by every measure **and** its import path churned between majors, which
> is exactly the kind of phase-one liability weight discipline is meant to avoid.

**Pick:** stdlib keypress loop. `golang.org/x/sys` is already in `go.mod` (indirect); the
only genuinely new direct dependency is `golang.org/x/term`. The TUI buys nothing the
control set needs today. **Reconsider** the TUI only if the surface gains search / filter /
scrollback.

### Fork B — Input: **raw-mode single-key via `x/term`** (n/b/space/q), not line-prompt

A transport wants single-keypress control (press `n`, advance — no Enter). Line-prompt's
Enter-per-command is worse UX for this surface. Raw mode carries one hazard — if the process
dies on an uncaught signal, the terminal is left in no-echo/raw state because deferred
`Restore` never runs (claim 4, **verified by probe**). That hazard is **fully mitigated by
the same signal handler the temp-dir cleanup already needs** (see §4) — two correctness needs
collapse into one handler, which is itself an argument for raw mode over line-prompt.

`golang.org/x/term` v0.44.0 supplies exactly the needed surface (claim 3, **verified**):

```
func MakeRaw(fd int) (*State, error)
func Restore(fd int, oldState *State) error
func IsTerminal(fd int) bool
```

Pure Go via `golang.org/x/sys` (no CGo) — confirmed by a `CGO_ENABLED=0` build of a program
calling both functions.

### Honesty call — ship "Stop / Replay block", not a bare "Pause"

`afplay` has **no runtime pause / seek / position IPC** (claim 5, **verified**): once started
it plays to end-of-file or until killed. Its entire option set (`-v` volume, `-t` time,
`-r` rate, `-q` rQuality, `-d` debug, `--leaks`, `-h`) is start-time parameters; nothing is a
mid-playback control.

Therefore the user-facing pause is the **coarse block-granular pseudo-pause**: stop the
current `afplay` and replay the current block **from its start** (issue #79 AC2). Shipping a
bare **"Pause"** with a mid-block progress bar would imply a mid-block resume the system
cannot deliver — a **CLAUDE.md honesty-rule violation** (never imply a capability you can't
honor). So the surface labels this control **"Stop / Replay block"**, not "Pause".

**The SIGSTOP/SIGCONT alternative (open by-ear item).** Sending `SIGSTOP` to the `afplay`
child PID *is* a genuine OS-level mid-block pause on Darwin, and `SIGCONT` resumes it —
`SIGSTOP` "cannot be caught or ignored" (claim 6, **verified mechanic**; the `S → T → S`
state transition was observed empirically). This would earn a *true* "Pause" label — **but
only if** a by-ear test proves the `SIGCONT` resume seam is audibly clean (CoreAudio buffer
state on resume is **unverified by any source** and cannot be checked without listening).
**Until that by-ear test passes, SIGSTOP/SIGCONT pause is the alternative, not the default**,
and the surface must not present a true "Pause" affordance. This is the single open unknown
this design carries forward (see §6).

---

## 3. Contract binding (issue #79 AC1, AC4)

The view-model binds to the **real** struct field names (verified directly against `plan/`,
claim 9 — the ADR prose drift is corrected here):

| View-model element | Real source (file:type.field) | JSON wire |
|---|---|---|
| block identity | `plan/plan.go` `Block.ID` | `blocks[].id` |
| spoken text | `plan/plan.go` `Block.Segments[]` (PLURAL) -> `Segment.Text` | `blocks[].segments[].text` |
| level | `plan/plan.go` `Block.Level` (`Level` int; L1/L2/L3) | `blocks[].level` |
| status | `plan/enums.go` `Block.Status` enum: `voiced` / `degraded` / `refused` | `blocks[].status` |
| block timing | `plan/timeline.go` `BlockTiming{BlockID, StartMs, EndMs, AudioRef}` | `timeline.blocks[].{block_id,start_ms,end_ms,audio_ref}` |

- **Derived transcript** comes from the single Transcript-Derivation function (#78,
  downstream) that **joins `Segment.Text` across a block's `Segments[]`**. The surface
  displays the SAME derived transcript the receipt carries — one function, no drift
  (issue #79 AC4).
- **Honesty / refused blocks:** refused blocks are **SHOWN** in the transcript (so the
  listener sees what was refused and why) but **SKIPPED for navigation** — the display set
  is a superset of the nav set. Same rule applies to zero-duration blocks per the ADR's
  Seek-Target Resolution Rule (issue #79 AC4). `StartMs`/`EndMs` are **display/accounting
  only**, never sub-block seek targets (block-level sync invariant).
- **Navigation is realized by playing whole segment files** (`<blockID>.wav` under
  `AudioStream.Dir`) — never sub-block seeking (issue #79 AC4).

### ASCII presentation sketch (keypress-loop controller)

```
+- narrate . listen --------------------------------------------------+
|  BLOCKS                              |  TRANSCRIPT (current)         |
|  ----------------------------------  |  ---------------------------  |
|    1  [v] voiced     heading         |  Replicas set to three.       |
|    2  [v] voiced     prose           |  The deployment keeps a warm  |
|  > 3  [~] degraded   prose    < now  |  pool so a node loss does not |
|    4  [x] refused    image (skipped) |  drop capacity.               |
|    5  [v] voiced     code            |                               |
|  ----------------------------------  |  status: degraded             |
|                                      |  (read verbatim, no gist)     |
+--------------------------------------------------------------------+
| block 3/5   00:12 / 00:31   > playing                              |
| [n]ext  [b]ack  [space] stop/replay block  [g] go-to  [q]uit       |
+--------------------------------------------------------------------+
```

- `>` = current-block pointer; `< now` marks the playing block.
- Per-block status glyphs: `[v] voiced` . `[~] degraded` . `[x] refused (skipped)`.
- Refused block 4 is **shown** but **`(skipped)`** for nav — next from 3 lands on 5.
- The time readout (`00:12 / 00:31`) is **block-level** accounting from
  `BlockTiming.StartMs/EndMs` — NOT a mid-block seek bar. No word/second seeking is offered.
- Keys: `n`/`b` navigate (clamp at 0 / last), `space` = stop + replay-current-block-from-
  start (the honest pseudo-pause), `g` = go-to-block (by id or index), `q` = quit.

---

## 4. Resilience model (issue #79 AC5) — verified by probe

The lifecycle was settled empirically with a throwaway Go probe in a temp sandbox
(`_history/probes/lifecycle_probe.go` + `defer_skip_selftest.go`), modelling the controller
driving an `afplay`-stand-in child:

1. **No overlapping `afplay`.** `os.Process.Kill` is **async** — the OS may not have reaped
   the child when `Kill` returns; you **must `Wait()`** to reap before `Start`-ing the next
   `afplay`, or two players overlap (claim 7, **verified (probe)** — `reap` mode: child
   reaped, only then the next child started; no overlap).
2. **Temp dir: one per session, idempotent cleanup.** `os.MkdirTemp` once at session start;
   `os.RemoveAll` on quit. `RemoveAll` is **idempotent** — it returns `nil` on an
   already-missing path, so a double-clean (handler + normal exit) is safe (claim 8,
   **verified (probe)** — three `RemoveAll` calls on a missing path all returned `nil`).
3. **The cleanup hazard, and why a signal handler is mandatory.** Under Go's **default**
   SIGINT/SIGTERM disposition the process **dies without unwinding the stack** — deferred
   cleanup is **skipped** (claim 4, **verified (probe)**: the self-test raised SIGINT to
   itself; neither the deferred `RemoveAll` nor the end-of-`main` ran — output was
   `RAISING_SIGINT` then `signal: interrupt`, process dead). So cleanup **cannot** rely on a
   bare `defer`. It **requires** `signal.Notify` / `signal.NotifyContext`, with the handler
   doing `RemoveAll` + kill-and-reap the child before exit.

**The one handler does two jobs.** This is the same handler Fork B needs for raw-mode
`Restore`. So the SIGINT/SIGTERM handler performs, in order: (a) `term.Restore` the tty,
(b) `Kill` + `Wait` the live `afplay` child, (c) `os.RemoveAll` the session temp dir, then
exit. Two correctness needs (terminal restore, temp-dir cleanup) collapse into one path.

**Idiom note:** stdlib `exec.Cmd.WaitDelay` is the simpler way to bound the kill-then-reap
window than the existing sink's hand-rolled `killGrace` goroutine — prefer it for the new
controller. (Design preference, not a verified claim.)

---

## 5. Verification verdict table (Stage 3)

| # | Claim | Grade | What backs it |
|---|---|---|---|
| 1 | bubbletea v2.0.7 ~17-18 mods; tview+tcell ~8; stdlib loop 0-1 | **contested** | Probe: tview+tcell = 8 build-deps OK; x/term = 1 extra (`x/sys`) OK; bubbletea = 16 build / 21 full (not 17-18) AND path moved to `charm.land/bubbletea/v2` (github v2 path fails to resolve). |
| 2 | all three pure-Go / no-CGo on macOS | **verified** | `CGO_ENABLED=0 go build` passes for all three; zero `CgoFiles` in the transitive closure. tcell README quote: *"pure Go, without any need for CGO."* bubbletea proven by execution (path-corrected). |
| 3 | x/term v0.44.0 `MakeRaw`/`Restore` signatures; pure-Go via x/sys | **verified** | v0.44.0 source quote-pinned: `MakeRaw(fd int) (*State, error)`, `Restore(fd int, oldState *State) error`, `IsTerminal(fd int) bool`; no `import "C"`; `CGO_ENABLED=0` build passes. |
| 4 | default SIGINT skips defers -> terminal stuck in raw mode unless signal-handled | **verified (probe)** | Self-test raised SIGINT to self -> defer + end-of-main both skipped (`signal: interrupt`, process dead). `man signal` corroborates default-terminate. |
| 5 | afplay has no pause/seek/position IPC | **verified** | Full `afplay --help` + `man afplay` option set enumerated; zero runtime controls (all start-time params). |
| 6 | SIGSTOP/SIGCONT true OS pause on Darwin; resume-seam glitch UNVERIFIED | **verified (mechanic) + OPEN (audio seam)** | `man signal`: SIGSTOP *"cannot be caught or ignored"*; empirical `S -> T -> S` via `kill -STOP`/`-CONT`. Audible resume-seam cleanliness: **unverified, open by-ear item** — neither confirmed nor refuted. |
| 7 | `Process.Kill` async -> `Wait()` required, no overlap | **verified (probe)** | `reap` mode: child reaped before next `Start`; no two children overlap. |
| 8 | `MkdirTemp`/`RemoveAll`; `RemoveAll` idempotent on missing path | **verified (probe)** | Three `RemoveAll` on a missing path -> all `nil`. |
| 9 | contract field names | **verified** | Read directly from `plan/plan.go` (`Block.ID`, `Block.Segments[].Text`, `Block.Level`, `Block.Status`) and `plan/timeline.go` (`BlockTiming{BlockID,StartMs,EndMs,AudioRef}`). |

**Did any verdict change a pick?** No pick flipped. One correction: **claim 1 (contested)**
revised the bubbletea numbers (16 build / 21 full, not 17-18) and its **import path**
(`charm.land/bubbletea/v2`, not the github path). Both changes *reinforce* the keypress-loop
pick — bubbletea is the heaviest option by every measure and its path churned across majors.
The honesty call and resilience model are unchanged and now probe-backed.

---

## 6. Open unknowns (carried forward, honestly)

1. **SIGCONT resume-seam audio cleanliness (open by-ear).** SIGSTOP/SIGCONT is a real OS
   mid-block pause on Darwin, but whether `SIGCONT` resumes `afplay` without an audible
   click/glitch (CoreAudio buffer state) is unverified and only checkable by listening. Until
   a by-ear `/verify` test passes, the transport ships "Stop / Replay block" (the honest
   default) and does NOT expose a true "Pause". *If* the seam proves clean, a later ticket may
   promote pause to true OS-pause on the current block. This is a follow-up by-ear test, not a
   blocker for the design.

---

## 7. AC coverage map (issue #79)

| AC | Status | Where |
|---|---|---|
| AC1 — consume the ADR data contract exactly | **answered** | section 3 (field binding table, claim 9 verified) |
| AC2 — expose only block-granular controls; honest pseudo-pause | **answered** | section 2 honesty call (claim 5 verified), section 3 sketch |
| AC3 — choose & justify form + input + dep tradeoff | **answered** | section 2 Fork A (claims 1,2 — bubbletea numbers/path corrected) + Fork B (claim 3) |
| AC4 — honor ADR invariants (whole-file nav, skip refused, single transcript fn, no MCP daemon) | **answered** | section 1 scope, section 3 binding + honesty rules |
| AC5 — resilience: temp-dir lifetime, SIGINT cleanup, no overlapping afplay | **answered** | section 4 (claims 4,7,8 verified by probe) |
| AC6 — output is a design, not code | **answered** | this report + `docs/design/listen-transport-ui.md`; rune `analysis` |

All six ACs answered. The one open item (section 6, SIGCONT audio seam) is a by-ear follow-up
that does not block the design — the design ships the honest default regardless of its outcome.

---

## Changelog

- **v1 (2026-06-25):** Initial report. Stage 3 adversarial verification of 8 load-bearing
  claims (5 blind `claim-verifier` workers + a throwaway resilience probe settling claims
  4/7/8). Claim 1 downgraded to `contested` — bubbletea module count revised (16 build / 21
  full, not 17-18) and import path corrected to `charm.land/bubbletea/v2`; correction
  reinforces the keypress-loop pick. SIGSTOP/SIGCONT resume-seam audio cleanliness recorded
  as the single open by-ear item. No pick flipped. Probe artifacts archived under
  `_history/probes/`.
