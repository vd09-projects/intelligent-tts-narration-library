package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

// doArtifact drives one GET /artifact through the full mux and returns the
// recorder. Pulled out so every artifact test shares the CORS + routing path.
func doArtifact(t *testing.T, args serverArgs, query, origin string) *httptest.ResponseRecorder {
	t.Helper()
	mux := newMux(args)
	r := httptest.NewRequest(http.MethodGet, "/artifact?"+query, nil)
	if origin != "" {
		r.Header.Set("Origin", origin)
	}
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, r)
	return w
}

// seedArtifactDir writes a minimal dir holding all four servable artifacts
// (audio.wav, manifest.json, plan.json, source.md), returning the dir plus the
// bytes the streaming route should echo back. Bytes are arbitrary — the route
// does not parse them, it streams them.
func seedArtifactDir(t *testing.T) (dir string, audioBytes, manifestBytes []byte) {
	t.Helper()
	dir = t.TempDir()
	audioBytes = []byte("RIFF....WAVEfake-audio-bytes")
	manifestBytes = []byte(`{"schema_version":1,"blocks":[]}`)
	writeFileT(t, filepath.Join(dir, "audio.wav"), audioBytes)
	writeFileT(t, filepath.Join(dir, "manifest.json"), manifestBytes)
	writeFileT(t, filepath.Join(dir, "plan.json"), []byte(`{"schema_version":"1"}`))
	writeFileT(t, filepath.Join(dir, "source.md"), []byte("# source\n\nbody\n"))
	return dir, audioBytes, manifestBytes
}

func TestArtifact_HappyPath_AudioAndManifest(t *testing.T) {
	dir, audioBytes, manifestBytes := seedArtifactDir(t)
	args := defaultArgs()

	cases := []struct {
		name        string
		contentType string
		want        []byte
	}{
		{"audio.wav", "audio/wav", audioBytes},
		{"manifest.json", "application/json", manifestBytes},
		// #70 widened the allowlist: plan.json + source.md now serve from the same
		// containment pipeline. Content-Type strings pinned from the map.
		{"plan.json", "application/json", []byte(`{"schema_version":"1"}`)},
		{"source.md", "text/markdown; charset=utf-8", []byte("# source\n\nbody\n")},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := doArtifact(t, args, "dir="+dir+"&name="+tc.name, "")
			if w.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200 (body %q)", w.Code, w.Body.String())
			}
			if got := w.Body.Bytes(); string(got) != string(tc.want) {
				t.Fatalf("body = %q, want %q", got, tc.want)
			}
			if ct := w.Header().Get("Content-Type"); ct != tc.contentType {
				t.Fatalf("Content-Type = %q, want %q", ct, tc.contentType)
			}
			if cc := w.Header().Get("Cache-Control"); cc != "no-store" {
				t.Fatalf("Cache-Control = %q, want no-store", cc)
			}
		})
	}
}

func TestArtifact_MethodNotAllowed(t *testing.T) {
	dir, _, _ := seedArtifactDir(t)
	mux := newMux(defaultArgs())
	r := httptest.NewRequest(http.MethodPost, "/artifact?dir="+dir+"&name=audio.wav", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, r)
	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want 405", w.Code)
	}
	if e := decodeErr(t, w); e.Reason != reasonMethodNotAllowed {
		t.Fatalf("reason = %q, want %q", e.Reason, reasonMethodNotAllowed)
	}
}

func TestArtifact_MissingFields(t *testing.T) {
	dir, _, _ := seedArtifactDir(t)
	args := defaultArgs()
	cases := []struct{ name, query string }{
		{"missing dir", "name=audio.wav"},
		{"empty dir", "dir=&name=audio.wav"},
		{"missing name", "dir=" + dir},
		{"empty name", "dir=" + dir + "&name="},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := doArtifact(t, args, tc.query, "")
			if w.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400 (body %q)", w.Code, w.Body.String())
			}
			if e := decodeErr(t, w); e.Reason != reasonMissingField {
				t.Fatalf("reason = %q, want %q", e.Reason, reasonMissingField)
			}
		})
	}
}

