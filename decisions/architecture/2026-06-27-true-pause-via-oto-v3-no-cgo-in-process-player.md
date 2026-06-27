# Listen-path true Pause/Resume via ebitengine/oto v3 in-process PCM player — no CGo on macOS, decoupled from the deferred sherpa-onnx CGo renderer

| Field    | Value            |
|----------|------------------|
| Date     | 2026-06-27       |
| Status   | accepted         |
| Category | architecture     |
| Tags     | listen-path, true-pause, pause-resume, oto, ebitengine, purego, no-cgo, in-process-renderer, pcm, kokoro, apache-2.0, sherpa-onnx, device-confirmed, issue-92, issue-84 |

## Context

Issue #92 ("research: true Pause on the listen path") was parented to spike #84 and carried an
explicit premise: that a true Pause/Resume on the `narrate` listen-path belongs to the in-process
renderer's `AudioQueuePause` (CGo path) and is therefore **deferred until the `sherpa-onnx-go` CGo
renderer phase begins**. Spike #84 had proved the subprocess seam cannot pause — `SIGSTOP` freezes
`afplay` but `coreaudiod` keeps draining the enqueued `AudioQueue`, so audio plays through the freeze
(decision 2026-06-26-afplay-sigstop-sigcont-no-true-pause.md). #84's revisit trigger was: "If the
project adopts the in-process `sherpa-onnx-go` (CGo) renderer, re-evaluate a true Pause via
`AudioQueuePause`."

The huginn research run (report `research/true-pause-cgo-renderer-92/report.md` v2) re-framed the
question and **falsified the ticket's assumed path**: true pause needs in-process feed control, but it
needs NEITHER CGo NOR the sherpa renderer.

## Options considered

### Option A: github.com/ebitengine/oto/v3 in-process PCM player (CHOSEN)
- **Pros**: zero CGo on macOS (purego — satisfies the phase-one no-CGo goal the ticket assumed
  unavoidable); native `io.Reader`-of-raw-PCM API with a library-managed freeze position
  (`Player.Pause()`/`Play()`); Apache-2.0 (cleanest license; GPL gotcha moot); consumes 24 kHz mono
  int16 PCM directly (Kokoro native, no resampling at the source); maintained (ebitengine org, stable
  v3.4.0). Decouples true-pause from the deferred CGo phase entirely.
- **Cons**: oto resamples 24 kHz -> device rate internally (by-ear quality on real Kokoro output is a
  /verify step, not yet confirmed); reads ahead in ~500 ms quanta so the byte counter sits up to ~1
  buffer ahead of audible position.

### Option B: Raw CoreAudio / AudioUnit binding (CGo)
- **Pros**: real synchronous `AudioOutputUnitStop/Start`; lowest-level control.
- **Cons**: requires CGo; most glue (C render callback + manual ASBD); freeze position is
  app-managed; this is exactly the cost the ticket wanted to defer. (Tier-3/4 sourcing on exact API
  wording, but "CGo required, most glue" is structurally certain.)

### Option C: malgo / miniaudio (CGo)
- **Pros**: public-domain/MIT-0; cross-platform.
- **Cons**: requires CGo; `Device.Stop/Start` = sleep/wake with app-managed position; push
  `DataProc` fill-callback (more glue); no named Pause.

### Option D: portaudio (CGo)
- **Pros**: MIT; widely used.
- **Cons**: requires CGo PLUS an external native PortAudio library; no named Pause; app-managed
  position; untagged, last push 2026-02-03.

## Decision

**Adopt `github.com/ebitengine/oto/v3` as the in-process PCM player for the listen-path transport to
get true Pause/Resume now — no CGo, no waiting on `sherpa-onnx-go`.** The renderer keeps emitting
24 kHz mono int16 PCM; oto consumes it from an `io.Reader`; `Player.Pause()`/`Play()` freeze and
resume the read position. oto is the only option with zero CGo on macOS, the only one with a native
`io.Reader`-of-PCM API and a library-managed freeze point, and the only Apache-2.0 pick — earned over
the three CGo alternatives across true-pause semantics, CGo cost, license, ergonomics, and
maintenance. **This is a plan, not a build.**

## Consequences

