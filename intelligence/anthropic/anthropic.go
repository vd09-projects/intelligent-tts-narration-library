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
// Dual auth (issue #32): the default authenticates with the x-api-key
// header, the form an Anthropic Console key (sk-ant-api03-) expects.
// A `claude setup-token` subscription OAuth token (sk-ant-oat01-) is
// rejected on x-api-key but accepted on Authorization: Bearer together
// with anthropic-beta: oauth-2025-04-20. Bearer is opt-in via
// WithBearerAuth and is auto-detected on the sk-ant-oat01 token prefix
// (an explicit option always wins over auto-detect). x-api-key stays the
// default and is never removed. Repurposing a subscription token as a
// raw-API credential is a Terms-of-Service gray area — see the caveat on
// WithBearerAuth before opting in.
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
	"strconv"
	"strings"
	"time"

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

// Retry policy constants (Phase 5, Decision v5). Max 2 retries (3 total
// attempts), trigger only on 429, exponential 1s/2s when Retry-After is
// absent, cap each sleep at 30s. No 5xx retry — the Anthropic API treats
// 5xx as "your retry is welcome but I make no promises" so silent retry
// would double the test matrix without a clear win.
const (
	retryMaxAttempts   = 3
	retrySleepCap      = 30 * time.Second
	retryBackoffFirst  = 1 * time.Second
	retryBackoffSecond = 2 * time.Second
)

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
	// authMode selects the credential header (issue #32). Zero value
	// authAPIKey = the default x-api-key path. authModeSet records
	// whether an explicit auth option (WithBearerAuth) was applied so
	// New's prefix auto-detect can defer to an explicit choice.
	authMode    authMode
	authModeSet bool
	// sleeper is the test seam for doWithRetry. Production wiring uses
	// defaultSleeper (time.After + ctx select). Tests inject an instant-
	// returning sleeper via WithSleeper so the suite does not actually
	// wait seconds between 429 retries.
	sleeper func(context.Context, time.Duration) error
}

