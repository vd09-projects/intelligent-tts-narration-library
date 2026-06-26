---
slug: afplay-sigcont-pause-seam-84
ticket: "#84"
intent: verify
version: 1
status: verified-draft
created: 2026-06-26
stage: by-ear verification spike — measured evidence gathered; one human-ear corroboration surfaced
linked_task: "#84"
relates_to: ["#79 (research report, parent)", "#83 (listen-path controller, shipped honest Stop/Replay)"]
---

# Verify: is the afplay SIGSTOP/SIGCONT resume seam clean enough for a true Pause?

**Verdict (one line):** **No — and not because the resume is glitchy, but because
`SIGSTOP` on the `afplay` PID does not pause audible playback at all** for any realistic
narration-block size. `afplay` front-loads the entire block's PCM into the CoreAudio
`AudioQueue`; the audio is then owned by `coreaudiod`, a *separate, un-frozen* daemon that
keeps draining the queue to completion while the `afplay` process sits frozen. `SIGSTOP`
freezes the wrong process. The honest **"Stop / Replay block"** model shipped in #83 stands;
the conditional "promote to true OS-level Pause via SIGSTOP/SIGCONT" follow-up is **falsified
and should not be filed**.

This spike ships **no product code** (issue #84 AC). The only code produced is two throwaway
measurement probes, archived under `_history/probes/`.

---

## 1. What was tested

The exact seam the listen-path controller uses: `afplay <wav>` spawned as a child
(`cmd/narrate/listen.go` `playBlock` -> `exec.CommandContext(callCtx, "afplay", wavPath)`),
then `SIGSTOP` followed by `SIGCONT` sent to the live `afplay` PID mid-playback.

- **Audio:** clips cut from the project fixture `player/public/fixtures/sample/audio.wav`
  (24 kHz mono Int16 — Kokoro native; no resampling). Two lengths: **10 s** and **60 s**.
- **Host:** macOS (Darwin 25.5.0), `/usr/bin/afplay`.
- **Probes:** `_history/probes/sigcont_probe.py` (state + timing classifier) and
  `_history/probes/sigcont_confirm.py` (decisive baseline-vs-freeze test).

The measurement principle: **if `SIGSTOP` truly pauses audible playback for a freeze window
of `H` seconds, total wall-clock to completion must grow by ~`H`. If the audio keeps playing
during the freeze, total wall-clock ~= baseline (freeze adds nothing).**

---

## 2. Measured data

### 2a. Process-state mechanics (probe 1, 10 s clip, stop@3 s, hold 4 s, 2 runs)

| Observation                          | Run 1   | Run 2   | Grade            |
|--------------------------------------|---------|---------|------------------|
| State after `SIGSTOP`                | `T` (stopped) | `T` | verified (probe) |
| State after `SIGCONT`                | `S` (running) | `S` | verified (probe) |
| Clean exit code                      | `0`     | `0`     | verified (probe) |
| Total wall-clock (start->exit)       | 10.99 s | 10.49 s | verified (probe) |

The process *signal mechanics work*: `SIGSTOP` freezes the PID, `SIGCONT` un-freezes it, exit
is clean. **But the total wall-clock is the tell:** a 10.07 s clip held frozen for 4.17 s
finished in ~10.5–11.0 s, not the ~14.2 s a real pause would require.

### 2b. Decisive baseline-vs-freeze test (probe 2)

| Condition                              | Total wall-clock | Added over baseline | Grade            |
|----------------------------------------|------------------|---------------------|------------------|
| Baseline, 10 s clip, no signals        | 10.57 s          | —                   | verified (probe) |
| 10 s clip, freeze **6.0 s** (stop@2 s) | 10.80 s          | **+0.24 s**         | verified (probe) |
| 10 s clip, freeze **8.0 s** (stop@1 s) | 10.79 s          | **+0.23 s**         | verified (probe) |
| 60 s clip, freeze **6.0 s** (stop@2 s) | 60.48 s          | **+0.41 s**         | verified (probe) |

Freezing the `afplay` process for 6–8 s added **~0.2–0.4 s** of wall-clock — essentially
nothing. The audio did **not** pause. Even at 60 s (~2.9 MB of PCM) `afplay` had already
handed the whole block to the `AudioQueue` before the freeze, so `coreaudiod` played straight
through. The naive "TRUE-RESUME" label probe 1 emits is an artifact of comparing against two
wrong models; the decisive test (2b) shows there is no pause to resume *from*.

---

## 3. Mechanism (why)

`afplay` is not a streaming player that meters audio against its own process clock. It opens
the file, enqueues the decoded PCM into a CoreAudio `AudioQueue`, and then blocks waiting for
the "queue drained" callback. For block-sized audio the **entire** clip is enqueued up front.

- `SIGSTOP` freezes the `afplay` process — i.e. the thing *waiting for the callback*.
- It does **not** freeze `coreaudiod`, the system audio daemon that owns the enqueued buffer
  and the actual DAC output. `coreaudiod` keeps playing.
- So during the "pause" the user **still hears the audio**, uninterrupted, to the end of the
  block. `SIGCONT` then lets `afplay` notice the already-completed playback and exit.

This is the opposite of a glitchy-but-working pause. There is no audible seam artifact to
judge because **there is no pause** — `SIGSTOP` freezes a process that is no longer the thing
producing sound.

---

## 4. Verdict and what needs a human ear

**Verdict: the seam is NOT clean enough for a true Pause.** It fails one level deeper than #84
anticipated — not "resume has a click/gap" but "the process freeze does not silence playback."
A true OS-signal Pause on `afplay` is not achievable in phase one.

**Decided by measurement (no ear required):** that audio keeps playing through the freeze —
the wall-clock evidence in section 2b is conclusive and reproducible.

**The one corroboration to confirm by ear (optional, non-load-bearing):** run
`python3 research/afplay-sigcont-pause-seam-84/_history/probes/sigcont_confirm.py` and confirm
that during the 6–8 s "freeze" window **you continue to hear the audio** (and that it is not
silent). The measured verdict does not depend on this — timing already proves it — but a human
ear listening once removes any doubt. (Author cannot self-attest an audible judgment.)

---

## 5. Implication for the listen-path transport controller

1. **Honest "Stop / Replay block" (#83) is the correct phase-one model and stays.** The code
   comment in `cmd/narrate/listen.go` ("`afplay` has no verified-clean pause seam in phase
   one") is now upgraded from *unverified caution* to *verified fact* — the seam is verified
   **absent**, not merely unverified.
2. **Do NOT promote to a SIGSTOP/SIGCONT Pause.** The conditional follow-up the ticket
   reserved for "resume is clean" is falsified. Filing it would chase an impossible seam.
3. **A true Pause, if ever wanted, needs a different audio path**, not a different signal:
   - `afplay` has no seek/offset flag, so "kill child + re-spawn at offset" is not available.
   - In-process `AudioQueuePause` control arrives only with the deferred `sherpa-onnx-go`
     CGo renderer (CLAUDE.md: "CGo deferred"). That is the natural home for true pause, later.

---

## 6. Proposed follow-ups (held for user confirmation — none filed yet)

- **(doc, low)** Update the `cmd/narrate/listen.go` rationale comment + the #79 report
  open-item to cite this spike: the pause seam is verified absent, not just unverified. Keeps
  the honesty-rule wording defensible. *(docs/comment only — no behavior change.)*
- **(research/design, low, deferred)** "True Pause on the listen path" — revisit only when the
  `sherpa-onnx-go` in-process renderer lands (CGo phase), using `AudioQueuePause` rather than
  OS signals. Parent: this spike. Explicitly deferred, not phase one.

No follow-up promotes Stop/Replay to a signal-based Pause — that path is closed by evidence.
