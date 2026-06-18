// Package intelligence defines the pluggable comprehension contract — the
// narrow seam between the planner and an LLM (or any other agent) that can
// turn raw block text into voiceable narration at a requested level.
//
// Concrete adapters live in subdirectories (planned: mcpsampling, anthropic).
// This package itself stays implementation-free: it only declares the
// interface and the request/result value types.
//
// Invariant (CLAUDE.md): this package imports only the Go standard library
// and plan/. No concrete adapter is referenced here. Enforced by
// deps_test.go.
//
// Honesty rule: an adapter that cannot faithfully voice at the requested
// level MUST return IntelligenceResult{Refused: true, ...} rather than
// fabricate. A Go-level error is reserved for transport/IO failures
// (network down, key invalid, etc.) — those propagate up and stop the
// pipeline. Refusals are data, errors stop.
package intelligence

import (
	"context"

	"github.com/vd09-projects/intelligent-tts-narration-library/plan"
)

// IntelligenceAdapter — the design doc §3.2 contract verbatim:
// "here is a block and the level I want; return voiceable text."
//
// A nil IntelligenceAdapter is a valid state — the planner takes the
// deterministic-degrade path (planner/degrade.go) for prose and any
// structured class whose requested level requires comprehension.
type IntelligenceAdapter interface {
	// Voice returns voiceable text for one block at the requested level.
	//
	// Implementations: MCP-sampling (client LLM via MCP, zero extra cost)
	// or direct-API (Anthropic, OpenAI, etc., user-supplied key).
	//
	// Honesty rule: if the model cannot faithfully voice req.BlockText at
	// req.Level, return IntelligenceResult{Refused: true, RefusalNote:"..."}
	// — never fabricate a gist. A returned Go error is a transport failure
	// (RPC down, auth expired, deadline exceeded) and stops the pipeline;
	// a Refused result is captured as a plan.Block with Status=refused and
	// the plan still renders end-to-end.
	Voice(ctx context.Context, req IntelligenceRequest) (IntelligenceResult, error)
}

// IntelligenceRequest — one block's text and metadata, plus the level the
// caller wants spoken. Facts carries deterministic structural information
// the planner has already extracted (e.g., "yaml dialect, 3 top-level keys")
// so the adapter does not re-derive it and can frame its answer.
type IntelligenceRequest struct {
	// BlockText — the block's raw source text (verbatim).
	BlockText string
	// Class — the planner's classification of the block. Helps the
	// adapter frame its answer (e.g., a code block gets "explain the
	// function" framing rather than "summarize the prose").
	Class plan.Class
	// Facts — deterministic structural facts the planner already
	// extracted. Strings, free-form, terse. Example for a yaml block:
	// ["dialect: yaml", "top_level_keys: 3", "lines: 18"].
	Facts []string
	// Level — 1 | 2 | 3. The level the adapter should target.
	Level plan.Level
	// Locale — BCP-47 tag. Phase one is always "en".
	Locale string
}

// IntelligenceResult — what the adapter returns. Exactly one of (Text non-
// empty) OR (Refused true) is the well-formed case; the planner treats any
// other combination as a refusal.
type IntelligenceResult struct {
	// Text — the voiceable text the renderer should speak. Final words,
	// not raw source. Empty when Refused is true.
	Text string
	// Model — identifier the planner records in Block.Provenance.Model.
	// Free-form. Examples: "claude-3.5-sonnet@2024-12-22",
	// "mcp-sampling@host-llm".
	Model string
	// Refused — true iff the adapter chose not to voice rather than risk
	// fabrication. The planner converts this to plan.RefuseTooLarge (or
	// carries RefusalNote into the message).
	Refused bool
	// RefusalNote — human-surface line, optional. Empty unless Refused.
	RefusalNote string
}
