// Tests for the issue #78 spoken-transcript projection on the MCP speak path.
//
// Coverage:
//   - Golden transcript SPLIT (BLOCKING-2):
//     (a) roster + projection with NO duration (must not depend on a timeline);
//     (b) duration join with a separate timeline fixture, refused block paired
//     with NO timeline row asserted status=refused AND duration_ms=0 together.
//   - transcriptFromResult refusal/degraded/voiced projection (refusal-is-data).
//   - success-path-only attachment (error return carries no transcript).
//   - handler dual-channel substance-equality (ADR #77 D3): decoded Content
//     JSON equal-in-substance to the auto-populated StructuredContent.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/vd09-projects/intelligent-tts-narration-library/pipeline"
	"github.com/vd09-projects/intelligent-tts-narration-library/plan"
)

// loadPlanFixture reads a checked-in plan.json fixture into a NarrationPlan.
func loadPlanFixture(t *testing.T, name string) plan.NarrationPlan {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("read plan fixture %s: %v", name, err)
	}
	var np plan.NarrationPlan
	if err := json.Unmarshal(raw, &np); err != nil {
		t.Fatalf("unmarshal plan fixture %s: %v", name, err)
	}
	return np
}

// loadTimelineFixture reads a checked-in timeline.json fixture.
func loadTimelineFixture(t *testing.T, name string) plan.Timeline {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("read timeline fixture %s: %v", name, err)
	}
	var tl plan.Timeline
	if err := json.Unmarshal(raw, &tl); err != nil {
		t.Fatalf("unmarshal timeline fixture %s: %v", name, err)
	}
	return tl
}

// loadGoldenTranscript reads the checked-in golden transcript JSON.
func loadGoldenTranscript(t *testing.T, name string) []transcriptEntry {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("read golden transcript %s: %v", name, err)
	}
	var want []transcriptEntry
	if err := json.Unmarshal(raw, &want); err != nil {
		t.Fatalf("unmarshal golden transcript %s: %v", name, err)
	}
	return want
}

// summarizeForTest mirrors what pipeline.Narrate does pre-render: builds the
// roster from the plan. We use the public projection (transcriptFromResult)
// against a NarrateResult carrying summaries built the same way the pipeline
// builds them — but since summarizeBlocks is unexported, we reconstruct the
// equivalent roster here from the fixture's blocks using the SAME documented
// rules the pipeline applies (top-level only, single-space non-empty join,
// refusal fields when present). The pipeline-internal test already pins that
// summarizeBlocks itself obeys these rules; here we exercise the cmd-side
// projection end-to-end against a real fixture.
func summarizeForTest(np plan.NarrationPlan) []pipeline.BlockSummary {
	out := make([]pipeline.BlockSummary, 0, len(np.Blocks))
	for _, b := range np.Blocks {
		spoken := ""
		for _, s := range b.Segments {
			if s.Text == "" {
				continue
			}
			if spoken != "" {
				spoken += " "
			}
			spoken += s.Text
		}
		bs := pipeline.BlockSummary{
			ID:         b.ID,
			Order:      b.Order,
			Class:      b.Class,
			Level:      b.Level,
			Status:     b.Status,
			StartLine:  b.SourceMap.StartLine,
			EndLine:    b.SourceMap.EndLine,
			SpokenText: spoken,
		}
		if b.Refusal != nil {
			bs.RefusalReason = b.Refusal.Reason
			bs.RefusalMessage = b.Refusal.Message
		}
		out = append(out, bs)
	}
	return out
}

// TestTranscript_GoldenRosterProjection_NoDuration is split-test (a): roster +
// projection from a fixture plan with one voiced, one degraded, one refused
// block, asserted against a checked-in golden transcript. Crucially this test
// does NOT involve a timeline — every duration_ms is 0 in the golden, guarding
// the v1 vacuous-pass trap (BLOCKING-2).
func TestTranscript_GoldenRosterProjection_NoDuration(t *testing.T) {
	t.Parallel()
	np := loadPlanFixture(t, "transcript_mixed_plan.json")
	result := pipeline.NarrateResult{BlockSummaries: summarizeForTest(np)}

	got := transcriptFromResult(result)
	want := loadGoldenTranscript(t, "transcript_mixed_golden.json")

	if !reflect.DeepEqual(got, want) {
		gotJSON, _ := json.MarshalIndent(got, "", "  ")
		wantJSON, _ := json.MarshalIndent(want, "", "  ")
		t.Fatalf("transcript mismatch.\n--- got ---\n%s\n--- want ---\n%s", gotJSON, wantJSON)
	}

	// Explicit: this projection carried no timeline, so every duration is 0.
	for _, e := range got {
		if e.DurationMs != 0 {
			t.Fatalf("roster-only projection must have duration_ms=0; %s = %d", e.BlockID, e.DurationMs)
		}
	}
	// Fixture must really contain all three statuses (success-metric coverage).
	seen := map[string]bool{}
	for _, e := range got {
		seen[e.Status] = true
	}
	for _, st := range []string{"voiced", "degraded", "refused"} {
		if !seen[st] {
			t.Fatalf("golden fixture must include a %q block; statuses present: %v", st, seen)
		}
	}
}

