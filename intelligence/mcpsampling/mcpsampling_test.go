package mcpsampling

import (
	"context"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/vd09-projects/intelligent-tts-narration-library/intelligence"
	"github.com/vd09-projects/intelligent-tts-narration-library/plan"
)

// fakeSamplingClient is the test seam: hands a CreateMessage call to
// the user-supplied fn. The atomic counter is used by the
// parallel-calls test to assert each goroutine reached the client.
type fakeSamplingClient struct {
	calls atomic.Int64
	fn    func(ctx context.Context, params *mcp.CreateMessageParams) (*mcp.CreateMessageResult, error)
}

func (f *fakeSamplingClient) CreateMessage(ctx context.Context, params *mcp.CreateMessageParams) (*mcp.CreateMessageResult, error) {
	f.calls.Add(1)
	return f.fn(ctx, params)
}

func newReq(text string, level plan.Level) intelligence.IntelligenceRequest {
	return intelligence.IntelligenceRequest{
		BlockText: text,
		Class:     plan.ClassProse,
		Level:     level,
		Locale:    "en",
	}
}

// TestVoice_SuccessPath_ReturnsTextAndCombinedModelString — happy path:
// fake returns plain text, adapter returns IntelligenceResult{Text,
// Model: "mcp-sampling@<clientID>/<actualModel>"}.
func TestVoice_SuccessPath_ReturnsTextAndCombinedModelString(t *testing.T) {
	t.Parallel()
	fake := &fakeSamplingClient{
		fn: func(_ context.Context, _ *mcp.CreateMessageParams) (*mcp.CreateMessageResult, error) {
			return &mcp.CreateMessageResult{
				Content: &mcp.TextContent{Text: "the gist"},
				Model:   "claude-haiku-fake",
			}, nil
		},
	}
	a := New(WithClientID("test-client"))
	ctx := WithSamplingClient(context.Background(), fake)

	got, err := a.Voice(ctx, newReq("hello world", plan.L2))
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if got.Refused {
		t.Fatalf("unexpected Refused=true; note=%q", got.RefusalNote)
	}
	if got.Text != "the gist" {
		t.Errorf("Text mismatch: got %q want %q", got.Text, "the gist")
	}
	if got.Model != "mcp-sampling@test-client/claude-haiku-fake" {
		t.Errorf("Model mismatch: got %q", got.Model)
	}
	if fake.calls.Load() != 1 {
		t.Errorf("CreateMessage call count: got %d want 1", fake.calls.Load())
	}
}

// TestVoice_RefusalViaSentinel — fake returns __REFUSE__ at the start;
// adapter strips and returns Refused=true with the trailing reason.
func TestVoice_RefusalViaSentinel(t *testing.T) {
	t.Parallel()
	fake := &fakeSamplingClient{
		fn: func(_ context.Context, _ *mcp.CreateMessageParams) (*mcp.CreateMessageResult, error) {
			return &mcp.CreateMessageResult{
				Content: &mcp.TextContent{Text: "__REFUSE__ contains sensitive data"},
				Model:   "anymodel",
			}, nil
		},
	}
	a := New()
	got, err := a.Voice(WithSamplingClient(context.Background(), fake), newReq("blk", plan.L2))
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !got.Refused {
		t.Fatalf("expected Refused=true; got %+v", got)
	}
	if got.RefusalNote != "contains sensitive data" {
		t.Errorf("RefusalNote: got %q want %q", got.RefusalNote, "contains sensitive data")
	}
	if got.Text != "" {
		t.Errorf("Refused result must have empty Text, got %q", got.Text)
	}
}

// TestVoice_RefusalSentinelNotAtStartIsContent — boundary rule: the
// token must be the first non-whitespace characters. Anywhere else is
// content, not refusal.
func TestVoice_RefusalSentinelNotAtStartIsContent(t *testing.T) {
	t.Parallel()
	body := "Here is the gist. __REFUSE__ is a token used internally."
	fake := &fakeSamplingClient{
		fn: func(_ context.Context, _ *mcp.CreateMessageParams) (*mcp.CreateMessageResult, error) {
			return &mcp.CreateMessageResult{
				Content: &mcp.TextContent{Text: body},
				Model:   "anymodel",
			}, nil
		},
	}
	a := New()
	got, err := a.Voice(WithSamplingClient(context.Background(), fake), newReq("blk", plan.L2))
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if got.Refused {
		t.Fatalf("expected Refused=false (sentinel mid-body); got Refused=true note=%q", got.RefusalNote)
	}
	if got.Text != body {
		t.Errorf("Text mismatch: got %q want %q", got.Text, body)
	}
}

