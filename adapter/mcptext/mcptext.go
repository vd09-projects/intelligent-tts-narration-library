// Package mcptext implements the in-memory MCP-text InputAdapter — takes a
// caller-supplied string at construction time, hashes the bytes, and emits
// a per-line byte→line offset map matching the file adapter's shape.
//
// Scope is intentionally tiny: no encoding detection, no BOM stripping,
// no line-ending normalization. The planner sees exactly the bytes the
// caller supplied. CRLF lines keep the \r in the span.
//
// Use case: the MCP `speak` tool's `text` arg path — caller passes inline
// markdown, the composition root hashes it, builds the URI
// "mcp://inline/<hex-sha256>", constructs *Adapter via New(text), and the
// planner sees a RawDocument with provenance distinguishable from a file
// read.
//
// Per CLAUDE.md project invariant: this package imports only the stdlib
// plus plan/ + adapter/. No filesystem, no network. Per the adapter
// contract, the adapter does not classify — refusals belong in the
// planner.
//
// Decision: convention — the offset-map line-walking logic is duplicated
// from adapter/file rather than lifted to a shared package. Per #17 plan
// Decision v5: a shared adapterutil package waits until a third adapter
// arrives (mcptext + file = 2; ocr would be the third trigger). The
// duplication is small (~30 lines) and intentional; lifting earlier would
// be speculative abstraction with two consumers.
package mcptext

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"

	"github.com/vd09-projects/intelligent-tts-narration-library/adapter"
	"github.com/vd09-projects/intelligent-tts-narration-library/plan"
)

// Version — adapter identity stamped into plan.SourceRef.Adapter so a
// reader of plan.json can tell which adapter produced the source map.
// Bumped on any change to OffsetMap shape, hashing rule, or URI format.
const Version = "mcptext@0.1.0"

// uriScheme — required prefix for the URI passed to Read. The hex sha256
// of the text follows the scheme; the adapter cross-checks it against the
// computed hash so callers cannot silently mismatch text and provenance.
// Per #17 plan Decision v3: URI hash cross-check stays.
const uriScheme = "mcp://inline/"

// Adapter — in-memory text reader. Carries the text bytes from construction
// time; not safe to mutate the underlying string between construction and
// Read. One Adapter per inline text payload — the composition root builds
// a new instance for each MCP tool call.
type Adapter struct {
	text []byte
}

// Compile-time interface check — fails the build if the interface drifts.
var _ adapter.InputAdapter = (*Adapter)(nil)

// New — constructor. Stores the text bytes by value (slice over string,
// which Go's compiler implements as a single read-only header copy — the
// underlying string memory is shared and immutable). The caller cannot
// invalidate the stored bytes by reassigning the source variable.
func New(text string) *Adapter {
	return &Adapter{text: []byte(text)}
}