// TestTranscript_DurationJoin is split-test (b): plan fixture + a SEPARATE
// timeline fixture. After the post-render join, voiced/degraded blocks carry
// duration_ms == EndMs-StartMs; the refused block (paired with NO timeline row)
// is asserted to carry status="refused" AND duration_ms=0 together (SUGGESTION-5).
func TestTranscript_DurationJoin(t *testing.T) {
	t.Parallel()
	np := loadPlanFixture(t, "transcript_mixed_plan.json")
	tl := loadTimelineFixture(t, "transcript_mixed_timeline.json")

	// Build the roster, then run the SAME duration-join the pipeline runs
	// post-render. joinTimelineDurations is unexported in pipeline/, but the
	// public path that feeds transcriptFromResult is BlockSummary.DurationMs;
	// here we replicate the documented join (EndMs-StartMs by BlockID, 0 when
	// no row) to prove the cmd-side projection carries it through. The
	// pipeline-internal test pins joinTimelineDurations itself.
	summaries := summarizeForTest(np)
	durByID := map[string]int{}
	for _, bt := range tl.Blocks {
		durByID[bt.BlockID] = bt.EndMs - bt.StartMs
	}
	for i := range summaries {
		summaries[i].DurationMs = durByID[summaries[i].ID] // 0 when absent
	}

	got := transcriptFromResult(pipeline.NarrateResult{BlockSummaries: summaries})

	byID := map[string]transcriptEntry{}
	for _, e := range got {
		byID[e.BlockID] = e
	}
	if byID["b001"].DurationMs != 1500 {
		t.Errorf("b001 duration_ms: want 1500, got %d", byID["b001"].DurationMs)
	}
	if byID["b002"].DurationMs != 2700 {
		t.Errorf("b002 duration_ms: want 2700, got %d", byID["b002"].DurationMs)
	}
	// SUGGESTION-5: refused block has NO timeline row -> refused AND 0.
	refused := byID["b003"]
	if refused.Status != "refused" || refused.DurationMs != 0 {
		t.Errorf("refused/no-row block: want status=refused AND duration_ms=0; got status=%q duration_ms=%d",
			refused.Status, refused.DurationMs)
	}
}

// TestTranscriptFromResult_RefusalDegradedProjection is the table-driven
// projection test: refused carries reason+message+status; degraded carries
// status with empty refusal; voiced carries empty refusal. No error involved —
// refusal is data.
func TestTranscriptFromResult_RefusalDegradedProjection(t *testing.T) {
	t.Parallel()
	result := pipeline.NarrateResult{
		BlockSummaries: []pipeline.BlockSummary{
			{ID: "b001", Order: 1, Level: plan.L1, Status: plan.StatusVoiced, SpokenText: "Voiced text.", DurationMs: 100},
			{ID: "b002", Order: 2, Level: plan.L2, Status: plan.StatusDegraded, SpokenText: "Read verbatim.", DurationMs: 200},
			{
				ID: "b003", Order: 3, Level: plan.L1, Status: plan.StatusRefused,
				SpokenText: "skipped notice", DurationMs: 0,
				RefusalReason: plan.RefuseNoIntelligence, RefusalMessage: "no adapter wired",
			},
		},
	}

	got := transcriptFromResult(result)
	if len(got) != 3 {
		t.Fatalf("want 3 entries, got %d", len(got))
	}

	voiced := got[0]
	if voiced.Status != "voiced" || voiced.RefusalReason != "" || voiced.RefusalMessage != "" {
		t.Errorf("voiced block must have empty refusal fields, got %+v", voiced)
	}
	degraded := got[1]
	if degraded.Status != "degraded" || degraded.RefusalReason != "" || degraded.RefusalMessage != "" {
		t.Errorf("degraded block must carry status=degraded with empty refusal, got %+v", degraded)
	}
	refused := got[2]
	if refused.Status != "refused" {
		t.Errorf("refused status: want refused, got %q", refused.Status)
	}
	if refused.RefusalReason != string(plan.RefuseNoIntelligence) {
		t.Errorf("refused reason: want %q, got %q", plan.RefuseNoIntelligence, refused.RefusalReason)
	}
	if refused.RefusalMessage != "no adapter wired" {
		t.Errorf("refused message: want %q, got %q", "no adapter wired", refused.RefusalMessage)
	}

	// omitempty proof: voiced/degraded entries serialize WITHOUT refusal keys.
	blob, err := json.Marshal(voiced)
	if err != nil {
		t.Fatalf("marshal voiced entry: %v", err)
	}
	if got := string(blob); strings.Contains(got, "refusal_reason") || strings.Contains(got, "refusal_message") {
		t.Errorf("voiced entry JSON must omit refusal fields, got %s", got)
	}
}

