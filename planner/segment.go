package planner

import (
	"strings"

	"github.com/vd09-projects/intelligent-tts-narration-library/adapter"
	"github.com/vd09-projects/intelligent-tts-narration-library/plan"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	gast "github.com/yuin/goldmark/extension/ast"
	"github.com/yuin/goldmark/text"
)

// blockKindHint — what the segmenter inferred about a raw block from the
// markdown AST (or from the plaintext-fallback shape). Hints are advisory:
// classify.go owns the final plan.Class decision and may override.
type blockKindHint int

const (
	hintUnknown blockKindHint = iota
	hintHeading
	hintList
	hintImage
	hintTable
	hintCode    // fenced code block — fenceInfo carries the language tag.
	hintProse   // paragraph or plaintext paragraph.
	hintBlockHTML
)

// rawBlock — one segmented unit before classification + leveling.
//
// Line numbers are 1-indexed, matching plan.SourceMap.StartLine /
// EndLine conventions (and the adapter.OffsetSpan origin).
type rawBlock struct {
	text      string
	startLine int
	endLine   int
	hint      blockKindHint
	// fenceInfo — only meaningful when hint == hintCode. The verbatim
	// info string from the opening fence (e.g. "yaml", "go", "mermaid",
	// or "" for an unmarked fence).
	fenceInfo string
}

// segment — convert a RawDocument into ordered rawBlocks.
//
// Detection: markdown if the doc URI ends in a known markdown extension
// OR the bytes contain a markdown-shaped signal (ATX heading at line
// start, fenced-code marker, table pipe row). Otherwise plaintext.
//
// Plaintext fallback: split on blank-line boundaries (two or more
// consecutive newlines). Each non-empty paragraph becomes one rawBlock
// with hint = hintProse.
//
// Markdown path: parse with goldmark (extension.Table enabled so table
// nodes appear in the AST) and walk top-level children. Each top-level
// node becomes one rawBlock; nested content stays inside the block text.
func segment(doc adapter.RawDocument) []rawBlock {
	if len(doc.Bytes) == 0 {
		return nil
	}
	if isMarkdown(doc) {
		return segmentMarkdown(doc.Bytes)
	}
	return segmentPlaintext(doc.Bytes)
}

// markdownExts — file extensions we treat as markdown without sniffing.
var markdownExts = []string{".md", ".markdown", ".mdx"}

// isMarkdown — best-effort detection. Extension wins; otherwise look for
// any of the three strongest markdown signals (ATX heading, fenced code,
// pipe-table separator). Conservative — when in doubt, fall through to
// plaintext rather than risk goldmark normalising prose into weird AST.
func isMarkdown(doc adapter.RawDocument) bool {
	uri := strings.ToLower(doc.Source.URI)
	for _, ext := range markdownExts {
		if strings.HasSuffix(uri, ext) {
			return true
		}
	}
	src := string(doc.Bytes)
	// ATX heading at line start.
	if startsWithLine(src, "# ") || startsWithLine(src, "## ") ||
		strings.Contains(src, "\n# ") || strings.Contains(src, "\n## ") {
		return true
	}
	// Fenced code.
	if strings.Contains(src, "\n```") || strings.HasPrefix(src, "```") {
		return true
	}
	// Pipe-table separator row.
	if strings.Contains(src, "|---") || strings.Contains(src, "| ---") {
		return true
	}
	return false
}

// startsWithLine — does src start with prefix at column zero?
func startsWithLine(src, prefix string) bool {
	return strings.HasPrefix(src, prefix)
}

