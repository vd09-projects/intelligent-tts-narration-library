// Tests for the issue #86 transcript cap on the MCP speak response.
//
// Coverage:
//   - capTranscript boundary behavior (table-driven): empty, one, under, exactly
//     at, just over, far over the limit, plus the limit<=0 defensive contract.
//     Asserts kept length, truncated flag, omitted count, head-keep plan order,
//     and — for the under/at-limit rows — slice-header identity (the no-copy /
//     no-alloc property that makes the byte-identical small case true).
//   - Oversized-roster handler seam (integration): drive real runSpeak (cap
//     applied) through the dual-channel handler; both channels carry the capped
//     length and the truncation signal fields agree.
//   - Byte-identical small case: under-cap roster marshals with NEITHER signal
//     key present (omitempty absence).
package main

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/vd09-projects/intelligent-tts-narration-library/pipeline"
	"github.com/vd09-projects/intelligent-tts-narration-library/sink"
)

// makeEntries builds n transcript entries with 1-based Order and b%03d ids so
// head-keep order is checkable by inspecting the first/last kept entry.
func makeEntries(n int) []transcriptEntry {
	if n == 0 {
		return nil
	}
	out := make([]transcriptEntry, n)
	for i := range out {
		out[i] = transcriptEntry{
			BlockID:    blockID(i + 1),
			Order:      i + 1,
			Level:      1,
			Status:     "voiced",
			SpokenText: "x",
		}
	}
	return out
}

func blockID(n int) string {
	// b001, b042, ... — zero-padded enough for the test sizes.
	digits := []byte{'0', '0', '0'}
	for i := len(digits) - 1; i >= 0 && n > 0; i-- {
		digits[i] = byte('0' + n%10)
		n /= 10
	}
	return "b" + string(digits)
}

func TestCapTranscript(t *testing.T) {
	t.Parallel()
	const limit = 5
	tests := []struct {
		name      string
		in        int // roster size
		limit     int
		wantKept  int
		wantTrunc bool
		wantOmit  int
		// wantIdentity: the returned slice must be the input unchanged (no copy).
		wantIdentity bool
	}{
		{name: "empty", in: 0, limit: limit, wantKept: 0, wantTrunc: false, wantOmit: 0, wantIdentity: true},
		{name: "one", in: 1, limit: limit, wantKept: 1, wantTrunc: false, wantOmit: 0, wantIdentity: true},
		{name: "under", in: limit - 1, limit: limit, wantKept: limit - 1, wantTrunc: false, wantOmit: 0, wantIdentity: true},
		{name: "exactly-at", in: limit, limit: limit, wantKept: limit, wantTrunc: false, wantOmit: 0, wantIdentity: true},
		{name: "just-over", in: limit + 1, limit: limit, wantKept: limit, wantTrunc: true, wantOmit: 1},
		{name: "far-over", in: limit * 3, limit: limit, wantKept: limit, wantTrunc: true, wantOmit: limit * 2},
		// limit<=0 is the defensive no-cap contract: input returned unchanged.
		{name: "limit-zero-no-cap", in: limit + 2, limit: 0, wantKept: limit + 2, wantTrunc: false, wantOmit: 0, wantIdentity: true},
		{name: "limit-negative-no-cap", in: limit + 2, limit: -1, wantKept: limit + 2, wantTrunc: false, wantOmit: 0, wantIdentity: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			entries := makeEntries(tc.in)
			kept, truncated, omitted := capTranscript(entries, tc.limit)

			if len(kept) != tc.wantKept {
				t.Errorf("kept len: want %d, got %d", tc.wantKept, len(kept))
			}
			if truncated != tc.wantTrunc {
				t.Errorf("truncated: want %v, got %v", tc.wantTrunc, truncated)
			}
			if omitted != tc.wantOmit {
				t.Errorf("omitted: want %d, got %d", tc.wantOmit, omitted)
			}

			// Head-keep plan order: first kept is Order 1, last kept is wantKept.
			if len(kept) > 0 {
				if kept[0].Order != 1 {
					t.Errorf("head order: first kept Order want 1, got %d", kept[0].Order)
				}
				if last := kept[len(kept)-1].Order; last != tc.wantKept {
					t.Errorf("head order: last kept Order want %d, got %d", tc.wantKept, last)
				}
			}

			// Identity (no-copy) for the under/at-limit and no-cap rows: the
			// returned slice must share the input's backing array. This is the
			// property that makes the small-case output byte-identical; assert it
			// as slice-header identity, not DeepEqual.
			if tc.wantIdentity && len(entries) > 0 {
				if &kept[0] != &entries[0] {
					t.Errorf("under/at-limit must return input unchanged (same backing array, no copy)")
				}
			}
		})
	}
}

