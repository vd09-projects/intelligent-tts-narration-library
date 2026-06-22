package planner

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/vd09-projects/intelligent-tts-narration-library/intelligence"
	"github.com/vd09-projects/intelligent-tts-narration-library/plan"
)

func TestLevel_HeadingAllLevels(t *testing.T) {
	t.Parallel()
	lex := compileLexicon()
	rb := rawBlock{text: "## Stack", hint: hintHeading, startLine: 5, endLine: 5}
	for _, lv := range []plan.Level{plan.L1, plan.L2, plan.L3} {
		got := level(rb, plan.ClassHeading, lv, lex)
		if len(got.segments) != 1 || !strings.Contains(got.segments[0].Text, "Section:") {
			t.Errorf("heading L%d: want Section: prefix, got %+v", lv, got)
		}
		if !got.deterministic || got.needsIntelligence {
			t.Errorf("heading should be deterministic, no intelligence needed at L%d", lv)
		}
	}
}

func TestLevel_ListAllLevels(t *testing.T) {
	t.Parallel()
	lex := compileLexicon()
	// Bare 3-item list voices identically at L1/L2/L3 (levelList has no
	// level switch). Exact-string equality pins ordinal cues, comma/period
	// punctuation, and ordering — the loose Contains check it replaced
	// could not catch an off-by-one in the ordinal table.
	rb := rawBlock{text: "- alpha\n- beta\n- gamma\n", hint: hintList}
	const want = "List of 3 items. First, alpha. Second, beta. Third, gamma."
	for _, lv := range []plan.Level{plan.L1, plan.L2, plan.L3} {
		got := level(rb, plan.ClassList, lv, lex)
		if len(got.segments) != 1 {
			t.Fatalf("list L%d: want 1 segment, got %d", lv, len(got.segments))
		}
		if got.segments[0].ID != "s1" {
			t.Errorf("list L%d: want segment id s1, got %q", lv, got.segments[0].ID)
		}
		if got.segments[0].Text != want {
			t.Errorf("list L%d text mismatch:\n want %q\n  got %q", lv, want, got.segments[0].Text)
		}
		if !got.deterministic || got.needsIntelligence {
			t.Errorf("list L%d should be deterministic, no intelligence", lv)
		}
	}
}

func TestLevel_ListTitled(t *testing.T) {
	t.Parallel()
	lex := compileLexicon()
	// CASE 1 — a leading non-marker line is the title, not an item. The
	// trailing colon becomes a period; N counts only the real items (3).
	rb := rawBlock{text: "Some items:\n- alpha\n- beta\n- gamma\n", hint: hintList}
	const want = "Some items. First, alpha. Second, beta. Third, gamma."
	got := level(rb, plan.ClassList, plan.L1, lex)
	if got.segments[0].Text != want {
		t.Errorf("titled list text mismatch:\n want %q\n  got %q", want, got.segments[0].Text)
	}
}

func TestLevel_ListTitledColonNormalises(t *testing.T) {
	t.Parallel()
	lex := compileLexicon()
	// Title detection requires a trailing colon (the only reliable label
	// signal given goldmark strips the first item's marker). The colon is
	// normalised to a single period with exactly one space before the first
	// cue — no double period.
	rb := rawBlock{text: "Steps to deploy:\n- alpha\n- beta\n", hint: hintList}
	const want = "Steps to deploy. First, alpha. Second, beta."
	got := level(rb, plan.ClassList, plan.L1, lex)
	if got.segments[0].Text != want {
		t.Errorf("titled-colon list mismatch:\n want %q\n  got %q", want, got.segments[0].Text)
	}
	if strings.Contains(got.segments[0].Text, "..") {
		t.Errorf("double period leaked: %q", got.segments[0].Text)
	}
}

func TestLevel_ListEmptyBare(t *testing.T) {
	t.Parallel()
	lex := compileLexicon()
	rb := rawBlock{text: "", hint: hintList}
	const want = "List of 0 items."
	got := level(rb, plan.ClassList, plan.L1, lex)
	if got.segments[0].Text != want {
		t.Errorf("empty bare list mismatch:\n want %q\n  got %q", want, got.segments[0].Text)
	}
	if strings.Contains(got.segments[0].Text, "..") {
		t.Errorf("double period leaked: %q", got.segments[0].Text)
	}
}

