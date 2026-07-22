# RVC worker stdin/stdout wire contract — closed ERR taxonomy + startup/runtime FATAL exit-code split

| Field    | Value            |
|----------|------------------|
| Date     | 2026-07-22       |
| Status   | accepted         |
| Category | architecture     |
| Tags     | rvc, voice-conversion, wire-contract, public-api, subprocess-protocol, err-taxonomy, exit-codes, ex-config, ex-software, single-line-err, retryable-vs-fatal, render-decorator, issue-144, issue-145 |

## Context

Issue #144 productionizes the piloted torch-free ONNX RVC pipeline into an ephemeral per-job Python worker (`scripts/rvc` + `scripts/rvc_worker.py`). The worker is not a leaf — it is the **shared engine the future Go `render.Renderer` decorator (#145) drives as a subprocess**. #144 therefore defines a public wire surface that #145 binds to, flagged with a `public-api-change` overlay (Consumer Inventory, Versioning Policy, Contract Tests in the plan). The top-level architecture decision `2026-07-22-torch-free-onnx-rvc-ephemeral-worker` settled *that* we run an ephemeral torch-free worker wrapped as a decorator, and why; it did **not** specify the byte-level contract across the subprocess boundary. Round-1 review (B3, S2, S4) forced that contract to be exact: multi-line `ERR` broke line framing, the retryable-vs-fatal signal needed a machine-parseable token, and catchable fatals needed to be distinguishable from per-block failures.

## Options considered

### Option A: free-text ERR strings, single generic nonzero exit
- **Pros**: simplest to emit.
- **Cons**: #145 would have to parse free English to decide retry-vs-abort; multi-line exception messages break `readline` framing (response count ≠ request count); no way to tell a transient bad block from a dead worker. Rejected.

### Option B: structured line protocol — closed category taxonomy + single-line framing + split fatal exit codes — CHOSEN
- **Pros**: #145 branches on one token, never on prose; one physical line per request keeps `readline` framing total; the exit code alone separates "misconfigured environment, don't retry" from "runtime software fault".
- **Cons**: the category set and exit codes are now a frozen v1 surface that must stay append-only.

## Decision

The worker's stdin/stdout protocol is a **v1 contract #145 binds to**, defined as:

- **Request:** one line, `shlex`-split into EXACTLY 5 positional tokens — `<in> <out> <voice> <index_rate> <pitch>`. Positional and append-only forever (new capability = trailing optional token with an engine-faithful default; never reorder/repurpose).
- **Response:** EXACTLY ONE physical line per request, so `response count == request count`:
  - `OK <out>` — success; `<out>` echoes the RAW written path verbatim (newline/CR-bearing `<in>`/`<out>` are rejected up front as `ERR bad-args`, so the echoed name can never diverge from the file on disk).
  - `ERR <category> <message>` — recoverable per-line failure; the loop continues. `<message>` has all newlines/CRs collapsed to spaces and is capped (≤300 chars) so an `ERR` is always one line.
- **Closed `ERR` category taxonomy (v1):** `{ bad-args | bad-voice | read-failed | infer-failed | write-failed }`. The set is CLOSED for v1 and append-only; a v1 consumer treats an unknown category as fatal (the safe default). An internal off-contract category is coerced to `infer-failed` with a `[bug: …]` prefix so a worker bug can never put an off-contract token on the wire (holds even under `python -O`).
- **Fatal exit-code split (the retryable-vs-fatal contract #145 reads):**
  - **78 = `EXIT_FATAL_STARTUP` (EX_CONFIG)** — construction/environment fault before the loop: missing shared artifact, torch present, bad venv, malformed `RVC_SEED`. Signals "don't retry until the environment is fixed."
  - **70 = `EXIT_FATAL_RUNTIME` (EX_SOFTWARE)** — catchable runtime fatal (`MemoryError`, and construction faults raising the fatal exception class). Distinct from a per-block `ERR` — a `MemoryError` exits nonzero rather than degrading to `ERR`.
  - Uncatchable native faults (segfault) kill the process with a nonzero signal exit (not interceptable in Python; documented). `BrokenPipeError` on stdout and EOF are both clean exit 0.

The `fatal-vs-recoverable` seam is enforced in code: the fatal exception class is re-raised past every per-stage `except`, so a fatal never masquerades as a per-block `ERR`.

## Consequences

- #145 can implement its retry/abort policy against tokens and exit codes alone, never parsing English. Timeouts (60s/block, 10min/wall) stay the Go caller's job; the worker only streams `OK`/`ERR` and exits.
- The category set and the 78/70 codes are a frozen v1 surface — additive only. Adding a category or a 6th token must not break a v1-built #145.
- The protocol-loop contract test (mixed batch: both voices + bad-voice + blank + non-zero-pitch) is the executable contract: it pins arg order, `OK`/`ERR` grammar, one-physical-line framing, per-line survival, and warm-load-once — so #145 binds to a tested surface, not a prose description.

## Related decisions

- [Torch-free ONNX RVC via an ephemeral per-job worker, wrapped as a render decorator](2026-07-22-torch-free-onnx-rvc-ephemeral-worker.md) — the parent decision (the WHY/approach). THIS decision is the byte-level subprocess contract that parent leaves unspecified; #145 (the render decorator) is the first consumer of both.
- [RVC phase-one rejects non-zero pitch; line index_rate is authoritative](../tradeoff/2026-07-22-rvc-reject-nonzero-pitch-index-rate-authoritative.md) — the semantic scope of two of the 5 request tokens.

## Revisit trigger

Reconsider when #145 needs a capability the 5-token line can't carry (e.g. non-zero pitch / semitone transpose, segment windowing) — added as a trailing optional token, never a reorder — or when a real consumer beyond #145 appears and the closed category set proves too coarse. Any change must stay additive within v1 or bump the protocol version.
