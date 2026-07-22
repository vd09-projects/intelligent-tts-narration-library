package rvc

import (
	"fmt"
	"sort"
)

// voiceEntry is the single-source-of-truth translation for one RVC target slug:
// the Kokoro *source* voice the RVC model was trained against, plus the per-voice
// worker knobs (index_rate on the request line, pitch always 0 in phase one).
type voiceEntry struct {
	// source is the Kokoro voice id the inner renderer must produce (the RVC
	// model's training source). The decorator overrides RenderOptions.Voice to
	// this before calling inner — the exactly-once translation of Decision 5.
	source string
	// indexRate is emitted verbatim as the <index_rate> request token.
	indexRate float64
	// pitch is emitted verbatim as the <pitch> request token — always 0.
	pitch int
}

// rvcVoices is the full target → {Kokoro source, index_rate, pitch} map, owned by
// the decorator per Locked Decision 5. This is the ONLY place an RVC target slug
// is translated; sherpa's resolveVoice therefore only ever sees {af_bella,
// am_michael}, never an RVC target. The index_rate values mirror the rvc-convert
// Makefile per-voice defaults (cool-jahns 0.75 / confident-neal 0.5) and must stay
// in sync with them.
var rvcVoices = map[string]voiceEntry{
	"cool-jahns":     {source: "am_michael", indexRate: 0.75, pitch: 0},
	"confident-neal": {source: "af_bella", indexRate: 0.5, pitch: 0},
}

// resolveVoice maps an RVC target slug to its voiceEntry. An unknown slug is a
// caller/config bug (ErrUnsupportedVoice), distinct from a worker bad-voice ERR
// (which would signal a decorator bug in request-line construction).
func resolveVoice(target string) (voiceEntry, error) {
	v, ok := rvcVoices[target]
	if !ok {
		return voiceEntry{}, fmt.Errorf("%w: %q (roster: cool-jahns, confident-neal)", ErrUnsupportedVoice, target)
	}
	return v, nil
}

// IsSupportedVoice reports whether slug is a known RVC target voice. The
// composition roots (#146) use it to validate a --voice / voice argument
// EAGERLY — a fast caller-error before any temp-dir or render work — without
// duplicating the target→Kokoro-source translation (that stays owned here, the
// single translation place). Membership ≠ translation: this exposes only which
// slugs exist, never how they map.
func IsSupportedVoice(slug string) bool {
	_, ok := rvcVoices[slug]
	return ok
}

// SupportedVoices returns the RVC target roster, sorted, for error messages and
// CLI/MCP help text. It exposes the set of valid slugs — never the Kokoro
// sources they map to, which remain the decorator's private business.
func SupportedVoices() []string {
	out := make([]string, 0, len(rvcVoices))
	for slug := range rvcVoices {
		out = append(out, slug)
	}
	sort.Strings(out)
	return out
}
