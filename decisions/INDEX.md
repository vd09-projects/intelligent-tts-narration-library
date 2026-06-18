# Decision Index

<!--
  This file is maintained by the decision-journal skill.
  Entries are in YAML format for machine-friendly querying.
  Newest entries go at the top. Do not manually reorder.
-->

```yaml
decisions:
  - id: 2026-06-19-sink-receipt-planned-duration-not-wall-time
    title: "SinkReceipt.TotalDurationMs reports planned duration, not subprocess wall time"
    date: 2026-06-19
    status: accepted
    category: convention
    tags: [sink, ephemeral, receipt, telemetry, phase-one]
    path: convention/2026-06-19-sink-receipt-planned-duration-not-wall-time.md
    summary: "SinkReceipt.TotalDurationMs is summed from Plan.Timeline BlockTiming, not wall time around afplay. Wall time is contaminated by subprocess startup, scheduler jitter, and is zero under the test stub. Additive-compatible: add ActualDurationMs later if telemetry needs it."

  - id: 2026-06-19-sink-imports-render-for-renderresult
    title: "sink/ imports render/ for RenderResult and AudioStream"
    date: 2026-06-19
    status: accepted
    category: architecture
    tags: [sink, render, layering, dependency-direction, phase-one]
    path: architecture/2026-06-19-sink-imports-render-for-renderresult.md
    summary: "Direction stays a DAG: plan/ ← render/ ← sink/. Re-defining RenderResult/AudioStream in sink/ would fork the contract; hoisting them into plan/ would inflate the engine-neutral plan surface with audio bytes the planner never touches."

  - id: 2026-06-19-ephemeral-stubbed-play-seam-build-tag
    title: "Ephemeral sink default play seam is stubbed; real afplay is opt-in behind //go:build manual"
    date: 2026-06-19
    status: accepted
    category: convention
    tags: [sink, ephemeral, testing, build-tags, phase-one]
    path: convention/2026-06-19-ephemeral-stubbed-play-seam-build-tag.md
    summary: "Package-level play function variable is no-op in unit tests; real afplay lives in ephemeral_smoke_test.go behind //go:build manual. Build tags chosen over env vars because env-var gating is invisible in go test output."

  - id: 2026-06-18-empty-text-blocks-zero-ms-no-audioref
    title: "Empty-text blocks emit zero-duration timing with empty AudioRef"
    date: 2026-06-18
    status: accepted
    category: convention
    tags: [render, timeline, audioref, pause, phase-one]
    path: convention/2026-06-18-empty-text-blocks-zero-ms-no-audioref.md
    summary: "All-pause / no-speech blocks emit BlockTiming{StartMs==EndMs, AudioRef=''} and skip the subprocess. Empty AudioRef is the honest signal 'no audio for this block'; alternative (writing 44-byte empty WAV) hides data gaps from sinks. Pauses are sink-side phase one."

  - id: 2026-06-18-kokoro-wrapper-in-scripts-dir
    title: "Kokoro wrapper script lives in `scripts/`, not `render/sherpa/`"
    date: 2026-06-18
    status: accepted
    category: convention
    tags: [render, kokoro, subprocess, layout, phase-one]
    path: convention/2026-06-18-kokoro-wrapper-in-scripts-dir.md
    summary: "scripts/kokoro + scripts/kokoro_runner.py at project root; render/sherpa default BinaryPath='./scripts/kokoro'. Co-locating the Python launcher inside the Go package dir would break tooling conventions and imply Go owns the Python runtime."

  - id: 2026-06-18-voice-resolution-order
    title: "Voice resolution order: opts > plan defaults > backend default"
    date: 2026-06-18
    status: accepted
    category: convention
    tags: [render, voice, kokoro, phase-one]
    path: convention/2026-06-18-voice-resolution-order.md
    summary: "RenderOptions.Voice (errors on unknown) > Plan.Defaults.Voice (silent fallback on unknown) > 'af_bella'. PlanDefaults stays engine-neutral per CLAUDE.md — unknown hint must not error or the planner would be coupled to renderer voice ids."

  - id: 2026-06-18-per-block-wavs-no-concat-in-renderer
    title: "Per-block WAVs stay separate; renderer does not concatenate"
    date: 2026-06-18
    status: accepted
    category: architecture
    tags: [render, sink, audiostream, escalation, phase-one]
    path: architecture/2026-06-18-per-block-wavs-no-concat-in-renderer.md
    summary: "Engine.Render writes one WAV per block + manifest.txt; concatenation is sink concern. Required for RenderBlock to be truly surgical (swap one WAV) and to keep the ephemeral sink from having to split a monolithic file."

  - id: 2026-06-18-subprocess-timeouts-60s-10min
    title: "Phase-one subprocess timeouts: 60 s per-block, 10 min wall"
    date: 2026-06-18
    status: accepted
    category: convention
    tags: [render, subprocess, timeout, phase-one]
    path: convention/2026-06-18-subprocess-timeouts-60s-10min.md
    summary: "RenderOptions exposes PerBlockTimeout (default 60 s) and WallClockTimeout (default 10 min). Exceeded → sherpa.ErrTimeout wrapping context.DeadlineExceeded. Timeouts are errors, not refusals — honesty rule applies only to readable-but-unvoiceable source, not backend failure."

  - id: 2026-06-18-refused-block-message-rendered
    title: "Refused blocks render Refusal.Message through the same Kokoro path"
    date: 2026-06-18
    status: accepted
    category: convention
    tags: [render, refusal, honesty-rule, phase-one]
    path: convention/2026-06-18-refused-block-message-rendered.md
    summary: "Status==StatusRefused blocks feed Block.Refusal.Message to Kokoro like any voiced block; BlockTiming emitted with AudioRef. Empty Refusal.Message → ErrMalformedPlan (upstream bug, not a refusal). Earcons / silence / dropping the block all violate the honesty rule."

  - id: 2026-06-18-audiostream-on-disk-handle
    title: "AudioStream is an on-disk handle, not in-memory bytes"
    date: 2026-06-18
    status: accepted
    category: architecture
    tags: [render, audiostream, memory, sink, phase-one]
    path: architecture/2026-06-18-audiostream-on-disk-handle.md
    summary: "render.AudioStream carries Dir+Files+ManifestPath, not []byte. Avoids 50–200 MB resident audio for long docs and makes RenderBlock surgical (swap one file). Sinks read from disk; renderer never reads back its own output."

  - id: 2026-06-18-intelligence-adapter-lives-in-intelligence-pkg
    title: "IntelligenceAdapter interface lives in `intelligence/` package, not `planner/`"
    date: 2026-06-18
    status: accepted
    category: architecture
    tags: [intelligence, planner, interface, module-layout, phase-one]
    path: architecture/2026-06-18-intelligence-adapter-lives-in-intelligence-pkg.md
    summary: "Place IntelligenceAdapter interface in intelligence/ per design doc §3.2; planner depends on it; future concrete adapters (mcpsampling, anthropic) implement from sibling subpackages without circular deps; intelligence/ allowlist is only plan/."

  - id: 2026-06-18-planner-deps-invariant-checks-direct-imports
    title: "Planner deps invariant checks direct .Imports, not transitive -deps"
    date: 2026-06-18
    status: accepted
    category: tradeoff
    tags: [planner, invariant, dependencies, goldmark, testing, phase-one]
    path: tradeoff/2026-06-18-planner-deps-invariant-checks-direct-imports.md
    summary: "Scope planner/'s no-IO invariant test to direct .Imports because goldmark (sanctioned segmenter) transitively pulls in os/syscall/net/url; the CLAUDE.md invariant is about source-file imports, not unavoidable transitive deps."

  - id: 2026-06-18-two-oversized-split-thresholds
    title: "Two oversized-split thresholds: prose vs structured"
    date: 2026-06-18
    status: accepted
    category: convention
    tags: [planner, level, oversized-split, phase-one, heuristics]
    path: convention/2026-06-18-two-oversized-split-thresholds.md
    summary: "Encode separate constants — prose 20 lines / 800 chars; structured 70 lines / 2500 chars — because prose audio-coherence is the binding constraint for prose while structured content tolerates longer runs (clean seams). Diagrams are intentionally not split."

  - id: 2026-06-18-default-lexicon-shipped-frozen-overridable
    title: "DefaultLexicon shipped frozen + user-overridable via WithLexicon"
    date: 2026-06-18
    status: accepted
    category: convention
    tags: [planner, voice, lexicon, phase-one, user-override]
    path: convention/2026-06-18-default-lexicon-shipped-frozen-overridable.md
    summary: "Ship an opinionated baseline lexicon (arrows, paths, dev acronyms) plus a WithLexicon(extra) overlay so user entries win on key collision; empty default would produce silent silly voicings like '->' as 'dash greater than'."

  - id: 2026-06-18-segmenter-walks-top-level-ast-only
    title: "Segmenter walks top-level AST children only, not the full tree"
    date: 2026-06-18
    status: accepted
    category: tradeoff
    tags: [planner, segmentation, goldmark, source-map, phase-one]
    path: tradeoff/2026-06-18-segmenter-walks-top-level-ast-only.md
    summary: "Walk goldmark's top-level children only so plan.SourceMap line ranges stay contiguous and non-overlapping — block-level sync is the load-bearing invariant. Tradeoff: a fenced code block nested in a blockquote folds into the blockquote's text."

  - id: 2026-06-18-planner-classifier-sniff-order
    title: "Planner classifier — deterministic priority sniff order"
    date: 2026-06-18
    status: accepted
    category: convention
    tags: [planner, classify, segmentation, determinism, phase-one]
    path: convention/2026-06-18-planner-classifier-sniff-order.md
    summary: "Apply 9 rules first-match-wins: heading → list → image → table → fenced-code-subtype → plaintext pipe-table → plaintext config sniff → ASCII diagram → prose default. ClassExample reserved for future intelligence-driven reclassification. Each rule has a named test case."

  - id: 2026-06-18-plan-zero-deps-via-go-list-subprocess
    title: "Zero-deps invariant enforced via `go list -deps` subprocess"
    date: 2026-06-18
    status: accepted
    category: schema
    tags: [plan, zero-deps, testing, invariant, go-tooling]
    path: schema/2026-06-18-plan-zero-deps-via-go-list-subprocess.md
    summary: "Enforce the plan/ zero-internal-deps invariant via a `go list -deps` subprocess in plan/deps_test.go scoped by module-qualified import path; rejected in-process AST traversal because the natural library (golang.org/x/tools) would itself be a non-stdlib import."

  - id: 2026-06-18-plan-testdata-verbatim-from-design-doc
    title: "Testdata fixtures committed verbatim from design doc §2.7"
    date: 2026-06-18
    status: accepted
    category: schema
    tags: [plan, testdata, fixtures, schema, documentation-drift]
    path: schema/2026-06-18-plan-testdata-verbatim-from-design-doc.md
    summary: "Commit plan/testdata/ JSON fixtures verbatim from the design doc §2.7 examples (voiced config, refused image, composed full plan) so schema-doc drift is caught by round-trip test failure at PR review."

  - id: 2026-06-18-plan-id-ulid-stdlib-only
    title: "PlanID — ULID generated with stdlib only"
    date: 2026-06-18
    status: accepted
    category: schema
    tags: [plan, ulid, zero-deps, schema, honesty-rule]
    path: schema/2026-06-18-plan-id-ulid-stdlib-only.md
    summary: "Implement NewPlanID() inline using time+crypto/rand+encoding/binary only — keeps plan/ zero-deps. Trades same-millisecond monotonicity for the invariant; panics on rand failure rather than silently fabricating a weak ID."

  - id: 2026-06-18-kokoro-distribution-kokoro-onnx-over-kokoro
    title: "Kokoro distribution — kokoro-onnx over kokoro"
    date: 2026-06-18
    status: accepted
    category: library-choice
    tags: [tts, rendering, kokoro, onnx, dependency, phase-one]
    path: library-choice/2026-06-18-kokoro-distribution-kokoro-onnx-over-kokoro.md
    summary: "Use kokoro-onnx 0.5.0 (MIT pkg, Apache-2.0 weights) as the phase-one TTS runtime via a venv-backed subprocess wrapper; rejected the torch-based kokoro 0.9.4 (~2 GB) and a nonexistent precompiled binary."
```
