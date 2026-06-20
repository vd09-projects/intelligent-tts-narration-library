package mcptext

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/vd09-projects/intelligent-tts-narration-library/adapter"
	"github.com/vd09-projects/intelligent-tts-narration-library/plan"
)

// emptySHA256 — sha256("") in hex. Pinned literal so we catch any drift
// in the canonical empty-string hash (same constant adapter/file pins).
const emptySHA256 = "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"

func TestRead_EmptyText(t *testing.T) {
	t.Parallel()
	got, err := New("").Read(context.Background(), plan.SourceRef{
		Kind: plan.SourceKindMCPText,
		URI:  uriScheme + emptySHA256,
	})
	if err != nil {
		t.Fatalf("Read empty: %v", err)
	}
	if len(got.Bytes) != 0 {
		t.Errorf("Bytes: want empty, got %d bytes", len(got.Bytes))
	}
	if len(got.OffsetMap) != 0 {
		t.Errorf("OffsetMap: want empty for empty text, got %d spans", len(got.OffsetMap))
	}
	if got.Source.ContentHash != emptySHA256 {
		t.Errorf("ContentHash: got %q want %q", got.Source.ContentHash, emptySHA256)
	}
	if got.Source.Adapter != Version {
		t.Errorf("Adapter: got %q want %q", got.Source.Adapter, Version)
	}
	if got.Source.Kind != plan.SourceKindMCPText {
		t.Errorf("Kind: got %q want %q", got.Source.Kind, plan.SourceKindMCPText)
	}
}

