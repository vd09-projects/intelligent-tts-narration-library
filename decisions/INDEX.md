# Decision Index

<!--
  This file is maintained by the decision-journal skill.
  Entries are in YAML format for machine-friendly querying.
  Newest entries go at the top. Do not manually reorder.
-->

```yaml
decisions:
  - id: 2026-07-25-rvc-faithful-pipeline-but-synthetic-source-mismatch
    title: "RVC pipeline is objectively faithful to torch, but a Kokoro-synthetic source ≠ recognizable target voice by ear"
    date: 2026-07-25
    status: accepted
    category: tradeoff
    tags: [rvc, voice, by-ear, parity, kokoro, source-mismatch, index-rate, cool-jahns, issue-147]
    path: tradeoff/2026-07-25-rvc-faithful-pipeline-but-synthetic-source-mismatch.md
    summary: "#147 verify-by-ear: `narrate --voice cool-jahns` passed every objective signal (manifest voice=cool-jahns, 40 kHz mono s16le, 191 s non-silent) but by ear did NOT sound like Cool Jahns. Diagnosis exonerates all shipped code: the Go decorator only frame-aligns sub-ms; the torch-free ONNX worker reproduces the Applio torch path (full-pipeline log-mel corr 0.9824; per-stage net_g 0.999993 / contentvec 1.0 / rmvpe 1.0; worker-vs-torch on the exact clip corr 0.9824, f0 110 vs 109 Hz). The Applio TORCH reference on a Kokoro am_michael source ALSO doesn't sound like Cool Jahns — so the ceiling is upstream: cool-jahns was trained on REAL Jeremy Jahns speech and a synthetic Kokoro source is out-of-distribution. DECISION: ship the faithful pipeline as-is; keep index_rate 0.75 (by-ear sweep 0.75/0.90/1.00 did not recover character — NOT a roster bump); spin the character fix (retrain/validate on synthetic sources, real-speech source, or different source voice/model) to a follow-up. #147 code+docs done; by-ear character sign-off deferred. Sibling to the single-voice-flow-gate decision (this is exactly the per-voice character regression the gate deliberately does NOT catch)."
  - id: 2026-07-24-rvc-cloned-voice-output-not-redistributable
    title: "RVC voice cloned from a real person without a license: its converted output is not publicly redistributable — swap the parity voice"
    date: 2026-07-24
    status: accepted
    category: convention
    tags: [rvc, license, voice-clone, fixtures, redistribution, honesty, d0, parity, cool-jahns, issue-151]
    path: convention/2026-07-24-rvc-cloned-voice-output-not-redistributable.md
    summary: "Task #151 (RVC parity fixtures) hard pre-publish gate (D0): an RVC voice model cloned from a real, named person WITHOUT documented consent/license (cool-jahns, cloned from ~59 min of Jeremy Jahns; no license/consent/model-card under assets/rvc-models/, models only in a private HF backup) may NOT have its CONVERTED OUTPUT publicly redistributed. Because parity/test fixtures ARE converted-output-derived (the *_ref.wav is converted output; the *_logmel_target.npy is a derivative of it), fixtures cannot host cool-jahns-derived assets — which is why fixtures.sha256 was left empty. RULE: vet the voice-model license BEFORE hosting any converted-output-derived asset. CHOSEN pivot (a): SWAP the parity voice to a licensed/self-trained voice whose converted-output public redistribution is clearable, then host that bundle (keeps the zero-setup fresh clone). REJECTED as primary (b): a documented regenerate-locally-only fresh-clone path — loses the zero-setup property; retained only as a fallback. Load-bearing: governs all future RVC parity/redistribution/hosting choices and prevents a contributor from 'fixing' the empty fixtures.sha256 by publishing non-redistributable converted output. Relates to torch-free-onnx-rvc-ephemeral-worker (#143) and the unified-voice-roster (#156) decisions."
  - id: 2026-07-24-rvc-parity-single-voice-flow-gate
    title: "make rvc-parity is a single-voice FLOW gate, not a per-voice correctness oracle"
    date: 2026-07-24
    status: accepted
    category: convention
    tags: [rvc, parity, testing, flow-gate, single-voice, honesty, issue-151]
    path: convention/2026-07-24-rvc-parity-single-voice-flow-gate.md
    summary: "Task #151: the RVC parity gate proves the conversion PIPELINE reproduces byte-for-byte on a fresh clone, using ONE voice (PARITY_VOICES, a single source of truth) that exercises the whole path (source -> rvc-convert.sh torch reference -> ONNX worker -> log-mel compare). Other voices are documented-excluded via EXCLUDED_PARITY_VOICES, with a disjoint/coverage meta-assert cross-checked against an INDEPENDENT roster + a negative test, so no voice can be silently absent from both sets and the parity matrix cannot silently re-widen. Future voices do NOT each earn a parity fixture. ACCEPTED TRADEOFF: a voice-specific conversion regression that does NOT affect the parity voice will NOT trip the gate; it surfaces in the by-ear /verify of that voice — a deliberate, documented boundary (honesty rule), not a blind spot. Load-bearing: sets the scope contract for every future RVC voice and fixes the shape of the parity test. REJECTED a per-voice parity oracle (fixture set + gate runtime grow with the roster; conflates 'pipeline reproduces' with 'each voice correct'). Sibling to the cloned-voice-not-redistributable decision (which picks WHICH voice is the parity voice)."
  - id: 2026-07-23-unified-voice-roster-namespace
    title: "Voice-selection namespace: one unified named roster is the primary selector; --gender demoted to a deprecated alias"
    date: 2026-07-23
    status: accepted
    category: architecture
    tags: [rvc, voice-conversion, voice-roster, voice-selection, cli-namespace, voice-flag, gender-flag, deprecation, backwards-compat, buildrenderer, requires-worker, err-worker-missing, err-unknown-voice, manifest-provenance, kokoro, issue-156, issue-146]
    path: architecture/2026-07-23-unified-voice-roster-namespace.md
    summary: "Issue #156: --voice becomes the SINGLE primary selector over ONE flat named-voice roster spanning both engines (af-bella / am-michael -> Kokoro 24 kHz; cool-jahns / confident-neal -> RVC 40 kHz), each entry carrying {engine, format, requires_worker} metadata that pipeline.BuildRenderer reads as the single source of truth for engine+format. --gender is demoted to a DEPRECATED back-compat alias (female->af-bella, male->am-michael) via pipeline.SlugForGender, kept working with a deprecation notice; --voice wins when both are set. WHY: the previous gender+voice split leaked an engine mechanic into the CLI (--voice != \"\" treated as a synonym for 'RVC / 40 kHz', true only by coincidence of the phase-one roster) and left two knobs disagreeing on primacy. One roster removes the coupling and gives a single honest selector that surfaces per-voice cost (engine / kHz / needs-worker) in help text. Two error classes with DISTINCT timings, never a silent fallback: an unknown voice stops PRE-render (eager pipeline.IsVoice; pipeline.ErrUnknownVoice backstop); a requires_worker voice with an unavailable worker stops AT RENDER TIME (render/rvc decorator ErrWorkerMissing). RequiresWorker is metadata for help-tagging + the --listen 40 kHz re-key only — no up-front worker probe. Manifest provenance (extends D6): manifest.voice records the engine-native resolved id (af_bella/am_michael for Kokoro; cool-jahns/confident-neal for RVC) so the alias path and the explicit --voice path stamp the SAME value. REJECTED: keep the gender+voice split (leaks the voice!=\"\"-means-RVC coupling; two knobs disagreeing on primacy). CROSS-LINKS D1+D2 (2026-07-23-pipeline-hosts-buildrenderer-factory — the factory this roster feeds) and D6 (2026-07-23-rvc-manifest-voice-records-character-slug — provenance)."
  - id: 2026-07-23-pipeline-hosts-buildrenderer-factory
    title: "pipeline.BuildRenderer is the shared renderer-factory home; pipeline/ now imports the concrete engines"
    date: 2026-07-23
    status: accepted
    category: architecture
    tags: [rvc, voice-conversion, buildrenderer, renderer-factory, composition-root, pipeline, render-sherpa, render-rvc, concrete-engines, import-cycle, engine-neutral, planner-deps, plan-deps, issue-146, issue-145]
    path: architecture/2026-07-23-pipeline-hosts-buildrenderer-factory.md
    summary: "Decision D2 of #146: the shared BuildRenderer(rvcVoice) (render.Renderer, plan.AudioFormat, error) factory lives in pipeline/ (pipeline/build_renderer.go). As a result pipeline/ now imports the concrete render/sherpa AND render/rvc engines, where it was previously interface-only. WHY: three separate package-main binaries (cmd/narrate, cmd/narrate-mcp, cmd/narrate-server) need ONE shared factory behind the user-facing voice knob; CLAUDE.md names the composition root as '(pipeline/, cmd/)' and blesses it to know concrete edges; cmd/ already depends on pipeline/, and pipeline/ importing sherpa+rvc creates NO import cycle (neither imports pipeline/). rvcVoice=='' -> plain Kokoro + render.DefaultFormat() (24kHz, byte-identical to before); rvcVoice!='' -> render/rvc decorator + rvc.OutputFormat() (40kHz), target slug passed straight into rvc.Config.Voice (factory translates nothing). Returning the AudioFormat ALONGSIDE the renderer (D1) couples renderer-format and sink-expected-format by construction. REJECTED: (a) hosting in render/ root -> import cycle (render -> render/sherpa -> render); (b) a new render/renderbuild or internal/renderbuild package -> a THIRD concrete-engine-aware location, a strained reading of the 'only pipeline/ and cmd/ know concrete edges' invariant. SAFE: planner/ + plan/ engine-neutrality/I-O-free invariant is unaffected and stays machine-guarded by planner/deps_test.go + plan/deps_test.go (go list over the package's own direct Imports); the multi-perspective build review APPROVED this boundary. Complements Decision D6 (2026-07-23 manifest.voice); both derive format/voice from the single BuildRenderer origin."
  - id: 2026-07-23-rvc-manifest-voice-records-character-slug
    title: "manifest.voice records the RVC character slug for RVC renders (Option A), not the hidden Kokoro source"
    date: 2026-07-23
    status: accepted
    category: tradeoff
    tags: [rvc, voice-conversion, manifest, provenance, honesty-rule, withvoice, persistent-sink, checkstale, content-hash, cool-jahns, confident-neal, af-bella, am-michael, cli-narrate, narrate-server, issue-146, issue-145, issue-144]
    path: tradeoff/2026-07-23-rvc-manifest-voice-records-character-slug.md
    summary: "Decision D6 of #146 (blocking review item F1): for a persistent RVC render, manifest.voice records the RVC CHARACTER slug the user asked to hear (cool-jahns / confident-neal), NOT its hidden Kokoro SOURCE voice (af_bella / am_michael). Wired by selecting persistent.WithVoice(args.Voice) when args.Voice != \"\" (else the existing gender-derived Kokoro id) at the four persistent-sink WithVoice sites: CLI full render (main.go:223) + CLI PatchBlock (main.go:497); server Consume (narrate.go:106) + server patchBlock (main.go:910). Rationale: honest provenance — stamping af_bella on a cool-jahns file misreports the render as plain Kokoro. SAFE because CheckStale keys ONLY on content_hash (check.go:63) and never reads manifest.Voice, and intelligence caching keys on (content hash, level, model) — so recording the slug costs nothing in caching/staleness; patch.go:290-291 already writes patchedManifest.Voice from WithVoice, so only the passed argument changes, not the sink. Scope: persistent-sink paths only — MCP is exempt (ephemeral sink + NewWAVFile write no manifest.json; WithVoice is a documented no-op on NewWAVFile). Additive: manifest.voice gains a new value class, schema_version unchanged. REJECTED Option B (keep the Kokoro source, only reword the standing-order doc) — an honest-provenance regression for zero benefit since Option A is free of caching/staleness cost. Locked by a new manifest-provenance test (Phase 2 + per-root in 4/6) plus an F5 negative guard that voice=='' still records the gender-derived Kokoro id."
  - id: 2026-07-22-rvc-decorator-owns-voice-map-single-translation
    title: "RVC decorator owns the target->{Kokoro source, index_rate, pitch} map; translation happens exactly once"
    date: 2026-07-22
    status: accepted
    category: architecture
    tags: [rvc, voice-conversion, render-decorator, voice-map, single-source-of-truth, kokoro-source-voice, index-rate, pitch, resolveVoice, sherpa, render-options, phase-3-thin, open-question-2, issue-145, issue-146]
    path: architecture/2026-07-22-rvc-decorator-owns-voice-map-single-translation.md
    summary: "Resolves OQ#2 of the #145 render/rvc decorator: the target->source/index_rate/pitch translation happens in EXACTLY ONE place, the decorator, via a map in one file `render/rvc/voice.go`. A user-facing RVC target (cool-jahns / confident-neal) maps to (a) the Kokoro SOURCE voice the model was trained against (am_michael / af_bella), (b) per-voice index_rate (0.75 / 0.5), (c) pitch (always 0). The decorator OVERRIDES the inner renderer's RenderOptions.Voice to the mapped Kokoro source before calling inner.Render/RenderBlock (so sherpa's resolveVoice only ever sees {af_bella, am_michael}) and emits {target, index_rate, pitch} on the worker request line. Phase 3 (#146 CLI/MCP/server wiring) passes the RVC target slug straight into rvc.Config.Voice and translates NOTHING. REJECTED: (a) neither layer translates -> sherpa gets 'cool-jahns' and hard-errors; (b) both decorator AND #146 translate -> double-translation / source-target drift. Single-owner keeps source+target impossible to drift and Phase 3 thin; a new voice = one map entry. Promoted from open question to locked decision after round-1 plan review."
  - id: 2026-07-22-rvc-per-block-err-fails-whole-render-hard-stop
    title: "A per-block RVC worker ERR fails the whole Render in phase one (no per-block skip or degrade)"
    date: 2026-07-22
    status: accepted
    category: tradeoff
    tags: [rvc, voice-conversion, render-decorator, error-handling, honesty-rule, all-or-nothing, per-block-err, no-skip, no-degrade, uniform-format, timeline, 40khz, error-not-refusal, issue-145]
    path: tradeoff/2026-07-22-rvc-per-block-err-fails-whole-render-hard-stop.md
    summary: "How the #145 decorator reacts to a per-block worker ERR (closed set {bad-args|bad-voice|read-failed|infer-failed|write-failed}): ANY per-block ERR is a HARD ERROR that stops the entire Render (returned up the pipeline) — NOT a per-block skip, NOT a degrade back to the block's 24kHz Kokoro audio. Rationale: mixing a 24kHz block into an otherwise-40kHz timeline breaks the uniform Format/Timeline contract; silently substituting/fabricating a repaint violates the honesty rule (never fabricate — this is an ERROR, not a refusal, so errors stop the pipeline); bad-args/bad-voice specifically signal a decorator bug building the request line and must surface loudly. REJECTED: (a) skip the failed block -> non-uniform timeline + silent gap; (b) degrade to plain 24kHz Kokoro -> silent format/quality inconsistency + fabricated-quality signal. An RVC job is all-or-nothing at 40kHz; failures stop loudly carrying ERR <category> <message> as a fix hint. Aligns with the honesty-rule-at-the-subprocess-edge theme of the #144 worker decisions."
  - id: 2026-07-22-rvc-worker-wire-contract-err-taxonomy-exit-codes
    title: "RVC worker stdin/stdout wire contract — closed ERR taxonomy + startup/runtime FATAL exit-code split"
    date: 2026-07-22
    status: accepted
    category: architecture
    tags: [rvc, voice-conversion, wire-contract, public-api, subprocess-protocol, err-taxonomy, exit-codes, ex-config, ex-software, single-line-err, retryable-vs-fatal, render-decorator, issue-144, issue-145]
    path: architecture/2026-07-22-rvc-worker-wire-contract-err-taxonomy-exit-codes.md
    summary: "The #144 RVC worker's stdin/stdout protocol is the v1 public contract #145 (Go render decorator) binds to — the byte-level surface the parent torch-free-onnx-rvc-ephemeral-worker decision left unspecified. Request = one line, shlex-split into EXACTLY 5 positional append-only tokens `<in> <out> <voice> <index_rate> <pitch>`. Response = EXACTLY ONE physical line per request (response count == request count): `OK <out>` (echoes the RAW written path; newline/CR-bearing paths rejected up front so OK-name and file-on-disk can't diverge) or `ERR <category> <message>` (recoverable, loop continues; message newline-collapsed + ≤300 chars). CLOSED v1 category taxonomy {bad-args | bad-voice | read-failed | infer-failed | write-failed}, append-only, unknown-category = fatal (safe default); an off-contract internal category is coerced to infer-failed with a [bug:] prefix (holds even under python -O). Retryable-vs-fatal exit-code split: 78 EXIT_FATAL_STARTUP (EX_CONFIG — missing artifact / torch present / bad venv / malformed RVC_SEED, don't retry until env fixed) vs 70 EXIT_FATAL_RUNTIME (EX_SOFTWARE — catchable runtime fatal like MemoryError, distinct from a per-block ERR); uncatchable native fault = nonzero signal exit; BrokenPipe + EOF = clean exit 0. Fatal exception class re-raised past every per-stage except so a fatal never degrades to a per-block ERR. The protocol-loop contract test is the executable contract. REJECTED free-text ERR + single generic nonzero exit (forces #145 to parse English; multi-line ERR breaks readline framing; no transient-vs-dead signal)."
  - id: 2026-07-22-rvc-index-blend-reconstruct-big-npy-in-worker
    title: "RVC index-blend reconstructs big_npy in-worker from the .index (reconstruct_n + make_direct_map fallback), not the index_vectors.npy artifact"
    date: 2026-07-22
    status: accepted
    category: algorithm
    tags: [rvc, voice-conversion, faiss, ivfflat, index-blend, reconstruct-n, make-direct-map, big-npy, index-vectors-npy, engine-faithful, go-knn-deferred, issue-143, issue-144, issue-145]
    path: algorithm/2026-07-22-rvc-index-blend-reconstruct-big-npy-in-worker.md
    summary: "The #144 index-blend stage (§4.5b) needs source vectors big_npy[ix] for its k=8 IVFFlat neighbours, and #143 produced TWO ways to get them: the original faiss `.index` and a `reconstruct_n` dump `index_vectors.npy`. DECISION: the Python worker reconstructs big_npy IN-PROCESS from the original `.index` via faiss `reconstruct_n` (enabling `make_direct_map()` at load if the IVF index lacks a direct map) and does NOT load `index_vectors.npy` — engine-faithful to the pilot (the worker HAS faiss and searches the `.index` directly, so the vectors are already reachable) and it avoids loading a second large (~557MB) artifact into the warm worker. `index_vectors.npy` stays RESERVED for the deferred in-process Go kNN path (the parent decision's Option-D endgame) where Go cannot call faiss reconstruct. big_npy is held as warm read-only cross-call state per loaded voice; parity judged by full-pipeline log-mel corr ≥0.98 (not raw samples), so IVFFlat search not being bit-identical to torch is accepted noise. Documented fallback: load index_vectors.npy if a future faiss/index format drops direct-map support (still torch-free). REJECTED pointing the worker at index_vectors.npy — couples the worker to the artifact meant for the Go fallback; a future engineer must not 'simplify' the two big_npy sources by doing so."
  - id: 2026-07-22-rvc-reject-nonzero-pitch-index-rate-authoritative
    title: "RVC phase-one rejects non-zero pitch with a clean ERR (no transpose ships); the request line's index_rate is authoritative"
    date: 2026-07-22
    status: accepted
    category: tradeoff
    tags: [rvc, voice-conversion, pitch, transpose, index-rate, phase-one-scope, reject-not-ignore, honesty-rule, wire-contract, issue-144, issue-145]
    path: tradeoff/2026-07-22-rvc-reject-nonzero-pitch-index-rate-authoritative.md
    summary: "Fixes the semantic scope of two of the #144 worker's five request tokens (resolves OQ#1). Both phase-one voices run pitch 0 and no semitone-transpose DSP was piloted/tested, so a non-zero `<pitch>` is REJECTED with `ERR bad-args pitch must be 0 in phase one` — the transpose path is NOT shipped (a wrong pitch shift is a confident mis-render) and pitch is NOT silently ignored (silent no-op = a lie); the honesty rule applied at the subprocess edge (refuse cleanly, keep serving). Transpose becomes a FUTURE trailing optional token with an engine-faithful default so a v1 caller stays valid. Separately, the request LINE's `<index_rate>` is AUTHORITATIVE — the per-voice 0.75 (cool-jahns) / 0.5 (confident-neal) values are just what #145 is expected to PASS on the line, not a baked-in worker constant that overrides it; validated float in [0,1], out-of-range → ERR bad-args. REJECTED shipping an untested transpose path and REJECTED silently clamping pitch to 0. #145 binds to both rules."
  - id: 2026-07-22-torch-free-onnx-rvc-ephemeral-worker
    title: "Torch-free ONNX RVC via an ephemeral per-job worker, wrapped as a render decorator"
    date: 2026-07-22
    status: accepted
    category: architecture
    tags: [rvc, voice-conversion, onnx, onnxruntime, torch-free, ephemeral-worker, render-decorator, kokoro, sherpa, cgo-deferred, faiss, mmap, huginn, issue-143, issue-144, issue-145, issue-146, issue-147]
    path: architecture/2026-07-22-torch-free-onnx-rvc-ephemeral-worker.md
    summary: "Run the two trained RVC voices as a torch-free ONNX pipeline (onnxruntime+numpy+faiss) in an ephemeral per-job Python worker (load once per document, warm across blocks, EXIT → 0 idle RAM), wrapped as a Go render.Renderer decorator over sherpa; shared buildRenderer(voice) helper feeds CLI --voice + MCP voice arg; worker-unavailable → hard error (honesty), 40kHz end-to-end when RVC on. REJECTED: shelling Applio per-clip (cold start + faiss/torch OpenMP segfault), a warm daemon (~3GB idle), and fully in-process Go/onnxruntime-go (D) — same native lib so no speed gain over C, costs un-deferring CGo + a Go DSP reimpl; D deferred to ride the future sherpa-onnx-go CGo migration. Also rejected rvc-python/MLX/CoreML and voice-family swaps (knn-vc/seed-vc/FreeVC discard the trained voices). Piloted on M1: parity user-confirmed by ear (per-stage corr ≥0.9999, log-mel 0.982); ONNX cold 9.0s vs torch 15.4s, warm 6.9 vs 7.8s/clip, RAM 2.75-3.7 vs 4.5-5.9GB; keep-alive saves only ~2s/clip so exit-after-job wins. mmap demoted to optional flag. Follow-ups #143-#147; trail in research/rvc-voice-conversion-integration/."
  - id: 2026-07-10-comma-grouped-degroup-prepass-not-tokenizer
    title: "Comma-grouped degroup handled as a step-0 pre-pass, not by extending the maximal-run tokenizer"
    date: 2026-07-10
    status: accepted
    category: algorithm
    tags: [issue-139, planner, voicing, cardinal-spell-out, comma-grouped, spellNumbersInProse, commaGroupedRun, spellableRun, maximal-run-tokenizer, pre-pass, degroup]
    path: algorithm/2026-07-10-comma-grouped-degroup-prepass-not-tokenizer.md
    summary: "#139 voices well-formed US comma-grouped integers (24,700 -> 'twenty-four thousand seven hundred') that #138 left as digits. Implemented as a step-0 detect+degroup PRE-PASS at the head of digit-run handling, BEFORE the plain scan — a new pure validator commaGroupedRun matches \\d{1,3}(,\\d{3})+, strips commas to a clean digit string, then REUSES the existing spellableRun neighbour gate (anchored on the grouped span's start) + the 1-6 length gate (on the degrouped count) + spellNumberToken. Malformed/oversized/bad-neighbour spans fall through to the unchanged plain path and LEAVE; the >=7-digit LEAVE applies naturally to the degrouped count (1,000,000 -> LEAVE); the retained plain-path comma-far-digit rejection in spellableRun is the load-bearing partial-spell guard for 12,3456. REJECTED extending the maximal-run tokenizer loop to swallow commas — that couples the comma-vs-punctuation call with the decimal-dot lookahead in one switch. Keeps the scanning loop and spellableRun predicate comma-agnostic."
  - id: 2026-07-10-grouped-decimal-declined-wholesale-left-byte-identity
    title: "Grouped-decimal spans declined wholesale and left byte-identity; grouped-decimal voicing deferred, never half-implemented"
    date: 2026-07-10
    status: accepted
    category: tradeoff
    tags: [issue-139, planner, voicing, cardinal-spell-out, comma-grouped, grouped-decimal, commaGroupedRun, honesty-rule, never-partial, byte-identity, leave-as-digits, ambiguity-resolves-to-leave, leading-zero, build-review]
    path: tradeoff/2026-07-10-grouped-decimal-declined-wholesale-left-byte-identity.md
    summary: "Build-review round 1 of #139 caught a value-altering PARTIAL render: a comma-grouped integer followed by a decimal point (1,234.5) spelled the integer half and orphaned the raw '.5', violating the honesty rule / never-partial faithful-value contract. DECISION: commaGroupedRun DECLINES (ok=false) when the terminator after the last triple is a decimal point + digit, so the entire grouped-decimal span falls through to the plain path and stays byte-identity (LEAVE). Grouped-decimal voicing (1,234.5 -> 'one thousand two hundred thirty-four point five') is a candidate FUTURE ticket, not this one. Leading-zero first groups (0,000) also declined so a format artifact never collapses to 'zero'. The never-partial fallback (emit original bytes verbatim, never a partial/guessed rendering) is load-bearing — a half-spelled grouped decimal is worse than a clunky-but-correct digit stream. Extends the standing #138 rule 'ambiguity resolves to LEAVE.'"
  - id: 2026-07-10-voicespelled-seam-strip-before-spell-before-lexicon
    title: "voiceSpelled seam orders strip → spellNumbersInProse → lexicon-scan"
    date: 2026-07-10
    status: accepted
    category: convention
    tags: [issue-138, planner, voicing, cardinal-spell-out, voiceSpelled, stripInlineMarkdown, spellNumbersInProse, scanLexicon, ordering-contract, pre-voice-transform, plan-review-regression]
    path: convention/2026-07-10-voicespelled-seam-strip-before-spell-before-lexicon.md
    summary: "#138's cardinal number pass MUST run on markdown-STRIPPED text and BEFORE the lexicon byte-scan, made explicit via a voiceSpelled seam. voice() calls stripInlineMarkdown() as its first internal line, so running the number pass on raw text made **24700** guard-out correctly as digits then get stripped to a bare digit stream (silent regression caught in plan review round 1 — the number pass and strip pass disagreed on the 'source' adjacency). REJECTED relying on stripInlineMarkdown idempotency (double-strip): idempotency is an implicit, untested invariant nothing guards. The seam names the ordering contract in code and shares one scanLexicon body so voice()/voiceSpelled() can't drift. Nearest precedent for any future pre-voice transform that inspects source adjacency."
  - id: 2026-07-10-conservative-leave-as-digits-cardinal-spell-out
    title: "Conservative 'leave as digits' scope for cardinal spell-out"
    date: 2026-07-10
    status: accepted
    category: tradeoff
    tags: [issue-138, planner, voicing, cardinal-spell-out, spellCardinal, leave-as-digits, conservative-scope, comma-grouped-deferred, negatives-deferred, ambiguity-resolves-to-leave]
    path: tradeoff/2026-07-10-conservative-leave-as-digits-cardinal-spell-out.md
    summary: "#138 spells ONLY plain standalone 1–6 digit integers/decimals in the no-intelligence verbatim prose path. Leaves as digits: comma-grouped (24,700), negatives (-5, sign-vs-hyphen ambiguous), dotted versions, hex, ≥7-digit runs, and any run adjacent to :/=// /- or a letter/_. Rationale: a wrong cardinal reading is worse than a clunky-but-correct digit stream, so ambiguity resolves to leave — keeping spellCardinal small and every spell-out provably safe. REJECTED aggressive spelling of grouped/negative/adjacent forms (confident mis-reads). Comma-grouped and negatives explicitly DEFERRED (follow-up ticket for comma-grouped), not rejected. Establishes the standing rule: ambiguity resolves to leave."
  - id: 2026-07-10-cardinals-degradeprose-no-intelligence-only
    title: "Cardinal spell-out applies to the degradeProse no-intelligence path only; adapter-voiced prose is out of scope"
    date: 2026-07-10
    status: accepted
    category: architecture
    tags: [issue-138, planner, voicing, cardinal-spell-out, degradeProse, no-intelligence-path, adapter-authoritative, no-double-processing, boundary]
    path: architecture/2026-07-10-cardinals-degradeprose-no-intelligence-only.md
    summary: "#138's cardinal spell-out runs ONLY on the degradeProse no-intelligence verbatim path (Status=degraded, short prose read verbatim). Adapter-voiced prose is explicitly out of scope: adapter output is authoritative spoken text and a second deterministic pass would risk double-processing (re-spelling numbers the adapter already voiced, or corrupting intended phrasing). The deterministic speller exists precisely to fill the gap where no intelligence adapter is available to voice numbers. REJECTED running cardinals on all voiced prose including adapter output (planner second-guessing the intelligence layer). Number handling is intentionally path-dependent; keeps the intelligence layer authoritative and the pass narrowly scoped."
  - id: 2026-06-29-global-hotkey-focus-guard-opt-in-allow-list
    title: "Global hotkey focus guard is an opt-in allow-list"
    date: 2026-06-29
    status: accepted
    category: convention
    tags: [issue-134, earshot, keyboard, transport, playback, a11y, accessibility, aria, focus-guard, allow-list, data-transport-hotkeys, safe-by-default]
    path: convention/2026-06-29-global-hotkey-focus-guard-opt-in-allow-list.md
    summary: "#134's global transport handler (Space, ←/→) is INERT by default and acts ONLY when focus is on document.body/null OR inside an explicitly marked [data-transport-hotkeys] region (the transcript <main>). Load-bearing; SUPERSEDES the earlier deny-list approach (flagged incomplete in plan review). The allow-list makes the #111 role=listbox, #113 role=radiogroup, #112 role=toolbar/role=slider, and any FUTURE keyboard-interactive ARIA widget safe-by-default with no per-widget maintenance — versus a deny-list that must enumerate every widget and silently breaks when a new one is added. A trimmed native-control bail inside the marked region is defense-in-depth only (full role-by-role deny enumeration deliberately NOT the primary mechanism)."
  - id: 2026-06-29-single-document-keydown-listener-via-ref-pattern
    title: "Single document keydown listener via the ref pattern"
    date: 2026-06-29
    status: accepted
    category: architecture
    tags: [issue-134, earshot, keyboard, transport, playback, react, ref-pattern, event-listener, stale-closure, usetransporthotkeys]
    path: architecture/2026-06-29-single-document-keydown-listener-via-ref-pattern.md
    summary: "The global hotkey hook (useTransportHotkeys) keeps exactly ONE mount-time document keydown listener (useEffect [] cleanup on unmount) whose body reads latestRef.current rather than closing over render values; registered on the BUBBLE phase. Each render assigns latestRef.current = { hasAudio, controls }. Immune to controls-identity churn (one add/remove pair for the component's life), and it does NOT lift hasAudio into shell state, so playback progress ticks neither re-render the shell nor re-bind the listener. Chosen over dep-array re-binding. Bubble-phase registration means widget handlers run first; the focus-guard allow-list is the sole real guarantee (a React synthetic stopPropagation does not reliably stop a native document listener). A stale-closure regression test locks the pattern."
  - id: 2026-06-29-arrow-keys-10s-time-seek-not-block-step
    title: "Left/Right arrows do ±10s time seek on the wav playhead, not block-step"
    date: 2026-06-29
    status: accepted
    category: tradeoff
    tags: [issue-134, earshot, keyboard, transport, playback, seek, block-level-sync, media-convention]
    path: tradeoff/2026-06-29-arrow-keys-10s-time-seek-not-block-step.md
    summary: "Resolves #134's design question: Left/Right arrows perform a ±10-second TIME seek on the combined wav playhead, NOT a block-step. A free-moving audio playhead is transport/display only and plan↔text-mapping-neutral — it persists nothing sub-block, so it does NOT violate the block-level-sync invariant. Block-step stays on the slider's native role=slider arrow keys. Option (b) (←/→ = prev/next block, ±10s remapped to ,/. or J/L) REJECTED as less familiar and against media convention. Implemented via a new skipSeconds(deltaSec) command-surface method that clamps both ends and no-ops when the audio element or duration metadata is absent. No plan/timeline schema change."
  - id: 2026-06-29-restore-gate-clears-only-on-landed-seek
    title: "Earshot resume restore gate clears only on a landed seek, never on a bare timeout"
    date: 2026-06-29
    status: accepted
    category: architecture
    tags: [issue-112, earshot, resume, restore-gate, useplayback, raf, seeked-event, preload-metadata, clobber, honest-over-clobber, code-review-found, playback-engine]
    path: architecture/2026-06-29-restore-gate-clears-only-on-landed-seek.md
    summary: "#112's Earshot resume uses a `restoring` gate in usePlayback that mutes the rAF active-block derivation AND the resume-writer until restore lands. DECISION: the gate clears ONLY when a real `seeked` event lands with currentTime inside the restored block's [start, nextStart) range — a bare 2s timeout must NOT clear it. With <audio preload=none/metadata> the issued seek can fail to land (currentTime stays 0); the first build cleared on timeout, which let rAF derive block 0 and let the resume-writer clobber the saved position — the exact bug the gate existed to prevent (found in code review round 1). Supporting changes: deck audio uses preload=metadata + load() so the seek has data to land against; the timeout RE-ASSERTS the seek rather than clearing; the resume-writer refuses to persist block 0 while readyState<1. Trade-off accepted (honest-over-clobber): on a genuinely unloadable URL the gate can stay muted for the session rather than clobbering the saved position with a fabricated block-0."
  - id: 2026-06-29-earshot-transport-two-tab-stops-not-one
    title: "Earshot transport deck exposes two keyboard tab stops, not one (APG toolbar + separate slider)"
    date: 2026-06-29
    status: accepted
    category: architecture
    tags: [issue-112, earshot, accessibility, a11y, apg, transport-deck, role-toolbar, roving-tabindex, role-slider, block-scrubber, tab-stop, stoppropagation, arrow-key-ownership, adr-108, playback-engine]
    path: architecture/2026-06-29-earshot-transport-two-tab-stops-not-one.md
    summary: "The #112 transport deck deliberately exposes TWO keyboard tab stops: a role=toolbar button group as a single roving-tabindex stop (Left/Right move between buttons), and the role=slider block scrubber as its own SEPARATE adjacent tab stop that owns its arrow keys and stopPropagation()s them so they never reach the toolbar roving handler. #108's original 'single composite tab stop for the whole deck' goal is explicitly RELAXED — this is the APG-conformant resolution of the toolbar-arrows (button traversal) vs slider-value-arrows (scrub) ownership conflict, which one composite stop cannot give two owners. Cost accepted: two Tab presses to cross the deck. Extends/implements ADR #108 rather than overturning it; the toolbar + roving-tabindex pattern stands."
  - id: 2026-06-29-single-useplayback-instance-at-narrationcontext
    title: "Single usePlayback instance lives at the NarrationContext provider"
    date: 2026-06-29
    status: accepted
    category: architecture
    tags: [issue-112, earshot, useplayback, narrationcontext, playbackcontext, single-instance, raf-sync-loop, restore-gate, transport-command-surface, shared-audio, context-provider, playback-engine]
    path: architecture/2026-06-29-single-useplayback-instance-at-narrationcontext.md
    summary: "usePlayback (the rAF sync loop + transport command surface + restoring gate) is instantiated EXACTLY ONCE, at the NarrationContext/PlaybackContext provider; all consumers (TransportDeck, BlockScrubber, BlockRow) read its command surface from context rather than calling usePlayback themselves. A second instance would fork the rAF loop, the restoring gate, and active-block truth over the single shared <audio> — three sources of truth fighting one element. Reinforces the whole-audio.wav playback-unit (2026-06-22) and rAF-transition-only-rerender (2026-06-21) decisions by ensuring a single owner. Breaks only if Earshot ever needs concurrent simultaneous playback (multiple audio elements)."
  - id: 2026-06-29-earshot-resume-persists-block-identity-only
    title: "Earshot resume persists block identity only (blockId + block-signature), never an ms offset"
    date: 2026-06-29
    status: accepted
    category: convention
    tags: [issue-112, earshot, resume, localstorage, block-level-sync, block-identity, block-signature, schema-version, self-healing, no-word-timing, no-ms-offset, playback-engine]
    path: convention/2026-06-29-earshot-resume-persists-block-identity-only.md
    summary: "#112's localStorage resume entry stores block IDENTITY only — blockId + blockOrder + blockSignature + schemaVersion — never a millisecond offset. startMs is re-derived LIVE from the current timeline on restore; on a blockSignature or schemaVersion mismatch the entry is dropped (self-healing). A stored ms offset would be a word/time-level position that contradicts the block-level-sync-only invariant (sync keyed by block_id; spoken text ≠ source text under gist mode, so sub-block timing is forbidden) and would go stale the instant a block re-renders (escalation reflows downstream offsets). Upholds block-level-sync-only for the persistence layer: the saved position is a block reference, not a time offset; resume granularity is block-start only, never mid-block."
  - id: 2026-06-29-escalation-refusal-keeps-control-mounted
    title: "An escalation that RETURNS a refusal keeps the control mounted (error path), distinct from an originally-refused block"
    date: 2026-06-29
    status: accepted
    category: architecture
    tags: [issue-113, earshot, refusal, honesty-rule, blockrow, levelcontrol, originally-refused, escalation-refusal, role-alert, error-path, hide-on-refused]
    path: architecture/2026-06-29-escalation-refusal-keeps-control-mounted.md
    summary: "#113 distinguishes two refusal cases for the per-block LevelControl. An ORIGINALLY-refused document block (first /narrate returned it refused, e.g. a bare image) never gets a control — hide-on-refused applies ONLY here. A voiced/degraded block the user escalates whose higher level comes back REFUSED is treated like the error path: do NOT flip the block to refused, keep LevelControl mounted, keep the prior committed level selected and PLAYABLE, snap aria-checked back to the prior level, surface the refusal inline via role=alert ('Block can't be voiced at L{n}; still at L{prior}'); never blank the block. REJECTED v1 unconditional hide-on-refused — a user escalating a voiced block into a refusal would lose the control and be stranded with a blanked block. BlockRow branches on the ORIGIN of the refusal, not just block.status; review test escalate→refusal-keeps-control locks it. Honors the honesty rule (refusal still surfaced, just not as a block-blanking transition)."
  - id: 2026-06-29-earshot-seed-level-snapshot-cache-no-model-rebill
    title: "Seed a per-(blockId,level) snapshot cache with paired timeline; de-escalate-to-seen-level is a no-model-rebill swap"
    date: 2026-06-29
    status: accepted
    category: architecture
    tags: [issue-113, earshot, state-management, useNarrationSession, snapshot-cache, no-model-rebill, timeline-snapshot, de-escalation, in-memory-cache, f1-offset-bug]
    path: architecture/2026-06-29-earshot-seed-level-snapshot-cache-no-model-rebill.md
    summary: "#113 lets a listener return to a previously-seen level without re-paying intelligence. DECISION: seed cache[(blockId, block.level)] = {block, timing, timelineSnapshot} for EVERY block whenever a transcript is set (initial load AND after each escalate), pairing each cached level with the timeline authoritative AT THAT LEVEL. On a cache hit, restore {block, timing} AND replace the entry's whole transcript.timeline with the cached timelineSnapshot (so an offline de-escalate cannot reintroduce the F1 stale-offset bug), bump the reload nonce, reload() — no postNarrateBlock call, no model re-bill. 'No re-bill' CLARIFIED = no model/intelligence spend: under the single-shared-<audio> model a de-escalate MAY still POST so the server rewrites the combined wav, but the server's (block hash, level, model) cache returns it without billing the model; the client snapshot cache additionally enables a zero-network swap for return-to-seen-level. REJECTED caching {block, timing} WITHOUT the timeline snapshot — restores a block against the post-escalate timeline and reintroduces F1 on the offline path. Cache lives in useNarrationSession beside entries (single server-cache owner), in-memory/session-scoped, lost on reload by design. Review test cache-hit-timeline-snapshot locks it."
  - id: 2026-06-29-levelcontrol-commit-on-activate-not-follow-focus
    title: "LevelControl commits on Space/Enter/click; arrows move roving focus only"
    date: 2026-06-29
    status: accepted
    category: architecture
    tags: [issue-113, earshot, accessibility, a11y, radiogroup, apg, roving-tabindex, commit-on-activate, manual-selection, segmentedtoggle-divergence, billable-renarration, wcag]
    path: architecture/2026-06-29-levelcontrol-commit-on-activate-not-follow-focus.md
    summary: "#113's per-block L1/L2/L3 LevelControl is an APG role=radiogroup with three role=radio cells and roving tabindex (one Tab stop). ←/→ and ↑/↓ move roving FOCUS ONLY — they do NOT select. Selection is COMMIT-ON-ACTIVATE: Space/Enter or click of the focused cell, never on arrow traversal. Exactly one aria-checked=true held across the async window, snapped back on failure; focus ring is now meaningfully distinct from selection (focus ≠ commit). INTENTIONAL divergence from SegmentedToggle's select-follows-focus, justified because each commit triggers a BILLABLE re-narration (POST /narrate/block with model spend on an unseen level) — arrowing L1→L2→L3 under select-follows-focus would fire three escalations. APG Radio Group pattern explicitly permits the manual-selection variant; WCAG 2.1 AA baseline preserved. REJECTED select-follows-focus parity with SegmentedToggle (bills on every arrow keypress). role=status (polite) announces loading; role=alert (assertive) carries an escalation-returned refusal. Two visually similar Earshot radiogroups now have deliberately different keyboard semantics; review test arrow-moves-focus-no-escalation locks it."
  - id: 2026-06-29-narrate-block-returns-full-post-patch-timeline
    title: "/narrate/block returns the FULL post-patch timeline; client replaces its timeline wholesale"
    date: 2026-06-29
    status: accepted
    category: architecture
    tags: [issue-113, earshot, narrate-server, narrate-block, timeline, patchblock, downstream-offsets, whole-timeline-replace, frozen-escalate-core, block-level-sync, api-contract]
    path: architecture/2026-06-29-narrate-block-returns-full-post-patch-timeline.md
    summary: "#113 escalates one block in place via POST /narrate/block (capturingSink → sink/persistent.PatchBlock → on-disk readBack). VERIFIED: PatchBlock rewrites the single combined audio.wav and SHIFTS ALL DOWNSTREAM OFFSETS (a +60ms grow on b1 moves b2 410→460ms). Today runNarrateBlock's readBack returns only the ONE patched block's timing, so the client cannot learn the shifted sibling offsets and a downstream-sibling seek lands on the wrong audio. DECISION: add a full timeline field to narrateBlockResponse ONLY, built inside runNarrateBlock from the POST-PATCH manifest (the same on-disk manifest readBack already loads); the Earshot client replaces the entry's whole transcript.timeline WHOLESALE with response.timeline while replacing only the one patched block in transcript.blocks (by id) — the client NEVER recomputes offsets (honors block-level-sync-only / no-word-timing). The frozen escalateResponse and the shared escalateInDir core are NOT touched — the new field is assembled in the /narrate/block handler AFTER the shared core returns, so runEscalate stays byte-identical. AC6 reinterpreted: sibling CONTENT untouched, sibling OFFSETS track the server timeline. SUPERSEDES the initially-planned v1 per-block merge-isolation / 'stale-elsewhere accepted' approach (never journaled) — correct for a per-dir sink but wrong for the single-combined-wav render where one patch reflows the whole file. Older server lacking timeline falls back to single-timing merge (documented degrade)."
  - id: 2026-06-29-earshot-render-id-additive-narrateresponse-field
    title: "Expose render_id as an additive narrateResponse field, not parsed out of audio_url"
    date: 2026-06-29
    status: accepted
    category: architecture
    tags: [issue-113, earshot, narrate-server, render-id, audio-url-opaque, additive-field, types-ts, escalation, api-contract]
    path: architecture/2026-06-29-earshot-render-id-additive-narrateresponse-field.md
    summary: "#113 makes Earshot the first client of POST /narrate/block (#110), which needs the prior render's render_id. But D4 (2026-06-28-earshot-narrate-contract-pinned-audio-url-opaque) treats audio_url as OPAQUE — the client never parses render_id out of it. DECISION: add RenderID string (json:render_id) to the server's narrateResponse in cmd/narrate-server/narrate.go, populated from the render id already computed for the audio URL, and mirror it as render_id: string on NarrateResponse in earshot/src/api/types.ts. The escalation client reads entry.transcript.render_id directly and never touches audio_url, preserving the audio-url-opaque invariant (D4). The field is additive/ignorable within the major schema_version (other /narrate clients ignore it); an older response lacking render_id disables escalate gracefully (LevelControl disabled). REJECTED parsing render_id out of audio_url — violates D4, bakes the /audio/{render_id}.wav URL scheme into the client, breaks on any scheme change, pure downside. Client now holds a render for two separated reasons: playing audio (opaque url) and escalating a block (render_id)."
  - id: 2026-06-29-earshot-parallel-per-entry-narration-model
    title: "Earshot uses a parallel per-entry narration model with in-memory client-side dedup"
    date: 2026-06-29
    status: accepted
    category: architecture
    tags: [issue-111, earshot, state-management, parallel-narration, in-memory-cache, client-side-dedup, entries-map, issue-112, issue-126]
    path: architecture/2026-06-29-earshot-parallel-per-entry-narration-model.md
    summary: "Earshot's useNarrationSession owns an `entries` Map keyed by a stable string (message.id for session messages, file.name for files), each holding {status: loading|ready|error, transcript, error}, replacing the single-currentTranscript model. Multiple narrations run concurrently with per-row/per-file status badges; selecting an already-ready entry switches the transcript view INSTANTLY with no re-narrate (in-memory client-side dedup). The Map persists across session/tab switches for the page lifetime, so navigate-away-and-back reuses prior renders — the dedup the user asked about. Each request bounded by a 2-minute AbortController timeout (raised from 30s). LIMITATION parked, not solved: in-memory only (lost on reload); narrate-server re-renders every POST /narrate (no server-side content-hash audio reuse); persistent audio cache deferred (user accepted). REJECTED single shared currentTranscript (one narration at a time, re-narrates every revisit). #112 resume persistence should fold this into a localStorage store keyed by session+message — a persistent superset of this in-memory key; relates to #126 dormant content-hash escalation cache."
  - id: 2026-06-28-earshot-narrate-contract-pinned-audio-url-opaque
    title: "Earshot pins the narrate-server contract in types.ts and treats audio_url as an opaque URL"
    date: 2026-06-28
    status: accepted
    category: architecture
    tags: [issue-111, earshot, narrate-server, http-bridge, contract-isolation, audio-url-opaque, message-ref, types-ts, divergence-point, issue-109]
    path: architecture/2026-06-28-earshot-narrate-contract-pinned-audio-url-opaque.md
    summary: "Earshot (#111) consumes the #109 HTTP bridge; all shape knowledge lives in earshot/src/api/types.ts and audio_url is treated as an OPAQUE URL used verbatim (fed to <audio src>, no render_id parsing, no /audio/{id}.wav reconstruction — D4). The assumed message_ref shape is the single clearly-commented mock↔live divergence point, so reconciling against live #109 is a one-line change. Open string-unions (| (string & {})) implement the schema-versioning rule so unknown enum values round-trip via a neutral fallback. REJECTED extracting a render_id to reconstruct the audio path (bakes the server URL scheme into the client; breaks on any scheme change; pure downside since the client never addresses audio by id). Known limitation parked to /verify: today's narrate path sends assembled text not message_ref, so returned block.ids are per-render not per-message. Sibling to the multipart file-input decision; server side is the sessions-messages structural-split decision."
  - id: 2026-06-28-earshot-file-input-multipart-upload-not-path-json
    title: "Earshot file input is multipart upload (FormData POST /narrate/file), not the design-doc {path} JSON variant"
    date: 2026-06-28
    status: accepted
    category: architecture
    tags: [issue-111, earshot, narrate-server, http-bridge, file-input, multipart, formdata, decide-not-discover, csrf, file-read-vector, issue-109]
    path: architecture/2026-06-28-earshot-file-input-multipart-upload-not-path-json.md
    summary: "Earshot's (#111) file pane input mode is multipart upload: the picked/dropped File is packed into FormData (a file field plus level/gender form fields) and sent as multipart POST /narrate/file, same response shape as POST /narrate. The {path} JSON variant exists nowhere in the client or mock. Settled at the human gate (D1) as a decide-not-discover call because Phases 4–5 build against mocks with no server to probe, and FormData-vs-JSON is non-abstractable client code. A test asserts the request body is an actual FormData. REJECTED Option A {path} JSON (a browser drop/pick yields a File the browser reads in-process, but {path} only addresses server-local paths the browser cannot read — wrong trust model; and {path} is the dropped #109 server-side file-read/CSRF vector). Distinct CLIENT-side decision carrying the same trust-boundary reasoning as the #109 server-side source-path-dropped security decision to the browser edge."
  - id: 2026-06-28-post-narrate-block-escalation-persists-a-3-file
    title: "POST /narrate/block escalation persists a 3-file persistent-sink dir per render_id (Option 1)"
    date: 2026-06-28
    status: accepted
    category: architecture
    tags: [narrate-server, http-bridge, escalation-endpoint, narrate-block, render-id, persistent-sink, patchblock, byte-preserving, three-file-dir, audio-url, posix-unlink, os-rename-atomic, content-hash-cache-dormant, supersedes, issue-110, issue-109]
    path: architecture/2026-06-28-post-narrate-block-escalation-persists-a-3-file.md
    summary: "#110 POST /narrate/block re-renders ONE block of a prior /narrate render, byte-preserving the rest. CHOSEN Option 1: narrate-server persists a 3-file persistent-sink directory per render_id (audio.wav+plan.json+manifest.json) under tempRoot/{render_id}/, keyed by render_id (NOT a user-supplied dir); the endpoint reuses the proven persistent.PatchBlock + readBack path and returns an escalateResponse with an audio_url field (not audio_ref+dir). Crux forcing a storage-model change: /narrate (#109) currently persists ONLY a combined wav + createdAt — plan, timeline, per-block wavs, AND source text are discarded [verified]; PatchBlock requires all 3 files present (ErrNothingToPatch on absence) [verified], so a full 3-file dir is mandatory. Byte-preserving via PatchBlock + manifest-derived byte ranges; POSIX open-fd survives unlink (Linux+macOS) and os.Rename is atomic same-dir/same-fs (EXDEV cross-fs; durability needs fsync) [verified]. Content-hash escalation cache stays DORMANT phase one (per-block hashes already rejected by 2026-06-20-pipeline-block-rerender-uses-document-hash; regen sub-100ms earns nothing). REJECTED Option 2 (in-memory plan/state between calls — reopens seam-gap R1, RSS balloon) and Option 3 (lazy materialization — unbuildable since /narrate discards plan AND source text, plus a new crash window + file/dir heterogeneity). SUPERSEDES the single-wav claim in 2026-06-28-render-id-wav-ttl-reaper-orphan-scan-lifecycle.md; trips (does not violate) the WAVFileSink-no-sidecars revisit trigger. All load-bearing claims adversarially verified in Stage 3; research/narrate-block-render-id-state-110/."
  - id: 2026-06-28-narrate-inline-text-only-source-path-dropped
    title: "POST /narrate accepts inline text only — server-side source path dropped as a CSRF/file-read vector"
    date: 2026-06-28
    status: accepted
    category: security
    tags: [narrate-server, http-bridge, csrf, dns-rebinding, ssrf, loopback, attack-surface, source-deferred, issue-109]
    path: security/2026-06-28-narrate-inline-text-only-source-path-dropped.md
    summary: "#109 POST /narrate accepts inline `text` ONLY; the original-AC `source` (server-side file path) input is DROPPED/deferred for phase one. The server is loopback-bound but browser-reachable, so a file-path argument is a CSRF / DNS-rebinding vector — an arbitrary-local-file read + render side effect that pinned CORS does not prevent (it blocks reading the response, not the side effect). Wider surface than the cmd/narrate --in CLI flag (whose trust boundary is the shell). The Earshot UI already has message text from GET /sessions, so inline text suffices. REJECTED/DEFERRED alternative: keep `source` constrained via a resolveWithin allowlisted base dir (still a CSRF-reachable file-read side effect + a security-critical config). A regression test asserts {text,source} -> HTTP 400. Mirrors the loopback-refuse-to-start posture: refuse capability, don't filter it after the fact."
  - id: 2026-06-28-sessions-messages-structural-split-not-planner
    title: "GET /sessions/{id}/messages pre-chunking is server-side structural-split-only, not a planner.Plan call"
    date: 2026-06-28
    status: accepted
    category: architecture
    tags: [narrate-server, http-bridge, structural-split, planner-boundary, no-io-in-planner, read-endpoint, segment-unexported, refusal-boundary, step-0-spike, issue-109]
    path: architecture/2026-06-28-sessions-messages-structural-split-not-planner.md
    summary: "#109 GET /sessions/{id}/messages pre-chunking is STRUCTURAL-SPLIT-ONLY via a server-side splitter in package main, NOT a planner.Plan call. The Step 0 spike found the planner exposes no render-free / voice-free segmentation entry — planner.Plan is the only exported entry and it voices+levels+refuses (segment() is unexported). Routing big messages through it would REFUSE large prose (>120 words) instead of structurally splitting it, and would couple I/O/voicing into a read endpoint. The server-side splitter returns plain ordered blocks and keeps planner/ and plan/ imports untouched. REJECTED: calling planner.Plan from the endpoint (large prose comes back refused, not chunked; no voice-free seam since segment() is unexported)."
  - id: 2026-06-28-audio-serve-releases-store-lock-before-streaming
    title: "GET /audio/{render_id}.wav holds the store read-lock only across resolve+open, then streams lock-free"
    date: 2026-06-28
    status: accepted
    category: concurrency
    tags: [narrate-server, http-bridge, rwmutex, writer-starvation, liveness, servecontent, posix-unlink, render-store, reaper, wall-clock-test, issue-109]
    path: concurrency/2026-06-28-audio-serve-releases-store-lock-before-streaming.md
    summary: "#109 GET /audio/{render_id}.wav takes the render-store read-lock ONLY across resolve + os.Open, releases it, then http.ServeContent streams from the held *os.File. A read-lock spanning the whole ServeContent lets a slow/large download hold the store read-lock; a waiting reaper write-lock then starves ALL new /narrate mints and /audio serves, because Go's RWMutex blocks new readers once a writer is waiting — a liveness bug -race cannot detect. On POSIX the open fd survives unlink, so the TTL reaper os.Remove's lock-free while a serve is in flight. A wall-clock test asserts a slow in-flight serve does not block a concurrent mint. REJECTED: holding the read-lock across the entire ServeContent (writer-starvation chain)."
  - id: 2026-06-28-render-id-wav-ttl-reaper-orphan-scan-lifecycle
    title: "render_id wav lifecycle — TTL reaper plus orphan-scan, deletes outside the store write-lock"
    date: 2026-06-28
    status: superseded
    category: architecture
    tags: [narrate-server, http-bridge, render-store, ttl-reaper, orphan-scan, crash-window, wavfilesink, single-wav, os-remove-outside-lock, snapshot-under-lock, issue-109]
    path: architecture/2026-06-28-render-id-wav-ttl-reaper-orphan-scan-lifecycle.md
    summary: "#109 render_id WAV lifecycle: a TTL reaper PLUS an orphan-scan for the crash-between-write-and-mint window (render completes on disk but the process dies before the store records the mint — TTL alone never sees those, so they leak). The render uses the single-wav WAVFileSink variant (combined wav, no plan/manifest sidecars), distinct from the 3-file persistent sink. os.Remove I/O happens OUTSIDE the store write-lock — expired entries are snapshotted under the lock, then deleted after release — so eviction never blocks a /narrate mint, and the reaper deletes lock-free (POSIX open-fd survives unlink, pairing with the audio-serve path). REJECTED: TTL-only with os.Remove under the write-lock (leaks crash-window orphans; couples eviction disk-I/O into the mint path). Builds on the separately-recorded single-wav-no-sidecars WAVFileSink decision; this records the server-side reaper/orphan-scan layer."
  - id: 2026-06-28-earshot-mockup-signed-off-design-approved-for-111
    title: "Earshot listener-UI mockup signed off — design approved for the #111 build"
    date: 2026-06-28
    status: accepted
    category: architecture
    tags: [earshot, mockup, sign-off, leveling-ui, transport-anchor, session-pane, file-pane, issue-115, issue-111]
    path: architecture/2026-06-28-earshot-mockup-signed-off-design-approved-for-111.md
    summary: "User signed off on the Earshot listener design via the clickable #115 mockup (earshot-mockup/EarshotMockup.jsx, served by make preview-mockup). Probe 1 (per-block L1/L2/L3 segmented level control — the inline escalation surface under Model A) accepted as-is; reads intuitively as a gist→detail ladder. Probe 2 (bottom-anchored transport) accepted — no confusion observed, bottom-anchor default held, no flip to top (REJECTED Option B top-anchor and Option C replacing the 3-state control). Two design-§4 gaps found during testing were fixed inline and are part of the approved surface: session-ID entry in the Session pane (honest glob-miss on unknown ID, never a fabricated session) and the File pane for use case 2 (drop-a-file → read out, with oversized-section chunking). Sign-off unblocks #111; the mockup is throwaway — approved patterns are hand-ported into earshot/, not wired in. Revisit if the 3-state control or bottom transport proves unworkable in the real app."
  - id: 2026-06-28-shared-transcript-parser-lives-in-internal
    title: "Shared transcript parser lives in internal/transcript; speak_last skip is caller policy; Message.Turn is an emit-index, not a stable id"
    date: 2026-06-28
    status: accepted
    category: architecture
    tags: [transcript-parser, internal-package, claude-code-jsonl, speak_last, tool-agnostic, caller-policy, emit-index, not-a-stable-id, additive-extensible, pagination-cursor, decoupled-listen-path, issue-106, issue-109, adr-86, adr-81]
    path: architecture/2026-06-28-shared-transcript-parser-lives-in-internal.md
    summary: "#106 extracts the Claude Code transcript .jsonl parser out of cmd/narrate-mcp's lastAssistantText (which was hard-wired to return only the last assistant turn AND knew the speak_last tool name for self-skip) into a new internal/ package internal/transcript (sibling to internal/errclass, internal/intelligencetmpl), exposing ParseMessages(path) ([]Message, error) returning the FULL ordered []Message{Turn, Role, Text, ToolNames}. internal/ keeps it off the public module surface, importable by both cmd/ roots without one binary depending on another. Decisions: (1) parser stays TOOL-AGNOSTIC — records tool_use names in ToolNames but knows no specific tool; the speak_last self-invocation skip stays CALLER-SIDE POLICY in cmd/narrate-mcp (slices.Contains over ToolNames), not parser logic. (2) Message.Turn is an EMIT-INDEX (position among emitted messages, tool-only turns included) that RENUMBERS if a previously-skipped/unparseable line later parses — NOT a stable identifier. Forward contract for #109 (GET /sessions/{id}/messages): must NOT use Message.Turn as a pagination cursor; prefer a line-derived stable id (timestamp/UUID); Message is additively extensible so #109 may widen it without breaking schema. REJECTED: exporting the parser from cmd/narrate-mcp (one binary depending on another's surface); keeping speak_last in the parser (re-bakes the removed coupling); string-only user-content assumption (real user turns carry array content like tool_result). Honors the standing 'keep the listen path decoupled from any durable sink' order — pure parser refactor, speak_last stays byte-identical (6-row TestLastAssistantText oracle unchanged). Corroborated by multi-perspective review."
  - id: 2026-06-28-low-priority-mid-build-issues-fix-inline-or-drop
    title: "Low-priority mid-build issues: fix inline or drop — never defer to new tickets"
    date: 2026-06-28
    status: accepted
    category: process
    tags: [process, workflow, review, backlog, standing-order]
    path: process/2026-06-28-low-priority-mid-build-issues-fix-inline-or-drop.md
    summary: "Standing order. Low-priority issues surfaced mid-build are FIXED INLINE in the same PR or DROPPED — NOT deferred to new tickets. Corollary guardrails: don't pick a tiny issue just so you can defer it; and don't log every trivial deferral in the decision journal either — this single standing-order entry IS the record, individual trivial cases are not journaled. Became standing philosophy during the #105 speak_to_file build (PR #114): two non-blocking review suggestions (S1 sink-arg leak, S3 mcptext URI-scheme DUP) were fixed inline in the same PR rather than spun out as follow-up tickets. Keeps the backlog free of never-actioned low-value tickets and the journal signal-dense. Revisit if dropping mid-build findings starts repeatedly costing real work."
  - id: 2026-06-28-wavfilesink-reuses-wav-concat-no-sidecars
    title: "WAVFileSink reuses persistent-sink wav-concat math but writes only the combined wav, no JSON sidecars"
    date: 2026-06-28
    status: accepted
    category: architecture
    tags: [speak-to-file, wav-sink, buildSegments, persistent-sink, no-sidecars, dry]
    path: architecture/2026-06-28-wavfilesink-reuses-wav-concat-no-sidecars.md
    summary: "speak_to_file's contract is one .wav at a caller path; the existing persistent sink writes a 3-file directory (audio.wav+plan.json+manifest.json). WAVFileSink reuses the per-block wav-concatenation math by extracting an unexported buildSegments helper out of sink/persistent.Consume (returns segments + counts only, not a receipt), then writes ONLY the combined wav — no plan.json/manifest.json sidecars. REJECTED duplicating the concat walk. Rationale: avoid duplicating the concat walk; each sink builds its own lean receipt; the single-wav contract must not drag in the directory-manifest contract. Revisit if a caller needs the plan/manifest alongside the wav."
  - id: 2026-06-28-speak-to-file-uniform-response-envelope
    title: "speak_to_file returns a uniform speakToFileResponse envelope across both path and no-path branches"
    date: 2026-06-28
    status: accepted
    category: architecture
    tags: [speak-to-file, mcp, response-envelope, tool-contract, dual-channel]
    path: architecture/2026-06-28-speak-to-file-uniform-response-envelope.md
    summary: "speak_to_file writes a file when output_path is given and falls back to speaking ephemerally when it is not. Both branches return the same speakToFileResponse envelope mirroring speak's dual-channel (human transcript + structured result) shape; the no-path branch reuses runSpeakWithCache and re-wraps its result into speakToFileResponse{output_path:\"\"}. REJECTED different shapes per branch. Rationale: one stable contract for the client LLM regardless of whether a file was written; output_path==\"\" is the wire signal that the tool spoke instead of writing."
  - id: 2026-06-28-speak-to-file-output-path-file-vs-dir-rule
    title: "resolveOutputPath rule for speak_to_file output_path (file vs directory)"
    date: 2026-06-28
    status: accepted
    category: architecture
    tags: [speak-to-file, output-path, path-resolution, file-vs-dir, resolveOutputPath]
    path: architecture/2026-06-28-speak-to-file-output-path-file-vs-dir-rule.md
    summary: "Resolves the 'settle at build' open item from the separate-tool decision. Rule: if output_path denotes a directory (trailing separator, '.' or '..', or an existing directory) → write a derived filename into that directory; otherwise treat it as a file path and append '.wav' case-insensitively if missing (no double extension); MkdirAll the parent; result is absolutized and Cleaned. Rationale: callers may pass either a directory or a full file path; the rule disambiguates from the value itself without a separate flag."
  - id: 2026-06-28-wavfilesink-in-persistent-package-debt
    title: "WAVFileSink lives in package sink/persistent, accepted as debt"
    date: 2026-06-28
    status: accepted
    category: architecture
    tags: [speak-to-file, tech-debt, package-naming, wavfile-sink, wavconcat, accepted-debt]
    path: architecture/2026-06-28-wavfilesink-in-persistent-package-debt.md
    summary: "The new single-wav WAVFileSink was placed in package sink/persistent, whose name now overstates its contents (it holds both the 3-file directory sink and the single-wav sink). Accepted as deliberate debt to reuse the extracted buildSegments core without a premature package split. REJECTED hoisting the core into internal/wavconcat now (premature). Trigger-on-objection follow-up: hoist the shared wav-concat core into internal/wavconcat. A self-describing code marker records this in wavfile.go (no issue number)."
  - id: 2026-06-28-earshot-rebuild-server-driven-listener-ui
    title: "Rebuild the listener as a server-driven UI (Earshot); delete the passive player/"
    date: 2026-06-28
    status: accepted
    category: architecture
    tags: [earshot, player, listener-ui, narrate-server, http-bridge, rebuild, leveling-ui, playback]
    path: architecture/2026-06-28-earshot-rebuild-server-driven-listener-ui.md
    summary: "The passive fixture-driven player/ (no session loading, no real play/pause/seek, no resume) cannot serve the real use cases (listen to a chat session, read out a big file). REJECTED extending player/ (Option A) — its passive preview model can't drive session loading + live playback and carries irrelevant fixture/companion/sourcepane machinery. CHOSE Option B: build Earshot (new earshot/ web app) backed by a new local narrate-server HTTP bridge that runs the pipeline (browser can't run Kokoro) and serves audio; delete player/ and its escalate-CLI-card/source-pane/companion code. Narration core (planner, plan schema, per-block leveling+escalation/patch, render, ephemeral+persistent) reused unchanged; no plan-schema fork. New concern: render-id wav lifecycle (temp-dir GC). Companion to the session-source and speak_to_file decisions. Revisit if a hosted/multi-user deployment is ever wanted."
  - id: 2026-06-28-earshot-session-id-via-local-transcript-glob
    title: "Resolve a session ID to a local transcript file by glob — no cloud API"
    date: 2026-06-28
    status: accepted
    category: architecture
    tags: [earshot, session-id, transcript-jsonl, claude-projects, glob, message-list, speak-last, no-cloud-api, no-auth]
    path: architecture/2026-06-28-earshot-session-id-via-local-transcript-glob.md
    summary: "Earshot's session pane needs full chat history for a session ID. User initially framed it as a cloud session ID (implying a claude.ai web API + auth). REJECTED Option A (cloud API) — unknown/unstable surface, auth+token handling, network dependency, scope creep none of it needed. CHOSE Option B: the transcript filename IS the session UUID (~/.claude/projects/<project-hash>/<session-id>.jsonl), so the ID maps to a file by glob; speak_last's lastAssistantText parser already reads these (16 MiB buffer, tool_use vs text handling) with zero auth/network. Generalize that parser into a shared function returning the FULL ordered message list (user+assistant turns); speak_last keeps calling it for last-assistant, narrate-server calls it for all-messages. Big messages chunked via the planner's existing oversized-block splitting (clean seams only). Limitation: only local-transcript sessions are reachable. Revisit if sessions that never touched this machine must be listened to."
  - id: 2026-06-28-speak-to-file-separate-mcp-tool
    title: "Ship speak_to_file as a separate MCP tool, not an option on speak"
    date: 2026-06-28
    status: accepted
    category: architecture
    tags: [earshot, mcp, speak-to-file, speak, single-wav-output, output-path, persistent-sink, tool-surface, priority-tool]
    path: architecture/2026-06-28-speak-to-file-separate-mcp-tool.md
    summary: "Use case 3 wants text OR a .md/text file converted to one wav at a caller-given path (no path → speak). Today MCP speak plays ephemerally only; the persistent sink (CLI-only) writes a 3-file directory (audio.wav+plan.json+manifest.json), not a single wav. REJECTED Option A (add output_path to speak) — overloads a play-only tool with a write-file mode, mixed contract, harder for the client LLM to invoke. CHOSE Option B: separate speak_to_file tool, {text | source, output_path?, level, gender}; input is inline text or a path to a text/.md file; writes one .wav to output_path (file or dir); falls back to speak when no path; reuses persistent-sink wav-concat math WITHOUT writing the JSON sidecars. This is the PRIORITY tool, built first. Settle at build: output_path file-vs-dir resolution rule. Revisit if a caller later needs the full plan/manifest alongside the wav."
  - id: 2026-06-28-earshot-ui-finalized-material-list-detail-bottom
    title: "Earshot UI finalized — Material list-detail, bottom transport, APG-grounded controls"
    date: 2026-06-28
    status: accepted
    category: architecture
    tags: [earshot, player-ui, react, list-detail, material-design-3, w3c-apg, accessibility, transport-bar, per-block-leveling, radiogroup, novel-synthesis, claude-artifacts, honesty-rule, quality-over-reuse, issue-108]
    path: architecture/2026-06-28-earshot-ui-finalized-material-list-detail-bottom.md
    summary: "#108 finalizes the Earshot reference-player UI via huginn research (7/7 load-bearing claims adversarially verified). Material list-detail layout: left net-new session pane, center spoken transcript (rebuilt BlockList/BlockRow), bottom-anchored persistent transport deck (role=toolbar + roving tabindex) replacing raw <audio controls>. Transport anchor decided = BOTTOM (delegated design call) on audio-first convention + clean reading column + block-level 'return to playing' pairing + touch reach; acknowledged tradeoff, cheap one-CSS flip. Interaction grounded in verified W3C APG roles (toolbar/slider-block-quantized-aria-valuetext/listbox/inline-escalate-never-modal). Per-block L1/L2/L3 = accessible radiogroup segmented control (NOT a disclosure — disclosure can't honestly model the L2 middle state), designed as explicit NOVEL SYNTHESIS with cited lineage (Shneiderman details-on-demand + screen-reader verbosity + summarizer length sliders) — weakest-grounded surface, flagged for user-testing. Mockup method = FREE Claude Artifacts (claude.ai Claude Design REJECTED: $20/mo, no free tier, violates no-recurring-spend). Quality-over-reuse steer: reuse usePlayback/escalation LOGIC but rebuild UI to production quality (fixes dual aria-hidden seek wrapper + up-only escalate bugs). Report at research/earshot-ui-design-108/report.md. Revisit anchor + level-control pattern if Artifact user-test disagrees; revisit reuse scope if #107 deletes player/."
  - id: 2026-06-27-oto-v3-4-player-teardown-is-pause-drop-reference
    title: "oto v3.4 player teardown is Pause()+drop-reference (GC finalizer), not Close()"
    date: 2026-06-27
    status: accepted
    category: architecture
    tags: [listen-path, oto, ebitengine, oto-v3.4, player-teardown, pause, finalizer, gc, no-close, sa1019, listenPlayer-seam, bytes-reader, bounded-retention, issue-101, issue-100]
    path: architecture/2026-06-27-oto-v3-4-player-teardown-is-pause-drop-reference.md
    summary: "#101 productionizes the single-path oto v3 listen player, applying the spike finding 2026-06-27-oto-v3-4-player-close-no-op-finalizer-teardown to shipped code. On every n/b/g/replay transition and on cleanup, the prior player is Pause()'d (removes it from oto's active mux set so it stops pulling its source) then its Go reference is dropped; real teardown is the runtime GC finalizer. Player.Close() is NOT called and NOT //nolint:staticcheck-suppressed — in oto v3.4 it is a no-op (returns nil, does not stop the read-pull) and trips SA1019, so it is avoided outright, not suppressed. The listenPlayer seam interface deliberately has NO Close() method so the no-op can't be called by mistake. The PCM source is an in-memory *bytes.Reader (no fd), so the finalizer reclaims only memory (the spike's fd-lifecycle debt does not apply here). Honest invariant — BOUNDED RETENTION: the prior player is Pause()'d and dereferenced on transition, becoming GC-reclaimable via oto's finalizer; NOT a deterministic synchronous free and NOT a process-lifetime leak. Builds on the close-no-op finding and is a sibling to the afplay-fallback-dropped decision on the same single path. Revisit if the PCM source becomes fd-backed."
  - id: 2026-06-27-afplay-listen-fallback-dropped-oto-is-the-sole
    title: "afplay listen fallback dropped — oto is the sole listen engine"
    date: 2026-06-27
    status: accepted
    category: architecture
    tags: [listen-path, oto, afplay, single-engine, build-tags, engine-flag, honesty-rule, device-open-error, no-fallback, zero-cgo, purego, issue-101, issue-100]
    path: architecture/2026-06-27-afplay-listen-fallback-dropped-oto-is-the-sole.md
    summary: "#101 collapses the //go:build oto / !oto tag pair so the in-process oto v3 player is the default and ONLY listen engine in cmd/narrate; the afplay listen fallback and the --engine escape hatch are removed. There is no second listen engine. If oto cannot open an audio device, listen mode returns a wrapped, actionable error UP the pipeline — per the CLAUDE.md honesty rule an engine/device-open failure is an ERROR (stops the pipeline), NEVER a Refusal (refusals are for readable-but-unvoiceable content, not an output device that won't open). The non-interactive speak/MCP path still uses sink/ephemeral afplay independently and is untouched — afplay is dropped only as a LISTEN engine. Rationale: afplay+SIGSTOP structurally cannot give true device-level Pause/Resume (see 2026-06-26-afplay-sigstop-sigcont-no-true-pause), so the fallback was strictly inferior on the listen path's defining feature; maintaining a second engine + build-tag matrix was pure cost. oto v3.4 via purego is zero-CGo (CGO_ENABLED=0 builds) so it is the default with no packaging regression. REJECTED keeping the afplay fallback + --engine hatch. Sibling to the oto-v3.4 teardown decision on the same single path."
  - id: 2026-06-27-oto-v3-4-player-close-no-op-finalizer-teardown
    title: "oto v3.4 Player.Close() is a no-op (finalizer teardown) — overturns the planned Close()-halts-read-pull invariant; spike halts via Pause(), fd-lifecycle fix carried to #101"
    date: 2026-06-27
    status: accepted
    category: architecture
    tags: [listen-path, oto, ebitengine, oto-v3.4, player-close, finalizer, teardown, fd-lifecycle, read-from-closed-fd, pause-then-close, sa1019, go-memory-safe, accepted-debt, spike, issue-100, issue-101]
    path: architecture/2026-06-27-oto-v3-4-player-close-no-op-finalizer-teardown.md
    summary: "Build-session finding for spike #100. The #100 plan mandated per-block teardown ordering 'player.Close() FIRST (stops oto's goroutine pulling the reader), THEN file.Close()', derived from the accepted oto-v3 decision's premise. That invariant is OVERTURNED against the resolved dependency: in github.com/ebitengine/oto/v3 v3.4.0, Player.Close() is documented as 'does nothing and always returns nil' (deprecated; teardown moved to a runtime GC finalizer). So Close() does NOT stop the read-pull, calling it trips SA1019, and the source reader (LimitReader->*os.File) stays alive until the finalizer runs at some later GC — the fd is NOT guaranteed to outlive the player. The spike works around it (Option B) by calling player.Pause() — the v3.4-correct deterministic halt (removes the player from oto's active mux set; a paused player does not pull) — before file.Close(); safe by ear, and the residual read-from-closed-fd window is Go-memory-safe (os.ErrClosed, never UB) and benign at a teardown we are already doing. The deprecated Close() is avoided outright, not nolint-suppressed (golangci-lint --build-tags oto = 0 issues). Production-grade finalizer-aware fd ownership (oto owns an io.ReadCloser, or reader retained until player collected, so the fd always outlives the player) is carried to #101 as tracked accepted debt — Option C, out of scope for a by-ear spike. Marker cmd/narrate/listen_oto.go:118. Corroborated in review by Error Handling + Tech Debt (highest-signal item). Amends, does NOT supersede, 2026-06-27-true-pause-via-oto-v3-no-cgo-in-process-player.md — the oto-v3 choice stands and is device-confirmed; only the plan-derived teardown sub-invariant is corrected against the as-resolved v3.4.0 API. Revisit when #101 begins."
  - id: 2026-06-27-true-pause-via-oto-v3-no-cgo-in-process-player
    title: "Listen-path true Pause/Resume via ebitengine/oto v3 in-process PCM player — no CGo, decoupled from the deferred sherpa-onnx CGo renderer"
    date: 2026-06-27
    status: accepted
    category: architecture
    tags: [listen-path, true-pause, pause-resume, oto, ebitengine, purego, no-cgo, in-process-renderer, pcm, kokoro, apache-2.0, sherpa-onnx, device-confirmed, issue-92, issue-84]
    path: architecture/2026-06-27-true-pause-via-oto-v3-no-cgo-in-process-player.md
    summary: "huginn research (#92) OVERTURNS the ticket premise that listen-path true Pause/Resume is deferred until the sherpa-onnx CGo renderer phase. Adopt github.com/ebitengine/oto/v3 as the in-process PCM player: zero CGo on macOS (purego), native io.Reader-of-PCM with library-managed freeze position (Player.Pause()/Play()), Apache-2.0, consumes 24 kHz mono int16 PCM (Kokoro native, no source resampling). Earned over raw-CoreAudio-CGo / malgo / portaudio across 5 axes — oto is the only zero-CGo, only io.Reader-of-PCM, only Apache-2.0 pick; the three competitors all reach resume-from-freeze but only via stop/start with app-managed position + fill-callback and all need CGo. DEVICE-CONFIRMED on-device probe (go1.26.1 darwin/arm64, oto v3.4.0, CoreAudio): pause-window delta 0 bytes / 0.0 ms, resume continuous from frozen offset 96000 -> 168000 with no restart. All 7 load-bearing claims verified or device-confirmed; none contested. EXTENDS decision 2026-06-26-afplay-sigstop-sigcont-no-true-pause.md and resolves its CGo-renderer revisit trigger WITHOUT CGo (the trigger condition need never be met); #84's afplay finding stands, not superseded. Research/decision output only — no implementation. Build follow-ups (spike + gated integration) tracked separately."
  - id: 2026-06-27-transcript-omitted-refused-count-additive-field
    title: "Report dropped refused blocks via additive transcript_omitted_refused_count on MCP speak receipt"
    date: 2026-06-27
    status: accepted
    category: architecture
    tags: [transcript-cap, mcp, speak, refusal-aware, omission-accounting, additive-compatible, omitempty, byte-identical, channel-2-no-text-leak, honesty-rule, issue-98, issue-86]
    path: architecture/2026-06-27-transcript-omitted-refused-count-additive-field.md
    summary: "Follow-on to #86 (named there as deferred refusal-aware omission accounting). When a >200-block document's truncated transcript tail drops refused blocks, the MCP speak receipt reports HOW MANY via a new additive omitempty field transcript_omitted_refused_count (enum-sourced: entry.Status == string(plan.StatusRefused)). Chose Option A (count field) over Option B (refusal-preferring retention). Option B REJECTED: breaks plan-order contiguity (same property that killed elide-middle in #86), makes capTranscript non-pure/allocation-bearing, and re-adds spoken/source text to the wire (violates Channel-2 no-text-leak). Count is additive-compatible, byte-identical under cap (omitempty elides), keeps capTranscript pure. Honesty rule untouched (audio+plan unaffected; refused blocks still spoken and counted in blocks_played). Accepted limitation: count-only, not an exhaustive refusal ledger. File cmd/narrate-mcp/main.go."
  - id: 2026-06-27-cap-mcp-speak-transcript-head-keep-tail-truncate
    title: "Cap the MCP speak per-block transcript via head-keep tail-truncate by entry count"
    date: 2026-06-27
    status: accepted
    category: architecture
    tags: [transcript-cap, mcp, speak, head-keep, tail-truncate, observability, additive-compatible, omitempty, byte-identical, honesty-rule, issue-86, adr-77]
    path: architecture/2026-06-27-cap-mcp-speak-transcript-head-keep-tail-truncate.md
    summary: "Issue #86 bounds the previously-unbounded, double-shipped (structuredContent + serialized TextContent, ADR #77 D3) per-block transcript[] on the MCP speak receipt. DECISION: HEAD-KEEP TAIL-TRUNCATE by entry count via a single named const transcriptMaxEntries = 200, signaled by two additive omitempty fields (transcript_truncated, transcript_omitted_count) on speakResponse. A pure, total, allocation-free capTranscript helper is applied at the single runSpeak success-path site (projection transcriptFromResult and bounding stay independently testable; dual-channel handler untouched — capping the one struct before its single marshal bounds both channels). Under cap returns the input slice unchanged (false,0) so both fields elide and the wire response is BYTE-IDENTICAL to before; over cap returns entries[:200] with omitted=len-cap; cap<=0 is a defensive no-op; error path untouched (nil transcript). REJECTED elide-middle (breaks the listen transport's plan-order contiguity — consumers walk transcript[] from Order=0, a contiguous 0..N-1 prefix is exactly what a front-to-back listener reaches first); byte-budget cap (non-deterministic, forces mid-entry SpokenText truncation which leaks a partial-spoken-text variant, barred by the Channel-2 no-text-leak decision); caller-tunable speakArgs field (deferred — v1 envelope stays minimal). Bounding an observability receipt is NOT fabrication: honesty rule untouched (audio + plan unaffected, refused blocks still spoken at playback time and still counted in receipt.blocks_played from SinkReceipt), and a truncated transcript is documented as NOT an exhaustive refusal ledger in both the cap-site comment and the client-visible tool Description. Relates to ADR #77 (playback observability), the receipt-only-envelope decision, and the Channel-2 no-text-leak decision. Known follow-on (a deferred TASK, not a decision): refusal-aware omission accounting when a >200-block document drops tail refusals."
  - id: 2026-06-27-table-degrade-gist-shares-leveltable-parser
    title: "Table L2/L3 degrade gist and levelTable share one parseTable — byte-identity is structural, not test-dependent"
    date: 2026-06-27
    status: accepted
    category: convention
    tags: [planner, levelTable, deterministicTableGist, parseTable, degrade, byte-identical, honesty-rule, dry, issue-47, issue-48]
    path: convention/2026-06-27-table-degrade-gist-shares-leveltable-parser.md
    summary: "Issue #47's table-at-L2/L3 work creates the same dual-path byte-identity hazard as code (#48): the deterministic header/row reading must be produced from both levelTable (the headers fact) and the no-adapter degrade path (degraded reading with Status=degraded when L2/L3 wanted an AI meaning-summary but no adapter is wired). DECISION: extract one MANDATORY shared parseTable consumed by BOTH levelTable and the gist, with deterministicTableGist as a thin formatter over that shared parse — so byte-identity between the requested-but-degraded reading and the deterministic reading is STRUCTURAL (same parse, divergence impossible) rather than test-dependent. This is the table analog of #48's deterministicCodeGist shared helper but a deliberately STRONGER guarantee: #48's byte-identity rests on a call-site doc-comment contract the compiler can't enforce, whereas the table case shares the parse step itself. Resolves #48's 'second structured class' revisit trigger by sharing the parser rather than lifting a generic levelResult.deterministicFallback field (still rejected as YAGNI). REJECTED Option B (copy #48's shape — shared gist builder + doc-comment contract) because tables had a cleaner seam to make the guarantee structural. The honesty rule (degraded reading must be an exact non-fabricated rendering of source cells) is now enforced at the parse layer. The class-specific TableUserL3 prompt added for symmetry with TableUserL2 was judged too minor to journal."
  - id: 2026-06-26-render-failure-server-player-readiness
    title: "Render-failure signal is a server/player readiness collaboration (on-disk truth + client-owned give-up bound), not a render-side sentinel"
    date: 2026-06-26
    status: accepted
    category: architecture
    tags: [architecture, http-api, react, readiness, mcp, ttsplayer, issue-76, render-failed, no-out-dir]
    path: architecture/2026-06-26-render-failure-server-player-readiness.md
    summary: "Ticket #76 Part B (optional React visual companion for MCP listen-mode). The render-failure signal is modeled as a SERVER/PLAYER COLLABORATION — on-disk truth + a client-owned give-up bound — not a render-side sentinel. cmd/narrate-server exposes GET /readiness?dir= as a tri-state: 200 {status:rendered} (plan.json+manifest.json+audio.wav all present & non-empty), 200 {status:rendering} (dir exists, triple incomplete), and a NEW closed-enum reason token no_out_dir (HTTP 404) when the dir is absent. no_out_dir is APPENDED to the server's closed append-only reason enum (sibling to existing source_not_found, which stays scoped to the escalate/artifact source-resolution path). The player owns the bounded give-up: useRenderReadiness polls at 1 Hz with MAX_POLL_ATTEMPTS=120 absolute cap + UNREACHABLE_FASTFAIL=8 consecutive-transport-reject dead-server fast path, collapsing 'never completes' / 'server gone' into a single DEFINED terminal phase render_failed — zero perpetual spinners even when the server is dead from poll #1. The 'rendered' guarantee depends on the persistent sink publishing each leaf via atomic tmp+os.Rename (a size>0 check would otherwise risk reading a truncated streamed file). REJECTED (a) reusing source_not_found for the dir-absent case — AC3 names 'no-outDir' as a distinct outcome and the append-only enum exists to add a clear machine-stable token, not overload one; (b) a render-side failure SENTINEL FILE written by the sink — the server reports on-disk truth only, a crashed-mid-render is caught by the player's give-up bound, keeping the sink contract unchanged. Standing order honored: companion artifacts come from a SEPARATE cmd/narrate --sink persistent invocation, never a tee off speak (decoupling; Risk 6 / AC6); speak's ephemeral play-then-delete lifetime stays intact."
  - id: 2026-06-26-afplay-sigstop-sigcont-no-true-pause
    title: "afplay SIGSTOP/SIGCONT cannot deliver a true Pause; honest Stop/Replay block stands"
    date: 2026-06-26
    status: accepted
    category: architecture
    tags: [listen-path, cmd-narrate, afplay, sigstop, sigcont, pseudo-pause, true-pause, coreaudiod, audioqueue, honesty-rule, by-ear-verify, issue-84, issue-79, issue-83]
    path: architecture/2026-06-26-afplay-sigstop-sigcont-no-true-pause.md
    summary: "Issue #84 by-ear /verify spike resolves the #79 revisit trigger (would the afplay SIGCONT resume seam be clean enough to promote Stop/Replay to a true Pause?). Answer NEGATIVE: SIGSTOP on the afplay PID does not pause audible playback at all. afplay front-loads the whole block PCM into the CoreAudio AudioQueue; coreaudiod (separate, un-frozen daemon) drains it to completion while the afplay process sits frozen (state T). Measured: freezing afplay 6-8s added only ~0.2-0.4s of wall-clock over baseline at 10s AND 60s clip lengths (a real pause would add ~the full freeze window). SIGSTOP freezes the wrong process. DECISION: keep honest Stop/Replay (#83); do NOT file the promote-to-Pause follow-up (falsified); a true Pause needs the deferred sherpa-onnx-go CGo renderer's AudioQueuePause, not OS signals. Probes + report under research/afplay-sigcont-pause-seam-84/."
  - id: 2026-06-26-channel2-mechanism-jsonl-tail-over-mcp-progress
    title: "Channel-2 live observer mechanism: append-only JSONL + tail -f, not MCP notifications/progress"
    date: 2026-06-26
    status: accepted
    category: architecture
    tags: [observability, channel-2, jsonl, tail, mcp-progress, notifyprogress, decoupled-observer, narrate-observe, issue-81, issue-77]
    path: architecture/2026-06-26-channel2-mechanism-jsonl-tail-over-mcp-progress.md
    summary: "Issue #81 builds ADR #77 D5 Channel 2 (a user-launched 2nd-terminal live observer). Mechanism DECISION: append-only JSONL + tail -f is the DEFAULT, beating native MCP notifications/progress (Session.NotifyProgress, go-sdk v1.5.0) for THIS surface — JSONL surfaces to a USER in a 2nd terminal (the D5 requirement), zero deps, ephemeral /tmp no durable coupling, live (emitted before each blocking play()); NotifyProgress surfaces to the MCP CLIENT not a human terminal and is invisible without client progressToken cooperation. NotifyProgress is DEFERRED not rejected — add additively alongside JSONL (same BlockEvent shape, guarded by the schema/v wire discriminator) when a real MCP-client UI wants progress. cmd/narrate-observe is the tail reader; opt-in via NARRATE_OBSERVE_FILE > NARRATE_OBSERVE truthy > off; off keeps the speak response byte-identical; no source/spoken text on the wire (secret-leak avoidance); 0600 scratch file."
  - id: 2026-06-26-observer-seam-sink-reads-plan-param
    title: "Observer seam placement: the sink reads Level/Status from the plan param it already receives, not a cmd-side BlockSummary closure"
    date: 2026-06-26
    status: accepted
    category: architecture
    tags: [observer-seam, sink-ephemeral, composition-root, blocksummary, liveness, import-wall, sub-blocks, issue-81, issue-77, plan-v2-deviation]
    path: architecture/2026-06-26-observer-seam-sink-reads-plan-param.md
    summary: "Issue #81 observer seam placement. The mimir plan v2 specified sink/ephemeral stay roster-free emitting only BlockTiming, with a cmd/narrate-mcp closure enriching Level/Status from pipeline.BlockSummary (justified by BlockSummary flattening oversized-split sub-blocks). Step 0 verification FALSIFIED this: (1) the closure is unbuildable — BlockSummaries exist only AFTER Narrate returns but the observer must be wired into the sink BEFORE Narrate runs; (2) the sub-block rationale is moot phase one — no producer populates Block.SubBlocks and render/sherpa emits one BlockTiming per top-level Block in plan order, so Timeline.Blocks and plan.Blocks are 1:1 co-ordered. DECISION: the sink reads Level/Status from the plan.NarrationPlan param it ALREADY receives (historically discarded), correlated 1:1 by BlockID, zero-Level/Status fallback on a miss (never fabricated); cmd owns only the concrete JSONL marshal + scratch lifecycle. Keeps liveness AND import-cleanliness (plan/ already imported; import set unchanged, now guarded by sink/ephemeral/deps_test.go). Playing is derived purely from AudioRef != empty — a refused block whose Refusal.Message is voiced is Playing:true (the plan's 'refused implies Playing:false' truth table was wrong; caught in review). Deviates from approved plan v2; surfaced + recorded."
  - id: 2026-06-25-listen-transport-keypress-loop-not-tui
    title: "LISTEN-path terminal transport: minimal raw-mode keypress loop, not a full-screen TUI; honest \"Stop / Replay block\", not \"Pause\""
    date: 2026-06-25
    status: accepted
    category: architecture
    tags: [listen-path, cmd-narrate, transport-ui, keypress-loop, tui, bubbletea, tview, tcell, x-term, raw-mode, afplay, honesty-rule, sigstop-sigcont, pseudo-pause, phase-one-weight-discipline, issue-79, issue-77]
    path: architecture/2026-06-25-listen-transport-keypress-loop-not-tui.md
    summary: "Issue #79 (rune analysis — a design, not code) designs the LISTEN-path terminal playback transport for the standalone cmd/narrate CONTROLLER (owns its tty, drives serial afplay, holds the temp dir) — NOT the ADR #80 Channel-2 observer, and cmd/narrate-mcp gains no cross-call state/daemon. Three coupled picks. FORK A (form): minimal raw-mode KEYPRESS LOOP (stdlib + golang.org/x/term, ~1 new module), NOT a full-screen TUI — phase-one weight discipline is the deciding axis since all three candidates are pure-Go/no-CGo; rejected tview+tcell (~8 build modules) and charm.land/bubbletea/v2 (~16 build / 21 full — heaviest, and its v2 import path churned off github to charm.land). Reconsider a TUI only on search/filter/scrollback. FORK B (input): raw-mode SINGLE-KEY via x/term (n/b/space/g/q), NOT line-prompt — the raw-mode restore hazard collapses into the same SIGINT handler temp-dir cleanup already needs. HONESTY CALL: ship 'Stop / Replay block' (stop afplay + replay-block-from-start), NOT a bare 'Pause' — afplay has no runtime pause/seek/position IPC (all start-time params), so implying mid-block resume violates the CLAUDE.md honesty rule. SIGSTOP/SIGCONT is a genuine OS mid-block pause on Darwin and could earn a true 'Pause' — GATED on a by-ear test that the SIGCONT resume seam (CoreAudio buffer state) is audibly clean (single open item). Resilience (probe-verified): one SIGINT/SIGTERM handler does term.Restore + Kill&Wait the afplay child (Process.Kill is async; reap before next afplay so no overlap) + os.RemoveAll the once-per-session temp dir (idempotent); Go's default signal disposition skips deferred cleanup so the handler is mandatory; prefer exec.Cmd.WaitDelay over a hand-rolled grace. Contract binds real plan/ names: Block.ID, Block.Segments[].Text (plural), Block.Level, Block.Status (voiced/degraded/refused), BlockTiming{BlockID,StartMs,EndMs,AudioRef}; StartMs/EndMs are display-only; refused/zero-duration blocks shown in transcript but skipped for navigation; navigation plays whole segment files, never sub-block seeks. Evidence-graded source: research/listen-transport-ui-issue-79/report.md (v1); deliverable: docs/design/listen-transport-ui.md."
  - id: 2026-06-24-playback-observability-control-model-issue-77
    title: "ADR: Playback observability & control model (issue #77)"
    date: 2026-06-24
    status: accepted
    category: architecture
    tags: [observability, transport-control, receipt-envelope, transcript, oto, afplay, decoupled-observer, jsonl-tail, sse, mcp-tool-result, mcp-progress-notifications, block-level-sync, issue-77, issue-78, issue-79]
    path: architecture/2026-06-24-playback-observability-control-model-issue-77.md
    summary: "Issue #77 ADR, RE-RESEARCHED + adversarially verified 2026-06-24 (9 load-bearing claims through blind claim-verifiers: 8 VERIFIED, 1 single-source). SUPERSEDES the draft's single-channel-only D5. The five #72/#73 accepted decisions stay CONFIRMED. DECISIONS: D1 (CONFIRMED+refined) — afplay phase one; oto v3.4.0 (Apache-2.0, no-CGo purego) phase two; full Seek sig is Seek(offset int64, whence int)(int64,error) and needs the source to implement io.Seeker. D2 (CONFIRMED+flag) — seek/back = previous-block; WAV byte = dataChunkStart + floor(StartMs/1000 × 24000) × BlockAlign — the ×2 is mono-16-bit-only, general code uses NumChannels × BitsPerSample/8, and dataChunkStart MUST be RIFF-parsed (a LIST/fact/cue chunk can precede data), not hardcoded 44 (sandbox probe landed data at 70). D3 (CONFIRMED+correction) — transcript in structuredContent + one duplicate serialized-JSON TextContent; CORRECTION: the draft's 'CC renders inline, untruncated, no collapse' is REFUTED — the CC CLI COLLAPSES tool results to one line by default, ctrl+o expands, so the TextContent is the model's after-the-fact record not a live display; CC ~v2.1.x drops text content when structuredContent present (inverse at v1.0.60), go-sdk v1.5.0 ToolHandlerFor auto-mirrors. D4 (CONFIRMED, wording DOWNGRADED to single-source) — transport control over a fully-rendered buffer ≠ streaming generation; the strong 'orthogonal/opposite of streaming' framing softened (GStreamer GstBaseSrc shows a push/streaming source can also be seekable). D5 — **FLIPPED** to a complementary TWO-CHANNEL model: CHANNEL 1 = inline MCP receipt (D3, after-the-fact, kept from draft); CHANNEL 2 = opt-in USER-launched DECOUPLED OBSERVER in a 2nd terminal/browser reading an ephemeral side channel — the ONLY surface that shows LIVE during-playback progress. CRUXES: MCP/go-sdk v1.5.0 tool calls are SYNCHRONOUS (CONFIRMED in-repo: sink/ephemeral Consume + playWithAfplay block on cmd.Wait() per block, so the speak handler returns only AFTER playback ends — receipt can't carry live progress); MCP stdio reserves only the SERVER stdout, a side channel (socket/FIFO/file/HTTP) breaks no rule; SCOPE GAP — the draft only weighed a same-terminal TUI and a server-spawned osascript window, never a user-launched 2nd-terminal observer, and raw mode is per-tty (verified) so there is no raw-mode conflict. DEFAULT side channel = append-only JSONL + tail -f (zero-dep, ephemeral, dodges durable-sink guardrail); localhost SSE (http.Flusher) is the richer opt-in; unix socket carries the macOS sun_path=104 caveat (use short /tmp path); FIFO most fragile. NEW ALT (note, not default): native MCP notifications/progress (Session.NotifyProgress, go-sdk v1.5.0) from inside the blocking handler — lighter but surfaces to the CLIENT not a user view; worth a spike. REJECTED: single-channel receipt-only as the complete story, same-terminal TUI, server-spawned osascript window — these do NOT extend to the user-launched observer. React player stays optional companion. OPEN PINS: #78 oto transport + transcript[] additive field + additive outputSchema; #79 data contract = D3 shape; NEW cmd/narrate-observe decoupled-observer spike + optional localhost SSE."
  - id: 2026-06-23-code-min-level-floor-on-planner-request
    title: "Code blocks default to L2 in listen-mode via an additive CodeMinLevel floor on planner.Request"
    date: 2026-06-23
    status: accepted
    category: architecture
    tags: [code-min-level, listen-mode, planner-request, pipeline-defaults, per-block-leveling, floor-field, api-contract, schema-neutral, issue-73, classcode]
    path: architecture/2026-06-23-code-min-level-floor-on-planner-request.md
    summary: "Issue #73 Part A implementation (distinct from the #72/#74 listen-not-read ADR). Code-blocks-default-to-L2 in listen-mode is expressed as an additive declarative FLOOR field `CodeMinLevel plan.Level` on `planner.Request`, read inside the existing structural pass as `target = max(effectiveLevel, CodeMinLevel)` for ClassCode only, and surfaced via `pipeline.PipelineDefaults` set by `cmd/narrate-mcp` newPipeline. WHY a floor not a hard-set: an explicit L3 listen request must survive (max keeps the higher), and the zero value is a no-op preserving zero drift for non-listen callers. WHY on planner.Request not plan.PlanDefaults: the ephemeral primary path needs no persisted round-trip, keeping the on-wire plan.json schema untouched and engine-neutral; and because block IDs/classes are assigned INSIDE Plan, a per-class floor must be a planner-read field, not composition-root Overrides resolution (which is block-ID-keyed and runs at the wrong layer). REJECTED a general `map[plan.Class]plan.Level` per-class override map as speculative generality — a single CodeMinLevel covers all of #73; revisit if a future ticket needs per-class floors for diagram/table. Resolves the open pin from `primary-listen-path-decoupled-from-durable-sink` on where the L2 listen-mode default lives. Standing honesty boundary reaffirmed (reuse of existing #48 behavior, not a new decision): code-L2-no-adapter = degraded, adapter-refuses = refused."
  - id: 2026-06-23-terminal-listen-not-read-is-ephemeral-afplay-audio-only
    title: "Terminal 'listen, not read' is the existing speak → ephemeral sink → afplay path (audio-only, no UI)"
    date: 2026-06-23
    status: accepted
    category: architecture
    tags: [mcp, speak, ephemeral-sink, afplay, listen-not-read, terminal, ticket-72, issue-73, v3-adr]
    path: architecture/2026-06-23-terminal-listen-not-read-is-ephemeral-afplay-audio-only.md
    summary: "Ticket #72 v2→v3 ADR correction. The primary terminal 'listen, not read' path is the EXISTING speak → sink/ephemeral → afplay path: audio out of the speakers, no UI, already shipped. Verified (file:line): cmd/narrate-mcp wires ephemeral.New() which shells out to afplay and plays each per-block WAV before returning a receipt-only envelope and deleting its temp dir; the MCP host only renders the receipt as text — the narrate-mcp process plays the sound itself. SUPERSEDES the v2 approach that made cmd/narrate-server + persistent outDir + React browser player the listen path (over-engineered: nothing in Go auto-opens a browser; the player is hand-launched). Two-surface friction and render-failure-spinner machinery re-scoped to the optional visual path only; terminal path has no spinner/no browser tab; failures are normal MCP tool errors. Partial-playback-then-error is defined behavior for #73; on this path a refusal is heard not seen."
  - id: 2026-06-23-react-player-optional-reuses-existing-player-50
    title: "React player is optional and reuses the existing player (#50) — no new UI"
    date: 2026-06-23
    status: accepted
    category: architecture
    tags: [react-player, visual-companion, narrate-server, issue-50, opt-in, reuse, ticket-72, issue-73, v3-adr]
    path: architecture/2026-06-23-react-player-optional-reuses-existing-player-50.md
    summary: "Ticket #72 v3 ADR. Optional visual sync (on-screen block highlight + transport + escalation + spinner) reuses the EXISTING React reference player from issue #50; NO new UI is built. It is a separate, opt-in, hand-launched path (player + cmd/narrate-server against a durable persistent outDir), not part of the primary terminal flow and not auto-opened (narrate-server only prints its URL to stderr; player launched via pnpm dev). The render-failure-spinner machinery and two-surface friction apply only to this optional path. Pin on #73: exact Makefile target vs underlying pnpm dev (Makefile:103)."
  - id: 2026-06-23-primary-listen-path-decoupled-from-durable-sink
    title: "Keep the primary listen path decoupled from any durable sink (standing guardrail)"
    date: 2026-06-23
    status: accepted
    category: tradeoff
    tags: [guardrail, decoupling, persistent-sink, ephemeral-sink, speak, sink-lifetime, two-invocations, ticket-72, issue-73, v3-adr]
    path: tradeoff/2026-06-23-primary-listen-path-decoupled-from-durable-sink.md
    summary: "Ticket #72 v3 ADR. The single highest-value guardrail for #73 (CORROBORATED: Dependency & Coupling + Tech Debt Sentinel). The primary terminal narration path (speak → ephemeral → afplay) STAYS DECOUPLED from the persistent/durable sink. A visual companion that needs persisted artifacts is a SEPARATE --sink persistent invocation, NOT a tee off the speak call — 'two paths, one render core'. This is a standing order constraining follow-up #73 so an implementer does not re-introduce sink-lifetime coupling. Keeps speak's ephemeral 'play then delete temp dir' lifetime intact; out_dir on the receipt is a debug-window field clients must not depend on. Open pin for #73: where the L2 listen-mode default lives (speakArgs/PipelineDefaults vs caller-passed level)."
  - id: 2026-06-23-mcp-sdk-discrepancy-note-for-73
    title: "MCP SDK discrepancy note — ticket says mark3labs/mcp-go, project uses official go-sdk (open question for #73)"
    date: 2026-06-23
    status: revisit-later
    category: library-choice
    tags: [mcp-sdk, mark3labs, modelcontextprotocol-go-sdk, discrepancy, note, open-question, ticket-72, issue-73, v3-adr]
    path: library-choice/2026-06-23-mcp-sdk-discrepancy-note-for-73.md
    summary: "Ticket #72 v3 ADR review. NOTE / open question for follow-up #73 (not an action now; the ADR ships no code): the originating ticket text refers to MCP SDK mark3labs/mcp-go, but the project actually uses the OFFICIAL github.com/modelcontextprotocol/go-sdk v1.5.0 (the SDK whose transitive requirement bumped the Go minimum to 1.25 in #12). #73 should treat the official go-sdk as authoritative and correct/disregard the ticket's mark3labs/mcp-go reference. Recorded so #73 inherits the context and an implementer does not pull the wrong SDK. Status revisit-later: close once #73 confirms the SDK choice."
  - id: 2026-06-22-list-title-detection-firstitemdemarkered-seam
    title: "Issue #54 colon-gated list-title detection resolved via upstream firstItemDemarkered seam; AC1 taken as documented divergence"
    date: 2026-06-22
    status: accepted
    category: planner
    tags: [planner, level.go, segment.go, list-voicing, honesty-rule, issue-54]
    path: planner/2026-06-22-list-title-detection-firstitemdemarkered-seam.md
    summary: "Issue #54. splitListTitle (planner/level.go) used a single signal — a trailing colon on the first non-marker line — to decide if a list has a leading title, but goldmark strips the first item's marker so that line is ambiguous (real title vs de-markered item one). DECISION (Option B-markdown): the segmenter sets a planner-internal firstItemDemarkered flag on rawBlock (set in rawBlockFromNode as hint == hintList), threaded into splitListTitle; when true the colon title branch is skipped and the colon-terminated first item is counted in N. This kills Direction 2 (AC2 — a bare list whose genuine first item ends in a colon mis-promoted to a title) by construction on the goldmark path — exact, not heuristic; AC3/AC4/AC5 fall out. No plan/ schema change, no planner I/O, honesty rule preserved. KEY RECORDED DECISION — AC1 (non-colon leading label as preamble) taken as wontfix-by-design divergence: in true markdown that title is already voiced correctly as its own top-level prose block (goldmark doesn't fold a preceding label paragraph into the list), and on the plaintext-fallback path it's indistinguishable from a marker-less first item without guessing, which would violate the honesty rule. Supersedes 2026-06-21-colon-gated-list-title-detection-goldmark (resolves the false positive it knowingly accepted). Latent debt (accepted): the trailing-colon branch is retained for the plaintext-fallback shape (firstItemDemarkered == false), of uncertain reachability. Rejected: Option A (narrow the colon heuristic via marker-stripped asymmetry — a heuristic explaining a heuristic; AC1 relaxation risky); Option B-plaintext titled-seam segmenter change (fold adjacent label into list — deferred, re-opens the top-level-walk tradeoff). Revisit if literal AC1 is ever wanted or the plaintext-fallback colon branch becomes reachable."
  - id: 2026-06-22-code-l2-overlong-reply-trim-to-first-sentence
    title: "Over-long code-L2 reply: trim to first sentence, refuse only when the first sentence overruns"
    date: 2026-06-22
    status: accepted
    category: tradeoff
    tags: [code-l2, intelligence, honesty-rule, refuse-too-large, trim, planner, issue-60]
    path: tradeoff/2026-06-22-code-l2-overlong-reply-trim-to-first-sentence.md
    summary: "Issue #60 enforces the CodeUserL2 one-sentence/~30-word cap on code-L2 adapter replies. DECISION (Option A): for an over-long reply, TRIM to the first sentence (the adapter's own words cut at a clean terminator seam = honest), with the 30-word count as a HARD ceiling that triggers REFUSAL (RefuseTooLarge) only when even the first sentence overruns. Rejected (b) refusing the whole reply whenever it exceeds the cap (loses voiceable honest content) and (c) mid-sentence truncation (fabrication — violates the honesty rule). Enforcement at the single callIntelligence choke point in planner/planner.go, guarded ClassCode && L2, reusing the existing RefuseTooLarge sentinel. Sub-decision: first-sentence scan requires whitespace/EOF after the terminator (keeps 'v1.5.0' intact) and accepts early-cut on abbreviations like 'e.g.' as honest over-trimming; standalone helper, deliberately NOT routed through splitProse (its proseMaxChars/2 size floor would defeat the trim). Extends 2026-06-22-code-semantic-gist-l2-only; applies the refuse-sentinel convention from 2026-06-20-mcpsampling-refuse-sentinel-token. Revisit if adapters reliably honor the budget or a second class needs the same trim."
  - id: 2026-06-22-artifact-route-serves-live-dir-resolves-refetch
    title: "Read-only GET /artifact route serves the live escalated dir; player re-fetch resolves against it"
    date: 2026-06-22
    status: accepted
    category: architecture
    tags: [player, escalate, server-mode, artifact-route, live-dir, path-traversal, allowlist, EvalSymlinks, loopback, CORS, issue-62]
    path: architecture/2026-06-22-artifact-route-serves-live-dir-resolves-refetch.md
    summary: "Issue #62 added a read-only GET /artifact?dir=&name= route to cmd/narrate-server that statically serves only {manifest.json, audio.wav} from the escalated dir. Containment is allowlist-before-join (name must be one of two permitted filenames) + filepath.EvalSymlinks + filepath.Rel boundary check — deliberately NOT a raw string-prefix comparison. It rides the existing loopback bind + pinned CORS, so no new exposure. The player resolves its re-fetch base against the live dir via a pure resolver reading effect-synced refs at call time, so repointAudio/reloadManifest now point at the user-supplied dir instead of FIXTURE_BASE. Supersedes the prior FIXTURE_BASE-relative re-fetch limitation."
  - id: 2026-06-22-artifact-read-side-per-dir-mutex
    title: "Read-side per-dir mutex in /artifact keyed like the escalate writer, over accepting a torn-read window"
    date: 2026-06-22
    status: accepted
    category: concurrency
    tags: [persistent-sink, commitPatch, atomic-rename, torn-read, per-dir-mutex, filepath-abs, artifact-route, escalate, issue-62]
    path: concurrency/2026-06-22-artifact-read-side-per-dir-mutex.md
    summary: "internal/sink/persistent commitPatch writes plan.json/manifest.json/audio.wav each via atomic tmp+rename, but renames them sequentially (audio LAST), so a concurrent reader can observe a torn cross-file state (new manifest + old audio). DECISION: the /artifact handler holds a read-side per-dir mutex keyed on filepath.Abs(dir) — the SAME key /escalate's writer holds — rather than accepting the cross-file observation window as debt. Reader and writer contend on the same per-dir lock, guaranteeing a consistent {plan,manifest,audio} triple. Underlying sequential-rename non-atomicity remains; the read-side lock compensates. Revisit if commitPatch is ever made cross-file atomic."
  - id: 2026-06-22-planner-fanout-first-error-wins-errgroup
    title: "First-error-wins via errgroup, not errors.Join, in planner fan-out"
    date: 2026-06-22
    status: accepted
    category: architecture
    tags: [planner, concurrency, errgroup, error-handling, stop-semantics, issue-46]
    path: architecture/2026-06-22-planner-fanout-first-error-wins-errgroup.md
    summary: "Issue #46 parallelizes per-block intelligence Voice calls. DECISION: use errgroup first-error-wins — the first failing worker's error propagates and cancels the shared context; deliberately NOT errors.Join. Rationale: preserves the serial planner's stop-on-first-error contract (an error stops the pipeline; the pipeline doesn't need the full failure set), and aggregating concurrent errors would be non-deterministic for no consumer benefit. Refusals remain data and are unaffected. Revisit if a batch-validation mode ever needs all concurrent failures surfaced at once."
  - id: 2026-06-22-planner-fanout-bounded-concurrency-default-4
    title: "Bounded Voice fan-out concurrency default of 4 (defaultIntelligenceConcurrency)"
    date: 2026-06-22
    status: accepted
    category: tradeoff
    tags: [planner, concurrency, anthropic, rate-limit, 429, hobby-key, issue-46]
    path: tradeoff/2026-06-22-planner-fanout-bounded-concurrency-default-4.md
    summary: "Issue #46 fans out per-block intelligence Voice calls (Anthropic API on a hobby key). DECISION: bound the fan-out with a limiter defaulting to 4 (defaultIntelligenceConcurrency). Rejected unbounded one-goroutine-per-block (burst 429s, impolite to a hobby/free endpoint). Rationale: 4 gives a clear speedup over serial for multi-block docs while keeping in-flight requests low enough to avoid burst rate-limits; local-only project with no recurring spend trades throughput for politeness. Single named knob, easy to raise later. Revisit if a higher-tier key or in-process backend removes rate-limit pressure."
  - id: 2026-06-22-planner-fanout-positional-diagnostics-slotting
    title: "Diagnostics positionally slotted per-block, flattened after g.Wait() in index order"
    date: 2026-06-22
    status: accepted
    category: architecture
    tags: [planner, concurrency, errgroup, determinism, race-freedom, diagnostics, issue-46]
    path: architecture/2026-06-22-planner-fanout-positional-diagnostics-slotting.md
    summary: "Issue #46 parallelizes per-block intelligence Voice calls in the pure no-I/O planner, whose output (incl. Diagnostics) must be deterministic. DECISION: each worker writes into a positionally-owned blockResult{block,diags} entry indexed by block position and never touches out.Diagnostics; the main goroutine flattens []blockResult into out.Diagnostics in index order only after the errgroup g.Wait() barrier. Rejected goroutine-appending to a shared out.Diagnostics slice (data race + scheduling-dependent, non-deterministic order, breaks golden plan.json fixtures). Rationale: disjoint index ownership = race-free; index-order flatten = deterministic, matches prior serial order, keeps go test -race clean and fixtures valid."
  - id: 2026-06-22-player-playback-unit-stays-whole-audio-wav
    title: "Player playback unit stays the whole audio.wav; re-point only the playing block on patch"
    date: 2026-06-22
    status: accepted
    category: architecture
    tags: [player, escalate, block-level-sync, audio-playback, react, issue-50]
    path: architecture/2026-06-22-player-playback-unit-stays-whole-audio-wav.md
    summary: "Task #50 in-place Escalate buttons. The player's playback unit stays the single whole audio.wav blob (honoring the block-level-sync invariant). On a block patch: re-point/re-fetch the blob ONLY when the patched block is the one currently playing; otherwise just update manifest offsets in state. Rejected per-block <audio> elements — that drifts toward the word/segment-level sync the core invariant forbids."
  - id: 2026-06-22-useplayback-reset-on-block-id-signature-not-manifest-identity
    title: "usePlayback tracking-reset keyed on sorted block-id signature, not top-level manifest identity"
    date: 2026-06-22
    status: accepted
    category: architecture
    tags: [player, escalate, usePlayback, react, reconcileManifest, identity, highlight, issue-50]
    path: architecture/2026-06-22-useplayback-reset-on-block-id-signature-not-manifest-identity.md
    summary: "Task #50. usePlayback resets active-block tracking on a sorted block-id signature (the real directory-swap signal), NOT on bare top-level manifest object identity. reconcileManifest deliberately changes top-level identity on every patch, so an identity-keyed reset would fire on every escalation and wipe the paused highlight (R7). The block-id set is stable across patches but changes on a true directory swap. Rejected: bare object-identity reset. Revisit if a patch can ever add/remove block ids (block split/merge)."
  - id: 2026-06-22-topbar-manual-absolute-dir-field-enables-server-escalate
    title: "Manual absolute-path dir field in TopBar is the server-mode escalate enabler"
    date: 2026-06-22
    status: accepted
    category: architecture
    tags: [player, escalate, server-mode, topbar, absolute-path, browser-fs, go-server, issue-50]
    path: architecture/2026-06-22-topbar-manual-absolute-dir-field-enables-server-escalate.md
    summary: "Task #50 server mode. The Go escalate server needs a real absolute FS path (filepath.Abs -> readManifest). No browser file loader exposes a real absolute path, so a manual absolute-path dir text field in the TopBar is the go/no-go enabler for server-mode escalation. Rejected deriving the dir from the existing fixture/picker loaders — browsers hide the true path, so it is impossible."
  - id: 2026-06-22-reconcilemanifest-preserves-per-block-identity
    title: "reconcileManifest preserves per-block object identity for React.memo short-circuit"
    date: 2026-06-22
    status: accepted
    category: performance
    tags: [player, escalate, reconcileManifest, react-memo, BlockRow, rerender, identity, issue-50]
    path: performance/2026-06-22-reconcilemanifest-preserves-per-block-identity.md
    summary: "Task #50. reconcileManifest returns the prior per-block object reference when a block is deep-equal, giving a new reference only to the patched block. React.memo(BlockRow) then short-circuits every unchanged sibling, so only the patched row re-renders — satisfying 'zero re-render on other rows'. Chosen over wholesale manifest overwrite (which would re-render the whole list). Note: it deliberately DOES change top-level manifest identity per patch, which is why the usePlayback reset must not key on top-level identity."
  - id: 2026-06-22-server-mode-refetch-resolves-against-fixture-base
    title: "Server-mode re-fetch resolves against FIXTURE_BASE, not an arbitrary server dir (phase-one limitation)"
    date: 2026-06-22
    status: superseded
    category: tradeoff
    tags: [player, escalate, server-mode, repointAudio, reloadManifest, fixture-base, phase-one-limitation, deferred, issue-50]
    path: tradeoff/2026-06-22-server-mode-refetch-resolves-against-fixture-base.md
    summary: "Task #50 known phase-one limitation. After an escalate patch, repointAudio/reloadManifest resolve their URLs against FIXTURE_BASE, not the arbitrary absolute server dir from the TopBar field — so post-patch re-fetch is correct only when the served dir is FIXTURE_BASE. The escalate POST itself works against the arbitrary dir; only the re-fetch is constrained. A full fix needs a server-contract change to serve patched outputs back from an arbitrary directory — accepted and deferred as a follow-up. Revisit when that contract change lands."
  - id: 2026-06-22-code-l2-size-gate-observed-behaviorally-no-schema-field
    title: "Code L2 size-gate is observed behaviorally (no `size_gated` plan field)"
    date: 2026-06-22
    status: accepted
    category: convention
    tags: [planner, levelCode, size-gate, plan-schema, additive-compatible, no-io-invariant, honesty-rule, observability, issue-48]
    path: convention/2026-06-22-code-l2-size-gate-observed-behaviorally-no-schema-field.md
    summary: "Implementation of issue #48's code-L2 size-gate (skip the LLM for blocks over ~250 lines, voice the deterministic gist instead). Question: how does the system signal a block was gated vs AI-gisted? DECISION (Option A): observe it BEHAVIORALLY — a gated block is Status=voiced + deterministic count+decls text + NO AI reply (no intelligence request emitted), indistinguishable in the plan from any other deterministically-voiced block. NO `size_gated` fact is ever emitted; a test (TestLevel_CodeL2Gate) pins the boundary at exactly codeGistMaxLines AND asserts the fact never leaks. Rejected Option B (add a `size_gated` Facts/Diagnostic schema field): permanent additive-schema surface for an internal optimization with no consumer, threads diagnostic state through the pure planner. Rationale: the gate is a cost optimization, not part of the narration contract; both the additive-schema rule and the planner purity/no-I/O invariant argue against minting an unconsumed field. Revisit if real operator tooling needs to declare gated blocks — add an additive fact then, justified by that consumer."
  - id: 2026-06-22-code-l2-deterministic-gist-shared-helper
    title: "Code L2 deterministic gist shared by degrade and size-gate paths (single helper, byte-identical)"
    date: 2026-06-22
    status: accepted
    category: convention
    tags: [planner, levelCode, degrade, size-gate, deterministicCodeGist, codeLangPhrase, honesty-rule, dry, yagni, issue-48]
    path: convention/2026-06-22-code-l2-deterministic-gist-shared-helper.md
    summary: "Issue #48 code-L2 created two paths that must voice the SAME deterministic count+decls gist: the no-adapter degrade path (degrade.go, Status=degraded) and the size-gate path (levelCode, Status=voiced). They must be byte-identical — the size-gate's behavioral-observability decision rests on the gated block's text matching a normal deterministic block. DECISION (Option A): both call a single deterministicCodeGist(body, langPhrase) helper, with langPhrase ALWAYS from a single codeLangPhrase(lang) — so output is byte-identical by construction, drift impossible without changing the shared helper. Contract ('langPhrase MUST be codeLangPhrase(lang) at every call site') backed by doc comments at both call sites + a test pinning the degrade segment to deterministicCodeGist. Rejected Option B (a per-class levelResult.deterministicFallback field) as YAGNI — only code needs it today, the field would be dead for every other class. Revisit if a second structured class needs the same degrade+size-gate fallback (two consumers justify the generic field then)."
  - id: 2026-06-22-code-semantic-gist-l2-only
    title: "AI semantic gist for code at L2 only (keep L1 free/instant/deterministic)"
    date: 2026-06-22
    status: accepted
    category: tradeoff
    tags: [planner, levelCode, intelligence, leveling, cost-model, deterministic-l1, core-invariant, honesty-rule, issue-48]
    path: tradeoff/2026-06-22-code-semantic-gist-l2-only.md
    summary: "Issue #48 unblocked. Code blocks only got a real 'what this code does' gist at L3+intelligence; default L1 was a bare line count. DECISION (Option 1 of 3): enrich at L2 ONLY — L1 keeps today's deterministic count, L2 sets needsIntelligence=true for a one-line meaning gist with honesty fallback to deterministic count/decls when no adapter (possibly size-gated to skip ~200-300+ line blocks). Rejected Option 2 (L1 behind opt-in flag — more surface, no L1 benefit user wanted) and Option 3 (full L1 AI gist by default — breaks the deterministic-L1 invariant, bills tokens+latency on every code block in every doc, needs explicit sign-off). Rationale: the deterministic-L1 invariant ('Deterministic L1 for structured classes') is load-bearing — the free/instant/zero-token property that makes the leveling cost story work; Option 1 delivers the gist one escalation away while leaving it intact, consistent with 'enriches L2/L3'. Same shape as the #47 table decision. Revisit Option 2/3 if demand for meaning-at-default-level grows."
  - id: 2026-06-22-table-meaning-summary-via-intelligence-l2-l3
    title: "Wire intelligence into table meaning-summary at L2/L3 (L1 stays deterministic)"
    date: 2026-06-22
    status: accepted
    category: tradeoff
    tags: [planner, levelTable, intelligence, leveling, cost-model, structured-classes, honesty-rule, issue-47]
    path: tradeoff/2026-06-22-table-meaning-summary-via-intelligence-l2-l3.md
    summary: "Issue #47 unblocked. levelTable was fully deterministic at every level (L1 shape, L2 headers+first+last row, L3 every row raw) — a table was never interpreted, unlike levelDiagram which already sets needsIntelligence=true at L2/L3. DECISION (Option A): L1 stays deterministic (shape); L2/L3 set needsIntelligence=true and pass table facts (cols/rows/headers) via IntelligenceRequest.Facts for a meaning summary; no adapter -> degrade to deterministic header/row reading, never fabricate. Rejected Option B (L3 only — leaves a smaller version of the same diagram inconsistency, L2 still meaningless) and Option C (reject — tables stay the one structured class with no interpretation path). Rationale: aligns with existing rule 'intelligence enriches L2/L3 for structured classes'; diagrams already do this, so this is a consistency fix, not a new cost model — hence accepted without the heavier sign-off #48 needed. Caching by (hash,level,model) means escalation doesn't re-bill. Revisit if the L1-deterministic property is ever pushed up to L2."
  - id: 2026-06-21-errclass-classcaller-routes-to-500-on-server-patch
    title: "ClassInternal is the zero value; ClassCaller routes to 500 on the server patch path (category-vs-wire split)"
    date: 2026-06-21
    status: accepted
    category: convention
    tags: [internal/errclass, cmd/narrate-server, cmd/narrate-mcp, error-classification, zero-value, fail-safe, wire-mapping, issue-51]
    path: convention/2026-06-21-errclass-classcaller-routes-to-500-on-server-patch.md
    summary: "Task #51 errclass owns CATEGORY only (no strings/HTTP codes/wrapping). DECISION 1: ClassInternal is the iota zero value so a forgotten/unrecognized/nil-classified path fails SAFE to internal — matching both roots' default branch (MCP 'internal_error: pipeline failure' / server 500 reasonInternal). DECISION 2: on the server patch path ClassCaller routes to 500 reasonInternal (falls through the existing default), NOT a 4xx — a deliberate category-vs-wire split where the SAME caller-class error (fs.ErrNotExist/ErrPermission) is 4xx invalid_argument on MCP but 500 on the server patch path. Each root owns its own Class->wire mapping; server adds no caller case; read-path source 400/404 in classifySourceErr untouched. Adding fs 4xx to the server patch path is out of scope. Revisit if a richer escalate contract needs caller errors surfaced as 4xx on the server."
  - id: 2026-06-21-errclass-imports-mcpsampling-one-classifier-place
    title: "errclass imports intelligence/mcpsampling so all shared classification lives in one place (Option A)"
    date: 2026-06-21
    status: superseded
    superseded_by: issue-58
    category: architecture
    tags: [internal/errclass, intelligence/mcpsampling, error-classification, import-coupling, layering, dedup, issue-51, issue-58]
    path: architecture/2026-06-21-errclass-imports-mcpsampling-one-classifier-place.md
    summary: "SUPERSEDED by issue #58 (2026-06-22) on DEAD-BRANCH grounds — NOT the pre-authorized cycle/layering-flag trigger (which never fired). Original (Option A): errclass imported intelligence/mcpsampling to recognize its two adapter sentinels (ErrNoSamplingClient, ErrUnexpectedContentKind) and classify both as ClassInternal, keeping ALL shared classification in ONE place; rejected Option B (omit the sentinels, re-handle at the MCP root). #58 discovered both sentinel branches were DEAD: each returned ClassInternal, identical to the default arm (ClassInternal is the fail-safe zero value), so the import was pure cost with zero classification benefit. #58 removed both branches and the import, restoring the 'only pipeline/ and cmd/ know concrete backends' invariant. Outcome coincides with the originally-rejected Option B (no concrete import in errclass) but for a different reason (dead branches, not coupling cost); Option B's 're-duplicate at MCP root / two classifiers' con does NOT materialize (the sentinels still classify to ClassInternal via the default arm). The 'all shared classification lives in one place' standing order STILL HOLDS — classification stays in one place; only a no-op branch was removed."
  - id: 2026-06-21-errclass-class-omits-isvalid-internal-return-type
    title: "errclass.Class deliberately omits IsValid() (closed internal return type), keeps only String()"
    date: 2026-06-21
    status: accepted
    category: convention
    tags: [internal/errclass, typed-enum, isvalid, string-method, error-classification, issue-51]
    path: convention/2026-06-21-errclass-class-omits-isvalid-internal-return-type.md
    summary: "Task #51 introduced the errclass.Class typed enum. DECISION: it deliberately DEPARTS from the project's typed-enum-with-IsValid() convention (the #10/#23 sweep, plan/enums.go) — NO IsValid(). Rationale turns the convention's own trigger around: the #10/#23 enums carry IsValid() because they are parsed from wire / deserialized / user-supplied and need input validation; Class is a closed INTERNAL return type produced only by Classify, never parsed/deserialized/user-supplied, so there's no untrusted input to validate and IsValid() would be dead code. String() IS provided (the only method) for debuggability + readable test failures. The departure is documented in a doc comment on the Class type so the absence reads as intentional. Clarifies the convention applies to wire/parsed enums, not closed internal return types. Revisit if Class ever becomes serialized or built from untrusted input."
  - id: 2026-06-21-errnothingtopatch-maps-to-4xx-not-patchblock-change
    title: "Map a real ErrNothingToPatch to a 4xx (Option A); do NOT add already-at-level detection to persistent.PatchBlock (Option B rejected)"
    date: 2026-06-21
    status: accepted
    category: error-handling
    tags: [cmd/narrate-server, http-escalate, errnothingtopatch, persistent-sink, patchblock, idempotency, dependency-contract, issue-49]
    path: error-handling/2026-06-21-errnothingtopatch-maps-to-4xx-not-patchblock-change.md
    summary: "Issue #49 HTTP escalate endpoint. Build review found the plan's B3/B5 idempotency premise was FALSE against live persistent.PatchBlock — which returns ErrNothingToPatch only for an INCOMPLETE/absent persistent-sink dir, NOT for a block already at the requested level (a same-level escalate flows through the happy path and re-renders content-identical bytes). DECISION (Option A): map a real ErrNothingToPatch to a 4xx source_not_found-class error (correct, since the sentinel means 'no complete prior output to patch into'), and document same-level-escalate convergence as happening via content-identical re-render through the happy path — no special-casing. Rejected Option B (add already-at-level detection to persistent.PatchBlock): overloads a shared dependency's error contract for marginal re-render savings on a local hobby tool and risks the existing offline --block patch path. Accepted cost: same-level escalate does a wasted re-render. Revisit if PatchBlock gains a real notion of current level."
  - id: 2026-06-21-loopback-enforcement-refuse-to-start-on-non-loopback-bind
    title: "Loopback enforcement — refuse to start (non-zero exit) on any non-loopback bind host, not bind-all-then-filter"
    date: 2026-06-21
    status: accepted
    category: security
    tags: [cmd/narrate-server, http-escalate, loopback, bind-host, local-only, fail-closed, secrets, issue-49]
    path: security/2026-06-21-loopback-enforcement-refuse-to-start-on-non-loopback-bind.md
    summary: "cmd/narrate-server (issue #49) introduces an HTTP network surface to a local-only tool that may speak secrets aloud. DECISION (Option A): the server REFUSES TO START (non-zero exit) if the bind host is not a loopback address, rather than binding all interfaces and filtering requests at runtime (Option B). Fail-closed by construction — you cannot accidentally expose the server if it refuses to bind a public interface in the first place; the dangerous publicly-bound-socket state never exists. Aligns with the local-only CLAUDE.md posture + the gotcha that secrets may be read aloud on the user's machine. Misconfig surfaces as an immediate loud startup failure, not a silent runtime hole. Rejected Option B (bind-all-then-filter): fail-open; a filter bug or future refactor re-exposes it. Revisit only paired with an explicit auth story if a real remote/multi-host use case appears."
  - id: 2026-06-21-escalate-response-from-on-disk-readback-not-narrateresult
    title: "Escalate response shaped from post-patch on-disk read-back (plan.json + manifest.json), not from NarrateResult; add additive persistent.ReadManifest"
    date: 2026-06-21
    status: accepted
    category: architecture
    tags: [cmd/narrate-server, http-escalate, narrateresult, readback, persistent-sink, readmanifest, seam-gap, plan-schema, issue-49]
    path: architecture/2026-06-21-escalate-response-from-on-disk-readback-not-narrateresult.md
    summary: "Issue #49 escalate endpoint must return the updated Block/BlockTiming/audio_ref after PatchBlock, but NarrateResult does NOT expose them (build-review seam gap R1). DECISION (Option A): shape the response from post-patch ON-DISK state — read plan.json + manifest.json back from the patched outDir, which are the authoritative post-patch artifacts — and add an ADDITIVE exported persistent.ReadManifest, rather than changing plan/ (engine-neutral, zero-deps) or NarrateResult's contract (Option B, larger blast radius). Read-back is correct-by-construction (matches what was persisted) and minimal-surface. Accepted WITH the explicit note that R1 remains a known seam gap: a future NarrateResult enrichment could close it and let the handler drop the read-back. Same package-scope-read-side pattern as persistent.CheckStale. Revisit (drop read-back) if NarrateResult is enriched to expose updated block state."
  - id: 2026-06-21-list-ordinal-cue-spelled-to-ten-numeric-beyond
    title: "List item ordinal cue: spelled ordinals 1–10, numeric 'item N' fallback beyond ten"
    date: 2026-06-21
    status: accepted
    category: algorithm
    tags: [list, ordinals, voicing, planner, ticket-45]
    path: algorithm/2026-06-21-list-ordinal-cue-spelled-to-ten-numeric-beyond.md
    summary: "Ticket #45 list voicing. Items 1–10 get spelled ordinals from a frozen First..Tenth lookup table; items 11+ use a numeric 'item N' fallback rather than spelled ordinals past ten. Deliberate: avoids building/maintaining a general ordinal-spelling engine, and keeps long lists legible (spoken 'item 23' tracks better than 'twenty-third'). Cue style intentionally changes shape at the 10→11 boundary. Rejected: spelled ordinals for all items (needs full speller for tens/hundreds/compounds; long spelled ordinals hurt listener tracking). Revisit if the boundary reads as jarring or a general ordinal speller arrives for another reason."
  - id: 2026-06-21-list-preamble-titled-reuses-source-bare-generates
    title: "List preamble: titled list reuses the source title, bare list generates 'List of N items.'"
    date: 2026-06-21
    status: accepted
    category: convention
    tags: [list, preamble, honesty-rule, voicing, planner, ticket-45]
    path: convention/2026-06-21-list-preamble-titled-reuses-source-bare-generates.md
    summary: "Ticket #45 list voicing. A list with a preceding source title reuses that title as the spoken preamble, normalising a trailing colon to a period ('Steps to deploy:' → 'Steps to deploy.'). A bare list with no preceding label generates a synthetic 'List of N items.' preamble. Honesty-rule rationale: speak the real source label rather than fabricate one; only fall back to a factual generated preamble (asserts only a true count, invents no topic) when no source label exists. Depends on colon-gated title detection to pick the branch."
  - id: 2026-06-21-colon-gated-list-title-detection-goldmark
    title: "Colon-gated list title detection under goldmark marker stripping"
    date: 2026-06-21
    status: superseded
    category: tradeoff
    tags: [list, title-detection, goldmark, heuristic, false-positive, planner, ticket-45]
    path: tradeoff/2026-06-21-colon-gated-list-title-detection-goldmark.md
    summary: "Ticket #45. goldmark strips the first list item's marker, so the planner cannot structurally tell a true preceding title line from a de-markered first item — both surface as plain text ahead of the list. Constraint-driven heuristic: a trailing colon on the preceding line promotes it to a title (eligible for the titled-preamble branch). Documented, accepted false-positive direction: a genuine first item ending in ':' can be mis-promoted to a title and dropped from the spoken items. Bias chosen knowingly — the colon is the only reliable distinguishing signal left, and titled intro lines ending in ':' are far more common than items ending in ':'. Rejected: AST/structural detection (signal already gone). Revisit if the segmenter changes or the false positive shows up in real input."
  - id: 2026-06-21-planner-test-seam-race-thread-clock-planid-per-call
    title: "Fix planner test-seam data race by threading clock/plan-id seams per-call (Option B)"
    date: 2026-06-21
    status: accepted
    category: architecture
    tags: [planner, data-race, t.Parallel, test-seam, concurrency, voiceoptions, purity]
    path: architecture/2026-06-21-planner-test-seam-race-thread-clock-planid-per-call.md
    summary: "make test-race flagged data races in planner: package-level test seams nowFunc/newPlanIDFunc were mutated by parallel t.Parallel() tests while a sibling Plan() read them. Chose Option B — delete the globals, thread unexported withClock/withPlanID VoiceOptions per-call, resolved to locals inside Plan() with wall-clock + plan.NewPlanID defaults. Root-cause fix: no shared mutable state left to race on, tests stay parallel, planner stays pure. Plan review (7-reviewer panel) and build review both APPROVE'd. Rejected: A) drop t.Parallel() from ~8 tests (global footgun remains), C) sync.RWMutex on the globals (serializes but doesn't stop logical cross-talk; adds lock machinery to a pure hot path — worst structural fit). Note: plan.NewPlanID() returns a plain string, no named plan.PlanID type exists."
  - id: 2026-06-21-preserve-variadic-compilelexicon-signature
    title: "Preserve the variadic compileLexicon(opts...) signature when surfacing resolved voiceOptions to Plan()"
    date: 2026-06-21
    status: accepted
    category: tradeoff
    tags: [planner, voiceoptions, compilelexicon, api-shape, blast-radius, test-call-sites]
    path: tradeoff/2026-06-21-preserve-variadic-compilelexicon-signature.md
    summary: "Option B needed clock/planID surfaced from parsed opts into Plan() (previously only the compiled lexicon escaped the parse). Added clock/planID fields to the voiceOptions struct and introduced resolveVoiceOptions + compileLexiconCfg, but KEPT the existing variadic compileLexicon(opts...) signature (now delegating to compileLexiconCfg) rather than changing it — to avoid breaking ~25 unrelated test call sites. Minimal-blast-radius API-shape tradeoff, faithful to Option B's spirit. Two compile entry points coexist intentionally; revisit (collapse the wrapper) if the variadic call sites are ever migrated en masse. Serves the planner test-seam race fix decision."
  - id: 2026-06-21-mcpsampling-cache-server-lifetime-lru-eviction
    title: "mcpsampling cache is server-lifetime with LRU + entry-count-cap eviction"
    date: 2026-06-21
    status: accepted
    category: convention
    tags: [intelligence, mcpsampling, cache, eviction, lru, cache-lifetime, issue-25]
    path: convention/2026-06-21-mcpsampling-cache-server-lifetime-lru-eviction.md
    supersedes: 2026-06-20-mcpsampling-cache-key-includes-full-model-id
    summary: "Moves the mcpsampling cache from per-call to server-lifetime with LRU + entry-count-cap eviction (DefaultCacheCapacity=512). The full-model-id cache key + two-phase last-known-model lookup CARRY FORWARD UNCHANGED from #13; only the per-call lifetime is superseded. Both the LRU and the last-known-model map now live on a single server-lifetime ServerCache, allocated once in cmd/narrate-mcp serve/newServer and shared across all runSpeak tool calls, so cross-call escalation stops re-billing. LRU+size-cap chosen over TTL because cached summaries are pure functions of (content_hash, level, full_model) and never go wall-clock stale — TTL would only force needless re-bills. The last-known-model map is NOT evicted: bounded by construction (one clientID per server via WithClientID). Benign cross-call stale-read window: a mid-session model switch costs at most one extra re-bill, never returns wrong data. Revisit if multiple clientIDs per server are ever introduced. Rejected: TTL eviction (content has no time dimension), unbounded cache (no memory bound)."
  - id: 2026-06-21-ephemeral-ctx-cancel-joined-error-not-ctx-only
    title: "Ephemeral sink ctx-cancel surfaces a joined error (ctx cause + process exit), not ctx-only"
    date: 2026-06-21
    status: accepted
    category: convention
    tags: [sink/ephemeral, ctx-cancel, errors.Join, error-handling, issue-11]
    path: convention/2026-06-21-ephemeral-ctx-cancel-joined-error-not-ctx-only.md
    summary: "playWithAfplay's cancel branch returns errors.Join(callCtx.Err(), waitErr) — the drained cmd.Wait() result (typically 'signal: killed') joined onto the ctx cause — instead of discarding it and returning only callCtx.Err(). Callers see both why playback stopped and how the child died. errors.Join preserves errors.Is(err, context.Canceled/DeadlineExceeded) (existing ctx-cancel test stays green) and drops waitErr when nil (clean reap collapses to bare ctx error). Fixed at the playWithAfplay layer, not Consume (Consume's between-blocks ctx-precedence unchanged). Locked by TestPlayWithAfplay_CtxCancel_Joins (asserts >=2 errors via Unwrap() []error). Rejected: ctx.Err()-only (loses process-death signal); fmt.Errorf %w (single chain can't carry two independent causes)."
  - id: 2026-06-21-ephemeral-testdata-skip-message-not-wav-fixture
    title: "sink/ephemeral testdata stays a smoke-test skip message, no committed WAV fixture"
    date: 2026-06-21
    status: accepted
    category: convention
    tags: [sink/ephemeral, testdata, golden-audio, smoke-test, build-tags, issue-11]
    path: convention/2026-06-21-ephemeral-testdata-skip-message-not-wav-fixture.md
    summary: "Issue #11 AC#7 offered a WAV fixture OR a smoke-test skip message; chose the skip-message branch and keep testdata/.gitkeep. The //go:build manual smoke test t.Skip's with a generate-via-scripts/kokoro hint when testdata/5s.wav is absent, so the manual-tag run stays green on a fresh checkout and no binary lands in the repo. Aligns with CLAUDE.md 'no golden audio (validated by ear during /verify)'. Recorded so a future contributor doesn't 'fix' the missing fixture by committing a binary. Rejected: commit a real/synthetic WAV."
  - id: 2026-06-21-anthropic-non-2xx-raw-body-excerpt-not-decoded
    title: "Anthropic non-2xx errors surface a raw body excerpt, not a decoded error type"
    date: 2026-06-21
    status: accepted
    category: convention
    tags: [intelligence/anthropic, error-handling, non-2xx, convention, issue-34, dead-code]
    path: convention/2026-06-21-anthropic-non-2xx-raw-body-excerpt-not-decoded.md
    summary: "Issue #34 lint cleanup removed the never-wired errorResponse type from intelligence/anthropic/api.go. The adapter's non-2xx error surface is the raw truncated body excerpt (errBodyExcerpt, <=512 bytes via fmt.Errorf in Voice), not a decoded {type, error:{message}} object — the upstream pipeline classifier owns retry-vs-fail, so the adapter only needs a human-readable error string. Behavior locked by TestVoice_Unauthorized401 / TestVoice_BadRequest400. Rejected: decode errorResponse.error.message (would change tested error strings; deferred as a future feature if adapter-local error-type branching is ever needed)."
  - id: 2026-06-21-persistent-block-patch-manifest-index-derived-ranges
    title: "--block patch into a persistent outDir: manifest is the INDEX, byte ranges are DERIVED"
    date: 2026-06-21
    status: accepted
    category: convention
    tags: [sink/persistent, cmd, narrate, block-rerender, persistent-sink, manifest, route-a, honesty-rule, crash-consistency, issue-28]
    path: convention/2026-06-21-persistent-block-patch-manifest-index-derived-ranges.md
    supersedes: 2026-06-20-block-with-persistent-sink-rejected-at-flag-time
    summary: "PatchBlock (package func, CheckStale precedent) patches one block into an existing persistent outDir. Route A: manifest.json is the authoritative block INDEX; per-block byte ranges in audio.wav are DERIVED from manifest timing + AudioFormat via the same silenceBytes() math the writer used — never stored (rejected Route B stored offsets as a drift-corruption class). F1 (container-vs-manifest length input-guard, refuses on mismatch) and F2 (stage-all-tmp + rename audio.wav LAST write ordering) are INDEPENDENT guarantees, not a detect-and-recover pair; output crash-consistency is carried by re-run-rewrites-everything (closes the zero-delta hole). plan.json + manifest.json both stay multi-block and agree on the patched block's classification. --expected-content-hash stays OPTIONAL (the manifest ContentHash gate already refuses cross-document patches). No manifest schema change (ManifestSchemaVersion=1). Supersedes v1.9.0 (block-with-persistent-rejected-at-flag-time)."

  - id: 2026-06-21-oauth-bearer-subscription-token-as-api-credential
    title: "Repurpose claude setup-token OAuth token as a raw-API credential via opt-in Bearer auth"
    date: 2026-06-21
    status: accepted
    category: tradeoff
    tags: [intelligence/anthropic, auth, credentials, oauth, bearer, x-api-key, tos, issue-32]
    path: tradeoff/2026-06-21-oauth-bearer-subscription-token-as-api-credential.md
    summary: "Adapter gains an opt-in Authorization: Bearer auth mode (+ anthropic-beta: oauth-2025-04-20) so a claude setup-token subscription OAuth token (sk-ant-oat01-) authenticates against /v1/messages. x-api-key stays the default and is auto-overridden to Bearer on the sk-ant-oat01 prefix; explicit WithBearerAuth wins. Accepted with a documented ToS gray-area caveat (ref anthropics/claude-code#1785) — opt-in default is the mitigation."

  - id: 2026-06-21-player-source-pane-uses-sibling-source-md
    title: "Player source pane consumes sibling source.md (not reconstruction from raw_excerpt)"
    date: 2026-06-21
    status: accepted
    category: convention
    tags: [player, source-map, honesty-rule, ux, issue-18]
    path: convention/2026-06-21-player-source-pane-uses-sibling-source-md.md
    summary: "Player loads source.md alongside plan.json+manifest.json+audio.wav so cursor-tracked highlights project onto exact start_line/end_line. If source.md absent, banner + per-block raw_excerpt fallback (advisory line numbers). Honesty rule extended to UI — never silently fabricate a source view. Rejected: reconstruct from raw_excerpt (raw_excerpt is normalized; concatenation drifts from start_line/end_line)."

  - id: 2026-06-21-player-dual-data-loading-fixture-and-picker
    title: "Player dual data loading — bundled fixture AND runtime directory picker"
    date: 2026-06-21
    status: accepted
    category: convention
    tags: [player, fixture, file-system-access, ux, issue-18]
    path: convention/2026-06-21-player-dual-data-loading-fixture-and-picker.md
    summary: "useFixture fetches /fixtures/sample/ on mount (zero-friction first-run demo); useDirectoryLoader + window.showDirectoryPicker (File System Access API) with <input webkitdirectory> Safari/Firefox fallback supplies the bring-your-own-output path. Two ACs from #18 collapsed into two code paths that share one {plan,manifest,audioUrl,source} shape. Rejected: picker-only (hostile first run) and fixture-only (player reads as hard-coded slideshow)."

  - id: 2026-06-21-player-synthetic-hand-authored-fixture
    title: "Player fixture is synthetic, committed, hand-authored (not real Kokoro output)"
    date: 2026-06-21
    status: accepted
    category: convention
    tags: [player, fixture, kokoro, demo, issue-18]
    path: convention/2026-06-21-player-synthetic-hand-authored-fixture.md
    summary: "Hand-authored plan.json + manifest.json mirroring docs/samples/sample.md, covering every Class enum (heading/prose/code/table/list/unknown) and every Status enum (voiced/degraded/refused). audio.wav = 24 kHz mono PCM-16 ~2s silent via make_silent_wav.py. Fixture proves UI loads + renders all branches; not demo audio quality. Rejected: real Kokoro fixture (gates fresh-checkout pnpm install && pnpm dev on Python venv + downloaded weights)."

  - id: 2026-06-21-player-raf-audio-sync-transition-only-rerender
    title: "Player audio sync via requestAnimationFrame; React state writes only on block transition"
    date: 2026-06-21
    status: accepted
    category: convention
    tags: [player, audio-sync, react, performance, block-level-sync, issue-18]
    path: convention/2026-06-21-player-raf-audio-sync-transition-only-rerender.md
    summary: "usePlayback runs a rAF loop sampling audio.currentTime, binary-searches manifest.blocks via findActiveBlock, dispatches SET_ACTIVE_BLOCK only when block id differs from ref-tracked previous. Audio drives time, React drives presentation. React renders only at block transitions, not 60 Hz. Rejected: timeupdate event (250 ms granularity misses sub-500 ms blocks), setInterval (jittery, always re-renders, runs in bg tabs)."

  - id: 2026-06-21-player-escalate-ux-inline-card-not-modal
    title: "Player escalate UX is an inline expanded command card (not modal, not toast)"
    date: 2026-06-21
    status: accepted
    category: convention
    tags: [player, escalate, ux, accessibility, issue-18]
    path: convention/2026-06-21-player-escalate-ux-inline-card-not-modal.md
    summary: "Per-block 'Escalate L3' button expands an inline card directly under the row containing the literal `narrate --block <id> --level 3 --file <source.uri>` command + Copy + Dismiss. Card stays open until dismissed or another block escalated. Only on non-refused blocks. No focus trap, no modal context, normal DOM tab order. Rejected: modal dialog (interrupts playback flow), toast (disappears before user can copy)."

  - id: 2026-06-21-player-dev-actions-via-makefile-targets
    title: "Player dev actions drive through Makefile targets, not raw pnpm commands"
    date: 2026-06-21
    status: accepted
    category: convention
    tags: [player, makefile, dev-workflow, issue-18]
    path: convention/2026-06-21-player-dev-actions-via-makefile-targets.md
    summary: "Five new root Makefile targets: player-dev, player-build, player-test, player-fixture-silent, player-fixture-kokoro. Updates make help. Honors CLAUDE.md mandate 'drive all repeatable dev actions through Makefile targets' — one muscle-memory pattern across Go + TS halves of the project. Rejected: raw pnpm commands in README (splits dev workflow into two patterns, forgetting cd player/ is a regular paper cut)."

  - id: 2026-06-20-anthropic-cache-single-phase
    title: "Anthropic cache is single-phase (no last-known-actual-model lookup)"
    date: 2026-06-20
    status: experimental
    category: convention
    tags: [intelligence, anthropic, cache, model-id, simplification, issue-15]
    path: convention/2026-06-20-anthropic-cache-single-phase.md
    summary: "Anthropic adapter chooses a.model at construction so the pre-call cache key is fully knowable — collapses mcpsampling's two-phase lookup to single-phase. Key uses configured a.model on Get + Put. Cache hit Model field uses configured model; first-call Model echoes resolved actual model from API response (intentional divergence). Stale-on-alias-update bounded by per-call cache lifetime."

  - id: 2026-06-20-anthropic-with-max-tokens-map-shape
    title: "WithMaxTokens uses a map shape (partial-override) on the anthropic adapter"
    date: 2026-06-20
    status: accepted
    category: convention
    tags: [intelligence, anthropic, options, max-tokens, issue-15]
    path: convention/2026-06-20-anthropic-with-max-tokens-map-shape.md
    summary: "WithMaxTokens(map[plan.Level]int) — callers tune only the levels they care about; unspecified keep defaults. Diverges from mcpsampling's positional (l1, l2, l3 int64) shape on purpose: anthropic is constructed once and tuning one level is the realistic case."

  - id: 2026-06-20-anthropic-new-returns-error-on-empty-key
    title: "anthropic.New returns error on empty API key"
    date: 2026-06-20
    status: accepted
    category: convention
    tags: [intelligence, anthropic, constructor, error-handling, issue-15]
    path: convention/2026-06-20-anthropic-new-returns-error-on-empty-key.md
    summary: "anthropic.New(opts ...Option) (*Adapter, error) — empty apiKey surfaces at construction, not as a 401 deep in Voice(). cmd/narrate's chooseIntelligence panics on non-nil error because validate() already enforces the env var; failing here would be a programmer bug."

  - id: 2026-06-20-anthropic-cache-forward-declared-phase-2
    title: "Anthropic Cache forward-declared in Phase 2, narrowed in Phase 4"
    date: 2026-06-20
    status: experimental
    category: convention
    tags: [intelligence, anthropic, cache, interface, phased-build, issue-15]
    path: convention/2026-06-20-anthropic-cache-forward-declared-phase-2.md
    summary: "Phase 2 scaffold needs a Cache type for WithCache(c Cache); Phase 4 has the real interface. Forward-declare as wide interface in Phase 2's anthropic.go; Phase 4 removes the declaration when cache.go lands. Always exactly one Cache definition at any point in history."

  - id: 2026-06-20-intelligence-anthropic-missing-env-is-flag-error
    title: "--intelligence anthropic with missing env is a flag-validation error"
    date: 2026-06-20
    status: experimental
    category: convention
    tags: [cmd, narrate, intelligence, anthropic, flag-validation, exit-codes, issue-15]
    path: convention/2026-06-20-intelligence-anthropic-missing-env-is-flag-error.md
    summary: "validate() checks ANTHROPIC_API_KEY when --intelligence=anthropic. Empty → error naming both the flag and the env var, wrapped in errFlagValidation, exit 2. Mirrors --block × --sink=persistent precedent. Rejected: silent fallback to none (hides misconfig), runtime error at first API call (mixes user-config with system failure)."

  - id: 2026-06-20-anthropic-retry-policy-429-only
    title: "Anthropic retry policy — 429 only, max 2 retries, 30s cap"
    date: 2026-06-20
    status: experimental
    category: convention
    tags: [intelligence, anthropic, retry, rate-limit, http, issue-15]
    path: convention/2026-06-20-anthropic-retry-policy-429-only.md
    summary: "doWithRetry: max 2 retries (3 attempts), 429-only (no 5xx), Retry-After parsed (int seconds → HTTP-date → exponential 1s/2s), 30s cap per sleep, ctx.Done propagation via injected sleeper. Sustained rate-limit surfaces as Go error after ~3s of trying. Mis-configured backend with multi-minute Retry-After bounded at 30s."

  - id: 2026-06-20-anthropic-http-test-seam-roundtripfunc
    title: "Anthropic HTTP test seam — roundTripFunc via WithHTTPClient"
    date: 2026-06-20
    status: accepted
    category: convention
    tags: [intelligence, anthropic, testing, http, stdlib, issue-15]
    path: convention/2026-06-20-anthropic-http-test-seam-roundtripfunc.md
    summary: "Tests inject *http.Client{Transport: roundTripFunc(...)} via WithHTTPClient(*http.Client). Stays close to stdlib — no project-specific Transport interface. Sleeper test seam (WithSleeper) follows the same pattern. Rejected: custom Transport iface (adds surface for what http.RoundTripper already names), httptest.Server (heavier than needed)."

  - id: 2026-06-20-no-anthropic-sdk
    title: "No Anthropic SDK — plain net/http + encoding/json"
    date: 2026-06-20
    status: accepted
    category: library-choice
    tags: [intelligence, anthropic, http, stdlib, dependencies, issue-15]
    path: library-choice/2026-06-20-no-anthropic-sdk.md
    summary: "intelligence/anthropic uses plain net/http + encoding/json against POST /v1/messages. Zero new go.mod deps. ~80 LOC of request/response structs. Full control over retry / Retry-After / context.Cancel flow. Rejected the official SDK because it pulls batch / files / streaming / MCP-client surface we don't use. Revisit if the project starts using batch or streaming."

  - id: 2026-06-20-cache-machinery-duplicate-not-lift
    title: "Cache machinery duplicate-not-lift between mcpsampling and anthropic"
    date: 2026-06-20
    status: experimental
    category: convention
    tags: [intelligence, anthropic, mcpsampling, cache, code-reuse, issue-15]
    path: convention/2026-06-20-cache-machinery-duplicate-not-lift.md
    summary: "Each adapter owns its own Cache + CacheKey + helpers (~80 LOC duplicated). mcpsampling's two-phase clientID-scoped key shape and anthropic's single-phase shape don't share enough to justify a lift today. Generalize when a 3rd adapter materializes — 'two consumers before lift' principle."

  - id: 2026-06-20-refusal-parser-duplicate-not-lift
    title: "Refusal-parser duplicate-not-lift between mcpsampling and anthropic"
    date: 2026-06-20
    status: experimental
    category: convention
    tags: [intelligence, anthropic, mcpsampling, refusal, code-reuse, issue-15]
    path: convention/2026-06-20-refusal-parser-duplicate-not-lift.md
    summary: "anthropic/refusal.go is a 10-line copy of mcpsampling's refuseSentinel + parseRefusal. Lifting would also require lifting the system-prompt contract test — too much surface area for the opening commit. Future drift caught by grep + deps_test. Generalize when a 3rd adapter materializes."

  - id: 2026-06-20-block-with-persistent-sink-rejected-at-flag-time
    title: "--block X with --sink=persistent rejected at flag-validation"
    date: 2026-06-20
    status: superseded
    superseded_by: 2026-06-21-persistent-block-patch-manifest-index-derived-ranges
    category: convention
    tags: [cmd, narrate, persistent-sink, block-rerender, flag-validation, honesty-rule, issue-16]
    path: convention/2026-06-20-block-with-persistent-sink-rejected-at-flag-time.md
    summary: "Combining --block with --sink=persistent would cause the persistent sink to overwrite a multi-block audio.wav with a single-block render. flagSet.validate() rejects the combination at flag-time, routing through errFlagValidation to exit 2. Error message names both flags + points at the planned follow-up ticket for block-level patch into an existing persistent outDir. Honesty rule preserved: refuse, don't corrupt."

  - id: 2026-06-20-persistent-voice-id-via-withvoice-option
    title: "Persistent sink voice id flows via WithVoice Option, not plan.Timeline"
    date: 2026-06-20
    status: accepted
    category: architecture
    tags: [sink, persistent, plan-schema, engine-neutrality, composition-root, voice-id, issue-16]
    path: architecture/2026-06-20-persistent-voice-id-via-withvoice-option.md
    summary: "manifest.json carries the engine voice id (e.g. af_bella). plan/ schema stays engine-neutral — no Timeline.Voice field. Composition root passes the voice id via persistent.WithVoice(genderToVoice[args.Gender]) at construction time. Sink stays oblivious to gender → voice mapping. Empty voice id is a valid manifest (sink does not invent). Rejected: contaminate plan.Timeline or render.RenderResult.Format with engine identifiers."

  - id: 2026-06-20-persistent-checkstale-not-on-outputsink-interface
    title: "Persistent sink's CheckStale is NOT part of the OutputSink interface"
    date: 2026-06-20
    status: accepted
    category: architecture
    tags: [sink, persistent, interface-design, read-side-query, ephemeral, issue-16]
    path: architecture/2026-06-20-persistent-checkstale-not-on-outputsink-interface.md
    summary: "CheckStale lives as a package-scope function in sink/persistent, not on the OutputSink interface. The interface stays narrow ('write the bytes' via Consume). Ephemeral sinks have no manifest.json — adding CheckStale to the interface would force a meaningless stub. Lift to a shared StalenessChecker interface only if a second sink genuinely shares the content-hash + source-URI staleness semantics."

  - id: 2026-06-20-persistent-leading-silence-before-block-zero
    title: "Persistent sink emits leading silence before block 0 if StartMs > 0"
    date: 2026-06-20
    status: accepted
    category: convention
    tags: [sink, persistent, audio-wav, silence, timeline-fidelity, issue-16]
    path: convention/2026-06-20-persistent-leading-silence-before-block-zero.md
    summary: "audio.wav's wall-clock at offset t ms equals Timeline.Blocks[i].StartMs for every i, including i=0. If Blocks[0].StartMs > 0, the sink emits StartMs milliseconds of silence as the leading prefix. Unified leading = blk.StartMs - cursorMs calculation with cursor=0 — no special-case code. Downstream consumers (React reference player, scrub bars) align by absolute time without offset bookkeeping."

  - id: 2026-06-20-persistent-atomic-tmp-rename-writes
    title: "Persistent sink uses atomic tmp+rename writes for all three output files"
    date: 2026-06-20
    status: accepted
    category: convention
    tags: [sink, persistent, atomic-write, honesty-rule, partial-state, ctx-cancel, issue-16]
    path: convention/2026-06-20-persistent-atomic-tmp-rename-writes.md
    summary: "audio.wav, plan.json, manifest.json each written via tmp file in the same directory + os.Rename. A ctx-cancel/crash/full-disk at any point leaves the previous output (or no output), never a corrupted file. atomicWriteFile() covers JSON; writeAudio() covers WAV. Tests assert no output files land on disk after per-block read failure or ctx-cancel. Honesty rule extended to bytes-on-disk."

  - id: 2026-06-20-persistent-new-takes-outdir-positional
    title: "Persistent Sink.New takes outDir as a positional argument"
    date: 2026-06-20
    status: accepted
    category: convention
    tags: [sink, persistent, api-shape, functional-options, mandatory-arg, issue-16]
    path: convention/2026-06-20-persistent-new-takes-outdir-positional.md
    summary: "persistent.New(outDir string, opts ...Option) — outDir mandatory at the constructor, expressed in the type system (omission = compile error). Functional Options (WithVoice, WithExpectedFormat) layer over defaults. Diverges from ephemeral.New(opts...) shape but expresses mandatory-vs-optional honestly. ErrNoOutDir runtime guard backs up the constructor for zero-value &Sink{} use."

  - id: 2026-06-20-persistent-manifest-no-build-timestamps
    title: "Persistent-sink manifest carries no build timestamp"
    date: 2026-06-20
    status: accepted
    category: convention
    tags: [sink, persistent, manifest, idempotency, determinism, issue-16]
    path: convention/2026-06-20-persistent-manifest-no-build-timestamps.md
    summary: "Manifest struct contains no built_at / created_at field. Consume produces byte-identical output bytes for identical input bytes — manifest.json + plan.json + audio.wav all stable across re-runs. Idempotent-rewrite AC is trivially provable; tooling can compare byte-equal. Filesystem mtime covers the 'when was this written?' need without env-var seams. TestConsume_IdempotentRewrite pins the property."

  - id: 2026-06-20-persistent-manifest-schema-version-additive
    title: "Persistent-sink ManifestSchemaVersion starts at 1, additive-compatible"
    date: 2026-06-20
    status: accepted
    category: schema
    tags: [sink, persistent, manifest, schema-version, additive-compatibility, issue-16]
    path: schema/2026-06-20-persistent-manifest-schema-version-additive.md
    summary: "const ManifestSchemaVersion = 1 (int, package-level). Field additions are backward-compatible within the major (json.Unmarshal ignores unknown fields). Renames/removals require a major bump. TestManifest_SchemaVersionIsOne pins the const so accidental bumps trigger review attention. Mirrors plan/ SchemaVersion convention."

  - id: 2026-06-20-persistent-wav-reader-hardcoded-default-format
    title: "Persistent sink WAV reader hard-coded to render.DefaultFormat()"
    date: 2026-06-20
    status: accepted
    category: convention
    tags: [sink, persistent, wav, format-validation, kokoro, phase-one, issue-16]
    path: convention/2026-06-20-persistent-wav-reader-hardcoded-default-format.md
    summary: "Phase-one persistent sink validates per-block WAVs against render.DefaultFormat() (24 kHz mono PCM s16le). Format mismatches return formatMismatchError naming the divergent field + block source path; container corruption returns ErrInvalidWAV. WithExpectedFormat Option exists for a future engine drop-in. Cross-engine format negotiation is a phase-two concern. Tight constraint catches upstream renderer regressions early."

  - id: 2026-06-20-mcptext-uri-sha256-cross-check
    title: "mcptext URI carries sha256(text); adapter cross-checks on Read"
    date: 2026-06-20
    status: accepted
    category: convention
    tags: [adapter, mcptext, uri, sha256, content-hash, composition-root, issue-17]
    path: convention/2026-06-20-mcptext-uri-sha256-cross-check.md
    summary: "Composition root assembles URI as mcp://inline/<hex-sha256-of-text>; adapter.Read computes sha256(a.text) and rejects on scheme mismatch or hex-suffix mismatch. Mismatch is a terminal error (wiring bug), not a refusal. Catches the class of composition-root bugs where caller computes the URI from one string and constructs New(other). URIFor helper exported so there is one URI-assembly routine. Supersedes 2026-06-19-text-arg-transient-sentinel."

  - id: 2026-06-20-adapter-offsetmap-duplication-deferred-extraction
    title: "adapter offset-map line walker duplicated between file + mcptext; shared adapterutil deferred"
    date: 2026-06-20
    status: accepted
    category: convention
    tags: [adapter, mcptext, file, offsetmap, duplication, speculative-abstraction, issue-17]
    path: convention/2026-06-20-adapter-offsetmap-duplication-deferred-extraction.md
    summary: "buildOffsetMap + estimateLineCount (~30 lines) duplicated byte-for-byte between adapter/file and adapter/mcptext rather than lifted to a shared adapterutil package. Two consumers is too thin a base for speculative abstraction; lift when the third byte-emitting adapter (ocr) lands and informs the helper's signature. Both packages carry source-level docstrings pointing at this decision; mirrored test fixtures guard against drift."

  - id: 2026-06-20-pipeline-block-rerender-uses-document-hash
    title: "Pipeline block re-render uses document-level content_hash"
    date: 2026-06-20
    status: accepted
    category: architecture
    tags: [pipeline, block-rerender, content-hash, staleness, plan-schema, phase-one, issue-14]
    path: architecture/2026-06-20-pipeline-block-rerender-uses-document-hash.md
    summary: "--expected-content-hash compares against plan.Source.ContentHash (the document hash), NOT a new per-block hash. Planner is deterministic so a document-hash match implies block-hash match. Avoids a new plan-schema field. Surfaced via NarrateResult.BlockHashMismatch (warning) and NarrateResult.DocumentContentHash (exposed on stdout as content_hash=<hex>)."

  - id: 2026-06-20-mcpsampling-cache-key-includes-full-model-id
    title: "mcpsampling cache key includes the full chosen-model id"
    date: 2026-06-20
    status: superseded
    superseded_by: 2026-06-21-mcpsampling-cache-server-lifetime-lru-eviction
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
    tags: [plan, enum, severity, sayas, emphasis, voicedby, ssml, additive-compatible, refactor, issue-10, issue-13, issue-23]
    path: schema/2026-06-20-typed-enum-pattern-wins-for-all-enum-shaped.md
    summary: "Adopt typed string alias + IsValid() pattern for all enum-shaped fields. Severity intentionally 2-valued; pipeline-stopping uses Go error per CLAUDE.md honesty rule. Wire format unchanged, additive-compat preserved. Rejected freeform-with-docs (Option B) and hybrid (Option C). Closes #10; validating use case #13. Amendment (#23): the sweep was actually off by one — Provenance.VoicedBy was a tenth field in plan.go that #10 missed; #23 typed it, so all TEN enum-shaped fields now uniform, zero bare-string-with-comment remaining."

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
    status: superseded
    superseded_by: 2026-06-20-mcptext-uri-sha256-cross-check
    category: convention
    tags: [cmd/narrate-mcp, mcp, text-arg, transient-sentinel, honesty-rule, phase-one, issue-12]
    path: convention/2026-06-19-text-arg-transient-sentinel.md
    summary: "text arg stays in the schema for forward-compat; handler fast-errors with errTextNotImplemented until mcptext adapter (#17) lands. Honest contract over silent fallback. Superseded 2026-06-20 by mcptext adapter landing — sentinel removed, text resolves end-to-end."

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
