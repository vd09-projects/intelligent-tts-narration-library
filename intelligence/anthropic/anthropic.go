// Package anthropic is the second concrete intelligence.IntelligenceAdapter
// in the project. It talks directly to the Anthropic Messages API
// (POST /v1/messages) using a user-supplied ANTHROPIC_API_KEY, giving
// L2/L3 prose summarization to the CLI entry point cmd/narrate without
// requiring an MCP client in the loop.
//
// Honesty rule (CLAUDE.md, restated): an adapter that cannot voice at the
// requested level returns IntelligenceResult{Refused: true, RefusalNote:"..."}
// — never fabricates. A returned Go error means transport / IO / auth /
// decode failed and the pipeline stops. Refusals are data, errors stop.
//
// Refusal contract: the LLM-side refusal signal is the literal token
// __REFUSE__ as the very first non-whitespace characters of the
// assistant's reply, optionally followed by a short human-readable reason
// after one space. The matching system prompts (shared via
// internal/intelligencetmpl) make this contract part of the LLM's
// instructions and the adapter recognizes the prefix in refusal.go.
//
// Refusals are NOT cached (Phase 4, mirroring mcpsampling). A transient
// prompt issue might refuse once and succeed on retry; caching the
// refusal would poison subsequent attempts.
//
// Composition: this package never imports planner/, never imports cmd/,
// never imports pipeline/. It depends only on plan/, the parent
// intelligence/ package, internal/intelligencetmpl, and the Go standard
// library. Enforced by deps_test.go in Phase 7.
package anthropic

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"

	"github.com/vd09-projects/intelligent-tts-narration-library/intelligence"
	"github.com/vd09-projects/intelligent-tts-narration-library/internal/intelligencetmpl"
	"github.com/vd09-projects/intelligent-tts-narration-library/plan"
)

// errBodyExcerptMax bounds the bytes of upstream error body we splice
// into Go-error strings. Long bodies (HTML 502 pages, etc.) bloat logs
// without adding signal; 512 bytes is enough to carry the canonical
// {error: {type, message}} JSON and a tail of context.
const errBodyExcerptMax = 512

// defaultModel is the production default. Direct-API path, Anthropic's
// smallest current-generation model — cheapest per-token and adequate for
// the L1/L2/L3 summarization workload. Overridable via WithModel.
const defaultModel = "claude-haiku-4-5"

// defaultTemperature biases the model toward faithful summarization. Low
// but nonzero to avoid the determinism brittleness of 0.0. Mirrors the
// mcpsampling default intent (different numeric value because the
// Anthropic API behaves differently than MCP sampling). Overridable via
// WithTemperature.
const defaultTemperature = 0.3

// Default per-level MaxTokens budgets. Mirror the mcpsampling
// per-level intent (L1 small gist, L2 medium summary, L3 longer detail).
// The numeric values differ from mcpsampling because the Anthropic
// Messages API counts tokens differently than MCP sampling and these
// numbers were tuned for the direct-API path specifically. Overridable
// via WithMaxTokens.
const (
	defaultMaxTokensL1 = 80
	defaultMaxTokensL2 = 240
	defaultMaxTokensL3 = 600
)

// Cache is the pluggable interface the adapter uses to skip the HTTP
// call on repeats. Keyed by (content_hash, level, model) per CLAUDE.md's
// caching rule — escalation must not re-bill. The concrete
// implementation, key type, and helpers land in Phase 4 (cache.go); this
// type is forward-declared as an interface here so the Adapter struct
// and WithCache option compile in Phase 2 without depending on
// not-yet-written code.
//
// With cache == nil (the New default), Voice() will skip the wrapper
// entirely once Phase 4 wires it.
type Cache interface {
	// Get / Put will accept a CacheKey value type once Phase 4 introduces
	// it. Defined as any here to keep the interface satisfiable by the
	// not-yet-defined concrete implementation. Phase 4 narrows this.
	Get(key any) (string, bool)
	Put(key any, value string)
}

// Adapter implements intelligence.IntelligenceAdapter against the
// Anthropic Messages API. Construct with New(opts...). Voice() is a stub
// in Phase 2; the real flow lands in Phase 3 (api.go + refusal.go).
type Adapter struct {
	apiKey          string
	model           string
	httpClient      *http.Client
	maxTokens       map[plan.Level]int
	temperature     float64
	promptTemplates map[plan.Class]intelligencetmpl.PromptTemplate
	cache           Cache
}

// Compile-time assertion: *Adapter satisfies intelligence.IntelligenceAdapter.
// If the interface changes or Voice's signature drifts, the build breaks
// here instead of at the call site in pipeline/.
var _ intelligence.IntelligenceAdapter = (*Adapter)(nil)

