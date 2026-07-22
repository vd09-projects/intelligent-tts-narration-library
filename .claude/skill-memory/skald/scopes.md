<!-- rune-generated: 2026-06-18 | git: unknown | rune: 1.0 -->

# Skald — Scope Registry

```yaml
scopes:
  - slug: comma-grouped-integer-spellout-139
    title: Comma-grouped integer spell-out in planner voicing (issue #139)
    created: 2026-07-09
    reasoning: |
      Follow-up to #138 (planner-number-spellout). GitHub issue #139 — extends the
      prose number-speller in planner/numwords.go to voice well-formed comma-grouped
      integers ("24,700" -> "twenty-four thousand seven hundred") that #138 explicitly
      deferred (left as digits). Scope slug supplied verbatim by the caller via
      --scope; issue-numbered to match the sibling planner-number-spellout lineage.
      Separate scope (not an iteration of planner-number-spellout) because it is a
      distinct GitHub issue with its own plan/build/review cycle.
  - slug: earshot-playback-engine-112
    title: Earshot playback engine — block-quantized transport deck + scrubber + block-level resume persistence (issue #112)
    created: 2026-06-29
    reasoning: |
      Plan cycle for GitHub issue #112 — the Earshot web-app playback ENGINE that
      replaces the #111 scaffold's placeholder native <audio controls> with a real
      transport: bottom-anchored APG role=toolbar deck + roving tabindex, a
      block-quantized role=slider scrubber (aria-valuetext = spoken-time + block
      index), a first-class "⟲ return to playing block" control, a rebuilt BlockRow
      "⏯ from here" single-button affordance (and removal of the OLD player/ dual
      aria-hidden mouse-only seek wrapper bug the ticket calls out), and block-level
      resume persistence across reloads via client-side localStorage (keyed by
      session+message / file). Resume granularity block-level ONLY — persists
      blockId + blockSignature and re-derives startMs from the live timeline, never
      a time offset (word-level timing forbidden invariant). mimir task depth, 6
      ordered steps (usePlayback rAF loop writing SET_ACTIVE_BLOCK only on block-id
      change → resumeStore localStorage module → resume wiring → TransportDeck
      toolbar → BlockScrubber slider → BlockRow rebuild + player/ bug removal).
      Overlays accessibility (APG toolbar/slider, action-label play/pause with NO
      aria-pressed) + state-management (persistence shape + derivation DAG, no state
      lib, listen path decoupled from persistent sink) force-relevant; phased-delivery
      NOT activated (single atomic PR, not a multi-phase rollout). Honors ADR #108
      (transport shape), ADR #77 (block-quantized seek), and the four playback
      decisions (rAF block-transition writes, whole-wav blob unit, re-point playing
      block on patch, block-id-signature reset). Slug pre-supplied + pre-approved via
      --scope by the build-session orchestrator (<area>-<issue> form); issue-numbered
      suffix continues convention. Distinct from earshot-leveling-ui-113 (leveling
      control) and earshot-scaffold-app-shell-111 (the shell this builds on). Plan
      persisted draft (awaiting user approval before sindri consumes).
  - slug: earshot-leveling-ui-113
    title: Earshot per-block L1/L2/L3 leveling control — escalate/de-escalate with in-place audio swap (issue #113)
    created: 2026-06-28
    reasoning: |
      Plan/build cycle for GitHub issue #113 — the per-block L1/L2/L3 leveling
      control in the Earshot React app (earshot/), letting a listener
      escalate/de-escalate any voiced or degraded block's fidelity and swap just
      that block's audio in place via the already-shipped POST /narrate/block
      (#110). First client consumer of /narrate/block, so it also carries one
      additive server field (render_id on narrateResponse) bundled in. mimir
      task depth, 6 phases (server field -> earshot types+postNarrateBlock ->
      useNarrationSession.escalateBlock + (blockId,level) cache + reload nonce ->
      LevelControl APG role=radiogroup modeled on SegmentedToggle -> BlockRow
      wiring hidden-on-refused -> tests). Overlays accessibility + state-management
      force-relevant (APG radiogroup/roving tabindex/role=status loading; single-
      owner server-cache merge). Slug pre-supplied + pre-approved via --scope by
      the build-session orchestrator (<area>-<issue> form); issue-numbered suffix
      continues convention. Plan persisted draft (awaiting user approval before
      sindri consumes).
  - slug: speak-to-file-mcp-105
    title: speak_to_file MCP tool — text/.md file to single wav at path (issue #105)
    created: 2026-06-28
    reasoning: |
      Plan/build/review cycle for GitHub issue #105 — add a third MCP tool
      speak_to_file to cmd/narrate-mcp that renders inline text or a text/.md
      file into a single .wav at a caller-given output_path, with no-path
      fallback delegating to ephemeral speak. Earshot use case 3 (priority
      tool, built first). Factors a reusable single-wav WAVFileSink (no
      plan.json/manifest.json sidecars) out of sink/persistent without
      coupling to the ephemeral or 3-file persistent paths. Slug supplied
      pre-resolved by the build-session orchestrator (<area>-<issue> form);
      issue-numbered suffix continues convention.
  - slug: refusal-omission-accounting-98
    title: Refusal-aware transcript omission accounting for MCP speak receipt (issue #98)
    created: 2026-06-27
    reasoning: |
      Plan/build/review cycle for GitHub issue #98 — the refusal-aware omission
      accounting follow-on explicitly named by the accepted #86 transcript-cap
      decision. Adds an additive omitempty field counting refused blocks dropped
      from the head-keep/tail-truncate cap so a consumer reading the transcript
      view alone sees they exist. Slug supplied pre-resolved by the build-session
      orchestrator (<area>-<issue> form); issue-numbered suffix continues convention.
  - slug: planner-table-meaning-l2-47
    title: Wire intelligence into table L2/L3 meaning-summary (issue #47)
    created: 2026-06-26
    reasoning: |
      Plan/build/review cycle for GitHub issue #47 — make levelTable L2/L3
      request an AI meaning summary (parity with levelDiagram/levelCode),
      degrading to the current deterministic header/row reading with no adapter.
      Slug was supplied pre-resolved by the build-session orchestrator
      (planner-<area>-<issue> form); issue-numbered suffix continues convention.
  - slug: cmd-narrate-issue-7
    title: Wire cmd/narrate vertical slice (issue #7)
    created: 2026-06-18
    reasoning: |
      Plan + build + review + iterate + harvest cycle for GitHub issue #7
      (cmd/narrate composition root: cobra CLI + pipeline.Pipeline wiring
      adapter/file + planner + render/sherpa + sink/ephemeral). Issue-numbered
      naming continues the convention.
  - slug: sink-ephemeral-issue-6
    title: Ephemeral OutputSink + sink interface (issue #6)
    created: 2026-06-19
    reasoning: |
      Build + review + iterate cycle for GitHub issue #6 (sink/ interface
      replacing stub + sink/ephemeral concrete impl shelling to afplay).
      Plan was approved out-of-band by user; this scope skips planner-task
      and goes straight to implementation-build. Issue-numbered naming
      continues the convention.
  - slug: render-sherpa-issue-5
    title: Renderer + Kokoro subprocess (issue #5)
    created: 2026-06-18
    reasoning: |
      Plan + build + review + iterate cycle for GitHub issue #5
      (render/render.go interface + render/sherpa/ subprocess backend
      wrapping the existing scripts/kokoro launcher). Issue-numbered
      naming continues the convention.
  - slug: planner-issue-4
    title: planner/ intelligence-light core (issue #4)
    created: 2026-06-18
    reasoning: |
      Plan + build + review + iterate + commit cycle for GitHub issue #4
      (planner core: segment, classify, voice, level, degrade, orchestrator).
      Issue-numbered for parity with adapter-file-issue-3 naming.
  - slug: adapter-file-issue-3
    title: File InputAdapter (issue #3)
    created: 2026-06-18
    reasoning: |
      Review + iterate + commit cycle for GitHub issue #3 (file InputAdapter).
      Slug includes issue number to disambiguate from future adapter-mcptext-issue-N,
      adapter-ocr-issue-N. Kept short.
```
  - slug: cmd-narrate-mcp-issue-12
    title: MCP server cmd/narrate-mcp (issue #12)
    created: 2026-06-19
    reasoning: |
      Plan + build + review + iterate + harvest cycle for GitHub issue #12
      (cmd/narrate-mcp stdio MCP server exposing `speak` tool, wiring the
      same pipeline.Pipeline composition root as cmd/narrate). Issue-numbered
      naming continues the convention.
  - slug: plan-enum-consistency-issue-10
    title: refactor(plan) enum consistency for VoicingDirective + Diagnostic (issue #10)
    created: 2026-06-20
    reasoning: |
      Plan + build + review + iterate + harvest cycle for GitHub issue #10
      (refactor plan/ to lift Diagnostic.Severity, VoicingDirective.SayAs,
      VoicingDirective.Emphasis from freeform strings to typed enum aliases
      with IsValid() — consistency with existing six enums in plan/enums.go).
      Issue-numbered naming continues the convention.
  - slug: intelligence-mcpsampling-issue-13
    title: intelligence/mcpsampling adapter — zero-cost L2/L3 via MCP client LLM (issue #13)
    created: 2026-06-20
    reasoning: |
      Plan + build + review + iterate + harvest cycle for GitHub issue #13
      (intelligence/mcpsampling/ — first concrete IntelligenceAdapter,
      riding MCP sampling/createMessage on the connected client LLM).
      Becomes the precedent #15 (intelligence/anthropic) follows. Issue-
      numbered naming continues the convention.
  - slug: cmd-narrate-block-rerender-14
    title: cmd/narrate --block + --expected-content-hash per-block re-render (issue #14)
    created: 2026-06-20
    reasoning: |
      Build + review + iterate + harvest cycle for GitHub issue #14
      (cmd/narrate --block <id> --level {2|3} + --expected-content-hash
      per-block re-render. Plan locked by prior mimir run; sindri builds
      to spec in 3 phases — pipeline plumbing, cmd flag + roster, README).
      Issue-numbered naming continues the convention.
  - slug: intelligence-anthropic-issue-15
    title: intelligence/anthropic direct-API adapter (issue #15)
    created: 2026-06-20
    reasoning: |
      Build + review + iterate + harvest cycle for GitHub issue #15
      (intelligence/anthropic — second concrete IntelligenceAdapter, direct
      Anthropic API path for CLI users using ANTHROPIC_API_KEY env var).
      Plan pre-supplied + pre-approved by user in orchestration prompt;
      sindri builds in 7 phases — lift prompt templates, scaffold, Voice(),
      cache, retry, CLI wire, deps_test. Issue-numbered naming continues
      the convention.
  - slug: player-react-issue-18
    title: React reference player (player/) — issue #18
    created: 2026-06-20
    reasoning: |
      Build + review + iterate + harvest cycle for GitHub issue #18
      (player/ — Vite + React + TypeScript reference UI consuming a
      sink/persistent output dir; demonstrates block-level sync, honest
      refusal display, per-block escalate command surface). Plan pre-supplied
      + pre-approved by user in orchestration prompt; sindri builds via the
      11-step ordered scaffold (Vite init → TS types → pure helpers →
      loaders → playback hook → fixture → components → Makefile → smoke
      test → README → manual verify). Issue-numbered naming continues the
      convention.
  - slug: intelligence-anthropic-oauth-bearer-32
    title: opt-in OAuth Bearer auth for subscription tokens (issue #32)
    created: 2026-06-20
    reasoning: |
      Plan + build + review + iterate + harvest cycle for GitHub issue #32
      (intelligence/anthropic — add opt-in Authorization: Bearer auth mode so
      a claude setup-token subscription OAuth token (sk-ant-oat01-) works
      against POST /v1/messages; x-api-key stays the default, Bearer is
      opt-in via WithBearerAuth() + auto-detected on the sk-ant-oat01 prefix).
      Distinct slug from intelligence-anthropic-issue-15 (the base adapter) —
      this is a follow-on auth feature on the same package, so an explicit
      oauth-bearer qualifier rather than reusing the issue-15 scope. Plan via
      mimir task depth + auth-authz overlay. Issue-numbered naming continues
      the convention.
  - slug: sink-persistent-block-patch-28
    title: sink/persistent --block patch into existing persistent outDir (issue #28)
    created: 2026-06-21
    reasoning: |
      Plan + build + review + iterate + harvest cycle for GitHub issue #28
      (feat(sink/persistent): --block patch into an existing persistent
      outDir). Reopens Decision v1.9.0 (block-with-persistent-sink-rejected-
      at-flag-time) from issue #16: replaces the flag-time blanket refusal
      with a guarded PatchBlock splice path that re-renders one block and
      rewrites audio.wav + manifest.json + plan.json atomically. Distinct
      slug from sink-persistent-issue-16 (the base feature) — this is a
      follow-on patch-path feature on the same package, so an explicit
      block-patch qualifier rather than reusing the issue-16 scope.
      Issue-numbered naming continues the convention.
  - slug: lint-remove-unused-34
    title: chore(lint) remove dead unused symbols flagged by golangci-lint on main (issue #34)
    created: 2026-06-20
    reasoning: |
      Build + review + close cycle for GitHub issue #34 (chore(lint): remove
      two pre-existing `unused` golangci-lint findings on clean main — the
      gotOpts test-scaffolding field in cmd/narrate/main_test.go and the
      never-wired errorResponse type in intelligence/anthropic/api.go). Vibe-
      sized 2-symbol lint cleanup; no mimir plan artifact (single scoped
      ticket, went straight to sindri build). Descriptive-qualifier slug
      (lint-remove-unused) + issue number, continuing the issue-numbered
      naming convention. Unblocked once #32 merged (PR #33).
  - slug: plan-typed-enum-voicedby-23
    title: typed enum for Provenance.VoicedBy (issue #23)
    created: 2026-06-20
    reasoning: |
      Plan + build + review + close cycle for GitHub issue #23
      (refactor(plan): typed enum for Provenance.VoicedBy — follow-up to #10).
      Finishes the plan/ enum-typing sweep: Provenance.VoicedBy is the last
      bare-string enum-shaped field; lift to typed alias + IsValid() per the
      accepted decision 2026-06-20-typed-enum-pattern-wins-for-all-enum-shaped.
      Single scoped vibe ticket with no open architectural decision, so planned
      via sindri plan mode (mimir skipped). Descriptive-qualifier slug
      (plan-typed-enum-voicedby) + issue number, continuing the issue-numbered
      naming convention.
  - slug: sink-ephemeral-polish-11
    title: sink/ephemeral polish — review-findings v1 suggestions (issue #11)
    created: 2026-06-21
    reasoning: |
      Plan + build + review + close cycle for GitHub issue #11 (polish bucket
      rolling up the 9 non-blocking suggestions from sink-ephemeral-issue-6's
      review-findings v1 APPROVE; parent #6). Distinct from sink-ephemeral-issue-6
      (the parent build scope) — this is the follow-up polish ticket, so it gets
      its own scope. Descriptive-qualifier slug (sink-ephemeral-polish) + issue
      number, continuing the issue-numbered naming convention. Priority low,
      rune:vibe; mimir planned (multi-AC bucket benefits from per-phase grouping).
  - slug: mcpsampling-cache-eviction-25
    title: "intelligence/mcpsampling: cross-call cache lifetime + eviction policy (issue #25)"
    created: 2026-06-21
    reasoning: |
      Plan + build + review + close cycle for GitHub issue #25 (resolve the
      two deferred items from #13: promote the per-call mcpsampling cache to
      per-server lifetime, and add eviction so the now-long-lived sync.Map
      stops being a memory leak). Distinct slug from intelligence-mcpsampling-
      issue-13 (the base adapter build) — this is a follow-on lifetime+eviction
      feature on the same package, so an explicit cache-eviction qualifier
      rather than reusing the issue-13 scope. Planned via mimir task depth with
      the concurrency overlay (eviction-time writes vs Voice() readers under
      the shared lifetime). Issue-numbered naming continues the convention.
  - slug: planner-data-races-42
    title: fix data races in planner package under -race (issue #42)
    created: 2026-06-21
    reasoning: |
      Plan cycle for GitHub issue #42 (kill the DATA RACE that make test-race
      surfaces in the planner batch). Root cause is two unsynchronized
      package-global test seams (nowFunc/newPlanIDFunc) swapped by ~8
      t.Parallel() tests while a sibling Plan() reads them; the three
      detector-named tests are bystanders. Follow-on to the planner-races
      note flagged during mcpsampling-cache-eviction-25 review (a follow-up
      was requested before make test-race becomes a CI gate). Distinct
      issue-numbered slug, planner- prefixed to mark the package under fix
      and disambiguate from planner-issue-4 (the original core build).
      Planned via mimir task depth with the concurrency overlay. Plan was
      pre-produced externally by mimir and persisted draft (awaiting user
      approval before sindri consumes). Issue-numbered naming continues the
      convention.
  - slug: code-semantic-gist-l2-48
    title: AI semantic gist for code at L2 (issue #48)
    created: 2026-06-22
    reasoning: |
      Plan cycle for GitHub issue #48 (feat(planner): AI semantic meaning-gist
      for code blocks at L2). Today L1 = deterministic line count, L2 = count +
      declarations (both deterministic), and a real "what this code does" gist
      only appears at L3 with an intelligence adapter. #48 enriches L2 to set
      needsIntelligence=true for a one-line AI gist, keeping L1 deterministic
      (load-bearing invariant), degrading to today's count+decls when no adapter
      is wired (honesty rule), and size-gating ~250-line seamless code blocks to
      keep the deterministic structural gist there. Sibling of #47 (table L2/L3)
      — same L1-deterministic / L2-needsIntelligence / degrade-to-deterministic /
      cache-by-(hash,level,model) shape; #48 may land the first in-source
      instance of that pattern for a structured class. Decision
      2026-06-22-code-semantic-gist-l2-only already recorded (accepted); no new
      decision needed. Planned via mimir task depth (no overlays triggered —
      pure-function internal planner change + additive internal/intelligencetmpl
      prompt, no public-API / migration / concurrency / perf surface). Plan
      persisted draft (awaiting user approval before sindri consumes). Issue-
      numbered naming continues the convention.
  - slug: planner-parallel-intelligence-46
    title: parallelize per-block intelligence Voice calls in the planner (issue #46)
    created: 2026-06-21
    reasoning: |
      Plan cycle for GitHub issue #46 (perf(planner): parallelize per-block
      intelligence Voice calls). Plan loops rawBlocks sequentially and each
      intelligence-needing block blocks on its own intel.Voice LLM round-trip,
      so N such blocks pay the sum of N serial calls (main win: the anthropic
      direct-API adapter; mcpsampling may serialize client-side). Plan fans the
      Voice calls out concurrently (errgroup + bounded SetLimit) while keeping
      the planner pure + deterministic + honest: a single-threaded structural
      pass assigns all IDs/Order (retiring the racy *idx pointer from the hot
      path, honoring the no-shared-mutable-state decision from
      planner-data-races-42) then a bounded concurrent pass fills only the
      Voice-dependent blocks into index-disjoint result slots. planner- prefix
      marks the package under change and disambiguates from planner-issue-4
      (original core build) and planner-data-races-42 (the -race seam fix this
      plan explicitly builds on). Planned via mimir task depth with perf-critical
      + concurrency overlays (both left ON in mimir config; brief is an explicit
      perf+concurrency task). Plan persisted draft (awaiting user approval before
      sindri consumes). Issue-numbered naming continues the convention.
    created: 2026-06-21
    reasoning: |
      Plan cycle for GitHub issue #51 (extract the duplicated classifyPipelineErr
      caller-vs-internal-vs-cancel error CLASSIFICATION into a new internal/errclass
      package consumed by both cmd/narrate-mcp and cmd/narrate-server). The // DUP
      marker in cmd/narrate-server/main.go statusForErr (added by #49) explicitly
      pointed at #51 as the "3rd consumer lands, extract now" trigger; recorded as
      carried tech debt across the server-escalate-endpoint-49 review rounds. Only
      the classification (sentinel -> Class enum) is shared; each composition root
      keeps its per-root wire-format mapping (MCP text prefixes vs server HTTP status
      + reason tokens) at the call site. internal/ placement (sibling to
      internal/intelligencetmpl) keeps the helper off the public surface, imported
      only by the cmd/ roots. Descriptive-qualifier slug (errclass-extract) + issue
      number, continuing the issue-numbered naming convention. Planned via mimir task
      depth (no overlays triggered — internal-only package, no public API / migration
      / concurrency surface). Plan persisted draft (awaiting user approval before
      sindri consumes).
  - slug: planner-list-ordinals-45
    title: speak list items with ordinals — first, second, third (issue #45)
    created: 2026-06-21
    reasoning: |
      Plan cycle for GitHub issue #45 (vibe). levelList in planner/level.go
      currently voices a list as "List of N items: a. b. c." with no spoken
      position markers; the change prefixes each item with an ordinal cue
      ("First, alpha. Second, beta. ...") plus a numeric fallback past ~10
      items ("item 11, ..."). Deterministic, no intelligence adapter, no
      schema change — only the string inside Segment.Text changes. planner-
      prefixed to mark the package under change; issue-numbered suffix
      continues the convention (cf. planner-data-races-42). Planned via mimir
      task depth, no overlays triggered (text-only single-segment change).
  - slug: player-escalate-buttons-50
    title: in-place Escalate L2/L3 buttons in React player (issue #50)
    created: 2026-06-22
    reasoning: |
      Plan cycle for GitHub issue #50 (player/ — evolve the per-block Escalate
      button from a copy-the-CLI-command card into a real in-place escalation:
      click hits the local HTTP escalate endpoint from #49, spinner on the row,
      then patches just that block into React state — text/status/level/refusal +
      swapped BlockTiming + re-pointed audio source — with the command card kept
      as the static-fixture fallback via a /healthz mode toggle). Front-end
      consumer of the #49 escalate server. player- prefixed to mark the package
      under change and disambiguate from player-react-issue-18 (the original
      reference-player build); issue-numbered suffix continues the convention.
      Planned via mimir task depth (5-phase breakdown). Plan persisted draft —
      awaiting user approval before sindri consumes.
  - slug: server-escalate-livedir-62
    title: Server-mode escalate re-fetch resolves against the live escalated dir, not FIXTURE_BASE (#62)
    created: 2026-06-21
    reasoning: |
      Plan cycle for GitHub issue #62 (server-mode escalate re-fetch must resolve
      against the live escalated outDir, not the hardwired FIXTURE_BASE). Follow-on
      to player-escalate-buttons-50: the in-place escalate landed there left two
      self-describing KNOWN GAP (tracked: #62) markers at player/src/App.tsx:128/161
      because both re-fetch call sites (repointAudio, reloadManifest) still point at
      FIXTURE_BASE, so a server-mode user escalating a non-fixture outDir re-fetches
      the wrong manifest/audio (stale downstream offsets). Fix is a server-contract
      change in cmd/narrate-server (new read-only GET /artifact?dir=&name= route
      serving ONLY the escalated dir's {manifest.json, audio.wav} allowlist under the
      existing withCORS + loopback-only) plus a pure player refetchBase resolver
      threaded through the live-ref pattern. server- prefixed to mark the cmd/narrate-
      server package as the primary edge under change (the player change is the
      consumer side) and to disambiguate from server-escalate-endpoint-49 (the base
      escalate-endpoint build this resolves the downstream-staleness gap of); livedir
      qualifier captures the live-escalated-dir-vs-FIXTURE_BASE crux. Planned via mimir
      task depth with public-api-change + infra-blast + state-management overlays
      (new public HTTP route surface + static-file-serving blast radius incl
      traversal/symlink containment + React re-fetch state invariants). Retires the
      [revisit-later] FIXTURE_BASE phase-one-limitation decision. Plan persisted draft
      — awaiting user approval before sindri consumes. Issue-numbered naming continues
      the convention.
  - slug: intelligence-code-l2-wordcap-60
    title: enforce one-sentence/~30-word cap on code-L2 adapter replies (issue #60)
    created: 2026-06-22
    reasoning: |
      Plan cycle for GitHub issue #60 (enforce the advertised CodeUserL2
      "one sentence, at most 30 words" cap on code-L2 intelligence-adapter
      replies). Today CodeUserL2 (internal/intelligencetmpl/tmpl.go:113) only
      instructs the adapter; nothing enforces it, so an over-long reply lands
      verbatim in Segment.Text — the constraint is advisory. Plan adds the
      enforcement at the single generic choke point in planner/planner.go
      callIntelligence (line 347, where res.Text becomes Segment.Text for BOTH
      the anthropic + mcpsampling adapters), guarded code-L2-only. Core decision
      (candidate 2026-06-22-code-l2-overlong-reply-trim-to-sentence): trim to the
      first sentence (adapter's own words = honest) with the word cap as a hard
      ceiling -> refuse via the existing res.Refused path when even the first
      sentence over-runs, never truncate mid-sentence (= borderline fabrication).
      Follow-on to code-semantic-gist-l2-48 (which introduced CodeUserL2);
      distinct slug because this is the enforcement feature on top of that gist
      build, not the gist build itself. intelligence- prefix marks the
      intelligence-reply enforcement surface; wordcap qualifier captures the crux;
      issue-numbered suffix continues the convention. Planned via mimir task depth,
      no overlays triggered (pure-function internal planner change, no public-API /
      migration / concurrency / perf surface). AC3 reframed as a ticket divergence
      (no // TODO marker exists in tmpl.go to remove — nothing to remove). Plan
      persisted draft — awaiting user approval before sindri consumes.
  - slug: errclass-drop-mcpsampling-58
    title: drop dead mcpsampling branches + concrete import from internal/errclass (issue #58)
    created: 2026-06-22
    reasoning: |
      Plan cycle for GitHub issue #58 (drop the two dead mcpsampling.Err*
      ladder branches (precedence 4-5: ErrNoSamplingClient,
      ErrUnexpectedContentKind) and the intelligence/mcpsampling import from
      internal/errclass). Both branches return ClassInternal — identical to the
      precedence-6 default arm — so they are pure no-ops with zero
      classification effect; their sole reason to exist was to justify importing
      the concrete mcpsampling backend into the shared classifier, which
      violates the only-pipeline/cmd-know-concrete-backends invariant. Removal
      restores that invariant. Byte-identical refactor proven by the existing
      cmd/narrate-mcp + cmd/narrate-server behavior oracles (no new assertions);
      supersedes the 2026-06-21-errclass-imports-mcpsampling-one-classifier-place
      decision on dead-branch grounds (NOT the originally-anticipated
      cycle/layering trigger), landing on Option B's outcome for the sentinels
      while "one classifier per root" still holds. Distinct slug from
      errclass-extract-51 (the base extraction that introduced the import) — this
      is a follow-on cleanup on the same package, so an explicit drop-mcpsampling
      qualifier rather than reusing the issue-51 scope. Slug pre-supplied +
      pre-approved by user in orchestration prompt. Planned via mimir task depth,
      no overlays triggered (internal-only no-op-branch removal, no public API /
      migration / concurrency / perf surface). Plan persisted draft (awaiting
      user approval before sindri consumes). Issue-numbered naming continues the
      convention.
  - slug: narrate-claude-code-mcp-listen
    title: narrate Claude Code response over MCP (listen-not-read) — block highlight, pause/play, code-as-L2, loading state
    created: 2026-06-22
    reasoning: |
      Architecture-depth ADR cycle for the listen-not-read feature (implementation
      issue #73). Bounded analysis (no shipped product code) on how to wire the MCP
      `speak` path so a user can LISTEN to a large Claude Code response block-by-block:
      persistent current-block highlight, pause/play, code blocks voiced at L2 only
      (honesty fallback to deterministic when no adapter), and a visible loading
      indicator while TTS generates. Recommends Option B — playback state lives in
      the existing React player fed by a sink/persistent outDir, with the MCP host
      buffering the COMPLETE response and handing it to the pipeline as one whole
      input (reconciles "as it arrives" against the no-streaming non-goal). The
      library emits nothing new: block-level Timeline/manifest.json keyed by block_id
      plus the whole audio.wav blob is already the UI contract. Reuses player +
      escalate/spinner (#50), cmd/narrate-server escalate endpoint (#49/#62), code-L2
      path (#48/#60), and the anthropic adapter (#32) — net new is only the
      buffer-then-render-then-point-player glue. Flags the stale mark3labs/mcp-go
      ticket reference vs the official modelcontextprotocol/go-sdk v1.5.0 (CLAUDE.md
      source of truth) and confirms this work forces no further SDK finalization.
      Honors all five prior decisions (whole-blob playback unit, usePlayback
      block-id-signature reset, code-L2-only AI gist, rAF block-transition-only
      state writes, receipt-only speak envelope). Slug pre-supplied + pre-approved
      via --scope in the orchestration prompt; feature-descriptive (not issue-
      numbered) since this ADR precedes and unblocks #73 rather than implementing it.
      Planned via mimir architecture depth, no overlays force-activated (bounded
      analysis, no shipped code). Plan persisted draft (awaiting user approval).
  - slug: narrate-mcp-listen-impl-73
    title: Implement Claude Code MCP narration listen path (issue #73, Part A)
    created: 2026-06-23
    reasoning: |
      Task-level implementation breakdown for GitHub issue #73, the build that
      the approved narrate-claude-code-mcp-listen v3 ADR unblocks. Part A
      (primary) only: whole-response buffering convention, code-class -> L2 plan
      default expressed via pipeline/PipelineDefaults (no new MCP arg), honesty
      fallback + error-path proofs, and the stale mark3labs/mcp-go -> official
      modelcontextprotocol/go-sdk v1.5.0 reference correction. Part B (React
      player visual companion) deferred to a separate follow-up issue per the
      decoupling standing order. Separate impl scope (not the architecture scope)
      to keep planner-architecture.md and the build artifact cleanly split.
      Slug pre-supplied via --scope; issue-numbered to mark it as the impl of #73.
  - slug: mcp-speak-transcript-78
    title: surface the spoken transcript from the MCP speak path (observability) — issue #78
    created: 2026-06-24
    reasoning: |
      Plan cycle for GitHub issue #78 (surface the verbatim spoken transcript +
      per-block level + refused/degraded status from the MCP speak path —
      cmd/narrate-mcp -> ephemeral sink -> afplay — which today returns a
      receipt-only envelope {blocks_played, total_duration_ms, out_dir} and
      discards the spoken text already computed in the rendered plan). The
      observability half of the playback gap; the design is already decided by
      ADR #77 (Playback observability & control model) — this is the scoped
      Channel-1 implementation ticket: an after-the-fact MCP receipt transcript
      in structuredContent + a duplicate serialized-JSON TextContent block (D3),
      block-granularity only (D5), no live progress (v1.5.0 calls synchronous).
      mcp-speak- prefix marks the MCP speak surface under change; transcript
      qualifier captures the crux; issue-numbered suffix continues the
      convention. Slug pre-supplied + pre-approved via --scope in the
      orchestration prompt. Planned via mimir task depth with the
      public-api-change overlay (the MCP speak tool result shape + pipeline
      NarrateResult/BlockSummary Go API are real consumer contracts; change is
      additive/minor). The mimir observability overlay was correctly NOT
      activated (it is in never_overlays — a production-telemetry overlay,
      irrelevant to "expose already-computed plan data on a receipt", and
      CLAUDE.md bars monitoring). Plan persisted draft (awaiting user approval
      before sindri consumes). Issue-numbered naming continues the convention.
  - slug: listen-transport-controller-83
    title: Listen-path keypress-loop transport controller for cmd/narrate (#83)
    created: 2026-06-25
    reasoning: |
      Plan cycle for GitHub issue #83 (cmd/narrate — add a raw-mode single-key
      transport loop so a listener can step next/back, replay, jump, and quit a
      block-by-block narration, with one SIGINT/SIGTERM handler owning the
      cooked-tty restore + afplay reap + once-per-session temp-dir removal).
      Lives entirely in the cmd/narrate composition root; planner/ + plan/ stay
      I/O-free. Design pre-accepted (decision 2026-06-25-listen-transport-
      keypress-loop-not-tui — keypress loop, not a full TUI). 7 ordered steps:
      1–3 parallel foundations (play seam mirroring sink/ephemeral
      playWithAfplay with exec.Cmd.WaitDelay, navigable-block index, raw-mode
      sync.Once guard), 4–6 integrate (keypress state machine, single signal
      handler, --listen cobra wire + mutually-exclusive validate vs
      --block/--sink=persistent), 7 Makefile/manual harness. Planned via mimir
      task depth with the concurrency overlay (shared afplay-child handle, tty
      raw/cooked state, single-owner temp-dir RemoveAll — at-most-one-child
      reap-before-spawn + idempotent restore from deferred AND handler).
      Honesty rule literal: controls labeled "Stop / Replay block", never
      "Pause". Listen path kept decoupled from the durable sink. listen-
      transport- prefix marks the new cmd/narrate transport surface; controller
      qualifier captures the crux; issue-numbered suffix continues the
      convention. Slug pre-supplied + pre-approved via --scope in the
      orchestration prompt. Plan pre-produced externally by mimir, persisted
      draft (awaiting user approval before sindri consumes).
  - slug: narrate-observe-jsonl-tail-81
    title: cmd/narrate-observe decoupled observer — JSONL tail-f live view (#81, ADR #77 D5 Channel 2)
    created: 2026-06-25
    reasoning: |
      Plan cycle for GitHub issue #81 (ADR #77 D5 Channel 2: a user-launched,
      read-only cmd/narrate-observe binary that tail -f's an ephemeral per-call
      JSONL scratch file the synchronous speak handler writes one line per block
      to — a live 2nd-terminal block-by-block playback view the after-the-fact
      Channel 1 receipt structurally cannot give). Wiring only: an injectable
      per-block emit seam in sink/ephemeral (WithBlockObserver Option +
      BlockEvent, defined purely over plan/render data, zero Channel-2 knowledge,
      nil-safe off-by-default), a concrete JSONL writer at the cmd/narrate-mcp
      composition root, and a ~30-line dumb reader. JSONL append + tail -f chosen
      as the DEFAULT Channel-2 mechanism over MCP NotifyProgress (surfaces to a
      human terminal vs the MCP client; zero deps; ephemeral /tmp no durable
      coupling) — decision mark to harvest: channel-mechanism accepted (JSONL
      default, NotifyProgress additive-later). Follow-on to mcp-speak-transcript-78
      (the Channel-1 receipt half of the same ADR #77 playback gap); distinct
      slug because this is the Channel-2 live-observer build, not the Channel-1
      transcript. narrate-observe- prefix marks the new observer surface; jsonl-tail
      qualifier captures the chosen mechanism/crux; issue-numbered suffix continues
      the convention. Slug pre-supplied + pre-approved via --scope in the
      orchestration prompt. Planned via mimir task depth with the public-api-change
      overlay (new WithBlockObserver Option + BlockEvent + JSONL wire contract +
      cmd/narrate-observe binary are real consumer surfaces). Plan pre-produced
      externally by mimir, persisted draft (awaiting user approval before the
      implementation skill consumes).
  - slug: react-visual-companion-listen-76
    title: Part B — optional React visual companion for MCP listen-mode (#76, parent #73)
    created: 2026-06-25
    reasoning: |
      Task-level implementation breakdown for GitHub issue #76 / parent #73
      Part B — the optional, opt-in on-screen React visual companion for MCP
      listen-mode. Part A (terminal-audio listen path) shipped in PR #75. Part B
      reuses the ENTIRE shipped React player (player/) + cmd/narrate-server
      stack (block highlight, whole-audio.wav play/pause, inline-card escalate
      spinner #50, usePlayback block-id-signature reset R7, #62 /artifact
      re-fetch) and builds nothing new except the render-failure terminal
      signal: a bounded readiness handshake resolving spinner -> loaded ->
      defined failure. Named tokens: server no_out_dir appended to the closed
      append-only reason enum (404 on a new GET /readiness?dir=); readiness
      states rendered/rendering on a 200 body; player terminal phase
      render_failed (from no_out_dir, bound-expiry, or dead-server). Honors the
      decoupling standing order (separate --sink persistent invocation, never a
      speak tee — Risk 6/AC6) and all five prior decisions (optional/no-new-UI,
      terminal listen = ephemeral afplay no spinner, block-id-signature reset
      not manifest identity, inline command card escalate UX, ADR #77 two-channel
      model). Slug pre-supplied + pre-approved via --scope in the orchestration
      prompt; feature-descriptive + issue-numbered (parallels the Part A impl
      scope narrate-mcp-listen-impl-73, kept separate so the companion build
      artifact stays cleanly split from Part A). Planned via mimir task depth
      with overlays public-api-change (new /readiness endpoint + no_out_dir token
      + plan.json /artifact allowlist entry — additive server-contract change)
      and state-management (the readiness poll/give-up state machine + App
      load-gate + the block-id-signature reset decision). Plan persisted draft
      (awaiting user approval before sindri consumes).
  - slug: listen-nav-boundary-tests-89
    title: Listen mode — add n-at-last / b-at-first navigation boundary tests (issue #89)
    created: 2026-06-27
    reasoning: |
      Plan cycle for GitHub issue #89 (test-only — add table-driven coverage
      for the two untested runListen navigation boundary guards in
      cmd/narrate/listen.go: `n` at the last navigable block, listen.go:388
      `if pos < len(nav)-1`, and `b` at position 0, listen.go:392 `if pos > 0`).
      Existing listen_test.go covers the keypress sequence, go-to, shutdown,
      ctx-cancel, and no-navigable cases but never the two boundary no-ops. One
      new table-driven TestRunListen_NavigationBoundaries (2 rows) asserts the
      no-op indirectly via spawn count (rec.playN) + last wav played
      (rec.playArgs) since `pos` is a runListen local not observable directly.
      Zero production-code changes, reuses the ephemeral stub harness
      (recordingCleanup/scriptedBytes/navTimeline) — no durable/persistent sink
      (standing decoupling order). Follow-on coverage on the cmd/narrate listen
      transport surface from listen-transport-controller-83 (the base keypress-
      loop build); distinct slug because this is a focused test-coverage ticket,
      so a descriptive listen-nav-boundary-tests qualifier rather than reusing
      the issue-83 scope. listen- prefix marks the listen transport surface;
      issue-numbered suffix continues the convention. Slug pre-supplied +
      pre-approved via --scope in the orchestration prompt. Plan pre-produced
      externally by mimir, reviewed APPROVE (review-findings v1), then consumed
      by sindri. Latest artifact: review-findings v2 (draft, review_type build,
      iteration 1) — APPROVE, 0 blocking; plan review v1 archived to
      _history/review-findings-v1.md. Single file touched
      (cmd/narrate/listen_test.go, +59, one table-driven
      TestRunListen_NavigationBoundaries, 2 rows), zero production-code change;
      make test/lint/sanity all green and the new test passes under -race
      (re-verified at build review). Build review carried both plan-review nits
      (load-bearing audioDir, precise failure-mode comment); 1 FYI + 1 optional
      cfg-builder suggestion, no blocking items, no accepted debt.
  - slug: listen-interruptible-read-88
    title: Listen mode — interruptible read so SIGTERM-while-idle is serviced without a keypress (issue #88)
    created: 2026-06-27
    reasoning: |
      Plan cycle for GitHub issue #88 (cmd/narrate listen mode — make the
      runListen blocking os.Stdin.Read interruptible so an external kill -TERM
      while the listener is idle on the read is serviced WITHOUT a keypress;
      today the deferred restore()/cleanup() never fires while idle, leaving the
      tty in raw mode and the afplay child unreaped until a key is pressed).
      Follow-up to listen-transport-controller-83 / PR #87 (the base keypress-loop
      build): MakeRaw disables ISIG so Ctrl-C arrives as byte 0x03 and the single
      signal handler only catches SIGTERM/terminal-close, but the loop is parked
      in a bare blocking readByte between iterations so the shutdown channel is
      not observed until the next byte. Chosen fix (mimir Approach A): a stdin
      reader goroutine pumping bytes onto byteCh + a loop select over
      {byteCh, shutdown, ctx.Done()} — preserves the listenConfig readByte/
      readLine/shutdown seam (every existing stub keeps working), no fragile
      tty-deadline/stdin-close tricks. Key finding surfaced during planning: the
      existing TestRunListen_ShutdownDuringRead is false-confidence — it
      pre-loads the shutdown request AND returns io.EOF immediately, so it never
      exercises a genuinely-blocked read and would pass against the broken code;
      the plan supersedes it with a table-driven TestRunListen_InterruptibleRead
      whose readByte blocks on a never-fed test channel. Distinct slug from
      listen-transport-controller-83 (the base build) and listen-nav-boundary-
      tests-89 (the boundary-test follow-on) — this is the interruptible-read
      fix on the same listen surface, so a descriptive listen-interruptible-read
      qualifier rather than reusing those scopes. listen- prefix marks the
      cmd/narrate listen transport surface; issue-numbered suffix continues the
      convention. Slug pre-supplied + pre-approved via --scope in the
      orchestration prompt. Planned via mimir task depth with the concurrency
      overlay (reader goroutine + byteCh/shutdown/ctx select, single-reader-of-
      stdin invariant, loop-stays-sole-owner-of-child reap, cleanup-never-joins-
      reader, -race test discipline). Plan pre-produced by mimir, persisted draft
      (awaiting user approval before sindri consumes).
  - slug: mcp-transcript-cap-86
    title: Cap transcript size on very large MCP speak responses (issue #86)
    created: 2026-06-27
    reasoning: |
      Plan cycle for GitHub issue #86 (cap/bound the per-block transcript[] the
      MCP speak tool returns on its receipt — today unbounded, and a very large
      speak response carries the whole array twice: once in structuredContent and
      once in the duplicate serialized-JSON TextContent block per ADR #77 D3). A
      number-less TODO(#86) marker sits at cmd/narrate-mcp/main.go:365 in runSpeak
      where transcriptFromResult is assembled into speakResponse.Transcript.
      Chosen strategy (mimir): head-keep / tail-truncate by entry count with a
      default const transcriptCap=200, plus two additive omitempty signal fields
      (transcript_truncated bool + transcript_omitted_count int) on speakResponse;
      cap lives in a pure capTranscript helper (identity-return under cap so the
      common small case stays byte-identical) wired at the TODO site — the
      dual-channel handler needs no change since it marshals the one capped struct,
      so both channels inherit the bound. Follow-on to mcp-speak-transcript-78 (the
      Channel-1 transcript build that introduced transcript[] and left the size-cap
      TODO); distinct slug because this is the bounding feature on top of that
      build, not the build itself. mcp- prefix marks the MCP speak surface;
      transcript-cap qualifier captures the crux; issue-numbered suffix continues
      the convention. Slug pre-supplied + pre-approved via --scope in the
      orchestration prompt. Planned via mimir task depth with the public-api-change
      overlay (the MCP speak result shape is a real consumer contract; change is
      additive/minor — two omitempty fields + a runtime array-length bound, no
      field removed/renamed/retyped, no deprecation). Honesty boundary noted:
      capping an observability receipt, not fabrication — no Refusal involved.
      Plan persisted draft (awaiting user approval before sindri consumes).
  - slug: narrate-block-escalation-110
    title: narrate-server /narrate/block single-block escalation endpoint — re-render one block by render_id (issue #110)
    created: 2026-06-28
    reasoning: |
      Plan cycle for GitHub issue #110 — add a narrate-server POST /narrate/block
      {render_id, block_id, level} endpoint that re-renders a SINGLE block at a
      higher/different level via the existing per-block patch path (no whole-doc
      re-plan). The HTTP-bridge (#109, now merged) analog of the CLI/core escalate
      shipped in #49/#50. Mimir task plan surfaced the load-bearing crux: #109's
      POST /narrate mints a single combined wav keyed by render_id (R-NB4 — no
      plan.json/manifest.json sidecars), so a render_id is NOT patchable by
      sink/persistent.PatchBlock today; the dir-keyed /escalate handler already
      implements the full capturingSink->PatchBlock->on-disk-readBack path.
      Recommended resolution (decision-journal candidate, Open Q1, load-bearing):
      switch the server /narrate render to a 3-file sink/persistent outDir per
      render_id, map render_id->outDir in renderStore, extract runEscalate's
      post-resolve body into a shared escalateInDir(...) core consumed by both
      /escalate and /narrate/block, add a thin handler. Patch path (PatchBlock,
      capturingSink, readBack, exported ReadManifest at sink/persistent/
      manifest.go:96) reused UNCHANGED — ReadManifest already exists so the brief's
      add-if-absent clause is moot. Overlay public-api-change (additive: one new
      route + response struct, reused reason-token enum, no existing wire shape
      changed). narrate-block- prefix marks the new escalation endpoint surface;
      issue-numbered suffix continues the convention. Slug pre-supplied +
      pre-approved via --scope in the orchestration prompt (build-session
      single-skill runner). Plan persisted draft (awaiting user approval before
      sindri consumes).

  - slug: integrate-oto-v3-player-101
    title: integrate oto v3 player + WAV-header-strip behind listen transport (#101)
    created: 2026-06-27
    reasoning: |
      Plan cycle for GitHub issue #101 — the PRODUCTION build that follows the
      #100 throwaway spike (spike-oto-v3-listen-pause-100). Productionizes the
      accepted 2026-06-27 true-pause-via-oto-v3-no-CGo decision: collapse the
      //go:build oto / !oto seam pair so github.com/ebitengine/oto/v3 (v3.4.0,
      Apache-2.0; purego v0.9.0 Apache-2.0 zero-CGo shim) becomes the DEFAULT and
      only cmd/narrate listen engine (afplay removed from the listen path only —
      sink/ephemeral afplay untouched). Resolves the two spike-carried debts: (1)
      oto v3.4 finalizer-teardown fd-lifecycle (oto-v3-4-player-close-no-op-
      finalizer-teardown decision) — chosen approach is an IN-MEMORY PCM buffer
      (*bytes.Reader over the stripped data chunk; no *os.File held while the
      player runs, so the read-from-closed-fd window is designed out, not merely
      sequenced; otoBlock drops its file field; Pause()-before-drop kept for
      deterministic halt); (2) RIFF dataChunkStart/io.Seeker — bytes.Reader is
      natively io.ReadSeeker+io.ReaderAt, satisfying the #77 Seek(offset,whence)
      substrate (actual seek KEYBINDINGS out of scope). Adds an injectable
      listenPlayer seam (Play/Pause/IsPlaying/Err) + newPlayer factory on
      listenConfig so the unified runListen is device-free unit-testable
      (oto.NewContext only in driveListen, the composition root — honors
      only-cmd/pipeline-know-concrete-edges + the listen-transport-keypress-loop
      decision). integrate- prefix + oto-v3-player qualifier marks the production
      player build; distinct slug from spike-oto-v3-listen-pause-100 (the
      throwaway proof this consumes). Slug pre-supplied + pre-approved via --scope
      in the orchestration prompt (build-session single-skill runner). Planned via
      mimir task depth with the concurrency overlay (fd-ownership/finalizer +
      loop-owned handle + single-reader-of-stdin + sync.Once cleanup; go test
      -race the gate) — brief explicitly pre-authorized concurrency if the
      fd-ownership surface warranted it; it did. Open question carried to build:
      drop the afplay listen fallback entirely (recommended) vs keep a
      --engine=afplay escape hatch. Plan persisted draft (awaiting user approval
      before sindri consumes). Issue-numbered naming continues the convention.
  - slug: spike-oto-v3-listen-pause-100
    title: oto v3 listen-path wiring + true Pause/Resume by-ear spike (#100)
    created: 2026-06-27
    reasoning: |
      Spike for GitHub issue #100 (parent research #92) — first wiring of the
      accepted 2026-06-27 true-pause-via-oto-v3-no-CGo decision into the
      cmd/narrate listen path. Proves github.com/ebitengine/oto/v3 can consume
      real Kokoro 24 kHz mono int16 PCM via io.Reader and deliver true
      Pause()/Resume() by ear, behind a //go:build oto tag so the shipped afplay
      run-listen loop stays the default and untouched. spike- prefix marks it a
      throwaway-grade proof (not the production player — that is #101);
      oto-v3-listen-pause qualifier captures the crux; issue-numbered suffix
      continues the convention. Slug pre-supplied + pre-approved via --scope in
      the orchestration prompt (build-session single-skill runner). Planned via
      mimir task depth, no overlays (solo project; concurrency model already
      handled by the existing transport; observability/cross-team silenced by
      mimir never_overlays). Plan persisted draft (awaiting user approval before
      sindri consumes). NOTE: this run re-persisted after a prior run produced
      the mimir plan but failed to write the scope dir.
  - slug: earshot-mockup-artifact-115
    title: Earshot clickable mockup — free static-React Claude Artifact validating L1/L2/L3 control + transport anchor (issue #115)
    created: 2026-06-28
    reasoning: |
      Plan cycle for GitHub issue #115 — the sign-off-gate mockup that informs
      the real earshot/ app build (#111/#112/#113). Build ONE self-contained
      static React file pasteable into a FREE Claude Artifact ($0 — no
      Live/AI/MCP/persistent features; claude.ai Claude Design rejected on
      no-recurring-spend) that makes the design's two weakest-grounded surfaces
      clickable and user-testable: the per-block L1/L2/L3 radiogroup segmented
      control (grade-F bounded-negative novel synthesis — radiogroup not a
      disclosure because L2 is a real middle state) and the bottom-anchored
      transport, with a cheap top/bottom anchor toggle so the comparison is
      testable in-place. All data hard-coded fixtures; playback + escalation
      SIMULATED in local state — never wired to narrate-server or the Go core
      (fidelity prototype only). Honors the accepted "Earshot UI finalized"
      decision (Material list-detail, bottom transport, APG-grounded controls,
      free-Artifact mockup method) and the server-driven-rebuild / glob-session
      decisions as context (mockup itself talks to nothing). Implements
      research/earshot-ui-design-108/report.md §5–§9 wireframe + interaction
      tables. earshot-mockup- prefix marks the throwaway fidelity-mockup surface
      (distinct from the real earshot/ build scopes #111/#112/#113); artifact
      qualifier captures the Claude-Artifact delivery medium; issue-numbered
      suffix continues the convention. Slug pre-supplied + pre-approved via
      --scope in the orchestration prompt (build-session single-skill runner).
      Planned via mimir task depth with overlays accessibility (entire spec is
      APG/ARIA/keyboard/focus/screen-reader + the radiogroup intuitiveness probe)
      and state-management (active-block as shared derived state across
      transport↔transcript; no persistence — deliberately throwaway). Plan
      persisted draft (awaiting user approval before sindri consumes).

  - slug: earshot-keyboard-transport-134
    title: Earshot keyboard transport shortcuts — Space play/pause + ←/→ ±10s seek (#134)
    created: 2026-06-29
    reasoning: |
      Plan cycle for GitHub issue #134 — a low-priority convenience layer adding
      YouTube-style keyboard shortcuts to the shipped #112 Earshot transport
      (earshot/): Space = play/pause, Left/Right = seek ±10s. Mimir task plan
      (accessibility overlay) RESOLVED the core design decision to issue Option
      (a): Left/Right = ±10s TIME seek on the combined wav (a free-moving playhead
      is transport/display only and violates no block-level-sync invariant, since
      nothing sub-block is persisted), NOT block-step — block-step stays on the
      slider's own native arrows. Plan adds seekBySeconds(deltaSec) to the
      block-quantized PlaybackControls surface (reads audioRef currentTime/duration,
      clamps [0,dur], no-ops when metadata unloaded), a new useTransportHotkeys hook
      mounted once in Shell (document-level keydown reading the SINGLE usePlayback
      via usePlaybackControls() — honors the single-instance decision), and a
      deny-list focus guard that bails inside role=toolbar / role=slider / native-
      Space owners so global keys never clash with the two #112 keyboard tab stops
      (the load-bearing two-tab-stop decision). Discoverability via aria-keyshortcuts
      + a visible legend (no help modal); no-audio/at-end/empty hardened. 5 ordered
      steps; focus-guard table test is the regression net. Honors the four prior
      Earshot decisions (two tab stops, single usePlayback at provider, block-level-
      sync-only with the playhead-granularity nuance, APG-grounded controls) and the
      standing order (low-pri mid-build: fix inline, no follow-up tickets). earshot-
      prefix groups the Earshot surface; keyboard-transport qualifier scopes this to
      the shortcut layer (distinct from earshot-playback-engine-112, the engine it
      builds on); issue-numbered suffix continues the convention. Slug pre-supplied +
      pre-approved via --scope by the build-session single-skill runner. Planned via
      mimir task depth with the accessibility overlay (entire feature is keyboard /
      APG / focus-guard / aria-keyshortcuts; state-management considered but NOT
      activated — no new persistence/derivation, just reads the existing context
      surface + ephemeral DOM focus checks). Plan persisted draft (awaiting user
      approval before sindri consumes).
  - slug: planner-number-spellout
    title: Deterministic cardinal number spell-out in planner voicing pass
    created: 2026-07-09
    reasoning: |
      Plan cycle for a deterministic number→words spell-out step in the planner
      voicing pass — plain standalone integers/decimals in PROSE spoken as
      cardinals (24700 → "twenty-four thousand seven hundred"; 3.5 → "three point
      five") instead of the Kokoro subprocess reading them digit-by-digit. Layer
      decision was settled BEFORE mimir ran (not relitigated): belongs in the
      deterministic planner voicing pass, NOT the intelligence layer — structured
      classes voice at L1 with no adapter (adapter never sees those numbers),
      number→words is deterministic + faithful (not fabrication → honesty rule
      satisfied), same "deterministic, opinionated, faithful" bucket as the
      lexicon. Verified seam: planner/voice.go voice(text,lex) strips markdown then
      runs the longest-match lexicon byte-scan; the prose verbatim path
      degradeProse() (planner/degrade.go:135) calls voice(TrimSpace(rb.text),lex);
      no number-to-words logic anywhere. Numbers are unbounded → cannot be lexicon
      entries → own pure pass. mimir task depth, 6 steps: new pure
      planner/numwords.go (spellCardinal(uint64) with ones/teens/tens + scale
      groups, no "and", hyphenated 21–99; decimal handling as "point" + per-digit
      fractional readout; spellNumbers(text) guard+tokenizer). Conservative scope
      — spell ONLY plain standalone numeric tokens; the guard LEAVES as digits
      anything `:`/`=`/`/`-adjacent, dotted-version (1.25 / v2.4.0, multi-dot), hex
      (0x…), ≥7-digit runs (ID/phone/port/epoch), or letter/`_`-adjacent; unsure →
      leave (a wrong cardinal is worse than clunky-but-correct digits). Negatives
      conservatively NOT spelled (`-5` sign-vs-hyphen ambiguity). Two load-bearing
      decisions surfaced in planning: (1) ORDERING per the planner-classifier-
      sniff-order precedent — the number pass is a DISTINCT function sequenced
      BEFORE the lexicon voice() call in degradeProse (voice(spellNumbers(
      TrimSpace(rb.text)),lex)) so the guard reads original source adjacency (the
      lexicon scan pads operators with spaces and would erase the colon/equals/
      slash signal) AND cardinals don't leak into structured-class gists (voice()
      is shared by code/table/diagram degrade paths); (2) BOUNDARY — a `.` is a
      decimal point only if a digit immediately follows, so "24700." at sentence
      end spells 24700 and preserves the terminal `.`, while 3.5 is a decimal.
      Table-driven test matrix (plain/decimal/thousands spells + version/hex/port/
      long-run/colon-adjacent leaves + boundaries 0 / single-digit / exactly-7-
      digits / trailing-punct) + golden plan.json check on any affected prose
      fixture; no golden audio. Tension recorded vs the 2026-06-21 list-ordinal-cue
      decision (which deliberately AVOIDED a general number speller, hand-rolling
      ordinals 1–10 + numeric "item N" fallback): this work INTRODUCES the general
      integer/decimal speller that decision punted on, but scope is cardinals-in-
      prose ONLY — level.go's list path is untouched (a future ticket could back-
      fill ordinals). Honors DefaultLexicon frozen+overridable spirit (deterministic,
      opinionated, faithful) and the no-I/O-in-planner invariant (pure string
      function). planner- prefix marks the package under change and disambiguates
      from planner-issue-4 (the original core build) and planner-list-ordinals-45;
      number-spellout qualifier captures the crux; NON-issue-numbered descriptive
      slug (no GH issue cited in the brief). Slug pre-supplied + pre-approved via
      --scope by the build-session single-skill runner. Planned via mimir task
      depth, no overlays triggered (pure-function internal planner change, no
      public-API / migration / concurrency / perf surface). Plan persisted draft
      (awaiting user approval before sindri consumes). Genuine FIRST persist —
      version:1, scope dir newly created.
  - slug: earshot-scaffold-app-shell-111
    title: Earshot scaffold — app shell + session pane + file pane (#111)
    created: 2026-06-28
    reasoning: |
      Plan/build cycle for GitHub issue #111 — the FIRST real earshot/ web app
      build scope (distinct from the throwaway earshot-mockup-artifact-115
      fidelity prototype). Builds the SHELL: scaffold + tooling, the #108
      Material list-detail layout (left session pane, center spoken transcript,
      bottom transport placeholder), session pane (role=listbox; session ID ->
      GET /sessions/{id}/messages -> click -> POST /narrate), file pane (drop/
      pick -> POST /narrate/file), center transcript with Spoken|Source toggle,
      and audio off GET /audio/{render_id}.wav. Server-driven off narrate-server
      (#109); replaces deleted passive player/ (#107); mockup signed off #115.
      Slug pre-supplied + pre-approved via --scope in the orchestration prompt
      (build-session single-skill runner). earshot- prefix groups the Earshot
      surface; scaffold-app-shell qualifier scopes this to the shell (deeper
      transport interactions, per-block escalation, resume-across-reload are
      explicit follow-on tickets, NOT this scope); issue-numbered suffix per
      convention. Planned via mimir task depth with overlays phased-delivery
      (6 per-phase commit boundaries; user wants per-phase commits not one
      bundle) and accessibility (entire #108 spec is APG/ARIA/keyboard/focus/
      screen-reader-grounded). Plan persisted draft (awaiting user approval
      before sindri consumes).
  - slug: rvc-torch-free-worker-144
    title: RVC P1 — torch-free Python RVC inference worker (scripts/rvc_worker.py, issue #144)
    created: 2026-07-21
    reasoning: |
      Plan cycle for GitHub issue #144 — the P1 of the RVC integration chain
      (#143 export tooling done -> #144 worker -> #145 render/rvc Go decorator ->
      #146 CLI/MCP wiring -> #147 verify+docs). Productionizes the pilot's
      throwaway onnx_infer.py into a durable EPHEMERAL PER-JOB worker: a
      scripts/rvc bash wrapper (mirrors scripts/kokoro) activating a DEDICATED
      lean torch-free venv (.venv-rvc: onnxruntime + numpy + faiss-cpu + scipy +
      librosa + soxr, NO torch — must NOT reuse the shared .venv, so the
      torch-free assertion is meaningful) that execs scripts/rvc_worker.py, which
      loads the #143-exported ONNX models (contentvec/rmvpe shared + per-voice
      net_g) + the original faiss .index ONCE, then services a stdin/stdout line
      loop (`<in> <out> <voice> <index_rate> <pitch>` -> `OK <out>` / `ERR ...`)
      converting every block warm until EOF, then EXITS -> 0 idle RAM. Uses the
      original faiss .index directly (the Python worker HAS faiss); the
      index_vectors.npy dump is the FUTURE Go-kNN fallback, out of scope here.
      Engine-faithful reproduction of PILOT_REPORT §4 DSP (soxr VHQ resample, N=5
      48Hz zero-phase Butterworth filtfilt, rmvpe STFT/htk-mel/cents-decode,
      faiss IVFFlat k=8 index-blend, pitch-protect 0.33, ×2 nearest interp,
      external rnd ~ N(0,1), [40000:-40000] trim) — explicitly NO DSP redesign.
      6 ordered steps (1 venv+requirements -> 2/3 parallel wrapper+DSP-core ->
      4 model-registry+warm-load-once+stdin/stdout loop -> 5 standalone parity
      test -> 6 Makefile targets+smoke+docs). Success metric = make rvc-parity
      green: per-stage ONNX-vs-refio corr >= 0.999 (contentvec/rmvpe/net_g fed
      the *_refio.npz fixtures) + full-pipeline log-mel corr >= 0.98 vs a torch
      reference WAV, both voices (cool-jahns index_rate 0.75, confident-neal
      index_rate 0.5) at pitch 0, torch-free asserted (`"torch" not in
      sys.modules`). Honors the four prior RVC/kokoro decisions (torch-free ONNX
      ephemeral per-job worker; kokoro-onnx-over-torch ONNX-via-subprocess
      precedent; wrapper lives in scripts/ not render/sherpa/; per-block WAVs stay
      separate, no concat -> one-WAV-in/one-WAV-out worker shape) and the standing
      orders (60s/10min timeouts owned by the LATER Go caller #145 not the worker;
      worker failure = `ERR ...`/nonzero exit surfaced as a hard error, never a
      Refusal; edge-only — zero plan/ or planner/ changes; Apache-2.0, torch kept
      OUT of the inference venv). New Makefile targets: rvc-worker-venv (create
      .venv-rvc from scripts/rvc-requirements.txt), rvc-parity (run the standalone
      parity test), rvc-convert (single-line by-ear smoke). rvc- prefix marks the
      RVC surface; torch-free-worker qualifier captures the crux; distinct from
      the sibling export scope (scripts/rvc-export, #143) — this is the runtime
      worker, not the export tooling; issue-numbered suffix continues the
      convention. Slug pre-supplied + pre-approved via --scope by the
      build-session single-skill runner. Planned via mimir task depth with the
      public-api-change overlay (the worker's stdin/stdout wire protocol is the
      load-bearing contract the #145 Go decorator binds to — consumer inventory +
      new-surface-v1 additive/append-only forward-compat rule + protocol-loop
      contract test; directly analogous to the narrate-observe-jsonl-tail-81
      precedent that activated public-api-change for a new wire contract + binary).
      infra-blast + perf-critical were CONSIDERED but NOT activated: infra-blast's
      content is production deploy/canary/rollback discipline (N/A to a local-only
      hobby project — CLAUDE.md bars deploy/CI), and the goal is pilot PARITY, not
      latency optimization (latency is inherently the pilot's ~7s/clip). Plan
      persisted draft (awaiting user approval before sindri consumes). Genuine
      FIRST persist — version 1, scope dir newly created.
  - slug: rvc-render-decorator-145
    title: RVC P2 — Go render/rvc decorator over sherpa (issue #145)
    created: 2026-07-22
    reasoning: |
      Plan cycle for GitHub issue #145 — P2 of the RVC integration chain
      (#143 export tooling done -> #144 torch-free worker done/PR #148 merged ->
      #145 render/rvc Go decorator [THIS] -> #146 CLI/MCP + sink 40kHz Format
      wiring -> #147 verify+docs). Directly consumes the frozen v1 wire contract
      shipped by the sibling scope rvc-torch-free-worker-144: a Go render.Renderer
      DECORATOR (render/rvc) wrapping the existing render/sherpa Kokoro engine that
      repaints each per-block 24kHz WAV through scripts/rvc_worker.py as an
      EPHEMERAL PER-JOB subprocess (spawn ONE worker per Render, warm across blocks
      via deadlock-free lockstep request/response + concurrent stderr drain, exit
      at job end -> ~0 idle RAM; one-shot cold convert for RenderBlock escalation).
      Owns the voice->Kokoro-source map (cool-jahns->am_michael index_rate 0.75 /
      confident-neal->af_bella index_rate 0.5, pitch 0 ALWAYS — non-zero rejected
      by the worker, not clamped) so the 24kHz source voice can't drift from the
      RVC target. Honesty rule: worker missing/startup-fatal(78)/runtime-fatal(70)/
      per-block ERR/protocol-violation = HARD Go error with a fix hint
      (make rvc-export / make rvc-worker-venv), NEVER a silent degrade to plain
      Kokoro. Rebuilds Timeline from the repainted 40kHz durations (recomputed, not
      scaled) and sets Format = 40kHz mono s16le on RenderResult/BlockRender/
      Timeline; the downstream sink Format.SampleRate de-hardcode is #146, flagged
      as a dependency but out of scope. Table-driven tests + a bash+python3 fake
      worker (mirrors render/sherpa/testdata/fake-kokoro.sh) so CI runs with no
      torch, no .venv-rvc, and no exported models. Does NOT touch index_vectors.npy
      (reserved for the deferred in-process Go kNN path); in-process onnxruntime-go
      (Option D) deferred. rvc- prefix marks the RVC surface; render-decorator
      qualifier captures the crux (Go decorator, distinct from the #144 Python
      worker scope and the #143 export scope); issue-numbered suffix continues the
      convention. Slug pre-supplied + pre-approved via --scope by the build-session
      single-skill runner. Planned via mimir task depth, base template (no overlays
      triggered — single-package Go build wrapping an already-frozen contract).
      Plan persisted draft (awaiting user approval before sindri consumes). Genuine
      FIRST persist — version 1, scope dir newly created.
  - slug: rvc-p3-cli-mcp-server-wiring-146
    title: RVC P3 — CLI + MCP + server wiring (shared buildRenderer + voice arg) (issue #146)
    created: 2026-07-22
    reasoning: |
      Plan cycle for GitHub issue #146 (RVC P3) — the pure-WIRING follow-on that
      threads the already-merged render/rvc decorator (#145) into all three
      composition roots behind an optional user-facing `voice` argument, routed
      through ONE shared pipeline.BuildRenderer(voice) -> (render.Renderer,
      plan.AudioFormat, error) helper. Six production seams construct a bare
      sherpa.New today; #146 introduces the helper, exposes a voice knob on every
      root (cool-jahns / confident-neal), and makes the decorator's 40kHz format
      land cleanly in the format-validating persistent sinks — translating NOTHING
      (the decorator owns the target->Kokoro-source map). 7 ordered commit-sized
      phases: (1) export rvc.IsSupportedVoice / rvc.SupportedVoices /
      rvc.OutputFormat (40kHz); (2) test-only proof that a 40kHz WAV round-trips
      through sink/persistent Consume/PatchBlock/NewWAVFile via WithExpectedFormat,
      plus a negative test that a 40kHz WAV against the default 24kHz expected
      format still fails ErrFormatMismatch (locks strict validation); (3)
      pipeline.BuildRenderer as the single shared factory; (4) cmd/narrate --voice
      + reject --voice+--listen; (5) cmd/narrate-mcp speak/speak_last voice arg;
      (6) cmd/narrate-server --voice launch flag; (7) docs + rvc-sanity make target
      + by-ear /verify. Load-bearing facts: 6 (not 5 — the server has two) bare
      sherpa.New seams; the ms<->byte math already reads format.SampleRate
      (prove-don't-rewrite); the real 40kHz gap is the persistent-family
      expectedFormat defaulting to 24kHz. Honesty rule literal: unknown voice /
      absent worker = returned error -> non-zero exit or MCP caller-error, NEVER a
      silent Kokoro fallback; empty voice = byte-identical plain Kokoro 24kHz.
      Invariant guard: only pipeline/ + cmd/ know concrete engines; pipeline/ newly
      importing concrete sherpa+rvc is invariant-legal (no cycle) and planner/ +
      plan/ stay I/O-free and engine-neutral (no RVC slug in the plan schema). 5
      decisions D1-D5 (D1 buildRenderer signature returns the format so
      renderer-format and sink-expected-format are single-origin; D2 home is
      pipeline/, not a new render/renderbuild package; D3 export membership+format
      from render/rvc not the translation map; D4 reject --voice+--listen for #146,
      full listen a follow-up; D5 server --voice is a launch flag, per-request
      deferred), 5 risks (stub-seam ripple across the 6 test-swapped var seams,
      missed WithExpectedFormat site, gender/voice confusion, pipeline
      concrete-engine imports, worker-unavailable ergonomics), 4 open questions
      (OQ1 listen reject, OQ2 server launch-flag, OQ3 pipeline home, OQ4
      gender/voice non-fatal notice). rvc- prefix marks the RVC surface;
      cli-mcp-server-wiring qualifier captures the crux and disambiguates from
      rvc-torch-free-worker-144 (Python worker) and rvc-render-decorator-145 (the
      Go decorator this consumes); issue-numbered suffix continues the convention.
      Slug pre-supplied + pre-approved via --scope by the build-session
      single-skill runner. Planned via mimir task depth, base template (no overlays
      triggered — pure wiring across composition roots over an already-frozen
      decorator contract). Plan persisted draft (awaiting user approval before
      sindri consumes). Genuine FIRST persist — version 1, scope dir newly created;
      a prior persistence attempt failed to write the scope dir, so nothing had
      landed and this is v1, not an iteration.