func TestArtifact_AllowlistRejectsNonServable(t *testing.T) {
	dir, _, _ := seedArtifactDir(t)
	args := defaultArgs()
	// Names that are not in the closed allowlist, OR that carry a separator / ".."
	// even for an allowlisted leaf, are missing_field — a bad artifact NAME, never
	// served, never a 404 (#70: plan.json + source.md are now ALLOWLISTED leaves,
	// so they move to the happy-path table; their traversal-decorated forms stay
	// here with SYMMETRIC coverage alongside the audio/manifest forms).
	cases := []string{
		// non-allowlisted name
		"index.html",
		// traversal / separator forms of the four allowlisted leaves — symmetric
		// reject net so widening the allowlist cannot open a traversal for any name.
		"../audio.wav", "sub/audio.wav",
		"../manifest.json", "sub/manifest.json",
		"../plan.json", "sub/plan.json",
		"../source.md", "sub/source.md",
		// bare traversal tokens
		"..", "audio.wav..",
	}
	for _, name := range cases {
		t.Run(name, func(t *testing.T) {
			w := doArtifact(t, args, "dir="+dir+"&name="+name, "")
			if w.Code != http.StatusBadRequest {
				t.Fatalf("name %q: status = %d, want 400 (body %q)", name, w.Code, w.Body.String())
			}
			if e := decodeErr(t, w); e.Reason != reasonMissingField {
				t.Fatalf("name %q: reason = %q, want %q", name, e.Reason, reasonMissingField)
			}
		})
	}
}

// TestArtifact_PathTraversal proves an allowlisted name cannot reach a file
// outside the dir: the allowlist already blocks "../" in the name, so traversal
// is attempted via a dir that resolves elsewhere — the served file must still be
// inside dir. We assert the only servable bytes are dir's own audio.wav.
func TestArtifact_TraversalViaName(t *testing.T) {
	dir, _, _ := seedArtifactDir(t)
	args := defaultArgs()
	// A name encoding a traversal is rejected by the allowlist (missing_field),
	// never resolved against the filesystem.
	for _, name := range []string{"../../etc/passwd", "/etc/passwd", "../audio.wav"} {
		w := doArtifact(t, args, "dir="+dir+"&name="+name, "")
		if w.Code != http.StatusBadRequest {
			t.Fatalf("name %q: status = %d, want 400", name, w.Code)
		}
		if e := decodeErr(t, w); e.Reason != reasonMissingField {
			t.Fatalf("name %q: reason = %q, want missing_field", name, e.Reason)
		}
	}
}

// TestArtifact_SiblingSharedPrefixRejected proves the filepath.Rel boundary
// check (not a raw strings.HasPrefix). base=/tmp/X/base, and a sibling
// /tmp/X/base-evil sharing the "base" prefix MUST NOT be reachable. We point dir
// at base but craft a symlink leaf escaping to base-evil to exercise the Rel
// boundary on a sibling-shared-prefix path.
func TestArtifact_SiblingSharedPrefixRejected(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink semantics differ on windows")
	}
	parent := t.TempDir()
	base := filepath.Join(parent, "base")
	evil := filepath.Join(parent, "base-evil")
	if err := os.MkdirAll(base, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(evil, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFileT(t, filepath.Join(evil, "audio.wav"), []byte("EVIL"))
	// audio.wav inside base is a symlink pointing into the sibling base-evil.
	if err := os.Symlink(filepath.Join(evil, "audio.wav"), filepath.Join(base, "audio.wav")); err != nil {
		t.Fatal(err)
	}

	w := doArtifact(t, defaultArgs(), "dir="+base+"&name=audio.wav", "")
	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404 (sibling-prefix escape must be rejected; body %q)", w.Code, w.Body.String())
	}
	if e := decodeErr(t, w); e.Reason != reasonSourceNotFound {
		t.Fatalf("reason = %q, want source_not_found", e.Reason)
	}
}

