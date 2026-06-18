// Package file implements the file InputAdapter — reads a path off disk,
// hashes the bytes, and emits a per-line byte→line offset map.
//
// Scope is intentionally tiny: no encoding detection, no BOM stripping,
// no line-ending normalization. The planner sees exactly what the file
// holds. CRLF lines keep the \r in the span.
package file

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/vd09-projects/intelligent-tts-narration-library/adapter"
	"github.com/vd09-projects/intelligent-tts-narration-library/plan"
)

// Version — adapter identity stamped into plan.SourceRef.Adapter so a
// reader of plan.json can tell which adapter produced the source map.
// Bumped on any change to OffsetMap shape or hashing rule.
const Version = "file@0.1.0"

// Adapter — stateless file reader. Safe for concurrent use.
type Adapter struct{}

// Compile-time interface check — fails the build if the interface drifts.
var _ adapter.InputAdapter = (*Adapter)(nil)

// New — constructor for symmetry with future adapters that will need config
// (e.g. encoding override). Returns by value; the type carries no state.
func New() *Adapter { return &Adapter{} }

// Read — load the file at ref.URI and emit a RawDocument with one
// OffsetSpan per line.
//
// Error policy: every failure path returns an error wrapped with the
// "file adapter: ..." prefix so callers can grep one string. ctx is
// checked once before I/O; we do not chunk-read large files in phase
// one, so there is no mid-read cancellation point.
func (a *Adapter) Read(ctx context.Context, ref plan.SourceRef) (adapter.RawDocument, error) {
	// Kind gate — this adapter only handles file. Anything else is a
	// composition-root bug, not a runtime condition, so the error is
	// terminal (no Refusal — that boundary lives in the planner).
	switch ref.Kind {
	case plan.SourceKindFile:
		// ok
	default:
		return adapter.RawDocument{}, fmt.Errorf("file adapter: unsupported source kind: %s", ref.Kind)
	}

	if ref.URI == "" {
		return adapter.RawDocument{}, errors.New("file adapter: empty uri")
	}

	if err := ctx.Err(); err != nil {
		return adapter.RawDocument{}, fmt.Errorf("file adapter: %w", err)
	}

	absPath, err := filepath.Abs(ref.URI)
	if err != nil {
		return adapter.RawDocument{}, fmt.Errorf("file adapter: resolve %s: %w", ref.URI, err)
	}

	data, err := os.ReadFile(absPath)
	if err != nil {
		return adapter.RawDocument{}, fmt.Errorf("file adapter: read %s: %w", absPath, err)
	}

	sum := sha256.Sum256(data)
	hash := hex.EncodeToString(sum[:])

	return adapter.RawDocument{
		Source: plan.SourceRef{
			Kind:        plan.SourceKindFile,
			URI:         absPath,
			ContentHash: hash,
			Adapter:     Version,
		},
		Bytes:     data,
		OffsetMap: buildOffsetMap(data),
	}, nil
}

// buildOffsetMap — walk the byte slice splitting on '\n'. Each line
// (including the terminating '\n' if present) becomes one OffsetSpan
// with byte-exclusive end. CRLF: the trailing '\r' stays in the span.
// Empty file → empty slice. Trailing-no-newline → final partial span.
//
// Lines are 1-indexed in the SourceMap (matches the design doc §2.4
// example: start_line=12, end_line=18). The Origin SourceMap on a
// per-line span has start_line == end_line.
func buildOffsetMap(data []byte) []adapter.OffsetSpan {
	if len(data) == 0 {
		return nil
	}

	// Pre-count newlines to size the slice once. One span per '\n',
	// plus one more if the file does not end with '\n'.
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
	// Trailing partial line — file did not end with '\n'.
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
