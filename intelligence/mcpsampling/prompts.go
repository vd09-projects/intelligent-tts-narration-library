package mcpsampling

import (
	"fmt"
	"strings"

	"github.com/vd09-projects/intelligent-tts-narration-library/intelligence"
	"github.com/vd09-projects/intelligent-tts-narration-library/plan"
)

// PromptTemplate is one class's per-level prompt set. System is the
// system prompt shared across all three levels (it states the honesty
// rule and the __REFUSE__ refusal contract — both are level-invariant).
// UserL1/L2/L3 are the user-prompt strings the renderer substitutes the
// block text, class, facts, and locale into per level.
//
// The strings use a tiny substitution vocabulary, applied by
// renderPrompt:
//
//	{{.Class}}     — req.Class string value (e.g. "prose", "code").
//	{{.Level}}     — req.Level numeric form ("1" | "2" | "3").
//	{{.LevelName}} — human-readable level name ("gist" | "summary" | "detail").
//	{{.Locale}}    — req.Locale BCP-47 tag (phase one always "en").
//	{{.Facts}}     — req.Facts joined on "; " (empty if none).
//	{{.BlockText}} — verbatim req.BlockText.
//
// No template-engine dependency on purpose — the substitution set is
// closed and the templates are short, so strings.Replace is simpler and
// keeps the deps invariant clean (we already import plan + intelligence,
// no text/template needed).
type PromptTemplate struct {
	System string
	UserL1 string
	UserL2 string
	UserL3 string
}

// honestySystemPreamble is appended to every per-class System prompt.
// It restates the honesty rule and the __REFUSE__ refusal contract so
// the contract lives in exactly one place. The boundary rule (sentinel
// must be the very first non-whitespace characters) is explicit in the
// prompt so the LLM does not place __REFUSE__ in the middle of a real
// summary by accident — matches the adapter-side parser in Voice()
// (Phase 3) and the documented contract in the package doc.
const honestySystemPreamble = `Honesty rule: never invent details that are not present in the block. ` +
	`If you cannot faithfully summarize at the requested level, respond with the literal token __REFUSE__ as the very first non-whitespace characters of your reply, optionally followed by a short reason after one space. ` +
	`Do not produce a misleading summary. The token __REFUSE__ must not appear anywhere else in your reply. ` +
	`Phase-one locale is fixed to {{.Locale}}; respond in {{.Locale}} only.`

// proseSystem, codeSystem, etc. are the per-class system intros. Each
// ends with honestySystemPreamble.
const (
	proseSystem = `You are a careful narrator. The block is running prose. Speak the gist faithfully at the requested level; never embellish, never invent. ` + honestySystemPreamble

	codeSystem = `You are a careful narrator. The block is source code. Describe what the code does in plain English at the requested level — name the function, its inputs and outputs, and the structural shape. Do not read syntax aloud. Never invent behavior. ` + honestySystemPreamble

	configSystem = `You are a careful narrator. The block is configuration (YAML, TOML, INI, or JSON). Speak the configuration's intent at the requested level — name the top-level keys and their roles, do not enumerate every value. Never invent options not present in the block. ` + honestySystemPreamble

	tableSystem = `You are a careful narrator. The block is tabular data. Speak the table's purpose at the requested level — its columns and what the rows represent. Do not read every cell. Never invent rows or columns. ` + honestySystemPreamble

	diagramSystem = `You are a careful narrator. The block is a diagram-as-text (Mermaid, ASCII diagram, or chart-as-YAML). Speak the diagram's structure at the requested level — its nodes, edges, and overall topology. Never invent relationships not present in the block. ` + honestySystemPreamble

	listSystem = `You are a careful narrator. The block is a bullet or numbered list. Speak the list's items at the requested level — at L1, name what the list enumerates; at higher levels, summarize the items. Never invent items. ` + honestySystemPreamble

	headingSystem = `You are a careful narrator. The block is a section heading. Speak it briefly at the requested level — at L1, read the heading; at higher levels, set the section's scope in one sentence. Never invent scope. ` + honestySystemPreamble

	exampleSystem = `You are a careful narrator. The block is a worked example (sample input/output, demonstration). Speak the example's purpose and outcome at the requested level. Never invent steps or outputs not present in the block. ` + honestySystemPreamble
)