// TestVoice_RefusalSentinelWithLeadingWhitespace — leading whitespace
// is trimmed before the sentinel check (per the package doc and the
// parseRefusal contract).
func TestVoice_RefusalSentinelWithLeadingWhitespace(t *testing.T) {
	t.Parallel()
	fake := &fakeSamplingClient{
		fn: func(_ context.Context, _ *mcp.CreateMessageParams) (*mcp.CreateMessageResult, error) {
			return &mcp.CreateMessageResult{
				Content: &mcp.TextContent{Text: "  \n\t__REFUSE__ too thin"},
				Model:   "anymodel",
			}, nil
		},
	}
	a := New()
	got, err := a.Voice(WithSamplingClient(context.Background(), fake), newReq("blk", plan.L3))
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !got.Refused {
		t.Fatalf("expected Refused=true after leading-whitespace trim; got %+v", got)
	}
	if got.RefusalNote != "too thin" {
		t.Errorf("RefusalNote: got %q want %q", got.RefusalNote, "too thin")
	}
}

// TestVoice_TransportError_WrappedReturnedNotRefusal — fake returns a
// Go error; adapter %w-wraps and returns the error, never a refusal.
// Errors stop the pipeline (CLAUDE.md honesty rule).
func TestVoice_TransportError_WrappedReturnedNotRefusal(t *testing.T) {
	t.Parallel()
	rpcErr := errors.New("rpc closed")
	fake := &fakeSamplingClient{
		fn: func(_ context.Context, _ *mcp.CreateMessageParams) (*mcp.CreateMessageResult, error) {
			return nil, rpcErr
		},
	}
	a := New()
	got, err := a.Voice(WithSamplingClient(context.Background(), fake), newReq("blk", plan.L1))
	if err == nil {
		t.Fatalf("expected error, got result %+v", got)
	}
	if !errors.Is(err, rpcErr) {
		t.Errorf("error chain broken: %v", err)
	}
	if !strings.Contains(err.Error(), "mcpsampling: createMessage:") {
		t.Errorf("missing wrap prefix; got %q", err.Error())
	}
}

// TestVoice_TimeoutPreservesDeadlineExceeded — context.DeadlineExceeded
// survives the %w wrap so the cmd/narrate-mcp classifier's existing
// "cancelled:" bucket catches it without a custom sentinel (S3).
func TestVoice_TimeoutPreservesDeadlineExceeded(t *testing.T) {
	t.Parallel()
	fake := &fakeSamplingClient{
		fn: func(_ context.Context, _ *mcp.CreateMessageParams) (*mcp.CreateMessageResult, error) {
			return nil, context.DeadlineExceeded
		},
	}
	a := New()
	_, err := a.Voice(WithSamplingClient(context.Background(), fake), newReq("blk", plan.L1))
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected errors.Is(err, context.DeadlineExceeded), got %v", err)
	}
}

// TestVoice_NoClientInCtx_ReturnsSentinelError — invoke with bare ctx;
// adapter returns ErrNoSamplingClient so the classifier can route to
// internal_error: (operator bug, not a caller bug).
func TestVoice_NoClientInCtx_ReturnsSentinelError(t *testing.T) {
	t.Parallel()
	a := New()
	_, err := a.Voice(context.Background(), newReq("blk", plan.L1))
	if !errors.Is(err, ErrNoSamplingClient) {
		t.Fatalf("expected ErrNoSamplingClient, got %v", err)
	}
}