func TestLevel_ListEmptyTitled(t *testing.T) {
	t.Parallel()
	lex := compileLexicon()
	// A title with no following items → "{title}." only, no item clauses,
	// no dangling space.
	rb := rawBlock{text: "Some items:\n", hint: hintList}
	const want = "Some items."
	got := level(rb, plan.ClassList, plan.L1, lex)
	if got.segments[0].Text != want {
		t.Errorf("empty titled list mismatch:\n want %q\n  got %q", want, got.segments[0].Text)
	}
}

func TestLevel_ListSingleItem(t *testing.T) {
	t.Parallel()
	lex := compileLexicon()
	// Bare single item. N=1 is the only singular case — the preamble agrees in
	// number ("1 item"); every other N (including N=0) stays plural ("items").
	bare := level(rawBlock{text: "- alpha\n", hint: hintList}, plan.ClassList, plan.L1, lex)
	if got, want := bare.segments[0].Text, "List of 1 item. First, alpha."; got != want {
		t.Errorf("single bare item mismatch:\n want %q\n  got %q", want, got)
	}
	// Titled single item.
	titled := level(rawBlock{text: "Just one:\n- alpha\n", hint: hintList}, plan.ClassList, plan.L1, lex)
	if got, want := titled.segments[0].Text, "Just one. First, alpha."; got != want {
		t.Errorf("single titled item mismatch:\n want %q\n  got %q", want, got)
	}
}

func TestLevel_ListOrdinalBoundary(t *testing.T) {
	t.Parallel()
	lex := compileLexicon()
	// >10 items: position 10 is spelled ("Tenth,"), position 11 crosses to
	// the numeric fallback ("item 11,"). Pins the ordinal-table upper bound
	// and the spelled→numeric handoff.
	var b strings.Builder
	for i := 1; i <= 11; i++ {
		fmt.Fprintf(&b, "- item%d\n", i)
	}
	got := level(rawBlock{text: b.String(), hint: hintList}, plan.ClassList, plan.L1, lex).segments[0].Text
	if !strings.Contains(got, "Tenth, item10.") {
		t.Errorf("position 10 should be spelled 'Tenth,': %q", got)
	}
	if !strings.Contains(got, "item 11, item11.") {
		t.Errorf("position 11 should be numeric 'item 11,': %q", got)
	}
	if !strings.HasPrefix(got, "List of 11 items.") {
		t.Errorf("bare preamble should count 11 items: %q", got)
	}
}

func TestLevel_CodeL1L2L3(t *testing.T) {
	t.Parallel()
	lex := compileLexicon()
	body := "```go\nfunc One() int { return 1 }\ntype S struct{ X int }\n```"
	rb := rawBlock{text: body, hint: hintCode, fenceInfo: "go"}

	l1 := level(rb, plan.ClassCode, plan.L1, lex)
	if !strings.Contains(l1.segments[0].Text, "Go code block") {
		t.Errorf("code L1 missing lang: %q", l1.segments[0].Text)
	}

	// Code L2 below the gate now asks for an AI semantic gist: empty
	// segments + needsIntelligence (issue #48). The deterministic decls
	// still flow to the adapter as Facts, not as voiced segments.
	l2 := level(rb, plan.ClassCode, plan.L2, lex)
	if !l2.needsIntelligence {
		t.Errorf("code L2 under the gate should need intelligence")
	}
	if len(l2.segments) != 0 {
		t.Errorf("code L2 under the gate should emit no deterministic segments, got %+v", l2.segments)
	}
	if !factsContain(l2.facts, "decls: 2") {
		t.Errorf("code L2 should carry decls fact for the adapter: %v", l2.facts)
	}

	l3 := level(rb, plan.ClassCode, plan.L3, lex)
	if !l3.needsIntelligence {
		t.Errorf("code L3 should need intelligence")
	}
}

// factsContain — small test helper: true if any fact string contains sub.
func factsContain(facts []string, sub string) bool {
	for _, f := range facts {
		if strings.Contains(f, sub) {
			return true
		}
	}
	return false
}

