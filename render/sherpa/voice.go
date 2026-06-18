package sherpa

import (
	"github.com/vd09-projects/intelligent-tts-narration-library/plan"
	"github.com/vd09-projects/intelligent-tts-narration-library/render"
)

// supportedVoices — Kokoro voice ids wired for phase one. Keep in sync with
// scripts/kokoro_runner.py's SUPPORTED_VOICES set and render/sherpa/README.md.
var supportedVoices = map[string]struct{}{
	"af_bella":   {},
	"am_michael": {},
}

// defaultVoice is the phase-one default (female), per CLAUDE.md domain rules.
const defaultVoice = "af_bella"

// resolveVoice picks the engine voice id for a render call.
//
// Order (per planner-task.md Decision 5):
//  1. opts.Voice (engine voice id) — used as-is when non-empty AND known.
//  2. p.Defaults.Voice (engine-neutral hint) — used when it maps to a known id.
//  3. defaultVoice — fallback. Never errors on unknown PlanDefaults.Voice;
//     errors only when opts.Voice is non-empty AND unknown (caller bug).
//
// Returns ("", ErrUnsupportedVoice) only for case (1) failure. PlanDefaults
// is a hint — unknown hints fall through silently to defaultVoice.
func resolveVoice(p plan.NarrationPlan, opts render.RenderOptions) (string, error) {
	if opts.Voice != "" {
		if _, ok := supportedVoices[opts.Voice]; ok {
			return opts.Voice, nil
		}
		return "", ErrUnsupportedVoice
	}
	if hint := p.Defaults.Voice; hint != "" {
		if _, ok := supportedVoices[hint]; ok {
			return hint, nil
		}
		// Unknown hint — fall through to default. Never error.
	}
	return defaultVoice, nil
}