// TestCapTranscript_RefusedHeadRetainsRefusalFields asserts that a refused entry
// surviving in the kept head carries its RefusalReason / RefusalMessage through
// the cap unchanged — refusals are the highest-signal rows a consumer scans for,
// and head-keep must not strip their refusal data. (capTranscript reslices the
// head; it never rewrites entries, so this is a guard against future regressions.)
func TestCapTranscript_RefusedHeadRetainsRefusalFields(t *testing.T) {
	t.Parallel()
	const limit = 5
	entries := makeEntries(limit * 2) // oversized → truncation fires
	// Make the head entry refused with both refusal fields populated.
	entries[0].Status = "refused"
	entries[0].SpokenText = ""
	entries[0].RefusalReason = "refuse_no_intelligence"
	entries[0].RefusalMessage = "skipped: prose too large to voice without intelligence"

	kept, truncated, omitted := capTranscript(entries, limit)
	if !truncated || omitted != limit {
		t.Fatalf("setup expects truncation: got truncated=%v omitted=%d", truncated, omitted)
	}
	head := kept[0]
	if head.Status != "refused" {
		t.Errorf("kept head status: want refused, got %q", head.Status)
	}
	if head.RefusalReason != "refuse_no_intelligence" {
		t.Errorf("kept head RefusalReason lost through cap: got %q", head.RefusalReason)
	}
	if head.RefusalMessage != "skipped: prose too large to voice without intelligence" {
		t.Errorf("kept head RefusalMessage lost through cap: got %q", head.RefusalMessage)
	}
}

// TestSpeakHandler_TranscriptCapped_DualChannel drives the REAL runSpeak (so the
// cap actually runs) with an oversized roster, through the dual-channel handler,
// and asserts both channels carry the capped length and agreeing signal fields.
func TestSpeakHandler_TranscriptCapped_DualChannel(t *testing.T) {
	// Cannot t.Parallel() — mutates package-level newPipeline var.
	const preCap = transcriptMaxEntries + 37
	stub := &stubNarrator{
		receipt:        sink.SinkReceipt{BlocksPlayed: preCap, TotalDurationMs: 1000},
		blockSummaries: makeBlockSummaries(preCap),
	}
	restore := withStubPipeline(t, stub)
	defer restore()

	tmpFile, err := os.CreateTemp(t.TempDir(), "sample-*.md")
	if err != nil {
		t.Fatalf("create temp source: %v", err)
	}
	_ = tmpFile.Close()

	deps := runDeps{run: runSpeak}
	h := speakHandler(deps)
	result, gotResp, err := h(context.Background(), nil, speakArgs{Source: tmpFile.Name()})
	if err != nil {
		t.Fatalf("handler: %v", err)
	}

	// Channel A — typed Out (StructuredContent source).
	if len(gotResp.Transcript) != transcriptMaxEntries {
		t.Errorf("structuredContent transcript len: want %d, got %d", transcriptMaxEntries, len(gotResp.Transcript))
	}
	if !gotResp.TranscriptTruncated {
		t.Errorf("transcript_truncated must be true on oversized roster")
	}
	if want := preCap - transcriptMaxEntries; gotResp.TranscriptOmittedCount != want {
		t.Errorf("transcript_omitted_count: want %d, got %d", want, gotResp.TranscriptOmittedCount)
	}
	// Head-keep: first kept is block order 1.
	if gotResp.Transcript[0].Order != 1 {
		t.Errorf("head-keep: first transcript entry Order want 1, got %d", gotResp.Transcript[0].Order)
	}
	// receipt.blocks_played is unaffected by transcript truncation: it derives
	// from SinkReceipt (all played blocks), independent of the capped transcript
	// view. preCap exceeds the cap, so this proves the receipt still counts every
	// played block while the transcript array was bounded to the cap.
	if gotResp.Receipt.BlocksPlayed != preCap {
		t.Errorf("receipt.blocks_played must be unaffected by cap: want %d (full roster), got %d", preCap, gotResp.Receipt.BlocksPlayed)
	}

	// Channel B — serialized TextContent must parse to the SAME capped length and
	// signal fields (both channels bounded by the one marshal).
	if result == nil || len(result.Content) != 1 {
		t.Fatalf("want exactly one Content entry, got %+v", result)
	}
	tc, ok := result.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatalf("Content[0] must be *mcp.TextContent, got %T", result.Content[0])
	}
	var decoded speakResponse
	if err := json.Unmarshal([]byte(tc.Text), &decoded); err != nil {
		t.Fatalf("decode TextContent: %v", err)
	}
	if len(decoded.Transcript) != transcriptMaxEntries {
		t.Errorf("serialized transcript len: want %d, got %d", transcriptMaxEntries, len(decoded.Transcript))
	}
	if !decoded.TranscriptTruncated || decoded.TranscriptOmittedCount != gotResp.TranscriptOmittedCount {
		t.Errorf("dual-channel signal mismatch: structured {%v,%d} vs serialized {%v,%d}",
			gotResp.TranscriptTruncated, gotResp.TranscriptOmittedCount,
			decoded.TranscriptTruncated, decoded.TranscriptOmittedCount)
	}
}

