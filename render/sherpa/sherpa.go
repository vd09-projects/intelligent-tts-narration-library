// Package sherpa is the phase-one Kokoro Renderer backend.
//
// It implements render.Renderer by shelling out to the scripts/kokoro
// wrapper (kokoro-onnx 0.5.0) per Decision
// 2026-06-18-kokoro-distribution-kokoro-onnx-over-kokoro. In-process CGo
// bindings (sherpa-onnx-go) are deferred.
//
// Invariants:
//   - Imports plan/ + render/ + stdlib only. No planner/, no adapter/, no sink/.
//   - Refused blocks are still rendered: their Refusal.Message goes through
//     the same subprocess. Empty Refusal.Message → ErrMalformedPlan (upstream
//     bug, not a refusal).
//   - The renderer never reads bytes back. WAVs sit on disk for the sink.
//   - 24 kHz mono PCM s16le — no resampling, no transcoding. If the wrapper
//     ever returns something else, that surfaces as an error (the wrapper
//     exits 4 rather than silently resampling).
package sherpa

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/vd09-projects/intelligent-tts-narration-library/plan"
	"github.com/vd09-projects/intelligent-tts-narration-library/render"
)

// Default timeouts (planner-task.md Decision 3).
const (
	defaultPerBlockTimeout  = 60 * time.Second
	defaultWallClockTimeout = 10 * time.Minute

	defaultBinaryPath = "./scripts/kokoro"
)

// Wrapper exit codes (mirrors render/sherpa/README.md "Failure modes").
const (
	exitOK             = 0
	exitBadArg         = 2 // unsupported voice OR empty text OR venv missing.
	exitModelMissing   = 3 // models/ files absent.
	exitSampleRateBad  = 4 // kokoro-onnx returned non-24000 Hz.
	exitScriptNotFound = 127
)

// EngineConfig — backend-level configuration for New.
//
// BinaryPath defaults to "./scripts/kokoro" when empty (the wrapper script
// shipped with this repo). Tests inject a fake binary here.
type EngineConfig struct {
	BinaryPath string
}

// Engine is the concrete render.Renderer backed by the Kokoro subprocess.
//
// Concurrent calls to Render / RenderBlock must use distinct OutDir
// values; the renderer writes files with deterministic block-id
// filenames (`<blockID>.wav`, `manifest.txt`) and provides no internal
// locking. Phase one's pipeline serializes calls; concurrency is not a
// goal.
//
// On context-timeout, exec.CommandContext sends SIGKILL by default
// (Go 1.20+) — fast-fail over graceful, intentional for phase one.
type Engine struct {
	binaryPath string
}

// Compile-time assertion: Engine implements render.Renderer.
var _ render.Renderer = (*Engine)(nil)

// New constructs an Engine. Pass EngineConfig{} to use the default wrapper
// path; tests inject EngineConfig{BinaryPath: "./testdata/fake-kokoro.sh"}.
func New(cfg EngineConfig) *Engine {
	bin := cfg.BinaryPath
	if bin == "" {
		bin = defaultBinaryPath
	}
	return &Engine{binaryPath: bin}
}

// Render produces one WAV per block under opts.OutDir and a Timeline.
// Refused blocks are rendered (Refusal.Message). Errors stop the pipeline.
func (e *Engine) Render(ctx context.Context, p plan.NarrationPlan, opts render.RenderOptions) (render.RenderResult, error) {
	if opts.OutDir == "" {
		return render.RenderResult{}, fmt.Errorf("sherpa: Render requires OutDir")
	}
	if err := os.MkdirAll(opts.OutDir, 0o755); err != nil {
		return render.RenderResult{}, fmt.Errorf("sherpa: create OutDir %q: %w", opts.OutDir, err)
	}

	voice, err := resolveVoice(p, opts)
	if err != nil {
		return render.RenderResult{}, err
	}

	wallClock := opts.WallClockTimeout
	if wallClock == 0 {
		wallClock = defaultWallClockTimeout
	}
	ctx, cancel := context.WithTimeout(ctx, wallClock)
	defer cancel()

	perBlock := opts.PerBlockTimeout
	if perBlock == 0 {
		perBlock = defaultPerBlockTimeout
	}

	format := render.DefaultFormat()
	timing := make([]plan.BlockTiming, 0, len(p.Blocks))
	files := make([]string, 0, len(p.Blocks))
	cursorMs := 0

	for _, blk := range p.Blocks {
		text, terr := spokenTextFor(blk)
		if terr != nil {
			return render.RenderResult{}, terr
		}

		relName := blk.ID + ".wav"
		outPath := filepath.Join(opts.OutDir, relName)

		var durMs int
		var audioRef string
		if text == "" {
			// All-pause / empty-voiced block — emit zero-duration timing,
			// no subprocess call, no WAV written. AudioRef stays empty so
			// downstream sinks do not hit ENOENT trying to open a missing
			// file (review-findings B2).
			durMs = 0
			audioRef = ""
		} else {
			if err := e.synthesize(ctx, perBlock, voice, text, outPath); err != nil {
				return render.RenderResult{}, err
			}
			d, derr := wavDurationMs(outPath, format)
			if derr != nil {
				return render.RenderResult{}, derr
			}
			durMs = d
			files = append(files, relName)
			audioRef = relName
		}

		timing = append(timing, plan.BlockTiming{
			BlockID:  blk.ID,
			StartMs:  cursorMs,
			EndMs:    cursorMs + durMs,
			AudioRef: audioRef,
		})
		cursorMs += durMs
	}

	manifestPath, err := writeManifest(opts.OutDir, p.Blocks, timing)
	if err != nil {
		return render.RenderResult{}, err
	}

	return render.RenderResult{
		Audio: render.AudioStream{
			Dir:          opts.OutDir,
			Files:        files,
			ManifestPath: manifestPath,
		},
		Timeline: plan.Timeline{
			PlanID: p.PlanID,
			Format: format,
			Blocks: timing,
		},
		Format: format,
	}, nil
}

