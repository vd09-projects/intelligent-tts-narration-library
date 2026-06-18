# Two-track benchmark methodology

- **Date:** 2026-06-18
- **Status:** accepted
- **Category:** convention
- **Tags:** [pipeline, benchmark, performance, planner, phase-one, issue-7]
- **Source:** harvested from cmd-narrate-issue-7 build summary v1, decision mark v6

## Context

Issue #7's performance acceptance criterion is: planner alone < 100 ms, end-to-end < 10 s for a 500-word doc on a dev laptop. The planner part is bounded and verifiable in CI-like conditions (no I/O, no subprocess). The end-to-end number is dominated by the Kokoro subprocess and afplay — both highly variable and inappropriate for automated benchmark gating.

The question: one benchmark or two? A single end-to-end bench with real Kokoro would over-report variance (subprocess startup, scheduler jitter, ONNX model warm-up) and mask any planner regression behind the subprocess noise floor. A single planner-only bench would miss the wiring cost (adapter + composition).

## Decision

Two benchmarks in `pipeline/pipeline_bench_test.go`, both using `docs/samples/sample.md` as input:

- **`BenchmarkNarratePlanner`** — measures `planner.Plan(ctx, doc, request, nil)` alone. This is the perf gate (≤ 100 ms / op). Landed at 0.344 ms/op on Apple M1 Pro (~290× headroom).
- **`BenchmarkNarrateEndToEnd`** — composes `pipeline.Pipeline` with a noop renderer and a noop sink that return canned values without doing real work. Reports planner + adapter + pipeline wiring cost without subprocess overhead. Landed at 0.346 ms/op — pipeline overhead ≈ 2 µs.

The real-Kokoro end-to-end wall (the ≤ 10 s gate) is verified via the manual smoke test, not via a benchmark — its variance makes a benchmark assertion impractical.

## Rejected alternative

**Single end-to-end benchmark with real Kokoro.** The Kokoro subprocess latency per block (subprocess spawn + ONNX inference + WAV write) dominates total time. A planner regression that doubles planner time from 0.3 ms to 0.6 ms would be invisible against ~50 ms per Kokoro block. The cheap regression-catcher is the planner-only bench; the end-to-end smoke is a complementary signal, not a substitute.

> Two bench shapes prevent masking.

## Consequences

- Any future perf-critical work in the planner (e.g., the segmenter, the voicing lexicon, or the leveling logic) should rerun `BenchmarkNarratePlanner` before commit and capture the number in the closing note of its ticket.
- If pipeline overhead ever grows materially (say, > 1 ms / op) the gap between the two benches surfaces it. Today the gap is ~2 µs, which is wiring noise.
- When the intelligence adapter lands (phase 2/3), the planner-only bench becomes less representative because real intelligence calls add HTTP latency. At that point a third benchmark may be needed: `BenchmarkNarratePlannerWithFakeIntelligence` to measure planner + adapter dispatch without network. Defer until then.

## Related decisions

- Pipeline composition root pattern (`architecture/2026-06-18-pipeline-composition-root-pattern.md`) — the bench composes the same struct production uses.
- Single canonical demo doc (`convention/2026-06-18-single-canonical-demo-doc.md`) — both benches read this file.
