package mcpsampling

import (
	"github.com/vd09-projects/intelligent-tts-narration-library/plan"
)

// Option configures an Adapter. Pure data; no I/O is performed in any
// Option closure.

// Option mutates an Adapter during construction.
type Option func(*Adapter)

// WithClientID overrides the default "unknown" clientID. The clientID
// flows into IntelligenceResult.Model as the prefix
// "mcp-sampling@<clientID>/<actualModel>" and is the first component of
// the cache key (Phase 4). Production wiring in cmd/narrate-mcp passes
// "narrate-mcp".
func WithClientID(id string) Option {
	return func(a *Adapter) {
		if id != "" {
			a.clientID = id
		}
	}
}

// WithMaxTokens overrides the per-level MaxTokens defaults
// (L1=600, L2=1500, L3=3000). A non-positive value for any level leaves
// that level's default in place — letting callers tune only the level
// they care about.
func WithMaxTokens(l1, l2, l3 int64) Option {
	return func(a *Adapter) {
		if l1 > 0 {
			a.maxTokensL1 = l1
		}
		if l2 > 0 {
			a.maxTokensL2 = l2
		}
		if l3 > 0 {
			a.maxTokensL3 = l3
		}
	}
}

// WithTemperature overrides the default sampling temperature (0.2 — low
// to bias toward faithful summarization). The MCP SDK clamps invalid
// values upstream; we pass through.
func WithTemperature(t float64) Option {
	return func(a *Adapter) {
		a.temperature = t
	}
}

// WithPromptTemplates overrides DefaultPromptTemplates with a caller-
// supplied per-class map. Missing entries cause Voice() to refuse for
// that class with "no prompt template for class …" — the honest path,
// per CLAUDE.md.
func WithPromptTemplates(m map[plan.Class]PromptTemplate) Option {
	return func(a *Adapter) {
		if len(m) == 0 {
			return
		}
		// Copy so the caller cannot mutate the adapter's template
		// table after construction.
		dst := make(map[plan.Class]PromptTemplate, len(m))
		for k, v := range m {
			dst[k] = v
		}
		a.promptTemplates = dst
	}
}

// WithCache injects a Cache implementation. Defaults to nil — when nil,
// Voice() skips the cache wrapper entirely. Production wiring in
// cmd/narrate-mcp (Phase 5) passes NewInMemoryCache() with per-call
// lifetime.
//
// Setting WithCache also allocates the per-adapter last-known-model
// map used by the two-phase lookup (see cache.go). Without it, the
// lookup has no way to build the full CacheKey BEFORE the call.
func WithCache(c Cache) Option {
	return func(a *Adapter) {
		a.cache = c
		if c != nil && a.cacheLookup == nil {
			a.cacheLookup = newCacheLookupState()
		}
	}
}
