package planner

import "testing"

func TestStripInlineMarkdown(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"plain text untouched", "replicas set to three", "replicas set to three"},
		{"strong unwrapped", "**feasible** but slow", "feasible but slow"},
		{"emphasis unwrapped", "this is *important* now", "this is important now"},
		{"underscore strong", "__bold__ claim", "bold claim"},
		{"code span backticks dropped", "the guard (`main.go:171`) is wired", "the guard (main.go:171) is wired"},
		{
			"mixed strong + code (the reported list item)",
			"The guard (`main.go:171`, `errPersistentNotImplemented`) is **still wired**.",
			"The guard (main.go:171, errPersistentNotImplemented) is still wired.",
		},
		{
			"heading body with arrow + strong (the reported heading)",
			`Persistence -> **feasible, but "lift guard" = SILENT FAIL.**`,
			`Persistence -> feasible, but "lift guard" = SILENT FAIL.`,
		},
		// CommonMark intraword rule: underscores inside a word are NOT emphasis,
		// so snake_case identifiers must survive byte-for-byte.
		{"snake_case preserved", "set err_persistent_not_implemented flag", "set err_persistent_not_implemented flag"},
		{"single snake identifier", "errPersistent_not_impl", "errPersistent_not_impl"},
		// A stray, unmatched delimiter is literal text in CommonMark and stays.
		{"unmatched strong stays literal", "two ** three", "two ** three"},
		{"link keeps label, drops url", "see [the docs](https://x.dev/y) now", "see the docs now"},
		{"fast path: no delimiters", "nothing to strip here", "nothing to strip here"},
		{"empty stays empty", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := stripInlineMarkdown(tt.in); got != tt.want {
				t.Errorf("stripInlineMarkdown(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

// TestVoiceStripsMarkdownBeforeLexicon proves the strip runs inside voice()
// ahead of the lexicon scan: a wrapped operator (**->**) must still resolve to
// its spoken form, and backticks/asterisks never reach Segment.Text.
func TestVoiceStripsMarkdownBeforeLexicon(t *testing.T) {
	lex := compileLexicon()
	if got := voice("**->**", lex); got != "arrow" {
		t.Errorf("voice(**->**) = %q, want %q", got, "arrow")
	}
	if got := voice("is **still wired**.", lex); got != "is still wired." {
		t.Errorf("voice with strong = %q, want %q", got, "is still wired.")
	}
}
