// Listen-mode composition wiring for cmd/narrate (issue #83).
//
// runListenMode is the listen-path branch of runNarrate: it renders the whole
// plan (no streaming) into a capturing sink so the durable sink is never
// touched, installs the single SIGINT/SIGTERM handler, enters raw mode, and
// hands the rendered Timeline + per-block WAV dir to runListen.
//
// Temp-dir ownership: runNarrate creates the temp dir and, for the listen path,
// transfers removal ownership to the listen loop (which removes it exactly once
// in its cleanup). runNarrate must NOT also defer-remove it for this path —
// that would be the double-owned-cleanup risk. See runNarrate (main.go) for the
// suppression of its defer on args.Listen.
package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/vd09-projects/intelligent-tts-narration-library/adapter/file"
	"github.com/vd09-projects/intelligent-tts-narration-library/pipeline"
	"github.com/vd09-projects/intelligent-tts-narration-library/plan"
	"github.com/vd09-projects/intelligent-tts-narration-library/render/sherpa"
)

// listenCodeFloor is the additive L2 floor applied to code blocks in listen
// mode (planner-task.md integration point: CodeMinLevel). Code at L1 is a bare
// line count; lifting the floor to L2 gives a listener the count + declarations
// without an intelligence adapter (deterministic, honesty-rule safe). Wired via
// pipeline.PipelineDefaults.CodeMinLevel (already plumbed by issue #73).
const listenCodeFloor = plan.L2

// newListenPipeline builds the render-only pipeline for listen mode: same
// composition root as runNarrate's whole-doc path but with a capturing sink
// (no playback, no durable write) and the code L2 floor. Package-level so tests
// can swap it for a stub that returns a canned Timeline without spawning Kokoro.
var newListenPipeline = func(outDir string, args flagSet, capturer *capturingSink) pipeline.Narrator {
	return pipeline.New(
		file.New(),
		chooseIntelligence(args),
		sherpa.New(sherpa.EngineConfig{}),
		capturer,
		pipeline.PipelineDefaults{
			Level:        plan.Level(args.Level),
			OutDir:       outDir,
			Locale:       "en",
			CodeMinLevel: listenCodeFloor,
		},
	)
}

// stdinReaders builds the raw-stdin byte + line readers runListen consumes.
// Package-level seam so tests inject scripted input. Production reads os.Stdin
// one byte at a time (it is already in raw mode by the time these run).
var stdinReaders = func() (readByte func() (byte, error), readLine func() (string, error)) {
	readByte = func() (byte, error) {
		var buf [1]byte
		n, err := os.Stdin.Read(buf[:])
		if err != nil {
			return 0, err
		}
		if n == 0 {
			return 0, io.EOF
		}
		return buf[0], nil
	}
	readLine = func() (string, error) {
		// Raw-mode line read: accumulate bytes until CR/LF. afplay-free; the
		// terminal does not echo in raw mode, so this is a minimal hand-rolled
		// line reader for the g target.
		var b [1]byte
		var line []byte
		for {
			n, err := os.Stdin.Read(b[:])
			if err != nil {
				return string(line), err
			}
			if n == 0 {
				continue
			}
			if b[0] == '\r' || b[0] == '\n' {
				return string(line), nil
			}
			line = append(line, b[0])
		}
	}
	return readByte, readLine
}

// loadBlockPCM reads a per-block WAV fully into memory and strips its RIFF header
// to raw 24 kHz mono int16 PCM. It is the production cfg.loadPCM seam: the whole
// buffer is in hand on return, so runListen constructs a player only after the
// full PCM is loaded (no player over a partially-read source), and the returned
// []byte wrapped in a *bytes.Reader has no file descriptor for the oto player's
// finalizer-driven teardown to read after close.
func loadBlockPCM(wavPath string) ([]byte, error) {
	f, err := os.Open(wavPath) //nolint:gosec // path is a renderer-produced temp WAV under our own outDir.
	if err != nil {
		return nil, fmt.Errorf("open block: %w", err)
	}
	defer func() { _ = f.Close() }()
	pcm, err := stripWAVToPCM(f)
	if err != nil {
		return nil, fmt.Errorf("parse wav: %w", err)
	}
	return pcm, nil
}

