# oto v3.4 Player.Close() is a no-op (finalizer teardown) — overturns the planned "Close() halts the read-pull before fd close"; spike halts via Pause(), fd-lifecycle fix carried to #101

| Field    | Value            |
|----------|------------------|
| Date     | 2026-06-27       |
| Status   | accepted         |
| Category | architecture     |
| Tags     | listen-path, oto, ebitengine, oto-v3.4, player-close, finalizer, teardown, fd-lifecycle, read-from-closed-fd, pause-then-close, sa1019, go-memory-safe, accepted-debt, spike, issue-100, issue-101 |

## Context

Build session for spike #100 (oto v3 listen-path wiring + true Pause/Resume by-ear,
behind a `//go:build oto` tag). The planner-task.md for #100 mandated a specific
per-block teardown ordering, derived from the premise of the accepted decision
`2026-06-27-true-pause-via-oto-v3-no-cgo-in-process-player.md` that oto's player owns a
goroutine pulling the `io.Reader`-of-PCM and that closing the player would deterministically
stop that read-pull. The plan (planner-task.md lines 60, 83–85, 196–198, 292–293) was
explicit: on every block transition call `player.Close()` **FIRST** (to stop oto's goroutine
reading the source), **THEN** `file.Close()`, with the per-block `os.File` close tied to
`player.Close()` so exactly one fd is closed and oto's goroutine can never read a closed fd.

During the build that lifecycle invariant was found to be **false against the resolved
dependency**. In `github.com/ebitengine/oto/v3` **v3.4.0**, `Player.Close()` is documented as
"does nothing and always returns nil" — it is deprecated, and real teardown was moved to a
runtime **GC finalizer** on the player. Consequences: (1) `Close()` does NOT stop the
read-pull, so the planned ordering buys nothing; (2) calling the deprecated method trips
`SA1019`; (3) the source reader (the `LimitReader`→`*os.File`) stays alive until the player's
finalizer runs at some later GC, so the backing fd is **not guaranteed to outlive the player**.

This is the single deliberate deviation from planner-task.md v2, flagged by the build report
(implementation-build.md follow-up #1) and corroborated in review by both the Error Handling
inspector and the Tech Debt Sentinel (review-findings.md, "Corroborated Findings" — highest
signal item). The Tech Debt reviewer explicitly recommended the build-session harvest record it
as tracked accepted debt so #101 inherits it as evidence, not rediscovery.

## Options considered

### Option A: keep the planned `player.Close()`-first teardown
- **Pros**: matches the plan as written; single documented ordering.
- **Cons**: impossible against oto v3.4 — `Close()` is a no-op, does not stop the read-pull,
  and trips SA1019. Would have to be `//nolint`-suppressed to even compile cleanly, while
  buying no actual halt. Rejected: the API reality removed it.

### Option B (spike, CHOSEN): `player.Pause()` then `file.Close()`
- **Pros**: `Pause()` is the v3.4-correct deterministic halt — it removes the player from
  oto's active mux set so a paused player does not pull; safe by ear at a block transition we
  are already tearing down. Avoids the deprecated call entirely (no SA1019, no nolint —
  `golangci-lint --build-tags oto` is 0 issues). Viability-grade, which is the spike bar.
- **Cons**: `Pause()` does not provably join an in-flight source `Read` on oto's internal
  goroutine before returning, and the player object stays alive until its finalizer runs — so
  there is a benign **read-from-closed-fd window** and the fd is not guaranteed to outlive the
  player. Critically Go-memory-safe: a read on a closed `*os.File` returns `os.ErrClosed`, never
  UB/crash — worst case a swallowed error or a sub-millisecond artifact at teardown.

### Option C (deferred to #101): finalizer-aware fd ownership
- **Pros**: production-correct. Hand oto an `io.ReadCloser` it owns (close-on-finalize), or
  retain the reader until the player is collected, so the fd can never be closed out from under
  a not-yet-finalized player.
- **Cons**: out of scope for a throwaway by-ear spike; belongs to the #101 single-path redesign
  that collapses the build-tag pair.

## Decision

The original lifecycle invariant — "`player.Close()` halts oto's read-pull before the backing
`os.File` is closed" — is **overturned** by oto v3.4 making `Player.Close()` a documented no-op
with teardown moved to a GC finalizer. The #100 spike halts deterministically via
`player.Pause()` before `file.Close()` (Option B), accepted as spike-grade debt because the
residual read-from-closed-fd window is Go-memory-safe (`os.ErrClosed`) and benign at a
transition we are already tearing down. The production-grade fix (Option C — finalizer-aware fd
ownership so the fd always outlives the player) is **carried to #101** as tracked accepted debt,
not fixed in the spike. Marker in code: `cmd/narrate/listen_oto.go:118`
(`// TODO(#101-followup): redesign teardown around oto v3.4's finalizer lifecycle …`).

This is recorded as a follow-on **amendment/finding** to the accepted oto v3 decision, not a
supersession: the parent decision's core choice (oto v3 as the zero-CGo in-process PCM player
with library-managed Pause/Resume) stands and is device-confirmed. Only the teardown-ordering
sub-invariant assumed by the plan is corrected here, against the as-resolved v3.4.0 API.