// TestLevel_CodeL1ByteIdentical — explicit regression guard (issue #48,
// Suggestion 3). L1 code behavior must stay BYTE-IDENTICAL after the L2
// enrichment landed; the L1 arm of levelCode was not to be touched.
func TestLevel_CodeL1ByteIdentical(t *testing.T) {
	t.Parallel()
	lex := compileLexicon()
	rb := rawBlock{text: "```go\npackage main\n\nfunc main() {\n\tprintln(\"hello\")\n}\n```", hint: hintCode, fenceInfo: "go"}
	got := level(rb, plan.ClassCode, plan.L1, lex)
	if len(got.segments) != 1 {
		t.Fatalf("code L1 should emit exactly one segment, got %d", len(got.segments))
	}
	const want = "A 5-line Go code block."
	if got.segments[0].Text != want {
		t.Errorf("code L1 text drifted:\n want %q\n  got %q", want, got.segments[0].Text)
	}
	if !got.deterministic || got.needsIntelligence {
		t.Errorf("code L1 must stay deterministic with no intelligence: %+v", got)
	}
}

// TestLevel_CodeL2Gate — the AI size-gate is behavioral: below the gate
// levelCode asks for intelligence (empty segments); above it voices the
// deterministic count+decls gist directly (no LLM, Status path = voiced),
// with no size_gated fact anywhere. Boundary pinned at exactly
// codeGistMaxLines and +1.
func TestLevel_CodeL2Gate(t *testing.T) {
	t.Parallel()
	lex := compileLexicon()

	// codeBlock builds a fenced go block with n body lines and a single
	// top-level decl seam so the body line count is deterministic.
	codeBlock := func(bodyLines int) rawBlock {
		var b strings.Builder
		b.WriteString("```go\n")
		b.WriteString("func process() {\n")
		for i := 0; i < bodyLines-2; i++ {
			fmt.Fprintf(&b, "\tx := %d\n", i)
		}
		b.WriteString("}\n")
		b.WriteString("```")
		return rawBlock{text: b.String(), hint: hintCode, fenceInfo: "go"}
	}

	cases := []struct {
		name            string
		bodyLines       int
		wantIntel       bool
		wantSegments    int
		wantSizeGatedNo bool
	}{
		{"at_gate_needs_intel", codeGistMaxLines, true, 0, true},
		{"plus_one_gated_voiced", codeGistMaxLines + 1, false, 1, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			rb := codeBlock(tc.bodyLines)
			// Confirm the fixture lands on the intended side of the gate.
			if got := countLines(stripFenceMarkers(rb.text)); (got > codeGistMaxLines) == tc.wantIntel {
				t.Fatalf("fixture line count %d is on the wrong side of gate %d for case %q", got, codeGistMaxLines, tc.name)
			}
			res := level(rb, plan.ClassCode, plan.L2, lex)
			if res.needsIntelligence != tc.wantIntel {
				t.Errorf("needsIntelligence = %v, want %v", res.needsIntelligence, tc.wantIntel)
			}
			if len(res.segments) != tc.wantSegments {
				t.Errorf("segments = %d, want %d", len(res.segments), tc.wantSegments)
			}
			// No size_gated fact must ever be emitted — the gate is
			// behavioral-only.
			if factsContain(res.facts, "size_gated") {
				t.Errorf("size_gated fact leaked into facts: %v", res.facts)
			}
		})
	}
}

// TestDeterministicCodeGist — the shared helper produces the expected
// raw string and decl count for representative inputs (with and without
// top-level declarations). langPhrase is passed by the caller exactly as
// codeLangPhrase produces it.
func TestDeterministicCodeGist(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name      string
		body      string
		lang      string
		wantText  string
		wantDecls int
	}{
		{
			name:      "with_decls",
			body:      "func main() {\n\tprintln(1)\n}",
			lang:      "go",
			wantText:  "A 3-line Go code block. Declares main.",
			wantDecls: 1,
		},
		{
			name:      "no_decls",
			body:      "x := 1\ny := 2",
			lang:      "go",
			wantText:  "A 2-line Go code block.",
			wantDecls: 0,
		},
		{
			name:      "unfenced_no_lang",
			body:      "echo hi",
			lang:      "",
			wantText:  "A 1-line code block.",
			wantDecls: 0,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			gotText, gotDecls := deterministicCodeGist(tc.body, codeLangPhrase(tc.lang))
			if gotText != tc.wantText {
				t.Errorf("text mismatch:\n want %q\n  got %q", tc.wantText, gotText)
			}
			if gotDecls != tc.wantDecls {
				t.Errorf("decl count = %d, want %d", gotDecls, tc.wantDecls)
			}
		})
	}
}

