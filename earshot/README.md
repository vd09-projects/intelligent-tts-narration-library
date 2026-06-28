# Earshot

Server-driven React listener UI for the intelligent TTS narration library
(issue #111). Replaces the deleted passive `player/` (#107). Earshot talks to the
local `narrate-server` HTTP bridge (#109) and renders the #108 Material
list-detail design (mockup signed off in #115).

This directory is the **shell**: scaffold + tooling, the three-region
list-detail layout, a session pane (session ID → message list → click to hear),
a file pane (drop/pick → read out), the center transcript pane with a
Spoken/Source toggle, and audio wiring off the server-supplied `audio_url`.

Deeper transport-deck interactions, per-block escalation, and resume-across-reload
are explicit follow-on tickets — not built here.

## Run it (from the repo root)

Drive everything through the repo `Makefile` (project convention):

| Target | What it does |
|---|---|
| `make earshot-dev` | Vite dev server on :5173 with a proxy to `narrate-server`. |
| `make earshot-build` | Type-check + production bundle (**compile/smoke check only** — see below). |
| `make earshot-test` | Vitest unit + component + a11y suite (no audio). |
| `make earshot-lint` | ESLint (flat config). |

First run installs dependencies under `earshot/node_modules` (gitignored).

### `make earshot-dev` needs a running server

The dev server proxies `/sessions`, `/narrate`, and `/audio` to the
`narrate-server` (default `http://127.0.0.1:8080`, override with
`VITE_NARRATE_SERVER`). Start the server first:

```
make run-server          # in another terminal
make earshot-dev
```

### `make earshot-build` is compile-smoke ONLY

`earshot-build` proves the app type-checks and bundles. It is **not** a runnable
deployment. The Vite dev proxy exists only under `earshot-dev`; a bundle served
standalone has no `/sessions` / `/narrate` / `/audio` proxy and cannot reach the
server. Do not chase a phantom CORS/404 bug against the built bundle.

## Architecture

- **One state owner.** `useNarrationSession` (mounted once via `NarrationProvider`)
  owns the current transcript, the opaque `activeAudioUrl`, and playback. Both the
  session pane and the file pane dispatch into it; neither keeps its own copy.
  The active block is **derived** from playback position vs the timeline, never
  stored.
- **All server-shape knowledge lives in `src/api/`.** `audio_url` is treated as an
  opaque URL (used verbatim as the `<audio>` source — never parsed). The assumed
  `message_ref` shape is the single mock↔live divergence point, pinned in
  `src/api/types.ts` with a comment.
- **Fixtures are the contract-of-record.** `src/fixtures/*.json` are derived from
  the live `narrate-server` shapes. The test mock (`src/mocks/server.ts`, a fetch
  shim) serves the same files the component tests assert against — one source, two
  readers.
- **Honesty rule on screen.** Refused blocks render their refusal text + source
  map and expose no level control; degraded blocks carry a non-color marker and
  are never silently shown as fully voiced.

## Layout

```
src/
  api/        types.ts (contract, D4), client.ts (typed fetch wrappers)
  state/      NarrationContext, Announcer, pure helpers (activeBlock, blockText)
  hooks/      useNarrationSession (owner), useSessionMessages, useAudio
  components/ AppHeader, SessionPane, SessionRow, StatusChip, FilePane,
              TranscriptPane, BlockRow, RefusalBlock, SegmentedToggle,
              TransportBar, LiveRegion, ErrorBanner
  fixtures/   committed contract-of-record JSON
  mocks/      test-only fetch shim
```

## Out of scope (this ticket)

Full transport deck (prev/play-pause/next, block scrubber), per-block escalation
(L1/L2/L3 + `POST /narrate/block`), resume-across-reload (`localStorage`),
save-to-disk, mobile top-sheet focus choreography (the mobile session pane is a
CSS-only collapse with no JS focus trap).
