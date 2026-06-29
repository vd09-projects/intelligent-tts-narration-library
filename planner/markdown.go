// Inline-markdown stripping for voicing. Segment.Text is the literal words to
// speak (CLAUDE.md voicing invariant), so the formatting delimiters that
// goldmark keeps in rawBlock.text — emphasis (*, _), strong (**, __), code
// spans (`), and link syntax — must be removed before a fragment is voiced.
// Without this a heading like "**feasible**" or a list item with a
// `code span` would be spoken with the literal asterisks / backticks.
//
// The strip is AST-based, NOT a regex: goldmark already owns CommonMark's
// flanking + intraword rules, so `snake_case` underscores survive (intraword
// `_` is not emphasis) while a real `**bold**` pair is unwrapped. A literal,
// unmatched delimiter (a stray `**` that opens nothing) is left as text by
// CommonMark and therefore by us too — that is the honest reading.

package planner

import (
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/text"
)

// markdownDelimiters are the characters that can begin inline markdown
// formatting. If a fragment contains none of them there is nothing to strip,
// so we skip the parse entirely (the common case for already-clean text).
const markdownDelimiters = "*_`[]<"

// stripInlineMarkdown returns s with inline markdown formatting removed,
// leaving only the spoken text. Emphasis/strong wrappers are unwrapped, code
// spans keep their content (backticks dropped), and links keep their label
// (destination dropped). Block structure is irrelevant here — callers pass a
// single already-segmented fragment (a heading's text, one list item, a
// config value), so the result is the concatenation of the fragment's text
// nodes.
//
// Defensive fallback: if the walk produces an empty string (e.g. a fragment
// that is pure formatting punctuation), the original s is returned rather than
// voicing silence — never blank out content on a parse surprise.
func stripInlineMarkdown(s string) string {
	if !strings.ContainsAny(s, markdownDelimiters) {
		return s
	}
	src := []byte(s)
	doc := goldmark.DefaultParser().Parse(text.NewReader(src))

	var b strings.Builder
	b.Grow(len(s))
	_ = ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		switch t := n.(type) {
		case *ast.Text:
			b.Write(t.Segment.Value(src))
			// A soft/hard break inside a multi-line fragment becomes a space so
			// adjacent words do not fuse ("foo\nbar" -> "foo bar").
			if t.SoftLineBreak() || t.HardLineBreak() {
				b.WriteByte(' ')
			}
		case *ast.String:
			b.Write(t.Value)
		case *ast.AutoLink:
			// <https://x> keeps the URL/email text rather than vanishing.
			b.Write(t.Label(src))
		}
		return ast.WalkContinue, nil
	})

	out := strings.TrimSpace(b.String())
	if out == "" {
		return s
	}
	return out
}
