// Deterministic cardinal number spell-out for the planner voicing pass.
//
// Numbers in narrated prose would otherwise reach the Kokoro subprocess as
// bare digit runs and be read digit-by-digit ("24700" -> "two four seven zero
// zero"). This file turns plain standalone integers and decimals in PROSE into
// spoken cardinals ("twenty-four thousand seven hundred"; "3.5" -> "three
// point five") as a faithful, value-preserving transform.
//
// Honesty rule: number->words is a faithful reading of the exact value, not a
// summary or invention — no Refusal, no Diagnostic. Pure and deterministic; no
// I/O (planner invariant). English only (Locale "en").
//
// Scope containment: spellNumbersInProse is invoked ONLY from voiceSpelled
// (voice.go), which is called ONLY from degradeProse (degrade.go) — the
// no-intelligence verbatim prose path. Cardinals must never leak into
// code/table/diagram structured-class gists, which keep plain voice().

package planner

import "strings"

// ones covers 0-19 (the irregular teens are not decomposable, so they live in
// one flat table). Index 0 is "zero"; index i is the word for i.
var ones = [20]string{
	"zero", "one", "two", "three", "four", "five", "six", "seven", "eight",
	"nine", "ten", "eleven", "twelve", "thirteen", "fourteen", "fifteen",
	"sixteen", "seventeen", "eighteen", "nineteen",
}

// tens is indexed by the tens digit: tens[2] == "twenty", tens[9] == "ninety".
// Indices 0 and 1 are unused (0-19 come from the ones table) and left empty.
var tens = [10]string{
	"", "", "twenty", "thirty", "forty", "fifty", "sixty", "seventy",
	"eighty", "ninety",
}

// scales names the thousands-group magnitudes, index 0 = units group (no word),
// 1 = "thousand", 2 = "million", and so on to quintillion — enough to name any
// uint64. The prose guard caps integer runs at 6 digits, but spellCardinal is
// unit-tested directly past that guard (1_000_000 and beyond), so the table
// must be correct independent of the guard.
var scales = [7]string{
	"", "thousand", "million", "billion", "trillion", "quadrillion",
	"quintillion",
}

// spellCardinal renders n as English cardinal words with no "and"
// ("one hundred five", not "one hundred and five") and hyphenated compound
// tens ("twenty-four"). 0 -> "zero". All-zero inner groups are elided:
// 1000 -> "one thousand", 1005 -> "one thousand five",
// 1100 -> "one thousand one hundred", 100000 -> "one hundred thousand".
//
// Correct for the full uint64 range; the prose caller guards the input length
// separately, so this function makes no assumptions about magnitude.
func spellCardinal(n uint64) string {
	if n < 20 {
		return ones[n]
	}

	// Break n into base-1000 groups, least significant first.
	var groups []uint64
	for n > 0 {
		groups = append(groups, n%1000)
		n /= 1000
	}

	var parts []string
	// Emit most-significant group first, attaching its scale word.
	for i := len(groups) - 1; i >= 0; i-- {
		g := groups[i]
		if g == 0 {
			continue // all-zero inner group contributes nothing
		}
		parts = append(parts, spellHundreds(g))
		if scales[i] != "" {
			parts = append(parts, scales[i])
		}
	}
	return strings.Join(parts, " ")
}

// spellHundreds renders a value in [1,999] as words. Callers guarantee g != 0.
func spellHundreds(g uint64) string {
	var parts []string
	if g >= 100 {
		parts = append(parts, ones[g/100], "hundred")
		g %= 100
	}
	switch {
	case g == 0:
		// nothing to add
	case g < 20:
		parts = append(parts, ones[g])
	default:
		t := tens[g/10]
		if u := g % 10; u != 0 {
			// Compound tens are a single hyphenated word ("twenty-four").
			parts = append(parts, t+"-"+ones[u])
		} else {
			parts = append(parts, t)
		}
	}
	return strings.Join(parts, " ")
}

