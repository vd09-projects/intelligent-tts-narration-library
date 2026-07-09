package planner

import (
	"context"
	"strings"
	"testing"

	"github.com/vd09-projects/intelligent-tts-narration-library/plan"
)

// TestPlan_ProseSpellsCardinals_EndToEnd — proves the cardinal-number-spellout
// pass is wired through the REAL Plan() entry via the degradeProse verbatim
// path (nil adapter = no intelligence), not just the voiceSpelled/spellCardinal
// unit helpers. If degradeProse ever reverted from voiceSpelled() to plain
// voice(), the unit tests would still pass but THIS test would fail: it reads
// the emitted prose Segment.Text straight off the returned plan. Prose is kept
// short (well under ~120 words) so it takes the degraded-verbatim path, not a
// refusal. The negatives pin the guard's identity fallback end-to-end: a dotted
// version and a >=7-digit run must survive as digits.
func TestPlan_ProseSpellsCardinals_EndToEnd(t *testing.T) {
	t.Parallel()
	seams := deterministicSeams(t)

	cases := []struct {
		name    string
		src     string
		want    []string // substrings that MUST appear in the prose Segment.Text
		notWant []string // substrings that MUST NOT appear
	}{
		{
			name:    "integer spells",
			src:     "We scaled the cluster to 24700 nodes last week.",
			want:    []string{"twenty-four thousand seven hundred"},
			notWant: []string{"24700"},
		},
		{
			name: "decimal spells",
			src:  "It ran 3.5 times faster.",
			want: []string{"three point five"},
		},
		{
			name:    "identifier negatives stay digits",
			src:     "We deployed v2.4.0 and request 5551234567 failed.",
			want:    []string{"2.4.0", "5551234567"},
			notWant: []string{"five thousand"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := Plan(context.Background(), stubDoc(tc.src, "prose.md"),
				Request{Level: plan.L1}, nil, seams...)
			if err != nil {
				t.Fatalf("Plan: %v", err)
			}
			// Find the degraded-verbatim prose block by class + status rather
			// than a hardcoded index, to stay robust to block ordering.
			var prose *plan.Block
			for i := range got.Blocks {
				if got.Blocks[i].Class == plan.ClassProse && got.Blocks[i].Status == plan.StatusDegraded {
					prose = &got.Blocks[i]
					break
				}
			}
			if prose == nil {
				t.Fatalf("no degraded prose block found: %+v", got.Blocks)
			}
			if len(prose.Segments) == 0 {
				t.Fatalf("prose block has no segments: %+v", prose)
			}
			text := prose.Segments[0].Text
			for _, w := range tc.want {
				if !strings.Contains(text, w) {
					t.Errorf("Segment.Text %q must contain %q", text, w)
				}
			}
			for _, nw := range tc.notWant {
				if strings.Contains(text, nw) {
					t.Errorf("Segment.Text %q must NOT contain %q", text, nw)
				}
			}
		})
	}
}

// TestSpellCardinal_Direct — direct-call matrix over spellCardinal, exercised
// PAST the prose guard's 6-digit cap (1_000_000 and up) so the scale table is
// proven correct independent of the guard. Covers ones/teens/tens boundaries,
// hyphenation, no-"and" rule, and all-zero inner groups.
func TestSpellCardinal_Direct(t *testing.T) {
	t.Parallel()
	cases := []struct {
		n    uint64
		want string
	}{
		{0, "zero"},
		{7, "seven"},
		{13, "thirteen"},
		{20, "twenty"},
		{24, "twenty-four"},
		{100, "one hundred"},
		{105, "one hundred five"},
		{999, "nine hundred ninety-nine"},
		{1000, "one thousand"},
		{1005, "one thousand five"},
		{1100, "one thousand one hundred"},
		{100000, "one hundred thousand"},
		{24700, "twenty-four thousand seven hundred"},
		{123456, "one hundred twenty-three thousand four hundred fifty-six"},
		{1000000, "one million"},
	}
	for _, tc := range cases {
		if got := spellCardinal(tc.n); got != tc.want {
			t.Errorf("spellCardinal(%d) = %q; want %q", tc.n, got, tc.want)
		}
	}
}

