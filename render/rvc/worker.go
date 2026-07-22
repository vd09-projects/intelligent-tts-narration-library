package rvc

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Worker exit codes (frozen v1 contract, mirrors scripts/rvc_worker.py).
const (
	exitWrapperVenvMissing = 2  // scripts/rvc: torch-free venv absent.
	exitFatalStartup       = 78 // EXIT_FATAL_STARTUP (EX_CONFIG).
	exitFatalRuntime       = 70 // EXIT_FATAL_RUNTIME (EX_SOFTWARE).
)

// stderrRingBytes bounds the captured-stderr ring. Sized comfortably above the
// torch banner + the three LOAD lines + any FATAL line (review refinement 3), so
// early LOAD lines are never evicted before the post-Wait() read on a long batch.
const stderrRingBytes = 64 << 10 // 64 KiB

// waitDelay bounds Wait() after a cancel/kill so a wedged worker holding a pipe
// cannot hang the call forever (same pattern the listen transport uses).
const waitDelay = 5 * time.Second

// ringWriter is a bounded, thread-safe io.Writer that retains the LAST
// stderrRingBytes bytes written to it. os/exec owns the copy goroutine that
// writes here (cmd.Stderr = ringWriter); cmd.Wait() joins that goroutine before
// we read via String(), so there is no concurrent reader during a run. The mutex
// makes the Write/read handoff safe under -race regardless of ordering.
type ringWriter struct {
	mu  sync.Mutex
	cap int
	buf []byte
}

func newRingWriter(capBytes int) *ringWriter {
	return &ringWriter{cap: capBytes}
}

func (r *ringWriter) Write(p []byte) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.buf = append(r.buf, p...)
	if len(r.buf) > r.cap {
		// Keep the last cap bytes; copy into a fresh slice so the backing array
		// does not grow without bound across a long-running batch.
		trimmed := make([]byte, r.cap)
		copy(trimmed, r.buf[len(r.buf)-r.cap:])
		r.buf = trimmed
	}
	return len(p), nil
}

func (r *ringWriter) String() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return string(r.buf)
}

// worker is the subprocess client that satisfies the frozen v1 wire contract:
// one warm process, stdin/stdout live pipes for the lockstep loop, stderr
// captured into a bounded ring by os/exec.
type worker struct {
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout *bufio.Reader
	stderr *ringWriter
	cancel context.CancelFunc // cancels the process ctx → SIGKILL.

	waited  bool
	waitErr error
}

