package transcript

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeJSONL writes the given lines (one per slice element) to a temp .jsonl
// file and returns its path.
func writeJSONL(t *testing.T, lines []string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "transcript.jsonl")
	body := ""
	for _, l := range lines {
		body += l + "\n"
	}
	if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
		t.Fatalf("write %s: %v", p, err)
	}
	return p
}

// TestParseMessages asserts the FULL ordered slice for each fixture — turn
// index, role, text, and tool names — not just a tail value. It covers both
// content shapes per role, multi-block join, tool-only turns, the speak_last
// signal surfacing in ToolNames, and the tolerance contract.
func TestParseMessages(t *testing.T) {
	tests := []struct {
		name  string
		lines []string
		want  []Message
	}{
		{
			name: "alternating user(string)/assistant(array) turns, ordered",
			lines: []string{
				`{"type":"user","message":{"content":"hi"}}`,
				`{"type":"assistant","message":{"content":[{"type":"text","text":"first reply"}]}}`,
				`{"type":"user","message":{"content":"more"}}`,
				`{"type":"assistant","message":{"content":[{"type":"text","text":"second reply"}]}}`,
			},
			want: []Message{
				{Turn: 0, Role: "user", Text: "hi"},
				{Turn: 1, Role: "assistant", Text: "first reply"},
				{Turn: 2, Role: "user", Text: "more"},
				{Turn: 3, Role: "assistant", Text: "second reply"},
			},
		},
		{
			name: "joins multiple text blocks in one assistant turn",
			lines: []string{
				`{"type":"assistant","message":{"content":[{"type":"text","text":"part one "},{"type":"text","text":"part two"}]}}`,
			},
			want: []Message{
				{Turn: 0, Role: "assistant", Text: "part one part two"},
			},
		},
		{
			name: "tool_use turn surfaces ToolNames incl speak_last with joined text",
			lines: []string{
				`{"type":"assistant","message":{"content":[{"type":"text","text":"sure, speaking"},{"type":"tool_use","name":"speak_last","input":{}}]}}`,
			},
			want: []Message{
				{Turn: 0, Role: "assistant", Text: "sure, speaking", ToolNames: []string{"speak_last"}},
			},
		},
		{
			// S2: the [text, tool_use speak_last, text] interleaving — proves the
			// parser joins ALL text blocks (both sides of the tool_use) and records
			// the tool name, so a caller can drop the whole turn on the signal.
			name: "interleaved text / tool_use speak_last / text joins both text blocks",
			lines: []string{
				`{"type":"assistant","message":{"content":[{"type":"text","text":"before "},{"type":"tool_use","name":"speak_last","input":{}},{"type":"text","text":"after"}]}}`,
			},
			want: []Message{
				{Turn: 0, Role: "assistant", Text: "before after", ToolNames: []string{"speak_last"}},
			},
		},
		{
			name: "tool-only assistant turn: empty Text, ToolNames populated, still emitted+counted",
			lines: []string{
				`{"type":"assistant","message":{"content":[{"type":"text","text":"earlier"}]}}`,
				`{"type":"assistant","message":{"content":[{"type":"tool_use","name":"some_other_tool","input":{}}]}}`,
			},
			want: []Message{
				{Turn: 0, Role: "assistant", Text: "earlier"},
				{Turn: 1, Role: "assistant", Text: "", ToolNames: []string{"some_other_tool"}},
			},
		},
		{
			// S1: a user turn with ARRAY content (e.g. a tool_result block) — both
			// content shapes are covered for the user role, not just string content.
			name: "user turn with array content (tool_result) yields empty text, user role",
			lines: []string{
				`{"type":"user","message":{"content":[{"type":"tool_result","content":"output"}]}}`,
			},
			want: []Message{
				{Turn: 0, Role: "user", Text: ""},
			},
		},
		{
			name: "tolerates non-json and skips non-conversation lines, keeps ordering",
			lines: []string{
				`not json at all`,
				`{"type":"user","message":{"content":"plain string content"}}`,
				`{"type":"summary","summary":"ignore me"}`,
				`{"type":"assistant","message":{"content":[{"type":"text","text":"kept"}]}}`,
			},
			want: []Message{
				{Turn: 0, Role: "user", Text: "plain string content"},
				{Turn: 1, Role: "assistant", Text: "kept"},
			},
		},
		{
			name: "bare non-string/non-array content (object) is skipped",
			lines: []string{
				`{"type":"user","message":{"content":{"unexpected":"object"}}}`,
				`{"type":"assistant","message":{"content":[{"type":"text","text":"survivor"}]}}`,
			},
			want: []Message{
				{Turn: 0, Role: "assistant", Text: "survivor"},
			},
		},
		{
			name:  "empty file yields empty slice and nil error",
			lines: []string{},
			want:  nil,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			p := writeJSONL(t, tc.lines)
			got, err := ParseMessages(p)
			if err != nil {
				t.Fatalf("ParseMessages: %v", err)
			}
			assertMessages(t, got, tc.want)
		})
	}
}

