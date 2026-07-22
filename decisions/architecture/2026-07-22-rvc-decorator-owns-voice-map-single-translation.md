# RVC decorator owns the target->{Kokoro source, index_rate, pitch} map; translation happens exactly once

| Field    | Value            |
|----------|------------------|
| Date     | 2026-07-22       |
| Status   | accepted         |
| Category | architecture     |
| Tags     | rvc, voice-conversion, render-decorator, voice-map, single-source-of-truth, kokoro-source-voice, index-rate, pitch, resolveVoice, sherpa, render-options, phase-3-thin, open-question-2, issue-145, issue-146 |

## Context

The `render/rvc` decorator (#145) wraps the sherpa (Kokoro) renderer: Kokoro writes the words, RVC repaints the timbre. The user-facing knob is an **RVC target voice** (`cool-jahns`, `confident-neal`). But three lower-level facts are derived from that one target and are needed to actually run a conversion:

- (a) the internal **Kokoro SOURCE voice** the RVC model was trained against (`am_michael` for `cool-jahns`, `af_bella` for `confident-neal`) — this is what the inner sherpa renderer must actually synthesize;
- (b) the per-voice **`index_rate`** (`0.75` for `cool-jahns`, `0.5` for `confident-neal`) — the faiss index-blend strength that is authoritative on the worker request line;
- (c) **`pitch`**, which is always `0` in phase one (non-zero pitch is refused by the worker).

The open question (OQ#2 in the #145 plan) was *where* the target→source/index_rate/pitch translation happens. If it happens in the wrong layer — or in two layers — either sherpa receives a non-Kokoro voice string and hard-errors, or the source and target drift apart. Phase 3 (#146) still has to wire the RVC target through CLI `--voice`, the MCP `voice` arg, and the narrate-server bridge, so the boundary had to be pinned before #146 could bind.

## Options considered

### Option A: neither layer translates — pass the target straight through to sherpa
- **Pros**: no map to maintain.
- **Cons**: sherpa's `resolveVoice` receives `"cool-jahns"`, which is not a Kokoro voice, and hard-errors. The inner renderer only ever knows `{af_bella, am_michael}`; a target string is meaningless to it. Rejected.

### Option B: both the decorator and Phase 3 (#146) translate
- **Pros**: each layer is "self-sufficient".
- **Cons**: double translation. The two mappings can drift; a source picked in #146 and a source picked in the decorator can disagree, producing a Kokoro voice the RVC model was not trained against (silent timbre corruption). Two owners of the same truth is exactly the drift this decision exists to prevent. Rejected.

### Option C: the decorator is the SINGLE translation point; Phase 3 passes the target through untouched — CHOSEN
- **Pros**: sherpa only ever sees a valid Kokoro source `{af_bella, am_michael}`; source and target are structurally impossible to drift because they are read from one map in one file; Phase 3 stays thin (pure pass-through, no RVC knowledge). One source of truth for the voice map.
- **Cons**: the decorator is the mandatory chokepoint for any new RVC voice (a new voice = one map entry) — accepted, since that is where the knowledge belongs.

## Decision

**The `render/rvc` decorator is the single place the RVC target is translated.** It owns the `target → {Kokoro source, index_rate, pitch}` map, which lives in **one file, `render/rvc/voice.go`**. Concretely, the decorator:

- looks up the target in the map, and **overrides the inner renderer's `RenderOptions.Voice`** to the mapped **Kokoro source** before calling `inner.Render` / `inner.RenderBlock` — so sherpa's `resolveVoice` only ever sees `{af_bella, am_michael}`;
- emits `{target, index_rate, pitch}` on the worker request line (the worker repaints Kokoro's per-block WAV into the target timbre).

**Phase 3 (#146 — CLI / MCP / narrate-server wiring) performs NO translation of its own.** It passes the RVC target slug straight into `rvc.Config.Voice`. All three surfaces (CLI `--voice`, MCP `voice` arg, server bridge) hand the raw target slug to the decorator and stay ignorant of Kokoro sources, index_rate, and pitch.

This resolves Open Question 2 in the #145 plan, promoted from an open question to a locked decision after round-1 plan review.

## Consequences

- sherpa never sees an RVC target string — it is impossible for `resolveVoice` to be handed `cool-jahns`.
- Source + target can never drift: they are the two halves of one map entry in one file.
- Phase 3 stays thin — no RVC-specific branching in CLI/MCP/server; adding those surfaces does not re-derive any RVC fact.
- Adding a new RVC voice is a single-file change (one entry in `render/rvc/voice.go`); the `index_rate`-authoritative and `pitch=0` rules the worker enforces are satisfied by construction because the map is the only producer of those tokens.
- A future engineer must not "helpfully" add a second translation in #146 — that would reintroduce the drift this decision forecloses.

## Related decisions

- [Torch-free ONNX RVC via an ephemeral per-job worker, wrapped as a render decorator](2026-07-22-torch-free-onnx-rvc-ephemeral-worker.md) — the parent architecture decision; this decision fixes *where* the target→source translation lives inside that decorator.
- [RVC worker stdin/stdout wire contract — closed ERR taxonomy + startup/runtime FATAL exit-code split](2026-07-22-rvc-worker-wire-contract-err-taxonomy-exit-codes.md) — the `{target, index_rate, pitch}` tokens this map emits are three of that contract's five positional request tokens.
- [RVC phase-one rejects non-zero pitch; line index_rate is authoritative](../tradeoff/2026-07-22-rvc-reject-nonzero-pitch-index-rate-authoritative.md) — the map bakes in `pitch = 0` and the per-voice `index_rate` precisely because the worker treats the line's `index_rate` as authoritative and refuses non-zero pitch.

## Revisit trigger

Reconsider if a per-voice default `index_rate` is ever wanted server-side (would move part of the map off the decorator), if a voice needs a non-zero pitch (adds a fourth mapped field once the worker grows a transpose path), or if a second consumer beyond the decorator legitimately needs the target→source map (extract it to a shared package rather than duplicating the translation).
