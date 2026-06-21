// Package errclass holds the single, shared pipeline-error classifier used by
// the cmd/ composition roots (cmd/narrate-mcp, cmd/narrate-server). It answers
// exactly one question — which CATEGORY a pipeline/patch error belongs to:
// caller-correctable, internal, or cancellation. It deliberately owns NOTHING
// about wire format: no HTTP status codes, no reason tokens, no text prefixes,
// no error wrapping. Each composition root keeps its own per-root mapping from
// Class to its wire contract (MCP text prefixes + %w wrapping; server HTTP
// status + reason tokens + err.Error() flattening) at the call site.
//
// This consolidates the caller-vs-internal-vs-cancel classification that was
// duplicated across the two roots (the // DUP marker in cmd/narrate-server
// pointed here as the "3rd consumer lands, extract now" trigger, #51).
//
// Dependency note (intentional coupling): this CLASSIFICATION package imports
// intelligence/mcpsampling solely to recognise that package's two adapter
// sentinels (ErrNoSamplingClient, ErrUnexpectedContentKind) and classify them
// as ClassInternal. That is a deliberate, named edge: errclass knows about two
// concrete adapter sentinels so the classification stays in ONE place rather
// than being re-duplicated at the MCP root. Future maintainers: this is the
// latent coupling — errclass -> intelligence/mcpsampling — recorded so the edge
// is understood, not discovered. There is no import cycle: mcpsampling imports
// only plan/, intelligence/, and the MCP SDK; it never reaches into internal/.
package errclass

import (
	"context"
	"errors"
	"io/fs"

	"github.com/vd09-projects/intelligent-tts-narration-library/intelligence/mcpsampling"
)

// Class is the closed category of a pipeline/patch error, as decided by
// Classify. ClassInternal is the iota zero value so a forgotten or
// uninitialised path fails safe to internal — matching both roots' default
// branch (MCP "internal_error: pipeline failure", server 500 reasonInternal).
//
// Deliberate departure from the project enum convention: the typed enums in
// plan/ (the #10/#23 sweep) all carry an IsValid() method because they are
// parsed from wire / deserialized / user-supplied and therefore need input
// validation. Class is NOT one of those: it is a closed INTERNAL return type
// produced only by Classify, never parsed from wire, never deserialized, never
// user-supplied. There is no untrusted input to validate, so IsValid() would be
// dead code — it is intentionally omitted. String() IS provided, purely for
// debuggability and readable test failures; it is the only method.
type Class int

const (
	// ClassInternal: server cannot fulfil the request regardless of how the
	// caller asks (render/sink fault, missing sampling client, non-text
	// sampling reply, any unrecognised error). Zero value = safe default.
	ClassInternal Class = iota
	// ClassCaller: caller could fix it by changing the request (source not
	// found, source permission denied).
	ClassCaller
	// ClassCancelled: context cancellation / deadline — its own bucket.
	ClassCancelled
)

// String renders the Class for debugging and test output. The only method on
// Class (see the type doc for why IsValid() is intentionally absent).
func (c Class) String() string {
	switch c {
	case ClassInternal:
		return "ClassInternal"
	case ClassCaller:
		return "ClassCaller"
	case ClassCancelled:
		return "ClassCancelled"
	default:
		return "Class(" + itoa(int(c)) + ")"
	}
}

// itoa avoids pulling strconv just for the unreachable default String() arm.
func itoa(n int) string {
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

// Classify decides the CATEGORY of a pipeline/patch error. It inspects the
// ORIGINAL err (with errors.Is, so it resolves through any existing wrap chain)
// and returns a Class. It does NOT wrap, consume, or re-format the error — the
// caller keeps the original err and applies its own per-root wire mapping.
//
// Precedence (order-significant): cancel > caller (fs) > internal
// (sampling sentinels + default). The ladder:
//  1. context.Canceled || context.DeadlineExceeded -> ClassCancelled
//  2. fs.ErrNotExist                                -> ClassCaller
//  3. fs.ErrPermission                              -> ClassCaller
//  4. mcpsampling.ErrNoSamplingClient               -> ClassInternal
//  5. mcpsampling.ErrUnexpectedContentKind          -> ClassInternal
//  6. default (incl. nil)                            -> ClassInternal
//
// nil never reaches Classify in production: both roots guard non-nil before
// calling (MCP keeps its nil -> nil guard; the server only classifies a
// non-nil error). For completeness Classify(nil) == ClassInternal — the
// defined-but-unreached behaviour, exercised by the unit test.
func Classify(err error) Class {
	switch {
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		return ClassCancelled
	case errors.Is(err, fs.ErrNotExist), errors.Is(err, fs.ErrPermission):
		return ClassCaller
	case errors.Is(err, mcpsampling.ErrNoSamplingClient),
		errors.Is(err, mcpsampling.ErrUnexpectedContentKind):
		return ClassInternal
	default:
		return ClassInternal
	}
}
