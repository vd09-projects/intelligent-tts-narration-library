// Voicing — turn raw source text into final spoken words. This file
// owns the default symbol / path / acronym lexicon and the longest-match
// replacement engine that consumes it.
//
// Voicing happens in the planner (CLAUDE.md invariant), not in the
// renderer. Segment.Text emitted downstream is the literal words to say.

package planner

import (
	"sort"
	"strings"
	"time"
)

// Lexicon — user-facing override map. Keys are verbatim source tokens
// (e.g. "->", "/etc/hosts", "K8s"); values are the spoken-word form
// (e.g. "arrow", "etc hosts", "kubernetes").
//
// Override mechanism: WithLexicon(extra Lexicon) overlays extra on top
// of DefaultLexicon — user entries win on key collision. Use this to
// add project-specific acronyms (e.g. "MCP" → "model context protocol")
// or to replace a default the project disagrees with.
type Lexicon map[string]string

// DefaultLexicon — shipped baseline. Three groups:
//   - arrow / comparison operators
//   - common system paths
//   - dev acronyms
//
// Decision: convention — ship a frozen DefaultLexicon and let users
// overlay project-specific entries via WithLexicon. Why: the alternative
// (ship empty, force every project to define basics) produces silent
// silly voicings on day one (`->` literally read as "dash greater
// than"). The baseline is opinionated but small and replaceable.
var DefaultLexicon = Lexicon{
	// Arrow / comparison operators. Longer keys first when sorted by
	// length (compileLexicon does this) so "==" beats "=" — see test.
	"->": "arrow",
	"=>": "fat arrow",
	"<-": "left arrow",
	"==": "is equal to",
	"!=": "is not equal to",
	"<=": "less than or equal to",
	">=": "greater than or equal to",
	"&&": "and",
	"||": "or",

	// Common system paths.
	"/etc/hosts":      "etc hosts",
	"/usr/local/bin":  "user local bin",
	"~/.ssh/config":   "home dot ssh config",
	"/var/log":        "var log",
	"/dev/null":       "dev null",

	// Dev acronyms (kept short — voiceable, not exhaustive).
	"API":   "A P I",
	"URL":   "U R L",
	"URI":   "U R I",
	"HTTP":  "H T T P",
	"HTTPS": "H T T P S",
	"JSON":  "jay son",
	"YAML":  "yamel",
	"SQL":   "sequel",
	"CLI":   "C L I",
	"TTS":   "text to speech",
	"MCP":   "M C P",
	"gRPC":  "G R P C",
	"K8s":   "kubernetes",
	"CI":    "C I",
	"CD":    "C D",
}

// VoiceOption — functional option threaded through Plan(). Today the
// only option is WithLexicon; the option type lets us add more without
// breaking the Plan signature (e.g. WithLocale, WithEmphasisRule).
type VoiceOption func(*voiceOptions)

type voiceOptions struct {
	extra Lexicon
	// clock / planID — per-call seams for deterministic tests. nil means
	// "use the wall-clock / real-ULID default", resolved inside Plan().
	// Carried on the existing VoiceOption channel so the planner stays
	// pure and stateless — no package-global swap, hence no data race
	// when parallel tests install distinct seams.
	clock  func() time.Time
	planID func() string
	// concurrencyLimit — max in-flight intel.Voice calls during the
	// Voice pass. 0 or negative means "use defaultIntelligenceConcurrency".
	// Set via WithMaxIntelligenceConcurrency. limit=1 yields a fully
	// serial fan-out — the sequential test oracle.
	concurrencyLimit int
}

// defaultIntelligenceConcurrency bounds the concurrent intel.Voice
// fan-out during the Voice pass when the caller has not set a limit.
// Chosen as 4: high enough to overlap several LLM round-trips, low
// enough to stay polite to the Anthropic API and avoid burst 429s on a
// hobby key.
const defaultIntelligenceConcurrency = 4

// WithMaxIntelligenceConcurrency — cap the number of concurrent
// intel.Voice calls the planner's Voice pass issues. n <= 0 (or unset)
// falls back to defaultIntelligenceConcurrency. n == 1 makes the Voice
// pass fully serial — the sequential oracle used by the determinism
// tests.
func WithMaxIntelligenceConcurrency(n int) VoiceOption {
	return func(o *voiceOptions) {
		o.concurrencyLimit = n
	}
}

// WithLexicon — overlay extra on top of DefaultLexicon. User entries
// win on key collision. Calling WithLexicon multiple times merges all
// extras; later calls win on collision.
func WithLexicon(extra Lexicon) VoiceOption {
	return func(o *voiceOptions) {
		if o.extra == nil {
			o.extra = make(Lexicon, len(extra))
		}
		for k, v := range extra {
			o.extra[k] = v
		}
	}
}