// TestVoice_UnsupportedClass_RefusesWithoutCallingClient — ClassUnknown
// has no template entry; adapter refuses before calling the client.
// Use a panicking fake to prove the client was not reached.
func TestVoice_UnsupportedClass_RefusesWithoutCallingClient(t *testing.T) {
	t.Parallel()
	fake := &fakeSamplingClient{
		fn: func(_ context.Context, _ *mcp.CreateMessageParams) (*mcp.CreateMessageResult, error) {
			t.Fatalf("client must not be called for ClassUnknown")
			return nil, nil
		},
	}
	a := New()
	req := newReq("blk", plan.L1)
	req.Class = plan.ClassUnknown
	got, err := a.Voice(WithSamplingClient(context.Background(), fake), req)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !got.Refused {
		t.Fatalf("expected Refused=true for ClassUnknown; got %+v", got)
	}
	if !strings.Contains(got.RefusalNote, "no prompt template") {
		t.Errorf("RefusalNote missing template hint: %q", got.RefusalNote)
	}
}

// TestVoice_WrongContentKind_ReturnsErrUnexpectedContentKind — fake
// returns image content; adapter returns the sentinel.
func TestVoice_WrongContentKind_ReturnsErrUnexpectedContentKind(t *testing.T) {
	t.Parallel()
	fake := &fakeSamplingClient{
		fn: func(_ context.Context, _ *mcp.CreateMessageParams) (*mcp.CreateMessageResult, error) {
			return &mcp.CreateMessageResult{
				Content: &mcp.ImageContent{Data: []byte("png"), MIMEType: "image/png"},
				Model:   "anymodel",
			}, nil
		},
	}
	a := New()
	_, err := a.Voice(WithSamplingClient(context.Background(), fake), newReq("blk", plan.L2))
	if !errors.Is(err, ErrUnexpectedContentKind) {
		t.Fatalf("expected ErrUnexpectedContentKind, got %v", err)
	}
}

// TestVoice_ParallelCallsShareCtxSession — two goroutines invoke
// Voice() with the same ctx (carrying the same fake). Both calls must
// reach CreateMessage (count == 2). Catches accidental adapter-state
// sharing if a future refactor stores per-call state on *Adapter.
// (Per S1.)
func TestVoice_ParallelCallsShareCtxSession(t *testing.T) {
	t.Parallel()
	fake := &fakeSamplingClient{
		fn: func(_ context.Context, _ *mcp.CreateMessageParams) (*mcp.CreateMessageResult, error) {
			return &mcp.CreateMessageResult{
				Content: &mcp.TextContent{Text: "summary"},
				Model:   "anymodel",
			}, nil
		},
	}
	a := New()
	ctx := WithSamplingClient(context.Background(), fake)

	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := a.Voice(ctx, newReq("blk", plan.L2)); err != nil {
				t.Errorf("Voice failed: %v", err)
			}
		}()
	}
	wg.Wait()

	if got := fake.calls.Load(); got != 2 {
		t.Errorf("CreateMessage call count: got %d want 2", got)
	}
}

// TestVoice_PromptsThreadingSpot_Sanity — the fake inspects the
// CreateMessageParams it receives to confirm SystemPrompt + Messages
// were rendered (not blank). Light contract test — full prompt content
// is covered by prompts_test.go.
func TestVoice_PromptsThreadingSpot_Sanity(t *testing.T) {
	t.Parallel()
	var seenSystem, seenUser string
	fake := &fakeSamplingClient{
		fn: func(_ context.Context, params *mcp.CreateMessageParams) (*mcp.CreateMessageResult, error) {
			seenSystem = params.SystemPrompt
			if len(params.Messages) > 0 {
				if tc, ok := params.Messages[0].Content.(*mcp.TextContent); ok {
					seenUser = tc.Text
				}
			}
			return &mcp.CreateMessageResult{
				Content: &mcp.TextContent{Text: "ok"},
				Model:   "anymodel",
			}, nil
		},
	}
	a := New()
	req := newReq("BLOCK_MARKER_99", plan.L1)
	if _, err := a.Voice(WithSamplingClient(context.Background(), fake), req); err != nil {
		t.Fatalf("Voice err: %v", err)
	}
	if !strings.Contains(seenSystem, "__REFUSE__") {
		t.Errorf("system prompt missing refusal contract: %q", seenSystem)
	}
	if !strings.Contains(seenUser, "BLOCK_MARKER_99") {
		t.Errorf("user prompt missing block text: %q", seenUser)
	}
}
