package mcpsampling

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/vd09-projects/intelligent-tts-narration-library/intelligence"
	"github.com/vd09-projects/intelligent-tts-narration-library/plan"
)

// keyN builds a deterministic distinct CacheKey for index n. Used by the
// LRU table tests where the model component is irrelevant.
func keyN(n int) CacheKey {
	return CacheKey{
		ContentHash: fmt.Sprintf("hash-%d", n),
		Level:       "2",
		Model:       "mcp-sampling@c/m",
	}
}

func resultN(n int) intelligence.IntelligenceResult {
	return intelligence.IntelligenceResult{Text: fmt.Sprintf("text-%d", n)}
}

// TestServerCache_NewClampsCapacity — a capacity <= 0 clamps to
// DefaultCacheCapacity (defensive guard; production passes the default
// explicitly). Table-driven.
func TestServerCache_NewClampsCapacity(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		give    int
		wantCap int
	}{
		{"zero clamps to default", 0, DefaultCacheCapacity},
		{"negative clamps to default", -5, DefaultCacheCapacity},
		{"positive kept", 8, 8},
		{"one kept", 1, 1},
	}
	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			sc := NewServerCache(tc.give)
			if sc.cap != tc.wantCap {
				t.Errorf("cap = %d, want %d", sc.cap, tc.wantCap)
			}
		})
	}
}

// TestServerCache_LRU — table-driven over the LRU contract: cap honored,
// eviction order (least-recently-used dropped), update-in-place does not
// grow the map, Get refreshes recency so a touched entry survives.
func TestServerCache_LRU(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		// ops mutate the cache; run in order.
		ops func(sc *ServerCache)
		// present keys expected to Get-hit after ops.
		present []int
		// absent keys expected to Get-miss after ops.
		absent []int
		// wantLen is the expected entry count (cap invariant).
		wantLen int
	}{
		{
			name:    "under cap keeps all",
			ops:     func(sc *ServerCache) { putN(sc, 0, 1, 2) },
			present: []int{0, 1, 2},
			wantLen: 3,
		},
		{
			name: "eviction order drops oldest first",
			// cap 3; insert 0,1,2 then 3 -> 0 (LRU) evicted.
			ops:     func(sc *ServerCache) { putN(sc, 0, 1, 2, 3) },
			present: []int{1, 2, 3},
			absent:  []int{0},
			wantLen: 3,
		},
		{
			name: "Get refreshes recency so touched entry survives",
			ops: func(sc *ServerCache) {
				putN(sc, 0, 1, 2)
				sc.Get(keyN(0)) // 0 now most-recently-used
				putN(sc, 3)     // evicts LRU == 1, not 0
			},
			present: []int{0, 2, 3},
			absent:  []int{1},
			wantLen: 3,
		},
		{
			name: "update in place does not grow",
			ops: func(sc *ServerCache) {
				putN(sc, 0, 1, 2)
				sc.Put(keyN(1), resultN(99)) // update existing key
			},
			present: []int{0, 1, 2},
			wantLen: 3,
		},
		{
			name: "cap of one keeps only newest",
			ops:  func(sc *ServerCache) { putN(sc, 0, 1) },
			// built with cap 1 below via capOne sentinel
			present: []int{1},
			absent:  []int{0},
			wantLen: 1,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			capacity := 3
			if tc.name == "cap of one keeps only newest" {
				capacity = 1
			}
			sc := NewServerCache(capacity)
			tc.ops(sc)

			if got := sc.len(); got != tc.wantLen {
				t.Errorf("len = %d, want %d", got, tc.wantLen)
			}
			if sc.len() > sc.cap {
				t.Errorf("cap invariant violated: len %d > cap %d", sc.len(), sc.cap)
			}
			for _, n := range tc.present {
				if _, ok := sc.Get(keyN(n)); !ok {
					t.Errorf("key %d: want present, got miss", n)
				}
			}
			for _, n := range tc.absent {
				if _, ok := sc.Get(keyN(n)); ok {
					t.Errorf("key %d: want evicted, got hit", n)
				}
			}
		})
	}
}

