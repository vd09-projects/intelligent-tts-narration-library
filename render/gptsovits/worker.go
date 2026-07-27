package gptsovits

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
)

// Worker exit codes (frozen #161 contract, mirrors scripts/gptsovits_worker.py +
// the scripts/gso wrapper).
const (
	exitWrapperVenvMissing = 2  // scripts/gso: .venv-gso torch venv absent.
	exitFatalStartup       = 78 // EXIT_FATAL_STARTUP (EX_CONFIG): torch/artifacts/clone.
	exitFatalRuntime       = 70 // EXIT_FATAL_RUNTIME (EX_SOFTWARE): e.g. MemoryError.
)

// stderrRingBytes bounds the captured-stderr ring. The GSO worker shunts the
// GPT-SoVITS stdout flood to stderr (D2 — the engine reads ONLY fd-1), so this is
// sized well above the LOAD/OUT_BASE lines + any FATAL line + that banner noise so
// early lines are not evicted before the post-Wait() read.
const stderrRingBytes = 128 << 10 // 128 KiB

// waitDelay bounds Wait() after a cancel/kill so a wedged worker holding a pipe
// cannot hang the call forever.
const waitDelay = 5 * time.Second

// renderBlockTeardownGrace pads RenderBlock's own wall bound above its single
// FirstBlockTimeout exchange to cover worker teardown — reapBounded's grace-timer
// kill (waitDelay) plus the cmd's WaitDelay I/O-drain backstop (waitDelay).
const renderBlockTeardownGrace = 2 * waitDelay

// ringWriter is a bounded, thread-safe io.Writer that retains the LAST
// stderrRingBytes bytes written to it. os/exec owns the copy goroutine (cmd.Stderr
// = ringWriter); cmd.Wait() joins that goroutine before we read via String(), so
// there is no concurrent reader during a run. The mutex makes the Write/read handoff
// safe under -race regardless of ordering.
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

