// Package mcpsampling is the first concrete intelligence.IntelligenceAdapter
// in the project. It rides the MCP client LLM via sampling/createMessage
// (server-side *mcp.ServerSession.CreateMessage), giving L2/L3 prose
// summarization "for free" whenever cmd/narrate-mcp is reachable from an
// MCP client (Claude Desktop, the mcp CLI, etc.).
//
// Honesty rule (CLAUDE.md, restated): an adapter that cannot voice at the
// requested level returns IntelligenceResult{Refused: true, RefusalNote:"..."}
// — never fabricates. A returned Go error means transport / IO failed and
// the pipeline stops. Refusals are data, errors stop.
//
// Refusal contract: the LLM-side refusal signal is the literal token
// __REFUSE__ as the very first non-whitespace characters of the assistant's
// reply, optionally followed by a short human-readable reason after one
// space. Adapter recognizes that prefix and returns
// IntelligenceResult{Refused: true, RefusalNote: <reason>}. Any other reply
// is treated as a successful summary. The boundary is explicit: __REFUSE__
// anywhere other than as the leading token is content, not refusal. The
// matching system prompts (in prompts.go) make this contract part of the
// LLM's instructions.
//
// Refusals are NOT cached (Phase 4). The same block text might refuse
// once due to a transient prompt issue and succeed on retry; caching the
// refusal would poison subsequent attempts.
//
// Composition: this package never imports planner/, never imports cmd/,
// never imports pipeline/. It depends only on plan/, the parent
// intelligence/ package, the MCP SDK, and stdlib. Enforced by
// deps_test.go.
package mcpsampling

import (
	"context"
	"fmt"
	"strings"
	"unicode"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/vd09-projects/intelligent-tts-narration-library/intelligence"
	"github.com/vd09-projects/intelligent-tts-narration-library/plan"
)

// refuseSentinel is the literal token the LLM uses to signal refusal.
// Documented in the package doc, restated in every system prompt
// (prompts.go), parsed by parseRefusal below.
const refuseSentinel = "__REFUSE__"

// roleUser is the MCP-SDK role string for the user side of a sampling
// message. Hoisted to a named constant so the call sites stay readable.
const roleUser mcp.Role = "user"

// Adapter implements intelligence.IntelligenceAdapter via the MCP client's
// sampling/createMessage RPC. Construct with New(opts...). The active
// SamplingClient is per-call and threaded through context.Context via
// WithSamplingClient — see client.go for the seam contract.
type Adapter struct {
	maxTokensL1     int64
	maxTokensL2     int64
	maxTokensL3     int64
	temperature     float64
	clientID        string
	promptTemplates map[plan.Class]PromptTemplate
	cache           Cache // Phase 4
}

// Compile-time assertion: *Adapter satisfies intelligence.IntelligenceAdapter.
var _ intelligence.IntelligenceAdapter = (*Adapter)(nil)

// New constructs an Adapter. Defaults follow the plan's coarse heuristic
// (MaxTokens scales with level), use "unknown" as the clientID when
// WithClientID is not supplied, and pull DefaultPromptTemplates from
// prompts.go. The production wiring in cmd/narrate-mcp (Phase 5) passes
// WithClientID("narrate-mcp") and a per-call in-memory cache.
func New(opts ...Option) *Adapter {
	a := &Adapter{
		maxTokensL1:     600,
		maxTokensL2:     1500,
		maxTokensL3:     3000,
		temperature:     0.2,
		clientID:        "unknown",
		promptTemplates: DefaultPromptTemplates,
	}
	for _, opt := range opts {
		opt(a)
	}
	return a
}

