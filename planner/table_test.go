package planner

import (
	"context"
	"strings"
	"testing"

	"github.com/vd09-projects/intelligent-tts-narration-library/intelligence"
	"github.com/vd09-projects/intelligent-tts-narration-library/plan"
)

// TestPlan_TableL2_AdapterRefuses_Refused — MANDATORY honesty-rule case
// (issue #47): a table asked at L2 with an adapter PRESENT that DECLINES
// (Refused) must land Status=refused with Spoken=true + a populated
// SourceMap, exercising the live callIntelligence res.Refused path (not
// buildRefusal directly). Distinct from the no-adapter degraded case so the
// two honesty outcomes are never conflated.
func TestPlan_TableL2_AdapterRefuses_Refused(t *testing.T) {
	t.Parallel()
	seams := deterministicSeams(t)
	intel := &scriptedIntel{replies: []intelligence.IntelligenceResult{
		{Refused: true, RefusalNote: "table too irregular to summarize faithfully", Model: "scripted@test"},
	}}
	src := "| name | role |\n|---|---|\n| Alice | lead |\n"
	got, err := Plan(context.Background(), stubDoc(src, "table.md"),
		Request{Level: plan.L2}, intel, seams...)
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	if intel.calls != 1 {
		t.Fatalf("adapter must be called once on the L2 table block, got %d", intel.calls)
	}
	if intel.reqs[0].Level != plan.L2 {
		t.Errorf("adapter must be asked at L2, got L%d", intel.reqs[0].Level)
	}
	if len(got.Blocks) != 1 {
		t.Fatalf("want 1 table block, got %d", len(got.Blocks))
	}
	blk := got.Blocks[0]
	if blk.Class != plan.ClassTable {
		t.Fatalf("want table class, got %s", blk.Class)
	}
	if blk.Status != plan.StatusRefused {
		t.Fatalf("adapter-declines must yield refused, got %s", blk.Status)
	}
	if blk.Refusal == nil || !blk.Refusal.Spoken {
		t.Errorf("refusal must be Spoken=true, got %+v", blk.Refusal)
	}
	if !honestyRuleSatisfied(blk) {
		t.Errorf("honesty rule violated (need Spoken=true + populated SourceMap): %+v", blk)
	}
}

// TestDegrade_TableNoHeaderL2 — a table with no `|---|` separator row has no
// detected header; the no-adapter L2 degrade must still produce the
// deterministic reading (Status=degraded), with NO "Headers:" clause and a
// "First row:" clause present. Exercises the headerIdx<0 branch of both
// parseTable and deterministicTableGist.
func TestDegrade_TableNoHeaderL2(t *testing.T) {
	t.Parallel()
	lex := compileLexicon()
	rb := rawBlock{text: "| a | b |\n| c | d |", startLine: 1, endLine: 2, hint: hintTable}
	sm := plan.SourceMap{Kind: plan.SourceKindLineRange, StartLine: 1, EndLine: 2}
	out := level(rb, plan.ClassTable, plan.L2, lex)
	if !out.needsIntelligence {
		t.Fatalf("table L2 must request intelligence")
	}
	blk, diag := degrade(rb, plan.ClassTable, plan.L2, out, sm, lex)
	if blk.Status != plan.StatusDegraded {
		t.Errorf("no-adapter table L2 should be degraded, got %s", blk.Status)
	}
	if blk.Level != plan.L2 {
		t.Errorf("table L2 degrade should stay at L2 (not downshift to L1), got L%d", blk.Level)
	}
	if blk.Provenance.VoicedBy != "planner" {
		t.Errorf("table L2 degrade VoicedBy: want planner, got %s", blk.Provenance.VoicedBy)
	}
	if diag == nil || diag.Code != "intelligence_unavailable" || diag.Severity != "info" {
		t.Errorf("expected intelligence_unavailable info diagnostic, got %+v", diag)
	}
	text := blk.Segments[0].Text
	if strings.Contains(text, "Headers:") {
		t.Errorf("no-header table must not emit a Headers clause, got %q", text)
	}
	if !strings.Contains(text, "First row:") {
		t.Errorf("no-header table L2 must emit a First row clause, got %q", text)
	}
}

// TestLevelTable_L2L3_NeedsIntelligence_FactsCarryHeaders — levelTable at
// L2 and L3 must mark needsIntelligence and carry a "headers: ..." fact
// built from the detected header row (issue #47, facts payload shape).
func TestLevelTable_L2L3_NeedsIntelligence_FactsCarryHeaders(t *testing.T) {
	t.Parallel()
	lex := compileLexicon()
	rb := rawBlock{text: "| name | role | team |\n|---|---|---|\n| Alice | lead | core |", hint: hintTable}
	for _, lvl := range []plan.Level{plan.L2, plan.L3} {
		out := level(rb, plan.ClassTable, lvl, lex)
		if !out.needsIntelligence {
			t.Errorf("L%d: table must request intelligence", int(lvl))
		}
		if len(out.segments) != 0 {
			t.Errorf("L%d: needsIntelligence path must leave segments empty, got %+v", int(lvl), out.segments)
		}
		var headersFact string
		for _, f := range out.facts {
			if strings.HasPrefix(f, "headers: ") {
				headersFact = f
			}
		}
		if headersFact != "headers: name, role, team" {
			t.Errorf("L%d: want headers fact 'headers: name, role, team', got %q (all facts: %v)", int(lvl), headersFact, out.facts)
		}
	}

	// No-header table → the explicit "(none)" literal.
	rbNoHdr := rawBlock{text: "| a | b |\n| c | d |", hint: hintTable}
	out := level(rbNoHdr, plan.ClassTable, plan.L2, lex)
	var headersFact string
	for _, f := range out.facts {
		if strings.HasPrefix(f, "headers: ") {
			headersFact = f
		}
	}
	if headersFact != "headers: (none)" {
		t.Errorf("no-header table want 'headers: (none)', got %q", headersFact)
	}
}

// TestDeterministicTableGist_SharesParseWithLevelTable — cheap structural
// proof that levelTable's headers fact and deterministicTableGist's L2
// reading agree on the header text for the same input, i.e. both consume
// the single shared parseTable (the byte-drift mitigation in issue #47).
func TestDeterministicTableGist_SharesParseWithLevelTable(t *testing.T) {
	t.Parallel()
	lex := compileLexicon()
	text := "| name | role | team |\n|---|---|---|\n| Alice | lead | core |\n| Bob | dev | edge |"
	rb := rawBlock{text: text, hint: hintTable}

	out := level(rb, plan.ClassTable, plan.L2, lex)
	var headersFact string
	for _, f := range out.facts {
		if rest, ok := strings.CutPrefix(f, "headers: "); ok {
			headersFact = rest
		}
	}
	if headersFact == "" {
		t.Fatalf("levelTable produced no headers fact: %v", out.facts)
	}
	gist := deterministicTableGist(text, plan.L2)
	if !strings.Contains(gist, "Headers: "+headersFact+".") {
		t.Errorf("gist header text disagrees with the headers fact:\n fact:  %q\n gist:  %q", headersFact, gist)
	}
}
