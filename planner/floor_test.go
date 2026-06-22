package planner

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/vd09-projects/intelligent-tts-narration-library/intelligence"
	"github.com/vd09-projects/intelligent-tts-narration-library/plan"
)

// TestApplyCodeFloor_TableDriven — the floor resolution contract (issue
// #73). Asserts both directions (L1 lifts to the floor; an explicit L3
// target survives because the floor is a max, not a set) and that the
// floor NEVER bleeds into a non-code class — every other class passes
// through its resolved target untouched at every document level. A
// zero/invalid floor is a no-op.
func TestApplyCodeFloor_TableDriven(t *testing.T) {
	t.Parallel()

	allClasses := []plan.Class{
		plan.ClassProse, plan.ClassHeading, plan.ClassList,
		plan.ClassTable, plan.ClassConfig, plan.ClassDiagramAsText,
	}

	cases := []struct {
		name   string
		cls    plan.Class
		target plan.Level
		floor  plan.Level
		want   plan.Level
	}{
		// Code, floor = L2 — both directions.
		{name: "code L1 lifts to L2 floor", cls: plan.ClassCode, target: plan.L1, floor: plan.L2, want: plan.L2},
		{name: "code L2 stays L2 at L2 floor", cls: plan.ClassCode, target: plan.L2, floor: plan.L2, want: plan.L2},
		{name: "code L3 survives L2 floor (floor not a set)", cls: plan.ClassCode, target: plan.L3, floor: plan.L2, want: plan.L3},
		// Code, floor = L3 — lift from below, equal stays.
		{name: "code L1 lifts to L3 floor", cls: plan.ClassCode, target: plan.L1, floor: plan.L3, want: plan.L3},
		{name: "code L2 lifts to L3 floor", cls: plan.ClassCode, target: plan.L2, floor: plan.L3, want: plan.L3},
		// Zero / invalid floor is a no-op for code.
		{name: "code zero floor is no-op", cls: plan.ClassCode, target: plan.L1, floor: plan.Level(0), want: plan.L1},
		{name: "code invalid floor is no-op", cls: plan.ClassCode, target: plan.L1, floor: plan.Level(99), want: plan.L1},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := applyCodeFloor(tc.cls, tc.target, tc.floor); got != tc.want {
				t.Errorf("applyCodeFloor(%s, %d, floor=%d) = %d, want %d", tc.cls, tc.target, tc.floor, got, tc.want)
			}
		})
	}

	// No-bleed: a valid floor of L2 (or L3) must leave EVERY non-code class
	// at its resolved target, at every document level. This is the
	// negative case the plan calls out — the floor is code-only.
	t.Run("no bleed into non-code classes", func(t *testing.T) {
		t.Parallel()
		for _, floor := range []plan.Level{plan.L2, plan.L3} {
			for _, target := range []plan.Level{plan.L1, plan.L2, plan.L3} {
				for _, cls := range allClasses {
					if got := applyCodeFloor(cls, target, floor); got != target {
						t.Errorf("floor leaked into %s: applyCodeFloor(%s, %d, floor=%d) = %d, want %d (unchanged)",
							cls, cls, target, floor, got, target)
					}
				}
			}
		}
	})
}

// TestPlan_CodeFloorZeroDrift_NoOp — the counter-metric (S4). A
// CodeMinLevel of zero (the field's zero value, what every pre-#73 caller
// supplies) must produce BYTE-IDENTICAL output to the existing mixed_doc
// golden, which was frozen before the floor existed. This is the
// regression net for non-listen callers: the additive field, left unset,
// changes nothing.
func TestPlan_CodeFloorZeroDrift_NoOp(t *testing.T) {
	seams := deterministicSeams(t)
	data, err := os.ReadFile(filepath.Join("testdata", "mixed_doc.md"))
	if err != nil {
		t.Fatalf("read mixed_doc.md: %v", err)
	}
	doc := stubDoc(string(data), "mixed_doc.md")
	// CodeMinLevel left at its zero value — the floor is off.
	got, err := Plan(context.Background(), doc, Request{Level: plan.L1}, nil, seams...)
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	gotBytes, err := json.MarshalIndent(got, "", "  ")
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	gotBytes = append(gotBytes, '\n')
	wantBytes, err := os.ReadFile(filepath.Join("testdata", "mixed_doc.want.json"))
	if err != nil {
		t.Fatalf("read mixed_doc.want.json: %v", err)
	}
	if string(gotBytes) != string(wantBytes) {
		t.Errorf("zero CodeMinLevel drifted from frozen golden — the additive field must be a no-op when unset:\n--- want ---\n%s\n--- got ---\n%s",
			summarizeForDiff(string(wantBytes)), summarizeForDiff(string(gotBytes)))
	}
}