// startWorker spawns the wrapper under a cancelable child of ctx (so a per-block
// timeout can kill the process without waiting for the wall-clock ctx) and wires
// stdin/stdout as live pipes with stderr captured into a bounded ring.
//
// A start failure (ENOENT / not executable) is returned raw for classifyStart to
// map to ErrWorkerMissing.
func startWorker(ctx context.Context, cfg Config) (*worker, error) {
	procCtx, cancel := context.WithCancel(ctx)

	args := []string{}
	if cfg.ModelsRoot != "" {
		args = append(args, "--models-root", cfg.ModelsRoot)
	}
	cmd := exec.CommandContext(procCtx, cfg.WrapperPath, args...)
	cmd.WaitDelay = waitDelay

	stdin, err := cmd.StdinPipe()
	if err != nil {
		cancel()
		return nil, fmt.Errorf("rvc: stdin pipe: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		cancel()
		return nil, fmt.Errorf("rvc: stdout pipe: %w", err)
	}
	ring := newRingWriter(stderrRingBytes)
	cmd.Stderr = ring // B1: os/exec owns the drain goroutine; Wait() joins it.

	if err := cmd.Start(); err != nil {
		cancel()
		return nil, err // raw — classifyStart maps ENOENT → ErrWorkerMissing.
	}

	return &worker{
		cmd:    cmd,
		stdin:  stdin,
		stdout: bufio.NewReader(stdout),
		stderr: ring,
		cancel: cancel,
	}, nil
}

// exchange performs ONE lockstep request/response over the warm worker: write one
// request line + flush, then read exactly one response line. The read runs on a
// goroutine so a per-block deadline (or caller cancel) can interrupt it; on
// interruption the process is killed and the read goroutine is drained BEFORE
// returning, so the caller can safely Wait() (respecting the StdoutPipe-before-
// Wait rule).
//
// A returned context.Canceled / context.DeadlineExceeded is classified by
// classifyExchange; a nil return means the response was `OK <expectedOut>`.
func (w *worker) exchange(ctx context.Context, timeout time.Duration, reqLine, expectedOut string) error {
	exCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	type exResult struct {
		line string
		err  error
	}
	resc := make(chan exResult, 1)
	go func() {
		if _, err := io.WriteString(w.stdin, reqLine+"\n"); err != nil {
			resc <- exResult{err: err}
			return
		}
		line, err := w.stdout.ReadString('\n')
		resc <- exResult{line: line, err: err}
	}()

	select {
	case <-exCtx.Done():
		// Per-block deadline, wall-clock deadline, or caller cancel. Kill the
		// process so the blocked read/write unblocks, then DRAIN the goroutine so
		// no read is in flight when the caller calls Wait().
		w.cancel()
		<-resc
		return exCtx.Err()
	case r := <-resc:
		if r.err != nil {
			return r.err // io error (EOF / EPIPE / short read) — worker likely died.
		}
		return parseResponse(r.line, expectedOut)
	}
}

// parseResponse parses a single worker response line with SplitN (never
// strings.Fields — the echoed out path may contain spaces).
func parseResponse(line, expectedOut string) error {
	line = strings.TrimRight(line, "\r\n")
	switch {
	case line == "OK" || strings.HasPrefix(line, "OK "):
		parts := strings.SplitN(line, " ", 2)
		if len(parts) != 2 {
			return fmt.Errorf("%w: OK response carried no path", ErrWorkerProtocol)
		}
		if parts[1] != expectedOut {
			return fmt.Errorf("%w: OK path mismatch: got %q, want %q", ErrWorkerProtocol, parts[1], expectedOut)
		}
		return nil
	case line == "ERR" || strings.HasPrefix(line, "ERR "):
		parts := strings.SplitN(line, " ", 3)
		if len(parts) < 2 {
			return fmt.Errorf("%w: ERR response carried no category", ErrWorkerProtocol)
		}
		cat := parts[1]
		msg := ""
		if len(parts) == 3 {
			msg = parts[2]
		}
		if !isKnownErrCategory(cat) {
			return fmt.Errorf("%w: unknown ERR category %q: %s", ErrWorkerProtocol, cat, msg)
		}
		return fmt.Errorf("%w: %s: %s", ErrConvertFailed, cat, msg)
	default:
		return fmt.Errorf("%w: unparseable response line %q", ErrWorkerProtocol, line)
	}
}

// buildRequest assembles the 5-token request line `<in> <out> <voice> <index_rate>
// <pitch>`. <in>/<out> are POSIX-shell-quoted so paths with spaces round-trip
// through the worker's shlex.split and the verbatim `OK <out>` echo.
func buildRequest(in, out, voice string, indexRate float64, pitch int) string {
	return fmt.Sprintf("%s %s %s %s %d",
		posixQuote(in),
		posixQuote(out),
		voice,
		strconv.FormatFloat(indexRate, 'g', -1, 64),
		pitch,
	)
}

// posixQuote single-quotes s for a POSIX shell (shlex), escaping embedded single
// quotes as '\”.
func posixQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// classifyExchange maps a failed exchange to a package sentinel. ORDER MATTERS:
// context state is checked FIRST so a kill-on-cancel is never mislabeled as a
// worker crash (a kill makes Wait() look like death-by-signal).
func (w *worker) classifyExchange(exchErr error) error {
	switch {
	case errors.Is(exchErr, context.Canceled):
		_ = w.wait()
		return fmt.Errorf("%w: %w", ErrCanceled, context.Canceled)
	case errors.Is(exchErr, context.DeadlineExceeded):
		_ = w.wait()
		return fmt.Errorf("%w: %w (stderr: %s)", ErrTimeout, context.DeadlineExceeded, oneLine(w.stderr.String()))
	case errors.Is(exchErr, ErrWorkerProtocol), errors.Is(exchErr, ErrConvertFailed):
		// Worker may still be alive (a per-block ERR keeps its loop running), but
		// phase-one policy hard-stops the batch. Kill + reap.
		w.cancel()
		_ = w.wait()
		return exchErr
	default:
		// io error (EOF / EPIPE / short read) → the worker died mid-batch. Reap
		// and branch on the exit code.
		waitErr := w.wait()
		return classifyExit(waitErr, w.stderr.String())
	}
}

// close shuts the worker down cleanly: close stdin → the worker hits EOF → clean
// exit 0 → reap. Returns the classified error if the process exited non-zero.
func (w *worker) close() error {
	_ = w.stdin.Close()
	err := w.wait()
	// Release the process context AFTER the clean reap (w.wait has returned), so
	// the stdin-close → EOF → exit-0 shutdown is never pre-empted by a SIGKILL.
	// This frees the child context on the happy path of BOTH Render and
	// RenderBlock: RenderBlock runs the worker under the raw caller ctx with no
	// wall-clock defer cancel() to release it, so without this a
	// context.Background() caller would leak one child context per escalation.
	// Idempotent — a no-op if cancel already fired on an error/timeout path.
	w.cancel()
	if err != nil {
		return classifyExit(err, w.stderr.String())
	}
	return nil
}

// shutdownQuiet is the deferred safety net: kill + reap if the worker was not
// already reaped by close/classifyExchange. Idempotent via wait().
func (w *worker) shutdownQuiet() {
	w.cancel()
	_ = w.wait()
}

// wait reaps the process exactly once, memoizing the result so the safety-net
// defer and the classify/close paths can all call it without a double-Wait error.
func (w *worker) wait() error {
	if w.waited {
		return w.waitErr
	}
	w.waited = true
	w.waitErr = w.cmd.Wait()
	return w.waitErr
}

func (w *worker) pid() int {
	if w.cmd.Process == nil {
		return 0
	}
	return w.cmd.Process.Pid
}

// classifyExit maps a Wait() error (or a clean exit reached via an unexpected
// EOF) to a package sentinel, with the fix hint and captured FATAL line attached.
func classifyExit(waitErr error, stderr string) error {
	if waitErr == nil {
		// Clean exit 0, but we only reach here after an io error / short count —
		// the worker closed stdout mid-batch. That is a framing violation.
		return fmt.Errorf("%w: worker exited cleanly before responding (stderr: %s)", ErrWorkerProtocol, oneLine(stderr))
	}
	var exitErr *exec.ExitError
	if errors.As(waitErr, &exitErr) {
		switch code := exitErr.ExitCode(); code {
		case exitWrapperVenvMissing:
			return fmt.Errorf("%w: wrapper exit 2 (stderr: %s)", ErrWorkerMissing, oneLine(stderr))
		case exitFatalStartup:
			return fmt.Errorf("%w: worker exit 78 (stderr: %s)", ErrWorkerStartup, oneLine(stderr))
		case exitFatalRuntime:
			return fmt.Errorf("%w: worker exit 70 (stderr: %s)", ErrWorkerRuntime, oneLine(stderr))
		case -1:
			// Death by signal (ExitCode == -1). ctx was already checked clean by
			// classifyExchange, so this is a genuine worker crash, not a cancel kill.
			return fmt.Errorf("%w: worker killed by signal (stderr: %s)", ErrWorkerRuntime, oneLine(stderr))
		default:
			return fmt.Errorf("%w: worker exit %d (stderr: %s)", ErrWorkerRuntime, code, oneLine(stderr))
		}
	}
	return fmt.Errorf("%w: %w (stderr: %s)", ErrWorkerRuntime, waitErr, oneLine(stderr))
}

// classifyStart maps a spawn failure to ErrWorkerMissing (ENOENT / not
// executable) or ErrWorkerRuntime (any other start failure).
func classifyStart(err error) error {
	if isENOENT(err) {
		return fmt.Errorf("%w: %w", ErrWorkerMissing, err)
	}
	return fmt.Errorf("%w: worker failed to start: %w", ErrWorkerRuntime, err)
}
