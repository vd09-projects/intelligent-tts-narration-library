// Package gptsovits is a phase-one GPT-SoVITS (GSO) Renderer backend — a PEER
// text->audio engine alongside render/sherpa (Kokoro), NOT a decorator like
// render/rvc.
//
// GPT-SoVITS speaks a block's words directly from a .ckpt/.pth checkpoint pair plus
// a short reference clip (zero-shot clone) — there is no base engine to wrap
// (wrapping Kokoro would synthesize each block twice). So the renderer takes
// render/sherpa's SHAPE (owns the block loop, derives spoken words from the plan via
// spokenTextFor, writes one WAV per block keyed by block id, builds Timeline +
// manifest from scratch) and drives it with render/rvc's warm-subprocess-worker
// TRANSPORT: one warm process per document (sherpa's per-block spawn would repay
// GSO's ~17-22 s cold load every block; AC2 requires one warm worker per document),
// a goroutine-based lockstep exchange, exit-code classification, and a clean
// stdin-close reap. RenderBlock is a single cold-load exchange (start → one exchange
// → close), exactly render/rvc's RenderBlock.
//
// Invariants (CLAUDE.md):
//   - Imports plan/ + render/ + stdlib only. No planner/, no pipeline/, no sink/.
//   - Refused blocks are still rendered: their Refusal.Message goes through the same
//     worker. Empty Refusal.Message → ErrMalformedPlan (upstream bug, not a refusal).
//   - Honesty rule / D5: a missing/broken worker OR any per-block ERR is a returned
//     error that STOPS the whole Render — never a silent degrade, never a Refusal,
//     never a per-block skip. A 32 kHz block cannot be dropped from a 32 kHz timeline.
//   - 32 kHz mono PCM s16le — no resampling (GSO v2Pro native rate; bytesPerMs = 64).
//   - Block-level sync only. Timeline offsets keyed by block_id; no word-level timing.
//   - The renderer never reads back a final file handed to the sink — it reads only
//     the worker's intermediate minted WAV to frame-align it into OutDir.
package gptsovits

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/vd09-projects/intelligent-tts-narration-library/plan"
	"github.com/vd09-projects/intelligent-tts-narration-library/render"
)

// Engine is the concrete render.Renderer backed by the GPT-SoVITS warm subprocess.
//
// A single Engine is NOT safe for concurrent Render / RenderBlock calls: the
// unexported lastStderr / lastPID / lastOutDir diagnostic fields are written after
// each worker shutdown without synchronization, which is safe only because
// phase-one callers serialize Render (one job at a time). Use one Engine per
// goroutine if concurrency is ever required. The engine is voice-neutral (OQ1): the
// voice is resolved per call from RenderOptions.Voice, not bound at construction.
type Engine struct {
	cfg EngineConfig

	// White-box diagnostics for same-package tests (not part of the public
	// contract): the most recent worker's stderr ring, pid, and per-Render
	// GSO_OUT_DIR. Phase-one callers serialize Render, so no locking is needed.
	lastStderr string
	lastPID    int
	lastOutDir string
}

// Compile-time assertion: Engine implements render.Renderer (AC1).
var _ render.Renderer = (*Engine)(nil)

// New constructs an Engine. Pass EngineConfig{} to use the default wrapper +
// models root; tests inject EngineConfig{WrapperPath: "./testdata/fake-gptsovits.sh",
// ModelsRoot: <temp>}.
func New(cfg EngineConfig) *Engine {
	return &Engine{cfg: cfg.withDefaults()}
}

