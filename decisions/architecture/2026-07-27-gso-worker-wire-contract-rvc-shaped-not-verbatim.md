# GSO worker wire contract is RVC-shaped but NOT verbatim — worker-minted content-addressed path, direct .ckpt/.pth, 32 kHz

| Field    | Value            |
|----------|------------------|
| Date     | 2026-07-27       |
| Status   | accepted         |
| Category | architecture     |
| Tags     | gso, gpt-sovits, wire-contract, public-api, subprocess-protocol, worker-minted-path, content-addressed, err-taxonomy, exit-codes, 32khz, ckpt-pth, frozen-v1, issue-161, issue-162 |

## Context

Issue #161 (root/unblocked of the GSO initiative #161–164) builds `scripts/gptsovits_worker.py`: an ephemeral, warm-load-once torch subprocess around `TTS_infer_pack` that the future Go renderer (#162) drives over stdin/stdout. The natural move was to reuse the already-accepted RVC worker wire contract (`2026-07-22-rvc-worker-wire-contract-err-taxonomy-exit-codes`) so #162 could bind to a familiar surface. Plan review round 1 caught that the plan's original wording ("reuse RVC's v1 shape verbatim") was factually wrong: RVC's request carried a caller-supplied `<out>` token, but GSO's AC2-locked request line has none. The contract needed to be frozen at #161 close (public-api-change overlay) so #162 could bind to it, which forced an explicit statement of exactly where GSO follows RVC and where it deliberately diverges.

## Options considered

### Option A: Reuse the RVC v1 line verbatim
- **Pros**: One contract, zero divergence for #162 to track.
- **Cons**: Factually impossible — RVC's `<in> <out> <voice> <index_rate> <pitch>` line does not fit GSO. GSO has no `<out>` (AC2 line is `<text> <ref_audio_path> <prompt_text> <text_split_method> <voice_id>`), GSO's tokens are free-text (spaces), and GSO emits a different rate. Freezing a wrong "verbatim" claim would mislead #162.

### Option B: A brand-new, unrelated GSO contract
- **Pros**: Freedom to design from scratch.
- **Cons**: Throws away the proven RVC framing (line loop, one-response-per-request, closed ERR taxonomy, FATAL exit split), forces #162 to learn two unrelated protocols, and re-litigates already-settled error/exit semantics.

### Option C (chosen): RVC-SHAPED but explicitly GSO-specific, frozen as v1
- **Pros**: Reuses RVC's proven *shape* while naming the three deliberate divergences so #162 binds to an accurate, frozen contract.
- **Cons**: #162 must track that GSO ≠ RVC on three specific points; the "shaped, not verbatim" distinction must be documented or a future engineer will re-introduce the verbatim error.

## Decision

The GSO worker wire contract **reuses the RVC v1 *shape*** — stdin line loop, positional append-only tokens, exactly one physical response line per request, the CLOSED append-only ERR taxonomy `bad-args | bad-voice | read-failed | infer-failed | write-failed`, and the startup/runtime FATAL exit split (78 startup / 70 runtime; native/uncatchable abort → process dies with no line, #162 treats as fatal hard-stop) — but **deliberately diverges from RVC in three GSO-specific ways**:

1. **The worker MINTS its own output path** (RVC took a caller-supplied `<out>`). AC2's request line carries no `<out>` token, so the worker resolves an output base dir once at startup (`GSO_OUT_DIR` env, else a per-process `mkdtemp()`), derives a **content-addressed** filename `gso-<voice_id>-<sha256(canonical request tuple)[:16]>.wav`, and returns it in `OK <out>`. Identical requests map to the same path → an **idempotent atomic overwrite** (temp-write + `os.replace`) is correct and intended. `hexdigest[:16]` is 64 bits (birthday-bounded ~2³²), so distinct requests "won't collide in practice at this scale" — not a hard "cannot." The **worker never deletes** minted files and holds no TTL; **#162 owns the full cleanup/TTL lifecycle** (mirrors the ephemeral-sink ownership model). Consequence: the worker is a **de-facto content-addressed cache** — identical requests resolve to ONE physical file, so #162 must NOT assume one file per `.run()` call.
2. **It consumes `.ckpt`/`.pth` directly, never ONNX** (RVC ran a torch-free ONNX pipeline). No ONNX export step exists for GSO; torch is asserted PRESENT at startup (the inverse of RVC's torch-free assertion), reached only across the subprocess boundary.
3. **It emits a third system sample rate, 32 kHz** (Kokoro 24 kHz, RVC its own rate). CLAUDE.md forbids resampling, so #162/#163 roster/format metadata MUST carry a per-voice/per-engine sample rate — 32 kHz cannot be assumed away.

A fourth practical difference: RVC's five tokens were all space-free, so `shlex.split` was incidental; GSO's `<text>` and `<prompt_text>` are natural-language strings with spaces/punctuation, making **shlex-quoting load-bearing** on the wire. Go has no stdlib shlex quoter, so #162 must implement/vendor one and validate it against the shared golden fixture (`scripts/testdata/gso-shlex-golden.json`) shipped in Phase 4.

**This is the frozen v1 contract #162 binds to.** Evolution is constrained to trailing-optional-token-with-a-proven-default; the ERR category set is append-only; no reorder/repurpose. Documented as "v1" in the worker docstring exactly as `rvc_worker.py` does.

## Consequences

- #162 gets a stable, accurate contract, but must respect three GSO-specific facts RVC did not have: worker-minted paths (#162 owns cleanup), content-addressed dedup (not one file per call), and 32 kHz in roster metadata.
- Freezing v1 at #161 close means any later capability (per-request sample rate, non-`cut4` splitting) is an additive trailing token, never a breaking change.
- The "shaped, not verbatim" framing must survive in the docstring/decision record or a future engineer will re-introduce the round-1 verbatim error.
- Build review round 2 re-confirmed the frozen wire v1 contract, closed ERR taxonomy, 78/70 split, and content-addressed dedup are byte-unchanged from the reviewed build.

## Related decisions

- [RVC worker stdin/stdout wire contract — closed ERR taxonomy + startup/runtime FATAL exit-code split](../architecture/2026-07-22-rvc-worker-wire-contract-err-taxonomy-exit-codes.md) — the RVC precedent whose *shape* this reuses; GSO diverges on worker-minted path, direct `.ckpt`/`.pth`, and 32 kHz.
- [Torch-free ONNX RVC via an ephemeral per-job worker](../architecture/2026-07-22-torch-free-onnx-rvc-ephemeral-worker.md) — RVC is torch-free ONNX; GSO is the inverse (torch present, `.ckpt`/`.pth` direct) behind the same subprocess boundary.
- [GSO wire-vs-packaged ref/prompt precedence + drift rule](../architecture/2026-07-27-gso-wire-vs-packaged-ref-prompt-precedence-drift.md) — sibling #161 decision governing the two ref-data homes this contract's `<ref_audio_path>`/`<prompt_text>` tokens create.

## Revisit trigger

Revisit if the ticket owner edits AC2 to drop `<ref_audio_path>`/`<prompt_text>` for a single-source-of-truth wire (re-freezes the contract), if a second GSO voice or a v4/48 kHz upgrade lands, or if #162 needs a per-request sample-rate or non-`cut4` split (each a new trailing optional token, not a reorder).
