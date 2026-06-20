package anthropic

import (
	"context"
	"net/http"
	"time"

	"github.com/vd09-projects/intelligent-tts-narration-library/internal/intelligencetmpl"
	"github.com/vd09-projects/intelligent-tts-narration-library/plan"
)

// Option configures an Adapter. Pure data; no I/O is performed in any
// Option closure. Apply order is caller-controlled — last write wins.
type Option func(*Adapter)

// WithAPIKey sets the Anthropic API key. Required — New returns an error
// when the resolved key is empty. The caller is responsible for reading
// the value from the environment (typically ANTHROPIC_API_KEY); this
// package does not touch os.Getenv to keep the package free of process-
// environment coupling for tests.
func WithAPIKey(key string) Option {
	return func(a *Adapter) {
		a.apiKey = key
	}
}

// WithModel overrides the default model (claude-haiku-4-5). Empty string
// is rejected at New time so callers cannot accidentally unset it. The
// model id is sent verbatim to the API and stamped into
// IntelligenceResult.Model as "anthropic@<actualModel>" (see Phase 3).
func WithModel(model string) Option {
	return func(a *Adapter) {
		if model != "" {
			a.model = model
		}
	}
}

// WithHTTPClient injects an *http.Client. The default is http.DefaultClient.
// Tests use this seam to substitute a roundTripFunc-backed client so the
// suite runs without network access (see anthropic_test.go in Phase 3).
// Passing nil is silently ignored — New restores the default to avoid a
// nil-deref on the first Do().
func WithHTTPClient(c *http.Client) Option {
	return func(a *Adapter) {
		if c != nil {
			a.httpClient = c
		}
	}
}

// WithMaxTokens overrides the per-level MaxTokens defaults
// (L1=80, L2=240, L3=600). Only entries supplied in the caller's map
// override; unspecified levels keep their defaults. A non-positive value
// for any level leaves that level's default in place — letting callers
// tune only the level they care about, matching mcpsampling's semantics.
func WithMaxTokens(m map[plan.Level]int) Option {
	return func(a *Adapter) {
		for level, n := range m {
			if n > 0 {
				a.maxTokens[level] = n
			}
		}
	}
}

// WithTemperature overrides the default sampling temperature (0.3 — low
// to bias toward faithful summarization). The Anthropic API rejects
// out-of-range values upstream; we pass through.
func WithTemperature(t float64) Option {
	return func(a *Adapter) {
		a.temperature = t
	}
}

// WithPromptTemplates overrides intelligencetmpl.DefaultPromptTemplates
// with a caller-supplied per-class map. Missing entries cause Voice() to
// refuse for that class with "no prompt template for class …" — the
// honest path, per CLAUDE.md. The map is copied so the caller cannot
// mutate the adapter's template table after construction.
func WithPromptTemplates(m map[plan.Class]intelligencetmpl.PromptTemplate) Option {
	return func(a *Adapter) {
		if len(m) == 0 {
			return
		}
		dst := make(map[plan.Class]intelligencetmpl.PromptTemplate, len(m))
		for k, v := range m {
			dst[k] = v
		}
		a.promptTemplates = dst
	}
}

// WithSleeper injects the sleep function doWithRetry uses between 429
// retries. The default (defaultSleeper in anthropic.go) does
// time.After + ctx select. Tests pass an instant-returning closure so
// the suite does not actually wait seconds. A nil sleeper is ignored
// (the default stays in place). Per Decision v5: the sleeper is the
// retry test seam — the retry semantics (cap, exponential, header
// parsing) live in doWithRetry where they can be unit-tested via
// captured durations.
func WithSleeper(s func(context.Context, time.Duration) error) Option {
	return func(a *Adapter) {
		if s != nil {
			a.sleeper = s
		}
	}
}

// WithCache injects a Cache implementation. Defaults to nil — when nil,
// Voice() (Phase 4) skips the cache wrapper entirely. Production wiring
// in cmd/narrate (Phase 6) passes NewInMemoryCache() with per-call
// lifetime. nil is explicitly allowed at construction so callers can opt
// out without a separate option.
func WithCache(c Cache) Option {
	return func(a *Adapter) {
		a.cache = c
	}
}
