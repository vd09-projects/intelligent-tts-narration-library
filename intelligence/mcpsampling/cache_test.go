package mcpsampling

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/vd09-projects/intelligent-tts-narration-library/intelligence"
	"github.com/vd09-projects/intelligent-tts-narration-library/plan"
)

// fakeWithModel returns a fakeSamplingClient that always replies with
// the given (text, model) pair.
func fakeWithModel(text, model string) *fakeSamplingClient {
	return &fakeSamplingClient{
		fn: func(_ context.Context, _ *mcp.CreateMessageParams) (*mcp.CreateMessageResult, error) {
			return &mcp.CreateMessageResult{
				Content: &mcp.TextContent{Text: text},
				Model:   model,
			}, nil
		},
	}
}

// TestCache_FirstCallPerClientAlwaysMisses (T1) — pins the documented
// behavior: when no last-known model is known for a clientID, the
// cache always misses and the client is called. This is the cost of
// the two-phase lookup; first call per clientID is unavoidable.
func TestCache_FirstCallPerClientAlwaysMisses(t *testing.T) {
	t.Parallel()
	fake := fakeWithModel("the gist", "claude-haiku")
	a := New(WithClientID("first-call"), WithCache(NewInMemoryCache()))
	ctx := WithSamplingClient(context.Background(), fake)

	if _, err := a.Voice(ctx, newReq("blk", plan.L2)); err != nil {
		t.Fatalf("Voice: %v", err)
	}
	if got := fake.calls.Load(); got != 1 {
		t.Fatalf("first call must miss (call count = %d, want 1)", got)
	}
}

// TestCache_HitSkipsClient — pre-populate via a first call, then a
// second identical call hits the cache and the client is NOT called
// again.
func TestCache_HitSkipsClient(t *testing.T) {
	t.Parallel()
	fake := fakeWithModel("the gist", "claude-haiku")
	a := New(WithClientID("hit-skips"), WithCache(NewInMemoryCache()))
	ctx := WithSamplingClient(context.Background(), fake)
	req := newReq("blk-text", plan.L2)

	first, err := a.Voice(ctx, req)
	if err != nil {
		t.Fatalf("first Voice: %v", err)
	}
	if fake.calls.Load() != 1 {
		t.Fatalf("expected 1 call after first Voice, got %d", fake.calls.Load())
	}

	second, err := a.Voice(ctx, req)
	if err != nil {
		t.Fatalf("second Voice: %v", err)
	}
	if fake.calls.Load() != 1 {
		t.Errorf("expected cache hit (calls still = 1), got %d", fake.calls.Load())
	}
	if second.Text != first.Text || second.Model != first.Model {
		t.Errorf("cached result mismatch:\n got %+v\n want %+v", second, first)
	}
}

// TestCache_KeyIsolationByLevel — same text, different level → two
// separate cache entries. Both miss, both call the client.
func TestCache_KeyIsolationByLevel(t *testing.T) {
	t.Parallel()
	fake := fakeWithModel("ok", "claude-haiku")
	a := New(WithClientID("level-iso"), WithCache(NewInMemoryCache()))
	ctx := WithSamplingClient(context.Background(), fake)

	if _, err := a.Voice(ctx, newReq("blk", plan.L1)); err != nil {
		t.Fatalf("L1 Voice: %v", err)
	}
	if _, err := a.Voice(ctx, newReq("blk", plan.L2)); err != nil {
		t.Fatalf("L2 Voice: %v", err)
	}
	if got := fake.calls.Load(); got != 2 {
		t.Errorf("expected 2 calls (level-distinct keys), got %d", got)
	}
}

// TestCache_KeyIncludesChosenModel (B1 fix) — first call returns
// model A; second identical call's lookup uses last-known=A, hits.
// Now flip the fake to return model B for any subsequent call. A
// third request for a DIFFERENT block text returns the new model id,
// updating last-known to B. A fourth call for the ORIGINAL block text
// now looks up with last-known=B → MISS → triggers a client call.
// This pins the boundary that the cache key includes the FULL chosen
// model, per CLAUDE.md.
func TestCache_KeyIncludesChosenModel(t *testing.T) {
	t.Parallel()
	var modelToReturn atomic.Value
	modelToReturn.Store("model-A")
	fake := &fakeSamplingClient{
		fn: func(_ context.Context, _ *mcp.CreateMessageParams) (*mcp.CreateMessageResult, error) {
			return &mcp.CreateMessageResult{
				Content: &mcp.TextContent{Text: "summary"},
				Model:   modelToReturn.Load().(string),
			}, nil
		},
	}
	a := New(WithClientID("model-key"), WithCache(NewInMemoryCache()))
	ctx := WithSamplingClient(context.Background(), fake)

	// Calls 1 + 2: same block, identical (hash, level, model-A) → hit.
	if _, err := a.Voice(ctx, newReq("orig", plan.L2)); err != nil {
		t.Fatalf("first call: %v", err)
	}
	if _, err := a.Voice(ctx, newReq("orig", plan.L2)); err != nil {
		t.Fatalf("second call: %v", err)
	}
	if got := fake.calls.Load(); got != 1 {
		t.Fatalf("after second call expected 1 (cache hit), got %d", got)
	}

	// Flip the fake to return model B.
	modelToReturn.Store("model-B")

	// Call 3: different block text, forces a fresh call. Now last-known
	// is model-B.
	if _, err := a.Voice(ctx, newReq("different", plan.L2)); err != nil {
		t.Fatalf("third call: %v", err)
	}
	if got := fake.calls.Load(); got != 2 {
		t.Fatalf("after third call expected 2, got %d", got)
	}

	// Call 4: original block text, level 2. Lookup uses
	// last-known=model-B; the cached entry was under model-A → MISS,
	// triggers a client call. Pins B1.
	if _, err := a.Voice(ctx, newReq("orig", plan.L2)); err != nil {
		t.Fatalf("fourth call: %v", err)
	}
	if got := fake.calls.Load(); got != 3 {
		t.Errorf("after fourth call expected 3 (model switch invalidated), got %d", got)
	}
}

