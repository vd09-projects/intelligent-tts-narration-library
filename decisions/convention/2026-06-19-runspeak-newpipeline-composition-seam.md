# runSpeak composition seam — `newPipeline` factory hook

- **Date:** 2026-06-19
- **Status:** accepted
- **Category:** convention
- **Tags:** [cmd/narrate-mcp, mcp, testability, composition-root, seam, factory, phase-one, issue-12]
- **Owner:** vd
- **Scope:** cmd-narrate-mcp-issue-12

## Context

Build-review v1 (B2 from Test Coverage Auditor) flagged that the original `runSpeak` had no seam to stub the pipeline composition — tests could only stub the whole `runSpeak` via `runDeps.run`, not its inside. That meant we could not verify that `level` / `voice` / `locale` correctly thread from `speakArgs` into `pipeline.PipelineDefaults` and `pipeline.NarrateRequest`. The composition wiring was implicitly trusted.

## Decision

Introduce a package-level factory hook in `cmd/narrate-mcp/main.go`:

```go
type narrator interface {
    Narrate(ctx context.Context, ref plan.SourceRef, req pipeline.NarrateRequest) (sink.SinkReceipt, error)
}

var newPipeline = func(outDir string, args speakArgs) narrator {
    return pipeline.New(
        file.New(),
        nil,
        sherpa.New(sherpa.EngineConfig{}),
        ephemeral.New(),
        pipeline.PipelineDefaults{
            Level:  plan.Level(args.Level),
            OutDir: outDir,
            Locale: "en",
        },
    )
}
```

Tests swap `newPipeline` (a package-level `var`) via a `withStubPipeline(t, stub)` helper. The `stubNarrator` captures `gotRef`, `gotReq`, `gotOutDir`, and an `outDirExistedAtNarrate` flag so the test can assert both the wired arg flow and the temp-dir lifecycle.

Why a `var` and not a parameter to `runSpeak`: keeping the seam at the package level means production callers (the MCP handler, the manual smoke test) don't need to pass a factory — they get the production wiring by default. The seam is invisible to production callers and only visible to in-package tests.

## Consequences

- `TestRunSpeak_HappyPath_WiresLevelVoiceLocaleAndReturnsReceipt` verifies the wiring contract directly.
- `TestRunSpeak_TempDir_CleanedUpOnSuccess` and `TestRunSpeak_TempDir_CleanedUpOnPipelineError` confirm `defer RemoveAll` actually fires.
- Mutating tests cannot use `t.Parallel()` — small price for the seam clarity.
- Pattern is reusable in `cmd/narrate` if a similar gap surfaces there.

## Related decisions

- decisions/architecture/2026-06-18-pipeline-composition-root-pattern.md
- decisions/convention/2026-06-19-mcp-error-classifier-caller-vs-internal-split.md

## Revisit trigger

If `runSpeak` grows enough non-pipeline composition (e.g. injectable temp-dir factory, injectable logger) that the package-level seam stops scaling — at that point promote to a constructor that takes a deps struct.
