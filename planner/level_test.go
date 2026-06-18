package planner

import (
	"strings"
	"testing"

	"github.com/vd09-projects/intelligent-tts-narration-library/plan"
)

func TestLevel_HeadingAllLevels(t *testing.T) {
	t.Parallel()
	lex := compileLexicon()
	rb := rawBlock{text: "## Stack", hint: hintHeading, startLine: 5, endLine: 5}
	for _, lv := range []plan.Level{plan.L1, plan.L2, plan.L3} {
		got := level(rb, plan.ClassHeading, lv, lex)
		if len(got.segments) != 1 || !strings.Contains(got.segments[0].Text, "Section:") {
			t.Errorf("heading L%d: want Section: prefix, got %+v", lv, got)
		}
		if !got.deterministic || got.needsIntelligence {
			t.Errorf("heading should be deterministic, no intelligence needed at L%d", lv)
		}
	}
}

func TestLevel_ListAllLevels(t *testing.T) {
	t.Parallel()
	lex := compileLexicon()
	rb := rawBlock{text: "- alpha\n- beta\n- gamma\n", hint: hintList}
	got := level(rb, plan.ClassList, plan.L1, lex)
	if !strings.Contains(got.segments[0].Text, "3 items") {
		t.Errorf("list count missing: %q", got.segments[0].Text)
	}
	for _, item := range []string{"alpha", "beta", "gamma"} {
		if !strings.Contains(got.segments[0].Text, item) {
			t.Errorf("list missing %s: %q", item, got.segments[0].Text)
		}
	}
}

func TestLevel_CodeL1L2L3(t *testing.T) {
	t.Parallel()
	lex := compileLexicon()
	body := "```go\nfunc One() int { return 1 }\ntype S struct{ X int }\n```"
	rb := rawBlock{text: body, hint: hintCode, fenceInfo: "go"}

	l1 := level(rb, plan.ClassCode, plan.L1, lex)
	if !strings.Contains(l1.segments[0].Text, "Go code block") {
		t.Errorf("code L1 missing lang: %q", l1.segments[0].Text)
	}

	l2 := level(rb, plan.ClassCode, plan.L2, lex)
	if !strings.Contains(l2.segments[0].Text, "Declares") {
		t.Errorf("code L2 missing decls: %q", l2.segments[0].Text)
	}
	if !strings.Contains(l2.segments[0].Text, "One") || !strings.Contains(l2.segments[0].Text, "S") {
		t.Errorf("code L2 missing identifier: %q", l2.segments[0].Text)
	}

	l3 := level(rb, plan.ClassCode, plan.L3, lex)
	if !l3.needsIntelligence {
		t.Errorf("code L3 should need intelligence")
	}
}

func TestLevel_ConfigL1(t *testing.T) {
	t.Parallel()
	lex := compileLexicon()
	body := "```yaml\nreplicas: 3\nname: web\n```"
	rb := rawBlock{text: body, hint: hintCode, fenceInfo: "yaml"}
	got := level(rb, plan.ClassConfig, plan.L1, lex)
	if !strings.Contains(got.segments[0].Text, "YAML config") {
		t.Errorf("config L1: %q", got.segments[0].Text)
	}
	if !strings.Contains(got.segments[0].Text, "2 top-level") {
		t.Errorf("config L1 key count: %q", got.segments[0].Text)
	}
}

func TestLevel_TableL1(t *testing.T) {
	t.Parallel()
	lex := compileLexicon()
	rb := rawBlock{text: "| a | b | c |\n|---|---|---|\n| 1 | 2 | 3 |\n| 4 | 5 | 6 |", hint: hintTable}
	got := level(rb, plan.ClassTable, plan.L1, lex)
	if !strings.Contains(got.segments[0].Text, "3-column") {
		t.Errorf("table L1 cols: %q", got.segments[0].Text)
	}
	if !strings.Contains(got.segments[0].Text, "2-row") {
		t.Errorf("table L1 rows: %q", got.segments[0].Text)
	}
}

func TestLevel_DiagramL1(t *testing.T) {
	t.Parallel()
	lex := compileLexicon()
	body := "```mermaid\ngraph TD\nA-->B\nB-->C\n```"
	rb := rawBlock{text: body, hint: hintCode, fenceInfo: "mermaid"}
	got := level(rb, plan.ClassDiagramAsText, plan.L1, lex)
	if !strings.Contains(got.segments[0].Text, "Mermaid diagram") {
		t.Errorf("diagram L1: %q", got.segments[0].Text)
	}
}

func TestLevel_ProseNeedsIntelligence(t *testing.T) {
	t.Parallel()
	lex := compileLexicon()
	rb := rawBlock{text: "A sentence.", hint: hintProse}
	got := level(rb, plan.ClassProse, plan.L1, lex)
	if !got.needsIntelligence {
		t.Errorf("prose should always need intelligence")
	}
}

func TestLevel_UnknownEmitsRefuseHint(t *testing.T) {
	t.Parallel()
	lex := compileLexicon()
	rb := rawBlock{text: "![](x.png)", hint: hintImage}
	got := level(rb, plan.ClassUnknown, plan.L1, lex)
	if got.refuseHint != plan.RefuseBareImage {
		t.Errorf("unknown class should hint RefuseBareImage, got %q", got.refuseHint)
	}
}

func TestLevel_OversizedProseSplits(t *testing.T) {
	t.Parallel()
	lex := compileLexicon()
	// Build prose well past 800 chars with sentence boundaries.
	var b strings.Builder
	for i := 0; i < 30; i++ {
		b.WriteString("Sentence number ")
		for range 10 {
			b.WriteString("filler ")
		}
		b.WriteString(". ")
	}
	rb := rawBlock{text: b.String(), hint: hintProse}
	got := level(rb, plan.ClassProse, plan.L1, lex)
	if len(got.subBlocks) < 2 {
		t.Errorf("oversized prose should split, got %d sub-blocks", len(got.subBlocks))
	}
}

func TestLevel_OversizedCodeSplitsOnDecls(t *testing.T) {
	t.Parallel()
	lex := compileLexicon()
	var b strings.Builder
	for i := 0; i < 80; i++ {
		b.WriteString("func F")
		b.WriteByte(byte('A' + i%26))
		b.WriteString("() {}\n  // body line\n")
	}
	rb := rawBlock{text: b.String(), hint: hintCode, fenceInfo: "go"}
	got := level(rb, plan.ClassCode, plan.L1, lex)
	if len(got.subBlocks) < 2 {
		t.Errorf("oversized code should split, got %d sub-blocks", len(got.subBlocks))
	}
}
