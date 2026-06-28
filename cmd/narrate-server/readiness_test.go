package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
)

// doReadiness drives one GET /readiness through the full mux and returns the
// recorder. Pulled out so every readiness test shares the CORS + routing path.
func doReadiness(t *testing.T, args serverArgs, query string) *httptest.ResponseRecorder {
	t.Helper()
	mux := newMux(args, newRenderStore(t.TempDir()))
	r := httptest.NewRequest(http.MethodGet, "/readiness?"+query, nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, r)
	return w
}

func decodeReadiness(t *testing.T, w *httptest.ResponseRecorder) readinessResponse {
	t.Helper()
	var rr readinessResponse
	if err := json.Unmarshal(w.Body.Bytes(), &rr); err != nil {
		t.Fatalf("decode readinessResponse: %v (body %q)", err, w.Body.String())
	}
	return rr
}

// TestReadiness_TriState table-drives the three on-disk situations the handler
// reports: a complete triple → rendered; a present dir with an incomplete /
// empty triple → rendering; an absent dir → no_out_dir 404.
func TestReadiness_TriState(t *testing.T) {
	args := defaultArgs()

	// rendered: all three leaves present & non-empty.
	rendered := t.TempDir()
	writeFileT(t, filepath.Join(rendered, "plan.json"), []byte(`{"schema_version":"1"}`))
	writeFileT(t, filepath.Join(rendered, "manifest.json"), []byte(`{"schema_version":1,"blocks":[]}`))
	writeFileT(t, filepath.Join(rendered, "audio.wav"), []byte("RIFF....WAVE"))

	// rendering (missing audio.wav).
	missingLeaf := t.TempDir()
	writeFileT(t, filepath.Join(missingLeaf, "plan.json"), []byte(`{"schema_version":"1"}`))
	writeFileT(t, filepath.Join(missingLeaf, "manifest.json"), []byte(`{}`))

	// rendering (audio.wav present but ZERO bytes — torn / mid-write).
	emptyLeaf := t.TempDir()
	writeFileT(t, filepath.Join(emptyLeaf, "plan.json"), []byte(`{"schema_version":"1"}`))
	writeFileT(t, filepath.Join(emptyLeaf, "manifest.json"), []byte(`{}`))
	writeFileT(t, filepath.Join(emptyLeaf, "audio.wav"), []byte{})

	// rendering (empty dir — exists but no leaves yet, e.g. mkdir -p before render).
	emptyDir := t.TempDir()

	absent := filepath.Join(t.TempDir(), "does-not-exist")

	cases := []struct {
		name       string
		dir        string
		wantStatus int
		wantBody   string // readiness status when 200
		wantReason string // error reason when non-200
	}{
		{"rendered", rendered, http.StatusOK, statusRendered, ""},
		{"rendering-missing-leaf", missingLeaf, http.StatusOK, statusRendering, ""},
		{"rendering-empty-leaf", emptyLeaf, http.StatusOK, statusRendering, ""},
		{"rendering-empty-dir", emptyDir, http.StatusOK, statusRendering, ""},
		{"no-out-dir", absent, http.StatusNotFound, "", reasonNoOutDir},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := doReadiness(t, args, "dir="+tc.dir)
			if w.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d (body %q)", w.Code, tc.wantStatus, w.Body.String())
			}
			if tc.wantStatus == http.StatusOK {
				if rr := decodeReadiness(t, w); rr.Status != tc.wantBody {
					t.Fatalf("status body = %q, want %q", rr.Status, tc.wantBody)
				}
				return
			}
			if e := decodeErr(t, w); e.Reason != tc.wantReason {
				t.Fatalf("reason = %q, want %q", e.Reason, tc.wantReason)
			}
		})
	}
}

// TestReadiness_NoOutDirTokenInClosedEnum pins the new token string AND proves
// it is drawn from the closed enum constant (no ad-hoc literal).
func TestReadiness_NoOutDirTokenInClosedEnum(t *testing.T) {
	if reasonNoOutDir != "no_out_dir" {
		t.Fatalf("reasonNoOutDir = %q, want %q (machine-stable wire token)", reasonNoOutDir, "no_out_dir")
	}
	absent := filepath.Join(t.TempDir(), "nope")
	w := doReadiness(t, defaultArgs(), "dir="+absent)
	e := decodeErr(t, w)
	if e.Reason != reasonNoOutDir {
		t.Fatalf("reason = %q, want %q", e.Reason, reasonNoOutDir)
	}
	if e.Message == "" {
		t.Fatalf("no_out_dir error must carry a human-actionable message")
	}
}

// TestReadiness_StatusTokenValues pins the machine-stable 200-body status wire
// values, proving they are sourced from the named consts (not ad-hoc literals) so
// a typo in one would fail the build/test rather than silently drift the contract.
func TestReadiness_StatusTokenValues(t *testing.T) {
	if statusRendered != "rendered" {
		t.Fatalf("statusRendered = %q, want %q (machine-stable wire token)", statusRendered, "rendered")
	}
	if statusRendering != "rendering" {
		t.Fatalf("statusRendering = %q, want %q (machine-stable wire token)", statusRendering, "rendering")
	}
}

// TestReadiness_MissingDirParam — no ?dir= at all is a caller-shape error
// (missing_field), distinct from no_out_dir (a dir was named but is absent).
func TestReadiness_MissingDirParam(t *testing.T) {
	w := doReadiness(t, defaultArgs(), "")
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
	if e := decodeErr(t, w); e.Reason != reasonMissingField {
		t.Fatalf("reason = %q, want missing_field", e.Reason)
	}
}

// TestReadiness_MethodNotAllowed — POST /readiness is rejected from the closed
// enum, matching /healthz + /artifact discipline.
func TestReadiness_MethodNotAllowed(t *testing.T) {
	mux := newMux(defaultArgs(), newRenderStore(t.TempDir()))
	r := httptest.NewRequest(http.MethodPost, "/readiness?dir=/tmp", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, r)
	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want 405", w.Code)
	}
	if e := decodeErr(t, w); e.Reason != reasonMethodNotAllowed {
		t.Fatalf("reason = %q, want method_not_allowed", e.Reason)
	}
}
