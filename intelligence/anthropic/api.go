package anthropic

// This file carries the Anthropic Messages API wire types and the
// transport-shaped constants. Kept narrow on purpose — only the fields
// Voice() actually reads or writes. New fields are added as the adapter
// grows (e.g. tool_use, prompt-caching headers); unknown fields in the
// response are ignored by encoding/json.

// apiEndpoint is the Messages API URL. Single endpoint for synchronous
// calls; the batch API is explicitly out of scope (planner-task.md).
const apiEndpoint = "https://api.anthropic.com/v1/messages"

// anthropicVersion is the API version header value. Anthropic requires
// this header on every request. Pinned here so the contract is visible
// in one place; bumped deliberately when API behavior changes warrant.
const anthropicVersion = "2023-06-01"

// messagesRequest is the POST body for /v1/messages. The shape mirrors
// the public API: model, max_tokens, optional temperature, optional
// system, and a list of role/content messages. The Anthropic API
// allows max_tokens of any positive integer; the adapter passes
// a.maxTokens[req.Level] directly.
type messagesRequest struct {
	Model       string         `json:"model"`
	MaxTokens   int            `json:"max_tokens"`
	Temperature float64        `json:"temperature"`
	System      string         `json:"system,omitempty"`
	Messages    []messageInput `json:"messages"`
}

// messageInput is one role/content turn in messagesRequest.Messages.
// The Messages API accepts richer content shapes (image blocks, tool
// inputs); we only emit plain text here — the planner's narration use
// case is text-in, text-out.
type messageInput struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// messagesResponse is the decoded body for a 2xx /v1/messages reply.
// The API returns more fields (usage, stop_sequence, id, type); the
// adapter only needs Model, Content, and StopReason. Unknown fields
// decode-silent per encoding/json default — additive forward-compat.
type messagesResponse struct {
	Model      string         `json:"model"`
	Content    []contentBlock `json:"content"`
	StopReason string         `json:"stop_reason"`
}

// contentBlock is one element of messagesResponse.Content. Anthropic
// returns a list because a single response can interleave text and
// tool_use blocks; we read the first text block (see firstTextBlock).
type contentBlock struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// errorResponse is the decoded body for a non-2xx /v1/messages reply.
// Anthropic returns a nested {type, error: {type, message}} object;
// we capture the message for the wrapped Go error in Voice(). When the
// body fails to decode against this shape, Voice() falls back to a
// truncated raw-body excerpt — the goal is a useful error string, not
// machine-readable error classification (the pipeline upstream owns
// retry-vs-fail classification via the existing classifier).
type errorResponse struct {
	Type  string `json:"type"`
	Error struct {
		Type    string `json:"type"`
		Message string `json:"message"`
	} `json:"error"`
}

// firstTextBlock returns the .Text of the first content block whose
// Type is "text", or ("", false) when no text block is present.
// Anthropic guarantees ordering, so "first text block" is the
// deterministic pick. tool_use / tool_result blocks are skipped here
// because the adapter does not configure tools — their presence in a
// response would mean the API surface changed and we'd rather refuse
// (via the no-text Go error path in Voice()) than guess.
func firstTextBlock(content []contentBlock) (string, bool) {
	for _, b := range content {
		if b.Type == "text" {
			return b.Text, true
		}
	}
	return "", false
}
