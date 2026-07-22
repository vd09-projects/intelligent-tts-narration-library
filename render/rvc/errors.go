// Package rvc errors — sentinel errors returned by the RVC render decorator.
//
// Honesty rule (CLAUDE.md, non-negotiable): every failure here is a returned
// error that STOPS the pipeline. A missing or broken worker is NEVER silently
// degraded back to plain Kokoro — the caller sees the error (with a fix hint)
// and decides. Refusals are still data (they carry Kokoro audio of the refusal
// message and are repainted like any other block); only worker/IO failures are
// errors.
package rvc

import (
	"errors"
	"os/exec"
	"strings"
)

// ErrUnsupportedVoice — Config.Voice is not a known RVC target slug. Surfaced by
// New at construction (a config bug), distinct from a worker `bad-voice` ERR.
var ErrUnsupportedVoice = errors.New("rvc: unsupported voice")

// ErrWorkerMissing — the scripts/rvc wrapper is not on disk (exec ENOENT) OR the
// wrapper exited 2 because the torch-free venv is missing. Fix: `make rvc-worker-venv`.
var ErrWorkerMissing = errors.New("rvc: worker wrapper or torch-free venv missing (run `make rvc-worker-venv`)")

// ErrWorkerStartup — the worker exited 78 (EXIT_FATAL_STARTUP): missing exported
// artifacts, torch present in the venv, or a bad venv. Fix: `make rvc-export`
// (and/or `make rvc-worker-venv`). Carries the captured stderr FATAL line.
var ErrWorkerStartup = errors.New("rvc: worker startup fatal (run `make rvc-export`)")

// ErrWorkerRuntime — the worker exited 70 (EXIT_FATAL_RUNTIME, e.g. MemoryError)
// or died by signal for a reason NOT attributable to caller cancellation.
var ErrWorkerRuntime = errors.New("rvc: worker runtime fatal")

// ErrConvertFailed — a per-block `ERR <category> <message>` from the closed set
// {bad-args|bad-voice|read-failed|infer-failed|write-failed}. Phase-one policy:
// ANY ERR is a HARD error that stops Render — a 24 kHz block cannot be mixed into
// a 40 kHz timeline, and fabricating a repaint would violate the honesty rule.
var ErrConvertFailed = errors.New("rvc: worker reported a conversion error")

// ErrWorkerProtocol — the worker violated the frozen v1 wire contract:
// response-count ≠ request-count, an `OK` path that does not echo the requested
// out path, an unparseable response line, or an unknown ERR category
// (forward-compat = fatal).
var ErrWorkerProtocol = errors.New("rvc: worker protocol violation")

// ErrTimeout — a per-block or wall-clock deadline was exceeded. Wraps
// context.DeadlineExceeded so callers can errors.Is down to it.
var ErrTimeout = errors.New("rvc: worker timed out")

// ErrCanceled — the caller cancelled the context. Distinct from ErrTimeout and
// NOT to be mislabeled as a worker crash: a kill-on-cancel makes Wait() look like
// death-by-signal, so the classifier checks ctx.Err() FIRST. Wraps context.Canceled.
var ErrCanceled = errors.New("rvc: render canceled")

// closedErrCategories is the frozen closed set of per-block ERR categories. An
// ERR whose category is outside this set is a protocol violation, not a convert
// failure (forward-compat = fatal).
var closedErrCategories = map[string]struct{}{
	"bad-args":     {},
	"bad-voice":    {},
	"read-failed":  {},
	"infer-failed": {},
	"write-failed": {},
}

func isKnownErrCategory(cat string) bool {
	_, ok := closedErrCategories[cat]
	return ok
}

// isENOENT detects "binary not on disk" wrapped inside an *exec.Error / os.PathError.
// Mirrors render/sherpa's helper: errors.Is(exec.ErrNotFound) misses it on some
// platforms, so we also string-match.
func isENOENT(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, exec.ErrNotFound) {
		return true
	}
	msg := err.Error()
	return strings.Contains(msg, "no such file or directory") ||
		strings.Contains(msg, "executable file not found")
}

// oneLine collapses captured multi-line stderr into a single diagnostic line.
func oneLine(s string) string {
	s = strings.TrimSpace(s)
	s = strings.ReplaceAll(s, "\r", " ")
	s = strings.ReplaceAll(s, "\n", " ")
	return s
}
