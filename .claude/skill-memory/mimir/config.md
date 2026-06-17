<!-- rune-generated: 2026-06-18 | git: unknown | rune: 1.0 -->

# Mimir — Project Config

Planning preferences for intelligent-tts-narration-library.

## default_depth

Ambiguous detection resolution.

```
default_depth: ask
```

## domain_expert_role

No `domain-expert`-role skill installed. Mimir routes architecture artifacts with `consumer_role: none`.

```
domain_expert_role: none
```

## always_overlays

Empty. Solo hobby project — no platform/data/team gates to force.

```
always_overlays: []
```

## never_overlays

Solo project + no public API yet + no streaming + no CI → silence overlays that would only add noise.

```
never_overlays:
  - cross-team
  - auth-authz
  - feature-flag
  - observability
  - i18n-l10n
  - data-migration
```

## Notes

- `public-api-change` left ON despite phase-one being pre-release: the narration-plan JSON schema is the load-bearing artifact and the React player + MCP clients are already-real consumers. Schema bumps need the overlay's discipline from day one.
- `infra-blast` left ON because shelling out to TTS / running an MCP server / writing audio files all carry local-machine blast radius worth thinking about.
- `concurrency` left ON for when block-parallel rendering (A17) lands.
- `perf-critical` left ON because narration latency is the felt UX.

confidence: HIGH
