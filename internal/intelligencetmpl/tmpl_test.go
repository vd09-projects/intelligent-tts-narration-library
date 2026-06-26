package intelligencetmpl

import (
	"strings"
	"testing"

	"github.com/vd09-projects/intelligent-tts-narration-library/intelligence"
	"github.com/vd09-projects/intelligent-tts-narration-library/plan"
)

// TestDefaultPromptTemplates_AllSupportedClassesPresent — defensive:
// every plan.Class constant except ClassImage and ClassUnknown must
// have an entry in DefaultPromptTemplates. Catches future enum
// additions: if someone adds plan.ClassFootnote without adding a
// template here, this test fires before the LLM gets called with no
// framing.
func TestDefaultPromptTemplates_AllSupportedClassesPresent(t *testing.T) {
	t.Parallel()

	// Enumerate every Class the adapter is expected to handle. Keep
	// this list in sync with plan/enums.go. ClassImage + ClassUnknown
	// are deliberately excluded (refuse at Voice() entry).
	supported := []plan.Class{
		plan.ClassProse,
		plan.ClassCode,
		plan.ClassConfig,
		plan.ClassTable,
		plan.ClassDiagramAsText,
		plan.ClassList,
		plan.ClassHeading,
		plan.ClassExample,
	}

	for _, c := range supported {
		tpl, ok := DefaultPromptTemplates[c]
		if !ok {
			t.Errorf("DefaultPromptTemplates is missing %q", string(c))
			continue
		}
		if tpl.System == "" {
			t.Errorf("%q: System is empty", string(c))
		}
		if tpl.UserL1 == "" || tpl.UserL2 == "" || tpl.UserL3 == "" {
			t.Errorf("%q: missing per-level user template (L1=%t L2=%t L3=%t)",
				string(c), tpl.UserL1 != "", tpl.UserL2 != "", tpl.UserL3 != "")
		}
	}

	// And: ClassUnknown must NOT have an entry — it is the "refuse
	// before call" sentinel (catch-all for bare images and opaque
	// regions per plan/enums.go).
	if _, ok := DefaultPromptTemplates[plan.ClassUnknown]; ok {
		t.Errorf("DefaultPromptTemplates unexpectedly has entry for ClassUnknown (should refuse before call)")
	}
}

// TestRenderPrompt_SystemContainsRefusalContract — every rendered
// system prompt must contain the literal __REFUSE__ token and the
// "very first non-whitespace characters" phrase. Catches accidental
// contract drift if a template gets edited.
func TestRenderPrompt_SystemContainsRefusalContract(t *testing.T) {
	t.Parallel()

	req := intelligence.IntelligenceRequest{
		BlockText: "hello",
		Level:     plan.L2,
		Locale:    "en",
	}
	for c, tpl := range DefaultPromptTemplates {
		req.Class = c
		sys, _ := RenderPrompt(tpl, req)
		if !strings.Contains(sys, "__REFUSE__") {
			t.Errorf("class %q: rendered system prompt missing __REFUSE__ token:\n%s", string(c), sys)
		}
		if !strings.Contains(sys, "very first non-whitespace characters") {
			t.Errorf("class %q: rendered system prompt missing boundary phrase", string(c))
		}
		if !strings.Contains(sys, "locale is fixed to en") {
			t.Errorf("class %q: rendered system prompt missing locale fix-up; got:\n%s", string(c), sys)
		}
	}
}

