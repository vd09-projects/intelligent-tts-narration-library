package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// fixtureSession writes lines to ~/.claude/projects/<proj>/<id>.jsonl under a
// fresh HOME and points HOME at it (so os.UserHomeDir + the glob resolve). Returns
// nothing — the caller drives the handler through the mux. t.Setenv bars
// t.Parallel for these tests (acceptable: each owns its own HOME).
func fixtureSession(t *testing.T, proj, id string, lines []string) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	dir := filepath.Join(home, ".claude", "projects", proj)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir projects: %v", err)
	}
	body := strings.Join(lines, "\n") + "\n"
	if err := os.WriteFile(filepath.Join(dir, id+".jsonl"), []byte(body), 0o600); err != nil {
		t.Fatalf("write jsonl: %v", err)
	}
}

// getMessages drives GET /sessions/{id}/messages through the full mux.
func getMessages(t *testing.T, id string) *httptest.ResponseRecorder {
	t.Helper()
	mux := newMux(defaultArgs(), newRenderStore(t.TempDir()))
	r := httptest.NewRequest(http.MethodGet, "/sessions/"+id+"/messages", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, r)
	return w
}

func decodeMessages(t *testing.T, w *httptest.ResponseRecorder) messagesResponse {
	t.Helper()
	var m messagesResponse
	if err := json.Unmarshal(w.Body.Bytes(), &m); err != nil {
		t.Fatalf("decode messagesResponse: %v (body %q)", err, w.Body.String())
	}
	return m
}

// ---------------------------------------------------------------------------
// Session id containment — the allowlist gate rejects bad ids BEFORE any fs
// touch (driven directly with SetPathValue so the mux's own path-cleaning does
// not pre-empt the handler's check).
// ---------------------------------------------------------------------------

func TestMessages_IDContainment(t *testing.T) {
	bad := []string{"", "..", "../etc", "a/b", "a.jsonl", "spaces here", "weird*char"}
	for _, id := range bad {
		t.Run("reject_"+id, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, "/sessions/x/messages", nil)
			r.SetPathValue("id", id)
			w := httptest.NewRecorder()
			messagesHandler(w, r)
			if w.Code != http.StatusBadRequest {
				t.Fatalf("id %q: status = %d, want 400", id, w.Code)
			}
			if got := decodeErr(t, w).Reason; got != reasonMissingField {
				t.Fatalf("id %q: reason = %q, want %q", id, got, reasonMissingField)
			}
		})
	}

	// A well-formed id passes the gate (and then 404s for no file under a HOME
	// with no matching transcript — proving the gate let it through).
	t.Run("accepts_valid", func(t *testing.T) {
		t.Setenv("HOME", t.TempDir())
		r := httptest.NewRequest(http.MethodGet, "/sessions/abc-123_DEF/messages", nil)
		r.SetPathValue("id", "abc-123_DEF")
		w := httptest.NewRecorder()
		messagesHandler(w, r)
		if w.Code != http.StatusNotFound {
			t.Fatalf("valid id with no file: status = %d, want 404", w.Code)
		}
		if got := decodeErr(t, w).Reason; got != reasonSourceNotFound {
			t.Fatalf("reason = %q, want %q", got, reasonSourceNotFound)
		}
	})
}

func TestMessages_MethodNotAllowed(t *testing.T) {
	r := httptest.NewRequest(http.MethodPost, "/sessions/x/messages", nil)
	r.SetPathValue("id", "x")
	w := httptest.NewRecorder()
	messagesHandler(w, r)
	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want 405", w.Code)
	}
	if got := decodeErr(t, w).Reason; got != reasonMethodNotAllowed {
		t.Fatalf("reason = %q, want %q", got, reasonMethodNotAllowed)
	}
}

// ---------------------------------------------------------------------------
// Glob + parse + mapping: ordered messages, id == server StableID (the line
// uuid), id is NOT Turn, stable across a re-parse (#106 regression guard).
// ---------------------------------------------------------------------------

