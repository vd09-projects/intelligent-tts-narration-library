package pipeline

import (
	"github.com/vd09-projects/intelligent-tts-narration-library/plan"
	"github.com/vd09-projects/intelligent-tts-narration-library/render"
	"github.com/vd09-projects/intelligent-tts-narration-library/render/rvc"
	"github.com/vd09-projects/intelligent-tts-narration-library/render/sherpa"
)

// BuildRenderer is the single engine factory the three composition roots
// (cmd/narrate, cmd/narrate-mcp, cmd/narrate-server) share to construct a
// renderer behind a user-facing voice knob — Decisions D1 + D2 (#146).
//
// rvcVoice == "" → the plain Kokoro engine plus render.DefaultFormat() (24 kHz),
// byte-identical to today's behavior for every caller that does not opt in.
// rvcVoice != "" → the render/rvc decorator wrapping Kokoro, plus
// rvc.OutputFormat() (40 kHz). The RVC target slug goes straight into
// rvc.Config.Voice; this helper translates nothing — the decorator owns the
// target→Kokoro-source map.
//
// Returning the AudioFormat ALONGSIDE the renderer is the load-bearing move
// (D1): each root hands the SAME format object to both the renderer and the
// format-validating persistent sink's WithExpectedFormat, so renderer-format and
// sink-expected-format are coupled by construction and single-origin. The sink's
// strict validation is therefore kept — it is told the expected rate explicitly
// rather than deriving it from whatever the renderer produced.
//
// Honesty rule: an unknown rvcVoice returns rvc.ErrUnsupportedVoice with a nil
// renderer — never a silent fallback to Kokoro. Roots validate membership
// eagerly (rvc.IsSupportedVoice) so in production this error is an
// unreachable-guard; it stays the construction-time backstop. Any OTHER
// construction error propagates unchanged.
//
// Home is pipeline/ (D2): three separate package-main binaries need it and
// CLAUDE.md blesses pipeline/ + cmd/ to know concrete edges. render/ cannot host
// it (render → render/sherpa → render import cycle); pipeline/ importing
// concrete sherpa + rvc creates no cycle (neither imports pipeline/). The
// planner/ + plan/ engine-neutrality invariant is unaffected and stays guarded
// by planner/deps_test.go and plan/deps_test.go.
func BuildRenderer(rvcVoice string) (render.Renderer, plan.AudioFormat, error) {
	if rvcVoice == "" {
		return sherpa.New(sherpa.EngineConfig{}), render.DefaultFormat(), nil
	}
	r, err := rvc.New(sherpa.New(sherpa.EngineConfig{}), rvc.Config{Voice: rvcVoice})
	if err != nil {
		return nil, plan.AudioFormat{}, err
	}
	return r, rvc.OutputFormat(), nil
}
