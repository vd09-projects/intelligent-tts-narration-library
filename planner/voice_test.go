package planner

import (
	"strings"
	"testing"
)

// TestVoice_EveryDefaultEntry — table-driven coverage over DefaultLexicon.
// Each entry is exercised in a wrapping sentence so the longest-match
// path is real (not just isolated key replacement).
func TestVoice_EveryDefaultEntry(t *testing.T) {
	t.Parallel()
	lex := compileLexicon()
	for raw, spoken := range DefaultLexicon {
		raw, spoken := raw, spoken
		t.Run(raw, func(t *testing.T) {
			input := "left " + raw + " right"
			got := voice(input, lex)
			if !strings.Contains(got, spoken) {
				t.Errorf("voice(%q) = %q; want %q substring", input, got, spoken)
			}
			// The literal raw token must not survive as a standalone
			// substring (unless it overlaps with the spoken form,
			// which none of the defaults do).
			if strings.Contains(spoken, raw) {
				return
			}
			if strings.Contains(got, raw) {
				t.Errorf("voice(%q) = %q; raw token %q still present", input, got, raw)
			}
		})
	}
}

func TestVoice_LongestMatchWins(t *testing.T) {
	t.Parallel()
	lex := compileLexicon()
	got := voice("if x >= y", lex)
	if !strings.Contains(got, "greater than or equal to") {
		t.Errorf(">= must beat > and =: got %q", got)
	}
	if strings.Contains(got, "is equal to") && !strings.Contains(got, "greater than or equal to") {
		t.Errorf(">= was clipped to ==: got %q", got)
	}
}

func TestVoice_UserOverridesWin(t *testing.T) {
	t.Parallel()
	lex := compileLexicon(WithLexicon(Lexicon{"->": "to"}))
	got := voice("a -> b", lex)
	if !strings.Contains(got, "to") || strings.Contains(got, "arrow") {
		t.Errorf("user override should win: got %q", got)
	}
}

func TestVoice_UserAddsNewEntry(t *testing.T) {
	t.Parallel()
	lex := compileLexicon(WithLexicon(Lexicon{"WAT": "what a tangle"}))
	got := voice("see WAT", lex)
	if !strings.Contains(got, "what a tangle") {
		t.Errorf("new lexicon entry not applied: got %q", got)
	}
}

func TestVoice_PassesThroughWithoutMatches(t *testing.T) {
	t.Parallel()
	lex := compileLexicon()
	got := voice("plain sentence without symbols", lex)
	if got != "plain sentence without symbols" {
		t.Errorf("untouched input mutated: got %q", got)
	}
}

func TestVoice_EmptyInput(t *testing.T) {
	t.Parallel()
	lex := compileLexicon()
	if got := voice("", lex); got != "" {
		t.Errorf("empty input: got %q", got)
	}
}

func TestCompileLexicon_StableKeyOrder(t *testing.T) {
	t.Parallel()
	a := compileLexicon()
	b := compileLexicon()
	if len(a.keys) != len(b.keys) {
		t.Fatalf("key counts differ: %d vs %d", len(a.keys), len(b.keys))
	}
	for i := range a.keys {
		if a.keys[i] != b.keys[i] {
			t.Errorf("key %d differs: %q vs %q", i, a.keys[i], b.keys[i])
		}
	}
}
