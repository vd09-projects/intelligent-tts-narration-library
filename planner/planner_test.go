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

// deterministicSeams — return the per-call clock + PlanID seam options
// for a test. No global swap, no deferred restore: each test passes these
// opts to its own Plan() call, so the seam values are owned by that call
// and never observed by a sibling. This is what makes the ~8 t.Parallel()
// planner tests race-free.
func deterministicSeams(t *testing.T) []VoiceOption {
	t.Helper()
	return []VoiceOption{
		withClock(func() time.Time { return time.Date(2026, 6, 18, 12, 0, 0, 0, time.UTC) }),
		withPlanID(func() string { return "01TESTPLANID0000000000000Z" }),
	}
}

func TestPlan_EmptyDocument(t *testing.T) {
	t.Parallel()
	seams := deterministicSeams(t)
	got, err := Plan(context.Background(), stubDoc("", "empty.md"), Request{Level: plan.L1}, nil, seams...)
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
	seams := deterministicSeams(t)
	src := "# Hello\n\nA short paragraph here.\n"
	got, err := Plan(context.Background(), stubDoc(src, "doc.md"), Request{Level: plan.L1}, nil, seams...)
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
	seams := deterministicSeams(t)
	src := "Intro.\n\n![chart](bench.png)\n"
	got, err := Plan(context.Background(), stubDoc(src, "doc.md"), Request{Level: plan.L1}, nil, seams...)
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
	seams := deterministicSeams(t)
	intel := &scriptedIntel{replies: []intelligence.IntelligenceResult{
		{Text: "L1 gist of the prose.", Model: "scripted@test"},
	}}
	src := "This is a paragraph.\n"
	got, err := Plan(context.Background(), stubDoc(src, "doc.md"), Request{Level: plan.L1}, intel, seams...)
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
	seams := deterministicSeams(t)
	intel := &scriptedIntel{replies: []intelligence.IntelligenceResult{
		{Refused: true, RefusalNote: "block too dense to faithfully summarize"},
	}}
	src := "Some prose.\n"
	got, _ := Plan(context.Background(), stubDoc(src, "doc.md"), Request{Level: plan.L2}, intel, seams...)
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
	seams := deterministicSeams(t)
	// Mix every refusal-producing case in one document.
	src := "# Intro\n\n![chart](bench.png)\n\n" + strings.Repeat("word ", 200) + "\n"
	got, err := Plan(context.Background(), stubDoc(src, "doc.md"), Request{Level: plan.L1}, nil, seams...)
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	for _, blk := range got.Blocks {
		if !honestyRuleSatisfied(blk) {
			t.Errorf("block %s violated honesty rule: %+v", blk.ID, blk)
		}
	}
}

// TestPlan_SeamsAreCallScoped — direct proof of per-call seam isolation.
// Two concurrent Plan() calls install DISTINCT clock + plan-id seams and
// each must observe its own values, never the sibling's. With the old
// package globals this is exactly the cross-talk the race detector
// flagged; with per-call threading it is correct by construction.
func TestPlan_SeamsAreCallScoped(t *testing.T) {
	t.Parallel()

	type want struct {
		planID  string
		created string
	}
	caseFor := func(id string, ts time.Time) (want, []VoiceOption) {
		return want{planID: id, created: ts.Format(time.RFC3339)},
			[]VoiceOption{
				withPlanID(func() string { return id }),
				withClock(func() time.Time { return ts }),
			}
	}

	wantA, optsA := caseFor("01AAAAAAAAAAAAAAAAAAAAAAAA", time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	wantB, optsB := caseFor("01BBBBBBBBBBBBBBBBBBBBBBBB", time.Date(2030, 12, 31, 23, 59, 59, 0, time.UTC))

	src := "# Heading\n"
	run := func(opts []VoiceOption) (string, string) {
		got, err := Plan(context.Background(), stubDoc(src, "doc.md"), Request{Level: plan.L1}, nil, opts...)
		if err != nil {
			t.Errorf("Plan: %v", err)
			return "", ""
		}
		return got.PlanID, got.CreatedAt
	}

	const iters = 200
	done := make(chan struct{}, 2)
	go func() {
		for i := 0; i < iters; i++ {
			id, created := run(optsA)
			if id != wantA.planID || created != wantA.created {
				t.Errorf("call A observed sibling seam: id=%q created=%q", id, created)
			}
		}
		done <- struct{}{}
	}()
	go func() {
		for i := 0; i < iters; i++ {
			id, created := run(optsB)
			if id != wantB.planID || created != wantB.created {
				t.Errorf("call B observed sibling seam: id=%q created=%q", id, created)
			}
		}
		done <- struct{}{}
	}()
	<-done
	<-done
}

// TestPlan_NoSeamUsesWallClockDefault — regression guard for the
// no-opt path. With the globals deleted, the wall-clock + real-ULID
// defaults must be resolved inside Plan(); a nil seam func must never
// reach the read path. Asserts no panic and that real (non-test)
// values are produced.
func TestPlan_NoSeamUsesWallClockDefault(t *testing.T) {
	t.Parallel()
	before := time.Now().UTC().Add(-time.Second)
	got, err := Plan(context.Background(), stubDoc("# Heading\n", "doc.md"), Request{Level: plan.L1}, nil)
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	if got.PlanID == "" {
		t.Errorf("default plan id should be non-empty")
	}
	if got.PlanID == "01TESTPLANID0000000000000Z" {
		t.Errorf("default path leaked the test seam plan id")
	}
	ts, perr := time.Parse(time.RFC3339, got.CreatedAt)
	if perr != nil {
		t.Fatalf("default CreatedAt not RFC3339: %q (%v)", got.CreatedAt, perr)
	}
	after := time.Now().UTC().Add(time.Second)
	if ts.Before(before) || ts.After(after) {
		t.Errorf("default CreatedAt %v not within wall-clock window [%v, %v]", ts, before, after)
	}
}
