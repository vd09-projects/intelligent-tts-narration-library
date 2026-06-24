package pipeline

import (
	"testing"

	"github.com/vd09-projects/intelligent-tts-narration-library/plan"
)

// TestSummarizeBlocks_SpokenTextJoin asserts the SpokenText join rule:
// single-space join of every non-empty Segment.Text in segment order; pause
// and empty-text segments contribute nothing.
func TestSummarizeBlocks_SpokenTextJoin(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name     string
		segments []plan.Segment
		want     string
	}{
		{
			name:     "single speech segment",
			segments: []plan.Segment{{ID: "s1", Kind: plan.SegmentKindSpeech, Text: "Hello world."}},
			want:     "Hello world.",
		},
		{
			name: "multiple speech segments joined single-space",
			segments: []plan.Segment{
				{ID: "s1", Kind: plan.SegmentKindSpeech, Text: "First."},
				{ID: "s2", Kind: plan.SegmentKindSpeech, Text: "Second."},
			},
			want: "First. Second.",
		},
		{
			name: "pause segment between speech contributes nothing",
			segments: []plan.Segment{
				{ID: "s1", Kind: plan.SegmentKindSpeech, Text: "Before."},
				{ID: "s2", Kind: plan.SegmentKindPause, PauseMs: 300},
				{ID: "s3", Kind: plan.SegmentKindSpeech, Text: "After."},
			},
			want: "Before. After.",
		},
		{
			name: "empty-text speech segment skipped (no leading/trailing space)",
			segments: []plan.Segment{
				{ID: "s1", Kind: plan.SegmentKindSpeech, Text: ""},
				{ID: "s2", Kind: plan.SegmentKindSpeech, Text: "Only this."},
				{ID: "s3", Kind: plan.SegmentKindSpeech, Text: ""},
			},
			want: "Only this.",
		},
		{
			name:     "no segments yields empty string",
			segments: nil,
			want:     "",
		},
		{
			name: "all pause/empty yields empty string",
			segments: []plan.Segment{
				{ID: "s1", Kind: plan.SegmentKindPause, PauseMs: 100},
				{ID: "s2", Kind: plan.SegmentKindSpeech, Text: ""},
			},
			want: "",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			b := plan.Block{ID: "b001", Segments: tc.segments}
			if got := spokenTextOf(b); got != tc.want {
				t.Fatalf("spokenTextOf: want %q, got %q", tc.want, got)
			}
		})
	}
}

// TestSummarizeBlocks_DoesNotRecurseIntoSubBlocks pins the SubBlocks traversal
// rule (SUGGESTION-1): summarizeBlocks flattens TOP-LEVEL blocks only and does
// NOT descend into Block.SubBlocks. The transcript projection mirrors this
// exactly, so this rule is load-bearing and asserted, not merely exercised.
func TestSummarizeBlocks_DoesNotRecurseIntoSubBlocks(t *testing.T) {
	t.Parallel()
	blocks := []plan.Block{
		{
			ID:       "b001",
			Order:    1,
			Class:    plan.ClassCode,
			Level:    plan.L2,
			Status:   plan.StatusVoiced,
			Segments: []plan.Segment{{ID: "s1", Kind: plan.SegmentKindSpeech, Text: "Parent block."}},
			SubBlocks: []plan.Block{
				{
					ID:       "b001-sub1",
					Order:    1,
					Class:    plan.ClassCode,
					Level:    plan.L2,
					Status:   plan.StatusVoiced,
					Segments: []plan.Segment{{ID: "s1", Kind: plan.SegmentKindSpeech, Text: "Child block."}},
				},
			},
		},
		{
			ID:       "b002",
			Order:    2,
			Class:    plan.ClassProse,
			Level:    plan.L1,
			Status:   plan.StatusVoiced,
			Segments: []plan.Segment{{ID: "s1", Kind: plan.SegmentKindSpeech, Text: "Second block."}},
		},
	}

	got := summarizeBlocks(blocks)

	if len(got) != 2 {
		t.Fatalf("summarizeBlocks must enumerate top-level blocks only (2), got %d: %+v", len(got), got)
	}
	for _, s := range got {
		if s.ID == "b001-sub1" {
			t.Fatalf("summarizeBlocks must NOT emit sub-block entries; found %q", s.ID)
		}
	}
	if got[0].ID != "b001" || got[1].ID != "b002" {
		t.Fatalf("roster order/ids wrong: got %q, %q", got[0].ID, got[1].ID)
	}
	// Parent's SpokenText is its own segments only — never includes child text.
	if got[0].SpokenText != "Parent block." {
		t.Fatalf("parent SpokenText must exclude sub-block text; got %q", got[0].SpokenText)
	}
}