// RenderBlock re-renders a single block. Caller patches the result into
// the existing RenderResult / Timeline (recomputing downstream offsets if
// needed — phase one keeps it simple and lets the caller decide).
func (e *Engine) RenderBlock(ctx context.Context, p plan.NarrationPlan, blockID string, opts render.RenderOptions) (render.BlockRender, error) {
	if opts.OutDir == "" {
		return render.BlockRender{}, fmt.Errorf("sherpa: RenderBlock requires OutDir")
	}
	if err := os.MkdirAll(opts.OutDir, 0o755); err != nil {
		return render.BlockRender{}, fmt.Errorf("sherpa: create OutDir %q: %w", opts.OutDir, err)
	}

	var blk plan.Block
	found := false
	for i := range p.Blocks {
		if p.Blocks[i].ID == blockID {
			blk = p.Blocks[i]
			found = true
			break
		}
	}
	if !found {
		return render.BlockRender{}, fmt.Errorf("%w: block %q not in plan", ErrMalformedPlan, blockID)
	}

	voice, err := resolveVoice(p, opts)
	if err != nil {
		return render.BlockRender{}, err
	}

	text, terr := spokenTextFor(blk)
	if terr != nil {
		return render.BlockRender{}, terr
	}

	perBlock := opts.PerBlockTimeout
	if perBlock == 0 {
		perBlock = defaultPerBlockTimeout
	}

	format := render.DefaultFormat()
	relName := blk.ID + ".wav"
	outPath := filepath.Join(opts.OutDir, relName)

	var durMs int
	var audioRef string
	files := []string{}
	if text != "" {
		if err := e.synthesize(ctx, perBlock, voice, text, outPath); err != nil {
			return render.BlockRender{}, err
		}
		d, derr := wavDurationMs(outPath, format)
		if derr != nil {
			return render.BlockRender{}, derr
		}
		durMs = d
		files = append(files, relName)
		audioRef = relName
	}

	return render.BlockRender{
		BlockID: blockID,
		Audio: render.AudioStream{
			Dir:   opts.OutDir,
			Files: files,
		},
		Timing: plan.BlockTiming{
			BlockID:  blockID,
			StartMs:  0, // caller patches into existing timeline.
			EndMs:    durMs,
			AudioRef: audioRef,
		},
		Format: format,
	}, nil
}

// spokenTextFor — derive the literal words to send to the subprocess for
// a block. Refused blocks speak their Refusal.Message (honesty rule).
func spokenTextFor(blk plan.Block) (string, error) {
	if blk.Status == plan.StatusRefused {
		if blk.Refusal == nil || strings.TrimSpace(blk.Refusal.Message) == "" {
			return "", fmt.Errorf("%w: block %q is refused but Refusal.Message is empty", ErrMalformedPlan, blk.ID)
		}
		return blk.Refusal.Message, nil
	}
	var parts []string
	for _, seg := range blk.Segments {
		if seg.Kind == plan.SegmentKindSpeech && seg.Text != "" {
			parts = append(parts, seg.Text)
		}
	}
	return strings.Join(parts, " "), nil
}