// TestSpeakHandler_SmallTranscript_OmitemptyAbsent is the byte-identical
// counter-metric: an under-cap roster must serialize with NEITHER signal key
// present (omitempty absence) and the full roster length intact.
func TestSpeakHandler_SmallTranscript_OmitemptyAbsent(t *testing.T) {
	// Cannot t.Parallel() — mutates package-level newPipeline var.
	const roster = 3 // well under transcriptMaxEntries
	stub := &stubNarrator{
		receipt:        sink.SinkReceipt{BlocksPlayed: roster, TotalDurationMs: 300},
		blockSummaries: makeBlockSummaries(roster),
	}
	restore := withStubPipeline(t, stub)
	defer restore()

	tmpFile, err := os.CreateTemp(t.TempDir(), "sample-*.md")
	if err != nil {
		t.Fatalf("create temp source: %v", err)
	}
	_ = tmpFile.Close()

	deps := runDeps{run: runSpeak}
	h := speakHandler(deps)
	result, gotResp, err := h(context.Background(), nil, speakArgs{Source: tmpFile.Name()})
	if err != nil {
		t.Fatalf("handler: %v", err)
	}

	if len(gotResp.Transcript) != roster {
		t.Errorf("transcript len: want %d (full roster), got %d", roster, len(gotResp.Transcript))
	}
	if gotResp.TranscriptTruncated || gotResp.TranscriptOmittedCount != 0 {
		t.Errorf("under-cap must not flag truncation, got {%v,%d}", gotResp.TranscriptTruncated, gotResp.TranscriptOmittedCount)
	}

	tc, ok := result.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatalf("Content[0] must be *mcp.TextContent, got %T", result.Content[0])
	}
	if strings.Contains(tc.Text, "transcript_truncated") {
		t.Errorf("omitempty: transcript_truncated must be absent under cap, got %s", tc.Text)
	}
	if strings.Contains(tc.Text, "transcript_omitted_count") {
		t.Errorf("omitempty: transcript_omitted_count must be absent under cap, got %s", tc.Text)
	}
}

// makeBlockSummaries builds an n-block roster the stub narrator returns so real
// runSpeak projects + caps it. 1-based Order matches makeEntries.
func makeBlockSummaries(n int) []pipeline.BlockSummary {
	out := make([]pipeline.BlockSummary, n)
	for i := range out {
		out[i] = pipeline.BlockSummary{
			ID:         blockID(i + 1),
			Order:      i + 1,
			Level:      1,
			Status:     "voiced",
			SpokenText: "x",
		}
	}
	return out
}
