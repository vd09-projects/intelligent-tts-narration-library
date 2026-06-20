package persistent

import "errors"

// Patch refusal sentinels (issue #28). All five are REFUSALS in the honesty-
// rule sense: the patch did not corrupt anything, it declined to act because
// acting would have been unsafe. The composition root (cmd/narrate) maps each
// to exit code 2 (caller-correctable input) via errors.Is, distinct from a
// hard pipeline/system error (exit 1) such as a corrupt manifest or a
// structurally unreadable WAV.
//
//   - ErrNothingToPatch — outDir is missing one of manifest.json / audio.wav /
//     plan.json. There is nothing to patch; fabricating an outDir would
//     violate the honesty rule.
//   - ErrStalePatch — CheckStale reports the outDir has drifted from its
//     source. Patching would splice stale neighbor audio.
//   - ErrContentHashMismatch — the on-disk manifest indexes a DIFFERENT
//     document than the re-render. Patching a different document is corruption.
//     This gate makes --expected-content-hash OPTIONAL: cross-document patches
//     are refused even without the flag (Open Question #3 — resolved optional).
//   - ErrUnknownBlock — the requested block id is not in the manifest index.
//   - ErrContainerMismatch — the on-disk audio.wav data-chunk length disagrees
//     with the manifest-derived length (F1). The container does not match its
//     index; do not slice.
var (
	ErrNothingToPatch      = errors.New("sink/persistent: nothing to patch (outDir missing manifest.json, audio.wav, or plan.json)")
	ErrStalePatch          = errors.New("sink/persistent: refusing to patch a stale outDir")
	ErrContentHashMismatch = errors.New("sink/persistent: refusing to patch — manifest content_hash differs from the re-rendered document")
	ErrUnknownBlock        = errors.New("sink/persistent: block id not found in manifest")
	ErrContainerMismatch   = errors.New("sink/persistent: audio.wav does not match its manifest (container/manifest length mismatch)")
)

// ErrFormatMismatch is the exported sentinel a *formatMismatchError matches via
// errors.Is. It lets the composition root (cmd/narrate) classify a format-
// mismatch refusal — the re-rendered block's audio format differs from the
// container's — to exit code 2 without depending on the unexported
// formatMismatchError type. Format mismatch is a REFUSAL (caller-correctable),
// not a hard error: PatchBlock refuses BEFORE any tmp write, leaving the
// existing outputs untouched (planner-task.md S3 / test j).
var ErrFormatMismatch = errors.New("sink/persistent: wav format mismatch")