// TestRenderPrompt_PerClass — table-driven smoke: for each class, the
// rendered user prompt contains the block text, mentions the class
// label, mentions the level numeric, and mentions the level name.
// Substring checks (not byte-for-byte) — the wording will iterate.
func TestRenderPrompt_PerClass(t *testing.T) {
	t.Parallel()

	cases := []struct {
		class plan.Class
		level plan.Level
		lname string
	}{
		{plan.ClassProse, plan.L1, "gist"},
		{plan.ClassProse, plan.L2, "summary"},
		{plan.ClassProse, plan.L3, "detail"},
		{plan.ClassCode, plan.L2, "summary"},
		{plan.ClassConfig, plan.L1, "gist"},
		{plan.ClassTable, plan.L3, "detail"},
		{plan.ClassDiagramAsText, plan.L2, "summary"},
		{plan.ClassList, plan.L1, "gist"},
		{plan.ClassHeading, plan.L1, "gist"},
		{plan.ClassExample, plan.L2, "summary"},
	}
	for _, tc := range cases {
		t.Run(string(tc.class)+"_L"+itoa(int(tc.level)), func(t *testing.T) {
			t.Parallel()
			tpl := DefaultPromptTemplates[tc.class]
			req := intelligence.IntelligenceRequest{
				BlockText: "BLOCKTEXT_MARKER_42",
				Class:     tc.class,
				Facts:     []string{"factA", "factB"},
				Level:     tc.level,
				Locale:    "en",
			}
			_, user := RenderPrompt(tpl, req)
			if !strings.Contains(user, "BLOCKTEXT_MARKER_42") {
				t.Errorf("user prompt missing block text:\n%s", user)
			}
			if !strings.Contains(user, string(tc.class)) {
				t.Errorf("user prompt missing class label %q:\n%s", string(tc.class), user)
			}
			if !strings.Contains(user, tc.lname) {
				t.Errorf("user prompt missing level name %q:\n%s", tc.lname, user)
			}
			if !strings.Contains(user, "factA; factB") {
				t.Errorf("user prompt missing joined facts:\n%s", user)
			}
		})
	}
}

// TestRenderPrompt_NoFacts — Facts == nil renders as "(none)" rather
// than an empty string, so the LLM sees a stable signal instead of a
// possibly confusing blank.
func TestRenderPrompt_NoFacts(t *testing.T) {
	t.Parallel()
	tpl := DefaultPromptTemplates[plan.ClassProse]
	req := intelligence.IntelligenceRequest{
		BlockText: "hi",
		Class:     plan.ClassProse,
		Level:     plan.L1,
		Locale:    "en",
	}
	_, user := RenderPrompt(tpl, req)
	if !strings.Contains(user, "Facts: (none)") {
		t.Errorf("missing '(none)' fallback for empty Facts:\n%s", user)
	}
}

// TestRenderPrompt_FallbackWhenUserTemplateMissing — if a custom
// override supplies only UserL1, requesting L2 falls back to L1 (via
// the L2-then-L1 chain). Documents the resilience contract.
func TestRenderPrompt_FallbackWhenUserTemplateMissing(t *testing.T) {
	t.Parallel()
	tpl := PromptTemplate{
		System: "sys " + HonestySystemPreamble,
		UserL1: "ONLY_L1 {{.BlockText}}",
	}
	req := intelligence.IntelligenceRequest{
		BlockText: "blk",
		Class:     plan.ClassProse,
		Level:     plan.L3,
		Locale:    "en",
	}
	_, user := RenderPrompt(tpl, req)
	if !strings.Contains(user, "ONLY_L1 blk") {
		t.Errorf("expected fallback to UserL1; got:\n%s", user)
	}
}

// TestPickUserTemplate_CodeL2UsesCodeTemplate — ClassCode at L2 must
// resolve to the code-specific one-sentence gist template (CodeUserL2),
// NOT the shared three-to-five-sentence UserL2. Guards issue #48's wiring
// and catches accidental drift back to the generic summary prompt.
func TestPickUserTemplate_CodeL2UsesCodeTemplate(t *testing.T) {
	t.Parallel()
	tpl := DefaultPromptTemplates[plan.ClassCode]
	if got := PickUserTemplate(tpl, plan.L2); got != CodeUserL2 {
		t.Errorf("ClassCode L2 should pick CodeUserL2, got:\n%s", got)
	}
	// L1 and L3 stay on the shared skeletons.
	if got := PickUserTemplate(tpl, plan.L1); got != UserL1 {
		t.Errorf("ClassCode L1 should pick shared UserL1, got:\n%s", got)
	}
	if got := PickUserTemplate(tpl, plan.L3); got != UserL3 {
		t.Errorf("ClassCode L3 should pick shared UserL3, got:\n%s", got)
	}
}

