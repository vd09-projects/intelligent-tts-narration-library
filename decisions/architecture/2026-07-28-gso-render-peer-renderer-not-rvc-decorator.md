# render/gptsovits is a peer render.Renderer (sherpa-shaped), deliberately NOT an rvc-style decorator

| Field    | Value            |
|----------|------------------|
| Date     | 2026-07-28       |
| Status   | accepted         |
| Category | architecture     |
| Tags     | gso, gpt-sovits, render-renderer, peer-engine, not-decorator, render-sherpa, render-rvc, text-to-audio, warm-subprocess-worker, block-loop, timeline, manifest, spoken-text-for, content-addressed, buffer-before-next-request, s4, issue-162, issue-161 |

## Context

Issue #162 builds `render/gptsovits`, the Go engine for GPT-SoVITS. Two prior engines set competing structural precedents this build had to reconcile:

- `render/sherpa` (Kokoro) is a **source engine**: it owns the block loop, derives the literal spoken words from each `plan.Block` via `spokenTextFor` (refused blocks speak `Refusal.Message`), writes one WAV per block keyed by block id, and builds `Timeline` + `manifest` from scratch. It wraps nothing.
- `render/rvc` is a **decorator** (`2026-07-22-torch-free-onnx-rvc-ephemeral-worker`): it takes an inner `render.Renderer`, renders the whole plan through Kokoro into a private intermediate dir, then repaints each already-existing WAV (audio→audio). It never sees plan text.

The standing RVC precedent — reinforced across the #143–#147 decisions — said "wrap the worker as a decorator over the base engine." Applying that precedent literally to GSO would make GSO a decorator over Kokoro. The plan's "Peer-vs-decorator reconciliation (S4)" section had to resolve whether GSO follows the RVC decorator shape or the sherpa peer shape.

## Options considered

### Option A: rvc-style decorator over a base Kokoro engine
- **Pros**: reuses the most recent, freshly-proven engine precedent; symmetric with RVC in `pipeline/build_renderer.go`.
- **Cons**: structurally wrong. GPT-SoVITS is **text→audio** (like Kokoro), not **audio→audio** (like RVC). It synthesizes from the block's text plus ref-audio/prompt-text conditioning — there is no base-engine audio to repaint. Wrapping Kokoro would mean synthesizing every block **twice** (once by Kokoro, then thrown away, then again by GSO) — nonsensical work, and the decorator would have to reach past its inner renderer to get the plan text it needs, which the decorator contract deliberately hides.

### Option B: peer render.Renderer modeled on render/sherpa
- **Pros**: matches what a text→audio source engine actually does — own the block loop, read plan text directly, emit `Timeline`/`manifest` from scratch. No double synthesis. Keeps the roster as the single validator (voice-neutral, sherpa-style).
- **Cons**: sherpa spawns a subprocess **per block**; for GSO's ~17–22 s cold model load that would repay the cold load on every block. So the sherpa shape needs one graft: keep sherpa's loop structure but swap per-block spawn for one warm worker per document (AC2), borrowing RVC's warm-subprocess-worker transport mechanics.

## Decision

**`render/gptsovits` implements `render.Renderer` directly as a PEER engine, modeled structurally on `render/sherpa`, and is deliberately NOT an rvc-style decorator.** `pipeline/build_renderer.go` returns a bare `gptsovits.New(...)` with no inner renderer to wrap (verified in the build review — the `EngineGSO` arm has no decorator wrap, unlike the RVC arm). The compile-time `var _ render.Renderer = (*Engine)(nil)` binds it as a peer.

The decisive reason: **GPT-SoVITS is text→audio, not audio→audio.** RVC's decorator shape exists because RVC repaints existing audio; a text-to-audio engine has no base audio to wrap, so the decorator precedent is structurally inapplicable. This resolves the standing-order tension where the RVC precedent said "wrap the worker as a decorator over the base engine" — that order is scoped to audio→audio conversion, not to peer TTS engines.

The synthesis that landed is a **sherpa-shaped peer renderer driven by an rvc-shaped warm subprocess worker**:
- From sherpa: the block loop, `spokenTextFor` (refused blocks voice `Refusal.Message`), empty-text handling (zero-duration, empty `AudioRef`, no exchange, block still present in `Timeline.Blocks`), `cursorMs` accumulation, and `Timeline`/`manifest` emission from scratch. Voice-neutral: the voice is resolved from `RenderOptions.Voice`, keeping the roster the single validator.
- From rvc: the warm subprocess worker — one warm process per document (AC2, so the ~17–22 s cold load is paid once, not per block), goroutine lockstep request/response exchange, exit-code taxonomy classification, and a clean stdin-close reap. `RenderBlock` is a single cold-load exchange (start → one exchange → close), exactly mirroring RVC's `RenderBlock`.
- Three GSO-specific wire deltas from RVC's transport (per the `2026-07-27-gso-worker-wire-contract-rvc-shaped-not-verbatim` contract): no `<out>` token (the worker mints a content-addressed path and echoes it), the `OK` payload is the literal `line[3:]` (never shlex-split, so a minted path containing a space survives), and D4 content-addressed **buffer-before-next-request** discipline (align each OK's WAV immediately, before the next exchange, because identical blocks mint the same file).

## Consequences

- Only the two S1 pipeline seams (`pipeline/voices.go`, `pipeline/build_renderer.go`) gain concrete-engine knowledge; the peer boundary keeps `planner/`/`plan/` byte-untouched and confines engine awareness exactly as CLAUDE.md's composition-root invariant requires.
- The one place sherpa's shape was insufficient (per-block subprocess spawn) is documented and grafted, not hidden — future readers comparing GSO to sherpa will find the warm-worker swap intentional.
- `writeManifest` becomes a third byte-for-byte unexported copy (sherpa + rvc + gptsovits), self-marked `// DUP:` for extraction to `render/internal` only when a fourth producer appears (accepted debt, no ticket — matches the low-pri-inline convention).
- Reviewers diff the renderer body against `render/sherpa/sherpa.go` and the worker transport against `render/rvc/worker.go`; the two-parent graft is the intended review lens.
- A future engineer must not "unify" GSO into the RVC decorator arm to reduce the two engines to one shape — the text→audio vs audio→audio distinction is the load-bearing reason they differ.

## Related decisions

- [Torch-free ONNX RVC via an ephemeral per-job worker, wrapped as a render decorator](2026-07-22-torch-free-onnx-rvc-ephemeral-worker.md) — the decorator precedent this decision deliberately departs from (RVC is audio→audio, so its decorator shape does not apply to a text→audio peer).
- [GSO worker wire contract is RVC-shaped but NOT verbatim](2026-07-27-gso-worker-wire-contract-rvc-shaped-not-verbatim.md) — the frozen #161 transport contract this peer engine consumes, including the no-`<out>` / literal-OK / content-addressed deltas.
- [pipeline.BuildRenderer is the shared renderer-factory home](2026-07-23-pipeline-hosts-buildrenderer-factory.md) — the factory whose `EngineGSO` arm returns the bare peer engine with no wrap.
- [GSO warmproof warm-reuse correctness oracle](../convention/2026-07-27-gso-warmproof-warm-reuse-correctness-oracle.md) — the warm-per-document reuse this peer's block loop depends on.

## Revisit trigger

If a future GSO integration ever needs to post-process another engine's already-rendered audio (i.e. becomes genuinely audio→audio for some mode), re-evaluate whether that mode warrants a decorator alongside the peer engine.
