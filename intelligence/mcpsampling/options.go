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

// WithCache injects a plain Cache implementation (the unbounded
// inMemoryCache tier — tests / per-call use). Defaults to nil; when nil,
// Voice() skips the cache wrapper entirely.
//
// This is the LEGACY / per-call path: it allocates a per-Adapter
// last-known-model map (cacheLookupState) for the two-phase lookup.
// That per-call last-known map is exactly what issue #25 proved cannot
// support cross-call hits — a fresh Adapter per tool call starts empty
// and misses though the answer sits in a shared cache. Production wiring
// must use WithServerCache instead, which shares the last-known map at
// server lifetime.
func WithCache(c Cache) Option {
	return func(a *Adapter) {
		a.cache = c
		if c != nil && a.lastKnownStore == nil {
			a.lastKnownStore = newCacheLookupState()
		}
	}
}

// WithServerCache injects the server-lifetime ServerCache handle. This
// is the PRODUCTION path: the same *ServerCache supplies BOTH the
// bounded LRU (as the Cache) AND the shared last-known-model map (as the
// lastKnownStore). Because every Adapter built per tool call shares one
// handle, a second Voice() for an already-cached key hits without a
// client call — the cross-call fix issue #25 exists for. It deliberately
// does NOT allocate a per-call cacheLookupState; reintroducing one would
// re-break cross-call hits.
//
// A nil handle is a no-op (caching stays off), matching WithCache(nil).
func WithServerCache(sc *ServerCache) Option {
	return func(a *Adapter) {
		if sc == nil {
			return
		}
		a.cache = sc
		a.lastKnownStore = sc
	}
}
