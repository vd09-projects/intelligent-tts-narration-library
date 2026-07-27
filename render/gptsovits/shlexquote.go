package gptsovits

import (
	"fmt"
	"strings"
)

// textSplitMethod is the ONLY text_split_method the frozen #161 worker accepts
// (gotcha 7); anything else is rejected as bad-args. Pinned as a named constant so
// the wire token has exactly one origin — the engine injects it, never the caller.
const textSplitMethod = "cut4"

// buildRequest assembles the 5-token, no-<out> request line for the frozen #161
// wire contract, in field order:
//
//	<text> <ref_audio_path> <prompt_text> <text_split_method> <voice_id>
//
// The four free-text/path tokens are POSIX-shell-quoted (posixQuote) so values
// with spaces, quotes, or shell metacharacters round-trip through the worker's
// shlex.split intact; text_split_method is the bare pinned constant. There is NO
// <out> token — the worker mints a content-addressed output path and echoes it in
// `OK <out>` (parsed as line[3:] literal, never shlex-split back).
//
// A token containing a newline or CR is rejected up front (bad-args parity with the
// worker) so the assembled request is always a single physical line and the `OK
// <out>` echo names exactly one path.
func buildRequest(text, refAudioPath, promptText, voiceID string) (string, error) {
	for _, tok := range []struct{ name, val string }{
		{"text", text},
		{"ref_audio_path", refAudioPath},
		{"prompt_text", promptText},
		{"voice_id", voiceID},
	} {
		if strings.ContainsAny(tok.val, "\n\r") {
			return "", fmt.Errorf("%w: <%s> contains a newline/CR — tokens must be single-line", ErrWorkerProtocol, tok.name)
		}
	}
	return strings.Join([]string{
		posixQuote(text),
		posixQuote(refAudioPath),
		posixQuote(promptText),
		textSplitMethod,
		posixQuote(voiceID),
	}, " "), nil
}

// posixQuote single-quotes s for a POSIX shell (shlex), escaping embedded single
// quotes as '\”. Vendored because Go has no stdlib shlex quoter; validated against
// the shared cross-language golden (scripts/testdata/gso-shlex-golden.json).
func posixQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
