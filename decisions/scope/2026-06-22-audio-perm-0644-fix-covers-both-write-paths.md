# 0600->0644 audio-permission bugfix covers BOTH audio-write paths (writeAudio + stageAudioTmp)

| Field    | Value            |
|----------|------------------|
| Date     | 2026-06-22       |
| Status   | accepted         |
| Category | scope            |
| Tags     | persistent-sink, audio.wav, file-permissions, 0644, writeAudio, stageAudioTmp, consume, block-patch, tempfile-chmod, drift, issue-70 |

## Context

The #70 brief cited a single 0600-vs-0644 audio-permission bug in `writeAudio` (persistent.go 217-240) — `audio.wav` was being written at 0600 while its JSON siblings (`plan.json`, `manifest.json`) get 0644 via `atomicWriteFile`. The pre-flight drift report found the identical 0600 bug in a second audio writer, `stageAudioTmp` (patch.go 561-577), used on the block-patch path. The `tempFile` interface already exposes `Chmod(mode os.FileMode) error`, so the fix seam exists in both places.

## Decision

Scope the fix to BOTH audio-write paths, not just the one cited:

- `writeAudio` (persistent.go) — initial `Consume`: add `tmp.Chmod(0o644)` before `tmp.Close()`, error-wrapped, mirroring manifest.go `atomicWriteFile`.
- `stageAudioTmp` (patch.go) — block-patch: same `tmp.Chmod(0o644)` before `tmp.Close()`, matching its JSON sibling `stageTmp`.

Fixing only `writeAudio` would leave block-patched audio outputs at 0600 — the patch path would silently regress the very permission the fix is meant to guarantee. Both audio writers now match the 0644 that `plan.json`/`manifest.json` already get.

This is normally a routine bugfix (not journal-worthy on its own), but the *scope* call — that the fix must span two write paths discovered via drift, not the single one the brief named — is the load-bearing decision: it changes which files the change touches and is the kind of thing a reviewer would ask "why did you also touch patch.go?" about.

## Consequences

- Both `Consume` and `PatchBlock` produce `audio.wav` at 0644.
- Dedicated per-path permission tests: assert `audio.wav` mode is 0644 after `Consume`, and after `PatchBlock`.

## Revisit trigger

If a third audio-write path is added (would need the same Chmod), or if a stricter-permission policy is ever wanted for on-disk audio.