func TestMessages_GlobParseMapping(t *testing.T) {
	const id = "sess-1"
	fixtureSession(t, "proj-a", id, []string{
		`{"type":"user","uuid":"uuid-A","message":{"content":"hello there"}}`,
		`{"type":"assistant","uuid":"uuid-B","message":{"content":[{"type":"text","text":"a short reply"}]}}`,
	})

	w := getMessages(t, id)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body %q)", w.Code, w.Body.String())
	}
	got := decodeMessages(t, w)
	if got.SessionID != id {
		t.Fatalf("session_id = %q, want %q", got.SessionID, id)
	}
	if len(got.Messages) != 2 {
		t.Fatalf("got %d messages, want 2", len(got.Messages))
	}
	if got.Messages[0].ID != "uuid-A" || got.Messages[1].ID != "uuid-B" {
		t.Fatalf("ids = %q,%q, want uuid-A,uuid-B (id must be the line uuid, not Turn)",
			got.Messages[0].ID, got.Messages[1].ID)
	}
	// Regression (#106): the id must NOT be the stringified Turn emit-index.
	for i, m := range got.Messages {
		if m.ID == "0" || m.ID == "1" || m.ID == itoa(m.Turn) {
			t.Fatalf("message[%d] id %q collides with Turn — the #106 cursor contract is broken", i, m.ID)
		}
	}

	// Re-parse: identical ids.
	w2 := getMessages(t, id)
	got2 := decodeMessages(t, w2)
	if got2.Messages[0].ID != got.Messages[0].ID || got2.Messages[1].ID != got.Messages[1].ID {
		t.Fatalf("ids not stable across re-parse: %v then %v", ids(got.Messages), ids(got2.Messages))
	}
}

// ---------------------------------------------------------------------------
// Missing-StableID identity (R4): two uuid-less messages get distinct, non-empty,
// "h:"-prefixed, re-parse-stable ids — they do NOT collide on "".
// ---------------------------------------------------------------------------

func TestMessages_MissingStableIDIdentity(t *testing.T) {
	const id = "sess-nouuid"
	fixtureSession(t, "proj-b", id, []string{
		`{"type":"user","message":{"content":"first uuid-less message"}}`,
		`{"type":"assistant","message":{"content":[{"type":"text","text":"second uuid-less message"}]}}`,
	})

	got := decodeMessages(t, getMessages(t, id))
	if len(got.Messages) != 2 {
		t.Fatalf("got %d messages, want 2", len(got.Messages))
	}
	a, b := got.Messages[0].ID, got.Messages[1].ID
	for _, x := range []string{a, b} {
		if x == "" {
			t.Fatal("uuid-less message got an empty id — identity collapsed")
		}
		if !strings.HasPrefix(x, "h:") {
			t.Fatalf("fallback id %q is not h:-prefixed", x)
		}
	}
	if a == b {
		t.Fatalf("two uuid-less messages collided on the same id %q", a)
	}

	// Re-parse: byte-identical fallback ids.
	got2 := decodeMessages(t, getMessages(t, id))
	if got2.Messages[0].ID != a || got2.Messages[1].ID != b {
		t.Fatalf("fallback ids not stable across re-parse: (%q,%q) then (%q,%q)",
			a, b, got2.Messages[0].ID, got2.Messages[1].ID)
	}
}

// ---------------------------------------------------------------------------
// Q5 multi-match tie-break: when one session id resolves under >1 project dir,
// newestByMTime deterministically returns the newest-mtime file (the live
// session) regardless of slice order.
// ---------------------------------------------------------------------------

func TestNewestByMTime(t *testing.T) {
	dir := t.TempDir()
	older := filepath.Join(dir, "older.jsonl")
	newer := filepath.Join(dir, "newer.jsonl")
	for _, p := range []string{older, newer} {
		if err := os.WriteFile(p, []byte("{}"), 0o600); err != nil {
			t.Fatalf("write %s: %v", p, err)
		}
	}
	// Pin mtimes explicitly so the test does not depend on write-order timing.
	base := time.Now()
	if err := os.Chtimes(older, base.Add(-2*time.Hour), base.Add(-2*time.Hour)); err != nil {
		t.Fatalf("chtimes older: %v", err)
	}
	if err := os.Chtimes(newer, base, base); err != nil {
		t.Fatalf("chtimes newer: %v", err)
	}

	tests := []struct {
		name    string
		matches []string
	}{
		{"newest_first", []string{newer, older}},
		{"newest_second", []string{older, newer}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := newestByMTime(tc.matches); got != newer {
				t.Fatalf("newestByMTime(%v) = %q, want %q (newest mtime)", tc.matches, got, newer)
			}
		})
	}
}

func ids(ms []sessionMessage) []string {
	out := make([]string, len(ms))
	for i, m := range ms {
		out[i] = m.ID
	}
	return out
}

func itoa(n int) string {
	// tiny local stringifier so the test does not import strconv just for this.
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
