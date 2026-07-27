# GSO warm-load proof is a correctness oracle (A,B,A determinism + warm-vs-cold), buffered before overwrite, guarded by a negative dry-check — not a latency check

| Field    | Value            |
|----------|------------------|
| Date     | 2026-07-27       |
| Status   | accepted         |
| Category | convention       |
| Tags     | gso, gpt-sovits, warm-load, ac5, correctness-oracle, determinism, warm-vs-cold, rms-tolerance, mps, read-and-buffer, content-addressed, negative-dry-check, false-green, test-methodology, issue-161 |

## Context

Issue #161's single novel risk (AC5) is proving that ONE warm `TTS_infer_pack` pipeline object serves N sequential `.run()` calls *correctly* — not just faster. The runbook had only ever run one `.run()` per fresh process, so warm reuse might silently corrupt request 2+ via cached/leaked ref-audio state. The first plan gated AC5 on a latency assertion (request-N vs request-1 wall-time). Plan review round 1 rejected that as near-tautological (request-1 always pays the one-time model load) and "outputs differ per input" as necessary-but-not-sufficient — neither detects cross-request state corruption. The correctness oracles that replaced it then collided with the worker's content-addressed output path (from the wire-contract decision): two identical requests mint the SAME path and the second `.run()` atomically overwrites the first, so a naive "re-read `out_A1` after the run" degenerates to `assert x == x` — a **false green** on the exact warm-state corruption AC5 exists to catch (round-2 blocking finding).

## Options considered

### Option A: Latency gate (request-N faster than request-1)
- **Pros**: Trivial to write.
- **Cons**: Near-tautological — request-1 always pays one-time model load, so warm amortization is guaranteed and proves nothing about correctness. Rejected in round 1; demoted to an informational perf note.

### Option B: Correctness oracles that re-read the output path after the run
- **Pros**: Compares actual audio, exercises the warm path.
- **Cons**: The worker's content-addressed idempotent overwrite means A1 and A2 mint the same file, so A2 clobbers A1 before the compare — a vacuous self-comparison that false-greens. Actively encoded the bug (round-2 blocking).

### Option C: Nonce the warmproof output paths so A1/A2 differ
- **Pros**: Two files coexist, no clobber.
- **Cons**: Requires a worker/wire change or a test-only nonce that diverges from the frozen content-addressed behavior. Rejected as the less clean of the two fixes.

### Option D (chosen): Read-and-buffer each response before the next request, plus a negative dry-check
- **Pros**: Harness-only fix; the worker's content-addressing/overwrite stays intact and correct for dedup; establishes a correct happens-before that closes the self-compare window; the negative dry-check proves the oracle can fail before it is trusted to pass.
- **Cons**: The harness must never re-read a content-addressed path after a later identical request, or it re-introduces the false-green (guarded by the negative dry-check as a regression tripwire).

## Decision

The AC5 warm-load proof (`make gso-warmproof`) is a **correctness oracle, not a latency check**, built on this standing methodology:

- **Repeat-input determinism (primary oracle):** feed inputs in order **A, B, A** to one warm worker; assert `bytes(A1) ≈ bytes(A2)` within an **RMS/spectral tolerance** (seeded run, not byte equality — MPS is not guaranteed bit-exact). This catches request-2+ corruption from request-1's cached/leaked state.
- **Warm-vs-cold equivalence (secondary oracle):** compare the genuinely-warm Nth call (`pcm_a2`, the 3rd request on the warm worker) against a fresh cold single-shot process on the same input, within the same tolerance. The warm and cold processes MUST use **distinct `GSO_OUT_DIR`s** so identical input cannot clobber across processes on the shared content-addressed name.
- **Read-and-buffer-at-OK-time discipline (the load-bearing fix):** because the worker mints a content-addressed path and idempotently overwrites, the harness **reads and buffers each `OK <out>`'s WAV bytes into memory the instant that response line arrives, BEFORE feeding the next request.** It compares buffered bytes and never re-reads a path after a subsequent request. This relies on the worker emitting `OK <out>` only after the atomic write completes, giving a correct happens-before that closes the vacuous `x ≈ x` self-compare window. The worker's content-addressing + idempotent atomic overwrite is deliberately kept as-is (correct for dedup, not weakened); the fix is entirely in the harness.
- **Negative dry-check (oracle self-test):** run the same determinism oracle against a deliberately **state-leaking fake worker stub** whose second identical response returns different bytes, and assert the oracle goes **RED**. A green determinism result is only trustworthy once the oracle has demonstrated it *can* detect divergence. This runs torch-free against a stub, so it is fully exercisable in a code-only session even before real artifacts exist. It also serves as the regression tripwire: if a future edit silently reduces the oracle to a self-comparison, the negative dry-check stops going RED and the harness fails.
- **Latency is informational only**, printed as a perf note, never a pass/fail gate.

Honesty boundary: the negative dry-check and the whole contract/parse/ERR layer are provable in-session; AC5's *actual green* (real warm run) and AC1's ~950MB fetch are runtime-gated on real checkpoints + a working `.venv-gso` on M1 Pro and must be reported "pending machine run," never green from code inspection.

## Consequences

- Warm-reuse correctness is provable and the proof is repeatable (scripted harness, not a one-off manual run).
- The negative dry-check is a durable guard: it was independently re-run green by both the builder and the reviewer (RED on a subtle near-threshold leak at RMS 2.01e-03 vs tol 1e-03, GREEN on sub-threshold jitter 2.14e-04, RED on gross leak, RED on stale-cache distinctness), demonstrating the oracle discriminates rather than rubber-stamps.
- A future engineer must not "simplify" the harness by re-reading a content-addressed path after a later identical request; the negative dry-check exists precisely to trip on that regression.
- The correctness-over-latency stance aligns with the honesty rule: a machine-gated result is deferred, not faked.

## Related decisions

- [GSO worker wire contract is RVC-shaped but NOT verbatim](../architecture/2026-07-27-gso-worker-wire-contract-rvc-shaped-not-verbatim.md) — the content-addressed idempotent-overwrite output path this harness must read around; the buffer-before-next-request discipline exists because of that dedup behavior.

## Revisit trigger

Revisit if the worker's output-path scheme changes away from content-addressed idempotent overwrite (the buffer-before-overwrite discipline may no longer be needed), if MPS/torch determinism guarantees change (the RMS/spectral tolerance could tighten toward byte equality), or if the AC5 real warm run reveals that warm reuse corrupts request 2+ (the correct response is hold + re-plan #162/#163, not a silent workaround).