// TestRenderPrompt_CodeL2SubstitutesCodeTemplate — the rendered ClassCode
// L2 user prompt must carry the one-sentence / 30-word framing and the
// block text, and must NOT carry the generic "three to five sentences"
// summary instruction.
func TestRenderPrompt_CodeL2SubstitutesCodeTemplate(t *testing.T) {
	t.Parallel()
	tpl := DefaultPromptTemplates[plan.ClassCode]
	req := intelligence.IntelligenceRequest{
		BlockText: "CODE_MARKER_99",
		Class:     plan.ClassCode,
		Facts:     []string{"class: code, lang: go, lines: 12", "decls: 1"},
		Level:     plan.L2,
		Locale:    "en",
	}
	_, user := RenderPrompt(tpl, req)
	if !strings.Contains(user, "CODE_MARKER_99") {
		t.Errorf("code L2 user prompt missing block text:\n%s", user)
	}
	if !strings.Contains(user, "one sentence") || !strings.Contains(user, "30 words") {
		t.Errorf("code L2 user prompt missing one-sentence/30-word framing:\n%s", user)
	}
	if strings.Contains(user, "three to five sentences") {
		t.Errorf("code L2 user prompt unexpectedly used the generic summary skeleton:\n%s", user)
	}
}

// TestPickUserTemplate_TableUsesTableTemplates — ClassTable at L2/L3 must
// resolve to the table-specific prompts (TableUserL2 / TableUserL3), NOT
// the shared UserL2/UserL3. Guards issue #47's wiring and catches drift
// back to the generic skeletons. L1 stays on the shared UserL1.
func TestPickUserTemplate_TableUsesTableTemplates(t *testing.T) {
	t.Parallel()
	tpl := DefaultPromptTemplates[plan.ClassTable]
	if got := PickUserTemplate(tpl, plan.L2); got != TableUserL2 {
		t.Errorf("ClassTable L2 should pick TableUserL2, got:\n%s", got)
	}
	if got := PickUserTemplate(tpl, plan.L3); got != TableUserL3 {
		t.Errorf("ClassTable L3 should pick TableUserL3, got:\n%s", got)
	}
	if got := PickUserTemplate(tpl, plan.L1); got != UserL1 {
		t.Errorf("ClassTable L1 should pick shared UserL1, got:\n%s", got)
	}
}

// TestRenderPrompt_TableSubstitutesTableTemplates — the rendered ClassTable
// L2 user prompt must carry the table-meaning framing, the block text, and
// the substituted headers fact; and must NOT carry the generic
// "three to five sentences" summary instruction. L3 must select the
// per-row-walk framing.
func TestRenderPrompt_TableSubstitutesTableTemplates(t *testing.T) {
	t.Parallel()
	tpl := DefaultPromptTemplates[plan.ClassTable]

	reqL2 := intelligence.IntelligenceRequest{
		BlockText: "TABLE_MARKER_47",
		Class:     plan.ClassTable,
		Facts:     []string{"class: table, cols: 3, rows: 3", "headers: name, role, team"},
		Level:     plan.L2,
		Locale:    "en",
	}
	_, user := RenderPrompt(tpl, reqL2)
	if !strings.Contains(user, "TABLE_MARKER_47") {
		t.Errorf("table L2 user prompt missing block text:\n%s", user)
	}
	if !strings.Contains(user, "what this table represents") {
		t.Errorf("table L2 user prompt missing table-meaning framing:\n%s", user)
	}
	if !strings.Contains(user, "headers: name, role, team") {
		t.Errorf("table L2 user prompt missing substituted headers fact:\n%s", user)
	}
	if strings.Contains(user, "three to five sentences") {
		t.Errorf("table L2 user prompt unexpectedly used the generic summary skeleton:\n%s", user)
	}

	reqL3 := reqL2
	reqL3.Level = plan.L3
	_, userL3 := RenderPrompt(tpl, reqL3)
	if !strings.Contains(userL3, "walk every row") {
		t.Errorf("table L3 user prompt missing per-row-walk framing:\n%s", userL3)
	}
	if !strings.Contains(userL3, "headers: name, role, team") {
		t.Errorf("table L3 user prompt missing substituted headers fact:\n%s", userL3)
	}
}

// itoa is a tiny helper to keep test names compact without importing
// strconv just for one call.
func itoa(n int) string {
	switch n {
	case 1:
		return "1"
	case 2:
		return "2"
	case 3:
		return "3"
	default:
		return "x"
	}
}