func TestRead_SinglePartialLine(t *testing.T) {
	// One line, no trailing newline → final partial-line span. Same shape
	// as adapter/file's TestRead_SinglePartialLine; the parity is
	// load-bearing because the planner relies on identical OffsetMap shape
	// regardless of source.
	t.Parallel()
	const body = "hello world"
	got, err := New(body).Read(context.Background(), plan.SourceRef{
		Kind: plan.SourceKindMCPText,
		URI:  URIFor(body),
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

func TestRead_MultiLineLF(t *testing.T) {
	// Two complete lines + a third partial. Confirms the per-line span
	// boundaries (and line numbering) match adapter/file's behavior for
	// plain LF text.
	t.Parallel()
	const body = "alpha\nbeta\ngamma" // 3 spans: [0,6), [6,11), [11,16)
	got, err := New(body).Read(context.Background(), plan.SourceRef{
		Kind: plan.SourceKindMCPText,
		URI:  URIFor(body),
	})
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	wantMap := []adapter.OffsetSpan{
		{StartByte: 0, EndByte: 6, Origin: plan.SourceMap{Kind: plan.SourceKindLineRange, StartLine: 1, EndLine: 1}},
		{StartByte: 6, EndByte: 11, Origin: plan.SourceMap{Kind: plan.SourceKindLineRange, StartLine: 2, EndLine: 2}},
		{StartByte: 11, EndByte: 16, Origin: plan.SourceMap{Kind: plan.SourceKindLineRange, StartLine: 3, EndLine: 3}},
	}
	if !reflect.DeepEqual(got.OffsetMap, wantMap) {
		t.Errorf("OffsetMap mismatch:\n got %+v\nwant %+v", got.OffsetMap, wantMap)
	}
}

func TestRead_CRLFKeepsCarriageReturn(t *testing.T) {
	// CRLF spans include the \r — line-ending normalization is not this
	// adapter's job (CLAUDE.md: the planner sees what the source holds).
	// Parity with adapter/file.TestRead_CRLFKeepsCarriageReturn.
	t.Parallel()
	const body = "a\r\nb\r\n"
	got, err := New(body).Read(context.Background(), plan.SourceRef{
		Kind: plan.SourceKindMCPText,
		URI:  URIFor(body),
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
	// Span 2: "b\r\n" → bytes [3,6).
	if got.OffsetMap[1].StartByte != 3 || got.OffsetMap[1].EndByte != 6 {
		t.Errorf("span 2 offsets: got [%d,%d) want [3,6)",
			got.OffsetMap[1].StartByte, got.OffsetMap[1].EndByte)
	}
}

func TestRead_MultiLineUTF8(t *testing.T) {
	// Multibyte runes — Greek letters are 2 bytes each in UTF-8. The
	// adapter records byte offsets, not rune offsets; this test pins
	// the byte-arithmetic invariant.
	//
	// "αβγ" = 6 bytes (3 × 2-byte runes). "δε" = 4 bytes. Layout:
	//   "αβγ\nδε\n" → 12 bytes total: 6 + 1 + 4 + 1.
	//   span 1: [0,7) = "αβγ\n"
	//   span 2: [7,12) = "δε\n"
	t.Parallel()
	const body = "αβγ\nδε\n"
	if len(body) != 12 {
		// Sanity: if Go's UTF-8 representation of the literal changes,
		// the test logic is wrong, not the adapter. Catch up front.
		t.Fatalf("test setup: want 12 bytes, got %d (UTF-8 layout drift)", len(body))
	}
	got, err := New(body).Read(context.Background(), plan.SourceRef{
		Kind: plan.SourceKindMCPText,
		URI:  URIFor(body),
	})
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	wantMap := []adapter.OffsetSpan{
		{StartByte: 0, EndByte: 7, Origin: plan.SourceMap{Kind: plan.SourceKindLineRange, StartLine: 1, EndLine: 1}},
		{StartByte: 7, EndByte: 12, Origin: plan.SourceMap{Kind: plan.SourceKindLineRange, StartLine: 2, EndLine: 2}},
	}
	if !reflect.DeepEqual(got.OffsetMap, wantMap) {
		t.Errorf("OffsetMap mismatch:\n got %+v\nwant %+v", got.OffsetMap, wantMap)
	}
}

func TestRead_UnsupportedKind(t *testing.T) {
	// Composition-root bug: caller passed a non-mcp_text SourceKind.
	// Terminal error — no refusal here, per the adapter contract.
	t.Parallel()
	_, err := New("hi").Read(context.Background(), plan.SourceRef{
		Kind: plan.SourceKindFile,
		URI:  URIFor("hi"),
	})
	if err == nil {
		t.Fatal("expected error for non-mcp_text SourceKind")
	}
	if !strings.Contains(err.Error(), "unsupported source kind") {
		t.Errorf("error should name the condition; got %q", err.Error())
	}
	if !strings.HasPrefix(err.Error(), "mcptext adapter: ") {
		t.Errorf("expected wrapped prefix; got %q", err.Error())
	}
}

func TestRead_EmptyURI(t *testing.T) {
	t.Parallel()
	_, err := New("hi").Read(context.Background(), plan.SourceRef{
		Kind: plan.SourceKindMCPText,
		URI:  "",
	})
	if err == nil {
		t.Fatal("expected error for empty URI")
	}
	if !strings.Contains(err.Error(), "empty uri") {
		t.Errorf("error should name the condition; got %q", err.Error())
	}
}

func TestRead_URIHashMismatch(t *testing.T) {
	// Decision v3 of #17 plan: the URI's hash suffix must match
	// sha256(text). A mismatch indicates the composition root wired text
	// and provenance inconsistently — caller bug, terminal error.
	t.Parallel()
	_, err := New("hello").Read(context.Background(), plan.SourceRef{
		Kind: plan.SourceKindMCPText,
		URI:  uriScheme + "deadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeef",
	})
	if err == nil {
		t.Fatal("expected error for URI hash mismatch")
	}
	if !strings.Contains(err.Error(), "uri hash mismatch") {
		t.Errorf("error should name the condition; got %q", err.Error())
	}
}

func TestRead_URIBadScheme(t *testing.T) {
	// Defensive: URI without the mcp://inline/ prefix is also a wiring
	// error. Surface it with a clear message rather than silently
	// computing a mismatched hash.
	t.Parallel()
	_, err := New("hello").Read(context.Background(), plan.SourceRef{
		Kind: plan.SourceKindMCPText,
		URI:  "file:///etc/passwd",
	})
	if err == nil {
		t.Fatal("expected error for URI without mcp://inline/ scheme")
	}
	if !strings.Contains(err.Error(), "uri must start with mcp://inline/") {
		t.Errorf("error should name the scheme; got %q", err.Error())
	}
}

func TestRead_ContextCancelled(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := New("anything").Read(ctx, plan.SourceRef{
		Kind: plan.SourceKindMCPText,
		URI:  URIFor("anything"),
	})
	if err == nil {
		t.Fatal("expected error for cancelled context")
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("expected errors.Is(err, context.Canceled); got %v", err)
	}
}

func TestRead_AdapterVersionStamped(t *testing.T) {
	// Independent check that Source.Adapter equals the package Version
	// constant — provenance discipline. If Version bumps, this test
	// pins the wire format.
	t.Parallel()
	got, err := New("x").Read(context.Background(), plan.SourceRef{
		Kind: plan.SourceKindMCPText,
		URI:  URIFor("x"),
	})
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if got.Source.Adapter != Version {
		t.Errorf("Source.Adapter: got %q want %q", got.Source.Adapter, Version)
	}
}

func TestURIFor_MatchesSha256(t *testing.T) {
	// Pin the URI-construction helper: it must produce the same hash the
	// adapter computes on the same bytes. If these drift, the cross-check
	// would reject every well-formed call.
	t.Parallel()
	const body = "compose me"
	got := URIFor(body)
	sum := sha256.Sum256([]byte(body))
	want := uriScheme + hex.EncodeToString(sum[:])
	if got != want {
		t.Errorf("URIFor: got %q want %q", got, want)
	}
}

// (Snapshot-by-value test removed: Go strings are immutable and
// `[]byte(text)` copies the backing array, so rebinding the caller's
// local cannot affect the stored bytes — there is no observable
// behavior left to assert beyond the type system. Kept this note for
// future readers who might wonder why the documented "stores by value"
// contract has no test.)