// synthesize runs the wrapper subprocess once and writes the WAV bytes
// from stdout to outPath atomically (temp file + rename).
//
// Stdout is captured into an in-memory buffer (one block of phase-one
// Kokoro audio is bounded — a 40 s clip is ~2 MB at 24 kHz mono s16le).
// Using bytes.Buffer avoids the StdoutPipe drain race documented in
// review-findings.md B1.
func (e *Engine) synthesize(ctx context.Context, perBlock time.Duration, voice, text, outPath string) error {
	callCtx, cancel := context.WithTimeout(ctx, perBlock)
	defer cancel()

	cmd := exec.CommandContext(callCtx, e.binaryPath, "--voice", voice, "--text", text)
	var stdout bytes.Buffer
	var stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return classifySubprocessErr(err, callCtx, stderr.String())
	}

	tmpPath := outPath + ".tmp"
	if err := os.WriteFile(tmpPath, stdout.Bytes(), 0o644); err != nil {
		return fmt.Errorf("sherpa: write temp %q: %w", tmpPath, err)
	}
	if err := os.Rename(tmpPath, outPath); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("sherpa: rename %q to %q: %w", tmpPath, outPath, err)
	}
	return nil
}

// classifySubprocessErr maps wrapper exit codes / context state / exec
// errors to the package's sentinel errors. Underlying errors are wrapped
// with %w so callers can errors.Is / errors.As all the way down (e.g.
// errors.Is(err, exec.ErrNotFound) for missing-binary diagnosis).
func classifySubprocessErr(waitErr error, callCtx context.Context, stderr string) error {
	if errors.Is(callCtx.Err(), context.DeadlineExceeded) {
		return fmt.Errorf("%w: %w (stderr: %s)", ErrTimeout, context.DeadlineExceeded, strings.TrimSpace(stderr))
	}
	// exec.Run can fail to start the process at all (binary not on disk).
	// That's our ErrMissingBinary case — surface before the ExitError check
	// since a start-failure has no exit code.
	if errors.Is(waitErr, exec.ErrNotFound) || isENOENT(waitErr) {
		return fmt.Errorf("%w: %w", ErrMissingBinary, waitErr)
	}
	var exitErr *exec.ExitError
	if errors.As(waitErr, &exitErr) {
		switch exitErr.ExitCode() {
		case exitBadArg:
			if strings.Contains(stderr, "unsupported voice") {
				return fmt.Errorf("%w: %s", ErrUnsupportedVoice, strings.TrimSpace(stderr))
			}
			return fmt.Errorf("%w: exit 2: %s", ErrSubprocess, strings.TrimSpace(stderr))
		case exitModelMissing:
			return fmt.Errorf("%w: %s", ErrMissingBinary, strings.TrimSpace(stderr))
		case exitSampleRateBad:
			return fmt.Errorf("%w: sample-rate breach: %s", ErrSubprocess, strings.TrimSpace(stderr))
		case exitScriptNotFound:
			return fmt.Errorf("%w: %s", ErrMissingBinary, strings.TrimSpace(stderr))
		default:
			return fmt.Errorf("%w: exit %d: %s", ErrSubprocess, exitErr.ExitCode(), strings.TrimSpace(stderr))
		}
	}
	return fmt.Errorf("%w: %w (stderr: %s)", ErrSubprocess, waitErr, strings.TrimSpace(stderr))
}

// isENOENT detects "binary not on disk" wrapped inside an os.PathError.
// exec.Cmd.Run returns *exec.Error{Err: syscall.ENOENT} on Linux/macOS
// when the binary path doesn't exist; errors.Is doesn't catch it without
// this unwrap on some Go versions.
func isENOENT(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "no such file or directory") ||
		strings.Contains(msg, "executable file not found")
}

// writeManifest writes a sink-readable manifest: "<order> <block_id> <relpath>"
// one block per line, plan-order. Returns the absolute path.
func writeManifest(outDir string, blocks []plan.Block, timing []plan.BlockTiming) (string, error) {
	path := filepath.Join(outDir, "manifest.txt")
	f, err := os.Create(path)
	if err != nil {
		return "", fmt.Errorf("sherpa: write manifest: %w", err)
	}
	defer func() { _ = f.Close() }()

	// timing aligns 1:1 with blocks in plan order.
	for i, blk := range blocks {
		ref := ""
		if i < len(timing) {
			ref = timing[i].AudioRef
		}
		if _, err := fmt.Fprintf(f, "%d %s %s\n", blk.Order, blk.ID, ref); err != nil {
			return "", fmt.Errorf("sherpa: write manifest line: %w", err)
		}
	}
	return path, nil
}