// segmentMarkdown — parse with goldmark + table extension and walk top-
// level children. Each top-level AST node becomes one rawBlock.
//
// Decision: tradeoff — we walk only top-level children rather than the
// full AST. This keeps a paragraph-with-inline-code as one rawBlock
// (correct: it's still prose). The cost is that a code fence nested
// inside a blockquote becomes part of the blockquote text — acceptable
// because blockquotes are rare in narration source.
// Why: the alternative (full AST traversal flattening fenced children
// out of parents) would re-order content and break source-map line
// ranges. Block-level sync is the load-bearing invariant.
func segmentMarkdown(src []byte) []rawBlock {
	md := goldmark.New(goldmark.WithExtensions(extension.Table))
	reader := text.NewReader(src)
	root := md.Parser().Parse(reader)

	var blocks []rawBlock
	for n := root.FirstChild(); n != nil; n = n.NextSibling() {
		rb, ok := rawBlockFromNode(n, src)
		if !ok {
			continue
		}
		blocks = append(blocks, rb)
	}
	return blocks
}

// rawBlockFromNode — extract a rawBlock from one top-level AST node.
// Returns ok=false for nodes that carry no narratable content (e.g.,
// thematic breaks).
func rawBlockFromNode(n ast.Node, src []byte) (rawBlock, bool) {
	hint, fenceInfo := nodeHint(n, src)

	// Materialise the verbatim source slice for this node so leveling
	// has the original bytes (with leading "## " for headings, fence
	// markers for code, etc.). This keeps Segment.Text construction
	// honest and the SourceMap.RawExcerpt populated.
	start, end := nodeByteSpan(n, src)
	if start < 0 || end <= start || end > len(src) {
		return rawBlock{}, false
	}
	text := string(src[start:end])
	if strings.TrimSpace(text) == "" {
		return rawBlock{}, false
	}
	startLine, endLine := byteSpanToLines(src, start, end)

	// Image-only paragraph detection — a paragraph node whose only
	// content is a single image becomes hintImage.
	if hint == hintProse && isImageOnlyParagraph(n) {
		hint = hintImage
	}

	return rawBlock{
		text:      text,
		startLine: startLine,
		endLine:   endLine,
		hint:      hint,
		fenceInfo: fenceInfo,
	}, true
}

// nodeHint — map a goldmark AST node type to a blockKindHint. Returns
// the fence info string for hintCode nodes.
func nodeHint(n ast.Node, src []byte) (blockKindHint, string) {
	switch v := n.(type) {
	case *ast.Heading:
		return hintHeading, ""
	case *ast.List:
		return hintList, ""
	case *ast.FencedCodeBlock:
		info := ""
		if v.Info != nil {
			info = string(v.Info.Segment.Value(src))
			// Goldmark exposes the full info string (lang + attrs).
			// Trim to the first whitespace-delimited token — the lang.
			if i := strings.IndexAny(info, " \t"); i >= 0 {
				info = info[:i]
			}
		}
		return hintCode, info
	case *ast.CodeBlock:
		// Indented (non-fenced) code block. Treat as code with empty
		// fence info — classifier will fall back to ClassCode default.
		return hintCode, ""
	case *gast.Table:
		return hintTable, ""
	case *ast.HTMLBlock:
		return hintBlockHTML, ""
	case *ast.Paragraph:
		return hintProse, ""
	}
	return hintUnknown, ""
}

// nodeByteSpan — compute the byte span of a top-level node within src.
// Goldmark exposes Lines() for block nodes; we take the first line's
// Start and the last line's Stop. For nodes without lines, return -1.
func nodeByteSpan(n ast.Node, _ []byte) (int, int) {
	if n.Type() != ast.TypeBlock {
		return -1, -1
	}
	lines := n.Lines()
	if lines == nil || lines.Len() == 0 {
		return -1, -1
	}
	first := lines.At(0)
	last := lines.At(lines.Len() - 1)
	return first.Start, last.Stop
}

// byteSpanToLines — convert byte offsets in src to 1-indexed line numbers.
// startLine is the line containing src[startByte]; endLine is the line
// containing src[endByte-1].
func byteSpanToLines(src []byte, startByte, endByte int) (int, int) {
	startLine := 1
	for i := 0; i < startByte && i < len(src); i++ {
		if src[i] == '\n' {
			startLine++
		}
	}
	endLine := startLine
	last := endByte - 1
	if last < startByte {
		last = startByte
	}
	if last >= len(src) {
		last = len(src) - 1
	}
	for i := startByte; i <= last && i < len(src); i++ {
		if src[i] == '\n' && i < last {
			endLine++
		}
	}
	return startLine, endLine
}

