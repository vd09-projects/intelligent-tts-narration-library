package anthropic

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/vd09-projects/intelligent-tts-narration-library/plan"
)

// sequenceTransport returns a transport that yields responses[i] on the
// i-th call and reuses the last response for any extra call. Calls is
// the call counter shared with the test for assertions.
func sequenceTransport(responses ...*http.Response) (roundTripFunc, *atomic.Int64) {
	var calls atomic.Int64
	return roundTripFunc(func(_ *http.Request) (*http.Response, error) {
		i := int(calls.Add(1) - 1)
		if i >= len(responses) {
			return responses[len(responses)-1], nil
		}
		return responses[i], nil
	}), &calls
}

// resp429 builds a 429 response with optional Retry-After header.
func resp429(retryAfter string) *http.Response {
	r := &http.Response{
		StatusCode: http.StatusTooManyRequests,
		Body:       io.NopCloser(strings.NewReader(`{"error":{"type":"rate_limit","message":"slow down"}}`)),
		Header:     http.Header{},
	}
	if retryAfter != "" {
		r.Header.Set("Retry-After", retryAfter)
	}
	return r
}

// resp200 builds a 200 response with the canned successJSON body.
func resp200() *http.Response {
	return jsonResp(200, successJSON)
}

// captureSleeper returns a sleeper that records each invocation's
// duration into *calls and optionally returns the supplied errors in
// order (one per call). When the call index exceeds len(returns) the
// sleeper returns nil.
func captureSleeper(calls *[]time.Duration, returns ...error) func(context.Context, time.Duration) error {
	return func(_ context.Context, d time.Duration) error {
		i := len(*calls)
		*calls = append(*calls, d)
		if i < len(returns) {
			return returns[i]
		}
		return nil
	}
}

// TestVoice_429ThenSuccess — a single 429 followed by 200 succeeds.
// Asserts 2 HTTP calls + 1 sleeper call.
func TestVoice_429ThenSuccess(t *testing.T) {
	t.Parallel()
	rt, calls := sequenceTransport(resp429("0"), resp200())
	var sleeps []time.Duration
	a := newAdapter(t, rt, WithSleeper(captureSleeper(&sleeps)))

	got, err := a.Voice(context.Background(), proseReq("hello", plan.L2))
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if got.Refused || got.Text != "summary text" {
		t.Errorf("unexpected result: %+v", got)
	}
	if calls.Load() != 2 {
		t.Errorf("HTTP calls: got %d want 2", calls.Load())
	}
	if len(sleeps) != 1 {
		t.Errorf("sleeper calls: got %d want 1", len(sleeps))
	}
}

// TestVoice_429RetryAfterSeconds — integer Retry-After respected.
func TestVoice_429RetryAfterSeconds(t *testing.T) {
	t.Parallel()
	rt, _ := sequenceTransport(resp429("2"), resp200())
	var sleeps []time.Duration
	a := newAdapter(t, rt, WithSleeper(captureSleeper(&sleeps)))

	if _, err := a.Voice(context.Background(), proseReq("hello", plan.L2)); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if len(sleeps) != 1 || sleeps[0] != 2*time.Second {
		t.Errorf("sleeps: got %v want [2s]", sleeps)
	}
}

// TestVoice_429RetryAfterHTTPDate — HTTP-date Retry-After respected
// (loose bound — duration is computed from time.Now()).
func TestVoice_429RetryAfterHTTPDate(t *testing.T) {
	t.Parallel()
	future := time.Now().Add(3 * time.Second).UTC().Format(http.TimeFormat)
	rt, _ := sequenceTransport(resp429(future), resp200())
	var sleeps []time.Duration
	a := newAdapter(t, rt, WithSleeper(captureSleeper(&sleeps)))

	if _, err := a.Voice(context.Background(), proseReq("hello", plan.L2)); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if len(sleeps) != 1 {
		t.Fatalf("sleeps: got %v want 1 entry", sleeps)
	}
	// HTTP-date resolution is whole seconds and time.Now drifts during
	// the test setup; the parsed delta lives in [1s, 4s] (truncation +
	// scheduling slack).
	if sleeps[0] < time.Second || sleeps[0] > 4*time.Second {
		t.Errorf("sleep duration out of range: got %v want [1s, 4s]", sleeps[0])
	}
}

// TestVoice_429NoHeaderExponential — two 429s with no Retry-After use
// the exponential fallback (1s, 2s).
func TestVoice_429NoHeaderExponential(t *testing.T) {
	t.Parallel()
	rt, _ := sequenceTransport(resp429(""), resp429(""), resp200())
	var sleeps []time.Duration
	a := newAdapter(t, rt, WithSleeper(captureSleeper(&sleeps)))

	if _, err := a.Voice(context.Background(), proseReq("hello", plan.L2)); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if len(sleeps) != 2 || sleeps[0] != time.Second || sleeps[1] != 2*time.Second {
		t.Errorf("sleeps: got %v want [1s, 2s]", sleeps)
	}
}

// TestVoice_429Exhausted — 3x 429 returns Go error mentioning 429.
// 3 HTTP calls, 2 sleeper calls.
func TestVoice_429Exhausted(t *testing.T) {
	t.Parallel()
	rt, calls := sequenceTransport(resp429("0"), resp429("0"), resp429("0"))
	var sleeps []time.Duration
	a := newAdapter(t, rt, WithSleeper(captureSleeper(&sleeps)))

	_, err := a.Voice(context.Background(), proseReq("hello", plan.L2))
	if err == nil {
		t.Fatal("expected err after 3x 429, got nil")
	}
	if !strings.Contains(err.Error(), "429") {
		t.Errorf("err should mention 429: %v", err)
	}
	if calls.Load() != 3 {
		t.Errorf("HTTP calls: got %d want 3", calls.Load())
	}
	if len(sleeps) != 2 {
		t.Errorf("sleeper calls: got %d want 2", len(sleeps))
	}
}

// TestVoice_429RetryAfterCappedAt30s — Retry-After 90 capped to 30s.
func TestVoice_429RetryAfterCappedAt30s(t *testing.T) {
	t.Parallel()
	rt, _ := sequenceTransport(resp429("90"), resp200())
	var sleeps []time.Duration
	a := newAdapter(t, rt, WithSleeper(captureSleeper(&sleeps)))

	if _, err := a.Voice(context.Background(), proseReq("hello", plan.L2)); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if len(sleeps) != 1 || sleeps[0] != 30*time.Second {
		t.Errorf("sleeps: got %v want [30s]", sleeps)
	}
}

// TestVoice_429ContextCancelDuringSleep — sleeper returning
// context.Canceled aborts the retry loop without another HTTP call.
func TestVoice_429ContextCancelDuringSleep(t *testing.T) {
	t.Parallel()
	rt, calls := sequenceTransport(resp429("0"), resp200())
	var sleeps []time.Duration
	a := newAdapter(t, rt, WithSleeper(captureSleeper(&sleeps, context.Canceled)))

	_, err := a.Voice(context.Background(), proseReq("hello", plan.L2))
	if err == nil {
		t.Fatal("expected err on ctx cancel, got nil")
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("err should wrap context.Canceled: %v", err)
	}
	if calls.Load() != 1 {
		t.Errorf("HTTP calls: got %d want 1 (no second call after cancel)", calls.Load())
	}
	if len(sleeps) != 1 {
		t.Errorf("sleeper calls: got %d want 1", len(sleeps))
	}
}