// TestCache_RefusalNotCached — first call replies with __REFUSE__;
// second identical call DOES re-invoke the fake. Refusals are not
// cached (per package doc).
func TestCache_RefusalNotCached(t *testing.T) {
	t.Parallel()
	fake := fakeWithModel("__REFUSE__ too thin", "claude-haiku")
	a := New(WithClientID("refusal-nc"), WithCache(NewInMemoryCache()))
	ctx := WithSamplingClient(context.Background(), fake)

	first, err := a.Voice(ctx, newReq("blk", plan.L3))
	if err != nil {
		t.Fatalf("first: %v", err)
	}
	if !first.Refused {
		t.Fatalf("expected first to be Refused")
	}

	if _, err := a.Voice(ctx, newReq("blk", plan.L3)); err != nil {
		t.Fatalf("second: %v", err)
	}
	if got := fake.calls.Load(); got != 2 {
		t.Errorf("refusal must not be cached (calls = %d, want 2)", got)
	}
}

// TestCache_ThreadSafeConcurrentPut — 20 goroutines, identical
// (hash, level), all racing the cache Put. The race detector catches
// any unprotected access; the test asserts that all 20 calls
// succeeded and produced consistent results. (T3 — RWMutex around
// the last-known map.)
func TestCache_ThreadSafeConcurrentPut(t *testing.T) {
	t.Parallel()
	fake := fakeWithModel("summary", "claude-haiku")
	a := New(WithClientID("concurrent"), WithCache(NewInMemoryCache()))
	ctx := WithSamplingClient(context.Background(), fake)
	req := newReq("shared-block", plan.L2)

	var wg sync.WaitGroup
	results := make([]intelligence.IntelligenceResult, 20)
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			r, err := a.Voice(ctx, req)
			if err != nil {
				t.Errorf("goroutine %d: %v", idx, err)
				return
			}
			results[idx] = r
		}(i)
	}
	wg.Wait()

	// All 20 results should be identical text — either via cache hit
	// or via an LLM call.
	for i, r := range results {
		if r.Text != "summary" {
			t.Errorf("result %d: text=%q want %q", i, r.Text, "summary")
		}
	}
	// Call count is racy under the two-phase lookup (any number between
	// 1 and 20 is acceptable depending on goroutine interleaving) —
	// the contract under test is "no data race", which the -race
	// detector verifies independently.
	if got := fake.calls.Load(); got < 1 || got > 20 {
		t.Errorf("call count out of bounds: %d", got)
	}
}

// TestCache_NilCache_VoiceWorks — sanity: with no cache the wrapper is
// fully bypassed (regression coverage if a future refactor accidentally
// requires cacheLookup).
func TestCache_NilCache_VoiceWorks(t *testing.T) {
	t.Parallel()
	fake := fakeWithModel("ok", "any")
	a := New() // no WithCache
	ctx := WithSamplingClient(context.Background(), fake)
	if _, err := a.Voice(ctx, newReq("blk", plan.L1)); err != nil {
		t.Fatalf("Voice with no cache: %v", err)
	}
	if got := fake.calls.Load(); got != 1 {
		t.Errorf("expected 1 call, got %d", got)
	}
}

// TestCacheKey_UsesFullModelString — direct unit test on the key
// composition, independent of the Voice() flow. Pins the literal
// format CLAUDE.md mandates.
func TestCacheKey_UsesFullModelString(t *testing.T) {
	t.Parallel()
	got := fullModelString("narrate-mcp", "claude-haiku")
	want := "mcp-sampling@narrate-mcp/claude-haiku"
	if got != want {
		t.Errorf("fullModelString: got %q want %q", got, want)
	}
}

// TestCacheKey_IncludesLevel — hard build-time guard (issue #48). The
// cache key MUST distinguish L2 from L3 so cross-level escalation does not
// return a stale lower-level gist or re-bill incorrectly. Two keys
// identical except for Level must be unequal as map keys, and the Level
// field must derive from the requested plan.Level (via levelToCacheKey).
// If a future refactor drops Level from CacheKey, this goes RED.
//
// Structural complement to TestCache_KeyIsolationByLevel (the behavioral
// proof). Today CacheKey carries Level, so this lands green.
func TestCacheKey_IncludesLevel(t *testing.T) {
	t.Parallel()
	k2 := CacheKey{ContentHash: "h", Level: levelToCacheKey(int(plan.L2)), Model: "m"}
	k3 := CacheKey{ContentHash: "h", Level: levelToCacheKey(int(plan.L3)), Model: "m"}
	if k2.Level == k3.Level {
		t.Fatalf("levelToCacheKey collapses L2 and L3 to %q", k2.Level)
	}
	if k2 == k3 {
		t.Fatalf("CacheKey ignores Level — L2 and L3 collide: %+v == %+v", k2, k3)
	}
	c := NewInMemoryCache()
	c.Put(k2, intelligence.IntelligenceResult{Text: "L2 summary"})
	c.Put(k3, intelligence.IntelligenceResult{Text: "L3 detail"})
	if v, _ := c.Get(k2); v.Text != "L2 summary" {
		t.Errorf("L2 entry clobbered by L3: got %q", v.Text)
	}
	if v, _ := c.Get(k3); v.Text != "L3 detail" {
		t.Errorf("L3 entry clobbered by L2: got %q", v.Text)
	}
}
