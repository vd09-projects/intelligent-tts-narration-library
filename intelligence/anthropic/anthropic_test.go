package anthropic

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/vd09-projects/intelligent-tts-narration-library/intelligence"
	"github.com/vd09-projects/intelligent-tts-narration-library/plan"
)

// roundTripFunc is the HTTP test seam: a function literal that
// satisfies http.RoundTripper. Tests build an *http.Client with one of
// these as Transport and inject it via WithHTTPClient. No live API
// calls, no httptest.Server overhead. Per Decision v4 (planner-task.md).
type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

// newAdapter is the test ctor shortcut. apiKey is always set (New
// rejects empty). Transport injects the supplied rt.
func newAdapter(t *testing.T, rt roundTripFunc, opts ...Option) *Adapter {
	t.Helper()
	all := append([]Option{
		WithAPIKey("test-key"),
		WithHTTPClient(&http.Client{Transport: rt}),
	}, opts...)
	a, err := New(all...)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return a
}

// proseReq is a minimal valid IntelligenceRequest for the happy path.
// ClassProse is registered in DefaultPromptTemplates so the template-
// lookup branch goes the call path.
func proseReq(text string, level plan.Level) intelligence.IntelligenceRequest {
	return intelligence.IntelligenceRequest{
		BlockText: text,
		Class:     plan.ClassProse,
		Level:     level,
		Locale:    "en",
	}
}

// jsonResp builds a 2xx http.Response carrying body as application/json.
func jsonResp(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Body:       io.NopCloser(strings.NewReader(body)),
		Header:     http.Header{"Content-Type": []string{"application/json"}},
	}
}

// TestVoice_Success — 2xx with a text block returns Text + the
// "anthropic@<resp.Model>" Model string and no error.
func TestVoice_Success(t *testing.T) {
	t.Parallel()
	var calls atomic.Int64
	rt := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		calls.Add(1)
		if r.Header.Get("x-api-key") != "test-key" {
			t.Errorf("missing x-api-key header: got %q", r.Header.Get("x-api-key"))
		}
		if r.Header.Get("anthropic-version") != anthropicVersion {
			t.Errorf("anthropic-version header: got %q want %q", r.Header.Get("anthropic-version"), anthropicVersion)
		}
		if r.URL.String() != apiEndpoint {
			t.Errorf("endpoint: got %q want %q", r.URL.String(), apiEndpoint)
		}
		return jsonResp(200, `{"model":"claude-haiku-4-5-test","content":[{"type":"text","text":"summary text"}],"stop_reason":"end_turn"}`), nil
	})

	got, err := newAdapter(t, rt).Voice(context.Background(), proseReq("hello world", plan.L2))
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if got.Refused {
		t.Fatalf("unexpected Refused=true: note=%q", got.RefusalNote)
	}
	if got.Text != "summary text" {
		t.Errorf("Text: got %q want %q", got.Text, "summary text")
	}
	if got.Model != "anthropic@claude-haiku-4-5-test" {
		t.Errorf("Model: got %q want %q", got.Model, "anthropic@claude-haiku-4-5-test")
	}
	if calls.Load() != 1 {
		t.Errorf("calls: got %d want 1", calls.Load())
	}
}

// TestVoice_RefusalViaSentinel — 2xx whose text begins with the refuse
// sentinel returns Refused=true (data, not error) and strips the
// sentinel from RefusalNote.
func TestVoice_RefusalViaSentinel(t *testing.T) {
	t.Parallel()
	rt := roundTripFunc(func(_ *http.Request) (*http.Response, error) {
		return jsonResp(200, `{"model":"claude-haiku-4-5","content":[{"type":"text","text":"__REFUSE__ block too thin to summarize"}],"stop_reason":"end_turn"}`), nil
	})

	got, err := newAdapter(t, rt).Voice(context.Background(), proseReq("x", plan.L3))
	if err != nil {
		t.Fatalf("unexpected err on refusal path: %v", err)
	}
	if !got.Refused {
		t.Fatalf("Refused: got false, want true (note=%q text=%q)", got.RefusalNote, got.Text)
	}
	if got.Text != "" {
		t.Errorf("Text on refusal should be empty: got %q", got.Text)
	}
	if got.RefusalNote != "block too thin to summarize" {
		t.Errorf("RefusalNote: got %q want %q", got.RefusalNote, "block too thin to summarize")
	}
}

