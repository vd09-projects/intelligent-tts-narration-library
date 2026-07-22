// Package rvc is a render.Renderer decorator that repaints each per-block Kokoro
// 24 kHz WAV into a 40 kHz mono s16le character voice via the torch-free ONNX RVC
// worker (scripts/rvc_worker.py, #144) run as an ephemeral per-job subprocess.
//
// It wraps an inner render.Renderer (the Kokoro/sherpa engine) and is a drop-in
// render.Renderer itself. One worker process is spawned per Render, kept warm
// across blocks, and reaped at job end (~0 idle RAM). RVC phase P2 (#145).
//
// Invariants (CLAUDE.md):
//   - Lives in render/ (an edge) — imports plan/ + render/ + stdlib only. No
//     planner/, no plan/ I/O rule touched.
//   - Honesty rule: a missing/broken worker is a returned error that STOPS the
//     pipeline (with a fix hint), NEVER a silent degrade back to plain Kokoro.
//   - Block-level sync only. Timeline offsets are keyed by block_id and written
//     by this renderer; no word-level timing.
//   - 40 kHz mono s16le end-to-end when RVC is on — RenderResult.Format,
//     BlockRender.Format, and Timeline.Format all reflect 40 kHz (never 24000).
//     The 40 kHz WAVs are frame-aligned to whole-ms into OutDir so the ms↔byte
//     round-trip stays exact for the persistent sink (#146).
//   - Apache-2.0: the worker is a separate process (no GPL linking).
//
// Locked Decision 5: this decorator owns the full RVC target →
// {Kokoro source voice, index_rate, pitch} map and is the ONLY place the target
// slug is translated — it overrides RenderOptions.Voice to the Kokoro source
// before calling inner, so sherpa's resolveVoice only ever sees a Kokoro voice.
package rvc

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/vd09-projects/intelligent-tts-narration-library/plan"
	"github.com/vd09-projects/intelligent-tts-narration-library/render"
)

// Renderer is the RVC render.Renderer decorator.
//
// A single Renderer is NOT safe for concurrent Render / RenderBlock calls: the
// unexported lastStderr / lastPID diagnostic fields are written after each
// worker shutdown without synchronization, which is safe only because phase-one
// callers serialize Render (one job at a time). Use one Renderer per goroutine
// if concurrency is ever required.
type Renderer struct {
	inner render.Renderer
	cfg   Config
	voice voiceEntry

	// lastStderr / lastPID capture diagnostics from the most recent worker for
	// white-box tests (warm-load-once: the three LOAD lines + a single PID).
	// Not part of the public contract; unexported so only same-package tests
	// read them. Phase-one callers serialize Render, so no locking is needed.
	lastStderr string
	lastPID    int
}

// Compile-time assertion: Renderer implements render.Renderer.
var _ render.Renderer = (*Renderer)(nil)

// New constructs an RVC decorator over inner. cfg.Voice is validated EAGERLY
// against the RVC roster — an unknown slug returns ErrUnsupportedVoice now
// (construction time), not on first Render.
func New(inner render.Renderer, cfg Config) (*Renderer, error) {
	if inner == nil {
		return nil, fmt.Errorf("rvc: inner renderer must not be nil")
	}
	v, err := resolveVoice(cfg.Voice)
	if err != nil {
		return nil, err
	}
	return &Renderer{inner: inner, cfg: cfg.withDefaults(), voice: v}, nil
}

// rvcFormat is the RVC output format: 40 kHz mono s16le (the analogue of
// render.DefaultFormat, which is Kokoro's 24 kHz).
func rvcFormat() plan.AudioFormat {
	return plan.AudioFormat{
		SampleRate: 40000,
		Channels:   1,
		Encoding:   "pcm_s16le",
	}
}

// OutputFormat is the exported RVC output format (40 kHz mono s16le) the
// composition roots (#146) hand to persistent.WithExpectedFormat so the
// format-validating sinks accept the 40 kHz container. It wraps the unexported
// rvcFormat so the decorator keeps a single source of truth for its rate.
func OutputFormat() plan.AudioFormat {
	return rvcFormat()
}