// TestPlan_CodeFloorLiftsCodeNotProse — end-to-end through Plan: a mixed
// prose+code document at document-Level L1 with CodeMinLevel=L2 must voice
// the code block at L2 while the prose block stays at L1. This proves the
// floor is wired into structuralPass and is code-scoped, not document-wide.
func TestPlan_CodeFloorLiftsCodeNotProse(t *testing.T) {
	t.Parallel()
	seams := deterministicSeams(t)
	// Short prose (degraded-verbatim path at L1, no adapter) + a small Go
	// code block. With CodeMinLevel=L2 the code lifts to L2; the prose,
	// being a non-code class, stays at the requested L1.
	src := "A short paragraph.\n\n```go\nfunc factorial(n int) int {\n\treturn n\n}\n```\n"
	got, err := Plan(context.Background(), stubDoc(src, "mixed.md"),
		Request{Level: plan.L1, CodeMinLevel: plan.L2}, nil, seams...)
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	if len(got.Blocks) != 2 {
		t.Fatalf("want 2 blocks (prose + code), got %d: %+v", len(got.Blocks), got.Blocks)
	}

	var prose, code *plan.Block
	for i := range got.Blocks {
		switch got.Blocks[i].Class {
		case plan.ClassProse:
			prose = &got.Blocks[i]
		case plan.ClassCode:
			code = &got.Blocks[i]
		}
	}
	if prose == nil || code == nil {
		t.Fatalf("expected one prose and one code block, got %+v", got.Blocks)
	}
	if prose.Level != plan.L1 {
		t.Errorf("prose must stay at requested L1 (no floor bleed), got %d", prose.Level)
	}
	if code.Level != plan.L2 {
		t.Errorf("code must lift to the L2 floor, got %d", code.Level)
	}
}

// TestPlan_CodeFloorL3RequestSurvives — an explicit document-Level L3
// request with CodeMinLevel=L2 must keep code at L3: the floor is a max,
// never a clobbering set.
func TestPlan_CodeFloorL3RequestSurvives(t *testing.T) {
	t.Parallel()
	seams := deterministicSeams(t)
	src := "```go\nfunc X() {}\n```\n"
	got, err := Plan(context.Background(), stubDoc(src, "code.md"),
		Request{Level: plan.L3, CodeMinLevel: plan.L2}, nil, seams...)
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	if len(got.Blocks) != 1 {
		t.Fatalf("want 1 code block, got %d", len(got.Blocks))
	}
	if got.Blocks[0].Level != plan.L3 {
		t.Errorf("explicit L3 request must survive the L2 floor, got %d", got.Blocks[0].Level)
	}
}

