package file

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/vd09-projects/intelligent-tts-narration-library/adapter"
	"github.com/vd09-projects/intelligent-tts-narration-library/plan"
)

// update — regenerate the golden fixture (testdata/sample.want.json)
// from the live adapter output. Run: `go test ./adapter/file -update`.
// The URI is zeroed before writing because absolute paths are
// machine-dependent.
var update = flag.Bool("update", false, "regenerate golden fixture")

// goldenFixturePath — single source of truth for the want.json path.
const goldenFixturePath = "testdata/sample.want.json"

func TestRead_EmptyFile(t *testing.T) {
	t.Parallel()
	path := writeTempFile(t, nil)
	got, err := New().Read(context.Background(), plan.SourceRef{
		Kind: plan.SourceKindFile,
		URI:  path,
	})
	if err != nil {
		t.Fatalf("Read empty: %v", err)
	}
	if len(got.Bytes) != 0 {
		t.Errorf("Bytes: want empty, got %d bytes", len(got.Bytes))
	}
	if len(got.OffsetMap) != 0 {
		t.Errorf("OffsetMap: want empty for empty file, got %d spans", len(got.OffsetMap))
	}
	// Empty file still has a deterministic sha256 (the empty-string hash).
	const emptySHA256 = "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"
	if got.Source.ContentHash != emptySHA256 {
		t.Errorf("ContentHash: got %q want %q", got.Source.ContentHash, emptySHA256)
	}
	if got.Source.Adapter != Version {
		t.Errorf("Adapter: got %q want %q", got.Source.Adapter, Version)
	}
}

func TestRead_SinglePartialLine(t *testing.T) {
	// One line, no trailing newline → final partial-line span.
	t.Parallel()
	const body = "hello world"
	path := writeTempFile(t, []byte(body))
	got, err := New().Read(context.Background(), plan.SourceRef{
		Kind: plan.SourceKindFile,
		URI:  path,
	})
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if string(got.Bytes) != body {
		t.Errorf("Bytes: got %q want %q", got.Bytes, body)
	}
	wantMap := []adapter.OffsetSpan{
		{
			StartByte: 0,
			EndByte:   len(body),
			Origin: plan.SourceMap{
				Kind:      plan.SourceKindLineRange,
				StartLine: 1,
				EndLine:   1,
			},
		},
	}
	if !reflect.DeepEqual(got.OffsetMap, wantMap) {
		t.Errorf("OffsetMap mismatch:\n got %+v\nwant %+v", got.OffsetMap, wantMap)
	}
}

func TestRead_CRLFKeepsCarriageReturn(t *testing.T) {
	// CRLF spans include the \r — line-ending normalization is not this
	// adapter's job (CLAUDE.md: the planner sees what the file holds).
	t.Parallel()
	body := []byte("a\r\nb\r\n")
	path := writeTempFile(t, body)
	got, err := New().Read(context.Background(), plan.SourceRef{
		Kind: plan.SourceKindFile,
		URI:  path,
	})
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if len(got.OffsetMap) != 2 {
		t.Fatalf("want 2 spans, got %d", len(got.OffsetMap))
	}
	// Span 1: "a\r\n" → bytes [0,3). The \r is inside the span.
	if got.OffsetMap[0].StartByte != 0 || got.OffsetMap[0].EndByte != 3 {
		t.Errorf("span 1 offsets: got [%d,%d) want [0,3)",
			got.OffsetMap[0].StartByte, got.OffsetMap[0].EndByte)
	}
	if string(got.Bytes[got.OffsetMap[0].StartByte:got.OffsetMap[0].EndByte]) != "a\r\n" {
		t.Errorf("span 1 content: %q (CRLF not preserved)",
			got.Bytes[got.OffsetMap[0].StartByte:got.OffsetMap[0].EndByte])
	}
}