## Consequences

- #101 (productionize / collapse the build-tag pair / make oto the default listen engine) must
  redesign teardown around the finalizer lifecycle — give oto an `io.ReadCloser` it owns, or
  retain the reader until the player is collected — so a block's fd is never closed under a
  not-yet-finalized player. This decision is the evidence #101 inherits.
- Every navigated block leaves a not-yet-collected `oto.Player` (each holding a buffer) until
  GC. Immaterial for a few-block spike; material for the #101 single-path redesign.
- The deprecated `Player.Close()` must continue to be avoided outright (not nolint-suppressed);
  `Pause()` is the correct halt primitive on v3.4.
- `-race` covered only the afplay seam (`listen_test.go` is now `//go:build !oto`); the oto
  loop's teardown safety is by-construction + by-ear, not race-tested — a known coverage gap
  for #101 to close.

## Related decisions

- [Listen-path true Pause/Resume via ebitengine/oto v3 in-process PCM player — no CGo](2026-06-27-true-pause-via-oto-v3-no-cgo-in-process-player.md) — parent decision this amends; its oto-v3 choice stands. This finding corrects the teardown-ordering sub-invariant the #100 plan derived from it, against the as-resolved oto v3.4.0 API (Close() is a no-op; teardown is finalizer-driven).

## Experiments

Build-verified during #100 (implementation-build.md, review-findings.md):
- `go build -tags oto ./cmd/narrate` and `CGO_ENABLED=0 go build -tags oto ./cmd/narrate` —
  PASS (zero-CGo premise holds; oto v3.4.0 + purego v0.9.0, both Apache-2.0).
- `golangci-lint run --build-tags oto ./cmd/narrate` — 0 issues, confirming nothing deprecated
  (SA1019) is called or suppressed; the deprecated `Player.Close()` is avoided via `Pause()`.
- `go test -race ./cmd/narrate` — PASS, but exercises only the `!oto` afplay seam; the oto
  teardown path is not race-covered.
- By-ear acceptance (freeze-at-sample / resume-from-sample, no restart/bleed across the
  Pause-then-close transition) deferred to a `/verify` session — needs tty + CoreAudio device.

## Revisit trigger

When #101 begins (productionize the listen path / collapse the build-tag pair): redesign oto
teardown around the v3.4 finalizer lifecycle so the backing fd is guaranteed to outlive the
player (oto owns an `io.ReadCloser`, or reader retained until player collected). Also revisit if
a future oto major restores a synchronous `Close()` that joins the read-pull — that would let
the original "Close()-first" ordering return and retire the Pause-then-close workaround.