// userL1 / userL2 / userL3 are shared user-prompt skeletons. The
// per-class PromptTemplate plugs the same skeletons in because the
// level instruction is structural, not class-specific — the system
// prompt carries the per-class framing.
const (
	userL1 = `Level 1 ({{.LevelName}}): one sentence that captures what this block is about. No more than 30 words. Class: {{.Class}}. Facts: {{.Facts}}.

Block:
{{.BlockText}}`

	userL2 = `Level 2 ({{.LevelName}}): three to five sentences that summarize this block's content. Cover the main points; skip detail. Class: {{.Class}}. Facts: {{.Facts}}.

Block:
{{.BlockText}}`

	userL3 = `Level 3 ({{.LevelName}}): eight to twelve sentences that walk through this block's content faithfully. Do not invent material to fill the count — if the block is too thin, refuse via the __REFUSE__ token instead. Class: {{.Class}}. Facts: {{.Facts}}.

Block:
{{.BlockText}}`
)

// DefaultPromptTemplates is the frozen, per-class prompt set shipped
// with this adapter. Override via WithPromptTemplates if needed (Phase 2
// wires the storage; Phase 1 declared the option inert).
//
// ClassUnknown is intentionally absent: an unclassified block (the
// planner's catch-all for bare images and opaque regions per
// plan/enums.go) gives the LLM no useful framing, so Voice() refuses
// before calling the client rather than risk fabrication. Any future
// plan.Class addition without a template entry follows the same path —
// adapter refuses honestly rather than guessing.
var DefaultPromptTemplates = map[plan.Class]PromptTemplate{
	plan.ClassProse: {
		System: proseSystem,
		UserL1: userL1, UserL2: userL2, UserL3: userL3,
	},
	plan.ClassCode: {
		System: codeSystem,
		UserL1: userL1, UserL2: userL2, UserL3: userL3,
	},
	plan.ClassConfig: {
		System: configSystem,
		UserL1: userL1, UserL2: userL2, UserL3: userL3,
	},
	plan.ClassTable: {
		System: tableSystem,
		UserL1: userL1, UserL2: userL2, UserL3: userL3,
	},
	plan.ClassDiagramAsText: {
		System: diagramSystem,
		UserL1: userL1, UserL2: userL2, UserL3: userL3,
	},
	plan.ClassList: {
		System: listSystem,
		UserL1: userL1, UserL2: userL2, UserL3: userL3,
	},
	plan.ClassHeading: {
		System: headingSystem,
		UserL1: userL1, UserL2: userL2, UserL3: userL3,
	},
	plan.ClassExample: {
		System: exampleSystem,
		UserL1: userL1, UserL2: userL2, UserL3: userL3,
	},
}

// levelName returns the human-readable name for a plan.Level. Unknown
// values fall back to a numeric form rather than panicking — the
// adapter's caller validates Level upstream.
func levelName(l plan.Level) string {
	switch l {
	case plan.L1:
		return "gist"
	case plan.L2:
		return "summary"
	case plan.L3:
		return "detail"
	default:
		return fmt.Sprintf("level-%d", int(l))
	}
}

// renderPrompt substitutes req into t and returns (system, user). The
// substitution is plain string replacement — see PromptTemplate's doc
// for the variable set. Pure function, no I/O.
//
// If t has no user template for req.Level (e.g. a custom override that
// only supplies UserL1), renderPrompt falls back to UserL2's template,
// then UserL1's. This keeps overrides resilient against partial
// configuration. The default templates always supply all three.
func renderPrompt(t PromptTemplate, req intelligence.IntelligenceRequest) (system, user string) {
	userTpl := pickUserTemplate(t, req.Level)
	subs := map[string]string{
		"{{.Class}}":     string(req.Class),
		"{{.Level}}":     fmt.Sprintf("%d", int(req.Level)),
		"{{.LevelName}}": levelName(req.Level),
		"{{.Locale}}":    req.Locale,
		"{{.Facts}}":     joinFacts(req.Facts),
		"{{.BlockText}}": req.BlockText,
	}
	return substitute(t.System, subs), substitute(userTpl, subs)
}

func pickUserTemplate(t PromptTemplate, l plan.Level) string {
	switch l {
	case plan.L1:
		if t.UserL1 != "" {
			return t.UserL1
		}
	case plan.L2:
		if t.UserL2 != "" {
			return t.UserL2
		}
	case plan.L3:
		if t.UserL3 != "" {
			return t.UserL3
		}
	}
	// Fallback chain: L2 then L1.
	if t.UserL2 != "" {
		return t.UserL2
	}
	return t.UserL1
}

func joinFacts(facts []string) string {
	if len(facts) == 0 {
		return "(none)"
	}
	return strings.Join(facts, "; ")
}

func substitute(tpl string, subs map[string]string) string {
	out := tpl
	for k, v := range subs {
		out = strings.ReplaceAll(out, k, v)
	}
	return out
}
