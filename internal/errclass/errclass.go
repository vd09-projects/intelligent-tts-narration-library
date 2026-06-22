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
// errclass imports NO concrete backend. It classifies all unrecognised internal
// faults — including adapter sentinels such as mcpsampling's — via the safe
// default arm, so the "only pipeline/ and cmd/ know concrete backends"
// invariant holds here (#58). Sentinel-specific wire text stays at each cmd
// root, which is allowed to know its backends.
package errclass

import (
	"context"
	"errors"
	"io/fs"
	"strconv"
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
	// caller asks (render/sink fault, any unrecognised error). Zero value =
	// safe default.
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
		return "Class(" + strconv.Itoa(int(c)) + ")"
	}
}

// Classify decides the CATEGORY of a pipeline/patch error. It inspects the
// ORIGINAL err (with errors.Is, so it resolves through any existing wrap chain)
// and returns a Class. It does NOT wrap, consume, or re-format the error — the
// caller keeps the original err and applies its own per-root wire mapping.
//
// Precedence (order-significant): cancel > caller (fs) > internal (default).
// The ladder:
//  1. context.Canceled || context.DeadlineExceeded -> ClassCancelled
//  2. fs.ErrNotExist                                -> ClassCaller
//  3. fs.ErrPermission                              -> ClassCaller
//  4. default (incl. nil + any unrecognised fault)  -> ClassInternal
//
// Adapter sentinels (e.g. mcpsampling.Err*) carry no dedicated branch: they are
// unrecognised here and fall to the default ClassInternal arm. Each cmd root
// re-checks them for wire text within its ClassInternal handling — errclass
// stays free of concrete backends (#58).
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
	default:
		return ClassInternal
	}
}