// TestParseMessages_LargeLineSurvives proves the raised 16 MiB line buffer: a
// single assistant line well over the default 64 KiB scanner cap is parsed AND
// its end-of-line content survives (a sentinel near the tail), not merely that
// the scan did not error.
func TestParseMessages_LargeLineSurvives(t *testing.T) {
	t.Parallel()
	const sentinel = "SENTINEL_TAIL_MARKER"
	// ~200 KiB of filler, then the sentinel at the very end of the text block —
	// far past the 64 KiB default cap, well under the 16 MiB ceiling.
	filler := strings.Repeat("x", 200*1024)
	big := `{"type":"assistant","message":{"content":[{"type":"text","text":"` + filler + sentinel + `"}]}}`

	p := writeJSONL(t, []string{big})
	got, err := ParseMessages(p)
	if err != nil {
		t.Fatalf("ParseMessages: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d messages, want 1 (line dropped by buffer cap?)", len(got))
	}
	if !strings.HasSuffix(got[0].Text, sentinel) {
		t.Fatalf("end-of-line sentinel did not survive: tail = %q", tail(got[0].Text, len(sentinel)+8))
	}
	if len(got[0].Text) != len(filler)+len(sentinel) {
		t.Fatalf("text length = %d, want %d (truncated mid-line?)", len(got[0].Text), len(filler)+len(sentinel))
	}
}

// TestParseMessages_LineIdentityFields proves the additive #109 fields: the
// line's top-level "uuid" / "timestamp" are captured verbatim, in file order,
// and a line that omits them yields empty fields (tolerated, not an error — the
// consumer supplies identity).
func TestParseMessages_LineIdentityFields(t *testing.T) {
	t.Parallel()
	p := writeJSONL(t, []string{
		`{"type":"user","uuid":"u-1","timestamp":"2026-06-28T00:00:01Z","message":{"content":"hi"}}`,
		`{"type":"assistant","uuid":"a-2","timestamp":"2026-06-28T00:00:02Z","message":{"content":[{"type":"text","text":"reply"}]}}`,
		`{"type":"user","message":{"content":"no uuid here"}}`,
	})
	got, err := ParseMessages(p)
	if err != nil {
		t.Fatalf("ParseMessages: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("got %d messages, want 3", len(got))
	}
	want := []struct{ uuid, ts string }{
		{"u-1", "2026-06-28T00:00:01Z"},
		{"a-2", "2026-06-28T00:00:02Z"},
		{"", ""},
	}
	for i, w := range want {
		if got[i].UUID != w.uuid || got[i].Timestamp != w.ts {
			t.Fatalf("message[%d]: got {UUID:%q Timestamp:%q}, want {UUID:%q Timestamp:%q}",
				i, got[i].UUID, got[i].Timestamp, w.uuid, w.ts)
		}
	}
}

// TestParseMessages_OpenError surfaces an I/O failure as an error (the only
// non-tolerant path) rather than swallowing it.
func TestParseMessages_OpenError(t *testing.T) {
	t.Parallel()
	_, err := ParseMessages(filepath.Join(t.TempDir(), "does-not-exist.jsonl"))
	if err == nil {
		t.Fatal("expected an error opening a missing transcript, got nil")
	}
}

func assertMessages(t *testing.T, got, want []Message) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("got %d messages, want %d\n got: %+v\nwant: %+v", len(got), len(want), got, want)
	}
	for i := range want {
		g, w := got[i], want[i]
		if g.Turn != w.Turn || g.Role != w.Role || g.Text != w.Text {
			t.Fatalf("message[%d]: got {Turn:%d Role:%q Text:%q}, want {Turn:%d Role:%q Text:%q}",
				i, g.Turn, g.Role, g.Text, w.Turn, w.Role, w.Text)
		}
		if !equalStrings(g.ToolNames, w.ToolNames) {
			t.Fatalf("message[%d] ToolNames: got %v, want %v", i, g.ToolNames, w.ToolNames)
		}
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func tail(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[len(s)-n:]
}