// spellNumberToken renders a single already-isolated numeric token as spoken
// words, returning ok=false when tok is not a plain non-negative integer or
// decimal. Contract: on ok=false the caller MUST emit the original token bytes
// verbatim (identity) — never a partial or guessed rendering.
//
// Accepts only ^\d+(\.\d+)?$. Negatives are deliberately rejected (a leading
// "-" is sign-vs-hyphen ambiguous, so "-5" is left as digits upstream).
//
// Decimals split on the single interior ".": the integer part via
// spellCardinal, then "point", then each fractional digit read individually
// L-to-R via the ones table (no normalization) — so 3.0 -> "three point zero",
// 0.05 -> "zero point zero five", 10.50 -> "ten point five zero".
func spellNumberToken(tok string) (string, bool) {
	if tok == "" {
		return "", false
	}

	intPart := tok
	fracPart := ""
	if dot := strings.IndexByte(tok, '.'); dot >= 0 {
		intPart = tok[:dot]
		fracPart = tok[dot+1:]
		// Exactly one interior dot, digits on both sides.
		if intPart == "" || fracPart == "" ||
			strings.IndexByte(fracPart, '.') >= 0 {
			return "", false
		}
	}

	if !allDigits(intPart) || (fracPart != "" && !allDigits(fracPart)) {
		return "", false
	}

	// parseDigits keeps us off strconv (which would overflow at 20+ digits and
	// force error handling); the prose guard already caps runs at 6 integer
	// digits, and direct spellCardinal tests stay well within uint64.
	n, ok := parseDigits(intPart)
	if !ok {
		return "", false
	}
	out := spellCardinal(n)

	if fracPart == "" {
		return out, true
	}

	var b strings.Builder
	b.WriteString(out)
	b.WriteString(" point")
	for i := 0; i < len(fracPart); i++ {
		b.WriteByte(' ')
		b.WriteString(ones[fracPart[i]-'0'])
	}
	return b.String(), true
}

