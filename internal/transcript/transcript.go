// Package transcript parses a Claude Code session transcript (.jsonl) into an
// ordered list of conversation messages. It is the single shared parser used by
// the cmd/ composition roots: cmd/narrate-mcp layers its speak_last
// last-assistant policy on top of ParseMessages, and cmd/narrate-server (#109,
// GET /sessions/{id}/messages) will serve the full ordered list directly.
//
// The parser is tolerant by design: non-JSON lines, schema-variant lines, and
// content shapes it does not recognise are skipped rather than erroring —
// transcripts evolve and a single odd line must not abort the scan. The only
// error it returns is an I/O failure (open / read).
//
// It is deliberately tool-agnostic: it records the names of any tool_use blocks
// in ToolNames but knows no specific tool. Callers apply their own tool-specific
// policy (e.g. excluding a self-invoking speak_last turn) over ToolNames.
package transcript

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// Message is one user or assistant message from a transcript, in file order.
//
// Turn is the sequential index (starting at 0) over ALL emitted user+assistant
// messages in file order — tool-only turns are counted and emitted, not
// skipped. Text MAY be empty (e.g. a turn whose only blocks are tool_use), in
// which case ToolNames carries the signal. Role is "user" or "assistant".
//
// UUID and Timestamp are the line-derived raw identity fields (#109). They
// carry the transcript line's own top-level "uuid" / "timestamp" VERBATIM —
// empty when the line omits them. The parser stays tool-agnostic and reports
// the raw line truthfully: it does NOT synthesise a fallback id when uuid is
// absent. Identity policy (e.g. a content-hash fallback for a uuid-less line)
// belongs to the consumer (cmd/narrate-server derives a StableID), not here.
type Message struct {
	Turn      int      // sequential index over all emitted messages, file order
	Role      string   // "user" | "assistant"
	Text      string   // joined text blocks; may be empty for tool-only turns
	ToolNames []string // names of tool_use blocks in this turn, in order
	UUID      string   // line's top-level "uuid" verbatim; "" when absent (#109)
	Timestamp string   // line's top-level "timestamp" verbatim; "" when absent (#109)
}

// ParseMessages reads the transcript .jsonl at path and returns every user and
// assistant message in file order. Non-recognised lines are skipped (tolerance);
// only an open/read failure returns an error. An empty or all-skipped file
// yields (nil, nil).
//
// It owns the 16 MiB line buffer so a single very long assistant turn is not
// silently dropped mid-line (the default bufio.Scanner cap is 64 KiB).
func ParseMessages(path string) ([]Message, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("internal_error: open transcript: %w", err)
	}
	defer func() { _ = f.Close() }()

	sc := bufio.NewScanner(f)
	// Messages can be large; raise the line cap well above the default 64 KiB so
	// a long turn is not silently dropped mid-line.
	sc.Buffer(make([]byte, 0, 1024*1024), 16*1024*1024)

	var msgs []Message
	turn := 0
	for sc.Scan() {
		var line struct {
			Type      string `json:"type"`
			UUID      string `json:"uuid"`
			Timestamp string `json:"timestamp"`
			Message   struct {
				Content json.RawMessage `json:"content"`
			} `json:"message"`
		}
		if json.Unmarshal(sc.Bytes(), &line) != nil {
			continue // not JSON we understand — skip
		}
		if line.Type != "user" && line.Type != "assistant" {
			continue // not a conversation message (summary, system, ...) — skip
		}

		text, toolNames, ok := extractContent(line.Message.Content)
		if !ok {
			continue // content neither a string nor a known block array — skip
		}
		msgs = append(msgs, Message{
			Turn:      turn,
			Role:      line.Type,
			Text:      text,
			ToolNames: toolNames,
			UUID:      line.UUID,
			Timestamp: line.Timestamp,
		})
		turn++
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("internal_error: read transcript: %w", err)
	}
	return msgs, nil
}

// extractContent pulls spoken text and tool_use names out of a message's
// `content`, handling both transcript shapes:
//
//   - string content (the common user shape: "content":"hi") → that string is
//     the text, no tool names.
//   - block-array content (the assistant shape, and array-valued user content
//     such as tool_result blocks) → join the "text" blocks and collect the
//     "name" of each "tool_use" block.
//
// ok is false only when content is neither shape (e.g. a JSON object), so the
// caller skips the line. A well-formed empty array yields ("", nil, true).
func extractContent(raw json.RawMessage) (text string, toolNames []string, ok bool) {
	// Try the string shape first (user turns carry a bare string).
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s, nil, true
	}

	// Otherwise try the block-array shape (assistant turns; array-valued user
	// content like tool_result).
	var blocks []struct {
		Type string `json:"type"`
		Text string `json:"text"`
		Name string `json:"name"`
	}
	if json.Unmarshal(raw, &blocks) != nil {
		return "", nil, false // neither string nor array — skip the line
	}

	var sb strings.Builder
	for _, b := range blocks {
		switch b.Type {
		case "text":
			sb.WriteString(b.Text)
		case "tool_use":
			if b.Name != "" {
				toolNames = append(toolNames, b.Name)
			}
		}
	}
	return sb.String(), toolNames, true
}
