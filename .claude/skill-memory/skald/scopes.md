<!-- rune-generated: 2026-06-18 | git: unknown | rune: 1.0 -->

# Skald — Scope Registry

```yaml
scopes:
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
