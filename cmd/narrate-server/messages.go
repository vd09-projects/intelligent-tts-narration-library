package main

// messages.go — GET /sessions/{id}/messages (#109 Steps 2 + 3). Reads a Claude
// Code transcript (~/.claude/projects/*/{id}.jsonl) as an ordered message list,
// each big message STRUCTURALLY pre-chunked into display blocks.
//
// # Structural-split-only (plan R3)
//
// Pre-chunking here splits a message into display blocks on clean structural
// seams and NEVER voices and NEVER refuses. Blocks carry source text + a coarse
// structural class only — no Segment.Text, no Status, no Level. Voicing/refusal
// (incl. the ~120-word prose-without-intelligence refusal) is a /narrate-time
// concern, chosen per block later. The Step 0 spike confirmed the planner has no
// exported voice-free segmentation entry point (planner.Plan voices+levels), so
// this route uses the in-plan server-side structural splitter on the documented
// seams (CommonMark paragraph / ~20 lines / ~800 chars), not a planner call —
// keeping plan/ + planner/ imports untouched.
//
// # Message identity (plan R4)
//
// The response id is a server-computed StableID: the line's own uuid when
// present (the Step 0 spike found uuid on 100% of real user+assistant lines),
// else a deterministic "h:"-prefixed content hash so two uuid-less messages
// cannot collapse onto the empty string. It is NEVER transcript.Message.Turn
// (the #106 emit-index-is-not-a-cursor contract).

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/vd09-projects/intelligent-tts-narration-library/internal/transcript"
)

// sessionIDPattern — a session id is the jsonl basename (a Claude Code session
// uuid). Allowlist-before-glob: only these chars, so "..", "/", a separator, or
// an empty id is rejected BEFORE any filesystem touch (dots excluded, so ".."
// can never pass).
var sessionIDPattern = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

// Structural seam thresholds (CLAUDE.md oversized-block splitting for prose:
// ~20 lines / ~800 chars). A message within BOTH thresholds is one block.
const (
	chunkMaxChars = 800
	chunkMaxLines = 20
)

// sessionBlockClass — the coarse structural class a /sessions block carries.
// This is NOT the planner's full classification (that happens at /narrate); it
// is a cheap structural hint the splitter can determine without voicing.
type sessionBlockClass string

const (
	classProse sessionBlockClass = "prose"
	classCode  sessionBlockClass = "code" // fenced ``` block
)

// sessionBlock — one display block. Source text + coarse class only (R3): no
// Segment.Text, no Status, no Level.
type sessionBlock struct {
	Text  string            `json:"text"`
	Class sessionBlockClass `json:"class"`
}

// sessionMessage — one message in the ordered list. ID is the StableID (never
// Turn). Turn is surfaced as informational emit-order only, explicitly NOT the
// id/cursor (#106).
type sessionMessage struct {
	ID     string         `json:"id"`
	Role   string         `json:"role"`
	Turn   int            `json:"turn"`
	Blocks []sessionBlock `json:"blocks"`
}

// messagesResponse — GET /sessions/{id}/messages 200 body.
type messagesResponse struct {
	SessionID string           `json:"session_id"`
	Messages  []sessionMessage `json:"messages"`
}

// parseMessages — seam over the shared parser so tests can drive the mapping
// without a fixture HOME when convenient. Defaults to the real parser.
var parseMessages = transcript.ParseMessages