// Voice implements intelligence.IntelligenceAdapter.
//
// Flow:
//  1. Extract SamplingClient from ctx (ErrNoSamplingClient if absent).
//  2. Phase 4 cache lookup (skipped if cache nil) using the last-known
//     model for this clientID.
//  3. Select prompt template for req.Class (refuse if no template).
//  4. Render system + user prompts.
//  5. Build CreateMessageParams and call client.CreateMessage(ctx, ...).
//  6. On error: %w-wrap and return (classifier upstream sorts caller-
//     vs internal-error; context.DeadlineExceeded survives the wrap so
//     the existing "cancelled:" bucket catches it).
//  7. On success: extract TextContent.Text. If __REFUSE__ leads after
//     whitespace, strip and return Refused. Otherwise return
//     IntelligenceResult{Text, Model: "mcp-sampling@<clientID>/<actualModel>"}.
//  8. Phase 4 cache Put (skipped if cache nil and never for refusals).
func (a *Adapter) Voice(ctx context.Context, req intelligence.IntelligenceRequest) (intelligence.IntelligenceResult, error) {
	client := samplingClientFromCtx(ctx)
	if client == nil {
		return intelligence.IntelligenceResult{}, ErrNoSamplingClient
	}

	// Phase 4: cache lookup using last-known model for this clientID.
	// First call for any clientID always misses (no last-known model
	// yet), which is documented in cache.go.
	if a.cache != nil {
		if hit, ok := a.cacheGet(req); ok {
			return hit, nil
		}
	}

	// Refuse before call if we have no template for the class. Covers
	// ClassUnknown (the planner's bare-image / opaque-region catch-all)
	// and any future plan.Class addition not yet templated.
	tpl, ok := a.promptTemplates[req.Class]
	if !ok {
		return intelligence.IntelligenceResult{
			Refused:     true,
			RefusalNote: fmt.Sprintf("no prompt template for class %q", string(req.Class)),
		}, nil
	}

	system, user := renderPrompt(tpl, req)

	params := &mcp.CreateMessageParams{
		Messages: []*mcp.SamplingMessage{
			{
				Role:    roleUser,
				Content: &mcp.TextContent{Text: user},
			},
		},
		SystemPrompt: system,
		MaxTokens:    a.maxTokensForLevel(req.Level),
		Temperature:  a.temperature,
	}

	result, err := client.CreateMessage(ctx, params)
	if err != nil {
		// %w preserves context.DeadlineExceeded / context.Canceled so the
		// classifier upstream routes timeouts to the "cancelled:" bucket
		// without a custom sentinel. See plan Decision (v3) + cmd/narrate-mcp
		// classifyPipelineErr.
		return intelligence.IntelligenceResult{}, fmt.Errorf("mcpsampling: createMessage: %w", err)
	}

	text, ok := textFromContent(result.Content)
	if !ok {
		return intelligence.IntelligenceResult{}, ErrUnexpectedContentKind
	}

	if note, isRefusal := parseRefusal(text); isRefusal {
		// Refusals are NOT cached (see package doc).
		return intelligence.IntelligenceResult{
			Refused:     true,
			RefusalNote: note,
		}, nil
	}

	out := intelligence.IntelligenceResult{
		Text:  text,
		Model: fmt.Sprintf("mcp-sampling@%s/%s", a.clientID, result.Model),
	}

	if a.cache != nil {
		a.cachePut(req, out)
	}

	return out, nil
}

// maxTokensForLevel picks the per-level token budget. Falls back to L2
// for unknown level values (defensive — the planner validates upstream).
func (a *Adapter) maxTokensForLevel(l plan.Level) int64 {
	switch l {
	case plan.L1:
		return a.maxTokensL1
	case plan.L2:
		return a.maxTokensL2
	case plan.L3:
		return a.maxTokensL3
	default:
		return a.maxTokensL2
	}
}

// textFromContent extracts the text from an mcp.Content. The MCP spec
// allows text / image / audio in sampling responses; we only know how
// to voice text. Returns (text, true) on success or ("", false) when
// the content is not TextContent.
func textFromContent(c mcp.Content) (string, bool) {
	if c == nil {
		return "", false
	}
	tc, ok := c.(*mcp.TextContent)
	if !ok {
		return "", false
	}
	return tc.Text, true
}

// parseRefusal returns (note, true) if text leads with __REFUSE__ after
// any whitespace, else ("", false). The boundary rule is in the package
// doc + every system prompt: __REFUSE__ anywhere else is content.
func parseRefusal(text string) (string, bool) {
	trimmed := strings.TrimLeftFunc(text, unicode.IsSpace)
	if !strings.HasPrefix(trimmed, refuseSentinel) {
		return "", false
	}
	rest := strings.TrimSpace(strings.TrimPrefix(trimmed, refuseSentinel))
	return rest, true
}