// TestPlan_CodeFloorLiftedNoAdapter_RoutesToDegradeCodeL2 — review
// suggestion 3: a code block lifted L1->L2 by the floor, with NO adapter
// wired, must land on the degradeCodeL2 path (the count+declarations gist
// at L2, Status=degraded) — NOT degradeStructuredDownshift (which would
// collapse it to the bare L1 line). Pinned by both the L2 level and the
// info-severity intelligence_unavailable diagnostic carrying the L2 gist
// wording.
func TestPlan_CodeFloorLiftedNoAdapter_RoutesToDegradeCodeL2(t *testing.T) {
	t.Parallel()
	seams := deterministicSeams(t)
	src := "```go\nfunc factorial(n int) int {\n\treturn n\n}\n```\n"
	got, err := Plan(context.Background(), stubDoc(src, "code.md"),
		Request{Level: plan.L1, CodeMinLevel: plan.L2}, nil, seams...)
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	if len(got.Blocks) != 1 {
		t.Fatalf("want 1 code block, got %d", len(got.Blocks))
	}
	blk := got.Blocks[0]
	if blk.Class != plan.ClassCode {
		t.Fatalf("want code class, got %s", blk.Class)
	}
	if blk.Level != plan.L2 {
		t.Errorf("lifted code with no adapter must stay at L2 (degradeCodeL2), got L%d", blk.Level)
	}
	if blk.Status != plan.StatusDegraded {
		t.Errorf("lifted code with no adapter must be degraded, got %s", blk.Status)
	}
	if blk.Provenance.VoicedBy != "planner" {
		t.Errorf("VoicedBy: want planner, got %s", blk.Provenance.VoicedBy)
	}
	// The L2 degrade diagnostic (info) — distinguishes degradeCodeL2 from
	// degradeStructuredDownshift, whose message says "downshifted to L1".
	var diag *plan.Diagnostic
	for i := range got.Diagnostics {
		if got.Diagnostics[i].Code == "intelligence_unavailable" {
			diag = &got.Diagnostics[i]
			break
		}
	}
	if diag == nil {
		t.Fatalf("expected an intelligence_unavailable diagnostic, got %+v", got.Diagnostics)
	}
	if diag.Severity != plan.SeverityInfo {
		t.Errorf("diagnostic severity: want info, got %q", diag.Severity)
	}
	if diag.Message != "code block read at deterministic L2 (count + declarations) without an intelligence adapter" {
		t.Errorf("diagnostic must be the degradeCodeL2 (not L1-downshift) message, got %q", diag.Message)
	}
}

// TestPlan_CodeFloorLifted_AdapterRefuses_Refused — review suggestion 1:
// a code block lifted to the L2 floor, with an adapter PRESENT that
// DECLINES (Refused), must exercise the real callIntelligence ->
// intelRefusalBlock path (RefuseTooLarge), landing Status=refused with a
// populated SourceMap + spoken notice. This is the distinct adapter-refuses
// fixture, kept separate from the no-adapter degraded case above so the two
// honesty outcomes are never conflated. It drives the live adapter-declines
// path (callIntelligence res.Refused), not buildRefusal directly.
func TestPlan_CodeFloorLifted_AdapterRefuses_Refused(t *testing.T) {
	t.Parallel()
	seams := deterministicSeams(t)
	intel := &scriptedIntel{replies: []intelligence.IntelligenceResult{
		{Refused: true, RefusalNote: "code block too dense to summarize at one sentence", Model: "scripted@test"},
	}}
	// Document Level L1 + CodeMinLevel L2 → the floor lifts the code block
	// to L2, which (with an adapter wired) dispatches to the adapter; the
	// adapter declines, so callIntelligence converts it to a refusal.
	src := "```go\nfunc factorial(n int) int {\n\treturn n\n}\n```\n"
	got, err := Plan(context.Background(), stubDoc(src, "code.md"),
		Request{Level: plan.L1, CodeMinLevel: plan.L2}, intel, seams...)
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	if intel.calls != 1 {
		t.Fatalf("adapter must be called once on the lifted L2 code block, got %d", intel.calls)
	}
	if intel.reqs[0].Level != plan.L2 {
		t.Errorf("adapter must be asked at the L2 floor, got L%d", intel.reqs[0].Level)
	}
	if len(got.Blocks) != 1 {
		t.Fatalf("want 1 code block, got %d", len(got.Blocks))
	}
	blk := got.Blocks[0]
	if blk.Status != plan.StatusRefused {
		t.Fatalf("adapter-declines must yield refused, got %s", blk.Status)
	}
	if blk.Refusal == nil || blk.Refusal.Reason != plan.RefuseTooLarge {
		t.Errorf("want RefuseTooLarge refusal, got %+v", blk.Refusal)
	}
	if !honestyRuleSatisfied(blk) {
		t.Errorf("honesty rule violated (need Spoken=true + populated SourceMap): %+v", blk)
	}
}