// defaultSleeper waits d, propagating ctx.Done so cancel does not strand
// a goroutine in time.After. Used by New when WithSleeper is absent.
func defaultSleeper(ctx context.Context, d time.Duration) error {
	select {
	case <-time.After(d):
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
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
//
// Auth mode (issue #32): the default is the x-api-key header. If no
// explicit auth option (WithBearerAuth) was applied and the key carries
// the sk-ant-oat01 subscription-token prefix, New auto-detects Bearer
// auth. An explicit WithBearerAuth always wins over auto-detect — the
// auto-detect runs only when the caller left the auth mode unset.
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
		sleeper:         defaultSleeper,
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
	// Auth-mode auto-detect (issue #32). Only when the caller did not set
	// an explicit auth mode: a subscription-token prefix routes to Bearer.
	// An explicit WithBearerAuth (authModeSet) always wins.
	if !a.authModeSet && strings.HasPrefix(a.apiKey, oatTokenPrefix) {
		a.authMode = authBearer
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

	// Cache lookup (Phase 4) — single-phase: the model is adapter-fixed
	// at construction, so the key is fully knowable pre-call. With
	// a.cache == nil this is a free no-op (state is still populated so
	// cachePut after a successful call can reuse the hash).
	hit, ok, cacheState := a.cacheGet(req)
	if ok {
		// Cache-hit Model uses fullModelString(a.model) — the actual
		// model id is folded into the key, so a hit by definition matched
		// the configured model.
		return intelligence.IntelligenceResult{
			Text:  hit,
			Model: fullModelString(a.model),
		}, nil
	}

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

	statusCode, respBody, err := a.doWithRetry(ctx, body)
	if err != nil {
		return intelligence.IntelligenceResult{}, err
	}

	if statusCode < 200 || statusCode >= 300 {
		return intelligence.IntelligenceResult{}, fmt.Errorf("anthropic: http %d: %s", statusCode, errBodyExcerpt(respBody))
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
		// Refusals are NOT cached — Put is skipped on this branch.
		return intelligence.IntelligenceResult{
			Refused:     true,
			RefusalNote: note,
		}, nil
	}

	a.cachePut(cacheState, text)

	return intelligence.IntelligenceResult{
		Text:  text,
		Model: fullModelString(parsed.Model),
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

// doWithRetry posts body to apiEndpoint up to retryMaxAttempts times,
// retrying only on 429. Returns the final attempt's status code + body
// + nil on a delivered response; returns a Go error on transport
// failure or context cancellation.
//
// Per Decision v5: max 2 retries (3 total attempts), trigger only on
// 429, parse Retry-After (integer seconds first, then HTTP-date), cap
// each sleep at retrySleepCap, exponential fallback (1s, 2s) when no
// header. No 5xx retry. ctx.Done propagates during the sleep — the
// sleeper closure does the select; on cancel we return ctx.Err()
// without making another HTTP call.
//
// Bodies are consumed by Do, so the request is rebuilt from a fresh
// bytes.NewReader on every attempt.
func (a *Adapter) doWithRetry(ctx context.Context, body []byte) (int, []byte, error) {
	var lastStatus int
	var lastBody []byte
	for attempt := 0; attempt < retryMaxAttempts; attempt++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiEndpoint, bytes.NewReader(body))
		if err != nil {
			return 0, nil, fmt.Errorf("anthropic: build request: %w", err)
		}
		// Credential header selection (issue #32). Bearer carries the
		// token in Authorization plus the oauth beta header and omits
		// x-api-key; the default carries x-api-key and omits both. Set
		// (not Add) so a rebuilt request on retry never accumulates
		// duplicate auth headers.
		switch a.authMode {
		case authBearer:
			req.Header.Set("Authorization", "Bearer "+a.apiKey)
			req.Header.Set("anthropic-beta", anthropicBetaOAuth)
		case authAPIKey:
			req.Header.Set("x-api-key", a.apiKey)
		}
		req.Header.Set("anthropic-version", anthropicVersion)
		req.Header.Set("content-type", "application/json")

		resp, err := a.httpClient.Do(req)
		if err != nil {
			// Transport error — no retry, no second guess. %w preserves
			// context.Canceled / context.DeadlineExceeded for downstream
			// classifiers.
			return 0, nil, fmt.Errorf("anthropic: http do: %w", err)
		}
		respBody, readErr := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if readErr != nil {
			return 0, nil, fmt.Errorf("anthropic: read response: %w", readErr)
		}
		lastStatus, lastBody = resp.StatusCode, respBody

		if resp.StatusCode != http.StatusTooManyRequests {
			return lastStatus, lastBody, nil
		}
		if attempt == retryMaxAttempts-1 {
			// 429 on the final attempt — surface the latest body to
			// callers as the error context.
			return lastStatus, lastBody, nil
		}

		dur := retryDelay(resp.Header.Get("Retry-After"), attempt)
		if err := a.sleeper(ctx, dur); err != nil {
			return 0, nil, err
		}
	}
	// Unreachable: the loop returns on every iteration.
	return lastStatus, lastBody, nil
}

// retryDelay parses Retry-After and returns the bounded sleep duration
// for this attempt. Empty / unparseable header → exponential fallback
// keyed on attempt index (0 → 1s, 1 → 2s). Caps at retrySleepCap.
func retryDelay(retryAfter string, attempt int) time.Duration {
	if retryAfter != "" {
		if secs, err := strconv.Atoi(retryAfter); err == nil && secs >= 0 {
			return clampDuration(time.Duration(secs) * time.Second)
		}
		if t, err := http.ParseTime(retryAfter); err == nil {
			delta := time.Until(t)
			if delta < 0 {
				delta = 0
			}
			return clampDuration(delta)
		}
	}
	// Exponential fallback. Only two retries are possible
	// (retryMaxAttempts == 3) so this lookup covers all reachable values.
	switch attempt {
	case 0:
		return retryBackoffFirst
	default:
		return retryBackoffSecond
	}
}

func clampDuration(d time.Duration) time.Duration {
	if d > retrySleepCap {
		return retrySleepCap
	}
	if d < 0 {
		return 0
	}
	return d
}