// withClock — test-only seam: override Plan()'s clock with a fixed
// function. Unexported on purpose — production callers never set it; the
// wall-clock default lives inside Plan(). Carried per-call so concurrent
// Plan() runs each observe their own clock with no shared mutable state.
func withClock(now func() time.Time) VoiceOption {
	return func(o *voiceOptions) {
		o.clock = now
	}
}

// withPlanID — test-only seam: override Plan()'s plan-id generator.
// plan.NewPlanID returns a plain string (there is no plan.PlanID type),
// so the seam is func() string. Unexported; the real-ULID default lives
// inside Plan(). Carried per-call — no package global, no race.
func withPlanID(newID func() string) VoiceOption {
	return func(o *voiceOptions) {
		o.planID = newID
	}
}

// resolveVoiceOptions — parse opts once into a voiceOptions value. Both
// the compiled lexicon and the clock/planID seams are surfaced from a
// single parse so Plan() can read them as locals. This is the seam that
// lets Plan() thread clock/planID per-call instead of through (removed)
// package globals.
func resolveVoiceOptions(opts ...VoiceOption) *voiceOptions {
	cfg := &voiceOptions{}
	for _, opt := range opts {
		opt(cfg)
	}
	return cfg
}

// compiledLex — precomputed lexicon: keys sorted by length descending so
// longest-match wins (e.g. ">=" beats ">").
type compiledLex struct {
	keys []string
	repl map[string]string
}

// compileLexicon — merge DefaultLexicon with any extras and sort keys
// for longest-match replacement.
//
// Empty inputs yield a non-nil compiledLex with an empty key slice — the
// caller can always invoke voice() without nil-checking. Variadic form
// kept for the many test call sites; Plan() uses compileLexiconCfg with
// the single parsed config so it surfaces the clock/planID seams too.
func compileLexicon(opts ...VoiceOption) *compiledLex {
	return compileLexiconCfg(resolveVoiceOptions(opts...))
}

// compileLexiconCfg — build the compiled lexicon from an already-parsed
// voiceOptions. Lets Plan() parse opts exactly once (surfacing
// clock/planID alongside the lexicon).
func compileLexiconCfg(cfg *voiceOptions) *compiledLex {
	merged := make(Lexicon, len(DefaultLexicon)+len(cfg.extra))
	for k, v := range DefaultLexicon {
		merged[k] = v
	}
	for k, v := range cfg.extra {
		merged[k] = v // user wins.
	}

	keys := make([]string, 0, len(merged))
	for k := range merged {
		keys = append(keys, k)
	}
	// Longest first; tie-break alphabetically for determinism.
	sort.Slice(keys, func(i, j int) bool {
		if len(keys[i]) != len(keys[j]) {
			return len(keys[i]) > len(keys[j])
		}
		return keys[i] < keys[j]
	})
	return &compiledLex{keys: keys, repl: merged}
}

// voice — apply the compiled lexicon to text. Longest-match-first
// left-to-right scan. Tokens are matched verbatim (case-sensitive) so
// "K8s" maps but "k8s" does not — keep the lexicon authoritative.
//
// Whitespace handling: replacements get a single space inserted on
// either side when the replacement would otherwise butt up against a
// non-space character. This prevents "->b" rendering as "arrowb".
// Replacements at the start/end of the input do not get leading/
// trailing space.
func voice(text string, lex *compiledLex) string {
	if lex == nil || len(lex.keys) == 0 || text == "" {
		return text
	}
	var out strings.Builder
	out.Grow(len(text))

	i := 0
	for i < len(text) {
		matched := false
		for _, k := range lex.keys {
			if strings.HasPrefix(text[i:], k) {
				// Pad with a space if the previous output char is
				// non-space and the replacement is non-empty.
				if out.Len() > 0 {
					last := out.String()[out.Len()-1]
					if last != ' ' && last != '\n' && last != '\t' {
						out.WriteByte(' ')
					}
				}
				out.WriteString(lex.repl[k])
				i += len(k)
				// Pad with a space if the next source char is
				// non-space.
				if i < len(text) {
					next := text[i]
					if next != ' ' && next != '\n' && next != '\t' &&
						next != '.' && next != ',' && next != ';' &&
						next != ':' && next != ')' && next != ']' {
						out.WriteByte(' ')
					}
				}
				matched = true
				break
			}
		}
		if !matched {
			out.WriteByte(text[i])
			i++
		}
	}
	return out.String()
}
