package pipeline

import (
	"github.com/vd09-projects/intelligent-tts-narration-library/plan"
	"github.com/vd09-projects/intelligent-tts-narration-library/render"
	"github.com/vd09-projects/intelligent-tts-narration-library/render/rvc"
	"github.com/vd09-projects/intelligent-tts-narration-library/render/sherpa"
)

// BuildRenderer is the single engine factory the three composition roots
// (cmd/narrate, cmd/narrate-mcp, cmd/narrate-server) share to construct a
// renderer behind a user-facing voice knob — Decisions D1 + D2 (#146), reworked
// onto the unified roster (#156).
//
// It looks voice up in the roster (voices.go, the single source of truth)
// instead of the old empty-vs-nonempty branch:
//   - "" → the roster default (af-bella) → the plain Kokoro engine plus
//     render.DefaultFormat() (24 kHz), byte-identical to today's no-voice path.
//   - a Kokoro slug (af-bella / am-michael) → the plain Kokoro engine + 24 kHz.
//     The engine is voice-neutral; the actual Kokoro voice flows separately via
//     NarrateRequest.Voice (RenderVoiceID), so BuildRenderer returns the SAME
//     *sherpa.Engine for every Kokoro voice.
//   - an RVC slug (cool-jahns / confident-neal) → the render/rvc decorator
//     wrapping Kokoro + rvc.OutputFormat() (40 kHz). The slug goes straight into
//     rvc.Config.Voice; this helper translates nothing — the decorator owns the
//     target→Kokoro-source map, so the RVC target→source map is NOT duplicated.
//
// Returning the AudioFormat ALONGSIDE the renderer is the load-bearing move
// (D1, signature FROZEN): each root hands the SAME format object to both the
// renderer and the format-validating persistent sink's WithExpectedFormat, so
// renderer-format and sink-expected-format are coupled by construction and
// single-origin (VoiceInfo.Format == this returned format). The sink's strict
// validation is therefore kept — told the expected rate explicitly rather than
// deriving it from whatever the renderer produced.
//
// Honesty rule: an unknown voice returns ErrUnknownVoice with a nil renderer and
// a zero format — never a silent fallback to Kokoro. Roots validate membership
// eagerly (IsVoice) so in production this error is an unreachable-guard; it stays
// the construction-time backstop (replacing #146's reliance on
// rvc.ErrUnsupportedVoice at this seam). A requires_worker voice whose worker is
// missing does NOT fail here — construction validates only the slug; the
// worker-unavailable hard stop (rvc.ErrWorkerMissing) fires at render time inside
// the decorator. Any OTHER construction error propagates unchanged.
//
// Home is pipeline/ (D2): three separate package-main binaries need it and
// CLAUDE.md blesses pipeline/ + cmd/ to know concrete edges. render/ cannot host
// it (render → render/sherpa → render import cycle); pipeline/ importing
// concrete sherpa + rvc creates no cycle (neither imports pipeline/). The
// planner/ + plan/ engine-neutrality invariant is unaffected.
func BuildRenderer(voice string) (render.Renderer, plan.AudioFormat, error) {
	info, err := ResolveVoice(voice)
	if err != nil {
		return nil, plan.AudioFormat{}, err
	}
	if info.Engine == EngineRVC {
		r, rerr := rvc.New(sherpa.New(sherpa.EngineConfig{}), rvc.Config{Voice: info.Slug})
		if rerr != nil {
			return nil, plan.AudioFormat{}, rerr
		}
		return r, info.Format, nil
	}
	return sherpa.New(sherpa.EngineConfig{}), info.Format, nil
}