// TestServerCache_UpdateInPlaceReturnsNewValue — updating an existing key
// replaces the value rather than inserting a duplicate.
func TestServerCache_UpdateInPlaceReturnsNewValue(t *testing.T) {
	t.Parallel()
	sc := NewServerCache(4)
	sc.Put(keyN(0), resultN(0))
	sc.Put(keyN(0), resultN(7))
	got, ok := sc.Get(keyN(0))
	if !ok {
		t.Fatal("want hit after update")
	}
	if got.Text != "text-7" {
		t.Errorf("value not updated: got %q want %q", got.Text, "text-7")
	}
	if sc.len() != 1 {
		t.Errorf("update created a duplicate: len = %d, want 1", sc.len())
	}
}

// putN inserts a distinct entry per index, in order.
func putN(sc *ServerCache, ns ...int) {
	for _, n := range ns {
		sc.Put(keyN(n), resultN(n))
	}
}

// TestServerCache_LastKnownSharedMap — the last-known map round-trips and
// is independent of the LRU half.
func TestServerCache_LastKnownSharedMap(t *testing.T) {
	t.Parallel()
	sc := NewServerCache(4)
	if _, ok := sc.lastKnown("cid"); ok {
		t.Fatal("want empty last-known before any set")
	}
	sc.setLastKnown("cid", "model-x")
	got, ok := sc.lastKnown("cid")
	if !ok || got != "model-x" {
		t.Errorf("lastKnown = (%q,%v), want (model-x,true)", got, ok)
	}
}

// TestServerCache_RaceGetPut (-race) — N goroutines hammer Get/Put on a
// small-cap cache. Asserts the cap invariant holds at the end and the
// race detector finds no torn state. Run under `make test-race`.
func TestServerCache_RaceGetPut(t *testing.T) {
	t.Parallel()
	const cap = 16
	const goroutines = 32
	const opsEach = 500
	sc := NewServerCache(cap)

	var wg sync.WaitGroup
	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			for i := 0; i < opsEach; i++ {
				k := keyN((g*opsEach + i) % (cap * 4)) // churn beyond cap
				if i%2 == 0 {
					sc.Put(k, resultN(i))
				} else {
					sc.Get(k)
				}
			}
		}(g)
	}
	wg.Wait()

	if got := sc.len(); got > cap {
		t.Errorf("cap invariant violated under load: len %d > cap %d", got, cap)
	}
}

// TestServerCache_RaceLastKnown (-race) — concurrent last-known
// read/write across goroutines. No torn state; last writer wins.
func TestServerCache_RaceLastKnown(t *testing.T) {
	t.Parallel()
	sc := NewServerCache(4)
	var wg sync.WaitGroup
	for g := 0; g < 24; g++ {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			for i := 0; i < 200; i++ {
				sc.setLastKnown("cid", fmt.Sprintf("model-%d", g))
				sc.lastKnown("cid")
			}
		}(g)
	}
	wg.Wait()
	if _, ok := sc.lastKnown("cid"); !ok {
		t.Error("want a last-known value after concurrent writes")
	}
}

// ---- Adapter-level integration tests against a shared ServerCache ----

// adapterSharing builds a fresh Adapter wired to the given shared
// ServerCache and clientID. Each call models a separate MCP tool call
// (buildIntelligence constructs a NEW Adapter every time) while the
// ServerCache stays server-lifetime.
func adapterSharing(sc *ServerCache, clientID string) *Adapter {
	return New(WithClientID(clientID), WithServerCache(sc))
}