- The #92 premise ("true pause deferred until the CGo renderer phase") is **overturned**: true
  pause/resume is achievable now, in-process, with no CGo. #84's revisit trigger ("if the project
  adopts the CGo renderer, re-evaluate a true Pause") is **resolved positively without CGo** — the
  trigger condition need never be met for true pause to land.
- #84's core finding stands unchanged (OS-signal `SIGSTOP/SIGCONT` pause on `afplay` is impossible;
  `coreaudiod` drains the queue). This decision does not supersede #84 — it extends it by closing its
  open question along a path #84 did not anticipate.
- Follow-up work is a build, not part of this research: a spike to wire oto into the listen-path and
  confirm pause+resume by ear on real Kokoro output, then a gated integration behind the transport.
- Phase-one no-real-time stance is preserved; no latency/CPU benchmark was run (out of scope). If
  latency ever becomes a driver, a separate spike compares oto vs CoreAudio/miniaudio.

## Related decisions

- [afplay SIGSTOP/SIGCONT cannot deliver a true Pause (#84)](2026-06-26-afplay-sigstop-sigcont-no-true-pause.md) — this decision **extends** #84 and **resolves its revisit trigger** without CGo. #84 proved OS-signal pause impossible and named the deferred CGo renderer as the only path to true pause; oto v3 provides true pause in-process with no CGo, so the trigger condition is moot. #84's finding remains valid and accepted.
- [oto v3.4 Player.Close() is a no-op (finalizer teardown) (#100 → #101)](2026-06-27-oto-v3-4-player-close-no-op-finalizer-teardown.md) — follow-on **amendment** from the #100 build. The plan's `player.Close()`-first teardown ordering, derived from this decision's premise, is overturned by oto v3.4 making `Close()` a no-op with teardown moved to a GC finalizer. This decision's oto-v3 choice stands (device-confirmed); only the plan-derived teardown sub-invariant is corrected. Production fd-lifecycle fix tracked to #101.

## Experiments

On-device verification probe (huginn Stage 3), run on this machine (go1.26.1 darwin/arm64,
oto v3.4.0, CoreAudio) — NOT deferred. Full write-up in `research/true-pause-cgo-renderer-92/report.md`
v2 §5. Probe artifacts were in a temp dir; nothing written to the repo.

- **Methodology:** Kokoro isn't wired, so the 24 kHz mono LEI16 PCM source came from macOS `say`
  (WAV chunks parsed, 631244 PCM bytes ~= 13.15 s). The `io.Reader` was wrapped in an atomic counting
  reader (oto only pulls bytes when feeding the device, so the byte-offset is the audible position
  modulo read-ahead). A 100 ms sampler logged offset + `IsPlaying()` across Play -> Pause(1.5 s) -> Play.
- **Result:**
  - offset @ Play-end: 96000 bytes (2000 ms), advancing
  - offset @ Pause+150ms: 96000 bytes, frozen
  - offset @ Pause-end (1.5 s): 96000 bytes, still frozen
  - offset @ Resume-end: 168000 bytes (3500 ms), continuous from 96000
  - pause-window delta: 0 bytes / 0.0 ms
  - resume-window delta: 72000 bytes / 1500 ms, no reset, no gap
- **LBC-4 PASS** — `Pause()` froze inside the first 100 ms window; delta exactly 0 bytes (tighter than
  "within ~1 buffer"). `IsPlaying()` flipped within one sample.
- **LBC-6 PASS** — `Play()` resumed from the frozen 96000 offset; no restart, no backward jump, no
  bleed past the pause.
- **Verification scoreboard:** all 7 load-bearing claims verified or device-confirmed; none contested
  or unsupported. LBC-3 (no CGo on macOS via purego), LBC-5 (consumes 24 kHz mono int16 from io.Reader),
  LBC-7 (oto Apache-2.0; competitors non-GPL but CGo) verified from docs/SPDX; LBC-1/LBC-2 from #84.

## Revisit trigger

- If oto v3's internal 24 kHz -> device-rate resampling proves audibly poor on real Kokoro output during
  the build spike's by-ear `/verify`, re-evaluate (source-rate match or an alternative player).
- If real-time / latency becomes a phase-two driver, run the deferred latency/CPU benchmark of oto vs
  CoreAudio/miniaudio.