// runListenMode renders the whole plan into a capturing sink and drives the
// keypress transport loop. It owns the listen path's temp-dir removal (the
// caller suppresses its own defer for this path).
//
// Fail-fast order (pre-loop, planner-task.md): non-tty stdin is rejected BEFORE
// any render/raw-mode work, with an honest message, so a listener never gets a
// half-entered raw terminal or a silent hang on a piped stdin. The engine's own
// failure (no openable audio device) surfaces honestly from oto.NewContext in
// driveListen, after the bounded ready-wait.
func runListenMode(ctx context.Context, args flagSet, outDir string, stdout, stderr io.Writer) (err error) {
	// Fail-fast 1: stdin must be an interactive terminal. Single-key transport
	// is meaningless on a pipe/redirect; refuse honestly rather than hang.
	if !isTerminal(int(os.Stdin.Fd())) {
		return fmt.Errorf("%w: --listen requires an interactive terminal (stdin is not a tty)", errFlagValidation)
	}
	// Fail-fast 2: the engine's pre-loop check. With oto as the sole listen
	// engine this is a no-op (there is no external binary to look up); the
	// engine's real failure mode — no openable audio device — surfaces honestly
	// from oto.NewContext in driveListen. Retained so the fail-fast structure is
	// unchanged.
	if perr := listenEnginePreflight(); perr != nil {
		return perr
	}

	// Render the whole plan into a capturing sink — no playback, no durable
	// write. capturingSink records the plan + render.RenderResult so we get the
	// per-block Timeline + Audio.Dir without routing through sink/ephemeral or
	// sink/persistent (listen path decoupled from the durable sink).
	capturer := &capturingSink{}
	pl := newListenPipeline(outDir, args, capturer)
	absPath, aerr := filepath.Abs(args.File)
	if aerr != nil {
		return fmt.Errorf("resolve --file: %w", aerr)
	}
	if _, nerr := pl.Narrate(ctx, plan.SourceRef{
		Kind: plan.SourceKindFile,
		URI:  absPath,
	}, pipeline.NarrateRequest{
		Voice: genderToVoice[args.Gender],
	}); nerr != nil {
		return nerr
	}
	if !capturer.captured {
		return fmt.Errorf("listen: render produced no captured result")
	}
	res := capturer.result

	// Fail-fast 3: nothing to play. If no block has audio + non-zero duration,
	// the loop has nothing to do — refuse before entering raw mode.
	if len(navigableBlocks(res.Timeline)) == 0 {
		return errors.New("listen: nothing navigable to play (no block has audio + non-zero duration)")
	}

	// Enter raw mode. The restore closure is idempotent (sync.Once) and is wired
	// into BOTH the deferred cleanup below AND the signal handler, so whichever
	// fires first restores the tty.
	restore, rerr := enterRaw(int(os.Stdin.Fd()))
	if rerr != nil {
		return fmt.Errorf("listen: %w", rerr)
	}
	// Deferred idempotent restore — the panic/normal-return safety net. The loop
	// also calls restore inside its own cleanup; sync.Once makes the duplicate a
	// no-op.
	defer restore()

	// Single SIGINT/SIGTERM handler for the whole listen session. It is
	// request-not-reap: on a signal it sends ONE shutdownRequest to the loop and
	// returns. The loop (sole owner of the player handle, the tty restore, and the
	// temp-dir removal) performs the actual teardown in order
	// restore → Pause()+drop-the-reference → RemoveAll. The handler never touches
	// those itself.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	shutdown := make(chan shutdownRequest, 1)
	// handlerCtx lets us tear the handler goroutine down on normal loop exit so
	// it is not a leak (Go-reference: every goroutine has a shutdown path).
	handlerCtx, stopHandler := context.WithCancel(ctx)
	defer stopHandler()
	go func() {
		select {
		case <-sigCh:
			// Request shutdown; non-blocking send (buffered 1) so a second
			// signal never blocks the handler.
			select {
			case shutdown <- shutdownRequest{}:
			default:
			}
		case <-handlerCtx.Done():
		}
		signal.Stop(sigCh)
	}()

	readByte, readLine := stdinReaders()
	// cfg.newPlayer is left unset here: driveListen owns engine wiring — it opens
	// the process-wide oto context and injects the real player factory. This keeps
	// listen_run.go engine-neutral (it never imports oto).
	cfg := listenConfig{
		audioDir:  res.Audio.Dir,
		loadPCM:   loadBlockPCM,
		restore:   restore,
		removeAll: os.RemoveAll,
		tempDir:   outDir,
		readByte:  readByte,
		readLine:  readLine,
		shutdown:  shutdown,
		out:       stdout,
	}

	if lerr := driveListen(ctx, cfg, res.Timeline); lerr != nil {
		return lerr
	}
	_, _ = fmt.Fprintln(stderr, "listen: done")
	return nil
}
