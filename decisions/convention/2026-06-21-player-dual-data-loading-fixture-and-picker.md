# Player dual data loading: bundled fixture AND runtime directory picker

| Field    | Value            |
|----------|------------------|
| Date     | 2026-06-21       |
| Status   | accepted         |
| Category | convention       |
| Tags     | player, fixture, file-system-access, ux, issue-18 |

## Context

Issue #18 requires the reference player to: (a) load the demo doc output from `docs/samples/` and (b) load *a* directory containing `plan.json` + `manifest.json` + `audio.wav`. These look like two acceptance criteria but they are really two access patterns: the quickstart demo path and the bring-your-own-output path. The player must satisfy both, on a fresh `pnpm install && pnpm dev` checkout.

## Options considered

### Option A: Picker only
- **Pros**: Single code path; the player is never hard-coded to demo data.
- **Cons**: First-run experience is hostile — the developer has to generate output before they can see anything. Fails the "loads the demo doc output" AC literally.

### Option B: Bundled fixture only
- **Pros**: Zero-friction first-mount demo.
- **Cons**: Reads as if the player were a hard-coded slideshow. Fails the "loads a directory" AC literally — the developer can't load anything else.

### Option C: Both — bundled fixture for first mount, runtime picker to load any directory
- **Pros**: Each AC has a dedicated code path; both are cheap to implement; first run is friction-free; the player is provably generalizable.
- **Cons**: Two loader hooks (`useFixture` + `useDirectoryLoader`) instead of one; slightly more state plumbing in `App.tsx`.

## Decision

**Choose C.** `useFixture` fetches from `/fixtures/sample/` (Vite serves `public/` statically) on mount; `useDirectoryLoader` exposes `pickDirectory()` driven from the top bar. The picker uses `window.showDirectoryPicker` (File System Access API) where available, with a `<input type="file" webkitdirectory multiple>` fallback for Safari / Firefox. No Node or Go server in the loop — purely static.

## Consequences

- The committed fixture under `player/public/fixtures/sample/` is part of the deliverable, not test scaffolding.
- Both code paths must produce the same `{ plan, manifest, audioUrl, source }` shape so `App.tsx` can swap between them without branching.
- Browser support split is real: Chromium gets the directory picker; Safari/Firefox fall back to multi-select. Test both code paths.

## Related decisions

- [[2026-06-21-player-synthetic-hand-authored-fixture]] — what the bundled fixture contains.
- [[2026-06-21-player-source-pane-uses-sibling-source-md]] — what the loader looks for in the directory.
