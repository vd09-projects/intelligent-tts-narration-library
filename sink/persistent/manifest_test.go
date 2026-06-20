package persistent

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"testing"

	"github.com/vd09-projects/intelligent-tts-narration-library/plan"
	"github.com/vd09-projects/intelligent-tts-narration-library/render"
)

// sampleManifest returns a minimally-populated Manifest used across the
// round-trip + field-order tests. Keeping it as a helper means schema
// additions only need to be added in one place.
func sampleManifest() Manifest {
	return Manifest{
		SchemaVersion:     ManifestSchemaVersion,
		PlanSchemaVersion: plan.SchemaVersion,
		SourceKind:        plan.SourceKindFile,
		SourceURI:         "/tmp/sample.md",
		ContentHash:       "sha256-abc123",
		AudioFormat:       render.DefaultFormat(),
		Voice:             "af_bella",
		Blocks: []ManifestBlock{
			{
				ID:       "b001",
				Class:    plan.ClassProse,
				Level:    plan.L1,
				Status:   plan.StatusVoiced,
				StartMs:  0,
				EndMs:    1000,
				AudioRef: "b001.wav",
			},
			{
				ID:       "b002",
				Class:    plan.ClassCode,
				Level:    plan.L1,
				Status:   plan.StatusDegraded,
				StartMs:  1000,
				EndMs:    2500,
				AudioRef: "b002.wav",
			},
		},
		Stale:       false,
		StaleReason: "",
	}
}

func TestManifest_SchemaVersionIsOne(t *testing.T) {
	// Pins the const so an accidental bump in a refactor surfaces in
	// review. If we ever genuinely need to bump, this test changes
	// alongside the version, and the code-review process catches it.
	if ManifestSchemaVersion != 1 {
		t.Errorf("ManifestSchemaVersion: got %d, want 1", ManifestSchemaVersion)
	}
}

func TestManifest_MarshalUnmarshalRoundTrip(t *testing.T) {
	orig := sampleManifest()
	raw, err := json.MarshalIndent(orig, "", "  ")
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var got Manifest
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	// Field-by-field comparison (struct equality without comparable
	// guarantees because of the slice).
	if got.SchemaVersion != orig.SchemaVersion {
		t.Errorf("SchemaVersion: got %d want %d", got.SchemaVersion, orig.SchemaVersion)
	}
	if got.PlanSchemaVersion != orig.PlanSchemaVersion {
		t.Errorf("PlanSchemaVersion mismatch")
	}
	if got.SourceKind != orig.SourceKind {
		t.Errorf("SourceKind mismatch")
	}
	if got.SourceURI != orig.SourceURI {
		t.Errorf("SourceURI mismatch")
	}
	if got.ContentHash != orig.ContentHash {
		t.Errorf("ContentHash mismatch")
	}
	if got.AudioFormat != orig.AudioFormat {
		t.Errorf("AudioFormat mismatch")
	}
	if got.Voice != orig.Voice {
		t.Errorf("Voice mismatch")
	}
	if len(got.Blocks) != len(orig.Blocks) {
		t.Fatalf("Blocks len: got %d want %d", len(got.Blocks), len(orig.Blocks))
	}
	for i := range orig.Blocks {
		if got.Blocks[i] != orig.Blocks[i] {
			t.Errorf("Blocks[%d]: got %+v want %+v", i, got.Blocks[i], orig.Blocks[i])
		}
	}
	if got.Stale != orig.Stale {
		t.Errorf("Stale mismatch")
	}
	if got.StaleReason != orig.StaleReason {
		t.Errorf("StaleReason mismatch")
	}
}

