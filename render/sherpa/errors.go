// Package sherpa errors — sentinel errors returned by the subprocess Kokoro
// backend. Errors stop the pipeline (per CLAUDE.md honesty rule); refusals
// do not — they are data carried through as audio.
package sherpa

import "errors"

// ErrMissingBinary — the wrapper script (or model files behind it) is not
// on disk. Maps to wrapper exit codes 127 (no script) and 3 (model files
// missing).
var ErrMissingBinary = errors.New("sherpa: kokoro wrapper binary or model files missing")

// ErrUnsupportedVoice — caller asked for a voice the wrapper does not
// recognize. Maps to wrapper exit code 2 with "unsupported voice" stderr.
// Caller error, not a refusal.
var ErrUnsupportedVoice = errors.New("sherpa: unsupported voice")

// ErrTimeout — per-block or wall-clock timeout exceeded. Wraps the original
// context.DeadlineExceeded so callers can errors.Is(err, context.DeadlineExceeded).
var ErrTimeout = errors.New("sherpa: subprocess timed out")

// ErrSubprocess — the wrapper exited non-zero for an unhandled reason
// (e.g. sample-rate breach, runtime crash). Wrapped error carries the
// captured stderr for diagnosis.
var ErrSubprocess = errors.New("sherpa: subprocess failed")

// ErrMalformedPlan — the plan handed in is internally inconsistent in a
// way the renderer cannot proceed past (e.g. refused block with empty
// Refusal.Message, RenderBlock asked for an id not in the plan). This is
// an upstream bug — not a refusal.
var ErrMalformedPlan = errors.New("sherpa: malformed plan")