// Render renders the whole plan through Kokoro (into a private intermediate dir)
// then repaints every non-empty block WAV through one warm RVC worker, landing
// frame-aligned 40 kHz finals under opts.OutDir with a rebuilt Timeline + manifest.
func (r *Renderer) Render(ctx context.Context, p plan.NarrationPlan, opts render.RenderOptions) (render.RenderResult, error) {
	if opts.OutDir == "" {
		return render.RenderResult{}, fmt.Errorf("rvc: Render requires OutDir")
	}
	if err := os.MkdirAll(opts.OutDir, 0o755); err != nil {
		return render.RenderResult{}, fmt.Errorf("rvc: create OutDir %q: %w", opts.OutDir, err)
	}

	interDir, cleanup, err := r.makeIntermediateDir()
	if err != nil {
		return render.RenderResult{}, err
	}
	defer cleanup()

	// Decision 5: translate the RVC target to its Kokoro source EXACTLY ONCE,
	// overriding the inner voice. sherpa never sees the RVC target.
	innerOpts := opts
	innerOpts.OutDir = interDir
	innerOpts.Voice = r.voice.source

	innerRes, err := r.inner.Render(ctx, p, innerOpts)
	if err != nil {
		return render.RenderResult{}, err // inner errors propagate unchanged.
	}

	// Wall-clock budget for the RVC repaint phase only (worker startup + per-block
	// exchanges); the inner Kokoro render above already ran under the caller's ctx.
	wallClock := r.cfg.WallClockTimeout
	ctx, cancel := context.WithTimeout(ctx, wallClock)
	defer cancel()

	w, err := startWorker(ctx, r.cfg)
	if err != nil {
		return render.RenderResult{}, classifyStart(err)
	}
	reaped := false
	defer func() {
		if !reaped {
			w.shutdownQuiet()
		}
	}()

	format := rvcFormat()
	timing := make([]plan.BlockTiming, 0, len(innerRes.Timeline.Blocks))
	files := make([]string, 0, len(innerRes.Timeline.Blocks))
	cursorMs := 0
	firstExchange := true

	for _, bt := range innerRes.Timeline.Blocks {
		var durMs int
		var audioRef string

		if bt.AudioRef == "" {
			// Empty-text block (all-pause / no speech): no repaint, zero duration,
			// empty AudioRef preserved — mirrors sherpa's empty-block handling.
			durMs = 0
			audioRef = ""
		} else {
			timeout := r.cfg.PerBlockTimeout
			if firstExchange {
				timeout = r.cfg.FirstBlockTimeout // block 1 pays cold-load.
				firstExchange = false
			}

			d, relName, err := r.repaintBlock(ctx, w, interDir, opts.OutDir, bt, timeout, format)
			if err != nil {
				return render.RenderResult{}, err
			}
			durMs = d
			audioRef = relName
			files = append(files, relName)
		}

		timing = append(timing, plan.BlockTiming{
			BlockID:  bt.BlockID,
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
	r.lastStderr = w.stderr.String()
	r.lastPID = w.pid()

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

// RenderBlock re-renders a single block (the escalation path): Kokoro into the
// intermediate dir → one cold-load worker exchange → frame-aligned 40 kHz final
// under opts.OutDir. Timing.StartMs = 0 (the caller patches into the existing
// timeline, same convention as sherpa).
func (r *Renderer) RenderBlock(ctx context.Context, p plan.NarrationPlan, blockID string, opts render.RenderOptions) (render.BlockRender, error) {
	if opts.OutDir == "" {
		return render.BlockRender{}, fmt.Errorf("rvc: RenderBlock requires OutDir")
	}
	if err := os.MkdirAll(opts.OutDir, 0o755); err != nil {
		return render.BlockRender{}, fmt.Errorf("rvc: create OutDir %q: %w", opts.OutDir, err)
	}

	interDir, cleanup, err := r.makeIntermediateDir()
	if err != nil {
		return render.BlockRender{}, err
	}
	defer cleanup()

	innerOpts := opts
	innerOpts.OutDir = interDir
	innerOpts.Voice = r.voice.source

	innerBR, err := r.inner.RenderBlock(ctx, p, blockID, innerOpts)
	if err != nil {
		return render.BlockRender{}, err
	}

	format := rvcFormat()

	// Empty-text block: no repaint (mirror sherpa) — zero duration, empty AudioRef.
	if innerBR.Timing.AudioRef == "" {
		return render.BlockRender{
			BlockID: blockID,
			Audio:   render.AudioStream{Dir: opts.OutDir, Files: []string{}},
			Timing:  plan.BlockTiming{BlockID: blockID, StartMs: 0, EndMs: 0, AudioRef: ""},
			Format:  format,
		}, nil
	}

	w, err := startWorker(ctx, r.cfg)
	if err != nil {
		return render.BlockRender{}, classifyStart(err)
	}
	reaped := false
	defer func() {
		if !reaped {
			w.shutdownQuiet()
		}
	}()

	// One cold-load exchange — RenderBlock pays LOAD net_g / LOAD index, so use
	// FirstBlockTimeout. RenderBlock ignores WallClockTimeout per the interface.
	bt := plan.BlockTiming{BlockID: blockID, AudioRef: innerBR.Timing.AudioRef}
	durMs, relName, err := r.repaintBlock(ctx, w, interDir, opts.OutDir, bt, r.cfg.FirstBlockTimeout, format)
	if err != nil {
		return render.BlockRender{}, err
	}

	if err := w.close(); err != nil {
		return render.BlockRender{}, err
	}
	reaped = true
	r.lastStderr = w.stderr.String()
	r.lastPID = w.pid()

	return render.BlockRender{
		BlockID: blockID,
		Audio:   render.AudioStream{Dir: opts.OutDir, Files: []string{relName}},
		Timing:  plan.BlockTiming{BlockID: blockID, StartMs: 0, EndMs: durMs, AudioRef: relName},
		Format:  format,
	}, nil
}

// repaintBlock converts one intermediate 24 kHz WAV through the warm worker to a
// raw 40 kHz WAV, then frame-aligns it into OutDir/<id>.wav. Returns the aligned
// duration (ms) and the relative final filename.
func (r *Renderer) repaintBlock(ctx context.Context, w *worker, interDir, outDir string, bt plan.BlockTiming, timeout time.Duration, format plan.AudioFormat) (int, string, error) {
	inPath := filepath.Join(interDir, bt.AudioRef)
	rawOut := filepath.Join(interDir, bt.BlockID+".40k.wav")
	relName := bt.BlockID + ".wav"
	finalPath := filepath.Join(outDir, relName)

	reqLine := buildRequest(inPath, rawOut, r.cfg.Voice, r.voice.indexRate, r.voice.pitch)
	if err := w.exchange(ctx, timeout, reqLine, rawOut); err != nil {
		return 0, "", w.classifyExchange(err)
	}

	durMs, err := alignInto(rawOut, finalPath, format)
	if err != nil {
		return 0, "", err
	}
	return durMs, relName, nil
}

// makeIntermediateDir creates the private 24 kHz staging dir and returns a cleanup
// that removes it on both success and error — unless RetainIntermediates is set,
// in which case it logs the retained path (debug knob). The sink only ever sees
// the final 40 kHz files under OutDir.
func (r *Renderer) makeIntermediateDir() (string, func(), error) {
	dir, err := os.MkdirTemp("", "rvc-inter-*")
	if err != nil {
		return "", nil, fmt.Errorf("rvc: create intermediate dir: %w", err)
	}
	cleanup := func() {
		if r.cfg.RetainIntermediates {
			fmt.Fprintf(os.Stderr, "rvc: retaining intermediate dir %s\n", dir)
			return
		}
		_ = os.RemoveAll(dir)
	}
	return dir, cleanup, nil
}

// writeManifest writes a sink-readable manifest: "<order> <block_id> <relpath>"
// one block per line, in plan order. Returns the absolute path.
//
// DUP: this is a byte-for-byte reimplementation of render/sherpa.writeManifest,
// which is unexported and cannot be called from here; the decorator must emit the
// same format pointing at the 40 kHz finals (not the inner sherpa manifest, which
// points at intermediates). Kept as a private copy under the no-ticket-churn
// standing order — see discovered_followups for the extract-to-render/internal
// option if a third manifest producer ever appears.
func writeManifest(outDir string, blocks []plan.Block, timing []plan.BlockTiming) (string, error) {
	path := filepath.Join(outDir, "manifest.txt")
	f, err := os.Create(path)
	if err != nil {
		return "", fmt.Errorf("rvc: write manifest: %w", err)
	}
	defer func() { _ = f.Close() }()

	for i, blk := range blocks {
		ref := ""
		if i < len(timing) {
			ref = timing[i].AudioRef
		}
		if _, err := fmt.Fprintf(f, "%d %s %s\n", blk.Order, blk.ID, ref); err != nil {
			return "", fmt.Errorf("rvc: write manifest line: %w", err)
		}
	}
	return path, nil
}
