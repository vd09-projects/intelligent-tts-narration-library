package anthropic

// Cache machinery for the Anthropic adapter.
//
// Decision (v10) — convention: experimental. This cache is single-phase
// (no two-phase last-known-actual-model lookup), unlike mcpsampling.
//
// mcpsampling needs two phases because the MCP client picks the actual
// model and the adapter only learns that id after the first CreateMessage
// reply — so the pre-call CacheKey can't include the actual model until
// a previous call has populated a per-clientID last-known map.
//
// The Anthropic Messages API is the opposite: the adapter chooses the
// model (a.model) at construction, so the cache key is fully knowable
// before the call. There is no clientID, no last-known map, no first-
// call-always-misses-for-this-client behavior. The lookup is one step.
//
// Per Decision v2 the package keeps its own cache (duplicate, not lift) —
// collapsing to single-phase here is a simplification driven by the
// different shape of the upstream API, not a divergence from the
// mcpsampling contract. CacheKey shape and Cache interface stay parallel
// so a future lift remains tractable.

import (
	"crypto/sha256"
	"encoding/hex"
	"sync"

	"github.com/vd09-projects/intelligent-tts-narration-library/intelligence"
	"github.com/vd09-projects/intelligent-tts-narration-library/plan"
)

// Cache is the pluggable interface the adapter uses to skip the HTTP
// call on repeats. Keyed by (content_hash, level, model) per CLAUDE.md's
// caching rule — escalation must not re-bill. Stores the previously-
// voiced summary text; refusals are not cached.
type Cache interface {
	Get(key CacheKey) (string, bool)
	Put(key CacheKey, value string)
}

// CacheKey is the cache identity per CLAUDE.md. Model is the FULL
// "anthropic@<actualModel>" string the adapter produces, so a model
// switch (different actual model id resolved by the API) invalidates
// without a separate cache-clear step.
type CacheKey struct {
	ContentHash string
	Level       plan.Level
	Model       string
}

// inMemoryCache is a mutex-guarded map. No eviction; per-call lifetime
// is the production wiring's responsibility (cmd/narrate Phase 6
// constructs a fresh cache per pipeline invocation). Implements Cache.
type inMemoryCache struct {
	mu sync.RWMutex
	m  map[CacheKey]string
}

// NewInMemoryCache returns a fresh map-backed Cache.
func NewInMemoryCache() Cache {
	return &inMemoryCache{m: make(map[CacheKey]string)}
}

func (c *inMemoryCache) Get(key CacheKey) (string, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	v, ok := c.m[key]
	return v, ok
}

func (c *inMemoryCache) Put(key CacheKey, value string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.m[key] = value
}

// cacheLookupState carries the content hash and level computed during
// cacheGet across to cachePut so the hash is not recomputed for the
// post-call write. The lookup happens before the HTTP call returns, so
// state is always populated even on a cache miss.
type cacheLookupState struct {
	ContentHash string
	Level       plan.Level
}

// hashContent computes the hex SHA-256 of the block text. Stable across
// adapter restarts; cheap; sufficient for same-text detection.
func hashContent(text string) string {
	sum := sha256.Sum256([]byte(text))
	return hex.EncodeToString(sum[:])
}

// fullModelString assembles "anthropic@<actualModel>" — the value the
// adapter stamps onto IntelligenceResult.Model and the CacheKey.Model
// component. Mirrors mcpsampling's helper of the same name (different
// prefix; no clientID).
func fullModelString(actual string) string {
	return "anthropic@" + actual
}

// cacheGet performs the single-phase lookup. State (hash + level) is
// always returned so cachePut can reuse it without rehashing. With
// a.cache == nil the lookup is a no-op miss (state still populated).
func (a *Adapter) cacheGet(req intelligence.IntelligenceRequest) (hit string, ok bool, state cacheLookupState) {
	state = cacheLookupState{
		ContentHash: hashContent(req.BlockText),
		Level:       req.Level,
	}
	if a.cache == nil {
		return "", false, state
	}
	key := CacheKey{
		ContentHash: state.ContentHash,
		Level:       state.Level,
		Model:       fullModelString(a.model),
	}
	v, found := a.cache.Get(key)
	return v, found, state
}

// cachePut writes the freshly-voiced summary. The key uses the
// configured a.model (not the actual resolved model from the response)
// so the post-call key matches the pre-call lookup key — the configured
// model id is what the adapter committed to at construction. Refusals
// are never cached (Voice skips this call on the refusal branch).
//
// Trade-off: if Anthropic silently moves the resolved model under a
// stable alias (e.g. "claude-haiku-4-5" → newer dated version), cached
// entries become stale until the next adapter construction. Acceptable
// because the production wiring (cmd/narrate Phase 6) constructs a
// fresh per-call cache, so staleness cannot outlive one pipeline run.
func (a *Adapter) cachePut(state cacheLookupState, text string) {
	if a.cache == nil {
		return
	}
	key := CacheKey{
		ContentHash: state.ContentHash,
		Level:       state.Level,
		Model:       fullModelString(a.model),
	}
	a.cache.Put(key, text)
}
