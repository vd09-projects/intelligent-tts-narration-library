# Decision Index

<!--
  This file is maintained by the decision-journal skill.
  Entries are in YAML format for machine-friendly querying.
  Newest entries go at the top. Do not manually reorder.
-->

```yaml
decisions:
  - id: 2026-06-20-mcpsampling-cache-key-includes-full-model-id
    title: "mcpsampling cache key includes the full chosen-model id"
    date: 2026-06-20
    status: experimental
    category: convention
    tags: [intelligence, mcpsampling, cache, cache-key, escalation, claude-md-rule, issue-13]
    path: convention/2026-06-20-mcpsampling-cache-key-includes-full-model-id.md
    summary: "CacheKey.Model is the full mcp-sampling@<clientID>/<actualModel> string. Two-phase lookup via per-clientID last-known-model map (sync.RWMutex). First call per clientID always misses; model switches invalidate; refusals never cached. Honors CLAUDE.md (content_hash, level, model) literally. Plan-review B1 fix."

  - id: 2026-06-20-mcpsampling-client-threaded-via-ctx
    title: "mcpsampling client threaded via ctx, not adapter constructor"
    date: 2026-06-20
    status: experimental
    category: convention
    tags: [intelligence, mcpsampling, mcp, server-session, ctx-threading, composition-root, pipeline, issue-13]
    path: convention/2026-06-20-mcpsampling-client-threaded-via-ctx.md
    summary: "SamplingClient interface + WithSamplingClient(ctx, client). *mcp.ServerSession satisfies it as-is. Keeps pipeline.New engine-neutral; avoids per-call pipeline rebuild and interface widening. ErrNoSamplingClient sentinel routes to internal_error: via classifier."

  - id: 2026-06-20-mcpsampling-refuse-sentinel-token
    title: "mcpsampling refusal sentinel — literal __REFUSE__ as the leading token"
    date: 2026-06-20
    status: experimental
    category: convention
    tags: [intelligence, mcpsampling, refusal, honesty-rule, llm-contract, issue-13]
    path: convention/2026-06-20-mcpsampling-refuse-sentinel-token.md
    summary: "Refusal contract: __REFUSE__ as the very first non-whitespace characters of the assistant reply, optional reason after one space. Boundary explicit (sentinel mid-body is content). Rejected stopReason matching (clients may ignore) and JSON-mode (overhead)."

  - id: 2026-06-20-mcpsampling-prompt-templates-stay-in-package-for-13
    title: "mcpsampling prompt templates stay inside the package for #13"
    date: 2026-06-20
    status: experimental
    category: convention
    tags: [intelligence, mcpsampling, prompts, templates, issue-13, issue-15, deferred-abstraction]
    path: convention/2026-06-20-mcpsampling-prompt-templates-stay-in-package-for-13.md
    summary: "DefaultPromptTemplates stays in intelligence/mcpsampling/prompts.go for #13. Lift to internal/intelligencetmpl when #15 (Anthropic direct-API) lands — file move + import rewrite. Avoids speculative abstraction with one consumer."

  - id: 2026-06-20-typed-enum-pattern-wins-for-all-enum-shaped
    title: "Typed-enum pattern wins for all enum-shaped string fields in plan/ (Severity, SayAs, Emphasis)"
    date: 2026-06-20
    status: accepted
    category: schema
    tags: [plan, enum, severity, sayas, emphasis, ssml, additive-compatible, refactor, issue-10, issue-13]
    path: schema/2026-06-20-typed-enum-pattern-wins-for-all-enum-shaped.md
    summary: "Adopt typed string alias + IsValid() pattern for Severity, SayAs, Emphasis — the three remaining freeform enum-shaped fields. Severity intentionally 2-valued; pipeline-stopping uses Go error per CLAUDE.md honesty rule. Wire format unchanged, additive-compat preserved. Rejected freeform-with-docs (Option B) and hybrid (Option C). Closes #10; validating use case #13."

  - id: 2026-06-19-runspeak-newpipeline-composition-seam
    title: "runSpeak composition seam — newPipeline factory hook"
    date: 2026-06-19
    status: accepted
    category: convention
    tags: [cmd/narrate-mcp, mcp, testability, composition-root, seam, factory, phase-one, issue-12]
    path: convention/2026-06-19-runspeak-newpipeline-composition-seam.md
    summary: "Package-level newPipeline var + narrator interface lets tests substitute the pipeline composition; verifies level/voice/locale wiring + temp-dir lifecycle without spawning Kokoro. Resolves build-review B2."

  - id: 2026-06-19-text-arg-transient-sentinel
    title: "text arg as transient sentinel — fast-error until ticket #17 lands"
    date: 2026-06-19
    status: accepted
    category: convention
    tags: [cmd/narrate-mcp, mcp, text-arg, transient-sentinel, honesty-rule, phase-one, issue-12]
    path: convention/2026-06-19-text-arg-transient-sentinel.md
    summary: "text arg stays in the schema for forward-compat; handler fast-errors with errTextNotImplemented until mcptext adapter (#17) lands. Honest contract over silent fallback."

  - id: 2026-06-19-mcp-tool-family-narrate-namespace
    title: "MCP tool family — narrate.* namespace"
    date: 2026-06-19
    status: accepted
    category: convention
    tags: [cmd/narrate-mcp, mcp, tool-naming, namespace, documentation, phase-one, issue-12]
    path: convention/2026-06-19-mcp-tool-family-narrate-namespace.md
    summary: "Tool family narrate.*; `speak` is the canonical entry point. README install snippet targets Claude Desktop's claude_desktop_config.json as canonical; mcp CLI is secondary."

  - id: 2026-06-19-mcp-error-classifier-caller-vs-internal-split
    title: "MCP error classifier — caller-error vs internal-error split"
    date: 2026-06-19
    status: accepted
    category: convention
    tags: [cmd/narrate-mcp, mcp, error-handling, classifier, honesty-rule, phase-one, issue-12]
    path: convention/2026-06-19-mcp-error-classifier-caller-vs-internal-split.md
    summary: "classifyPipelineErr splits caller-errors (fs.ErrNotExist, fs.ErrPermission, validation, text-arg, sink=persistent) from internal-errors (renderer/sink failure). Wire prefixes 'caller-error: invalid_argument:' and 'internal_error:' make the split observable in IsError content. Cancellation gets its own 'cancelled:' bucket."

  - id: 2026-06-19-mcp-speak-response-receipt-only-envelope
    title: "speak tool response envelope — receipt-only for v1"
    date: 2026-06-19
    status: accepted
    category: convention
    tags: [cmd/narrate-mcp, mcp, response-envelope, schema-version, phase-one, issue-12]
    path: convention/2026-06-19-mcp-speak-response-receipt-only-envelope.md
    summary: "Response v1 is receipt-only: {receipt: {blocks_played, total_duration_ms, out_dir}}. plan envelope deferred as additive future change under CLAUDE.md schema_version rule. Locked at plan-review v1 (B1) to prevent contract drift before build."

  - id: 2026-06-18-pipeline-composition-root-pattern
    title: "Pipeline composition root pattern"
    date: 2026-06-18
    status: accepted
    category: architecture
    tags: [pipeline, composition-root, cmd, mcp, phase-one, issue-7]
    path: architecture/2026-06-18-pipeline-composition-root-pattern.md
    summary: "pipeline.Pipeline is the only struct holding concrete edge instances; Narrate is the only public method; ctor takes interfaces so cmd/narrate and cmd/narrate-mcp reuse it without duplication. Rejected per-cmd wiring and global singleton."

  - id: 2026-06-18-cli-flag-taxonomy-named-only
    title: "cmd/narrate CLI flag taxonomy: named flags only"
    date: 2026-06-18
    status: accepted
    category: convention
    tags: [cmd/narrate, cli, cobra, flags, phase-one, issue-7]
    path: convention/2026-06-18-cli-flag-taxonomy-named-only.md
    summary: "--file required, --level {1|2|3} default 1, --sink {ephemeral|persistent} default ephemeral, --gender {female|male} default female. Named flags only — no positional args. Engine-neutral --gender maps to engine voice ids inside the renderer."

  - id: 2026-06-18-persistent-sink-deferred-fast-error
    title: "--sink=persistent deferred to phase 2 with fast-error"
    date: 2026-06-18
    status: accepted
    category: tradeoff
    tags: [cmd/narrate, sink, persistent, honesty-rule, phase-one, issue-7]
    path: tradeoff/2026-06-18-persistent-sink-deferred-fast-error.md
    summary: "Vertical slice rejects --sink=persistent fast with errPersistentNotImplemented and exit code 2. Rejected silent fallback to ephemeral. Honest contract beats silent fallback — extends the project's refusal-as-data discipline from the narration layer to the CLI surface."

  - id: 2026-06-18-single-canonical-demo-doc
    title: "Single canonical demo doc at docs/samples/sample.md"
    date: 2026-06-18
    status: accepted
    category: convention
    tags: [docs, samples, fixtures, demo, phase-one, issue-7]
    path: convention/2026-06-18-single-canonical-demo-doc.md
    summary: "One 561-word file covering prose + code + list + table + bare-image refusal serves the README quickstart, the manual smoke test, and the planner benchmark. Rejected a directory of per-class fixtures because drift across siblings is worse than a single concentrated example."

  - id: 2026-06-18-pipeline-manual-smoke-build-tag-gating
    title: "Pipeline manual smoke test gated by //go:build manual"
    date: 2026-06-18
    status: accepted
    category: convention
    tags: [pipeline, testing, build-tags, manual-smoke, phase-one, issue-7]
    path: convention/2026-06-18-pipeline-manual-smoke-build-tag-gating.md
    summary: "pipeline/pipeline_manual_smoke_test.go uses //go:build manual matching the sink-side pattern. Default `go test ./...` skips it; `go test -tags manual ./pipeline/...` runs it. Rejected env-var gating because env-var skips are invisible in `go test` output."

  - id: 2026-06-18-two-track-benchmark-methodology
    title: "Two-track benchmark methodology"
    date: 2026-06-18
    status: accepted
    category: convention
    tags: [pipeline, benchmark, performance, planner, phase-one, issue-7]
    path: convention/2026-06-18-two-track-benchmark-methodology.md
    summary: "BenchmarkNarratePlanner measures planner alone (gate 100 ms, landed 0.344 ms — 290× headroom). BenchmarkNarrateEndToEnd uses noop renderer + sink so pipeline overhead is observable separately. Rejected single end-to-end bench with real Kokoro because subprocess latency would mask planner regressions."

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