// TestCodeL2_GateAndDegradeByteIdentical — cross-path byte-identity
// (issue #48, Suggestion 2). The SAME code body taken through (a) the
// gated levelCode path and (b) the no-adapter degradeCodeL2 path must
// produce byte-identical segment text, because both compute langPhrase
// via codeLangPhrase and call deterministicCodeGist.
func TestCodeL2_GateAndDegradeByteIdentical(t *testing.T) {
	t.Parallel()
	lex := compileLexicon()
	// An oversized (>gate) seamless block so the gated path voices
	// deterministically rather than asking for intelligence.
	var b strings.Builder
	b.WriteString("```go\n")
	b.WriteString("func process() {\n")
	for i := 0; i < codeGistMaxLines+5; i++ {
		fmt.Fprintf(&b, "\tx := %d\n", i)
	}
	b.WriteString("}\n")
	b.WriteString("```")
	rb := rawBlock{text: b.String(), hint: hintCode, fenceInfo: "go"}

	gated := level(rb, plan.ClassCode, plan.L2, lex)
	if len(gated.segments) != 1 {
		t.Fatalf("gated path should voice one segment, got %d", len(gated.segments))
	}
	degraded, _ := degradeCodeL2(rb, plan.SourceMap{}, lex)
	if len(degraded.Segments) != 1 {
		t.Fatalf("degrade path should voice one segment, got %d", len(degraded.Segments))
	}
	if gated.segments[0].Text != degraded.Segments[0].Text {
		t.Errorf("gate vs degrade text not byte-identical:\n gate     %q\n degraded %q",
			gated.segments[0].Text, degraded.Segments[0].Text)
	}
}

func TestLevel_ConfigL1(t *testing.T) {
	t.Parallel()
	lex := compileLexicon()
	body := "```yaml\nreplicas: 3\nname: web\n```"
	rb := rawBlock{text: body, hint: hintCode, fenceInfo: "yaml"}
	got := level(rb, plan.ClassConfig, plan.L1, lex)
	if !strings.Contains(got.segments[0].Text, "YAML config") {
		t.Errorf("config L1: %q", got.segments[0].Text)
	}
	if !strings.Contains(got.segments[0].Text, "2 top-level") {
		t.Errorf("config L1 key count: %q", got.segments[0].Text)
	}
}

func TestLevel_TableL1(t *testing.T) {
	t.Parallel()
	lex := compileLexicon()
	rb := rawBlock{text: "| a | b | c |\n|---|---|---|\n| 1 | 2 | 3 |\n| 4 | 5 | 6 |", hint: hintTable}
	got := level(rb, plan.ClassTable, plan.L1, lex)
	if !strings.Contains(got.segments[0].Text, "3-column") {
		t.Errorf("table L1 cols: %q", got.segments[0].Text)
	}
	if !strings.Contains(got.segments[0].Text, "2-row") {
		t.Errorf("table L1 rows: %q", got.segments[0].Text)
	}
}

func TestLevel_DiagramL1(t *testing.T) {
	t.Parallel()
	lex := compileLexicon()
	body := "```mermaid\ngraph TD\nA-->B\nB-->C\n```"
	rb := rawBlock{text: body, hint: hintCode, fenceInfo: "mermaid"}
	got := level(rb, plan.ClassDiagramAsText, plan.L1, lex)
	if !strings.Contains(got.segments[0].Text, "Mermaid diagram") {
		t.Errorf("diagram L1: %q", got.segments[0].Text)
	}
}

func TestLevel_ProseNeedsIntelligence(t *testing.T) {
	t.Parallel()
	lex := compileLexicon()
	rb := rawBlock{text: "A sentence.", hint: hintProse}
	got := level(rb, plan.ClassProse, plan.L1, lex)
	if !got.needsIntelligence {
		t.Errorf("prose should always need intelligence")
	}
}

func TestLevel_UnknownEmitsRefuseHint(t *testing.T) {
	t.Parallel()
	lex := compileLexicon()
	rb := rawBlock{text: "![](x.png)", hint: hintImage}
	got := level(rb, plan.ClassUnknown, plan.L1, lex)
	if got.refuseHint != plan.RefuseBareImage {
		t.Errorf("unknown class should hint RefuseBareImage, got %q", got.refuseHint)
	}
}

