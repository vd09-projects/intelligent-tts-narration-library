package mcpsampling

import (
	"github.com/vd09-projects/intelligent-tts-narration-library/intelligence"
)

// Cache is the pluggable interface the adapter uses to skip the LLM
// call on repeats. Keyed by (content_hash, level, model) per
// CLAUDE.md's caching rule — escalation must not re-bill.
//
// Phase 3 declares the interface so Voice() can compile against it;
// Phase 4 ships NewInMemoryCache and the two-phase lookup helpers that
// actually populate the cache. With cache == nil (the New default),
// Voice() skips the wrapper entirely.
type Cache interface {
	Get(key CacheKey) (intelligence.IntelligenceResult, bool)
	Put(key CacheKey, val intelligence.IntelligenceResult)
}

// CacheKey is the cache identity per CLAUDE.md.
//
// Model is the full IntelligenceResult.Model string the adapter
// produces — "mcp-sampling@<clientID>/<actualModel>" — not just the
// clientID. Per plan Decision (v4) / B1 fix: keying on the full model
// id prevents cross-client and cross-model leakage. The cache lookup
// happens BEFORE the call using a per-clientID "last known model"
// hint, since the actual model id is the client's prerogative and
// only known after the call returns.
type CacheKey struct {
	ContentHash string
	Level       string
	Model       string
}

// cacheGet is the Phase 3 stub: Phase 4 implements the two-phase
// lookup. Phase 3's Voice() guards on a.cache != nil so this method
// is never reached with a non-nil cache that lacks the lookup helper
// — but we provide a definition here so the package compiles.
//
// Implementation note: Phase 4 replaces this body with the
// last-known-model probe and Cache.Get call.
func (a *Adapter) cacheGet(_ intelligence.IntelligenceRequest) (intelligence.IntelligenceResult, bool) {
	// Phase 4 wires the real probe; Phase 3 always misses, which is a
	// safe default — a miss just triggers the LLM call.
	return intelligence.IntelligenceResult{}, false
}

// cachePut is the Phase 3 stub paired with cacheGet. Phase 4 fills it
// in to write the full key + update the last-known-model map.
func (a *Adapter) cachePut(_ intelligence.IntelligenceRequest, _ intelligence.IntelligenceResult) {
	// Phase 4 wires the real put.
}
