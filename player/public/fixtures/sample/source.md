# What this library does and why

This document is the canonical demo input for the vertical-slice CLI. The narrator reads each block by class — headings as headings, prose as prose, code as code, lists item by item, tables row by row — and refuses to fabricate when no intelligence adapter is wired. The honest refusal you will hear later in this document is a feature, not a bug.

## The premise

Most text-to-speech systems either spell every character out loud, which is exhausting, or summarize the entire document with a separate AI pass, which is expensive and hides what was actually written. This library takes a middle path. Per-block leveling lets the listener ask for a one-sentence gist of one paragraph, a summary of one section, or a verbatim read of another, without re-planning the whole document. The planner is the brain. The renderer is the voice. The sink is the speaker. Edges plug into the planner through narrow interfaces, so swapping the renderer from Kokoro to something else, or the file adapter for an MCP one, does not ripple through the rest of the code.

The honesty rule is the load-bearing one. If the renderer is asked to voice a chart it cannot see, or a thousand-word essay without an intelligence adapter that could summarize it, the planner does not guess. It records a refusal in the plan with a short spoken notice and a pointer to the source. The listener hears, in plain English, what was not voiced and why. The refusal travels through the pipeline as data, not as an error. The whole document still plays end to end.

## Pipeline shape

The four edges sit around the planner core. None of them imports the planner, and the planner imports none of them. Composition happens once, at the binary entry point. This makes it cheap to add a phase-two MCP server, a phase-three Anthropic-backed intelligence adapter, or a phase-four React reference player without touching the contract.

## Example code

```go
pl := pipeline.New(
    file.New(),
    nil,                       // no intelligence adapter — deterministic + degraded
    sherpa.New(sherpa.EngineConfig{}),
    ephemeral.New(),
    pipeline.PipelineDefaults{Level: plan.L1, OutDir: outDir, Locale: "en"},
)
receipt, err := pl.Narrate(ctx, ref, pipeline.NarrateRequest{Voice: "af_bella"})
```

## Defaults at a glance

| Knob | Default | Notes |
|---|---|---|
| Level | L1 (gist) | Re-requestable per block. |
| Voice | af_bella | Kokoro female. Switch to am_michael via --gender=male. |
| Sink | ephemeral | Plays via afplay. Persistent sink is phase two. |
| Locale | en | Multilingual is deferred. |

## What you should notice

- The headings each become their own short utterance, distinct from the prose around them.
- The code block above is read by structure, not character by character.
- The table reads row by row, with the header announced once.
- The bare image below is refused honestly — you will hear a brief notice naming the source line, not a fabricated description.

![chart](nonexistent-chart.png)

The block immediately above is a bare Markdown image with no surrounding caption or alt text. The planner has no way to see what the image depicts. The refusal you just heard is the honesty rule at work. If a future intelligence adapter is wired in with vision support, that adapter could describe the image and turn the refusal into a voiced block. Until then, silence-with-explanation beats invention.
