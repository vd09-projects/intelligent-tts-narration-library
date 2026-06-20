package mcpsampling

import (
	"github.com/vd09-projects/intelligent-tts-narration-library/intelligence"
	"github.com/vd09-projects/intelligent-tts-narration-library/internal/intelligencetmpl"
	"github.com/vd09-projects/intelligent-tts-narration-library/plan"
)

// The prompt-template machinery (PromptTemplate, DefaultPromptTemplates,
// the per-class System constants, the renderer helpers) lives in
// internal/intelligencetmpl since issue #15 Phase 1. It was lifted there
// so a second adapter (intelligence/anthropic) can share it without
// drift.
//
// This file preserves the public symbols mcpsampling already exposed
// (PromptTemplate, DefaultPromptTemplates) via Go type aliases and a
// shared package-level variable. WithPromptTemplates and callers that
// referenced PromptTemplate by name continue to compile and link
// unchanged.
//
// Internal helpers (renderPrompt) stay as thin wrappers so the rest of
// mcpsampling does not have to import the tmpl pkg directly.

// PromptTemplate aliases intelligencetmpl.PromptTemplate so callers of
// WithPromptTemplates continue to use the same name and the public API
// of this package is unchanged.
type PromptTemplate = intelligencetmpl.PromptTemplate

// DefaultPromptTemplates re-exports the shared default template set.
// Held as a package-level variable (not a constant — Go has no map
// constants) so the alias-equivalent in this package points at the
// canonical map without copying.
var DefaultPromptTemplates = intelligencetmpl.DefaultPromptTemplates

// renderPrompt is a thin wrapper over intelligencetmpl.RenderPrompt so
// the rest of mcpsampling (Voice) keeps its existing call site.
func renderPrompt(t PromptTemplate, req intelligence.IntelligenceRequest) (system, user string) {
	return intelligencetmpl.RenderPrompt(t, req)
}

// Compile-time guard: keep the plan import live in this file so that
// future edits removing the alias do not leave an unused import dangling.
var _ plan.Class
