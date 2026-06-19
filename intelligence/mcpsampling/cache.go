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
// entirely. Cross-call lifetime (per-server cache) is intentionally
// out of scope for #13; the production wiring constructs a fresh
// per-call cache to catch intra-call escalation only.
type Cache interface {
	Get(key CacheKey) (intelligence.IntelligenceResult, bool)
	Put(key CacheKey, val intelligence.IntelligenceResult)
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

// inMemoryCache is a sync.Map-backed Cache. No eviction; long-lived
// callers should bound lifetime externally (the cmd/narrate-mcp wiring
// in Phase 5 uses a per-call cache, so the in-memory growth is bounded
// by one MCP tool call). Implements Cache.
type inMemoryCache struct {
	m sync.Map // map[CacheKey]intelligence.IntelligenceResult
}

// NewInMemoryCache returns a fresh sync.Map-backed Cache. Suitable for
// per-call lifetime; cross-call use requires an external eviction policy
// (out of scope for #13).
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

// cacheLookupState holds the per-Adapter "last known model" map used by
// the two-phase lookup. The map records, per clientID, the most-recent
// actualModel observed from a CreateMessage reply, so the cache key
// before the call can be assembled.
//
// Unboundedness note: this map is theoretically unbounded — one entry
// per distinct clientID seen. In practice the adapter is constructed
// per-call by cmd/narrate-mcp (Phase 5) with a single clientID, so the
// map holds exactly one entry for the lifetime of that call. If a
// future production path reuses an Adapter across many clientIDs,
// bounding the map becomes a follow-up (eviction policy ticket, same
// scope as cross-call cache eviction).
type cacheLookupState struct {
	mu             sync.RWMutex
	lastKnownByCID map[string]string // clientID -> actualModel
}

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
	if a.cacheLookup == nil {
		return intelligence.IntelligenceResult{}, false
	}
	lastModel, ok := a.cacheLookup.lastKnown(a.clientID)
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
	if a.cacheLookup == nil {
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
	a.cacheLookup.setLastKnown(a.clientID, actualModel)
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