// TestSpellNumberToken_Direct — direct-call matrix over spellNumberToken,
// covering integer + decimal readout and the decimal zero-traps (leading /
// trailing / interior zeros read literally, no normalization).
func TestSpellNumberToken_Direct(t *testing.T) {
	t.Parallel()
	cases := []struct {
		tok  string
		want string
	}{
		{"3.5", "three point five"},
		{"3.14", "three point one four"},
		{"3.0", "three point zero"},
		{"0.05", "zero point zero five"},
		{"0.25", "zero point two five"},
		{"10.50", "ten point five zero"},
		{"24700", "twenty-four thousand seven hundred"},
	}
	for _, tc := range cases {
		got, ok := spellNumberToken(tc.tok)
		if !ok {
			t.Errorf("spellNumberToken(%q) ok=false; want true", tc.tok)
			continue
		}
		if got != tc.want {
			t.Errorf("spellNumberToken(%q) = %q; want %q", tc.tok, got, tc.want)
		}
	}
}

// TestSpellNumberToken_Reject — the ok=false fallback-identity contract at the
// speller level: malformed / negative / multi-dot / non-numeric tokens must be
// rejected so the caller emits the original bytes verbatim (Suggestion 3).
func TestSpellNumberToken_Reject(t *testing.T) {
	t.Parallel()
	for _, tok := range []string{
		"",      // empty
		"-5",    // negative — sign vs hyphen ambiguous
		"1.2.3", // multi-dot version
		"3.",    // trailing dot, no fractional digits
		".5",    // leading dot, no integer part
		"0xFF",  // hex
		"12a",   // trailing letter
		"1,000", // embedded comma
	} {
		if got, ok := spellNumberToken(tok); ok {
			t.Errorf("spellNumberToken(%q) = (%q, true); want ok=false", tok, got)
		}
	}
}

// TestCommaGroupedRun_Direct — direct-call matrix over the shape validator
// (#139). Shape-only: it accepts well-formed US comma grouping and returns the
// comma-stripped digits, declining plain runs and every malformed shape. The
// 1-6 length gate is NOT applied here (that is spellNumbersInProse's job), so
// 7- and 8-digit shapes report ok=true with their full joined digits; the
// prose matrix pins that they then LEAVE at the caller's length gate.
func TestCommaGroupedRun_Direct(t *testing.T) {
	t.Parallel()
	cases := []struct {
		text       string
		wantSpan   string
		wantJoined string
		wantEnd    int
		wantOK     bool
	}{
		// Accept.
		{"24,700", "24,700", "24700", 6, true},
		{"1,234", "1,234", "1234", 5, true},
		{"12,345", "12,345", "12345", 6, true},
		{"123,456", "123,456", "123456", 7, true},
		// Shape true, but the caller's 1-6 length gate LEAVEs these.
		{"1,000,000", "1,000,000", "1000000", 9, true},
		{"12,345,678", "12,345,678", "12345678", 10, true},
		// Decline: malformed grouping.
		{"1,23", "", "", 0, false},      // 2 digits in second group
		{"12,34", "", "", 0, false},     // 2 digits in second group
		{"1,2345", "", "", 0, false},    // 5 digits after a valid triple boundary
		{"12,3456", "", "", 0, false},   // partial-spell hazard: 4-digit group
		{"1234,567", "", "", 0, false},  // 4-digit first group
		{"24,7000", "", "", 0, false},   // 4-digit group
		{"24,700,", "", "", 0, false},   // trailing comma
		{"1,000x", "", "", 0, false},    // letter after last triple
		{"1,234.5", "", "", 0, false},   // decimal point after last triple — decline whole span
		{"24,700.99", "", "", 0, false}, // decimal point after last triple — decline whole span
		{"0,000", "", "", 0, false},     // leading-zero first group — drops info if collapsed
		{"01,000", "", "", 0, false},    // leading-zero first group
		{"24700", "", "", 0, false},     // no comma — a plain run
	}
	for _, tc := range cases {
		span, joined, end, ok := commaGroupedRun(tc.text, 0)
		if ok != tc.wantOK {
			t.Errorf("commaGroupedRun(%q) ok=%v; want %v", tc.text, ok, tc.wantOK)
			continue
		}
		if !ok {
			continue // span/joined/end are don't-care on decline
		}
		if span != tc.wantSpan || joined != tc.wantJoined || end != tc.wantEnd {
			t.Errorf("commaGroupedRun(%q) = (%q, %q, %d, %v); want (%q, %q, %d, %v)",
				tc.text, span, joined, end, ok,
				tc.wantSpan, tc.wantJoined, tc.wantEnd, tc.wantOK)
		}
	}
}