// New constructs an Adapter. Defaults are applied first, then opts in the
// caller's order. Returns an error when required fields (apiKey, model)
// resolve to empty after applying opts — silently shipping a no-auth
// adapter would surface as a 401 deep inside Voice() instead of as a
// construction-time misconfiguration.
//
// The default httpClient is http.DefaultClient. Tests inject a custom
// transport via WithHTTPClient — see anthropic_test.go in Phase 3.
func New(opts ...Option) (*Adapter, error) {
	a := &Adapter{
		model:       defaultModel,
		httpClient:  http.DefaultClient,
		temperature: defaultTemperature,
		maxTokens: map[plan.Level]int{
			plan.L1: defaultMaxTokensL1,
			plan.L2: defaultMaxTokensL2,
			plan.L3: defaultMaxTokensL3,
		},
		// Shared default template set — the same map mcpsampling uses,
		// so the two adapters cannot drift on prompt wording, honesty
		// preamble, or refusal sentinel. WithPromptTemplates overrides.
		promptTemplates: intelligencetmpl.DefaultPromptTemplates,
	}
	for _, opt := range opts {
		opt(a)
	}
	if a.apiKey == "" {
		return nil, errors.New("anthropic: WithAPIKey is required (got empty key)")
	}
	if a.model == "" {
		return nil, errors.New("anthropic: WithModel produced empty model string")
	}
	if a.httpClient == nil {
		// Guard against WithHTTPClient(nil) — would panic on first Do().
		a.httpClient = http.DefaultClient
	}
	return a, nil
}

// Voice implements intelligence.IntelligenceAdapter.
//
// Flow:
//  1. Pick the per-class prompt template from a.promptTemplates. No
//     template for req.Class → Refused (honest no-template path).
//  2. Render system + user prompts via intelligencetmpl.RenderPrompt.
//  3. Build the messagesRequest, JSON-marshal it.
//  4. POST to apiEndpoint with the Anthropic auth + version headers.
//  5. Non-2xx response → Go error with status + a truncated body
//     excerpt (caller / pipeline classifier sorts retry-vs-fail).
//  6. Decode the body; decode failure → Go error.
//  7. Extract the first text block; no text block → Go error.
//  8. Run parseRefusal on the text; refusal → Refused result (data,
//     not error).
//  9. Otherwise return the text and Model "anthropic@<resp.Model>".
//
// Honesty contract (CLAUDE.md, restated): refusals are data; HTTP /
// auth / decode failures are errors. Phase 5 will wrap the HTTP call
// in doWithRetry for bounded 429 retry; Phase 4 will wrap in cache
// lookup/put. The shape established here is the spine for both.
func (a *Adapter) Voice(ctx context.Context, req intelligence.IntelligenceRequest) (intelligence.IntelligenceResult, error) {
	tpl, ok := a.promptTemplates[req.Class]
	if !ok {
		return intelligence.IntelligenceResult{
			Refused:     true,
			RefusalNote: fmt.Sprintf("no prompt template for class %q", string(req.Class)),
		}, nil
	}

	system, user := intelligencetmpl.RenderPrompt(tpl, req)

	body, err := json.Marshal(messagesRequest{
		Model:       a.model,
		MaxTokens:   a.maxTokens[req.Level],
		Temperature: a.temperature,
		System:      system,
		Messages: []messageInput{
			{Role: "user", Content: user},
		},
	})
	if err != nil {
		// Marshal of plain string-bearing structs fails only under
		// internal package corruption; surface as an error rather than
		// pretending the call happened.
		return intelligence.IntelligenceResult{}, fmt.Errorf("anthropic: marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, apiEndpoint, bytes.NewReader(body))
	if err != nil {
		return intelligence.IntelligenceResult{}, fmt.Errorf("anthropic: build request: %w", err)
	}
	httpReq.Header.Set("x-api-key", a.apiKey)
	httpReq.Header.Set("anthropic-version", anthropicVersion)
	httpReq.Header.Set("content-type", "application/json")

	resp, err := a.httpClient.Do(httpReq)
	if err != nil {
		// %w preserves context.Canceled / context.DeadlineExceeded for
		// downstream classifiers, matching mcpsampling's behavior.
		return intelligence.IntelligenceResult{}, fmt.Errorf("anthropic: http do: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return intelligence.IntelligenceResult{}, fmt.Errorf("anthropic: read response: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return intelligence.IntelligenceResult{}, fmt.Errorf("anthropic: http %d: %s", resp.StatusCode, errBodyExcerpt(respBody))
	}

	var parsed messagesResponse
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return intelligence.IntelligenceResult{}, fmt.Errorf("anthropic: decode response: %w", err)
	}

	text, ok := firstTextBlock(parsed.Content)
	if !ok {
		return intelligence.IntelligenceResult{}, errors.New("anthropic: no text content in response")
	}

	if note, refused := parseRefusal(text); refused {
		// Refusals are NOT cached (Phase 4 will skip Put on this branch).
		return intelligence.IntelligenceResult{
			Refused:     true,
			RefusalNote: note,
		}, nil
	}

	return intelligence.IntelligenceResult{
		Text:  text,
		Model: "anthropic@" + parsed.Model,
	}, nil
}

// errBodyExcerpt returns up to errBodyExcerptMax bytes of body for
// inclusion in error messages. Truncated bodies get a trailing
// "...(truncated)" marker so log readers know there was more.
func errBodyExcerpt(b []byte) string {
	if len(b) <= errBodyExcerptMax {
		return string(b)
	}
	return string(b[:errBodyExcerptMax]) + "...(truncated)"
}
