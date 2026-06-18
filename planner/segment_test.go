package planner

import (
	"testing"

	"github.com/vd09-projects/intelligent-tts-narration-library/adapter"
	"github.com/vd09-projects/intelligent-tts-narration-library/plan"
)

// stubDoc — build an adapter.RawDocument from a string + URI without
// touching the file adapter. Keeps tests inside planner/ honest about
// the no-I/O invariant.
func stubDoc(text, uri string) adapter.RawDocument {
	return adapter.RawDocument{
		Source: plan.SourceRef{
			Kind:        plan.SourceKindFile,
			URI:         uri,
			ContentHash: "deadbeef",
			Adapter:     "test@0.0.1",
		},
		Bytes: []byte(text),
	}
}

func TestSegment_MarkdownTopLevelNodes(t *testing.T) {
	t.Parallel()
	src := "# Title\n\nFirst paragraph.\n\n```go\nfunc main() {}\n```\n\n- a\n- b\n"
	blocks := segment(stubDoc(src, "sample.md"))
	if len(blocks) != 4 {
		t.Fatalf("want 4 blocks, got %d (%+v)", len(blocks), blocks)
	}
	wantHints := []blockKindHint{hintHeading, hintProse, hintCode, hintList}
	for i, want := range wantHints {
		if blocks[i].hint != want {
			t.Errorf("block %d: want hint %v, got %v", i, want, blocks[i].hint)
		}
	}
	if blocks[2].fenceInfo != "go" {
		t.Errorf("code fence info: want go, got %q", blocks[2].fenceInfo)
	}
}

func TestSegment_PlaintextBlankLineParagraphs(t *testing.T) {
	t.Parallel()
	src := "First paragraph line one.\nFirst paragraph line two.\n\n\nSecond paragraph.\n"
	blocks := segment(stubDoc(src, "notes.txt"))
	if len(blocks) != 2 {
		t.Fatalf("want 2 plaintext paragraphs, got %d", len(blocks))
	}
	if blocks[0].hint != hintProse || blocks[1].hint != hintProse {
		t.Errorf("plaintext fallback should emit hintProse, got %v / %v", blocks[0].hint, blocks[1].hint)
	}
}

func TestSegment_ImageOnlyParagraph(t *testing.T) {
	t.Parallel()
	src := "Intro.\n\n![chart](bench.png)\n"
	blocks := segment(stubDoc(src, "doc.md"))
	if len(blocks) != 2 {
		t.Fatalf("want 2 blocks, got %d", len(blocks))
	}
	if blocks[1].hint != hintImage {
		t.Errorf("image-only paragraph: want hintImage, got %v", blocks[1].hint)
	}
}

func TestSegment_TableNode(t *testing.T) {
	t.Parallel()
	src := "Intro.\n\n| a | b |\n|---|---|\n| 1 | 2 |\n"
	blocks := segment(stubDoc(src, "doc.md"))
	if len(blocks) < 2 {
		t.Fatalf("want at least 2 blocks, got %d", len(blocks))
	}
	if blocks[1].hint != hintTable {
		t.Errorf("table block: want hintTable, got %v", blocks[1].hint)
	}
}

func TestSegment_EmptyInput(t *testing.T) {
	t.Parallel()
	if blocks := segment(stubDoc("", "")); blocks != nil {
		t.Errorf("empty input: want nil blocks, got %d", len(blocks))
	}
}

func TestSegment_LineRangesAreOneIndexed(t *testing.T) {
	t.Parallel()
	src := "# Top\n\nPara on line 3.\n"
	blocks := segment(stubDoc(src, "doc.md"))
	if len(blocks) != 2 {
		t.Fatalf("want 2 blocks, got %d", len(blocks))
	}
	if blocks[0].startLine != 1 {
		t.Errorf("heading startLine: want 1, got %d", blocks[0].startLine)
	}
	if blocks[1].startLine != 3 {
		t.Errorf("paragraph startLine: want 3, got %d", blocks[1].startLine)
	}
}