func TestLevel_OversizedProseSplits(t *testing.T) {
	t.Parallel()
	lex := compileLexicon()
	// Build prose well past 800 chars with sentence boundaries.
	var b strings.Builder
	for range 30 {
		b.WriteString("Sentence number ")
		for range 10 {
			b.WriteString("filler ")
		}
		b.WriteString(". ")
	}
	rb := rawBlock{text: b.String(), hint: hintProse}
	got := level(rb, plan.ClassProse, plan.L1, lex)
	if len(got.subBlocks) < 2 {
		t.Errorf("oversized prose should split, got %d sub-blocks", len(got.subBlocks))
	}
}

func TestLevel_OversizedCodeSplitsOnDecls(t *testing.T) {
	t.Parallel()
	lex := compileLexicon()
	var b strings.Builder
	for i := range 80 {
		b.WriteString("func F")
		b.WriteByte(byte('A' + i%26))
		b.WriteString("() {}\n  // body line\n")
	}
	rb := rawBlock{text: b.String(), hint: hintCode, fenceInfo: "go"}
	got := level(rb, plan.ClassCode, plan.L1, lex)
	if len(got.subBlocks) < 2 {
		t.Errorf("oversized code should split, got %d sub-blocks", len(got.subBlocks))
	}
}

// nWords builds a single sentence (period-terminated) with exactly n
// whitespace-separated words, so the cap-boundary fixtures are
// self-evidently correct rather than hand-counted.
func nWords(n int) string {
	if n <= 0 {
		return ""
	}
	parts := make([]string, n)
	for i := range parts {
		parts[i] = "w"
	}
	return strings.Join(parts, " ") + "."
}

// TestFirstSentenceWithinCap — the standalone code-L2 enforcement helper
// (issue #60). Locks the trim-to-first-sentence + hard-word-cap behavior:
// compliant pass-through, multi-sentence trim, the exactly-cap / cap+1
// boundary (pins `>` not `>=`), terminator-less single-candidate handling,
// empty/whitespace non-panicking refusal, and the documented abbreviation
// early-cut tradeoff. No size floor — short multi-sentence input still
// trims (the splitProse path would not).
func TestFirstSentenceWithinCap(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name       string
		text       string
		wantCapped string
		wantOK     bool
	}{
		{
			name:       "compliant_passthrough_byte_identical",
			text:       "Sorts the slice in place using quicksort.",
			wantCapped: "Sorts the slice in place using quicksort.",
			wantOK:     true,
		},
		{
			name:       "overlong_multi_sentence_trimmed_to_first",
			text:       "Parses the config file. Then it validates each key. Finally it returns the struct.",
			wantCapped: "Parses the config file.",
			wantOK:     true,
		},
		{
			name:       "exactly_cap_words_voiced",
			text:       nWords(codeL2MaxWords),
			wantCapped: nWords(codeL2MaxWords),
			wantOK:     true,
		},
		{
			name:   "cap_plus_one_words_single_sentence_refused",
			text:   nWords(codeL2MaxWords + 1),
			wantOK: false,
		},
		{
			name:       "no_terminator_within_cap_voiced",
			text:       "Registers the route handler on the mux",
			wantCapped: "Registers the route handler on the mux",
			wantOK:     true,
		},
		{
			name:   "no_terminator_over_cap_refused",
			text:   strings.TrimSuffix(nWords(codeL2MaxWords+1), "."),
			wantOK: false,
		},
		{
			name:   "empty_refused_no_panic",
			text:   "",
			wantOK: false,
		},
		{
			name:   "whitespace_only_refused_no_panic",
			text:   "   \n\t  ",
			wantOK: false,
		},
		{
			// Accepted tradeoff (plan Risks): the terminator predicate fires
			// at the period after "e.g" because it is followed by a space, so
			// the candidate trims early to "e.g." Over-trimming is honest —
			// still the adapter's own words — and lower-harm than over-voicing.
			name:       "abbreviation_early_cut_documented_tradeoff",
			text:       "e.g. foo bar.",
			wantCapped: "e.g.",
			wantOK:     true,
		},
		{
			// Whitespace-after-terminator is required, so a version string is
			// NOT a cut point (guards the decimal/version edge case).
			name:       "version_string_not_a_cut_point",
			text:       "Pins the dep to v1.5.0 exactly.",
			wantCapped: "Pins the dep to v1.5.0 exactly.",
			wantOK:     true,
		},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			capped, ok := firstSentenceWithinCap(tc.text, codeL2MaxWords)
			if ok != tc.wantOK {
				t.Fatalf("ok = %v, want %v (capped=%q)", ok, tc.wantOK, capped)
			}
			if ok && capped != tc.wantCapped {
				t.Errorf("capped mismatch:\n want %q\n  got %q", tc.wantCapped, capped)
			}
			if !ok && capped != "" {
				t.Errorf("ok=false must return empty capped, got %q", capped)
			}
		})
	}
}

