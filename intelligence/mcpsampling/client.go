package mcpsampling

import (
	"context"
	"errors"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// SamplingClient is the narrow seam between the adapter and the MCP
// SDK. The production path is *mcp.ServerSession (its CreateMessage
// method signature matches verbatim); tests pass a fake implementation.
//
// The seam exists because the adapter cannot hold a *mcp.ServerSession
// at construction time — the session is per-call, lives only for the
// duration of one MCP request. Threading the session into ctx via
// WithSamplingClient keeps pipeline.New engine-neutral (it does not
// need to know about MCP sessions). See Decision (v3) in the plan
// artifact for the rejected alternatives.
type SamplingClient interface {
	CreateMessage(ctx context.Context, params *mcp.CreateMessageParams) (*mcp.CreateMessageResult, error)
}

// clientCtxKey is a typed key so context.WithValue / ctx.Value lookups
// cannot collide with any other package's key. Stored as a struct{}
// per the Go convention for context-key types.
type clientCtxKey struct{}

// WithSamplingClient stuffs a SamplingClient into ctx. The speak
// handler in cmd/narrate-mcp (Phase 5) calls this before invoking
// pipeline.Narrate so the per-call *mcp.ServerSession travels with the
// request without crossing layer boundaries. Pure: returns a new
// context, does not mutate the input.
func WithSamplingClient(ctx context.Context, c SamplingClient) context.Context {
	return context.WithValue(ctx, clientCtxKey{}, c)
}

// samplingClientFromCtx is the inverse — returns the SamplingClient or
// nil if absent. Kept unexported so external callers cannot bypass
// WithSamplingClient.
func samplingClientFromCtx(ctx context.Context) SamplingClient {
	c, _ := ctx.Value(clientCtxKey{}).(SamplingClient)
	return c
}

// ErrNoSamplingClient is the sentinel returned by Voice when ctx does
// not carry a SamplingClient. The cmd/narrate-mcp classifier (Phase 5)
// routes this to the "internal_error:" wire bucket — the server failed
// to thread the session, which is an operator bug, not a caller bug.
var ErrNoSamplingClient = errors.New("mcpsampling: no SamplingClient in ctx")

// ErrUnexpectedContentKind is returned when the client's reply is not
// *mcp.TextContent. The MCP spec allows text / image / audio in
// sampling responses; the adapter only knows how to voice text.
// Classifier routes this to "internal_error:" — the client is
// misbehaving from our perspective.
var ErrUnexpectedContentKind = errors.New("mcpsampling: unexpected content kind (want TextContent)")
