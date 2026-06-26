<!-- rune-generated: 2026-06-18 | git: unknown | rune: 1.0 -->

# Skald — Scope Registry

```yaml
scopes:
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