// worker is the subprocess client that satisfies the frozen #161 wire contract: one
// warm process, stdin/stdout live pipes for the lockstep loop, stderr captured into
// a bounded ring by os/exec. The engine reads ONLY fd-1 (stdout); everything the
// worker prints to fd-2 (LOAD/OUT_BASE, the GPT-SoVITS flood, FATAL lines) lands in
// the ring for diagnosis (D2).
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
// The child inherits the parent environment PLUS GSO_OUT_DIR pointing at the
// engine-owned per-Render mkdtemp — via cmd.Env = append(os.Environ(), ...), NOT
// os.Setenv (which is a process-global race under the concurrency overlay) and NOT a
// single-var Env (which would wipe GSO_REPO / PATH / the venv activation the wrapper
// needs). The worker resolves GSO_OUT_DIR once at startup (_resolve_out_base).
//
// A start failure (ENOENT / not executable) is returned raw for classifyStart to map
// to ErrWorkerMissing.
func startWorker(ctx context.Context, cfg EngineConfig, outDir string) (*worker, error) {
	procCtx, cancel := context.WithCancel(ctx)

	cmd := exec.CommandContext(procCtx, cfg.WrapperPath)
	cmd.WaitDelay = waitDelay
	cmd.Env = append(os.Environ(), "GSO_OUT_DIR="+outDir)

	stdin, err := cmd.StdinPipe()
	if err != nil {
		cancel()
		return nil, fmt.Errorf("gptsovits: stdin pipe: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		cancel()
		return nil, fmt.Errorf("gptsovits: stdout pipe: %w", err)
	}
	ring := newRingWriter(stderrRingBytes)
	cmd.Stderr = ring // os/exec owns the drain goroutine; Wait() joins it.

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
// returning, so the caller can safely Wait() (respecting the StdoutPipe-before-Wait
// rule).
//
// On success it returns the worker-minted output path (the `OK <out>` payload,
// line[3:] verbatim). A returned context.Canceled / context.DeadlineExceeded, an io
// error, or a parseResponse sentinel is classified by classifyExchange.
func (w *worker) exchange(ctx context.Context, timeout time.Duration, reqLine string) (string, error) {
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
		// Per-block deadline, wall-clock deadline, or caller cancel. Kill the process
		// so the blocked read/write unblocks, then DRAIN the goroutine so no read is
		// in flight when the caller calls Wait().
		w.cancel()
		<-resc
		return "", exCtx.Err()
	case r := <-resc:
		if r.err != nil {
			return "", r.err // io error (EOF / EPIPE / short read) — worker likely died.
		}
		return parseResponse(r.line)
	}
}

// parseResponse parses a single worker response line. GSO delta from the RVC worker:
// there is NO expectedOut to compare against — the worker MINTS a content-addressed
// path, so the `OK <out>` payload is the LITERAL remainder after the 3-char prefix
// `OK ` (out = line[3:]) and is returned verbatim. It is NEVER shlex-split or
// round-trip-quoted (that would corrupt a path containing a space — e.g. a
// GSO_OUT_DIR with a space). An `OK` with no/empty path is a protocol violation.
func parseResponse(line string) (string, error) {
	line = strings.TrimRight(line, "\r\n")
	switch {
	case strings.HasPrefix(line, "OK "):
		out := line[3:] // literal remainder — the worker-minted path (D1).
		if out == "" {
			return "", fmt.Errorf("%w: OK response carried an empty path", ErrWorkerProtocol)
		}
		return out, nil
	case line == "OK":
		return "", fmt.Errorf("%w: OK response carried no path", ErrWorkerProtocol)
	case strings.HasPrefix(line, "ERR "), line == "ERR":
		parts := strings.SplitN(line, " ", 3)
		if len(parts) < 2 {
			return "", fmt.Errorf("%w: ERR response carried no category", ErrWorkerProtocol)
		}
		cat := parts[1]
		msg := ""
		if len(parts) == 3 {
			msg = parts[2]
		}
		if !isKnownErrCategory(cat) {
			return "", fmt.Errorf("%w: unknown ERR category %q: %s", ErrWorkerProtocol, cat, msg)
		}
		return "", fmt.Errorf("%w: %s: %s", ErrWorkerError, cat, msg)
	default:
		return "", fmt.Errorf("%w: unparseable response line %q", ErrWorkerProtocol, line)
	}
}

// classifyExchange maps a failed exchange to a package sentinel. ORDER MATTERS:
// context state is checked FIRST so a kill-on-cancel is never mislabeled as a worker
// crash (a kill makes Wait() look like death-by-signal).
func (w *worker) classifyExchange(exchErr error) error {
	switch {
	case errors.Is(exchErr, context.Canceled):
		_ = w.wait()
		return fmt.Errorf("%w: %w", ErrCanceled, context.Canceled)
	case errors.Is(exchErr, context.DeadlineExceeded):
		_ = w.wait()
		return fmt.Errorf("%w: %w (stderr: %s)", ErrTimeout, context.DeadlineExceeded, oneLine(w.stderr.String()))
	case errors.Is(exchErr, ErrWorkerProtocol), errors.Is(exchErr, ErrWorkerError):
		// Worker may still be alive (a per-block ERR keeps its loop running), but
		// D5 hard-stops the whole Render. Kill + reap.
		w.cancel()
		_ = w.wait()
		return exchErr
	default:
		// io error (EOF / EPIPE / short read) → the worker died mid-batch, OR a
		// contract-violating worker closed stdout WITHOUT exiting. reapBounded reaps
		// with a grace-timer kill so a wedged worker cannot block Wait() forever, while
		// a worker that truly died is reaped immediately with its real exit code intact.
		waitErr := w.reapBounded()
		return classifyExit(waitErr, w.stderr.String())
	}
}

// close shuts the worker down cleanly: close stdin → the worker hits EOF → clean
// exit 0 → reap. Returns the classified error if the process exited non-zero.
//
// The reap is bounded (reapBounded): a cooperative worker exits 0 well within the
// grace window and its clean status is preserved — the process context is cancelled
// only AFTER the reap, so the stdin-close → EOF → exit-0 shutdown is never pre-empted
// by a SIGKILL — while a contract-violating worker that ignores stdin-EOF and never
// exits is force-killed rather than hanging Wait() forever.
func (w *worker) close() error {
	_ = w.stdin.Close()
	err := w.reapBounded()
	// Release the process context now that the reap has landed. Idempotent — a no-op
	// if reapBounded's grace timer already fired cancel() on a wedged worker, or if
	// cancel already fired on an error/timeout path.
	w.cancel()
	if err != nil {
		return classifyExit(err, w.stderr.String())
	}
	return nil
}

// shutdownQuiet is the deferred safety net: kill + reap if the worker was not already
// reaped by close/classifyExchange. Idempotent via wait().
func (w *worker) shutdownQuiet() {
	w.cancel()
	_ = w.wait()
}

// wait reaps the process exactly once, memoizing the result so the safety-net defer
// and the classify/close paths can all call it without a double-Wait error.
func (w *worker) wait() error {
	if w.waited {
		return w.waitErr
	}
	w.waited = true
	w.waitErr = w.cmd.Wait()
	return w.waitErr
}

// reapBounded reaps the process but bounds the wait: it schedules w.cancel() on a
// grace timer (waitDelay) so a worker that is wedged — holding a pipe, ignoring
// stdin-EOF, or closing stdout WITHOUT exiting — is force-killed and Wait() cannot
// block forever. A worker that has already exited (cleanly or with a fatal code) is
// reaped within the grace, the timer is stopped before it fires, and its real Wait()
// status is preserved intact. Cancelling the context up front instead would poison an
// exit-0 reap: os/exec's Wait injects ctx.Err() (context.Canceled) when the process
// exits successfully under an already-cancelled context, which would mislabel a
// clean-EOF protocol violation as a runtime crash. After cancel() fires, the SIGKILL
// lands and the cmd's WaitDelay bounds the residual I/O drain.
func (w *worker) reapBounded() error {
	killTimer := time.AfterFunc(waitDelay, w.cancel)
	err := w.wait()
	killTimer.Stop()
	return err
}

func (w *worker) pid() int {
	if w.cmd.Process == nil {
		return 0
	}
	return w.cmd.Process.Pid
}

// classifyExit maps a Wait() error (or a clean exit reached via an unexpected EOF) to
// a package sentinel, with the fix hint and captured FATAL line attached.
func classifyExit(waitErr error, stderr string) error {
	if waitErr == nil {
		// Clean exit 0, but we only reach here after an io error / short count — the
		// worker closed stdout mid-batch. That is a framing violation.
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

// classifyStart maps a spawn failure to ErrWorkerMissing (ENOENT / not executable)
// or ErrWorkerRuntime (any other start failure).
func classifyStart(err error) error {
	if isENOENT(err) {
		return fmt.Errorf("%w: %w", ErrWorkerMissing, err)
	}
	return fmt.Errorf("%w: worker failed to start: %w", ErrWorkerRuntime, err)
}