func TestManifest_FieldOrderStable(t *testing.T) {
	// Regex check: the schema-version field must precede content_hash,
	// which must precede blocks. Catches accidental reorderings — if
	// fields move around, the byte-identical idempotent-rewrite AC
	// breaks silently.
	raw, err := json.MarshalIndent(sampleManifest(), "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	s := string(raw)
	re := regexp.MustCompile(`(?s)"schema_version":.*"content_hash":.*"blocks":`)
	if !re.MatchString(s) {
		t.Errorf("field order regression: schema_version → content_hash → blocks not found in:\n%s", s)
	}
}

func TestManifest_StaleReasonOmittedWhenEmpty(t *testing.T) {
	// stale_reason has omitempty — the JSON output for a fresh write
	// should not include the key.
	raw, err := json.Marshal(sampleManifest())
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(raw, []byte(`"stale_reason"`)) {
		t.Errorf("expected stale_reason omitted, got: %s", raw)
	}
}

func TestWriteManifest_AtomicTmpRename(t *testing.T) {
	// Happy path: write to a tmp dir, confirm manifest.json exists with
	// the right content, and no .tmp file leftover.
	dir := t.TempDir()
	path := filepath.Join(dir, "manifest.json")
	if err := writeManifest(path, sampleManifest()); err != nil {
		t.Fatalf("writeManifest: %v", err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	// Should round-trip back to the original.
	var got Manifest
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("unmarshal written file: %v", err)
	}
	if got.ContentHash != sampleManifest().ContentHash {
		t.Errorf("round-trip mismatch")
	}
	// No tmp leftover.
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		name := e.Name()
		if name == "manifest.json" {
			continue
		}
		t.Errorf("unexpected leftover file: %s", name)
	}
}

func TestWriteManifest_TrailingNewline(t *testing.T) {
	// Convention: manifest.json ends in a newline (cat-friendly).
	dir := t.TempDir()
	path := filepath.Join(dir, "manifest.json")
	if err := writeManifest(path, sampleManifest()); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(raw) == 0 || raw[len(raw)-1] != '\n' {
		t.Errorf("expected trailing newline; last bytes: %q", string(raw[max(0, len(raw)-10):]))
	}
}

func TestWriteManifest_Idempotent(t *testing.T) {
	// Decision v1.2.0 — no build timestamps. Same input twice → byte-equal.
	dir := t.TempDir()
	path := filepath.Join(dir, "manifest.json")
	if err := writeManifest(path, sampleManifest()); err != nil {
		t.Fatal(err)
	}
	first, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := writeManifest(path, sampleManifest()); err != nil {
		t.Fatal(err)
	}
	second, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(first, second) {
		t.Errorf("non-deterministic writeManifest: lengths %d vs %d", len(first), len(second))
	}
}

func TestReadManifest_MissingFile(t *testing.T) {
	_, err := readManifest("/nonexistent/manifest.json")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, os.ErrNotExist) {
		t.Errorf("expected os.ErrNotExist (wrapped), got %v", err)
	}
}

func TestReadManifest_CorruptJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "manifest.json")
	if err := os.WriteFile(path, []byte("{not valid json"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := readManifest(path)
	if err == nil {
		t.Fatal("expected error for corrupt JSON, got nil")
	}
	// Should NOT be os.ErrNotExist — it's a parse error.
	if errors.Is(err, os.ErrNotExist) {
		t.Errorf("expected parse error, got ErrNotExist: %v", err)
	}
}

func TestReadManifest_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "manifest.json")
	orig := sampleManifest()
	if err := writeManifest(path, orig); err != nil {
		t.Fatal(err)
	}
	got, err := readManifest(path)
	if err != nil {
		t.Fatal(err)
	}
	if got.ContentHash != orig.ContentHash || got.SourceURI != orig.SourceURI {
		t.Errorf("round-trip mismatch via disk")
	}
	if len(got.Blocks) != len(orig.Blocks) {
		t.Errorf("Blocks len mismatch")
	}
}

func TestAtomicWriteFile_NoTmpLeftoverOnSuccess(t *testing.T) {
	// Explicit guard against partial-state-on-disk: a successful write
	// leaves only the target file, no .tmp sibling.
	dir := t.TempDir()
	path := filepath.Join(dir, "target.txt")
	if err := atomicWriteFile(path, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 {
		t.Errorf("expected 1 file, got %d: %v", len(entries), entries)
	}
}

func TestFilepathDir(t *testing.T) {
	cases := map[string]string{
		"/tmp/a.txt":          "/tmp",
		"a.txt":               ".",
		"/a.txt":              "/",
		"./relative/path.txt": "./relative",
	}
	for in, want := range cases {
		if got := filepathDir(in); got != want {
			t.Errorf("filepathDir(%q) = %q, want %q", in, got, want)
		}
	}
}
