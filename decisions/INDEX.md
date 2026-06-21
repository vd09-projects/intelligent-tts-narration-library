# Decision Index

<!--
  This file is maintained by the decision-journal skill.
  Entries are in YAML format for machine-friendly querying.
  Newest entries go at the top. Do not manually reorder.
-->

```yaml
decisions:
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
    status: accepted
    category: architecture
    tags: [internal/errclass, intelligence/mcpsampling, error-classification, import-coupling, layering, dedup, issue-51]
    path: architecture/2026-06-21-errclass-imports-mcpsampling-one-classifier-place.md
    summary: "Task #51 consolidated the duplicated caller-vs-internal-vs-cancel ladder (the // DUP marker) into internal/errclass. DECISION (Option A): errclass imports intelligence/mcpsampling solely to recognize its two adapter sentinels (ErrNoSamplingClient, ErrUnexpectedContentKind) and classify both as ClassInternal — keeping ALL shared classification in ONE place, honoring the prior fact that sampling sentinels route to internal. Rejected Option B (omit the sentinels, re-handle them at the MCP root): re-duplicates the very logic #51 consolidates and leaves MCP with two classifiers. P1 verified no import cycle (mcpsampling imports only plan/, intelligence/, MCP SDK; never reaches internal/) and no layering-lint flag. The latent coupling errclass -> intelligence/mcpsampling is documented as a named edge in the errclass.go package doc. Revisit (fall back to Option B for the two sentinels) if a cycle or layering flag appears."
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
    status: accepted
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