func TestRead_MultiBlockMarkdownGolden(t *testing.T) {
	// Golden fixture — testdata/sample.md → testdata/sample.want.json.
	// Source.URI is machine-dependent and zeroed before compare/write.
	t.Parallel()
	srcPath, err := filepath.Abs("testdata/sample.md")
	if err != nil {
		t.Fatalf("abs sample.md: %v", err)
	}

	got, err := New().Read(context.Background(), plan.SourceRef{
		Kind: plan.SourceKindFile,
		URI:  "testdata/sample.md",
	})
	if err != nil {
		t.Fatalf("Read sample.md: %v", err)
	}

	// Live URI is asserted directly — it should be the abs path of the
	// fixture, regardless of cwd.
	if got.Source.URI != srcPath {
		t.Errorf("live Source.URI: got %q want %q", got.Source.URI, srcPath)
	}

	// Zero URI on a copy so the golden compare is host-independent.
	gotForGolden := got
	gotForGolden.Source.URI = ""

	gotJSON, err := json.MarshalIndent(gotForGolden, "", "  ")
	if err != nil {
		t.Fatalf("marshal got: %v", err)
	}
	gotJSON = append(gotJSON, '\n')

	if *update {
		if err := os.WriteFile(goldenFixturePath, gotJSON, 0o644); err != nil {
			t.Fatalf("write golden: %v", err)
		}
		t.Logf("updated %s", goldenFixturePath)
		return
	}

	wantJSON, err := os.ReadFile(goldenFixturePath)
	if err != nil {
		t.Fatalf("read golden (run with -update to seed): %v", err)
	}
	if string(gotJSON) != string(wantJSON) {
		t.Errorf("golden mismatch — run `go test ./adapter/file -update` to inspect drift.\n"+
			"got (%d bytes):\n%s\nwant (%d bytes):\n%s",
			len(gotJSON), gotJSON, len(wantJSON), wantJSON)
	}
}

func TestRead_FileNotFound(t *testing.T) {
	t.Parallel()
	_, err := New().Read(context.Background(), plan.SourceRef{
		Kind: plan.SourceKindFile,
		URI:  filepath.Join(t.TempDir(), "does-not-exist.md"),
	})
	if err == nil {
		t.Fatal("expected error for missing file")
	}
	if !errors.Is(err, os.ErrNotExist) {
		t.Errorf("expected errors.Is(err, os.ErrNotExist); got %v", err)
	}
	if !strings.HasPrefix(err.Error(), "file adapter: ") {
		t.Errorf("expected wrapped prefix; got %q", err.Error())
	}
}

func TestRead_PermissionDenied(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("chmod semantics differ on windows")
	}
	if os.Geteuid() == 0 {
		t.Skip("running as root — chmod 000 does not deny root")
	}
	t.Parallel()

	path := writeTempFile(t, []byte("secret"))
	if err := os.Chmod(path, 0o000); err != nil {
		t.Fatalf("chmod 000: %v", err)
	}
	// Restore so t.TempDir cleanup works.
	t.Cleanup(func() { _ = os.Chmod(path, 0o600) })

	_, err := New().Read(context.Background(), plan.SourceRef{
		Kind: plan.SourceKindFile,
		URI:  path,
	})
	if err == nil {
		t.Fatal("expected permission error")
	}
	if !errors.Is(err, os.ErrPermission) {
		t.Errorf("expected errors.Is(err, os.ErrPermission); got %v", err)
	}
}

func TestRead_UnsupportedKind(t *testing.T) {
	t.Parallel()
	_, err := New().Read(context.Background(), plan.SourceRef{
		Kind: plan.SourceKindMCPText,
		URI:  "ignored",
	})
	if err == nil {
		t.Fatal("expected error for non-file SourceKind")
	}
	if !strings.Contains(err.Error(), "unsupported source kind") {
		t.Errorf("error should name the condition; got %q", err.Error())
	}
}

func TestRead_EmptyURI(t *testing.T) {
	t.Parallel()
	_, err := New().Read(context.Background(), plan.SourceRef{
		Kind: plan.SourceKindFile,
		URI:  "",
	})
	if err == nil {
		t.Fatal("expected error for empty URI")
	}
	if !strings.Contains(err.Error(), "empty uri") {
		t.Errorf("error should name the condition; got %q", err.Error())
	}
}

func TestRead_ContextCancelled(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := New().Read(ctx, plan.SourceRef{
		Kind: plan.SourceKindFile,
		URI:  writeTempFile(t, []byte("anything")),
	})
	if err == nil {
		t.Fatal("expected error for cancelled context")
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("expected errors.Is(err, context.Canceled); got %v", err)
	}
}

// writeTempFile — helper. Creates a fresh file in t.TempDir() with the
// given bytes and returns the path. Empty content is allowed.
func writeTempFile(t *testing.T, content []byte) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "input.md")
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatalf("write temp file: %v", err)
	}
	return path
}
