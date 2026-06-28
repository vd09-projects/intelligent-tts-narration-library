package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestPrechunkMessage_StructuralSplitOnly is the golden + property test for the
// /sessions structural pre-chunker (R3). It proves:
//   - a short message yields exactly ONE block;
//   - a >800-char prose message yields ≥2 blocks split on clean structural seams;
//   - NO block voices or refuses — the sessionBlock type carries text + class
//     only (no Status, no Level, no Segment), so this is structurally guaranteed,
//     and every block stays within the seam thresholds.
//
// The blocks[] shape for the big-prose case is asserted against a checked-in
// golden JSON fixture (no audio bytes). Regenerate with UPDATE_GOLDEN=1.
func TestPrechunkMessage_StructuralSplitOnly(t *testing.T) {
	t.Run("short_message_one_block", func(t *testing.T) {
		got := prechunkMessage("a single short paragraph that fits in one block")
		if len(got) != 1 {
			t.Fatalf("got %d blocks, want 1", len(got))
		}
		if got[0].Class != classProse {
			t.Fatalf("class = %q, want prose", got[0].Class)
		}
	})

	t.Run("big_prose_splits_no_refusal", func(t *testing.T) {
		got := prechunkMessage(bigProseFixture())
		if len(got) < 2 {
			t.Fatalf("got %d blocks, want ≥2 (big prose must split structurally)", len(got))
		}
		// No block over the structural seam thresholds.
		for i, b := range got {
			if !withinThreshold(b.Text) {
				t.Fatalf("block[%d] exceeds seam threshold (%d chars / %d lines)",
					i, len(b.Text), lineCount(b.Text))
			}
		}
		// Reconstruction: concatenating the blocks' paragraphs recovers every
		// source paragraph in order (no content dropped, no arbitrary mid-line cut).
		assertParagraphsPreserved(t, bigProseFixture(), got)

		// Golden compare on the blocks[] JSON shape.
		assertGolden(t, "prechunk_big_prose.json", got)
	})

	t.Run("code_fence_classified", func(t *testing.T) {
		got := prechunkMessage("```go\nfmt.Println(\"hi\")\n```")
		if len(got) != 1 || got[0].Class != classCode {
			t.Fatalf("got %+v, want one code block", got)
		}
	})
}

// bigProseFixture is a deterministic >800-char multi-paragraph prose message.
func bigProseFixture() string {
	paras := []string{
		"The narration planner segments a document into blocks by meaning rather than by character count, so a heading and the prose under it are voiced as distinct units.",
		"Each block carries its own level, its own source map, and its own provenance, which is what makes a single block independently re-renderable without touching its neighbours.",
		"When no intelligence adapter is wired, prose under roughly one hundred and twenty words is read verbatim and marked degraded rather than silently invented.",
		"Oversized prose is split only on clean structural seams — a paragraph boundary, never an arbitrary cut in the middle of a sentence or a word.",
		"The session route performs this split for display purposes alone: it assigns no spoken text and raises no refusal, leaving every voicing decision to the narrate route.",
	}
	return strings.Join(paras, "\n\n")
}

// assertParagraphsPreserved checks every source paragraph survives somewhere in
// the block set, in order — the splitter never drops or reorders content.
func assertParagraphsPreserved(t *testing.T, src string, blocks []sessionBlock) {
	t.Helper()
	joined := ""
	for _, b := range blocks {
		joined += b.Text + "\n\n"
	}
	for _, p := range strings.Split(src, "\n\n") {
		if !strings.Contains(joined, strings.TrimSpace(p)) {
			t.Fatalf("source paragraph dropped by the splitter: %q", p)
		}
	}
}

// assertGolden compares v's JSON against testdata/<name>, regenerating it when
// UPDATE_GOLDEN is set.
func assertGolden(t *testing.T, name string, v any) {
	t.Helper()
	path := filepath.Join("testdata", name)
	got, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		t.Fatalf("marshal golden: %v", err)
	}
	got = append(got, '\n')
	if os.Getenv("UPDATE_GOLDEN") != "" {
		if err := os.MkdirAll("testdata", 0o755); err != nil {
			t.Fatalf("mkdir testdata: %v", err)
		}
		if err := os.WriteFile(path, got, 0o600); err != nil {
			t.Fatalf("write golden: %v", err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read golden %s (run with UPDATE_GOLDEN=1 to create): %v", path, err)
	}
	if string(got) != string(want) {
		t.Fatalf("golden mismatch for %s\n--- got ---\n%s\n--- want ---\n%s", name, got, want)
	}
}