// TestSpellNumbersInProse — the prose-path matrix, asserted through the full
// strip+spell seam voiceSpelled (no lexicon overlay) so the assertions reflect
// exactly what degradeProse emits. Uses an empty compiled lexicon so only the
// number pass is under test. Markdown-wrapped rows exercise the strip-before-
// spell ordering fix in BOTH directions (spell AND leave — Suggestion 2).
func TestSpellNumbersInProse(t *testing.T) {
	t.Parallel()
	lex := compileLexicon() // DefaultLexicon only; no plain-word keys.
	cases := []struct {
		name string
		in   string
		want string
	}{
		// Markdown-wrapped SPELL (Blocker-2 guard): strip removes emphasis, the
		// bare number then presents with whitespace neighbours and spells.
		{"bold spells", "**24700** ops", "twenty-four thousand seven hundred ops"},
		{"emph decimal spells", "it hit *3.5* today", "it hit three point five today"},
		// Markdown-wrapped LEAVE (Suggestion 2): strip removes the wrapper but
		// preserves the ":"/version signal, so the guard still leaves as digits.
		{"backtick config leaves", "`replicas: 3` today", "replicas: 3 today"},
		{"bold version leaves", "**v2.4.0** shipped", "v2.4.0 shipped"},
		// Plain prose spells.
		{"plain integer spells", "there are 24700 nodes", "there are twenty-four thousand seven hundred nodes"},
		{"times spells", "it was 3.5 times faster", "it was three point five times faster"},
		{"exactly 6 digits spells", "exactly 123456 rows", "exactly one hundred twenty-three thousand four hundred fifty-six rows"},
		{"zero spells", "0 errors found", "zero errors found"},
		{"single digit spells", "9 of them", "nine of them"},
		// Guard LEAVES.
		{"letter-adjacent x leaves", "scaled to 3.5x throughput", "scaled to 3.5x throughput"},
		{"colon leaves", "replicas: 3", "replicas: 3"},
		{"equals leaves", "set x=5 here", "set x=5 here"},
		{"slash leaves", "the ratio 1/2", "the ratio 1/2"},
		{"version leaves", "upgrade to v2.4.0 now", "upgrade to v2.4.0 now"},
		{"hex leaves", "mask 0xFF applied", "mask 0xFF applied"},
		{"ten digits leaves", "call 5551234567 today", "call 5551234567 today"},
		{"seven digits leaves", "exactly 1234567 rows", "exactly 1234567 rows"},
		{"underscore leaves", "port_8080 open", "port_8080 open"},
		{"letter-prefixed leaves", "sha256 digest", "sha256 digest"},
		{"negative leaves", "-5 degrees", "-5 degrees"},
		// Boundary punctuation preserved.
		{"trailing period spells keep dot", "we saw 24700.", "we saw twenty-four thousand seven hundred."},
		{"trailing comma spells keep comma", "the value 3.5, roughly", "the value three point five, roughly"},
		// Comma-grouped SPELL (#139 step 0 pre-pass): well-formed groups spell as
		// a single cardinal on their comma-stripped digits.
		{"comma-grouped spells (relocated from LEAVE)", "total 24,700 rows", "total twenty-four thousand seven hundred rows"},
		{"comma-grouped spells both (relocated from identity)", "grid 24,700 by 24,700 ok", "grid twenty-four thousand seven hundred by twenty-four thousand seven hundred ok"},
		{"comma-grouped four-digit-first spells", "we saw 1,234 nodes", "we saw one thousand two hundred thirty-four nodes"},
		{"comma-grouped trailing period keep dot", "we saw 24,700.", "we saw twenty-four thousand seven hundred."},
		{"comma-grouped trailing comma keep comma", "the value 24,700, roughly", "the value twenty-four thousand seven hundred, roughly"},
		// Comma-grouped LEAVE (#139 conservative-LEAVE net): oversized count,
		// malformed grouping, partial-spell hazard, and neighbour-guarded contexts
		// all fall through to the plain path and stay as digits.
		{"comma-grouped seven digits leaves", "exactly 1,000,000 hits", "exactly 1,000,000 hits"},
		{"comma-grouped malformed leaves", "malformed 1,23 here", "malformed 1,23 here"},
		{"comma-grouped partial-spell hazard leaves whole span", "batch 12,3456 done", "batch 12,3456 done"},
		{"comma-grouped letter-prefixed leaves", "id x24,700 tag", "id x24,700 tag"},
		{"comma-grouped unit-suffixed leaves", "24,700px wide", "24,700px wide"},
		{"comma-grouped version leaves", "v1,024 build", "v1,024 build"},
		{"comma-grouped config equals leaves", "set count=1,024 now", "set count=1,024 now"},
		{"comma-grouped config colon leaves", "replicas: 1,024", "replicas: 1,024"},
		// Comma-grouped DECIMAL (#139 out-of-scope): a grouped integer immediately
		// followed by a decimal point must decline the WHOLE span and stay identity
		// — never spell "1,234" and orphan the raw ".5" (value-altering partial).
		{"comma-grouped decimal leaves whole span", "value 1,234.5 here", "value 1,234.5 here"},
		{"comma-grouped decimal two-frac leaves whole span", "value 24,700.99 x", "value 24,700.99 x"},
		// Leading-zero first group leaves (never collapse "0,000" -> "zero").
		{"comma-grouped leading-zero first group leaves", "count 0,000 total", "count 0,000 total"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := voiceSpelled(tc.in, lex)
			if got != tc.want {
				t.Errorf("voiceSpelled(%q) = %q; want %q", tc.in, got, tc.want)
			}
		})
	}
}

// TestSpellNumbersInProse_FallbackIdentity — the identity contract
// (Suggestion 3): a run that is NOT spelled must leave the surrounding bytes,
// and the run itself, byte-for-byte unchanged. Every input here contains only
// runs the guard leaves as digits (slash / hex / letter / neighbour-guarded
// grouping), so the whole string must come back identical. This pins the "leave
// as digits" fallback that the ok=false branch also relies on: never partial,
// never guessed, always the original bytes.
func TestSpellNumbersInProse_FallbackIdentity(t *testing.T) {
	t.Parallel()
	cases := []string{
		"the ratio 1/2 holds",     // slash-guarded
		"mask 0xFF and 0x1A here", // hex
		"ids sha1 and md5 vals",   // letter-adjacent
		"malformed 1,23 grouping", // malformed comma group — pre-pass declines
		"v1,024 tag here",         // neighbour-guarded comma group (version prefix)
	}
	for _, in := range cases {
		if got := spellNumbersInProse(in); got != in {
			t.Errorf("spellNumbersInProse(%q) = %q; want identity (unchanged)", in, got)
		}
	}
}