// TestServerCache_CrossCallHitAcrossAdapters (6a — the load-bearing
// regression). TWO SEPARATE Adapters share ONE ServerCache. The first
// Voice() bills once and populates the shared cache + last-known map. A
// second Voice() for the SAME (content_hash, level, full_model) key,
// issued through the OTHER Adapter, must make ZERO client calls — proving
// both the LRU AND the last-known map are server-lifetime and shared.
// This FAILS against the v1 design (per-call last-known map): the second
// Adapter starts empty, cacheGet returns a miss without consulting the
// shared LRU, and the call re-bills.
func TestServerCache_CrossCallHitAcrossAdapters(t *testing.T) {
	t.Parallel()
	fake := fakeWithModel("the gist", "claude-haiku")
	sc := NewServerCache(DefaultCacheCapacity)
	ctx := WithSamplingClient(context.Background(), fake)
	req := newReq("shared-block-text", plan.L2)

	// Tool call 1: a fresh Adapter sharing sc.
	a1 := adapterSharing(sc, "narrate-mcp")
	if _, err := a1.Voice(ctx, req); err != nil {
		t.Fatalf("first Voice: %v", err)
	}
	if got := fake.calls.Load(); got != 1 {
		t.Fatalf("first call must bill once, got %d", got)
	}

	// Tool call 2: a DIFFERENT fresh Adapter, same shared sc, same key.
	a2 := adapterSharing(sc, "narrate-mcp")
	hit, err := a2.Voice(ctx, req)
	if err != nil {
		t.Fatalf("second Voice: %v", err)
	}
	if got := fake.calls.Load(); got != 1 {
		t.Fatalf("cross-call hit must make ZERO additional client calls (total calls = %d, want 1)", got)
	}
	if hit.Text != "the gist" {
		t.Errorf("cross-call hit returned wrong text: %q", hit.Text)
	}
}

// TestServerCache_EvictThenReRequest (6b) — a cap-1 cache. Block A is
// cached, then block B evicts it. Re-requesting A must re-bill exactly
// once (the evicted entry is gone but the answer is still correct, just
// re-fetched).
func TestServerCache_EvictThenReRequest(t *testing.T) {
	t.Parallel()
	fake := fakeWithModel("summary", "claude-haiku")
	sc := NewServerCache(1) // cap 1 forces eviction on the 2nd distinct key
	ctx := WithSamplingClient(context.Background(), fake)

	reqA := newReq("block-A", plan.L2)
	reqB := newReq("block-B", plan.L2)

	a := adapterSharing(sc, "narrate-mcp")

	// 1: cache A (bill 1).
	if _, err := a.Voice(ctx, reqA); err != nil {
		t.Fatalf("Voice A: %v", err)
	}
	// 2: cache B, evicting A (bill 2).
	if _, err := a.Voice(ctx, reqB); err != nil {
		t.Fatalf("Voice B: %v", err)
	}
	if got := fake.calls.Load(); got != 2 {
		t.Fatalf("after A,B expected 2 bills, got %d", got)
	}
	// 3: A was evicted -> exactly one re-bill (bill 3).
	if _, err := a.Voice(ctx, reqA); err != nil {
		t.Fatalf("Voice A re-request: %v", err)
	}
	if got := fake.calls.Load(); got != 3 {
		t.Errorf("evict-then-re-request expected exactly one re-bill (total 3), got %d", got)
	}
}

