// Package gptsovits errors — sentinel errors returned by the GPT-SoVITS (GSO)
// subprocess renderer.
//
// Honesty rule (CLAUDE.md, non-negotiable): every failure here is a returned
// error that STOPS the pipeline. A missing/broken worker or a per-block worker
// ERR is NEVER a silent degrade and NEVER a Refusal — the caller sees the error
// (with a fix hint) and decides. Refusals are still data (a refused block speaks
// its Refusal.Message like any other block); only worker/IO failures are errors.
//
// Precedent lock D5: any per-block ERR (ErrWorkerError, the GSO analogue of RVC's
// ErrConvertFailed) is a HARD error stopping the whole Render — no per-block skip,
// no degrade. A 32 kHz block cannot be dropped from a 32 kHz timeline, and
// fabricating one would violate the honesty rule.
package gptsovits

import (
	"errors"
	"os/exec"
	"strings"
)

// ErrUnsupportedVoice — the caller asked for a voice the GSO roster does not know.
// A caller/config bug, distinct from a worker `bad-voice` ERR.
var ErrUnsupportedVoice = errors.New("gptsovits: unsupported voice")

// ErrWorkerMissing — the scripts/gso wrapper is not on disk (exec ENOENT) OR the
// wrapper exited 2 because the torch venv (.venv-gso) is missing. Fix hint baked
// in: run `make gso-worker-venv`.
var ErrWorkerMissing = errors.New("gptsovits: worker wrapper or torch venv missing (run `make gso-worker-venv`)")

// ErrWorkerStartup — the worker exited 78 (EXIT_FATAL_STARTUP / EX_CONFIG): torch
// not importable, missing base models or voice checkpoints, or a missing GSO_REPO
// clone. Carries the captured stderr FATAL line.
var ErrWorkerStartup = errors.New("gptsovits: worker startup fatal")

// ErrWorkerRuntime — the worker exited 70 (EXIT_FATAL_RUNTIME, e.g. MemoryError)
// or died by signal for a reason NOT attributable to caller cancellation.
var ErrWorkerRuntime = errors.New("gptsovits: worker runtime fatal")

// ErrWorkerError — a per-block `ERR <category> <message>` from the closed set
// {bad-args|bad-voice|read-failed|infer-failed|write-failed}. Phase-one policy
// (D5): ANY ERR is a HARD error that stops Render. GSO analogue of the RVC
// worker's ErrConvertFailed.
var ErrWorkerError = errors.New("gptsovits: worker reported an inference error")

// ErrWorkerProtocol — the worker violated the frozen #161 wire contract: an `OK`
// line carrying no path (or an empty path), an unparseable response line, an
// unknown ERR category (forward-compat = fatal), or the worker closing stdout
// mid-batch before answering.
var ErrWorkerProtocol = errors.New("gptsovits: worker protocol violation")

// ErrTimeout — a per-block or wall-clock deadline was exceeded. Wraps
// context.DeadlineExceeded so callers can errors.Is down to it.
var ErrTimeout = errors.New("gptsovits: worker timed out")

// ErrCanceled — the caller cancelled the context. Distinct from ErrTimeout and
// NOT to be mislabeled as a worker crash: a kill-on-cancel makes Wait() look like
// death-by-signal, so the classifier checks ctx.Err() FIRST. Wraps context.Canceled.
var ErrCanceled = errors.New("gptsovits: render canceled")

// ErrMalformedPlan — the plan handed in is internally inconsistent in a way the
// renderer cannot proceed past (a refused block with empty Refusal.Message, or a
// RenderBlock id absent from the plan). An upstream bug — not a refusal.
var ErrMalformedPlan = errors.New("gptsovits: malformed plan")

// closedErrCategories is the frozen closed set of per-block ERR categories (same
// tokens as the RVC worker). An ERR whose category is outside this set is a
// protocol violation, not an inference failure (forward-compat = fatal).
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
// Mirrors render/sherpa + render/rvc: errors.Is(exec.ErrNotFound) misses it on some
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
