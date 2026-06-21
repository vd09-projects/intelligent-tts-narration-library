package mcpsampling

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strconv"
	"sync"

	"github.com/vd09-projects/intelligent-tts-narration-library/intelligence"
)

// Cache is the pluggable interface the adapter uses to skip the LLM
// call on repeats. Keyed by (content_hash, level, model) per
// CLAUDE.md's caching rule — escalation must not re-bill.
//
// With cache == nil (the New default), Voice() skips the wrapper
// entirely. Two implementations ship:
//
//   - ServerCache (server_cache.go) — the production tier: a bounded
//     LRU plus the SHARED, server-lifetime last-known-model map. Issue
//     #25 made the cache cross-call so escalation across MCP tool calls
//     stops re-billing; that lifetime is what requires both the eviction
//     cap and the shared last-known map.
//   - inMemoryCache (below) — the unbounded simple tier for tests and
//     any per-call use. No eviction.
type Cache interface {
	Get(key CacheKey) (intelligence.IntelligenceResult, bool)
	Put(key CacheKey, val intelligence.IntelligenceResult)
}

// lastKnownStore is the last-known-model half of the two-phase lookup,
// decoupled from the Cache so the Adapter can source it from whichever
// owner is wired. Production passes a *ServerCache (which satisfies both
// Cache and lastKnownStore) so the map is server-lifetime and SHARED
// across every Adapter built per tool call. Tests / per-call use pass a
// plain Cache via WithCache, and the Adapter falls back to a per-call
// cacheLookupState. The shared path is the one that makes cross-call
// hits work (issue #25); the per-call path is the legacy/test tier.
type lastKnownStore interface {
	lastKnown(clientID string) (string, bool)
	setLastKnown(clientID, actualModel string)
}

// CacheKey is the cache identity per CLAUDE.md.
//
// Model is the FULL IntelligenceResult.Model string the adapter
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

// inMemoryCache is the unbounded simple tier: a sync.Map-backed Cache
// with NO eviction. It is intended for tests and any deliberately
// per-call use where the cache is discarded when the caller returns, so
// its growth is naturally bounded by that short lifetime. It is NOT the
// production tier — server-lifetime caching uses ServerCache, which adds
// the LRU eviction cap a long-lived unbounded map would otherwise leak.
// Implements Cache.
type inMemoryCache struct {
	m sync.Map // map[CacheKey]intelligence.IntelligenceResult
}

// NewInMemoryCache returns a fresh unbounded sync.Map-backed Cache. Use
// it for tests or genuinely per-call lifetime; for server-lifetime
// caching use NewServerCache, which bounds growth via LRU eviction.
func NewInMemoryCache() Cache {
	return &inMemoryCache{}
}

func (c *inMemoryCache) Get(key CacheKey) (intelligence.IntelligenceResult, bool) {
	v, ok := c.m.Load(key)
	if !ok {
		return intelligence.IntelligenceResult{}, false
	}
	r, ok := v.(intelligence.IntelligenceResult)
	return r, ok
}

func (c *inMemoryCache) Put(key CacheKey, val intelligence.IntelligenceResult) {
	c.m.Store(key, val)
}

// cacheLookupState holds the per-Adapter "last known model" map for the
// two-phase lookup. The map records, per clientID, the most-recent
// actualModel observed from a CreateMessage reply, so the cache key
// before the call can be assembled.
//
// This is the per-call / test tier of the last-known store, paired with
// inMemoryCache. The PRODUCTION last-known store is ServerCache's shared
// map (server_cache.go): issue #25 proved a per-call last-known map
// defeats cross-call caching entirely — a fresh Adapter per tool call
// starts empty, so cacheGet misses without ever consulting the shared
// LRU. cacheLookupState therefore stays ONLY for callers that pass a
// plain Cache via WithCache (tests, intra-call use).
//
// Boundedness: one entry per distinct clientID. cmd/narrate-mcp wires a
// single clientID, so even at server lifetime the matching ServerCache
// map holds effectively one entry — bounded by construction, no
// eviction needed. Multi-clientID reuse is the Revisit trigger.
type cacheLookupState struct {
	mu             sync.RWMutex
	lastKnownByCID map[string]string // clientID -> actualModel
}

