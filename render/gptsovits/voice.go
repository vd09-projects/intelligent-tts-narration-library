package gptsovits

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/vd09-projects/intelligent-tts-narration-library/plan"
	"github.com/vd09-projects/intelligent-tts-narration-library/render"
)

// gsoFormat is the GSO output format: 32 kHz mono s16le — GPT-SoVITS v2Pro's
// native rate. It is a THIRD system output rate (Kokoro 24 kHz, RVC 40 kHz,
// GSO 32 kHz); CLAUDE.md forbids resampling, so the roster/format metadata carries
// a per-engine rate and 32 kHz is never assumed away.
func gsoFormat() plan.AudioFormat {
	return plan.AudioFormat{
		SampleRate: 32000,
		Channels:   1,
		Encoding:   "pcm_s16le",
	}
}

// OutputFormat is the exported GSO output format (32 kHz mono s16le). It is the
// SINGLE source of truth for this engine's rate: RenderResult.Format,
// BlockRender.Format, Timeline.Format, the pipeline roster's VoiceInfo.Format, and
// the format handed to the persistent sink's WithExpectedFormat all originate here.
func OutputFormat() plan.AudioFormat {
	return gsoFormat()
}

// gsoVoices is the GSO roster — the set of known voice slugs. cool-jahns-gso is the
// only phase-one voice (multi-GSO-voice is out of scope, #162). The per-voice
// conditioning (ref audio + prompt transcript) is loaded from the packaged
// artifacts at resolve time (loadVoice), not baked here.
var gsoVoices = map[string]struct{}{
	"cool-jahns-gso": {},
}

// defaultVoiceSlug is the phase-one default GSO voice (Open Question 1: the engine
// is voice-neutral and resolves opts.Voice, defaulting here).
const defaultVoiceSlug = "cool-jahns-gso"

// voiceEntry is the resolved wire conditioning for one GSO voice: the two
// authoritative-on-the-wire tokens the worker needs to synthesize from scratch.
type voiceEntry struct {
	// slug is the resolved roster voice id (the <voice_id> wire token).
	slug string
	// refAudioPath is the absolute path to the packaged reference clip (the
	// <ref_audio_path> wire token). NOT stat'd here — the worker owns read-failed
	// and the wire-vs-packaged drift classification (D3: wire authoritative).
	refAudioPath string
	// promptText is the packaged reference transcript (the <prompt_text> wire
	// token), read from ref_transcript.txt (D3 source-of-record / canonical value).
	promptText string
}

// resolveVoice picks the GSO voice for a render call and loads its packaged
// conditioning. Selection mirrors render/sherpa's peer shape (Decision 5 / OQ1):
//  1. opts.Voice (roster slug) — used when non-empty AND known; errors if unknown.
//  2. p.Defaults.Voice (engine-neutral hint) — used only if it maps to a known slug.
//  3. defaultVoiceSlug — fallback. Never errors on an unknown PlanDefaults hint.
func resolveVoice(modelsRoot string, p plan.NarrationPlan, opts render.RenderOptions) (voiceEntry, error) {
	slug, err := selectVoiceSlug(p, opts)
	if err != nil {
		return voiceEntry{}, err
	}
	return loadVoice(modelsRoot, slug)
}

// selectVoiceSlug resolves the slug only (no I/O), sherpa-style. Returns
// ErrUnsupportedVoice only for case (1) — a non-empty, unknown opts.Voice.
func selectVoiceSlug(p plan.NarrationPlan, opts render.RenderOptions) (string, error) {
	if opts.Voice != "" {
		if _, ok := gsoVoices[opts.Voice]; ok {
			return opts.Voice, nil
		}
		return "", fmt.Errorf("%w: %q (roster: %s)", ErrUnsupportedVoice, opts.Voice, strings.Join(SupportedVoices(), ", "))
	}
	if hint := p.Defaults.Voice; hint != "" {
		if _, ok := gsoVoices[hint]; ok {
			return hint, nil
		}
		// Unknown engine-neutral hint — fall through to the default. Never error.
	}
	return defaultVoiceSlug, nil
}

// loadVoice reads the packaged per-voice artifacts and builds the wire tokens
// (D3). The engine emits the packaged canonical <ref_audio_path>/<prompt_text> as
// wire tokens; the worker owns any drift classification (wire wins — no duplicate
// Go-side guardrail, OQ3). A missing/empty transcript is a config fault
// (ErrUnsupportedVoice), surfaced before any subprocess spawn.
func loadVoice(modelsRoot, slug string) (voiceEntry, error) {
	dir := filepath.Join(modelsRoot, slug)
	transcriptPath := filepath.Join(dir, "ref_transcript.txt")

	raw, err := os.ReadFile(transcriptPath)
	if err != nil {
		return voiceEntry{}, fmt.Errorf("%w: reading packaged transcript for %q: %w", ErrUnsupportedVoice, slug, err)
	}
	prompt := strings.TrimSpace(string(raw))
	if prompt == "" {
		return voiceEntry{}, fmt.Errorf("%w: packaged transcript for %q is empty: %s", ErrUnsupportedVoice, slug, transcriptPath)
	}

	// Absolute ref path so the worker resolves the same file regardless of its own
	// CWD; existence/drift is the worker's job (read-failed / bad-args), not ours.
	refAudioPath := filepath.Join(dir, "ref_audio.wav")
	if abs, aerr := filepath.Abs(refAudioPath); aerr == nil {
		refAudioPath = abs
	}

	return voiceEntry{slug: slug, refAudioPath: refAudioPath, promptText: prompt}, nil
}

// IsSupportedVoice reports whether slug is a known GSO roster voice. The pipeline
// roster<->engine consistency test uses it as the single membership authority.
func IsSupportedVoice(slug string) bool {
	_, ok := gsoVoices[slug]
	return ok
}

// SupportedVoices returns the GSO roster slugs, sorted, for error messages and the
// roster<->engine consistency guard.
func SupportedVoices() []string {
	out := make([]string, 0, len(gsoVoices))
	for slug := range gsoVoices {
		out = append(out, slug)
	}
	sort.Strings(out)
	return out
}