// TestServerCache_HotEntrySurvivesEscalation (6c) — L1, L2, L3 of the
// same block are DISTINCT cache keys (level is part of the key). Escalate
// L1 -> L2 -> L3 with a cap large enough to hold all three, interleaving
// re-requests of the hot (L1) block; the hot entry must stay cached
// (Get refreshes its recency) so re-requesting L1 makes no extra call.
func TestServerCache_HotEntrySurvivesEscalation(t *testing.T) {
	t.Parallel()
	fake := fakeWithModel("gist", "claude-haiku")
	sc := NewServerCache(8)
	ctx := WithSamplingClient(context.Background(), fake)
	a := adapterSharing(sc, "narrate-mcp")

	blk := "hot-block"

	// L1 (bill 1).
	if _, err := a.Voice(ctx, newReq(blk, plan.L1)); err != nil {
		t.Fatalf("L1: %v", err)
	}
	// L2 escalation (bill 2 — distinct key).
	if _, err := a.Voice(ctx, newReq(blk, plan.L2)); err != nil {
		t.Fatalf("L2: %v", err)
	}
	// Touch L1 again — must be a hit, refreshing recency.
	if _, err := a.Voice(ctx, newReq(blk, plan.L1)); err != nil {
		t.Fatalf("L1 re-request: %v", err)
	}
	// L3 escalation (bill 3 — distinct key).
	if _, err := a.Voice(ctx, newReq(blk, plan.L3)); err != nil {
		t.Fatalf("L3: %v", err)
	}
	if got := fake.calls.Load(); got != 3 {
		t.Fatalf("L1/L2/L3 are distinct keys; expected 3 bills (L1 re-request was a hit), got %d", got)
	}
	// Hot L1 entry still cached after escalation.
	if _, err := a.Voice(ctx, newReq(blk, plan.L1)); err != nil {
		t.Fatalf("final L1: %v", err)
	}
	if got := fake.calls.Load(); got != 3 {
		t.Errorf("hot L1 entry must survive escalation (no extra bill); total bills = %d, want 3", got)
	}
}

// TestServerCache_RefusalNeverCachedSharedTier — refusals are not cached
// even on the server tier: a refusing block re-bills every time across
// two Adapters sharing the handle.
func TestServerCache_RefusalNeverCachedSharedTier(t *testing.T) {
	t.Parallel()
	fake := fakeWithModel("__REFUSE__ too thin", "claude-haiku")
	sc := NewServerCache(DefaultCacheCapacity)
	ctx := WithSamplingClient(context.Background(), fake)
	req := newReq("thin-block", plan.L3)

	a1 := adapterSharing(sc, "narrate-mcp")
	first, err := a1.Voice(ctx, req)
	if err != nil {
		t.Fatalf("first: %v", err)
	}
	if !first.Refused {
		t.Fatal("want first refused")
	}

	a2 := adapterSharing(sc, "narrate-mcp")
	if _, err := a2.Voice(ctx, req); err != nil {
		t.Fatalf("second: %v", err)
	}
	if got := fake.calls.Load(); got != 2 {
		t.Errorf("refusal must never be cached even on shared tier (calls = %d, want 2)", got)
	}
}

// TestServerCache_VoiceUnderConcurrentLoad (-race) — multiple FRESH
// Adapters (one per simulated tool call) share ONE ServerCache and issue
// Voice() concurrently. Exercises the now-contended shared last-known map
// plus the LRU under real Voice() traffic. Asserts no data race (via
// -race), the cap invariant, and that every call returned the summary.
func TestServerCache_VoiceUnderConcurrentLoad(t *testing.T) {
	t.Parallel()
	const goroutines = 24
	const blocks = 64
	var calls atomic.Int64
	fake := &fakeSamplingClient{
		fn: func(_ context.Context, _ *mcp.CreateMessageParams) (*mcp.CreateMessageResult, error) {
			calls.Add(1)
			return &mcp.CreateMessageResult{
				Content: &mcp.TextContent{Text: "summary"},
				Model:   "claude-haiku",
			}, nil
		},
	}
	sc := NewServerCache(32) // smaller than block count -> forces eviction churn
	ctx := WithSamplingClient(context.Background(), fake)

	var wg sync.WaitGroup
	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			a := adapterSharing(sc, "narrate-mcp") // fresh Adapter per goroutine
			for i := 0; i < blocks; i++ {
				req := newReq(fmt.Sprintf("block-%d", i%blocks), plan.L2)
				r, err := a.Voice(ctx, req)
				if err != nil {
					t.Errorf("goroutine %d: %v", g, err)
					return
				}
				if r.Text != "summary" {
					t.Errorf("goroutine %d: text = %q", g, r.Text)
					return
				}
			}
		}(g)
	}
	wg.Wait()

	if got := sc.len(); got > 32 {
		t.Errorf("cap invariant violated under Voice load: len %d > cap 32", got)
	}
}
