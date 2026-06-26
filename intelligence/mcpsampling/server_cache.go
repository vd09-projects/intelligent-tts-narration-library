package mcpsampling

import (
	"container/list"
	"sync"

	"github.com/vd09-projects/intelligent-tts-narration-library/intelligence"
)

// DefaultCacheCapacity is the entry-count ceiling the production wiring
// (cmd/narrate-mcp) passes to NewServerCache. 512 distinct
// (content_hash, level, full_model) entries is a sub-MB ceiling for the
// IntelligenceResult values cached here, which is the hard bound issue
// #25 exists to enforce: server-lifetime caching turns the previously
// per-call (harmlessly unbounded) cache into a real leak, so eviction
// must cap it.
const DefaultCacheCapacity = 512

// ServerCache is the server-lifetime cache handle. It owns BOTH pieces
// of state the two-phase cache lookup needs, and both must outlive a
// single MCP tool call for cross-call escalation to stop re-billing
// (issue #25, the load-bearing fix):
//
//  1. An eviction-aware LRU keyed by CacheKey (content_hash, level,
//     full_model). Implements the Cache interface verbatim, so it drops
//     in wherever a Cache is expected.
//  2. The shared last-known-model map (clientID -> actualModel). The
//     two-phase gate (cacheGet) needs this to assemble the full CacheKey
//     BEFORE the call. v1 left this map per-call; review proved that
//     defeats the whole ticket — a fresh Adapter per tool call starts
//     with an empty last-known map, so cacheGet returns a miss without
//     ever consulting the shared LRU, and every cross-call request
//     re-bills. Hoisting BOTH maps to one server-lifetime handle is the
//     fix.
//
// Eviction strategy: LRU + entry-count cap, deliberately NOT TTL. The
// cached value is a deterministic function of the key — it never goes
// wall-clock stale, so a TTL would only evict still-correct entries and
// force re-billing, violating "escalation must not re-bill". The size
// cap gives the predictable ceiling. Eviction is inline in Put under the
// LRU mutex (no background sweeper goroutine — intentional; see Put).
//
// Concurrency: the two halves use SEPARATE mutexes. They are never held
// simultaneously, so no lock-ordering deadlock is possible. The LRU
// mutex is a full sync.Mutex (not RWMutex) because Get mutates recency.
type ServerCache struct {
	// --- LRU half ---
	mu    sync.Mutex
	ll    *list.List // front = most-recently-used; back = LRU
	index map[CacheKey]*list.Element
	cap   int

	// --- shared last-known-model half ---
	lkMu           sync.RWMutex
	lastKnownByCID map[string]string // clientID -> actualModel
}

// lruEntry is the value stored in each list.Element. Holding the key
// alongside the value lets eviction delete the index entry when it pops
// the back of the list.
type lruEntry struct {
	key CacheKey
	val intelligence.IntelligenceResult
}

// Compile-time assertion: *ServerCache satisfies the Cache interface.
var _ Cache = (*ServerCache)(nil)

// NewServerCache returns a server-lifetime cache with the given LRU
// entry-count cap. The cmd/narrate-mcp wiring allocates exactly one of
// these and threads it through every tool call.
//
// A capacity <= 0 clamps to DefaultCacheCapacity. This is a defensive
// guard only — production passes DefaultCacheCapacity explicitly; the
// single-cap API (no WithCacheCapacity option) means there is no other
// way to set it.
func NewServerCache(capacity int) *ServerCache {
	if capacity <= 0 {
		capacity = DefaultCacheCapacity
	}
	return &ServerCache{
		ll:             list.New(),
		index:          make(map[CacheKey]*list.Element, capacity),
		cap:            capacity,
		lastKnownByCID: make(map[string]string),
	}
}

// Get implements Cache. On hit it moves the entry to the front (most
// recently used) and returns its value. Takes the full mutex — not a
// read lock — because the recency move is a write to the list.
func (c *ServerCache) Get(key CacheKey) (intelligence.IntelligenceResult, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	el, ok := c.index[key]
	if !ok {
		return intelligence.IntelligenceResult{}, false
	}
	c.ll.MoveToFront(el)
	return el.Value.(*lruEntry).val, true
}

// Put implements Cache. Inserts a new entry at the front, or updates an
// existing entry's value in place and moves it to the front. After an
// insert grows the map past cap, it evicts the back (least-recently-used)
// entry inline, in the same critical section as the insert, so the size
// invariant (len <= cap) holds at every lock release. Eviction is
// inline by design — no background goroutine — so the ceiling is
// enforced synchronously without an extra moving part.
func (c *ServerCache) Put(key CacheKey, val intelligence.IntelligenceResult) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if el, ok := c.index[key]; ok {
		// Update in place; refresh recency.
		el.Value.(*lruEntry).val = val
		c.ll.MoveToFront(el)
		return
	}

	el := c.ll.PushFront(&lruEntry{key: key, val: val})
	c.index[key] = el

	// Evict the least-recently-used entries until within cap. A single
	// insert can only overflow by one, but the loop is robust to a
	// future cap that shrinks at runtime.
	for c.ll.Len() > c.cap {
		oldest := c.ll.Back()
		if oldest == nil {
			break
		}
		c.ll.Remove(oldest)
		delete(c.index, oldest.Value.(*lruEntry).key)
	}
}

// lastKnown returns the most-recent actualModel observed for clientID,
// or ("", false) if none yet. Guarded by its own RWMutex, distinct from
// the LRU mutex.
//
// Concurrency note (benign cross-call stale-read window): under the
// shared, server-lifetime last-known map a mid-session model switch can
// be observed across calls. Call A may resolve a NEW model and set
// last-known while call B is mid-flight reading the OLD value. The worst
// outcome is ONE extra re-bill — B builds the CacheKey with the stale
// model, misses, calls the client, and writes the correct entry. The
// value is keyed on the full model, so a stale read NEVER returns wrong
// data; it only costs one cache miss. Acceptable, and far cheaper than
// the alternative (per-call lifetime), which made EVERY cross-call
// request a re-bill (issue #25).
func (c *ServerCache) lastKnown(clientID string) (string, bool) {
	c.lkMu.RLock()
	defer c.lkMu.RUnlock()
	v, ok := c.lastKnownByCID[clientID]
	return v, ok
}

// setLastKnown records the actualModel for clientID. Last writer wins;
// concurrent first-calls for the same clientID resolve the same model
// (the client's prerogative), so the racing writes are idempotent.
//
// The map is NOT capped. It is bounded by construction: cmd/narrate-mcp
// wires a single clientID ("narrate-mcp", main.go WithClientID), so the
// map holds effectively one entry for the life of the server. "Needs no
// eviction" (true, bounded by construction) is a different claim from
// "can stay per-call" (false — proven by the cross-call regression).
// Multi-clientID reuse is the Revisit trigger for capping this map.
func (c *ServerCache) setLastKnown(clientID, actualModel string) {
	c.lkMu.Lock()
	c.lastKnownByCID[clientID] = actualModel
	c.lkMu.Unlock()
}

// len reports the current LRU entry count. Test-facing helper for the
// cap-honored assertions; not part of the Cache interface.
func (c *ServerCache) len() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.ll.Len()
}