// messagesHandler serves GET /sessions/{id}/messages.
func messagesHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, reasonMethodNotAllowed, "GET only")
		return
	}

	id := r.PathValue("id")
	// Allowlist gate FIRST (before any fs touch). Empty / ".." / separators are
	// rejected here as a malformed id.
	if !sessionIDPattern.MatchString(id) {
		writeError(w, http.StatusBadRequest, reasonMissingField,
			"session id must match [A-Za-z0-9_-]+ (no separators or '..')")
		return
	}

	home, err := os.UserHomeDir()
	if err != nil {
		writeError(w, http.StatusInternalServerError, reasonInternal, "resolve home dir: "+err.Error())
		return
	}

	matches, err := filepath.Glob(filepath.Join(home, ".claude", "projects", "*", id+".jsonl"))
	if err != nil {
		writeError(w, http.StatusInternalServerError, reasonInternal, "glob sessions: "+err.Error())
		return
	}
	if len(matches) == 0 {
		writeError(w, http.StatusNotFound, reasonSourceNotFound, "no session transcript for id: "+id)
		return
	}

	// Q5 — deterministic pick on >1 match: newest mtime (the live session).
	path := newestByMTime(matches)

	msgs, err := parseMessages(path)
	if err != nil {
		writeError(w, http.StatusInternalServerError, reasonInternal, "parse transcript: "+err.Error())
		return
	}

	resp := messagesResponse{SessionID: id, Messages: make([]sessionMessage, 0, len(msgs))}
	for _, m := range msgs {
		resp.Messages = append(resp.Messages, sessionMessage{
			ID:     stableID(m),
			Role:   m.Role,
			Turn:   m.Turn,
			Blocks: prechunkMessage(m.Text),
		})
	}
	writeJSON(w, http.StatusOK, resp)
}

// newestByMTime returns the match with the newest mtime — the live session when
// one id appears under multiple project dirs (Q5). Stat faults are skipped; if
// all fail, the first match is returned deterministically.
func newestByMTime(matches []string) string {
	best := matches[0]
	var bestMod int64 = -1
	for _, m := range matches {
		fi, err := os.Stat(m)
		if err != nil {
			continue
		}
		if mod := fi.ModTime().UnixNano(); mod > bestMod {
			bestMod = mod
			best = m
		}
	}
	return best
}

// stableID computes the server-owned message id (R4). The parser stays
// tool-agnostic and never synthesises this — identity policy lives here.
//
//   - line uuid present → use it verbatim.
//   - uuid absent → "h:" + first 16 hex of sha256(role | 0x00 | turn | 0x00 |
//     text). Deterministic across re-parse, distinct for two uuid-less messages
//     (turn is the deterministic emit-index), and visibly namespaced ("h:") so a
//     debugger can tell a fallback id from a real uuid.
//
// NOTE (divergence from plan's literal formula): the plan wrote sha256 over the
// raw jsonl line bytes, but transcript.Message does not carry the raw line and
// widening it further than the planned uuid/timestamp fields is out of scope.
// role+turn+text satisfies every required property the plan pins (non-empty,
// deterministic, distinct, re-parse-stable, h:-prefixed). Turn is used only as
// an opaque hash INPUT — it is never surfaced as the id (the #106 contract bars
// Turn as the cursor/id, not as a hash ingredient).
func stableID(m transcript.Message) string {
	if m.UUID != "" {
		return m.UUID
	}
	h := sha256.Sum256([]byte(m.Role + "\x00" + strconv.Itoa(m.Turn) + "\x00" + m.Text))
	return "h:" + hex.EncodeToString(h[:])[:16]
}

