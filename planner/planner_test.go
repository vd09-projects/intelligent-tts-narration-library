package planner

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/vd09-projects/intelligent-tts-narration-library/intelligence"
	"github.com/vd09-projects/intelligent-tts-narration-library/plan"
)

// scriptedIntel — deterministic mock IntelligenceAdapter for tests and
// golden fixtures. Returns scripted strings indexed by call order; a
// configurable "refuse" mode forces a Refused result instead. Lives in
// _test.go so it never bleeds into the planner's production import
// graph (kept honest by planner/deps_test.go).
type scriptedIntel struct {
	replies []intelligence.IntelligenceResult
	calls   int
}

func (s *scriptedIntel) Voice(_ context.Context, _ intelligence.IntelligenceRequest) (intelligence.IntelligenceResult, error) {
	idx := s.calls
	s.calls++
	if idx >= len(s.replies) {
		return intelligence.IntelligenceResult{
			Text:  fmt.Sprintf("scripted reply %d (unscripted overflow)", idx+1),
			Model: "scripted@test",
		}, nil
	}
	return s.replies[idx], nil
}

// installDeterministicSeams — replace clock + PlanID with deterministic
// values for the duration of one test. Returns a restorer.
func installDeterministicSeams(t *testing.T) func() {
	t.Helper()
	origNow := nowFunc
	origID := newPlanIDFunc
	nowFunc = func() time.Time { return time.Date(2026, 6, 18, 12, 0, 0, 0, time.UTC) }
	newPlanIDFunc = func() string { return "01TESTPLANID0000000000000Z" }
	return func() {
		nowFunc = origNow
		newPlanIDFunc = origID
	}
}

func TestPlan_EmptyDocument(t *testing.T) {
	t.Parallel()
	defer installDeterministicSeams(t)()
	got, err := Plan(context.Background(), stubDoc("", "empty.md"), Request{Level: plan.L1}, nil)
	if err != nil {
		t.Fatalf("Plan returned error: %v", err)
	}
	if len(got.Blocks) != 0 {
		t.Errorf("empty doc should produce zero blocks, got %d", len(got.Blocks))
	}
	if len(got.Diagnostics) == 0 || got.Diagnostics[0].Code != "empty_document" {
		t.Errorf("empty doc should produce empty_document diagnostic")
	}
}

func TestPlan_HeadingPlusProseNoIntel(t *testing.T) {
	t.Parallel()
	defer installDeterministicSeams(t)()
	src := "# Hello\n\nA short paragraph here.\n"
	got, err := Plan(context.Background(), stubDoc(src, "doc.md"), Request{Level: plan.L1}, nil)
	if err != nil {
		t.Fatalf("Plan returned error: %v", err)
	}
	if len(got.Blocks) != 2 {
		t.Fatalf("want 2 blocks, got %d", len(got.Blocks))
	}
	if got.Blocks[0].Status != plan.StatusVoiced {
		t.Errorf("heading should be voiced, got %s", got.Blocks[0].Status)
	}
	if got.Blocks[1].Status != plan.StatusDegraded {
		t.Errorf("short prose without intel should be degraded, got %s", got.Blocks[1].Status)
	}
	if got.Blocks[0].ID != "b001" || got.Blocks[1].ID != "b002" {
		t.Errorf("block IDs: %q %q", got.Blocks[0].ID, got.Blocks[1].ID)
	}
}

func TestPlan_RefusedImageHonestyRule(t *testing.T) {
	t.Parallel()
	defer installDeterministicSeams(t)()
	src := "Intro.\n\n![chart](bench.png)\n"
	got, err := Plan(context.Background(), stubDoc(src, "doc.md"), Request{Level: plan.L1}, nil)
	if err != nil {
		t.Fatalf("Plan returned error: %v", err)
	}
	var imageBlock *plan.Block
	for i := range got.Blocks {
		if got.Blocks[i].Status == plan.StatusRefused {
			imageBlock = &got.Blocks[i]
			break
		}
	}
	if imageBlock == nil {
		t.Fatalf("expected at least one refused block, got %+v", got.Blocks)
	}
	if !honestyRuleSatisfied(*imageBlock) {
		t.Errorf("honesty rule violated: %+v", imageBlock)
	}
	if imageBlock.Refusal.Reason != plan.RefuseBareImage {
		t.Errorf("want RefuseBareImage, got %s", imageBlock.Refusal.Reason)
	}
}

func TestPlan_IntelligenceVoicesProse(t *testing.T) {
	t.Parallel()
	defer installDeterministicSeams(t)()
	intel := &scriptedIntel{replies: []intelligence.IntelligenceResult{
		{Text: "L1 gist of the prose.", Model: "scripted@test"},
	}}
	src := "This is a paragraph.\n"
	got, err := Plan(context.Background(), stubDoc(src, "doc.md"), Request{Level: plan.L1}, intel)
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	if len(got.Blocks) != 1 || got.Blocks[0].Status != plan.StatusVoiced {
		t.Errorf("prose with intel should be voiced, got %+v", got.Blocks)
	}
	if !strings.Contains(got.Blocks[0].Segments[0].Text, "L1 gist") {
		t.Errorf("scripted text not picked up: %q", got.Blocks[0].Segments[0].Text)
	}
	if got.Blocks[0].Provenance.VoicedBy != "intelligence" {
		t.Errorf("VoicedBy: want intelligence, got %s", got.Blocks[0].Provenance.VoicedBy)
	}
}

func TestPlan_IntelligenceRefusedConvertsToRefusal(t *testing.T) {
	t.Parallel()
	defer installDeterministicSeams(t)()
	intel := &scriptedIntel{replies: []intelligence.IntelligenceResult{
		{Refused: true, RefusalNote: "block too dense to faithfully summarize"},
	}}
	src := "Some prose.\n"
	got, _ := Plan(context.Background(), stubDoc(src, "doc.md"), Request{Level: plan.L2}, intel)
	if len(got.Blocks) != 1 || got.Blocks[0].Status != plan.StatusRefused {
		t.Fatalf("intel.Refused should produce refused block, got %+v", got.Blocks)
	}
	if got.Blocks[0].Refusal.Reason != plan.RefuseTooLarge {
		t.Errorf("want RefuseTooLarge, got %s", got.Blocks[0].Refusal.Reason)
	}
	if !honestyRuleSatisfied(got.Blocks[0]) {
		t.Errorf("honesty rule violated: %+v", got.Blocks[0])
	}
}

func TestPlan_ContextCancellation(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := Plan(ctx, stubDoc("hi.\n", "doc.md"), Request{Level: plan.L1}, nil)
	if err == nil {
		t.Errorf("cancelled ctx should produce error")
	}
}

func TestPlan_HonestyRuleAcrossAllBlocks(t *testing.T) {
	t.Parallel()
	defer installDeterministicSeams(t)()
	// Mix every refusal-producing case in one document.
	src := "# Intro\n\n![chart](bench.png)\n\n" + strings.Repeat("word ", 200) + "\n"
	got, err := Plan(context.Background(), stubDoc(src, "doc.md"), Request{Level: plan.L1}, nil)
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	for _, blk := range got.Blocks {
		if !honestyRuleSatisfied(blk) {
			t.Errorf("block %s violated honesty rule: %+v", blk.ID, blk)
		}
	}
}
