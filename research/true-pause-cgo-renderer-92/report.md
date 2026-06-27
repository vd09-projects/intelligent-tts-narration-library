# huginn — research report — ticket #92 "true Pause on the listen path"

Report path: research/true-pause-cgo-renderer-92/report.md (v2)
Changelog v1 → v2: (1) added §3 alternatives comparison defending the oto v3 pick against raw-CoreAudio-CGo / malgo / portaudio across 5 axes; (2) ran the cmd/oto-pause-probe spike ON-DEVICE instead of deferring it — LBC-4 and LBC-6 promoted from source-only to device-confirmed; scoreboard + caveats updated. (v1 was surfaced in-session but never persisted to this scope; v2 is the first on-disk version.)

## 1. Question & framing
X (want): a true Pause/Resume on the narrate listen-path — audio freezes at a point and resumes from exactly that point, not a restart.
Y (the ticket's assumed path): "true Pause belongs to the in-process renderer's AudioQueuePause (CGo path), deferred until sherpa-onnx-go lands" — i.e. true-pause assumed blocked on the deferred CGo renderer phase.
Why now: spike #84 proved the subprocess seam can't pause — SIGSTOP freezes afplay but coreaudiod keeps draining the queued AudioQueue, so audio plays through the freeze (decision 2026-06-26-afplay-sigstop-sigcont-no-true-pause.md). True pause needs in-process control of the byte feed.
Load-bearing reframe: the ticket's assumption is FALSIFIED. True pause needs in-process feed control but NOT CGo and NOT the sherpa renderer. github.com/ebitengine/oto/v3 gives in-process true pause/resume of a streamed PCM io.Reader with zero CGo on macOS (purego), Apache-2.0 — decoupling true-pause from the deferred CGo phase.

## 2. Recommendation
Adopt github.com/ebitengine/oto/v3 as the in-process PCM player for the listen-path transport to get true Pause/Resume now — no CGo, no waiting on sherpa-onnx-go. Renderer keeps emitting 24 kHz mono int16 PCM (Kokoro native, no resampling); oto consumes it from an io.Reader; Player.Pause()/Play() freeze and resume the read position. Device-confirmed (§5), earned over alternatives (§3). This is a plan, not a build.

## 3. Alternatives comparison
Pinned June 2026: oto v3.4.0 · malgo v0.11.25 · portaudio v0.0.0-20260203164431 · raw CoreAudio = current macOS framework.

| Axis | oto v3 | Raw CoreAudio/AudioUnit (CGo) | malgo / miniaudio (CGo) | portaudio (CGo) |
|---|---|---|---|---|
| True pause/resume | Named Pause()/Play(); Player holds the io.Reader → freeze-point preserved by the library | AudioOutputUnitStop/Start synchronous; real, but position app-managed | Device.Stop/Start = sleep/wake; position app-managed | Stream.Stop/Start; no named Pause; position app-managed |
| CGo on macOS | None (purego) | Required | Required | Required + external native PortAudio lib |
| License | Apache-2.0 | Apple system framework | Unlicense / miniaudio public-domain·MIT-0 | MIT |
| io.Reader/PCM ergonomics | Native io.Reader of raw PCM, minimal glue | Most glue: C render callback + manual ASBD | Push DataProc you fill | Callback / blocking typed Write; no io.Reader |
| Maintenance | ebitengine org; stable v3.4.0 (2025-10-04), pushed 2026-06-14 | You maintain the binding | gen2brain; v0.11.25 (2026-05-13) | Untagged; last push 2026-02-03 |

Why oto wins (earned): only option with zero CGo on macOS (satisfies the phase-one no-CGo goal the ticket assumed unavoidable), only one with a native io.Reader-of-PCM API and a library-managed freeze position, and Apache-2.0 (cleanest license; GPL gotcha moot). The three competitors can all reach resume-from-freeze, but only via stop/start with app-managed position + fill-callback (more glue), and all three need CGo (portaudio also an external native lib) — exactly the cost the ticket wanted to defer. Only axis a competitor could win is raw latency/control (CoreAudio/miniaudio) — not a requested axis and unverified here.
Nuance: oto itself drives CoreAudio via purego, so "raw binding ⇒ CGo" holds only for the hand-written CGo binding as posed, not absolutely. Doesn't change the pick.

## 4. Verification scoreboard
| # | Load-bearing claim | Grade | Evidence |
|---|---|---|---|
| LBC-1 | afplay SIGSTOP/SIGCONT can't true-pause (coreaudiod drains queue) | verified | spike #84 + decision 2026-06-26 |
| LBC-2 | True pause needs in-process feed control | verified (derived) | #84 + CoreAudio drain semantics |
| LBC-3 | oto v3 needs no CGo on macOS (purego) | verified | oto README "macOS (no Cgo required!)" v3.4.0 |
| LBC-4 | oto Pause() halts source reads within ~1 buffer | DEVICE-CONFIRMED | probe: pause-window delta 0 bytes / 0.0 ms |
| LBC-5 | oto consumes 24 kHz mono int16 PCM from io.Reader | verified | oto godoc: FormatSignedInt16LE, mono, arbitrary rate |
| LBC-6 | oto Play() resumes from frozen offset, not restart | DEVICE-CONFIRMED | probe: 96000 → 168000 continuous, no reset |
| LBC-7 | oto Apache-2.0; competitors non-GPL but CGo | verified | GitHub SPDX (oto Apache-2.0, malgo Unlicense, portaudio MIT) |

All load-bearing claims behind the recommendation are verified or device-confirmed; none contested or unsupported.

## 5. On-device verification pass
The probe was actually run on this machine (go1.26.1 darwin/arm64, oto v3.4.0, CoreAudio), not deferred. Kokoro isn't wired, so the 24 kHz mono LEI16 PCM source came from macOS say (WAV chunks parsed, 631244 PCM bytes ≈ 13.15 s). The io.Reader was wrapped in an atomic counting reader — oto only pulls bytes when feeding the device, so the byte-offset is the audible position (modulo read-ahead). A 100 ms sampler logged offset + IsPlaying() across Play → Pause(1.5 s) → Play:

offset @ Play-end        :  96000 bytes  (2000 ms)   advancing
offset @ Pause+150ms     :  96000 bytes              frozen
offset @ Pause-end(1.5s) :  96000 bytes              still frozen
offset @ Resume-end      : 168000 bytes  (3500 ms)   continuous from 96000
pause-window delta       :      0 bytes / 0.0 ms
resume-window delta      :  72000 bytes / 1500 ms, no reset, no gap

LBC-4 PASS — froze inside the first 100 ms window; delta exactly 0 bytes (tighter than "within ~1 buffer"). IsPlaying() flipped within one sample.
LBC-6 PASS — resumed from the frozen 96000 offset; no restart, no backward jump, no bleed past the pause.
Env was not a blocker: oto fetched over network, CoreAudio opened (OTO_CONTEXT_READY), build/run exit 0. Artifacts in a temp dir; nothing written to the repo.

## 6. Caveats & gaps
- Read-ahead (not a contradiction): oto reads ahead in ~500 ms quanta, so the counter sits up to ~1 buffer ahead of audible position — exactly why the claim says "within ~1 buffer." Pause-delta of 0 confirms prompt stop.
- CoreAudio-CGo cells are tier-3/4: raw-CoreAudio rows rest on an Apple Forum thread + a dated blog (canonical AudioToolbox HeaderDoc 404'd). Conclusion (CGo required, most glue) is structurally certain; exact API wording isn't tier-1 hardened. Doesn't move the pick.
- miniaudio license tag NOASSERTION (public-domain OR MIT-0) — unambiguously not GPL; exact text not fetched. Moot.
- No latency/CPU benchmark of the four (out of scope; phase-one isn't real-time). If latency becomes a driver, separate spike.
- Resampling: oto resamples 24 kHz → device rate internally; probe ran clean against a 48 kHz device, but by-ear quality on real Kokoro output is a /verify step for the build ticket.

## 7. Acceptance-criteria mapping
AC: "Research/decision only; deferred until CGo renderer phase begins." → ANSWERED, and the premise is overturned: true pause/resume is achievable now, in-process, no CGo via oto v3 (device-confirmed). The deferral condition no longer holds. Research/decision output only — no implementation done.

## 8. Sources
oto v3 godoc/README (v3.4.0, 2025-10-04) · malgo godoc (v0.11.25) · portaudio godoc (v0.0.0-20260203164431) · raw CoreAudio: Apple Forums 117962 + kaniini blog (tier-3/4) · on-device probe 2026-06-27 · spike #84, decision 2026-06-26.

---

## Inline decision mark (harvested by decision-journal)
**Decision (v2) — architecture: accepted** — listen-path true Pause/Resume via github.com/ebitengine/oto/v3 in-process PCM player (no CGo on macOS, Apache-2.0), decoupled from and NOT gated on the deferred sherpa-onnx CGo renderer phase. Supersedes the #92 premise and extends decision 2026-06-26-afplay-sigstop-sigcont-no-true-pause.md (which proved OS-signal pause impossible and named the CGo renderer as the revisit trigger — that trigger is now resolved without CGo). Device-confirmed: pause-delta 0 bytes, resume from frozen offset.
