package anthropic

import (
	"context"
	"net/http"
	"sync/atomic"
	"testing"

	"github.com/vd09-projects/intelligent-tts-narration-library/plan"
)

// successJSON is the canned upstream body for cache_test calls. Same
// model id across calls so the cache key Model field is stable.
const successJSON = `{"model":"claude-haiku-4-5-test","content":[{"type":"text","text":"summary text"}],"stop_reason":"end_turn"}`

// refusalJSON returns __REFUSE__ so parseRefusal flips Refused=true.
const refusalJSON = `{"model":"claude-haiku-4-5-test","content":[{"type":"text","text":"__REFUSE__ block too thin"}],"stop_reason":"end_turn"}`

// countingTransport returns the same body on every call and tracks how
// many times it was hit. Tests assert the count rather than inspect
// request bodies — the cache decision is binary (call or skip).
func countingTransport(body string) (roundTripFunc, *atomic.Int64) {
	var calls atomic.Int64
	rt := roundTripFunc(func(_ *http.Request) (*http.Response, error) {
		calls.Add(1)
		return jsonResp(200, body), nil
	})
	return rt, &calls
}

// TestVoice_FirstCallMisses_SecondCallHits — the second call with the
// same (BlockText, Level) returns the cached summary without making a
// second HTTP request.
func TestVoice_FirstCallMisses_SecondCallHits(t *testing.T) {
	t.Parallel()
	rt, calls := countingTransport(successJSON)
	a := newAdapter(t, rt, WithCache(NewInMemoryCache()))

	first, err := a.Voice(context.Background(), proseReq("hello world", plan.L2))
	if err != nil {
		t.Fatalf("first call err: %v", err)
	}
	second, err := a.Voice(context.Background(), proseReq("hello world", plan.L2))
	if err != nil {
		t.Fatalf("second call err: %v", err)
	}
	if calls.Load() != 1 {
		t.Errorf("HTTP calls: got %d want 1 (second call should hit cache)", calls.Load())
	}
	if first.Text != second.Text {
		t.Errorf("cached Text differs: first=%q second=%q", first.Text, second.Text)
	}
	// Model on cache hit uses fullModelString(a.model) — the configured
	// model id is what the adapter committed to at construction. The
	// first-call value carries the actual resolved model id from the API
	// response, which may include a dated suffix. The divergence is
	// intentional: cache hits skip the HTTP round-trip, so we cannot
	// echo back an actual model we never resolved.
	if second.Model != fullModelString(a.model) {
		t.Errorf("cache-hit Model: got %q want %q", second.Model, fullModelString(a.model))
	}
}

// TestVoice_RefusalNotCached — a refusal response must not poison the
// cache. A second call hits HTTP again (and could succeed if the
// upstream changes its mind).
func TestVoice_RefusalNotCached(t *testing.T) {
	t.Parallel()
	rt, calls := countingTransport(refusalJSON)
	a := newAdapter(t, rt, WithCache(NewInMemoryCache()))

	_, err := a.Voice(context.Background(), proseReq("hello world", plan.L2))
	if err != nil {
		t.Fatalf("first call err: %v", err)
	}
	_, err = a.Voice(context.Background(), proseReq("hello world", plan.L2))
	if err != nil {
		t.Fatalf("second call err: %v", err)
	}
	if calls.Load() != 2 {
		t.Errorf("HTTP calls: got %d want 2 (refusals must not be cached)", calls.Load())
	}
}

// TestVoice_NoCacheIsNoop — WithCache(nil) (or absent) means every call
// hits HTTP. Validates the nil-safe fast-path.
func TestVoice_NoCacheIsNoop(t *testing.T) {
	t.Parallel()
	rt, calls := countingTransport(successJSON)
	a := newAdapter(t, rt) // no WithCache → a.cache == nil

	for i := 0; i < 2; i++ {
		if _, err := a.Voice(context.Background(), proseReq("hello world", plan.L2)); err != nil {
			t.Fatalf("call %d err: %v", i, err)
		}
	}
	if calls.Load() != 2 {
		t.Errorf("HTTP calls: got %d want 2 (no cache)", calls.Load())
	}
}

// TestVoice_DifferentLevelInvalidates — same text, different level →
// different CacheKey → cache miss on the second call.
func TestVoice_DifferentLevelInvalidates(t *testing.T) {
	t.Parallel()
	rt, calls := countingTransport(successJSON)
	a := newAdapter(t, rt, WithCache(NewInMemoryCache()))

	if _, err := a.Voice(context.Background(), proseReq("hello world", plan.L2)); err != nil {
		t.Fatalf("L2 err: %v", err)
	}
	if _, err := a.Voice(context.Background(), proseReq("hello world", plan.L3)); err != nil {
		t.Fatalf("L3 err: %v", err)
	}
	if calls.Load() != 2 {
		t.Errorf("HTTP calls: got %d want 2 (different level should miss)", calls.Load())
	}
}

// TestCacheKey_IncludesLevel — hard build-time guard (issue #48). The
// cache key MUST distinguish L2 from L3 so escalation across levels does
// not return a stale lower-level summary. Two keys identical except for
// Level must be unequal as map keys. If a future refactor drops Level
// from CacheKey, this test goes RED rather than silently re-billing or
// returning a stale gist.
//
// This is the structural complement to TestVoice_DifferentLevelInvalidates
// (the behavioral HTTP-count proof): together they pin "(hash, level,
// model)" per CLAUDE.md. Today the Anthropic CacheKey carries Level, so
// this lands green.
func TestCacheKey_IncludesLevel(t *testing.T) {
	t.Parallel()
	k2 := CacheKey{ContentHash: "h", Level: plan.L2, Model: "anthropic@m"}
	k3 := CacheKey{ContentHash: "h", Level: plan.L3, Model: "anthropic@m"}
	if k2 == k3 {
		t.Fatalf("CacheKey ignores Level — L2 and L3 collide: %+v == %+v", k2, k3)
	}
	// And the cache must treat them as distinct entries.
	c := NewInMemoryCache()
	c.Put(k2, "L2 summary")
	c.Put(k3, "L3 detail")
	if v, _ := c.Get(k2); v != "L2 summary" {
		t.Errorf("L2 entry clobbered by L3: got %q", v)
	}
	if v, _ := c.Get(k3); v != "L3 detail" {
		t.Errorf("L3 entry clobbered by L2: got %q", v)
	}
}

// TestVoice_DifferentBlockTextInvalidates — different text, same level
// → different ContentHash → cache miss on the second call.
func TestVoice_DifferentBlockTextInvalidates(t *testing.T) {
	t.Parallel()
	rt, calls := countingTransport(successJSON)
	a := newAdapter(t, rt, WithCache(NewInMemoryCache()))

	if _, err := a.Voice(context.Background(), proseReq("hello world", plan.L2)); err != nil {
		t.Fatalf("first err: %v", err)
	}
	if _, err := a.Voice(context.Background(), proseReq("different text", plan.L2)); err != nil {
		t.Fatalf("second err: %v", err)
	}
	if calls.Load() != 2 {
		t.Errorf("HTTP calls: got %d want 2 (different block text should miss)", calls.Load())
	}
}
