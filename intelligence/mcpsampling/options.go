package mcpsampling

// Option configures an Adapter. Pure data; no I/O is performed in any
// Option closure. Cache and PromptTemplates land in Phase 2 / Phase 4 —
// the options are declared here so the New signature is stable from
// Phase 1.

// Option mutates an Adapter during construction.
type Option func(*Adapter)

// WithClientID overrides the default "unknown" clientID. The clientID
// flows into IntelligenceResult.Model as the prefix
// "mcp-sampling@<clientID>/<actualModel>" and is the first component of
// the cache key (Phase 4). Production wiring in cmd/narrate-mcp passes
// "narrate-mcp".
func WithClientID(id string) Option {
	return func(a *Adapter) {
		if id != "" {
			a.clientID = id
		}
	}
}

// WithMaxTokens overrides the per-level MaxTokens defaults
// (L1=600, L2=1500, L3=3000). A non-positive value for any level leaves
// that level's default in place — letting callers tune only the level
// they care about.
func WithMaxTokens(l1, l2, l3 int64) Option {
	return func(a *Adapter) {
		if l1 > 0 {
			a.maxTokensL1 = l1
		}
		if l2 > 0 {
			a.maxTokensL2 = l2
		}
		if l3 > 0 {
			a.maxTokensL3 = l3
		}
	}
}

// WithTemperature overrides the default sampling temperature (0.2 — low
// to bias toward faithful summarization). The MCP SDK clamps invalid
// values upstream; we pass through.
func WithTemperature(t float64) Option {
	return func(a *Adapter) {
		a.temperature = t
	}
}

// WithPromptTemplates overrides DefaultPromptTemplates with a caller-
// supplied per-class map. Phase 2 defines DefaultPromptTemplates and
// wires the override path; this Option is declared here so the New
// surface is stable across the phase sequence.
//
// Phase 1 stores the override on the Adapter but does not consume it
// (Voice still stubs). The actual storage field lands in Phase 2 when
// prompts.go is added.
func WithPromptTemplates(_ any) Option {
	// Phase 2 will replace the parameter type with
	// map[plan.Class]PromptTemplate and the body will assign to
	// a.promptTemplates. For Phase 1 we keep the surface visible but
	// inert so cmd/narrate-mcp wiring in Phase 5 compiles unchanged.
	return func(_ *Adapter) {}
}

// WithCache injects a custom Cache implementation. Phase 4 defines the
// Cache interface and consumes this option; declared here for the same
// stability reason as WithPromptTemplates.
func WithCache(_ any) Option {
	return func(_ *Adapter) {}
}