// Read — emit a RawDocument with one OffsetSpan per line of the stored
// text, after validating that the URI's hash suffix matches sha256(text).
//
// Error policy: every failure path returns an error wrapped with the
// "mcptext adapter: ..." prefix so callers can grep one string. ctx is
// checked once before producing output; the in-memory path has no
// I/O so there is no later cancellation point.
//
// The URI hash cross-check (Decision v3 of #17 plan) catches a class of
// composition-root bugs: caller computes the URI hash from one string
// then passes a different string to New. Mismatch is a terminal error,
// not a refusal — the bug is in the wiring, not in the document.
func (a *Adapter) Read(ctx context.Context, ref plan.SourceRef) (adapter.RawDocument, error) {
	// Kind gate — this adapter only handles mcp_text. Anything else is a
	// composition-root bug, not a runtime condition, so the error is
	// terminal (no Refusal — that boundary lives in the planner).
	switch ref.Kind {
	case plan.SourceKindMCPText:
		// ok
	default:
		return adapter.RawDocument{}, fmt.Errorf("mcptext adapter: unsupported source kind: %s", ref.Kind)
	}

	if ref.URI == "" {
		return adapter.RawDocument{}, errors.New("mcptext adapter: empty uri")
	}

	if err := ctx.Err(); err != nil {
		return adapter.RawDocument{}, fmt.Errorf("mcptext adapter: %w", err)
	}

	sum := sha256.Sum256(a.text)
	computedHash := hex.EncodeToString(sum[:])

	// URI hash cross-check (Decision v3 of #17 plan). The URI shape is
	// mcp://inline/<hex-sha256>. We accept (and only accept) that exact
	// prefix; any other shape is a wiring error.
	if !strings.HasPrefix(ref.URI, uriScheme) {
		return adapter.RawDocument{}, fmt.Errorf("mcptext adapter: uri must start with %s: uri=%s", uriScheme, ref.URI)
	}
	uriHash := strings.TrimPrefix(ref.URI, uriScheme)
	if uriHash != computedHash {
		return adapter.RawDocument{}, fmt.Errorf("mcptext adapter: uri hash mismatch: uri=%s computed=%s", ref.URI, computedHash)
	}

	return adapter.RawDocument{
		Source: plan.SourceRef{
			Kind:        plan.SourceKindMCPText,
			URI:         ref.URI,
			ContentHash: computedHash,
			Adapter:     Version,
		},
		Bytes:     a.text,
		OffsetMap: buildOffsetMap(a.text),
	}, nil
}

// buildOffsetMap — walk the byte slice splitting on '\n'. Each line
// (including the terminating '\n' if present) becomes one OffsetSpan
// with byte-exclusive end. CRLF: the trailing '\r' stays in the span.
// Empty input → empty slice. Trailing-no-newline → final partial span.
//
// Lines are 1-indexed in the SourceMap (matches the design doc §2.4
// example: start_line=12, end_line=18). The Origin SourceMap on a
// per-line span has start_line == end_line.
//
// Decision: convention — duplicated from adapter/file.buildOffsetMap by
// design. Per #17 plan Decision v5 a shared adapterutil package is
// deferred until a third adapter arrives. See package doc.
func buildOffsetMap(data []byte) []adapter.OffsetSpan {
	if len(data) == 0 {
		return nil
	}

	// Pre-count newlines to size the slice once. One span per '\n',
	// plus one more if the input does not end with '\n'.
	spans := make([]adapter.OffsetSpan, 0, estimateLineCount(data))

	lineNo := 1
	start := 0
	for i := range len(data) {
		if data[i] != '\n' {
			continue
		}
		spans = append(spans, adapter.OffsetSpan{
			StartByte: start,
			EndByte:   i + 1, // exclusive — include the '\n'
			Origin: plan.SourceMap{
				Kind:      plan.SourceKindLineRange,
				StartLine: lineNo,
				EndLine:   lineNo,
			},
		})
		lineNo++
		start = i + 1
	}
	// Trailing partial line — input did not end with '\n'.
	if start < len(data) {
		spans = append(spans, adapter.OffsetSpan{
			StartByte: start,
			EndByte:   len(data),
			Origin: plan.SourceMap{
				Kind:      plan.SourceKindLineRange,
				StartLine: lineNo,
				EndLine:   lineNo,
			},
		})
	}
	return spans
}

// estimateLineCount — count '\n' plus one for a potential trailing
// no-newline line. Used only for slice pre-sizing; correctness of
// buildOffsetMap does not depend on this count being exact.
func estimateLineCount(data []byte) int {
	n := 1
	for _, b := range data {
		if b == '\n' {
			n++
		}
	}
	return n
}

// URIFor — small helper for the composition root: given text, compute the
// canonical URI the adapter expects. Pulled out so callers do not duplicate
// the hash + scheme assembly and so the URI format has one definition.
// Exposed because cmd/narrate-mcp (composition root) must construct the
// URI before constructing the adapter — circular otherwise.
func URIFor(text string) string {
	sum := sha256.Sum256([]byte(text))
	return uriScheme + hex.EncodeToString(sum[:])
}