// TestArtifact_SymlinkEscapeRejected — an allowlisted audio.wav that is a
// symlink whose target is OUTSIDE the dir is rejected 404 (never 500).
func TestArtifact_SymlinkEscapeRejected(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink semantics differ on windows")
	}
	outside := t.TempDir()
	writeFileT(t, filepath.Join(outside, "secret.wav"), []byte("SECRET"))
	dir := t.TempDir()
	if err := os.Symlink(filepath.Join(outside, "secret.wav"), filepath.Join(dir, "audio.wav")); err != nil {
		t.Fatal(err)
	}

	w := doArtifact(t, defaultArgs(), "dir="+dir+"&name=audio.wav", "")
	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404 (symlink escape must be rejected, not 500; body %q)", w.Code, w.Body.String())
	}
	if e := decodeErr(t, w); e.Reason != reasonSourceNotFound {
		t.Fatalf("reason = %q, want source_not_found", e.Reason)
	}
}

// TestArtifact_SymlinkInsideDirServed — an allowlisted audio.wav that is a
// symlink to a sibling file INSIDE the resolved dir is served 200.
func TestArtifact_SymlinkInsideDirServed(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink semantics differ on windows")
	}
	dir := t.TempDir()
	writeFileT(t, filepath.Join(dir, "real.wav"), []byte("REALBYTES"))
	if err := os.Symlink(filepath.Join(dir, "real.wav"), filepath.Join(dir, "audio.wav")); err != nil {
		t.Fatal(err)
	}

	w := doArtifact(t, defaultArgs(), "dir="+dir+"&name=audio.wav", "")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (in-dir symlink should serve; body %q)", w.Code, w.Body.String())
	}
	if got := w.Body.String(); got != "REALBYTES" {
		t.Fatalf("body = %q, want REALBYTES", got)
	}
}

// TestArtifact_AbsentArtifact — dir exists but audio.wav is absent →
// EvalSymlinks ErrNotExist → source_not_found 404, asserted DISTINCT from 500.
func TestArtifact_AbsentArtifact(t *testing.T) {
	dir := t.TempDir()
	writeFileT(t, filepath.Join(dir, "manifest.json"), []byte("{}"))
	// No audio.wav.
	w := doArtifact(t, defaultArgs(), "dir="+dir+"&name=audio.wav", "")
	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404 (absent artifact, not 500; body %q)", w.Code, w.Body.String())
	}
	e := decodeErr(t, w)
	if e.Reason != reasonSourceNotFound {
		t.Fatalf("reason = %q, want source_not_found (NOT internal)", e.Reason)
	}
	if e.Reason == reasonInternal {
		t.Fatalf("absent artifact must not surface as internal/500")
	}
}

// TestArtifact_SourceMdAbsentIs404 — source.md is an ALLOWLISTED but OPTIONAL
// artifact (#70). When the dir exists but source.md is absent, the request must
// resolve to source_not_found 404 (EvalSymlinks ErrNotExist), NOT a 500. This is
// the honesty-rule boundary the player relies on to treat source as optional.
func TestArtifact_SourceMdAbsentIs404(t *testing.T) {
	dir := t.TempDir()
	// A complete-enough dir, but deliberately NO source.md.
	writeFileT(t, filepath.Join(dir, "manifest.json"), []byte("{}"))
	writeFileT(t, filepath.Join(dir, "plan.json"), []byte("{}"))
	w := doArtifact(t, defaultArgs(), "dir="+dir+"&name=source.md", "")
	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404 (absent optional source.md, not 500; body %q)", w.Code, w.Body.String())
	}
	e := decodeErr(t, w)
	if e.Reason != reasonSourceNotFound {
		t.Fatalf("reason = %q, want source_not_found (NOT internal)", e.Reason)
	}
	if e.Reason == reasonInternal {
		t.Fatalf("absent source.md must not surface as internal/500")
	}
}

func TestArtifact_AbsentDir(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "does-not-exist")
	w := doArtifact(t, defaultArgs(), "dir="+dir+"&name=manifest.json", "")
	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404 (body %q)", w.Code, w.Body.String())
	}
	if e := decodeErr(t, w); e.Reason != reasonSourceNotFound {
		t.Fatalf("reason = %q, want source_not_found", e.Reason)
	}
}

