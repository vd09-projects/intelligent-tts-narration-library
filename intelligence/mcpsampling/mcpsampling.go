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
// matching system prompts (in prompts.go, Phase 2) make this contract part
// of the LLM's instructions.
//
// Refusals are NOT cached. The same block text might refuse once due to a
// transient prompt issue and succeed on retry; caching the refusal would
// poison subsequent attempts.
//
// Composition: this package never imports planner/, never imports cmd/,
// never imports pipeline/. It depends only on plan/, the parent
// intelligence/ package, the MCP SDK, and stdlib. Enforced by
// deps_test.go.
package mcpsampling

import (
	"context"

	"github.com/vd09-projects/intelligent-tts-narration-library/intelligence"
)

// Adapter implements intelligence.IntelligenceAdapter via the MCP client's
// sampling/createMessage RPC. Construct with New(opts...). The active
// SamplingClient is per-call and threaded through context.Context via
// WithSamplingClient — see Phase 3 for the seam contract.
//
// Phase 1 ships a stub Voice() that always refuses with "not implemented".
// Phase 3 replaces the body with the real CreateMessage call.
type Adapter struct {
	// Options. Wired by New + Option closures.
	maxTokensL1 int64
	maxTokensL2 int64
	maxTokensL3 int64
	temperature float64
	clientID    string
}

// Compile-time assertion: *Adapter satisfies intelligence.IntelligenceAdapter.
var _ intelligence.IntelligenceAdapter = (*Adapter)(nil)

// New constructs an Adapter. Defaults follow the plan's coarse heuristic
// (MaxTokens scales with level) and use "unknown" as the clientID when
// WithClientID is not supplied — the production wiring in cmd/narrate-mcp
// passes WithClientID("narrate-mcp").
func New(opts ...Option) *Adapter {
	a := &Adapter{
		maxTokensL1: 600,
		maxTokensL2: 1500,
		maxTokensL3: 3000,
		temperature: 0.2,
		clientID:    "unknown",
	}
	for _, opt := range opts {
		opt(a)
	}
	return a
}

// Voice is the intelligence.IntelligenceAdapter implementation.
//
// Phase 1 (this file, current state) returns Refused with note "not
// implemented" so the package compiles and the interface conformance test
// passes. Phase 3 replaces this body with the real flow:
//
//  1. Extract SamplingClient from ctx (or return ErrNoSamplingClient).
//  2. Select prompt template for req.Class.
//  3. Build CreateMessageParams (system + user + max_tokens by level).
//  4. Call client.CreateMessage(ctx, params).
//  5. Inspect result.Content (TextContent expected); parse __REFUSE__
//     sentinel if present.
//  6. Return IntelligenceResult{Text, Model: "mcp-sampling@<clientID>/<actualModel>"}.
func (a *Adapter) Voice(_ context.Context, _ intelligence.IntelligenceRequest) (intelligence.IntelligenceResult, error) {
	return intelligence.IntelligenceResult{
		Refused:     true,
		RefusalNote: "mcpsampling: not implemented (Phase 1 scaffold)",
	}, nil
}
