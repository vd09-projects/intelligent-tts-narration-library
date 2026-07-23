# Voice-selection namespace: one unified named roster is the primary selector; --gender demoted to a deprecated alias

| Field    | Value            |
|----------|------------------|
| Date     | 2026-07-23       |
| Status   | accepted         |
| Category | architecture     |
| Tags     | rvc, voice-conversion, voice-roster, voice-selection, cli-namespace, voice-flag, gender-flag, deprecation, backwards-compat, buildrenderer, requires-worker, err-worker-missing, err-unknown-voice, manifest-provenance, kokoro, issue-156, issue-146 |

## Context

After #146 landed the RVC render decorator and the shared `pipeline.BuildRenderer` factory, the user-facing voice knob was split across two flags with tangled semantics:

- `--gender` (female | male) was the *primary* selector, mapping to a Kokoro voice.
- `--voice` existed only to select an RVC voice, and `--voice != ""` was treated throughout the roots as a de-facto synonym for "this render is RVC / 40 kHz."

That coupling was true only by coincidence of the phase-one roster (the only non-empty `--voice` values happened to be RVC). It leaked an engine mechanic (`voice!="" ⇒ RVC ⇒ 40 kHz`) into the CLI surface, and left two knobs disagreeing about primacy: `--gender` claimed to be the main selector while `--voice`, when set, silently overrode it. Adding a second Kokoro-only named voice, or an RVC voice reachable by gender, would have broken the `voice!="" ⇒ RVC` assumption outright.

Issue #156 asked whether to keep the split or unify the namespace into a single selector over one roster spanning both engines.

## Options considered

### Option A: keep the gender+voice split
- **Pros**: no migration; existing `--gender` scripts unchanged; smallest diff.
- **Cons**: perpetuates the `voice!="" ⇒ RVC / 40 kHz` coupling (an engine mechanic leaking into the CLI, false the moment a Kokoro-only named voice is added); two knobs that disagree on primacy; no single honest place to surface per-voice cost (engine / sample rate / needs-worker). Rejected.

### Option B: one unified named-voice roster, `--voice` primary, `--gender` a deprecated alias — CHOSEN
- **Pros**: a single honest selector over one flat roster; each entry carries `{engine, format, requires_worker}` metadata that `pipeline.BuildRenderer` reads as the single source of truth for engine + format, so the CLI no longer infers the engine from a string being non-empty; help text can surface per-voice cost (`Kokoro · 24kHz · fast` vs `RVC · 40kHz · needs worker`); `--gender` keeps working as a thin alias so existing scripts do not break.
- **Cons**: introduces a deprecation surface (`--gender` alias + notices) that must be carried until removal; one more resolver (`pipeline.ResolveVoice` / `SlugForGender`) to keep aligned with the roster. Accepted — the coupling removal and single-selector honesty outweigh the alias-carrying cost.

## Decision

**`--voice` becomes the single primary selector over ONE flat named-voice roster spanning both engines**: `af-bella` / `am-michael` → Kokoro (24 kHz); `cool-jahns` / `confident-neal` → RVC (40 kHz). Each roster entry carries `{engine, format, requires_worker}` metadata, and `pipeline.BuildRenderer` reads that metadata as **the single source of truth for engine + format** — the CLI never again infers "this is RVC / 40 kHz" from `--voice` being non-empty.

**`--gender` is demoted to a DEPRECATED back-compat alias** (`female → af-bella`, `male → am-michael`) resolved via `pipeline.SlugForGender`. It keeps working and emits a deprecation notice steering callers to `--voice`. When both are set, `--voice` wins (with an "ignored because --voice is set" notice).

**Two error classes with distinct timings, never a silent fallback:**
- An **unknown voice** stops **pre-render** — roots validate membership eagerly via `pipeline.IsVoice`, with `pipeline.ErrUnknownVoice` as the `BuildRenderer` construction-time backstop.
- A **`requires_worker` voice whose worker is unavailable** stops **at render time** — the `render/rvc` decorator returns `ErrWorkerMissing` when it renders. `RequiresWorker` is metadata used only for help-text tagging and the `--listen` 40 kHz re-key; there is deliberately **no up-front worker-liveness probe**.

**Manifest provenance:** `manifest.voice` records the engine-native resolved id — `af_bella` / `am_michael` for Kokoro voices, `cool-jahns` / `confident-neal` for RVC voices — so the deprecated `--gender` alias path and the explicit `--voice` path record the **same** value for the same voice (extends D6 from the RVC-only rule to the whole roster).

## Consequences

- The `voice!="" ⇒ RVC` coupling is gone: engine + format come only from roster metadata, so a future Kokoro-only named voice or a gender-reachable RVC voice no longer breaks the CLI.
- A single selector surfaces per-voice cost honestly in help text; callers see the sample rate and worker requirement before rendering.
- `--gender` must be carried as a deprecated alias (notices pinned once in `pipeline`, emitted verbatim by every root) until an eventual removal — a small standing maintenance surface.
- The two-timing error contract is now load-bearing: adding a new engine or worker-backed voice must preserve pre-render membership validation and at-render-time worker-missing hard stops (no silent fallback, honesty rule).
- Server `/narrate` keeps its own precedence: an explicit launch `--voice` overrides the per-request `gender`, but a launch `--gender` does not (per-request gender still wins) — locked by a regression test.

## Related decisions

- [pipeline.BuildRenderer is the shared renderer-factory home; pipeline/ now imports the concrete engines](2026-07-23-pipeline-hosts-buildrenderer-factory.md) — Decisions D1 (BuildRenderer keeps the `(render.Renderer, plan.AudioFormat, error)` signature) and D2 (the factory lives in `pipeline/`); this roster is the single source of truth that feeds that factory's engine + format choice.
- [manifest.voice records the RVC character slug for RVC renders (Option A), not the hidden Kokoro source](../tradeoff/2026-07-23-rvc-manifest-voice-records-character-slug.md) — Decision D6, extended here from the RVC-only rule to the whole roster: Kokoro engine id for Kokoro voices, RVC slug for RVC voices, so the alias and explicit paths stamp the same value.
- [RVC decorator owns the target->{Kokoro source, index_rate, pitch} map; translation happens exactly once](2026-07-22-rvc-decorator-owns-voice-map-single-translation.md) — the roster passes the RVC slug straight through; the decorator, not the roster, owns target→Kokoro-source translation.

## Revisit trigger

Reconsider when `--gender` is actually removed (drop the alias + `SlugForGender` + notices), or if per-request per-voice selection (#155) lands and the launch-voice-vs-request-voice precedence needs to generalize beyond the current "explicit launch --voice wins" rule.
