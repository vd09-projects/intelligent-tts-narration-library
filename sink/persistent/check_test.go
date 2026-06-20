package persistent

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vd09-projects/intelligent-tts-narration-library/plan"
)

// writePopulatedOutDir runs Consume against the twoBlockPlan fixtures so
// the outDir holds a valid manifest.json + plan.json + audio.wav. Used
// by every TestCheckStale_* case that needs a real persisted manifest.
func writePopulatedOutDir(t *testing.T) (string, plan.NarrationPlan) {
	t.Helper()
	p, res := twoBlockPlan(t)
	outDir := t.TempDir()
	s := New(outDir, WithVoice("af_bella"))
	if _, err := s.Consume(context.Background(), p, res); err != nil {
		t.Fatalf("Consume: %v", err)
	}
	return outDir, p
}

func TestCheckStale_NoManifest(t *testing.T) {
	t.Parallel()
	dir := t.TempDir() // empty
	p := plan.NarrationPlan{Source: plan.SourceRef{ContentHash: "anything"}}
	stale, reason, err := CheckStale(dir, p)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if stale {
		t.Errorf("absent manifest: got stale=true (want false — no prior manifest is not staleness)")
	}
	if reason != "" {
		t.Errorf("reason should be empty for no-manifest: %q", reason)
	}
}

func TestCheckStale_ContentHashMatch(t *testing.T) {
	dir, p := writePopulatedOutDir(t)
	stale, reason, err := CheckStale(dir, p)
	if err != nil {
		t.Fatal(err)
	}
	if stale {
		t.Errorf("identical plan: got stale=true (want false); reason=%q", reason)
	}
}

func TestCheckStale_ContentHashMismatch(t *testing.T) {
	dir, p := writePopulatedOutDir(t)
	mutated := p
	mutated.Source.ContentHash = "sha256-fixture-bbb-mutated"
	stale, reason, err := CheckStale(dir, mutated)
	if err != nil {
		t.Fatal(err)
	}
	if !stale {
		t.Errorf("mutated content_hash: got stale=false (want true)")
	}
	if !strings.Contains(reason, "content_hash") {
		t.Errorf("reason should mention content_hash: %q", reason)
	}
}

func TestCheckStale_SourceURIMismatch(t *testing.T) {
	dir, p := writePopulatedOutDir(t)
	mutated := p
	mutated.Source.URI = "/tmp/different-file.md"
	stale, reason, err := CheckStale(dir, mutated)
	if err != nil {
		t.Fatal(err)
	}
	if !stale {
		t.Errorf("mutated source URI: got stale=false (want true)")
	}
	if !strings.Contains(reason, "source path") {
		t.Errorf("reason should mention source path: %q", reason)
	}
}

func TestCheckStale_SourceKindMismatch(t *testing.T) {
	dir, p := writePopulatedOutDir(t)
	mutated := p
	mutated.Source.Kind = plan.SourceKindMCPText
	stale, reason, err := CheckStale(dir, mutated)
	if err != nil {
		t.Fatal(err)
	}
	if !stale {
		t.Errorf("mutated source kind: got stale=false (want true)")
	}
	if !strings.Contains(reason, "source path") {
		t.Errorf("reason should mention source path: %q", reason)
	}
}

func TestCheckStale_CorruptManifest(t *testing.T) {
	// Corrupt JSON → err != nil, stale=false. The honesty rule says we do
	// NOT claim staleness when we can't read the manifest — corruption is
	// "we cannot tell", not "definitely stale".
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, manifestFilename), []byte("{not valid json"), 0o644); err != nil {
		t.Fatal(err)
	}
	p := plan.NarrationPlan{Source: plan.SourceRef{ContentHash: "whatever"}}
	stale, reason, err := CheckStale(dir, p)
	if err == nil {
		t.Fatal("expected error for corrupt manifest, got nil")
	}
	if errors.Is(err, os.ErrNotExist) {
		t.Errorf("corrupt manifest err should NOT be os.ErrNotExist; got %v", err)
	}
	if stale {
		t.Errorf("corrupt manifest: got stale=true (want false — cannot tell)")
	}
	if reason != "" {
		t.Errorf("corrupt manifest: reason should be empty: %q", reason)
	}
}

func TestCheckStale_EmptyContentHashInManifest(t *testing.T) {
	// S3 (review v1): write a manifest with content_hash = "" directly and
	// confirm CheckStale flags it as stale + reason mentions the missing
	// content_hash.
	dir := t.TempDir()
	m := Manifest{
		SchemaVersion:     ManifestSchemaVersion,
		PlanSchemaVersion: plan.SchemaVersion,
		SourceKind:        plan.SourceKindFile,
		SourceURI:         "/tmp/sample.md",
		ContentHash:       "", // the load-bearing condition
	}
	if err := writeManifest(filepath.Join(dir, manifestFilename), m); err != nil {
		t.Fatal(err)
	}
	p := plan.NarrationPlan{Source: plan.SourceRef{Kind: plan.SourceKindFile, URI: "/tmp/sample.md", ContentHash: "sha256-aaa"}}
	stale, reason, err := CheckStale(dir, p)
	if err != nil {
		t.Fatal(err)
	}
	if !stale {
		t.Error("empty content_hash in manifest: got stale=false (want true per S3)")
	}
	if !strings.Contains(reason, "missing content_hash") {
		t.Errorf("reason should mention 'missing content_hash': %q", reason)
	}
}