// allDigits reports whether s is non-empty and all ASCII digits.
func allDigits(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

// parseDigits converts an all-digit string to uint64, returning ok=false on
// overflow. Callers have already validated s is non-empty ASCII digits.
func parseDigits(s string) (uint64, bool) {
	var n uint64
	for i := 0; i < len(s); i++ {
		d := uint64(s[i] - '0')
		// Overflow guard: n*10 + d must not wrap uint64.
		if n > (^uint64(0)-d)/10 {
			return 0, false
		}
		n = n*10 + d
	}
	return n, true
}

// spellNumbersInProse rewrites plain standalone integer/decimal runs in text
// as spoken cardinals, leaving every other byte identical.
//
// PROSE-ONLY, single caller (voiceSpelled from degradeProse); cardinals must
// NOT leak into code/table/diagram gists. Input MUST already be markdown-
// stripped so the guard reads true source adjacency (see voiceSpelled).
//
// Decimal-vs-sentence-dot rule: a "." is a decimal point IFF a digit
// immediately follows; a "." with no following digit is sentence punctuation
// and is left attached to the (spelled) run ("we saw 24700." -> spell 24700,
// keep the ".").
//
// Fallback: when spellNumberToken returns ok=false the ORIGINAL run bytes are
// emitted verbatim (identity) — never a partial or guessed rendering.
//
// A run R (maximal \d + optional single interior ".") is SPELLED iff ALL hold:
//  1. R matches ^\d+(\.\d+)?$ (enforced by spellNumberToken);
//  2. integer digit count is 1-6;
//  3. prev neighbour is start/whitespace/opening "(" "[" `"` — not a letter,
//     "_", ":", "=", "/", ".", "-", "+", digit, or a comma with a digit on its
//     far side (comma-grouped); a ":" or "=" one byte behind a whitespace
//     prev-neighbour is a config key:value assignment and also LEAVES;
//  4. next neighbour is not a letter, "_", ":", "=", "/", digit, or a comma
//     with a digit on its far side; a trailing sentence "." (no digit after)
//     is allowed and preserved;
//  5. spellNumberToken(R) ok=true, else R is emitted verbatim.
//
// Otherwise R is left as digits. When unsure, leave as digits.
func spellNumbersInProse(text string) string {
	if text == "" {
		return text
	}

	var b strings.Builder
	b.Grow(len(text))

	i := 0
	for i < len(text) {
		c := text[i]
		if c < '0' || c > '9' {
			b.WriteByte(c)
			i++
			continue
		}

		// Found the start of a digit run. Extend it maximally, allowing at most
		// one interior "." that is immediately followed by a digit (decimal
		// point). A "." with no digit after is sentence punctuation and ends
		// the run.
		start := i
		sawDot := false
		for i < len(text) {
			ch := text[i]
			switch {
			case ch >= '0' && ch <= '9':
				i++
			case ch == '.' && !sawDot &&
				i+1 < len(text) && text[i+1] >= '0' && text[i+1] <= '9':
				sawDot = true
				i++
			default:
				goto runEnd
			}
		}
	runEnd:
		run := text[start:i]

		prev := byte(0)
		if start > 0 {
			prev = text[start-1]
		}
		next := byte(0)
		if i < len(text) {
			next = text[i]
		}

		if spellableRun(run, prev, next, start, i, text) {
			spoken, ok := spellNumberToken(run)
			if ok {
				b.WriteString(spoken)
				continue
			}
		}
		// Leave as digits (guard rejected OR speller returned ok=false —
		// fallback identity: emit the original run bytes verbatim).
		b.WriteString(run)
	}

	return b.String()
}

// spellableRun applies the guard predicate to an isolated run and its byte
// neighbours. prev/next are 0 at the string edges. runStart/runEnd bound the
// run in text so the comma-grouping check can look one byte past the comma.
func spellableRun(run string, prev, next byte, runStart, runEnd int, text string) bool {
	// (2) integer digit count 1-6. Hex ("0xFF") needs no dedicated prefix guard:
	// the run stops at the non-digit "x", leaving "0" with a letter next
	// neighbour, which the (4) letter check rejects.
	intLen := len(run)
	if dot := strings.IndexByte(run, '.'); dot >= 0 {
		intLen = dot
	}
	if intLen < 1 || intLen > 6 {
		return false
	}

	// Config key:value pattern across a single space ("replicas: 3",
	// "count = 5"): a ":" or "=" one byte behind a whitespace prev-neighbour is
	// still a value assignment, so LEAVE. Without this, the immediate whitespace
	// neighbour would wrongly pass the run through and spell the value.
	if (prev == ' ' || prev == '\t') && runStart >= 2 {
		back := text[runStart-2]
		if back == ':' || back == '=' {
			return false
		}
	}

	// (3) prev neighbour: reject identifier / operator / version / signed /
	// digit / comma-grouped contexts. Signs "-"/"+" force LEAVE (sign vs hyphen
	// ambiguous: "-5 degrees" stays digits, enforced by the guard, not just a
	// test).
	switch {
	case prev == 0:
		// start of string — OK
	case isLetter(prev) || prev == '_' ||
		prev == ':' || prev == '=' || prev == '/' || prev == '.' ||
		prev == '-' || prev == '+' ||
		(prev >= '0' && prev <= '9'):
		return false
	case prev == ',':
		// Comma-grouped (e.g. "24,700"): a digit on the far side of the comma
		// means this run is a thousands group — leave the whole thing as digits.
		// TODO: spell comma-grouped integers ("24,700" -> "twenty-four thousand
		// seven hundred") in spellNumbersInProse; deferred this PR, currently
		// left as digits because the tokenizer breaks on "," and spelling the
		// halves would mis-read them.
		if runStart >= 2 {
			far := text[runStart-2]
			if far >= '0' && far <= '9' {
				return false
			}
		}
	}

	// (4) next neighbour: reject identifier / operator / version-dot / digit /
	// comma-grouped. A trailing sentence "." (no digit after — the run already
	// consumed any decimal point) is allowed and preserved by the caller.
	switch {
	case next == 0:
		// end of string — OK
	case isLetter(next) || next == '_' ||
		next == ':' || next == '=' || next == '/' ||
		(next >= '0' && next <= '9'):
		return false
	case next == ',':
		if runEnd+1 < len(text) {
			far := text[runEnd+1]
			if far >= '0' && far <= '9' {
				return false
			}
		}
	}

	return true
}

// isLetter reports whether b is an ASCII letter. Non-ASCII bytes are not
// letters here — the guard is intentionally conservative and English-only.
func isLetter(b byte) bool {
	return (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z')
}
