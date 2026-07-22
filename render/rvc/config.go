package rvc

import "time"

// Default worker path + timeouts. Mirrors sherpa's 60 s / 10 min budget, plus a
// larger cold-load budget for the first block (which pays LOAD net_g / LOAD index
// on top of inference — see FirstBlockTimeout).
const (
	defaultWrapperPath = "./scripts/rvc"

	defaultPerBlockTimeout  = 60 * time.Second
	defaultWallClockTimeout = 10 * time.Minute
	// defaultFirstBlockTimeout budgets block 1 (and every RenderBlock call)
	// separately: the worker lazily loads net_g + index on first use, so the
	// first exchange is materially slower than the warm ones. ~90 s is a guess
	// pending a by-ear /verify against the real worker (Open Question 3).
	defaultFirstBlockTimeout = 90 * time.Second
)

// Config configures the RVC render decorator.
//
// Voice is the single RVC target slug for the whole job (one character voice per
// job, Locked Decision 4) — cool-jahns | confident-neal. It is validated eagerly
// by New; an unknown slug is a construction-time ErrUnsupportedVoice.
type Config struct {
	// Voice is the RVC target slug (cool-jahns | confident-neal).
	Voice string

	// WrapperPath is the path to the scripts/rvc wrapper. Empty → defaultWrapperPath.
	// Tests inject ./testdata/fake-rvc-worker.sh here.
	WrapperPath string

	// ModelsRoot, when non-empty, is passed to the worker as --models-root.
	// Empty → the worker uses its own default (<repo>/assets/rvc-models).
	ModelsRoot string

	// PerBlockTimeout caps a single warm repaint exchange. Zero → package default.
	PerBlockTimeout time.Duration

	// WallClockTimeout caps the RVC repaint phase of a Render call — worker
	// startup plus every per-block repaint exchange. It is applied AFTER the inner
	// Kokoro render returns, so it does NOT bound that inner render (which runs
	// under the caller's own RenderOptions budget). Zero → package default.
	// Ignored by RenderBlock (mirrors the render.Renderer contract).
	WallClockTimeout time.Duration

	// FirstBlockTimeout caps the first (cold-load) exchange of a Render and every
	// RenderBlock exchange. Zero → package default (larger than PerBlockTimeout).
	FirstBlockTimeout time.Duration

	// RetainIntermediates keeps the private 24 kHz intermediate dir on disk
	// (debug knob). Default false → the dir is removed on both success and error.
	RetainIntermediates bool
}

// withDefaults returns a copy of c with zero-valued knobs filled in.
func (c Config) withDefaults() Config {
	if c.WrapperPath == "" {
		c.WrapperPath = defaultWrapperPath
	}
	if c.PerBlockTimeout == 0 {
		c.PerBlockTimeout = defaultPerBlockTimeout
	}
	if c.WallClockTimeout == 0 {
		c.WallClockTimeout = defaultWallClockTimeout
	}
	if c.FirstBlockTimeout == 0 {
		c.FirstBlockTimeout = defaultFirstBlockTimeout
	}
	return c
}