// TestSummarizeBlocks_RefusalFields asserts refusal reason/message are carried
// only when the block has a Refusal, and empty otherwise.
func TestSummarizeBlocks_RefusalFields(t *testing.T) {
	t.Parallel()
	blocks := []plan.Block{
		{
			ID:       "b001",
			Order:    1,
			Status:   plan.StatusVoiced,
			Segments: []plan.Segment{{Kind: plan.SegmentKindSpeech, Text: "Voiced."}},
		},
		{
			ID:     "b002",
			Order:  2,
			Status: plan.StatusRefused,
			Refusal: &plan.Refusal{
				Reason:  plan.RefuseBareImage,
				Message: "skipped a chart",
				Spoken:  true,
			},
			Segments: []plan.Segment{{Kind: plan.SegmentKindSpeech, Text: "skipped a chart"}},
		},
	}
	got := summarizeBlocks(blocks)
	if got[0].RefusalReason != "" || got[0].RefusalMessage != "" {
		t.Fatalf("voiced block must have empty refusal fields, got %+v", got[0])
	}
	if got[1].RefusalReason != plan.RefuseBareImage {
		t.Fatalf("refused block RefusalReason: want %q, got %q", plan.RefuseBareImage, got[1].RefusalReason)
	}
	if got[1].RefusalMessage != "skipped a chart" {
		t.Fatalf("refused block RefusalMessage: want %q, got %q", "skipped a chart", got[1].RefusalMessage)
	}
	// DurationMs is NOT set by summarizeBlocks (pre-render) — must be 0.
	for i, s := range got {
		if s.DurationMs != 0 {
			t.Fatalf("summarizeBlocks must leave DurationMs zero (pre-render); entry %d = %d", i, s.DurationMs)
		}
	}
}

// TestJoinTimelineDurations covers the SEPARATE post-render pass: each block's
// DurationMs is set to EndMs-StartMs from the matching BlockTiming; a block
// with no timing row stays 0; refused block (no row) is locked at 0.
func TestJoinTimelineDurations(t *testing.T) {
	t.Parallel()
	summaries := []BlockSummary{
		{ID: "b001", Status: plan.StatusVoiced},
		{ID: "b002", Status: plan.StatusDegraded},
		{ID: "b003", Status: plan.StatusRefused}, // intentionally no timeline row
	}
	tl := plan.Timeline{
		Blocks: []plan.BlockTiming{
			{BlockID: "b001", StartMs: 0, EndMs: 1500},
			{BlockID: "b002", StartMs: 1500, EndMs: 4200},
		},
	}

	joinTimelineDurations(summaries, tl)

	if summaries[0].DurationMs != 1500 {
		t.Errorf("b001 DurationMs: want 1500, got %d", summaries[0].DurationMs)
	}
	if summaries[1].DurationMs != 2700 {
		t.Errorf("b002 DurationMs: want 2700 (4200-1500), got %d", summaries[1].DurationMs)
	}
	// SUGGESTION-5: refused block, no timeline row -> status refused AND
	// duration 0, locked together as one fact.
	if summaries[2].Status != plan.StatusRefused || summaries[2].DurationMs != 0 {
		t.Errorf("refused/no-row block must be status=refused AND duration=0; got status=%q duration=%d",
			summaries[2].Status, summaries[2].DurationMs)
	}
}

// TestJoinTimelineDurations_EmptyInputs is a guard: nil/empty summaries or an
// empty timeline must not panic and must leave durations at zero.
func TestJoinTimelineDurations_EmptyInputs(t *testing.T) {
	t.Parallel()
	// Empty timeline: durations stay 0.
	summaries := []BlockSummary{{ID: "b001"}}
	joinTimelineDurations(summaries, plan.Timeline{})
	if summaries[0].DurationMs != 0 {
		t.Fatalf("empty timeline must leave DurationMs 0, got %d", summaries[0].DurationMs)
	}
	// Nil summaries: no panic.
	joinTimelineDurations(nil, plan.Timeline{Blocks: []plan.BlockTiming{{BlockID: "x", EndMs: 10}}})
}
