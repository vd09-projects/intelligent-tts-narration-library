package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// writeLines writes the given JSONL lines to a temp file and returns its path.
func writeLines(t *testing.T, dir, name string, lines []string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	body := ""
	for _, l := range lines {
		body += l + "\n"
	}
	if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
		t.Fatalf("write %s: %v", p, err)
	}
	return p
}

func TestLastAssistantText(t *testing.T) {
	tests := []struct {
		name  string
		lines []string
		want  string
	}{
		{
			name: "picks last assistant text turn",
			lines: []string{
				`{"type":"user","message":{"content":"hi"}}`,
				`{"type":"assistant","message":{"content":[{"type":"text","text":"first reply"}]}}`,
				`{"type":"user","message":{"content":"more"}}`,
				`{"type":"assistant","message":{"content":[{"type":"text","text":"second reply"}]}}`,
			},
			want: "second reply",
		},
		{
			name: "excludes the invoking speak_last turn, returns prior",
			lines: []string{
				`{"type":"assistant","message":{"content":[{"type":"text","text":"the real answer"}]}}`,
				`{"type":"user","message":{"content":"speak it"}}`,
				`{"type":"assistant","message":{"content":[{"type":"text","text":"sure, speaking"},{"type":"tool_use","name":"speak_last","input":{}}]}}`,
			},
			want: "the real answer",
		},
		{
			name: "joins multiple text blocks in one turn",
			lines: []string{
				`{"type":"assistant","message":{"content":[{"type":"text","text":"part one "},{"type":"text","text":"part two"}]}}`,
			},
			want: "part one part two",
		},
		{
			name: "tolerates non-json and string-content lines",
			lines: []string{
				`not json at all`,
				`{"type":"user","message":{"content":"plain string content"}}`,
				`{"type":"assistant","message":{"content":[{"type":"text","text":"kept"}]}}`,
				`{"type":"summary","summary":"ignore me"}`,
			},
			want: "kept",
		},
		{
			name: "skips assistant turns with no text (tool-only)",
			lines: []string{
				`{"type":"assistant","message":{"content":[{"type":"text","text":"earlier"}]}}`,
				`{"type":"assistant","message":{"content":[{"type":"tool_use","name":"some_other_tool","input":{}}]}}`,
			},
			want: "earlier",
		},
		{
			name: "empty transcript yields empty",
			lines: []string{
				`{"type":"user","message":{"content":"only user"}}`,
			},
			want: "",
		},
	}

	dir := t.TempDir()
	for i, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			p := writeLines(t, dir, "t"+time.Now().Format("150405")+string(rune('a'+i))+".jsonl", tc.lines)
			got, err := lastAssistantText(p)
			if err != nil {
				t.Fatalf("lastAssistantText: %v", err)
			}
			if got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestNewestTranscript(t *testing.T) {
	root := t.TempDir()

	// Two project dirs, each with a transcript; the newer mtime must win.
	projA := filepath.Join(root, "-Users-x-repoA")
	projB := filepath.Join(root, "-Users-x-repoB")
	for _, d := range []string{projA, projB} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	older := writeLines(t, projA, "old.jsonl", []string{`{"type":"assistant","message":{"content":[{"type":"text","text":"old"}]}}`})
	newer := writeLines(t, projB, "new.jsonl", []string{`{"type":"assistant","message":{"content":[{"type":"text","text":"new"}]}}`})

	// Force a clear mtime ordering regardless of write speed.
	past := time.Now().Add(-2 * time.Hour)
	if err := os.Chtimes(older, past, past); err != nil {
		t.Fatal(err)
	}

	got, err := newestTranscript(root)
	if err != nil {
		t.Fatalf("newestTranscript: %v", err)
	}
	if got != newer {
		t.Fatalf("got %q, want %q", got, newer)
	}
}

func TestNewestTranscriptEmptyRoot(t *testing.T) {
	if _, err := newestTranscript(t.TempDir()); err == nil {
		t.Fatal("expected error for empty projects root, got nil")
	}
}