// isImageOnlyParagraph — true when n is a paragraph whose only inline
// content is a single image (optionally surrounded by whitespace).
//
// This catches the bare-image case (`![chart](bench.png)`) that the
// honesty rule refuses downstream.
func isImageOnlyParagraph(n ast.Node) bool {
	if n.Kind() != ast.KindParagraph {
		return false
	}
	var imgCount, otherCount int
	for c := n.FirstChild(); c != nil; c = c.NextSibling() {
		switch c.Kind() {
		case ast.KindImage:
			imgCount++
		case ast.KindText:
			// Allow text children only if they're whitespace.
			if t, ok := c.(*ast.Text); ok {
				// We cannot resolve the segment without src here;
				// safe heuristic: any text child disqualifies unless
				// the paragraph holds at least one image and exactly
				// one text child. Treat any text as content.
				_ = t
				otherCount++
			} else {
				otherCount++
			}
		default:
			otherCount++
		}
	}
	return imgCount == 1 && otherCount == 0
}

// segmentPlaintext — split bytes on blank-line boundaries. Each non-empty
// paragraph becomes one rawBlock with hint = hintProse. Line numbers are
// 1-indexed.
//
// A "blank line" is a line containing only whitespace (spaces, tabs,
// CR). Two or more consecutive blank lines delimit paragraphs.
func segmentPlaintext(src []byte) []rawBlock {
	var blocks []rawBlock
	lines := splitLinesKeep(src)

	lineNo := 1
	i := 0
	for i < len(lines) {
		// Skip leading blank lines.
		for i < len(lines) && isBlankLine(lines[i]) {
			lineNo++
			i++
		}
		if i >= len(lines) {
			break
		}
		startLine := lineNo
		var buf strings.Builder
		for i < len(lines) && !isBlankLine(lines[i]) {
			buf.WriteString(lines[i])
			i++
			lineNo++
		}
		endLine := lineNo - 1
		text := buf.String()
		if strings.TrimSpace(text) == "" {
			continue
		}
		blocks = append(blocks, rawBlock{
			text:      text,
			startLine: startLine,
			endLine:   endLine,
			hint:      hintProse,
		})
	}
	return blocks
}

// splitLinesKeep — split src into lines preserving the trailing '\n'.
// Mirrors adapter/file's per-line span semantics.
func splitLinesKeep(src []byte) []string {
	var out []string
	start := 0
	for i := range len(src) {
		if src[i] == '\n' {
			out = append(out, string(src[start:i+1]))
			start = i + 1
		}
	}
	if start < len(src) {
		out = append(out, string(src[start:]))
	}
	return out
}

// isBlankLine — true if line contains only whitespace (spaces, tabs, CR,
// LF). Used as the plaintext paragraph delimiter.
func isBlankLine(line string) bool {
	for i := 0; i < len(line); i++ {
		switch line[i] {
		case ' ', '\t', '\r', '\n':
			continue
		default:
			return false
		}
	}
	return true
}

// rawBlockSourceMap — derive a per-block plan.SourceMap from a rawBlock
// and the source-document SourceRef. Kept in this file because it pairs
// 1:1 with segmentation output; classify/level/degrade all consume the
// result.
func rawBlockSourceMap(rb rawBlock, doc adapter.RawDocument) plan.SourceMap {
	kind := plan.SourceKindLineRange
	switch doc.Source.Kind {
	case plan.SourceKindRawText, plan.SourceKindMCPText:
		kind = plan.SourceKindCharSpan
	case plan.SourceKindOCRScreenshot:
		kind = plan.SourceKindPixelRegion
	}
	return plan.SourceMap{
		Kind:       kind,
		StartLine:  rb.startLine,
		EndLine:    rb.endLine,
		RawExcerpt: rb.text,
	}
}
