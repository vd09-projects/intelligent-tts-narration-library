# Persistent sink voice id flows via WithVoice Option, not plan.Timeline

- **Date:** 2026-06-20
- **Status:** accepted
- **Category:** architecture
- **Tags:** [sink, persistent, plan-schema, engine-neutrality, composition-root, voice-id, issue-16]
- **Owner:** vd
- **Scope:** issue-16

## Context

`manifest.json` carries the engine voice id (e.g. `af_bella`, `am_michael`) so a downstream player or audit reader can answer "what voice produced this audio?". The voice id is engine-specific (Kokoro vocab); the plan schema is engine-neutral. The question: how does the voice id reach the sink without contaminating the plan?

The composition root (`cmd/narrate`) already owns the gender → voice id mapping (`genderToVoice`). The renderer also has the voice id internally. The sink could get it from three places:

## Options considered

### Option A: `persistent.WithVoice(voiceID string)` Option on the sink (CHOSEN)
- **Pros**: Composition root passes the voice id at construction time. `plan/` schema stays engine-neutral. The sink stays oblivious to gender → voice mapping. Empty voice id is a valid manifest (the sink does not invent).
- **Cons**: The voice id is duplicated at the composition root — once for the renderer's `pipeline.NarrateRequest.Voice`, once for the sink's `WithVoice`. Acceptable: composition roots are the right place to own seam-spanning identity.

### Option B: Add a `Voice` field to `plan.Timeline`
- **Pros**: One source of truth (the plan), no composition-root duplication.
- **Cons**: Contaminates the engine-neutral plan with engine-specific identifiers. The plan/ invariant from CLAUDE.md ("Plan stays engine-neutral and pre-render") is load-bearing. Hard no.

### Option C: Infer the voice from a previously-written manifest at re-write time
- **Pros**: Self-healing.
- **Cons**: Circular — the manifest doesn't exist on a fresh write. The sink would have to invent on first write and read-back on subsequent writes. Two code paths for one field.

### Option D: Read the voice from the renderer (`render.RenderResult.Format` extended with voice)
- **Pros**: The renderer knows the voice.
- **Cons**: `render.RenderResult.Format` is `plan.AudioFormat` — engine-neutral (sample rate, channels, encoding). Adding voice to it contaminates a second engine-neutral type. Same flavor of problem as Option B.

## Decision

`sink/persistent` exposes `persistent.WithVoice(voiceID string) Option`. The composition root (`cmd/narrate.chooseSink`) calls `persistent.New(args.Out, persistent.WithVoice(genderToVoice[args.Gender]))`. The sink writes the voice id verbatim into `manifest.Voice`; an absent option leaves the field empty (`""`), which is a valid manifest.

The voice id is duplicated at the composition root: once into `pipeline.NarrateRequest.Voice` (for the renderer to resolve), once into `persistent.WithVoice` (for the manifest). The duplication is honest — the composition root is the only place that knows both the renderer's input format and the sink's manifest field.

## Consequences

- `plan/` stays engine-neutral. The plan-is-the-contract invariant is preserved.
- A caller that constructs the persistent sink without `WithVoice` gets an empty `manifest.Voice` field. Loud rather than silent: an empty voice id is observable to downstream consumers, who can flag the wiring bug.
- The duplication at the composition root is one line of code today (`genderToVoice[args.Gender]` referenced twice). Not a maintenance burden.
- A future intelligence adapter that picks the voice (rather than the composition root) would write the voice into `plan.PlanDefaults.Voice` (already engine-neutral hint) and the composition root would re-resolve to a concrete engine id before passing to `WithVoice`. The fallback chain documented in `render.RenderOptions` already allows this.

## Related decisions

- [Voice resolution order](../convention/2026-06-18-voice-resolution-order.md) — composition root drives voice selection from gender → engine id mapping.
- [Persistent-sink Sink.New takes outDir positional](../convention/2026-06-20-persistent-new-takes-outdir-positional.md) — companion constructor decision.

## Revisit trigger

If a second intelligence adapter (e.g. `intelligence/anthropic`, issue #15) starts picking voices from a richer LLM-driven hint, lift the voice-resolution logic to a shared helper so the composition root stays one-line. Otherwise the WithVoice Option scales fine.
