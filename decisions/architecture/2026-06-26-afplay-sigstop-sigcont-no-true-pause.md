# afplay SIGSTOP/SIGCONT cannot deliver a true Pause — process freeze does not silence CoreAudio playback; honest "Stop / Replay block" stands

| Field    | Value            |
|----------|------------------|
| Date     | 2026-06-26       |
| Status   | accepted         |
| Category | architecture     |
| Tags     | listen-path, cmd-narrate, afplay, sigstop, sigcont, pseudo-pause, true-pause, coreaudiod, audioqueue, honesty-rule, by-ear-verify, issue-84, issue-79, issue-83 |

## Context

Issue #84 is the by-ear `/verify` spike reserved by the #79 listen-transport design. The shipped
controller (#83) deliberately exposes an honest "Stop / Replay block" rather than a true mid-stream
Pause, because in phase one `afplay` had **no verified-clean pause seam**. The #79 decision carried an
explicit revisit trigger: *"If a by-ear `/verify` test proves the `afplay` `SIGCONT` resume seam is
audibly clean, promote the 'Stop / Replay block' control to a true OS-level 'Pause'."* This decision
records the result of running that test.

The seam under test is the exact one the controller uses: `afplay <wav>` spawned as a child
(`cmd/narrate/listen.go` `playBlock`), then `SIGSTOP` followed by `SIGCONT` sent to the live `afplay`
PID mid-block.

## Options considered

### Option A: Promote "Stop / Replay block" to a true Pause via SIGSTOP/SIGCONT on the afplay PID
- **Pros**: zero new deps; reuses the existing afplay child; OS signals are already available.
- **Cons**: **falsified by measurement.** `SIGSTOP` freezes the `afplay` process but not `coreaudiod`,
  the separate system daemon that owns the enqueued audio and the DAC. `afplay` front-loads the whole
  block's PCM into the CoreAudio `AudioQueue`, so the audio keeps playing to completion during the
  "pause." `SIGSTOP` freezes the wrong process — there is no audible pause to resume from.

### Option B: Keep the honest "Stop / Replay block" model (status quo, #83)
- **Pros**: truthful to what the system can actually do (honesty rule); already shipped; no false Pause
  affordance.
- **Cons**: a listener restarts the current block rather than resuming it — accepted phase-one limit.

### Option C: True Pause via a different audio path (future)
- **Pros**: a real pause is achievable with in-process `AudioQueuePause`.
- **Cons**: not available in phase one. `afplay` has no seek/offset flag (so "kill + re-spawn at
  offset" is impossible), and in-process queue control arrives only with the deferred `sherpa-onnx-go`
  CGo renderer. Out of scope now.

## Decision

**Keep the honest "Stop / Replay block" model (Option B). A true OS-signal Pause on `afplay` is not
achievable and the promotion path reserved by #79 is closed by evidence.** The seam fails one level
deeper than #84 anticipated: not "the resume has a click/gap" but "the process freeze does not silence
playback at all." Measurement (below) is decisive and reproducible across 10 s and 60 s blocks. The
`cmd/narrate/listen.go` rationale is upgraded from *unverified caution* ("no verified-clean pause seam")
to *verified fact* (the seam is verified **absent**). If a true Pause is ever wanted, it belongs to the
deferred CGo / `sherpa-onnx-go` in-process renderer using `AudioQueuePause` (Option C), not OS signals.

## Consequences

- The #79 revisit trigger is **resolved negative** — do not file the "promote to true Pause" follow-up;
  it would chase an impossible seam. (The #79 decision itself stands unchanged; only its open question
  is now answered.)
- "True Pause on the listen path" moves to a deferred CGo-phase concern, parented to this spike.
- The honesty-rule wording in the controller and the #79 report is now defensible by measurement, not
  just by caution.

## Related decisions

- [LISTEN-path terminal transport: keypress loop, honest "Stop / Replay block", not "Pause" (#79)](2026-06-25-listen-transport-keypress-loop-not-tui.md) — this spike resolves that decision's by-ear `SIGCONT` revisit trigger; answer is negative, so the honest model stays.
- [Terminal listen-not-read is ephemeral-afplay audio-only](2026-06-23-terminal-listen-not-read-is-ephemeral-afplay-audio-only.md) — `afplay`'s lack of runtime IPC / queue control is the root cause; this measurement confirms it concretely.

## Experiments

By-ear `/verify` spike (issue #84). Probes archived at
`research/afplay-sigcont-pause-seam-84/_history/probes/`; full report at
`research/afplay-sigcont-pause-seam-84/report.md`.

- **Methodology:** play a fixture clip (24 kHz mono Int16, Kokoro-native) via `afplay`; `SIGSTOP` the
  PID mid-block, hold a freeze window `H`, `SIGCONT`, measure total wall-clock. If audio truly pauses,
  total grows by ~`H`; if it keeps playing, total ~= baseline.
- **Process mechanics:** `SIGSTOP` -> state `T`, `SIGCONT` -> state `S`, exit code `0` (both runs).
  Signal delivery works.
- **Decisive timing:** baseline 10 s clip = 10.57 s. Freeze **6.0 s** -> total 10.80 s (**+0.24 s**).
  Freeze **8.0 s** -> total 10.79 s (**+0.23 s**). 60 s clip, freeze **6.0 s** -> 60.48 s (**+0.41 s**).
  Freezing the process for 6–8 s added only ~0.2–0.4 s — the audio did not pause.
- **Mechanism:** `afplay` enqueues the whole block PCM into the CoreAudio `AudioQueue` up front;
  `coreaudiod` (separate, un-frozen) drains it to completion regardless of the frozen `afplay` process.
- **Human-ear corroboration (optional, non-load-bearing):** re-running
  `sigcont_confirm.py` lets a human confirm audio is still audible during the freeze. The verdict does
  not depend on it — timing is conclusive.

## Revisit trigger

- If the project adopts the in-process `sherpa-onnx-go` (CGo) renderer, re-evaluate a true Pause via
  `AudioQueuePause` — a real pause becomes available then (Option C), independent of OS signals.