// TestCallIntelligence_CodeL2CapGuard — the choke-point guard in
// callIntelligence (issue #60). Confirms the cap is enforced ONLY for
// ClassCode at L2: a compliant reply passes through byte-identical, an
// over-cap code-L2 reply becomes a RefuseTooLarge refusal (Spoken=true,
// SourceMap populated — honesty rule), and non-code classes plus code at
// L1/L3 are UNTOUCHED (the raw reply survives verbatim, no refusal).
func TestCallIntelligence_CodeL2CapGuard(t *testing.T) {
	t.Parallel()
	overCap := nWords(codeL2MaxWords + 1)
	multi := "Parses the config file. Then it validates each key."
	sm := plan.SourceMap{StartLine: 1, EndLine: 4}

	cases := []struct {
		name string
		cls  plan.Class
		lvl  plan.Level
		// reply is the adapter's scripted Text.
		reply string
		// wantRefused → expect Status=refused with RefuseTooLarge.
		wantRefused bool
		// wantText → expected Segment.Text when voiced (ignored if refused).
		wantText string
	}{
		{
			name:        "code_L2_overcap_refused",
			cls:         plan.ClassCode,
			lvl:         plan.L2,
			reply:       overCap,
			wantRefused: true,
		},
		{
			name:     "code_L2_multi_sentence_trimmed",
			cls:      plan.ClassCode,
			lvl:      plan.L2,
			reply:    multi,
			wantText: "Parses the config file.",
		},
		{
			name:     "code_L2_compliant_passthrough",
			cls:      plan.ClassCode,
			lvl:      plan.L2,
			reply:    "Sorts the slice in place.",
			wantText: "Sorts the slice in place.",
		},
		{
			// Non-code class at L2 is untouched even when over-cap.
			name:     "prose_L2_untouched",
			cls:      plan.ClassProse,
			lvl:      plan.L2,
			reply:    overCap,
			wantText: overCap,
		},
		{
			// Code at L1 is untouched even when over-cap.
			name:     "code_L1_untouched",
			cls:      plan.ClassCode,
			lvl:      plan.L1,
			reply:    overCap,
			wantText: overCap,
		},
		{
			// Code at L3 is untouched even when over-cap.
			name:     "code_L3_untouched",
			cls:      plan.ClassCode,
			lvl:      plan.L3,
			reply:    overCap,
			wantText: overCap,
		},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			intel := &scriptedIntel{replies: []intelligence.IntelligenceResult{
				{Text: tc.reply, Model: "scripted@test"},
			}}
			rb := rawBlock{text: "irrelevant raw source", startLine: 1, endLine: 4}
			blk, _, err := callIntelligence(
				context.Background(), rb, tc.cls, tc.lvl, sm, levelResult{}, intel,
			)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tc.wantRefused {
				if blk.Status != plan.StatusRefused {
					t.Fatalf("status = %q, want refused", blk.Status)
				}
				if blk.Refusal == nil {
					t.Fatal("refused block must carry a Refusal")
				}
				if blk.Refusal.Reason != plan.RefuseTooLarge {
					t.Errorf("refusal reason = %q, want %q", blk.Refusal.Reason, plan.RefuseTooLarge)
				}
				if !blk.Refusal.Spoken {
					t.Error("refusal must be Spoken (honesty rule)")
				}
				if blk.Refusal.SourceMap != sm {
					t.Errorf("refusal SourceMap = %+v, want %+v", blk.Refusal.SourceMap, sm)
				}
				if len(blk.Segments) != 0 {
					t.Errorf("refused block must have no segments, got %d", len(blk.Segments))
				}
				return
			}
			if blk.Status != plan.StatusVoiced {
				t.Fatalf("status = %q, want voiced", blk.Status)
			}
			if len(blk.Segments) != 1 {
				t.Fatalf("voiced block must have one segment, got %d", len(blk.Segments))
			}
			if blk.Segments[0].Text != tc.wantText {
				t.Errorf("segment text mismatch:\n want %q\n  got %q", tc.wantText, blk.Segments[0].Text)
			}
		})
	}
}