// TestVoice_Unauthorized401 — non-2xx surfaces as a Go error mentioning
// the status code. Refused MUST stay false (the model never spoke).
func TestVoice_Unauthorized401(t *testing.T) {
	t.Parallel()
	rt := roundTripFunc(func(_ *http.Request) (*http.Response, error) {
		return jsonResp(401, `{"type":"error","error":{"type":"authentication_error","message":"invalid api key"}}`), nil
	})

	got, err := newAdapter(t, rt).Voice(context.Background(), proseReq("x", plan.L2))
	if err == nil {
		t.Fatalf("expected err on 401, got nil (result=%+v)", got)
	}
	if !strings.Contains(err.Error(), "401") {
		t.Errorf("err should mention 401: %v", err)
	}
	if got.Refused {
		t.Errorf("Refused should be false on transport error: %+v", got)
	}
}

// TestVoice_BadRequest400 — any non-2xx (not just 401) is a Go error.
// Validates the boundary is on status range, not on specific code.
func TestVoice_BadRequest400(t *testing.T) {
	t.Parallel()
	rt := roundTripFunc(func(_ *http.Request) (*http.Response, error) {
		return jsonResp(400, `{"type":"error","error":{"type":"invalid_request_error","message":"max_tokens too large"}}`), nil
	})

	_, err := newAdapter(t, rt).Voice(context.Background(), proseReq("x", plan.L2))
	if err == nil {
		t.Fatal("expected err on 400, got nil")
	}
	if !strings.Contains(err.Error(), "400") {
		t.Errorf("err should mention 400: %v", err)
	}
}

// TestVoice_MalformedJSON — 2xx with a body that does not parse as
// messagesResponse is a Go error (we can't honestly return content
// from undefined input).
func TestVoice_MalformedJSON(t *testing.T) {
	t.Parallel()
	rt := roundTripFunc(func(_ *http.Request) (*http.Response, error) {
		return jsonResp(200, `{not valid json`), nil
	})

	_, err := newAdapter(t, rt).Voice(context.Background(), proseReq("x", plan.L2))
	if err == nil {
		t.Fatal("expected err on malformed JSON, got nil")
	}
	if !strings.Contains(err.Error(), "decode") {
		t.Errorf("err should mention decode: %v", err)
	}
}

// TestVoice_ContextCancel — when the transport returns ctx.Err()
// (canceled), Voice surfaces it as a Go error. Validates the ctx is
// threaded through to the HTTP request and the cancellation path is
// not silently swallowed.
func TestVoice_ContextCancel(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // pre-cancelled — request will fail synchronously.

	rt := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		select {
		case <-r.Context().Done():
			return nil, r.Context().Err()
		default:
			return jsonResp(200, `{"model":"x","content":[{"type":"text","text":"should not see this"}]}`), nil
		}
	})

	_, err := newAdapter(t, rt).Voice(ctx, proseReq("x", plan.L2))
	if err == nil {
		t.Fatal("expected err on cancelled ctx, got nil")
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("err should wrap context.Canceled: %v", err)
	}
}

// TestVoice_NoTemplateForClass — class without a registered template
// returns Refused=true (honesty: refuse instead of guess) without
// making an HTTP call.
func TestVoice_NoTemplateForClass(t *testing.T) {
	t.Parallel()
	var calls atomic.Int64
	rt := roundTripFunc(func(_ *http.Request) (*http.Response, error) {
		calls.Add(1)
		return jsonResp(200, `{}`), nil
	})

	req := intelligence.IntelligenceRequest{
		BlockText: "x",
		Class:     plan.ClassUnknown, // not in DefaultPromptTemplates
		Level:     plan.L2,
		Locale:    "en",
	}
	got, err := newAdapter(t, rt).Voice(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !got.Refused {
		t.Fatal("Refused: got false, want true for class with no template")
	}
	if !strings.Contains(got.RefusalNote, "unknown") {
		t.Errorf("RefusalNote should mention the class: %q", got.RefusalNote)
	}
	if calls.Load() != 0 {
		t.Errorf("no HTTP call expected on no-template refusal: got %d", calls.Load())
	}
}
