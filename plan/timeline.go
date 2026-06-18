package plan

// Timeline — block-level sync the renderer adds after audio exists
// (design doc §2.6). NarrationPlan stays engine-neutral and pre-render; the
// renderer emits a Timeline keyed by Block.ID so a consumer can JOIN to the
// plan to get the full picture: spoken segment → source block → lines/region.
//
// Invariant: block-level only. No per-word, per-segment, per-char, or per-
// rune timings — word-level contradicts gist mode (spoken text ≠ source text).
// Enforced by reflection in plan_test.go.
type Timeline struct {
	PlanID string        `json:"plan_id"`
	Format AudioFormat   `json:"format"`
	Blocks []BlockTiming `json:"blocks"`
}

// BlockTiming — one row of block-level sync per Block in the plan.
// AudioRef is an opaque handle written by the sink (file path + offset,
// or whatever the sink chose); the core never interprets it.
type BlockTiming struct {
	BlockID  string `json:"block_id"`
	StartMs  int    `json:"start_ms"`
	EndMs    int    `json:"end_ms"`
	AudioRef string `json:"audio_ref,omitempty"`
}

// AudioFormat — describes the audio the timeline points into.
// Phase-one Kokoro defaults: 24 kHz mono PCM/WAV (CLAUDE.md domain rule).
type AudioFormat struct {
	SampleRate int    `json:"sample_rate"` // Hz, e.g. 24000.
	Channels   int    `json:"channels"`    // 1 = mono.
	Encoding   string `json:"encoding"`    // "pcm_s16le" | "wav".
}
