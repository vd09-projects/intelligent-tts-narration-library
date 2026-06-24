# Terminal playback transport UI/UX — LISTEN path (`cmd/narrate` controller)

> Design deliverable for issue #79, built against the ADR #77 Data Contract.
> Evidence-graded source of record: `research/listen-transport-ui-issue-79/report.md`.
> This is a **design**, not an implementation (rune `analysis`).

## Decision summary

- **Form:** minimal **single-key raw-mode keypress loop** — stdlib + `golang.org/x/term`.
  NOT a full-screen TUI. Driver: phase-one weight discipline. tview+tcell adds 8 modules,
  `charm.land/bubbletea/v2` adds 16; the keypress loop adds one (`golang.org/x/term`;
  `golang.org/x/sys` is already in the tree). All three are pure-Go / no-CGo, so weight is
  the deciding axis. Reconsider a TUI only if the surface gains search / filter / scrollback.
- **Input:** raw-mode single-key (`n`/`b`/`space`/`g`/`q`) via `term.MakeRaw` / `term.Restore`.
  No Enter per command.
- **Pause semantics (honest):** ship **"Stop / Replay block"** — stop `afplay`, replay the
  current block from its start. NOT a bare "Pause" with a mid-block progress bar, because
  `afplay` has no mid-block resume and implying one violates the CLAUDE.md honesty rule.
  (A true OS pause via SIGSTOP/SIGCONT on the `afplay` PID is possible but gated on a by-ear
  test of the resume seam — see Open item.)

## Scope boundary

This is the standalone `cmd/narrate` **controller** (owns its tty, drives serial `afplay`,
holds the temp dir). It is NOT the ADR #80 Channel-2 passive observer (`cmd/narrate-observe`).
`cmd/narrate-mcp` MUST NOT gain cross-call playback state or a background daemon — the
transport is a `cmd/narrate` terminal feature only.

## Data contract binding (real `plan/` field names)

| View element | Source | JSON wire |
|---|---|---|
| block id | `Block.ID` | `blocks[].id` |
| spoken text | `Block.Segments[]` (plural) -> `Segment.Text` | `blocks[].segments[].text` |
| level | `Block.Level` (int; L1/L2/L3) | `blocks[].level` |
| status | `Block.Status` (`voiced`/`degraded`/`refused`) | `blocks[].status` |
| timing | `BlockTiming{BlockID,StartMs,EndMs,AudioRef}` | `timeline.blocks[].{block_id,start_ms,end_ms,audio_ref}` |

- Transcript text = the single Transcript-Derivation function (#78) joining `Segment.Text`
  across a block's `Segments[]`. Surface shows the SAME derived transcript the receipt carries.
- Refused (and zero-duration) blocks are **SHOWN in the transcript** but **SKIPPED for
  navigation** (display set is a superset of the nav set). Per the ADR Seek-Target Resolution
  Rule.
- `StartMs`/`EndMs` are **display/accounting only**. Navigation plays whole segment files
  (`<blockID>.wav` under `AudioStream.Dir`) — never sub-block seeking (block-level-sync
  invariant).

## Interaction model

| Key | Action | Clamp |
|---|---|---|
| `n` | next block (play its whole segment file) | stop at last block |
| `b` | back one block | stop at block 0 |
| `space` | stop current `afplay` + replay current block from start (honest pseudo-pause) | — |
| `g` | go-to block by `block_id` or index | resolves to nearest navigable per skip rule |
| `q` | quit (restore tty, kill+reap child, remove temp dir) | — |

Navigation skips refused/zero-duration blocks: e.g. next from block 3 when block 4 is refused
lands on block 5.

## Presentation (ASCII sketch)

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

`>` current-block pointer; `< now` the playing block; status glyphs `[v]`/`[~]`/`[x]`.
The `00:12 / 00:31` readout is block-level accounting from `BlockTiming`, not a seek bar.

## Resilience (mandatory)

One SIGINT/SIGTERM handler does three jobs in order, because Go's default signal disposition
skips deferred cleanup (verified by probe):

1. `term.Restore(fd, oldState)` — un-stick the terminal from raw mode.
2. `Kill` + `Wait` the live `afplay` child — `Kill` is async; you must reap before exit, and
   reap before starting any next `afplay` so two players never overlap.
3. `os.RemoveAll(tempDir)` — idempotent; safe to also run on normal exit.

`os.MkdirTemp` once per session; keep the segment-file dir alive until quit. Prefer stdlib
`exec.Cmd.WaitDelay` to bound the kill-then-reap window over a hand-rolled grace goroutine.

## Open item (by-ear, non-blocking)

SIGSTOP/SIGCONT on the `afplay` PID is a genuine OS-level mid-block pause on Darwin, which
would earn a *true* "Pause" label — but only if a by-ear `/verify` test proves the SIGCONT
resume seam is audibly clean (CoreAudio buffer state on resume is unverified). Until then the
surface ships "Stop / Replay block". A later ticket may promote this to a true pause if the
seam is clean.
