package pipeline

// voices.go — the unified named-voice roster (#156). ONE ordered roster spans
// both engines: af-bella / am-michael (Kokoro, 24 kHz) and cool-jahns /
// confident-neal (RVC, 40 kHz). It is the single source of truth BuildRenderer
// reads to pick engine + format, and the resolver every composition root
// (cmd/narrate, cmd/narrate-mcp, cmd/narrate-server) consults to turn a
// user-facing --voice / voice / --gender knob into a renderer, a NarrateRequest
// voice hint, a manifest provenance id, and an expected sink format.
//
// Slug convention (D-A): user-facing slugs are lowercase-hyphenated
// (af-bella, am-michael, cool-jahns, confident-neal); the underscore ENGINE ids
// (af_bella, am_michael) stay internal. The hyphen<->underscore reconciliation
// lives in EXACTLY ONE place — a Kokoro entry's KokoroVoiceID field. The RVC
// target->Kokoro-source map is NOT duplicated here: it stays the render/rvc
// decorator's private business, so an RVC entry's KokoroVoiceID is a MEANINGFUL
// empty (BuildRenderer passes the slug straight into rvc.Config.Voice).
//
// Home is pipeline/ (D2): CLAUDE.md blesses pipeline/ + cmd/ to know concrete
// engines, and pipeline/ already imports concrete sherpa + rvc. It cannot live
// in render/ (render -> render/sherpa cycle; render must not import its sibling
// render/rvc). planner/ and plan/ stay engine-neutral — no roster slug ever
// reaches the plan schema or Timeline.

import (
	"errors"
	"fmt"
	"strings"

	"github.com/vd09-projects/intelligent-tts-narration-library/plan"
	"github.com/vd09-projects/intelligent-tts-narration-library/render"
	"github.com/vd09-projects/intelligent-tts-narration-library/render/gptsovits"
	"github.com/vd09-projects/intelligent-tts-narration-library/render/rvc"
)

// VoiceEngine identifies which rendering engine backs a roster voice. Exported so
// VoiceInfo.Engine is fully usable by the composition roots; the format + worker
// facts most roots need are also surfaced directly on VoiceInfo (Format,
// RequiresWorker) so switching on the engine is rarely necessary.
type VoiceEngine int

const (
	// EngineKokoro is the plain Kokoro/sherpa engine (24 kHz mono).
	EngineKokoro VoiceEngine = iota
	// EngineRVC is the render/rvc decorator over Kokoro (40 kHz mono, needs the
	// RVC worker subprocess).
	EngineRVC
	// EngineGSO is the render/gptsovits PEER engine (GPT-SoVITS, 32 kHz mono, needs
	// the GSO worker subprocess). Unlike EngineRVC it wraps nothing — it is a source
	// engine like Kokoro, and like Kokoro it is voice-neutral (the slug flows via
	// RenderVoiceID, not bound at construction).
	EngineGSO
)

// label is the human-facing engine tag used in --voice help text.
func (e VoiceEngine) label() string {
	switch e {
	case EngineRVC:
		return "RVC"
	case EngineGSO:
		return "GPT-SoVITS"
	default:
		return "Kokoro"
	}
}

// voiceEntry is one row of the unified roster.
type voiceEntry struct {
	// Slug is the user-facing lowercase-hyphenated selector (af-bella,
	// cool-jahns, ...).
	Slug string
	// Engine picks the renderer + derives the format (24 kHz Kokoro / 40 kHz RVC).
	Engine VoiceEngine
	// KokoroVoiceID is the underscore engine id for a Kokoro entry (af_bella,
	// am_michael) — the single hyphen<->underscore reconciliation point. For an
	// RVC entry this is the empty string: a MEANINGFUL empty, not a missing value.
	// The render/rvc decorator owns the RVC target->Kokoro-source map, so a Kokoro
	// id here would be a lie; BuildRenderer passes the RVC slug straight into
	// rvc.Config.Voice and the decorator paints its own source.
	KokoroVoiceID string
	// RequiresWorker is true for an RVC entry (Engine == EngineRVC). It is stored
	// explicitly, not recomputed at each read site, so help-text tagging and the
	// --listen re-key read one field. It drives help text + the --listen 40 kHz
	// re-key ONLY — never an up-front worker-liveness probe (the worker-missing
	// hard stop fires at render time inside the rvc decorator, ErrWorkerMissing).
	RequiresWorker bool
}