// TestTranscriptFromResult_EmptyRoster returns nil for an empty roster.
func TestTranscriptFromResult_EmptyRoster(t *testing.T) {
	t.Parallel()
	if got := transcriptFromResult(pipeline.NarrateResult{}); got != nil {
		t.Fatalf("empty roster must project to nil, got %+v", got)
	}
}

// TestSpeakHandler_ErrorPath_NoTranscript proves success-path-only attachment
// (SUGGESTION-3): when deps.run returns an error, the handler returns a nil
// result, a zero speakResponse (no transcript), and the error — clean
// error-vs-refusal boundary.
func TestSpeakHandler_ErrorPath_NoTranscript(t *testing.T) {
	t.Parallel()
	boom := errors.New("internal_error: pipeline failure: kokoro down")
	deps := runDeps{
		run: func(_ context.Context, _ speakArgs) (speakResponse, error) {
			return speakResponse{}, boom
		},
	}
	h := speakHandler(deps)
	result, resp, err := h(context.Background(), nil, speakArgs{Source: "doc.md"})
	if !errors.Is(err, boom) {
		t.Fatalf("want errors.Is boom, got %v", err)
	}
	if result != nil {
		t.Errorf("error path must return nil *CallToolResult, got %+v", result)
	}
	if resp.Transcript != nil {
		t.Errorf("error path must attach no transcript, got %+v", resp.Transcript)
	}
}

// TestSpeakHandler_DualChannel is the ADR #77 D3 substance-equality test
// (BLOCKING-1). On success the handler must return a *CallToolResult whose:
//   - typed Out (the speakResponse) drives StructuredContent (we verify the
//     handler returns a non-zero typed resp the SDK would auto-populate from),
//   - Content carries a TextContent whose decoded JSON is substance-equal to
//     that same speakResponse — same transcript through both channels.
func TestSpeakHandler_DualChannel(t *testing.T) {
	t.Parallel()
	want := speakResponse{
		Receipt: speakReceipt{BlocksPlayed: 2, TotalDurationMs: 4200, OutDir: "/tmp/narrate-mcp-x"},
		Transcript: []transcriptEntry{
			{BlockID: "b001", Order: 1, Level: 1, Status: "voiced", SpokenText: "Hello.", DurationMs: 1500},
			{
				BlockID: "b002", Order: 2, Level: 1, Status: "refused", SpokenText: "skipped",
				DurationMs: 0, RefusalReason: "bare_image_no_description", RefusalMessage: "skipped",
			},
		},
	}
	deps := runDeps{
		run: func(_ context.Context, _ speakArgs) (speakResponse, error) {
			return want, nil
		},
	}
	h := speakHandler(deps)
	result, gotResp, err := h(context.Background(), nil, speakArgs{Source: "doc.md"})
	if err != nil {
		t.Fatalf("want nil err, got %v", err)
	}

	// Channel A — the typed Out. The SDK auto-populates StructuredContent from
	// this on the wire; here we assert the handler returns it intact.
	if !reflect.DeepEqual(gotResp, want) {
		t.Fatalf("typed Out (StructuredContent source) mismatch:\n got %+v\nwant %+v", gotResp, want)
	}

	// Channel B — the manual Content. Must be a single TextContent with the
	// serialized speakResponse.
	if result == nil {
		t.Fatal("success path must return a non-nil *CallToolResult (manual Content)")
	}
	if len(result.Content) != 1 {
		t.Fatalf("want exactly one Content entry, got %d", len(result.Content))
	}
	tc, ok := result.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatalf("Content[0] must be *mcp.TextContent, got %T", result.Content[0])
	}

	// Substance-equality: decode the Content JSON and compare to the typed Out.
	var decoded speakResponse
	if err := json.Unmarshal([]byte(tc.Text), &decoded); err != nil {
		t.Fatalf("Content JSON must decode to speakResponse: %v\nraw: %s", err, tc.Text)
	}
	if !reflect.DeepEqual(decoded, want) {
		t.Fatalf("dual-channel divergence: decoded Content != typed Out.\n decoded %+v\n want %+v", decoded, want)
	}

	// And specifically the transcript carried through both channels matches.
	if !reflect.DeepEqual(decoded.Transcript, gotResp.Transcript) {
		t.Fatalf("transcript differs between Content and StructuredContent channels")
	}

	// Sanity: we did NOT set StructuredContent ourselves (SDK does it from Out).
	if result.StructuredContent != nil {
		t.Errorf("handler must leave StructuredContent for the SDK to auto-populate from Out; got %v", result.StructuredContent)
	}
}
