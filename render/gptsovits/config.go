package gptsovits

import "time"

// Default worker path + timeouts. GSO's cold load (~17-22 s on an M1 Pro) is
// materially slower than a warm block (~3.6-3.8 s), so the first block of a
// Render (and every RenderBlock call) gets a larger budget than warm blocks.
//
// First-pass figures — headroomed guesses pending a by-ear /verify against the
// real worker (#164). Do not treat these as measured.
const (
	defaultWrapperPath = "./scripts/gso"

	defaultModelsRoot = "assets/gptsovits-models"

	defaultPerBlockTimeout   = 30 * time.Second
	defaultFirstBlockTimeout = 45 * time.Second
	defaultWallClockTimeout  = 10 * time.Minute
)

// EngineConfig configures the GSO peer renderer.
//
// The engine is voice-neutral (Open Question 1): the voice is resolved per call
// from RenderOptions.Voice (default cool-jahns-gso), matching render/sherpa's peer
// shape — NOT bound at construction like the render/rvc decorator.
type EngineConfig struct {
	// WrapperPath is the path to the scripts/gso wrapper. Empty → defaultWrapperPath.
	// Tests inject ./testdata/fake-gptsovits.sh here.
	WrapperPath string

	// ModelsRoot locates the packaged per-voice artifacts the engine reads to
	// build the wire <ref_audio_path>/<prompt_text> tokens (D3 source-of-record:
	// <ModelsRoot>/<voice>/ref_audio.wav + ref_transcript.txt). Empty →
	// defaultModelsRoot ("assets/gptsovits-models", relative to the process CWD —
	// the repo root under the make targets). Tests inject a temp dir.
	ModelsRoot string

	// PerBlockTimeout caps a single warm block exchange. Zero → package default.
	PerBlockTimeout time.Duration

	// FirstBlockTimeout caps the first (cold-load) exchange of a Render and every
	// RenderBlock exchange. Zero → package default (larger than PerBlockTimeout).
	FirstBlockTimeout time.Duration

	// WallClockTimeout caps the whole Render call (worker startup + every block
	// exchange). Zero → package default. Ignored by RenderBlock (mirrors the
	// render.Renderer contract).
	WallClockTimeout time.Duration

	// RetainOutDir keeps the private per-Render GSO_OUT_DIR mkdtemp on disk (debug
	// knob). Default false → the dir is RemoveAll'd on both success and error. The
	// worker never deletes inside it (#162 owns the minted-file lifecycle), so a
	// leaked dir means leaked temp WAVs.
	RetainOutDir bool
}

// withDefaults returns a copy of c with zero-valued knobs filled in.
func (c EngineConfig) withDefaults() EngineConfig {
	if c.WrapperPath == "" {
		c.WrapperPath = defaultWrapperPath
	}
	if c.ModelsRoot == "" {
		c.ModelsRoot = defaultModelsRoot
	}
	if c.PerBlockTimeout == 0 {
		c.PerBlockTimeout = defaultPerBlockTimeout
	}
	if c.FirstBlockTimeout == 0 {
		c.FirstBlockTimeout = defaultFirstBlockTimeout
	}
	if c.WallClockTimeout == 0 {
		c.WallClockTimeout = defaultWallClockTimeout
	}
	return c
}
