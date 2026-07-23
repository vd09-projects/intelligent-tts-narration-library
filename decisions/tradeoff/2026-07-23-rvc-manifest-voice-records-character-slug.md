# manifest.voice records the RVC character slug for RVC renders (Option A), not the hidden Kokoro source

| Field    | Value            |
|----------|------------------|
| Date     | 2026-07-23       |
| Status   | accepted         |
| Category | tradeoff         |
| Tags     | rvc, voice-conversion, manifest, provenance, honesty-rule, withvoice, persistent-sink, checkstale, content-hash, cool-jahns, confident-neal, af-bella, am-michael, cli-narrate, narrate-server, issue-146, issue-145, issue-144 |

## Context

Decision **D6** of the #146 RVC CLI/MCP/server wiring plan (the load-bearing, review-round-1 addition — blocking feedback item F1, corroborated by the Domain Logic, API & Contract, and Naming reviewers).

`manifest.voice` in a persistent render directory records "what voice produced this audio?" for a downstream player or audit reader. Today it records the Kokoro **source** voice: all four production `WithVoice` sites pass `genderToVoice[args.Gender]` (e.g. `af_bella`) — `cmd/narrate/main.go:223` (full Consume), `cmd/narrate/main.go:497` (PatchBlock), `cmd/narrate-server/narrate.go:106` (Consume), `cmd/narrate-server/main.go:910` (patchBlock).

The `render/rvc` decorator (#145) repaints a Kokoro render into a 40 kHz RVC **character** voice — `cool-jahns` / `confident-neal` — and owns the target→Kokoro-source map (`cool-jahns`→`am_michael`, `confident-neal`→`af_bella`) as the single translation point. #146 is pure wiring: it exposes a user-facing `voice` knob and routes it into `rvc.Config.Voice`. The open question F1 raised: for a persistent **RVC** render, what does `manifest.voice` record — the character voice the user asked for, or its hidden Kokoro source?

## Options considered

### Option A: record the RVC character slug in manifest.voice — CHOSEN
- **Pros**: Honest provenance — the manifest reports the voice the user actually asked to hear (`cool-jahns` / `confident-neal`). A `manifest.json` reader sees the true character render, not a plain-Kokoro claim. Least-surprising contract; aligns with the project's honesty north star.
- **Cons**: `manifest.voice` gains a new value class (RVC target slugs alongside Kokoro voice ids). Additive only — no field removed/renamed/retyped, `schema_version` unchanged.

### Option B: keep the Kokoro source in manifest.voice, only reword the standing-order doc
- **Pros**: Zero wiring change; `manifest.voice` keeps a single value class (Kokoro ids).
- **Cons**: Stamps `af_bella` on a `cool-jahns` file — misreports the render. A manifest reader would believe the file is plain Kokoro. An honest-provenance regression for **zero benefit**, since Option A carries no caching/staleness cost. Rejected.

## Decision

For a **persistent RVC render**, pass the RVC target slug to `WithVoice` so `manifest.voice` reports the character voice the user asked to hear, never its hidden Kokoro source. Concretely, at each persistent-sink `WithVoice` site, select `persistent.WithVoice(args.Voice)` when `args.Voice != ""`, else keep the existing gender-derived Kokoro voice.

**Why Option A over Option B:** the project's north star is honest provenance. Stamping `af_bella` on a `cool-jahns` file misreports the render — a `manifest.json` reader would believe the file is plain Kokoro. Reporting the character slug is the honest, least-surprising contract.

**Why it is safe (the caching/staleness check F1 asked for):** `CheckStale` compares **only** `content_hash` (`check.go:63`: `m.ContentHash != p.Source.ContentHash`) and never reads `manifest.Voice`; intelligence caching keys on `(content hash, level, model)`. Changing what `manifest.voice` records touches neither staleness nor caching — so the honesty argument wins with no downside. `patch.go:290-291` already writes `patchedManifest.Voice = cfg.Voice` when set, so the patch path needs only the changed **argument**, not sink surgery.

**Scope:** persistent-sink paths only — CLI full render (`cmd/narrate/main.go:223`) + CLI PatchBlock (`main.go:497`); server Consume (`cmd/narrate-server/narrate.go:106`) + server patchBlock (`main.go:910`). The MCP paths are exempt: the ephemeral sink and `NewWAVFile` write no `manifest.json` (`WithVoice` is a documented no-op on `NewWAVFile`, `wavfile.go:59-61`), so there is no `manifest.voice` to misreport. The engine-neutral invariant holds — the slug is a *voice id string* in a persistent-sink receipt, not an engine id in `RenderResult.Format` and not in the plan schema.

## Consequences

- A persistent RVC render stamps `manifest.voice = <slug>` (`cool-jahns` / `confident-neal`); a plain-Kokoro render is unchanged (`manifest.voice` = gender-derived Kokoro id such as `af_bella`).
- Additive-compatible public surface: `manifest.voice` semantics change only for RVC renders (a new value class in an existing field). `schema_version` unchanged; no field removed/renamed/retyped.
- No caching/staleness cost: neither `CheckStale` (content-hash only) nor intelligence caching `(content hash, level, model)` reads `manifest.Voice`, so escalation does not re-bill and staleness is unaffected.
- The stale "Standing order" wording in the plan Constraints — which wrongly implied `WithVoice` alone carried the RVC voice while no phase set it — is corrected: two distinct things flow to the persistent sinks at construction/patch time, the 40 kHz **format** via `WithExpectedFormat` and the **manifest voice id** via `WithVoice`, the latter carrying the RVC slug under D6.
- Implemented at the four persistent-sink `WithVoice` call sites (CLI full + patch, server Consume + patch), locked by a new manifest-provenance test (Phase 2 harness plus per-root assertions in Phases 4 and 6) asserting `manifest.Voice == "cool-jahns"` for an RVC render, and an F5 negative guard that `voice == ""` still flows the gender-derived Kokoro id.

## Related decisions

- [Persistent sink voice id flows via WithVoice Option, not plan.Timeline](../architecture/2026-06-20-persistent-voice-id-via-withvoice-option.md) — establishes that `manifest.voice` is composition-root-supplied via `WithVoice`; D6 changes only what value is passed for an RVC render, not the sink mechanism.
- [RVC decorator owns the target->{Kokoro source, index_rate, pitch} map; translation happens exactly once](../architecture/2026-07-22-rvc-decorator-owns-voice-map-single-translation.md) — the decorator owns the target→Kokoro-source translation; D6's slug is the user-facing target, recorded verbatim without re-translation.
- [Torch-free ONNX RVC via an ephemeral per-job worker, wrapped as a render decorator](../architecture/2026-07-22-torch-free-onnx-rvc-ephemeral-worker.md) — the parent #143/#144/#145 decision (worker-unavailable = hard error, 40 kHz end-to-end when RVC on) that #146 wires into the three composition roots.
- [A per-block RVC worker ERR fails the whole Render in phase one](2026-07-22-rvc-per-block-err-fails-whole-render-hard-stop.md) — sibling honesty-rule tradeoff from the #145 set; both apply the project's honest-provenance north star to the RVC path.

## Revisit trigger

Reconsider if `manifest.voice` ever needs to carry *both* the character voice and its Kokoro source (e.g. a manifest schema that adds a separate `source_voice` field for full render provenance), or if a future caching/staleness key starts reading `manifest.Voice` (which would remove the "free" property this decision relies on).