// prechunkMessage splits text into structural display blocks (R3). Within both
// thresholds → exactly one block. Over → split on blank-line (CommonMark
// paragraph) seams, packing paragraphs into blocks up to the threshold; a single
// over-threshold paragraph is line-split (never an arbitrary mid-line cut). No
// voicing, no refusal — blocks carry text + coarse class only.
func prechunkMessage(text string) []sessionBlock {
	if withinThreshold(text) {
		return []sessionBlock{{Text: text, Class: classifyStructural(text)}}
	}

	paras := splitParagraphs(text)
	var blocks []sessionBlock
	var cur strings.Builder

	flush := func() {
		if cur.Len() == 0 {
			return
		}
		s := cur.String()
		blocks = append(blocks, sessionBlock{Text: s, Class: classifyStructural(s)})
		cur.Reset()
	}

	for _, p := range paras {
		if !withinThreshold(p) {
			// Paragraph itself too big: flush the accumulator, then line-split it.
			flush()
			// Carry the fence-open state across the split: classify the WHOLE
			// paragraph once and give every line-split sub-block that class, so a
			// continuation of a fenced code block stays "code" instead of being
			// mislabelled "prose" (only the first sub-block opens with the fence).
			subClass := classifyStructural(p)
			for _, sub := range splitByLines(p) {
				blocks = append(blocks, sessionBlock{Text: sub, Class: subClass})
			}
			continue
		}
		// Would appending p overflow the current block? If so, seal it first.
		if cur.Len() > 0 && !withinThreshold(cur.String()+"\n\n"+p) {
			flush()
		}
		if cur.Len() > 0 {
			cur.WriteString("\n\n")
		}
		cur.WriteString(p)
	}
	flush()

	if len(blocks) == 0 {
		// Degenerate (e.g. all-whitespace) — never voice, never refuse: one block.
		return []sessionBlock{{Text: text, Class: classifyStructural(text)}}
	}
	return blocks
}

// withinThreshold reports whether s is within BOTH the char and line seam
// thresholds (so it can stand as a single structural block).
func withinThreshold(s string) bool {
	return len(s) <= chunkMaxChars && lineCount(s) <= chunkMaxLines
}

// lineCount counts lines (1 + number of '\n'); empty string is 0 lines.
func lineCount(s string) int {
	if s == "" {
		return 0
	}
	return 1 + strings.Count(s, "\n")
}

// splitParagraphs splits on blank-line seams (CommonMark paragraph boundary),
// dropping empty groups and trimming trailing whitespace from each.
func splitParagraphs(text string) []string {
	lines := strings.Split(text, "\n")
	var paras []string
	var cur []string
	flush := func() {
		if len(cur) == 0 {
			return
		}
		p := strings.TrimRight(strings.Join(cur, "\n"), "\n")
		if strings.TrimSpace(p) != "" {
			paras = append(paras, p)
		}
		cur = cur[:0]
	}
	for _, ln := range lines {
		if strings.TrimSpace(ln) == "" {
			flush()
			continue
		}
		cur = append(cur, ln)
	}
	flush()
	if len(paras) == 0 {
		return []string{text}
	}
	return paras
}

// splitByLines breaks an over-threshold paragraph into ≤chunkMaxLines /
// ≤chunkMaxChars sub-blocks on line boundaries only (never mid-line).
func splitByLines(p string) []string {
	lines := strings.Split(p, "\n")
	var out []string
	var cur []string
	// curLen tracks the byte length of strings.Join(cur, "\n") incrementally so
	// the threshold check is O(1) per line — no per-line rebuild of the joined
	// candidate (which would be O(n^2) on one pathological huge paragraph).
	curLen := 0
	flush := func() {
		if len(cur) == 0 {
			return
		}
		out = append(out, strings.Join(cur, "\n"))
		cur = cur[:0]
		curLen = 0
	}
	for _, ln := range lines {
		// Byte length of the joined block if ln were appended (+1 for the '\n').
		candidateLen := len(ln)
		if len(cur) > 0 {
			candidateLen = curLen + 1 + len(ln)
		}
		if len(cur) > 0 && (candidateLen > chunkMaxChars || len(cur)+1 > chunkMaxLines) {
			flush()
		}
		if len(cur) > 0 {
			curLen += 1 + len(ln)
		} else {
			curLen = len(ln)
		}
		cur = append(cur, ln)
	}
	flush()
	if len(out) == 0 {
		return []string{p}
	}
	return out
}

// classifyStructural assigns the coarse structural class (R3) — a cheap
// determination the splitter can make without voicing. A block that opens with a
// CommonMark code fence is "code"; everything else is "prose".
func classifyStructural(s string) sessionBlockClass {
	trimmed := strings.TrimLeft(s, " \t\r\n")
	if strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~") {
		return classCode
	}
	return classProse
}
