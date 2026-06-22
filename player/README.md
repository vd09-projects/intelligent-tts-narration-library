# player — React reference UI

Phase 5 deliverable for the intelligent TTS narration library. A Vite + React
+ TypeScript single-page app that consumes a `sink/persistent` output
directory (`audio.wav` + `plan.json` + `manifest.json`) and demonstrates:

- **Block-level sync.** The currently-playing block is highlighted in the
  list; click any block in the list (or in the source pane) to seek to it.
- **Honest refusal.** Refused blocks render with a red REFUSED badge, the
  machine reason (e.g. `bare_image_no_description`), and the spoken notice
  the listener heard. Refusal is data, not error.
- **Per-block escalate.** Each block exposes an "Escalate L3" action that
  expands an inline card containing the literal CLI command to re-narrate
  that one block at L3 detail. Phase one displays the command; in-app
  re-render is out of scope.
- **Stale badge.** When `manifest.stale === true`, the top bar shows an
  amber STALE pill with the reason tooltip — never auto-regenerates.

## Quickstart

```
pnpm install
pnpm dev
```

`http://localhost:5173` opens the player against the bundled fixture under
`public/fixtures/sample/`. Click **Load directory…** to point the player
at a real `sink/persistent` output directory.

If your browser does not ship the File System Access API (Safari, Firefox)
use the "or pick folder" file-input fallback next to it; both go through
the same loader.

## Make targets (from the repo root)

```
make player-dev               # cd player && pnpm install && pnpm dev
make player-build             # cd player && pnpm install && pnpm build
make player-test              # cd player && pnpm install && pnpm test
make player-fixture-silent    # regenerate the silent fixture WAV
make player-fixture-kokoro    # narrate docs/samples/sample.md → public/fixtures/sample/
```

## Layout

```
player/
├── README.md
├── package.json, pnpm-lock.yaml, tsconfig.json, tsconfig.app.json,
│   tsconfig.node.json, vite.config.ts, index.html, .gitignore
├── public/fixtures/
│   ├── make_silent_wav.py         # helper, used by make player-fixture-silent
│   └── sample/                    # bundled fixture (committed)
│       ├── plan.json, manifest.json, audio.wav, source.md, README.md
├── src/
│   ├── main.tsx, App.tsx, styles.css, vite-env.d.ts
│   ├── types/        plan.ts (mirrors Go plan/), manifest.ts, audioFormat.ts
│   ├── lib/          findActiveBlock.ts, escalateCommand.ts, loadDirectory.ts,
│   │                 refetchBase.ts (#62 server-mode re-fetch URL resolver)
│   ├── hooks/        useFixture.ts, useDirectoryLoader.ts, usePlayback.ts
│   └── components/   TopBar.tsx, BlockList.tsx, BlockRow.tsx,
│                     SourcePane.tsx, RefusalBadge.tsx, StaleBadge.tsx,
│                     EscalateCard.tsx
└── test/             findActiveBlock.test.ts, escalateCommand.test.ts,
                      App.smoke.test.tsx, setup.ts
```

## Architecture

- One-way data flow. `useFixture` (mount) or `useDirectoryLoader` (after
  user picks a directory) populates `LoadedDirectory` = `{ plan, manifest,
  audioUrl, source, warnings }`. `App` threads it into `BlockList`,
  `SourcePane`, and the bottom `<audio>` element.
- Playback sync via `usePlayback`. A `requestAnimationFrame` loop reads
  `audio.currentTime` and calls `findActiveBlock(manifest.blocks, tMs)`.
  React state is written **only** when the active id changes — the rAF
  tick itself does not trigger re-renders.
- Block-level only. The plan + manifest carry no per-word / per-segment
  timings, and the player intentionally does not invent them.
- `plan.ts` and `manifest.ts` are hand-authored TypeScript mirrors of the
  Go schema in `plan/` and `sink/persistent/manifest.go`. Keep them in
  sync when the Go schema grows (mismatch surfaces as a warning, not a
  crash — additive-compatible).
- **Server-mode re-fetch resolution (#62).** After an in-place escalate the
  player must re-read the patched `audio.wav` + `manifest.json` so downstream
  offsets + audio reflect the just-patched dir. In **server mode** that dir is
  an arbitrary path the user typed, so the re-fetch resolves against the
  escalate server's `GET /artifact?dir=&name=` route (`src/lib/refetchBase.ts`),
  not the bundled fixture origin. In **fixture mode** it stays on
  `FIXTURE_BASE`. The resolver is a pure function of `(serverMode, dir,
  serverBaseUrl, fixtureBase)`; `App` reads `serverMode` + `dir` from live refs
  at call time so a dir typed after the initial render is honored (no stale
  closure). Trade-off: a failed re-fetch keeps the just-committed patch and
  flags `staleDownstream` rather than rolling back — the audio/offsets may lag
  one patch behind, but the patch is never lost.

## Acceptance criteria coverage (issue #18)

| AC | Where to look |
|---|---|
| Vite + React + TS scaffold | `package.json`, `vite.config.ts`, `tsconfig.app.json` |
| Loads `plan.json` + `manifest.json` + `audio.wav` from a directory | `src/lib/loadDirectory.ts`, `src/hooks/useFixture.ts`, `src/hooks/useDirectoryLoader.ts` |
| Block-level scrubber + click-to-seek | `src/hooks/usePlayback.ts`, `src/components/BlockRow.tsx` (`onSeek`) |
| Source pane with cursor-tracked highlight | `src/components/SourcePane.tsx` |
| Per-block escalate UI (literal CLI command) | `src/components/EscalateCard.tsx`, `src/lib/escalateCommand.ts` |
| Refused blocks display message + badge | `src/components/RefusalBadge.tsx`, `src/components/BlockRow.tsx` (refused branch) |
| `pnpm install && pnpm dev` README | this file |
| End-to-end demo doc | `public/fixtures/sample/` (mirrors `docs/samples/sample.md`) |

## Tests

```
pnpm test          # one-shot
pnpm test:watch    # vitest watch
```

Three smoke files under `test/`:

- `findActiveBlock.test.ts` — table-driven boundary checks for the binary
  search (empty list, pre-roll, intra-block, inter-block gap, post-roll).
- `escalateCommand.test.ts` — file / mcp_text / raw_text / unknown source
  kinds.
- `App.smoke.test.tsx` — mounts the app with the fixture mocked into
  `fetch`, asserts each of the 8 ACs from issue #18 has a DOM checkpoint,
  and asserts that the tab order reaches TopBar → BlockList → audio.

## Out of scope (phase one)

- In-app re-render after Escalate. The literal command is the contract;
  the user runs it on their host and reloads.
- Word-level highlight. Sync is block-level only (load-bearing invariant
  in `CLAUDE.md` — word-level contradicts gist mode where spoken text
  differs from source text).
- Multi-document selection, persistent state (localStorage / IndexedDB),
  full WCAG audit, source-pane scroll-sync, deploy.