// Compile-time assertion: *cacheLookupState satisfies lastKnownStore.
var _ lastKnownStore = (*cacheLookupState)(nil)

func newCacheLookupState() *cacheLookupState {
	return &cacheLookupState{lastKnownByCID: make(map[string]string)}
}

func (s *cacheLookupState) lastKnown(clientID string) (string, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := s.lastKnownByCID[clientID]
	return v, ok
}

func (s *cacheLookupState) setLastKnown(clientID, actualModel string) {
	s.mu.Lock()
	s.lastKnownByCID[clientID] = actualModel
	s.mu.Unlock()
}

// hashContent computes the hex SHA-256 of the block text. Stable across
// adapter restarts; cheap; sufficient for "same text" detection.
func hashContent(text string) string {
	sum := sha256.Sum256([]byte(text))
	return hex.EncodeToString(sum[:])
}

// fullModelString assembles "mcp-sampling@<clientID>/<actualModel>" —
// the value the adapter stamps onto IntelligenceResult.Model and the
// CacheKey.Model component.
func fullModelString(clientID, actualModel string) string {
	return fmt.Sprintf("mcp-sampling@%s/%s", clientID, actualModel)
}

// levelToCacheKey renders a plan.Level into the CacheKey Level field as
// a numeric string. strconv.Itoa keeps the format stable across
// underlying-type changes.
func levelToCacheKey(l int) string {
	return strconv.Itoa(l)
}

// cacheGet implements the two-phase lookup (Phase 4):
//
//  1. If we have a last-known model for this clientID, build the full
//     CacheKey with it and Get from Cache. On hit, return the cached
//     IntelligenceResult.
//  2. On miss (or no last-known yet), the Adapter falls through to the
//     LLM call. After the call succeeds the Adapter calls cachePut,
//     which writes the full key AND updates last-known.
//
// First call ever for any clientID always misses (no last-known yet).
// Model switches invalidate the cache for that clientID — the lookup
// uses the OLD model id, misses, the call returns the NEW model id,
// and cachePut updates last-known to the NEW id.
func (a *Adapter) cacheGet(req intelligence.IntelligenceRequest) (intelligence.IntelligenceResult, bool) {
	if a.cache == nil {
		return intelligence.IntelligenceResult{}, false
	}
	if a.lastKnownStore == nil {
		return intelligence.IntelligenceResult{}, false
	}
	lastModel, ok := a.lastKnownStore.lastKnown(a.clientID)
	if !ok {
		return intelligence.IntelligenceResult{}, false
	}
	key := CacheKey{
		ContentHash: hashContent(req.BlockText),
		Level:       levelToCacheKey(int(req.Level)),
		Model:       fullModelString(a.clientID, lastModel),
	}
	return a.cache.Get(key)
}

// cachePut writes the full-key entry and refreshes last-known. Only
// called for successful (non-refusal) responses — refusals are never
// cached, per the package doc.
func (a *Adapter) cachePut(req intelligence.IntelligenceRequest, result intelligence.IntelligenceResult) {
	if a.cache == nil {
		return
	}
	if a.lastKnownStore == nil {
		return
	}
	// Extract the actualModel from result.Model. We stamped it ourselves
	// so the prefix split is safe; defensive nonetheless.
	actualModel := actualModelFromFull(result.Model, a.clientID)
	key := CacheKey{
		ContentHash: hashContent(req.BlockText),
		Level:       levelToCacheKey(int(req.Level)),
		Model:       result.Model, // full string
	}
	a.cache.Put(key, result)
	a.lastKnownStore.setLastKnown(a.clientID, actualModel)
}

// actualModelFromFull parses "mcp-sampling@<clientID>/<actualModel>"
// back into <actualModel>. If the format does not match (callers
// constructing a custom Cache + populating it externally), fall back
// to the full string.
func actualModelFromFull(fullModel, clientID string) string {
	prefix := fmt.Sprintf("mcp-sampling@%s/", clientID)
	if len(fullModel) > len(prefix) && fullModel[:len(prefix)] == prefix {
		return fullModel[len(prefix):]
	}
	return fullModel
}
