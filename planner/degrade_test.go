package planner

import (
	"strings"
	"testing"

	"github.com/vd09-projects/intelligent-tts-narration-library/plan"
)

// honestyRuleSatisfied — every Status=refused block must carry
// Spoken=true and a populated SourceMap (Kind set + a non-zero
// line/char/region marker).
func honestyRuleSatisfied(b plan.Block) bool {
	if b.Status != plan.StatusRefused {
		return true
	}
	if b.Refusal == nil || !b.Refusal.Spoken {
		return false
	}
	sm := b.Refusal.SourceMap
	if sm.Kind == "" {
		return false
	}
	if sm.StartLine == 0 && sm.StartChar == 0 && sm.Region == nil {
		return false
	}
	return true
}

func TestDegrade_ProseShortVerbatim(t *testing.T) {
	t.Parallel()
	lex := compileLexicon()
	rb := rawBlock{text: "Short prose.", startLine: 1, endLine: 1, hint: hintProse}
	sm := plan.SourceMap{Kind: plan.SourceKindLineRange, StartLine: 1, EndLine: 1}
	out := level(rb, plan.ClassProse, plan.L1, lex)
	blk, diag := degrade(rb, plan.ClassProse, plan.L1, out, sm, lex)
	if blk.Status != plan.StatusDegraded {
		t.Errorf("short prose without intel should be degraded, got %s", blk.Status)
	}
	if blk.Provenance.VoicedBy != "verbatim" {
		t.Errorf("short prose VoicedBy: want verbatim, got %s", blk.Provenance.VoicedBy)
	}
	if diag == nil || diag.Code != "intelligence_unavailable" {
		t.Errorf("diagnostic missing or wrong code")
	}
}

func TestDegrade_ProseLongRefused(t *testing.T) {
	t.Parallel()
	lex := compileLexicon()
	var b strings.Builder
	for i := 0; i < 150; i++ {
		b.WriteString("word ")
	}
	rb := rawBlock{text: b.String(), startLine: 1, endLine: 5, hint: hintProse}
	sm := plan.SourceMap{Kind: plan.SourceKindLineRange, StartLine: 1, EndLine: 5}
	out := level(rb, plan.ClassProse, plan.L1, lex)
	blk, _ := degrade(rb, plan.ClassProse, plan.L1, out, sm, lex)
	if blk.Status != plan.StatusRefused {
		t.Errorf("long prose without intel should be refused, got %s", blk.Status)
	}
	if blk.Refusal == nil || blk.Refusal.Reason != plan.RefuseNoIntelligence {
		t.Errorf("long prose should refuse RefuseNoIntelligence, got %+v", blk.Refusal)
	}
	if !honestyRuleSatisfied(blk) {
		t.Errorf("honesty rule violated: %+v", blk)
	}
}

func TestDegrade_ImageAlwaysRefused(t *testing.T) {
	t.Parallel()
	lex := compileLexicon()
	rb := rawBlock{text: "![](x.png)", startLine: 7, endLine: 7, hint: hintImage}
	sm := plan.SourceMap{Kind: plan.SourceKindLineRange, StartLine: 7, EndLine: 7}
	out := level(rb, plan.ClassUnknown, plan.L1, lex)
	blk, _ := degrade(rb, plan.ClassUnknown, plan.L1, out, sm, lex)
	if blk.Refusal == nil || blk.Refusal.Reason != plan.RefuseBareImage {
		t.Errorf("image should refuse RefuseBareImage, got %+v", blk.Refusal)
	}
	if !honestyRuleSatisfied(blk) {
		t.Errorf("honesty rule violated: %+v", blk)
	}
}

func TestDegrade_StructuredVoicedNoChange(t *testing.T) {
	t.Parallel()
	lex := compileLexicon()
	rb := rawBlock{text: "```yaml\nkey: 1\n```", hint: hintCode, fenceInfo: "yaml"}
	sm := plan.SourceMap{Kind: plan.SourceKindLineRange, StartLine: 1, EndLine: 3}
	out := level(rb, plan.ClassConfig, plan.L1, lex)
	blk, diag := degrade(rb, plan.ClassConfig, plan.L1, out, sm, lex)
	if blk.Status != plan.StatusVoiced {
		t.Errorf("deterministic L1 config should be voiced, got %s", blk.Status)
	}
	if diag != nil {
		t.Errorf("no diagnostic expected when level already voiced: %+v", diag)
	}
}

func TestDegrade_CodeL3DownshiftsToL1(t *testing.T) {
	t.Parallel()
	lex := compileLexicon()
	rb := rawBlock{text: "```go\nfunc X() {}\n```", hint: hintCode, fenceInfo: "go"}
	sm := plan.SourceMap{Kind: plan.SourceKindLineRange, StartLine: 1, EndLine: 3}
	out := level(rb, plan.ClassCode, plan.L3, lex)
	blk, diag := degrade(rb, plan.ClassCode, plan.L3, out, sm, lex)
	if blk.Status != plan.StatusDegraded {
		t.Errorf("code L3 without intel should be degraded, got %s", blk.Status)
	}
	if diag == nil {
		t.Errorf("expected info diagnostic on downshift")
	}
}
