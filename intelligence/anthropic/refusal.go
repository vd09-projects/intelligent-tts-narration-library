package anthropic

import (
	"strings"
	"unicode"
)

// refuseSentinel is the literal token the LLM uses to signal refusal.
// Documented in the package doc (anthropic.go) and restated in every
// system prompt (internal/intelligencetmpl). The boundary rule is the
// same as mcpsampling's: the sentinel must be the very first non-
// whitespace characters of the assistant's reply — anywhere else it is
// content, not refusal.
//
// INTENTIONAL DUPLICATE of intelligence/mcpsampling/mcpsampling.go's
// refuseSentinel + parseRefusal. Per the project's session decisions
// (issue #15, Decision v1) we copy rather than lift to a shared
// internal/ package until a 3rd adapter justifies the extraction (rule
// of three). The two implementations must stay in lockstep on the
// sentinel string and the leading-whitespace tolerance — any change
// here needs the same change in mcpsampling.
const refuseSentinel = "__REFUSE__"

// parseRefusal returns (note, true) if text leads with __REFUSE__ after
// any whitespace, else ("", false). Boundary rule per refuseSentinel doc.
//
// INTENTIONAL DUPLICATE — see refuseSentinel doc above.
func parseRefusal(text string) (string, bool) {
	trimmed := strings.TrimLeftFunc(text, unicode.IsSpace)
	if !strings.HasPrefix(trimmed, refuseSentinel) {
		return "", false
	}
	rest := strings.TrimSpace(strings.TrimPrefix(trimmed, refuseSentinel))
	return rest, true
}