// TestArtifact_CORS — OPTIONS preflight returns the pinned origin and allows
// GET; a GET never echoes a foreign Origin (the configured literal is pinned).
func TestArtifact_CORS(t *testing.T) {
	args := defaultArgs()
	dir, _, _ := seedArtifactDir(t)

	// OPTIONS preflight.
	mux := newMux(args)
	pre := httptest.NewRequest(http.MethodOptions, "/artifact?dir="+dir+"&name=audio.wav", nil)
	pre.Header.Set("Origin", "http://evil.example")
	pw := httptest.NewRecorder()
	mux.ServeHTTP(pw, pre)
	if pw.Code != http.StatusNoContent {
		t.Fatalf("preflight status = %d, want 204", pw.Code)
	}
	if got := pw.Header().Get("Access-Control-Allow-Origin"); got != args.CORSOrigin {
		t.Fatalf("preflight allow-origin = %q, want pinned %q (never the foreign Origin)", got, args.CORSOrigin)
	}
	if methods := pw.Header().Get("Access-Control-Allow-Methods"); !strings.Contains(methods, "GET") {
		t.Fatalf("preflight allow-methods = %q, want it to include GET", methods)
	}

	// GET never echoes the foreign Origin.
	w := doArtifact(t, args, "dir="+dir+"&name=audio.wav", "http://evil.example")
	if got := w.Header().Get("Access-Control-Allow-Origin"); got != args.CORSOrigin {
		t.Fatalf("GET allow-origin = %q, want pinned %q (foreign Origin must never be echoed)", got, args.CORSOrigin)
	}
}

// TestArtifact_ReasonsFromClosedEnum asserts every error reason this route emits
// is drawn ONLY from the closed enum — no bad_request / not_found tokens.
func TestArtifact_ReasonsFromClosedEnum(t *testing.T) {
	dir, _, _ := seedArtifactDir(t)
	args := defaultArgs()
	closed := map[string]bool{
		reasonInvalidLevel: true, reasonMissingField: true, reasonMethodNotAllowed: true,
		reasonSourceNotFound: true, reasonStalePatch: true, reasonContentHashMismatch: true,
		reasonUnknownBlock: true, reasonContainerMismatch: true, reasonFormatMismatch: true,
		reasonCancelled: true, reasonReadbackFailed: true, reasonInternal: true,
	}
	queries := []string{
		"name=audio.wav",                      // missing dir
		"dir=" + dir,                          // missing name
		"dir=" + dir + "&name=index.html",     // not allowlisted
		"dir=" + dir + "/nope&name=audio.wav", // absent
	}
	for _, q := range queries {
		w := doArtifact(t, args, q, "")
		e := decodeErr(t, w)
		if !closed[e.Reason] {
			t.Fatalf("query %q produced reason %q not in the closed enum", q, e.Reason)
		}
	}
}

// TestArtifact_ReadDuringWrite proves the GET takes the SAME per-dir mutex the
// writer holds: while a "writer" holds the lock, a concurrent GET blocks until
// the writer releases, so a reader never observes a torn cross-file state.
func TestArtifact_ReadDuringWrite(t *testing.T) {
	dir, _, manifestBytes := seedArtifactDir(t)
	args := defaultArgs()

	absDir, err := filepath.Abs(dir)
	if err != nil {
		t.Fatal(err)
	}
	muIface, _ := dirLocks.LoadOrStore(absDir, &sync.Mutex{})
	mu := muIface.(*sync.Mutex)

	// Simulate a writer holding the per-dir lock across a (rename-like) mutation.
	mu.Lock()
	getDone := make(chan *httptest.ResponseRecorder, 1)

	go func() {
		// This GET must block on the per-dir lock until the writer releases.
		getDone <- doArtifact(t, args, "dir="+dir+"&name=manifest.json", "")
	}()

	// Give the goroutine time to reach the lock acquisition. The GET must NOT
	// have completed yet (it is blocked on mu).
	select {
	case <-getDone:
		mu.Unlock()
		t.Fatal("GET completed while writer held the per-dir lock — read-side lock is missing")
	case <-time.After(50 * time.Millisecond):
		// expected: still blocked
	}

	mu.Unlock()

	w := <-getDone
	if w.Code != http.StatusOK {
		t.Fatalf("post-release GET status = %d, want 200", w.Code)
	}
	if w.Body.String() != string(manifestBytes) {
		t.Fatalf("post-release body = %q, want %q", w.Body.String(), manifestBytes)
	}
}