// roster is the ordered, single-source-of-truth voice list. Order is stable so
// help text and VoiceSlugs() are deterministic. Adding a voice = one entry here
// (+ one render/rvc.rvcVoices entry for an RVC voice — guarded by the
// roster<->rvc consistency test).
var roster = []voiceEntry{
	{Slug: "af-bella", Engine: EngineKokoro, KokoroVoiceID: "af_bella", RequiresWorker: false},
	{Slug: "am-michael", Engine: EngineKokoro, KokoroVoiceID: "am_michael", RequiresWorker: false},
	{Slug: "cool-jahns", Engine: EngineRVC, KokoroVoiceID: "", RequiresWorker: true},
	{Slug: "confident-neal", Engine: EngineRVC, KokoroVoiceID: "", RequiresWorker: true},
	// GSO peer engine (#162): voice-neutral like Kokoro, so KokoroVoiceID reuses the
	// field as the engine-native voice id (the slug the gptsovits engine resolves
	// from RenderOptions.Voice) — NOT a Kokoro id and NOT the meaningful empty RVC uses.
	{Slug: "cool-jahns-gso", Engine: EngineGSO, KokoroVoiceID: "cool-jahns-gso", RequiresWorker: true},
}

// defaultVoiceSlug is the phase-one default (female), byte-identical to today's
// behavior for callers that select no voice: ResolveVoice("") resolves here, and
// its ManifestVoice is af_bella — the value existing --gender=female persistent
// renders already stamp.
const defaultVoiceSlug = "af-bella"

// rosterIndex is the slug->entry lookup built once from roster.
var rosterIndex = func() map[string]voiceEntry {
	m := make(map[string]voiceEntry, len(roster))
	for _, e := range roster {
		m[e.Slug] = e
	}
	return m
}()

// genderSlug is the deprecated --gender alias, centralized here (D-B, OQ-2):
// female->af-bella, male->am-michael. It replaces the three per-root
// genderToVoice maps. The alias resolves to a SLUG which then flows through
// ResolveVoice, so female -> af-bella -> RenderVoiceID af_bella stays
// byte-identical to the old genderToVoice["female"] == "af_bella".
var genderSlug = map[string]string{
	"female": "af-bella",
	"male":   "am-michael",
}

// ErrUnknownVoice is the pipeline-seam sentinel for a voice that is not in the
// roster. Roots validate membership eagerly (IsVoice) so this is the
// BuildRenderer backstop; it replaces reliance on rvc.ErrUnsupportedVoice at the
// pipeline seam (that sentinel stays the rvc decorator's internal backstop).
var ErrUnknownVoice = errors.New("unknown voice")

// Deprecation notice messages, pinned ONCE here (D-C, S7) and emitted verbatim by
// the CLI and server so exact-string tests assert the constant, not a guessed
// literal. #146's RVC-specific "(RVC selects the Kokoro source)" parenthetical is
// retired — it is false for a Kokoro --voice.
const (
	// NoticeGenderDeprecated is emitted when only --gender is set: steer to --voice.
	NoticeGenderDeprecated = "--gender is deprecated; use --voice instead (female → af-bella, male → am-michael)"
	// NoticeGenderIgnored is emitted when BOTH --gender and --voice are set:
	// --voice wins.
	NoticeGenderIgnored = "--gender ignored because --voice is set (--voice is the primary selector)"
)

// VoiceInfo is what a composition root needs to wire a voice BEYOND the renderer.
type VoiceInfo struct {
	// Slug is the resolved roster slug (the default af-bella for an empty request).
	Slug string
	// Engine backs the voice (Kokoro / RVC).
	Engine VoiceEngine
	// RenderVoiceID is the engine-native id a root threads into
	// NarrateRequest.Voice (-> RenderOptions.Voice). For a Kokoro voice this is the
	// underscore engine id (af_bella / am_michael). For an RVC voice it is the
	// empty string: a MEANINGFUL empty — the rvc decorator overrides the Kokoro
	// source, so any id here would be discarded.
	RenderVoiceID string
	// ManifestVoice is the engine-native provenance id recorded in manifest.voice
	// (D-D): the Kokoro engine id for a Kokoro voice (af_bella / am_michael, so the
	// --gender alias path and the explicit --voice path stamp the SAME value), and
	// the RVC slug for an RVC voice (the character voice the user hears).
	ManifestVoice string
	// Format is the sink's expected audio format, derived from Engine
	// (Kokoro -> 24 kHz, RVC -> 40 kHz). Single-origin with BuildRenderer's
	// returned format — both come from this roster resolution, so
	// sink-expected-format cannot drift from renderer-format.
	Format plan.AudioFormat
	// RequiresWorker mirrors the entry: true for RVC voices.
	RequiresWorker bool
}

