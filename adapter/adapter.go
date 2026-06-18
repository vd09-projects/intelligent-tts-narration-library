// Package adapter defines the InputAdapter interface — the seam between
// raw bytes on disk / over a wire / from a screenshot and the planner's
// view of a document. Concrete adapters live in subdirs (file, mcptext,
// ocr) and stay invisible to planner/.
//
// Per the project invariant (CLAUDE.md): this package imports only the
// stdlib and plan/. No I/O. No concrete adapter is referenced here.
package adapter

import (
	"context"

	"github.com/vd09-projects/intelligent-tts-narration-library/plan"
)

// InputAdapter — produces a RawDocument from a plan.SourceRef. The pipeline
// calls Read exactly once per document; cancellation flows through ctx.
//
// Errors are returned (and stop the pipeline). Refusals belong in the plan,
// not at this seam — the adapter does not classify, it only reads bytes
// and reports the byte→line offset map.
type InputAdapter interface {
	Read(ctx context.Context, ref plan.SourceRef) (RawDocument, error)
}

// RawDocument — what the planner receives. Bytes plus an explicit
// byte→line offset map so segmentation and SourceMap construction can
// be done without re-walking the file.
//
// Source is the doc-level provenance the adapter populates: Kind, URI,
// ContentHash (sha256 hex), and Adapter (the "name@version" of the
// concrete adapter that read it).
//
// RawDocument and OffsetSpan are in-process types passed adapter →
// planner. They are never marshaled to plan.json or sent over MCP; the
// narration plan is the only on-wire contract (CLAUDE.md).
type RawDocument struct {
	Source    plan.SourceRef
	Bytes     []byte
	OffsetMap []OffsetSpan
}

// OffsetSpan — one byte span per source line. StartByte / EndByte are
// byte offsets into RawDocument.Bytes; EndByte is exclusive. Names
// reflect semantics — these are raw byte indices, not rune indices; a
// UTF-8 multibyte rune occupies multiple byte positions within a span.
// Origin is a fully-populated plan.SourceMap for that line so block-
// level SourceMaps can be composed by joining adjacent spans.
//
// CRLF: the trailing \r is part of the span (we don't strip line endings
// here — that's a planner concern). The final line of a file with no
// trailing newline is emitted as a partial-line span.
type OffsetSpan struct {
	StartByte int
	EndByte   int
	Origin    plan.SourceMap
}