// Render produces one 32 kHz WAV per non-empty block under opts.OutDir and a
// Timeline, driving every block through ONE warm worker. Refused blocks are
// rendered (Refusal.Message). Errors — including any per-block worker ERR (D5) —
// stop the whole Render.
func (e *Engine) Render(ctx context.Context, p plan.NarrationPlan, opts render.RenderOptions) (render.RenderResult, error) {
	if opts.OutDir == "" {
		return render.RenderResult{}, fmt.Errorf("gptsovits: Render requires OutDir")
	}
	if err := os.MkdirAll(opts.OutDir, 0o755); err != nil {
		return render.RenderResult{}, fmt.Errorf("gptsovits: create OutDir %q: %w", opts.OutDir, err)
	}

	voice, err := resolveVoice(e.cfg.ModelsRoot, p, opts)
	if err != nil {
		return render.RenderResult{}, err
	}

	// Engine-owned per-Render GSO_OUT_DIR mkdtemp + guaranteed cleanup. The worker
	// never deletes inside it (#162 owns the minted-file lifecycle), so this
	// RemoveAll on EVERY exit path — success, a per-block ERR early return (D5), or
	// a panic-free error — is what prevents leaked temp WAVs.
	outDir, cleanup, err := e.makeOutDir()
	if err != nil {
		return render.RenderResult{}, err
	}
	defer cleanup()

	wallClock := e.cfg.WallClockTimeout
	ctx, cancel := context.WithTimeout(ctx, wallClock)
	defer cancel()

	w, err := startWorker(ctx, e.cfg, outDir)
	if err != nil {
		return render.RenderResult{}, classifyStart(err)
	}
	// Deferred safety-net reap. On the D5 per-block-ERR early return below,
	// classifyExchange has already killed+reaped the worker; this defer (guarded by
	// reaped) plus the deferred cleanup() above together spell the
	// shutdownQuiet + RemoveAll(GSO_OUT_DIR) obligation on the ERR path, not just the
	// success tail.
	reaped := false
	defer func() {
		if !reaped {
			w.shutdownQuiet()
		}
	}()

	format := gsoFormat()
	timing := make([]plan.BlockTiming, 0, len(p.Blocks))
	files := make([]string, 0, len(p.Blocks))
	cursorMs := 0
	firstExchange := true

	for _, blk := range p.Blocks {
		text, terr := spokenTextFor(blk)
		if terr != nil {
			return render.RenderResult{}, terr
		}

		relName := blk.ID + ".wav"
		finalPath := filepath.Join(opts.OutDir, relName)

		var durMs int
		var audioRef string
		if text == "" {
			// All-pause / empty-voiced block — zero-duration timing, no exchange, no
			// WAV, empty AudioRef so downstream sinks do not hit ENOENT. Still appears
			// in Timeline.Blocks. Mirrors sherpa/rvc empty-block handling.
			durMs = 0
			audioRef = ""
		} else {
			timeout := e.cfg.PerBlockTimeout
			if firstExchange {
				timeout = e.cfg.FirstBlockTimeout // block 1 pays the cold load.
				firstExchange = false
			}
			d, serr := e.synthBlock(ctx, w, voice, text, finalPath, timeout, format)
			if serr != nil {
				// D5: any per-block worker ERR is a HARD stop of the whole Render — no
				// partial Timeline, no skip, no degrade, not a Refusal.
				return render.RenderResult{}, serr
			}
			durMs = d
			audioRef = relName
			files = append(files, relName)
		}

		timing = append(timing, plan.BlockTiming{
			BlockID:  blk.ID,
			StartMs:  cursorMs,
			EndMs:    cursorMs + durMs,
			AudioRef: audioRef,
		})
		cursorMs += durMs
	}

	if err := w.close(); err != nil {
		return render.RenderResult{}, err
	}
	reaped = true
	e.lastStderr = w.stderr.String()
	e.lastPID = w.pid()

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

// RenderBlock re-renders a single block (the escalation path): a single cold-load
// worker exchange (start → one FirstBlockTimeout exchange → close). Timing.StartMs =
// 0 — the caller patches the result into the existing timeline, leaving other blocks'
// audio + sync untouched (same convention as sherpa/rvc). RenderBlock ignores the
// whole-document WallClockTimeout (render.Renderer contract); it instead derives its
// OWN wall bound from the single exchange's FirstBlockTimeout plus teardown headroom
// (see the context.WithTimeout below) — the single-block analogue of Render's
// WallClockTimeout wrap — so a wedged worker can never block the escalation path
// indefinitely even when the caller passes a deadline-less ctx.
func (e *Engine) RenderBlock(ctx context.Context, p plan.NarrationPlan, blockID string, opts render.RenderOptions) (render.BlockRender, error) {
	if opts.OutDir == "" {
		return render.BlockRender{}, fmt.Errorf("gptsovits: RenderBlock requires OutDir")
	}
	if err := os.MkdirAll(opts.OutDir, 0o755); err != nil {
		return render.BlockRender{}, fmt.Errorf("gptsovits: create OutDir %q: %w", opts.OutDir, err)
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

	voice, err := resolveVoice(e.cfg.ModelsRoot, p, opts)
	if err != nil {
		return render.BlockRender{}, err
	}

	text, terr := spokenTextFor(blk)
	if terr != nil {
		return render.BlockRender{}, terr
	}

	format := gsoFormat()
	relName := blk.ID + ".wav"
	finalPath := filepath.Join(opts.OutDir, relName)

	if text == "" {
		// Empty-text block: no exchange (mirror sherpa/rvc) — zero duration, empty AudioRef.
		return render.BlockRender{
			BlockID: blockID,
			Audio:   render.AudioStream{Dir: opts.OutDir, Files: []string{}},
			Timing:  plan.BlockTiming{BlockID: blockID, StartMs: 0, EndMs: 0, AudioRef: ""},
			Format:  format,
		}, nil
	}

	outDir, cleanup, err := e.makeOutDir()
	if err != nil {
		return render.BlockRender{}, err
	}
	defer cleanup()

	// Own wall bound: render.Renderer says RenderBlock ignores the whole-document
	// WallClockTimeout, so cap the whole escalation lifecycle — startWorker → the
	// single cold-load exchange → close — at the exchange's FirstBlockTimeout budget
	// plus teardown headroom. Comfortably exceeds FirstBlockTimeout so a legitimate
	// cold load is never clipped, yet a wedged worker cannot block RenderBlock forever
	// even if the caller passed a deadline-less ctx (the analogue of Render's
	// WallClockTimeout wrap for a single block).
	ctx, cancelDeadline := context.WithTimeout(ctx, e.cfg.FirstBlockTimeout+renderBlockTeardownGrace)
	defer cancelDeadline()

	w, err := startWorker(ctx, e.cfg, outDir)
	if err != nil {
		return render.BlockRender{}, classifyStart(err)
	}
	reaped := false
	defer func() {
		if !reaped {
			w.shutdownQuiet()
		}
	}()

	durMs, serr := e.synthBlock(ctx, w, voice, text, finalPath, e.cfg.FirstBlockTimeout, format)
	if serr != nil {
		return render.BlockRender{}, serr
	}

	if err := w.close(); err != nil {
		return render.BlockRender{}, err
	}
	reaped = true
	e.lastStderr = w.stderr.String()
	e.lastPID = w.pid()

	return render.BlockRender{
		BlockID: blockID,
		Audio:   render.AudioStream{Dir: opts.OutDir, Files: []string{relName}},
		Timing:  plan.BlockTiming{BlockID: blockID, StartMs: 0, EndMs: durMs, AudioRef: relName},
		Format:  format,
	}, nil
}

// synthBlock runs ONE warm exchange for a block: assemble the 5-token request,
// exchange it, then frame-align the worker-minted WAV into OutDir/<id>.wav. The
// align happens IMMEDIATELY after the OK (before the next exchange) — the D4
// buffer-before-next-request discipline (see alignInto). Returns the aligned
// duration (ms).
func (e *Engine) synthBlock(ctx context.Context, w *worker, voice voiceEntry, text, finalPath string, timeout time.Duration, format plan.AudioFormat) (int, error) {
	reqLine, err := buildRequest(text, voice.refAudioPath, voice.promptText, voice.slug)
	if err != nil {
		return 0, err // newline/CR in a token — engine-side bad-args parity, hard stop.
	}
	mintedPath, err := w.exchange(ctx, timeout, reqLine)
	if err != nil {
		return 0, w.classifyExchange(err)
	}
	durMs, err := alignInto(mintedPath, finalPath, format)
	if err != nil {
		return 0, err
	}
	return durMs, nil
}

// makeOutDir creates the private per-Render GSO_OUT_DIR (the worker mints its
// content-addressed WAVs here) and returns a cleanup that RemoveAll's it on both
// success and error — unless RetainOutDir is set, in which case it logs the retained
// path (debug knob). The sink only ever sees the final aligned WAVs under OutDir.
func (e *Engine) makeOutDir() (string, func(), error) {
	dir, err := os.MkdirTemp("", "gso-out-*")
	if err != nil {
		return "", nil, fmt.Errorf("gptsovits: create GSO_OUT_DIR: %w", err)
	}
	e.lastOutDir = dir // white-box: distinct per Render (isolation test).
	cleanup := func() {
		if e.cfg.RetainOutDir {
			fmt.Fprintf(os.Stderr, "gptsovits: retaining GSO_OUT_DIR %s\n", dir)
			return
		}
		_ = os.RemoveAll(dir)
	}
	return dir, cleanup, nil
}

// spokenTextFor derives the literal words to send to the worker for a block. Refused
// blocks speak their Refusal.Message (honesty rule); an empty Refusal.Message is an
// upstream bug (ErrMalformedPlan), not a refusal. Identical to sherpa's derivation.
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

// writeManifest writes a sink-readable manifest: "<order> <block_id> <relpath>" one
// block per line, in plan order. Returns the absolute path.
//
// DUP: writeManifest is duplicated in render/sherpa + render/rvc + here (each an
// unexported byte-for-byte copy, callable from none of the others). If a fourth
// producer ever appears, extract a shared helper to render/internal.
func writeManifest(outDir string, blocks []plan.Block, timing []plan.BlockTiming) (string, error) {
	path := filepath.Join(outDir, "manifest.txt")
	f, err := os.Create(path)
	if err != nil {
		return "", fmt.Errorf("gptsovits: write manifest: %w", err)
	}
	defer func() { _ = f.Close() }()

	for i, blk := range blocks {
		ref := ""
		if i < len(timing) {
			ref = timing[i].AudioRef
		}
		if _, err := fmt.Fprintf(f, "%d %s %s\n", blk.Order, blk.ID, ref); err != nil {
			return "", fmt.Errorf("gptsovits: write manifest line: %w", err)
		}
	}
	return path, nil
}