// format derives the entry's audio format from its engine — never a copied
// 24000/40000 literal, so the format SoT stays in render / render/rvc.
func (e voiceEntry) format() plan.AudioFormat {
	switch e.Engine {
	case EngineRVC:
		return rvc.OutputFormat()
	case EngineGSO:
		return gptsovits.OutputFormat()
	default:
		return render.DefaultFormat()
	}
}

// renderVoiceID is the NarrateRequest.Voice hint: the Kokoro engine id for a
// Kokoro voice; the meaningful empty for an RVC voice (decorator overrides it).
func (e voiceEntry) renderVoiceID() string {
	if e.Engine == EngineRVC {
		return ""
	}
	return e.KokoroVoiceID
}

// manifestVoice is the manifest.voice provenance id (D-D): the Kokoro engine id
// for a Kokoro voice; the slug (the character voice the user hears) for an RVC or
// GSO voice.
func (e voiceEntry) manifestVoice() string {
	if e.Engine == EngineRVC || e.Engine == EngineGSO {
		return e.Slug
	}
	return e.KokoroVoiceID
}

// info projects an entry into the root-facing VoiceInfo.
func (e voiceEntry) info() VoiceInfo {
	return VoiceInfo{
		Slug:           e.Slug,
		Engine:         e.Engine,
		RenderVoiceID:  e.renderVoiceID(),
		ManifestVoice:  e.manifestVoice(),
		Format:         e.format(),
		RequiresWorker: e.RequiresWorker,
	}
}

// tags is the help-text descriptor for one entry, e.g. "Kokoro · 24kHz · fast"
// or "RVC · 40kHz · needs worker". The rate is derived from format (single SoT).
func (e voiceEntry) tags() string {
	worker := "fast"
	if e.RequiresWorker {
		worker = "needs worker"
	}
	return fmt.Sprintf("%s · %dkHz · %s", e.Engine.label(), e.format().SampleRate/1000, worker)
}

// ResolveVoice turns a roster slug into the VoiceInfo a root needs. An empty
// voice resolves to the default (af-bella) — byte-identical to today's
// no-voice-selected behavior. An unknown voice returns ErrUnknownVoice with a
// tagged-roster message and a zero VoiceInfo (never a silent fallback).
func ResolveVoice(voice string) (VoiceInfo, error) {
	slug := voice
	if slug == "" {
		slug = defaultVoiceSlug
	}
	e, ok := rosterIndex[slug]
	if !ok {
		return VoiceInfo{}, fmt.Errorf("%w: %q (choose one of: %s)", ErrUnknownVoice, voice, VoiceHelp())
	}
	return e.info(), nil
}

// IsVoice reports whether slug is a known roster voice. Roots call it to validate
// a user-supplied --voice / voice EAGERLY — a fast caller-error before any
// temp-dir or render work (F1 error class 1). An empty string is NOT a member
// (roots guard with a non-empty check; "" means "no voice selected", handled by
// ResolveVoice's default).
func IsVoice(slug string) bool {
	_, ok := rosterIndex[slug]
	return ok
}

// VoiceSlugs returns the roster slugs in stable order (for tests + terse listings).
func VoiceSlugs() []string {
	out := make([]string, 0, len(roster))
	for _, e := range roster {
		out = append(out, e.Slug)
	}
	return out
}

// VoiceHelp is the tagged, engine-honest --voice help string (SC3), e.g.
// "af-bella (Kokoro · 24kHz · fast) | ... | cool-jahns (RVC · 40kHz · needs
// worker)". Roots use it for flag help + unknown-voice error messages; it
// replaces the undifferentiated strings.Join(rvc.SupportedVoices()…) list.
func VoiceHelp() string {
	parts := make([]string, 0, len(roster))
	for _, e := range roster {
		parts = append(parts, fmt.Sprintf("%s (%s)", e.Slug, e.tags()))
	}
	return strings.Join(parts, " | ")
}

// SlugForGender maps a deprecated --gender value to its roster slug
// (female->af-bella, male->am-michael). ok is false for an unknown gender, so a
// root can reuse it as the --gender enum validator. This is the centralized
// replacement for the per-root genderToVoice maps (behavior preserved, not
// deleted — D-B).
func SlugForGender(gender string) (slug string, ok bool) {
	slug, ok = genderSlug[gender]
	return slug, ok
}
