package planner

import (
	"fmt"
	"strings"

	"github.com/vd09-projects/intelligent-tts-narration-library/plan"
)

// Oversized-block split thresholds (CLAUDE.md domain rules, A7 nuance).
//
// Decision: convention — prose and structured classes use separate
// thresholds. Prose is harder to read aloud than structured content, so
// a tighter cap (~20 lines / ~800 chars) keeps gist mode coherent.
// Structured content has clean seams (func boundaries, top-level keys,
// table rows) and can hold longer before splitting (~60-80 lines /
// ~2000-3000 chars).
// Why: a single uniform threshold would either split structured blocks
// too aggressively (breaking class-aware leveling) or let prose blocks
// run past audio-coherence limits.
const (
	proseMaxLines      = 20
	proseMaxChars      = 800
	structuredMaxLines = 70   // midpoint of 60-80 range.
	structuredMaxChars = 2500 // midpoint of 2000-3000 range.
)

// codeGistMaxLines is the AI size-GATE for code at L2, distinct from the
// split THRESHOLD above. The two do different jobs and must not be
// confused:
//
//   - GATE (codeGistMaxLines): a code block whose line count exceeds this
//     is NOT sent to the intelligence adapter at L2. It is voiced
//     deterministically (count + declarations) with Status=voiced. The
//     gate is a billing / quality safety net for very large blocks — it
//     does NOT cut the block, it just declines the LLM call for it.
//   - SPLIT (structuredMaxLines / structuredMaxChars, used by maybeSplit):
//     chops an oversized block into child blocks at clean structural seams
//     (func / type / top-level key / table row). It restructures; the gate
//     does not.
//
// Coexistence (why the gate is not dead code): a >250-line code block is
// usually already split by maybeSplit before leveling — UNLESS it has no
// clean seam (splitOnLineMatch needs ≥2 seams, e.g. one long function body
// or a flat run of indented statements). The gate is the safety net for
// exactly those large, seamless code blocks that survive splitting and
// reach levelCode whole.
const codeGistMaxLines = 250

// levelResult — what level() returns. The orchestrator inspects this to
// decide whether to call an IntelligenceAdapter (needsIntelligence) and
// how to assemble the final plan.Block.
type levelResult struct {
	// segments — the spoken segments, already voiced. Empty when the
	// block needs intelligence and none is available (orchestrator
	// degrade path takes over) or when the block is refused outright.
	segments []plan.Segment
	// subBlocks — non-nil when oversized-split produced child blocks.
	// The orchestrator emits the parent as a wrapper and re-levels
	// each child independently.
	subBlocks []rawBlock
	// provenance — to copy into the resulting plan.Block. Voicedby is
	// "planner" when level() produced the segments deterministically.
	provenance plan.Provenance
	// deterministic — true when segments were produced without any
	// intelligence call (informational; copied into provenance).
	deterministic bool
	// needsIntelligence — true when the (class, level) cell of the
	// design-doc §4 matrix requires comprehension. The orchestrator
	// calls IntelligenceAdapter.Voice if one is plugged in, otherwise
	// degrade.go decides verbatim-vs-refuse.
	needsIntelligence bool
	// refuseHint — non-empty when level() detected an un-voiceable
	// block (e.g. image). Carries a refusal reason for degrade.go.
	refuseHint plan.RefusalReason
	// facts — deterministic structural facts the orchestrator passes
	// to IntelligenceAdapter.Voice via IntelligenceRequest.Facts.
	facts []string
}

// level — apply the per-class L1/L2/L3 rules from the design doc §4 to
// rb. The caller (orchestrator) handles oversized-split detection by
// inspecting the returned subBlocks slice and emitting a wrapper.
func level(rb rawBlock, cls plan.Class, target plan.Level, lex *compiledLex) levelResult {
	// Oversized split runs first — if the block is too big, we hand
	// the orchestrator sub-blocks and let it re-level each.
	if subs := maybeSplit(rb, cls); subs != nil {
		return levelResult{
			subBlocks:     subs,
			provenance:    plan.Provenance{VoicedBy: "planner", Deterministic: true, LevelAsked: target},
			deterministic: true,
		}
	}

	switch cls {
	case plan.ClassHeading:
		return levelHeading(rb, target, lex)
	case plan.ClassList:
		return levelList(rb, target, lex)
	case plan.ClassCode:
		return levelCode(rb, target, lex)
	case plan.ClassConfig:
		return levelConfig(rb, target, lex)
	case plan.ClassTable:
		return levelTable(rb, target, lex)
	case plan.ClassDiagramAsText:
		return levelDiagram(rb, target, lex)
	case plan.ClassUnknown:
		return levelResult{
			provenance: plan.Provenance{VoicedBy: "planner", Deterministic: true, LevelAsked: target},
			refuseHint: plan.RefuseBareImage,
		}
	case plan.ClassProse, plan.ClassExample:
		fallthrough
	default:
		return levelProse(rb, target, lex)
	}
}

// ---------------------------------------------------------------------
// Per-class leveling
// ---------------------------------------------------------------------

// levelHeading — voice "Section: {text}" at all levels.
func levelHeading(rb rawBlock, target plan.Level, lex *compiledLex) levelResult {
	stripped := strings.TrimSpace(strings.TrimLeft(rb.text, "#"))
	stripped = strings.TrimSpace(stripped)
	text := "Section: " + voice(stripped, lex) + "."
	return levelResult{
		segments: []plan.Segment{
			{ID: "s1", Kind: plan.SegmentKindSpeech, Text: text},
		},
		provenance:    plan.Provenance{VoicedBy: "planner", Deterministic: true, LevelAsked: target},
		deterministic: true,
		facts:         []string{"class: heading"},
	}
}

// ordinalWords — frozen 1-based spelled-ordinal table for list positions
// 1..10. Index n-1 holds the cue for position n (ordinalWords[0] == "First").
// Past ten we fall back to a numeric "item N" cue (see ordinalCue) rather
// than ship an ordinal-spelling engine for long lists.
var ordinalWords = [...]string{
	"First", "Second", "Third", "Fourth", "Fifth",
	"Sixth", "Seventh", "Eighth", "Ninth", "Tenth",
}

// ordinalCue — spoken position cue for the n-th list item.
//
// CONTRACT: n is 1-BASED (item 1 → "First"), NOT the 0-based loop index.
// Callers iterating items must pass i+1, never i.
//
// 1..10 → spelled word from ordinalWords[n-1]; n ≥ 11 → numeric "item N".
//
// SPLIT-OFFSET TRAP: lists are not split by maybeSplit today (there is no
// ClassList case there), so a list voices as one block and positions run
// 1..N naturally. If list-splitting is ever added, each chunk MUST be given
// a global position offset — chunk 2's first item is global position N+1,
// not 1 — or this 1-based numbering will restart per chunk and mislabel.
func ordinalCue(n int) string {
	if n >= 1 && n <= len(ordinalWords) {
		return ordinalWords[n-1]
	}
	return fmt.Sprintf("item %d", n)
}

// levelList — voice a list with spoken ordinal cues at all levels:
// "{preamble} First, {item1}. Second, {item2}. …".
//
// Preamble has two forms (see splitListTitle):
//   - TITLED list (a leading non-marker line precedes the markers): the
//     source title becomes the preamble, with a trailing ":" converted to
//     ". " and any other ending normalised so exactly one ". " separates it
//     from the first cue. "Some items:" → "Some items. First, alpha. …".
//   - BARE list (no leading title): generated "List of N items. ".
//
// In both forms N counts only the real list items — a title line is never
// counted as an item. Items are extracted by stripping markdown list markers
// ("-", "*", "+", or digit-period); indented continuation lines fold into
// their parent item.
func levelList(rb rawBlock, target plan.Level, lex *compiledLex) levelResult {
	title, items := splitListTitle(rb.text, rb.firstItemDemarkered)

	var spoken strings.Builder
	if title != "" {
		spoken.WriteString(listTitlePreamble(voice(title, lex)))
	} else {
		noun := "items"
		if len(items) == 1 {
			noun = "item"
		}
		fmt.Fprintf(&spoken, "List of %d %s.", len(items), noun)
	}
	for i, item := range items {
		// ordinalCue is 1-based; the loop index is 0-based → pass i+1.
		spoken.WriteString(" ")
		spoken.WriteString(ordinalCue(i + 1))
		spoken.WriteString(", ")
		spoken.WriteString(voice(item, lex))
		spoken.WriteString(".")
	}
	return levelResult{
		segments: []plan.Segment{
			{ID: "s1", Kind: plan.SegmentKindSpeech, Text: spoken.String()},
		},
		provenance:    plan.Provenance{VoicedBy: "planner", Deterministic: true, LevelAsked: target},
		deterministic: true,
		facts:         []string{fmt.Sprintf("class: list, items: %d", len(items))},
	}
}

// listTitlePreamble — normalise a list title into a preamble ending in a
// single period (no trailing space; levelList adds the space before each
// cue). A trailing ":" becomes "."; an existing "." is kept (no double
// period); any other ending gets "." appended.
func listTitlePreamble(title string) string {
	title = strings.TrimSpace(title)
	switch {
	case strings.HasSuffix(title, ":"):
		return strings.TrimSuffix(title, ":") + "."
	case strings.HasSuffix(title, "."):
		return title
	default:
		return title + "."
	}
}

// splitListTitle — separate an optional leading title line from the real
// list items. A title is a leading non-list-marker line, ending in ":",
// that precedes the first list marker (e.g. the "Some items:" label above a
// bullet list). The title is NOT counted as an item; the returned items
// slice holds only real list items.
//
// The trailing-colon requirement is load-bearing, not cosmetic, on the
// plaintext-fallback path (firstItemDemarkered == false), where list markers
// survive intact: there a leading non-marker line is ambiguous (a real title
// OR a marker-less first line) and the colon is the only reliable label
// signal that disambiguates the two.
//
// firstItemDemarkered carries the goldmark seam (set by the segmenter, see
// rawBlock.firstItemDemarkered): when true the leading line is provably the
// AST-stripped first item, NEVER a title, so the colon title branch is
// skipped entirely — the whole block goes to extractListItems and the
// colon-terminated first item is counted in N. This makes issue #54's
// Direction 2 (a de-markered colon-terminated first item mis-promoted to a
// title, undercounting N and shifting every ordinalCue) impossible by
// construction on the markdown path.
//
// Returns ("", items) for a bare list (no titled label) so levelList takes
// the generated "List of N items." preamble path.
func splitListTitle(text string, firstItemDemarkered bool) (string, []string) {
	lines := strings.Split(text, "\n")
	for i, ln := range lines {
		trimmed := strings.TrimSpace(ln)
		if trimmed == "" {
			continue
		}
		if isListMarkerLine(trimmed) {
			break // first non-blank line is already a marker → bare list.
		}
		if !firstItemDemarkered && strings.HasSuffix(trimmed, ":") {
			// Plaintext-fallback titled list: markers survive, so a trailing
			// colon is the disambiguating label signal. This label line is the
			// preamble; the remaining lines are the real items.
			return trimmed, extractListItems(strings.Join(lines[i+1:], "\n"))
		}
		// Direction 2 (a colon-terminated de-markered first item mis-promoted
		// to a title) is now fixed by the firstItemDemarkered seam carried from
		// the segmenter — on the goldmark path the first line is never
		// title-promoted. The only residual is the plaintext non-colon leading
		// title (AC1), a ticket divergence (#54): in true markdown that label
		// is already its own prose block and is voiced correctly.
		break // leading non-marker, non-label line → treat whole block as bare.
	}
	return "", extractListItems(text)
}

// extractListItems — pull items from a markdown-shaped list block.
func extractListItems(text string) []string {
	var items []string
	lines := strings.Split(text, "\n")
	var current strings.Builder
	flush := func() {
		v := strings.TrimSpace(current.String())
		if v != "" {
			items = append(items, v)
		}
		current.Reset()
	}
	for _, ln := range lines {
		trimmed := strings.TrimSpace(ln)
		if trimmed == "" {
			continue
		}
		if isListMarkerLine(trimmed) {
			flush()
			// Drop the marker.
			rest := strings.TrimSpace(stripListMarker(trimmed))
			current.WriteString(rest)
		} else if len(ln) > 0 && (ln[0] == ' ' || ln[0] == '\t') {
			// Continuation line of the current item.
			current.WriteString(" ")
			current.WriteString(trimmed)
		} else {
			// Looks like a non-list line — flush and start fresh.
			flush()
			current.WriteString(trimmed)
		}
	}
	flush()
	return items
}

func isListMarkerLine(line string) bool {
	if line == "" {
		return false
	}
	if line[0] == '-' || line[0] == '*' || line[0] == '+' {
		return len(line) > 1 && line[1] == ' '
	}
	// Numbered list: "1." / "2." / etc.
	i := 0
	for i < len(line) && line[i] >= '0' && line[i] <= '9' {
		i++
	}
	if i > 0 && i < len(line) && line[i] == '.' &&
		i+1 < len(line) && line[i+1] == ' ' {
		return true
	}
	return false
}

func stripListMarker(line string) string {
	if line == "" {
		return line
	}
	if line[0] == '-' || line[0] == '*' || line[0] == '+' {
		return line[1:]
	}
	i := 0
	for i < len(line) && line[i] >= '0' && line[i] <= '9' {
		i++
	}
	if i > 0 && i < len(line) && line[i] == '.' {
		return line[i+1:]
	}
	return line
}

// codeLangPhrase — the deterministic language phrase used in every code
// gist string ("Go code", "Python code", or bare "code" for an unfenced
// block). Centralised so the size-gate path in levelCode and the
// no-adapter degrade path in degrade.go compute it IDENTICALLY — the two
// deterministic strings must be byte-identical (issue #48, Suggestion 2).
func codeLangPhrase(lang string) string {
	lang = strings.TrimSpace(lang)
	if lang == "" {
		return "code"
	}
	return humanLang(lang) + " code"
}

// deterministicCodeGist — build the raw (un-voiced) deterministic code
// gist body and the declaration count, shared by the L2 size-gate path
// (levelCode) and the no-adapter degrade path (degradeCodeL2). The body
// is "A {N}-line {langPhrase} block." plus, when decls are present,
// " Declares {decls}." Callers apply voice(...) themselves before
// emitting a Segment, so this stays a pure string builder.
//
// langPhrase MUST be codeLangPhrase(lang) at every call site — that is
// what guarantees the gate and degrade paths emit byte-identical text.
func deterministicCodeGist(body, langPhrase string) (string, int) {
	nLines := countLines(body)
	l1 := fmt.Sprintf("A %d-line %s block.", nLines, langPhrase)
	decls := extractTopLevelDecls(body)
	var spoken strings.Builder
	spoken.WriteString(l1)
	if len(decls) > 0 {
		spoken.WriteString(" Declares ")
		spoken.WriteString(strings.Join(decls, ", "))
		spoken.WriteString(".")
	}
	return spoken.String(), len(decls)
}

// levelCode — L1 = "a {N}-line {lang} code block" (deterministic, free).
// L2 = AI one-line semantic meaning-gist (needsIntelligence=true) for
// blocks at or under the codeGistMaxLines gate; degrade.go falls back to
// a deterministic count+decls gist when no adapter is plugged in. Over
// the gate, L2 voices the deterministic count+decls gist directly (no LLM
// call, Status=voiced). L3 = marks needsIntelligence=true; degrade.go
// downshifts to L1 if no intelligence is plugged in.
func levelCode(rb rawBlock, target plan.Level, lex *compiledLex) levelResult {
	body := stripFenceMarkers(rb.text)
	nLines := countLines(body)
	lang := strings.TrimSpace(rb.fenceInfo)
	langPhrase := codeLangPhrase(lang)
	l1 := fmt.Sprintf("A %d-line %s block.", nLines, langPhrase)

	facts := []string{
		fmt.Sprintf("class: code, lang: %s, lines: %d", lang, nLines),
	}

	switch target {
	case plan.L1:
		return levelResult{
			segments:      []plan.Segment{{ID: "s1", Kind: plan.SegmentKindSpeech, Text: l1}},
			provenance:    plan.Provenance{VoicedBy: "planner", Deterministic: true, LevelAsked: target},
			deterministic: true,
			facts:         facts,
		}
	case plan.L2:
		gist, declCount := deterministicCodeGist(body, langPhrase)
		facts = append(facts, fmt.Sprintf("decls: %d", declCount))
		if nLines > codeGistMaxLines {
			// Over the GATE — do NOT bill the LLM. Voice the deterministic
			// count+decls gist directly with Status=voiced. The gate is
			// observable only behaviorally (Status + Segments); no
			// size_gated fact is emitted.
			return levelResult{
				segments:      []plan.Segment{{ID: "s1", Kind: plan.SegmentKindSpeech, Text: voice(gist, lex)}},
				provenance:    plan.Provenance{VoicedBy: "planner", Deterministic: true, LevelAsked: target},
				deterministic: true,
				facts:         facts,
			}
		}
		// Under the gate — request an AI semantic gist. Empty segments
		// signal the orchestrator to call the adapter (or degrade.go to
		// fall back deterministically when none is plugged in).
		return levelResult{
			provenance:        plan.Provenance{LevelAsked: target},
			needsIntelligence: true,
			facts:             facts,
		}
	case plan.L3:
		// Line-by-line meaning needs intelligence.
		return levelResult{
			provenance:        plan.Provenance{LevelAsked: target},
			needsIntelligence: true,
			facts:             facts,
		}
	}
	return levelResult{}
}

// isIdentChar — identifier-rune predicate for declared-symbol scraping.
// Underscore + ASCII letter + ASCII digit. Kept separate so staticcheck
// doesn't dock the inline form for De Morgan's law applicability.
func isIdentChar(c rune) bool {
	if c == '_' {
		return true
	}
	if c >= 'a' && c <= 'z' {
		return true
	}
	if c >= 'A' && c <= 'Z' {
		return true
	}
	if c >= '0' && c <= '9' {
		return true
	}
	return false
}

func humanLang(lang string) string {
	switch strings.ToLower(lang) {
	case "go", "golang":
		return "Go"
	case "py", "python":
		return "Python"
	case "js", "javascript":
		return "JavaScript"
	case "ts", "typescript":
		return "TypeScript"
	case "sh", "bash", "zsh":
		return "shell"
	case "rs", "rust":
		return "Rust"
	default:
		return strings.ToUpper(lang[:1]) + lang[1:]
	}
}

func extractTopLevelDecls(body string) []string {
	var decls []string
	prefixes := []string{"func ", "type ", "class ", "def ", "function ", "interface ", "struct "}
	for ln := range strings.SplitSeq(body, "\n") {
		// Only top-level — no leading whitespace.
		if len(ln) == 0 || ln[0] == ' ' || ln[0] == '\t' {
			continue
		}
		for _, p := range prefixes {
			if rest, ok := strings.CutPrefix(ln, p); ok {
				name := strings.TrimSpace(rest)
				// Trim at first non-identifier char.
				cut := len(name)
				for i, c := range name {
					if !isIdentChar(c) {
						cut = i
						break
					}
				}
				if cut > 0 {
					decls = append(decls, name[:cut])
				}
				break
			}
		}
	}
	return decls
}

// levelConfig — L1 = "a {dialect} config block, {N} top-level keys".
// L2 = enumerate keys. L3 = read every key/value pair (voicing applied).
func levelConfig(rb rawBlock, target plan.Level, lex *compiledLex) levelResult {
	body := stripFenceMarkers(rb.text)
	dialect := configDialect(rb.fenceInfo, body)
	keys := extractTopLevelKeys(body, dialect)
	l1 := fmt.Sprintf("A %s config block, %d top-level keys.", dialect, len(keys))

	facts := []string{
		fmt.Sprintf("class: config, dialect: %s, top_level_keys: %d", dialect, len(keys)),
	}

	switch target {
	case plan.L1:
		return levelResult{
			segments:      []plan.Segment{{ID: "s1", Kind: plan.SegmentKindSpeech, Text: l1}},
			provenance:    plan.Provenance{VoicedBy: "planner", Deterministic: true, LevelAsked: target},
			deterministic: true,
			facts:         facts,
		}
	case plan.L2:
		var spoken strings.Builder
		spoken.WriteString(l1)
		if len(keys) > 0 {
			spoken.WriteString(" Keys: ")
			spoken.WriteString(strings.Join(keys, ", "))
			spoken.WriteString(".")
		}
		return levelResult{
			segments:      []plan.Segment{{ID: "s1", Kind: plan.SegmentKindSpeech, Text: voice(spoken.String(), lex)}},
			provenance:    plan.Provenance{VoicedBy: "planner", Deterministic: true, LevelAsked: target},
			deterministic: true,
			facts:         facts,
		}
	case plan.L3:
		pairs := extractKeyValuePairs(body, dialect)
		var spoken strings.Builder
		spoken.WriteString(l1)
		for _, kv := range pairs {
			spoken.WriteString(" ")
			spoken.WriteString(voice(kv, lex))
			spoken.WriteString(".")
		}
		return levelResult{
			segments:      []plan.Segment{{ID: "s1", Kind: plan.SegmentKindSpeech, Text: spoken.String()}},
			provenance:    plan.Provenance{VoicedBy: "planner", Deterministic: true, LevelAsked: target},
			deterministic: true,
			facts:         facts,
		}
	}
	return levelResult{}
}

func configDialect(fenceInfo, body string) string {
	switch strings.ToLower(strings.TrimSpace(fenceInfo)) {
	case "yaml", "yml":
		return "YAML"
	case "json":
		return "JSON"
	case "toml":
		return "TOML"
	case "ini", "properties", "env":
		return "INI"
	}
	trimmed := strings.TrimSpace(body)
	if strings.HasPrefix(trimmed, "{") || strings.HasPrefix(trimmed, "[") {
		if strings.Contains(trimmed, "\":") {
			return "JSON"
		}
	}
	if strings.HasPrefix(trimmed, "[") && strings.Contains(trimmed, "]\n") {
		return "TOML"
	}
	return "YAML"
}

func extractTopLevelKeys(body, dialect string) []string {
	var keys []string
	switch dialect {
	case "JSON":
		// Cheap top-level "key": pull on lines starting with whitespace +
		// quoted key + colon.
		for ln := range strings.SplitSeq(body, "\n") {
			trimmed := strings.TrimSpace(ln)
			if strings.HasPrefix(trimmed, "\"") {
				if i := strings.Index(trimmed[1:], "\""); i > 0 {
					key := trimmed[1 : 1+i]
					rest := strings.TrimSpace(trimmed[1+i+1:])
					if strings.HasPrefix(rest, ":") {
						keys = append(keys, key)
					}
				}
			}
		}
	case "TOML", "INI":
		for ln := range strings.SplitSeq(body, "\n") {
			trimmed := strings.TrimSpace(ln)
			if strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]") {
				keys = append(keys, strings.Trim(trimmed, "[]"))
			}
		}
	default: // YAML
		for ln := range strings.SplitSeq(body, "\n") {
			if isYAMLKeyLine(ln) {
				name := ln
				if i := strings.Index(name, ":"); i > 0 {
					name = name[:i]
				}
				keys = append(keys, strings.TrimSpace(name))
			}
		}
	}
	return keys
}

func extractKeyValuePairs(body, dialect string) []string {
	var pairs []string
	for ln := range strings.SplitSeq(body, "\n") {
		trimmed := strings.TrimSpace(ln)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		switch dialect {
		case "JSON":
			if strings.HasPrefix(trimmed, "\"") && strings.Contains(trimmed, ":") {
				pairs = append(pairs, kvHumanize(trimmed))
			}
		default:
			if strings.Contains(trimmed, ":") && !strings.HasPrefix(trimmed, "[") {
				pairs = append(pairs, kvHumanize(trimmed))
			} else if strings.Contains(trimmed, "=") {
				pairs = append(pairs, kvHumanize(trimmed))
			}
		}
	}
	return pairs
}

// kvHumanize — "replicas: 3" → "replicas set to 3".
func kvHumanize(line string) string {
	sep := ":"
	idx := strings.Index(line, sep)
	if idx < 0 {
		sep = "="
		idx = strings.Index(line, sep)
	}
	if idx < 0 {
		return line
	}
	k := strings.TrimSpace(line[:idx])
	v := strings.TrimSpace(strings.TrimSuffix(strings.TrimSuffix(line[idx+1:], ","), "\""))
	v = strings.TrimPrefix(v, "\"")
	if v == "" {
		return k
	}
	return k + " set to " + v
}

// levelTable — L1 = "a {C}-column, {R}-row table". L2 = headers + first/
// last rows. L3 = read every row meaningfully.
//
// Row counting: if row[1] is a separator (markdown's `|---|---|`), then
// row[0] is the header and data rows start at index 2. Otherwise every
// non-separator row is data.
func levelTable(rb rawBlock, target plan.Level, lex *compiledLex) levelResult {
	rows := parseTableRows(rb.text)
	cols := 0
	if len(rows) > 0 {
		cols = len(rows[0])
	}
	headerIdx := -1
	var dataRows [][]string
	if len(rows) >= 2 && isSeparatorRow(rows[1]) {
		headerIdx = 0
		dataRows = rows[2:]
	} else {
		for _, r := range rows {
			if isSeparatorRow(r) {
				continue
			}
			dataRows = append(dataRows, r)
		}
	}
	nRows := len(dataRows)

	l1 := fmt.Sprintf("A %d-column, %d-row table.", cols, nRows)
	facts := []string{fmt.Sprintf("class: table, cols: %d, rows: %d", cols, nRows)}

	switch target {
	case plan.L1:
		return levelResult{
			segments:      []plan.Segment{{ID: "s1", Kind: plan.SegmentKindSpeech, Text: l1}},
			provenance:    plan.Provenance{VoicedBy: "planner", Deterministic: true, LevelAsked: target},
			deterministic: true,
			facts:         facts,
		}
	case plan.L2:
		var spoken strings.Builder
		spoken.WriteString(l1)
		if headerIdx >= 0 {
			spoken.WriteString(" Headers: ")
			spoken.WriteString(strings.Join(rows[headerIdx], ", "))
			spoken.WriteString(".")
		}
		if nRows > 0 {
			spoken.WriteString(" First row: ")
			spoken.WriteString(strings.Join(dataRows[0], ", "))
			spoken.WriteString(".")
			if nRows > 1 {
				spoken.WriteString(" Last row: ")
				spoken.WriteString(strings.Join(dataRows[nRows-1], ", "))
				spoken.WriteString(".")
			}
		}
		return levelResult{
			segments:      []plan.Segment{{ID: "s1", Kind: plan.SegmentKindSpeech, Text: voice(spoken.String(), lex)}},
			provenance:    plan.Provenance{VoicedBy: "planner", Deterministic: true, LevelAsked: target},
			deterministic: true,
			facts:         facts,
		}
	case plan.L3:
		var spoken strings.Builder
		spoken.WriteString(l1)
		for _, r := range dataRows {
			spoken.WriteString(" Row: ")
			spoken.WriteString(strings.Join(r, ", "))
			spoken.WriteString(".")
		}
		return levelResult{
			segments:      []plan.Segment{{ID: "s1", Kind: plan.SegmentKindSpeech, Text: voice(spoken.String(), lex)}},
			provenance:    plan.Provenance{VoicedBy: "planner", Deterministic: true, LevelAsked: target},
			deterministic: true,
			facts:         facts,
		}
	}
	return levelResult{}
}

func parseTableRows(text string) [][]string {
	var rows [][]string
	for ln := range strings.SplitSeq(text, "\n") {
		trimmed := strings.TrimSpace(ln)
		if trimmed == "" {
			continue
		}
		// Strip leading/trailing pipe.
		s := strings.TrimPrefix(trimmed, "|")
		s = strings.TrimSuffix(s, "|")
		cells := strings.Split(s, "|")
		for i, c := range cells {
			cells[i] = strings.TrimSpace(c)
		}
		if len(cells) > 0 {
			rows = append(rows, cells)
		}
	}
	return rows
}

func isSeparatorRow(row []string) bool {
	for _, c := range row {
		stripped := c
		for _, r := range "-: " {
			stripped = strings.ReplaceAll(stripped, string(r), "")
		}
		if stripped != "" {
			return false
		}
	}
	return len(row) > 0
}

// levelDiagram — L1 = "a {dialect} diagram, {N} nodes". L2 = nodes + edges
// in meaning when cheaply derivable. L3 marks needsIntelligence.
func levelDiagram(rb rawBlock, target plan.Level, lex *compiledLex) levelResult {
	body := stripFenceMarkers(rb.text)
	dialect := diagramDialect(rb.fenceInfo, body)
	nodes := countMermaidNodes(body)
	l1 := fmt.Sprintf("A %s diagram, %d nodes.", dialect, nodes)
	facts := []string{fmt.Sprintf("class: diagram_as_text, dialect: %s, nodes: %d", dialect, nodes)}

	switch target {
	case plan.L1:
		return levelResult{
			segments:      []plan.Segment{{ID: "s1", Kind: plan.SegmentKindSpeech, Text: l1}},
			provenance:    plan.Provenance{VoicedBy: "planner", Deterministic: true, LevelAsked: target},
			deterministic: true,
			facts:         facts,
		}
	case plan.L2, plan.L3:
		// Beyond a structural gist, diagram traversal needs comprehension.
		_ = lex
		return levelResult{
			provenance:        plan.Provenance{LevelAsked: target},
			needsIntelligence: true,
			facts:             facts,
		}
	}
	return levelResult{}
}

func diagramDialect(fenceInfo, _ string) string {
	switch strings.ToLower(strings.TrimSpace(fenceInfo)) {
	case "mermaid":
		return "Mermaid"
	case "dot":
		return "DOT"
	case "plantuml":
		return "PlantUML"
	}
	return "text"
}

// countMermaidNodes — count distinct identifier tokens that look like
// Mermaid node references. Heuristic: split each non-empty line on
// whitespace and arrow tokens, then collect tokens that look like
// identifiers (alphanumeric).
func countMermaidNodes(body string) int {
	seen := map[string]bool{}
	arrowReps := []string{"-->", "---", "==>", "===", "-->>", "-->|", "|--"}
	for ln := range strings.SplitSeq(body, "\n") {
		s := strings.TrimSpace(ln)
		if s == "" || strings.HasPrefix(s, "graph ") || strings.HasPrefix(s, "flowchart ") ||
			strings.HasPrefix(s, "sequenceDiagram") || strings.HasPrefix(s, "classDiagram") {
			continue
		}
		for _, a := range arrowReps {
			s = strings.ReplaceAll(s, a, " ")
		}
		for tok := range strings.FieldsSeq(s) {
			id := trimToIdent(tok)
			if id != "" {
				seen[id] = true
			}
		}
	}
	return len(seen)
}

func trimToIdent(s string) string {
	start := 0
	for start < len(s) && !isIdentStart(s[start]) {
		start++
	}
	end := start
	for end < len(s) && (isIdentRune(s[end])) {
		end++
	}
	return s[start:end]
}

// levelProse — all levels require intelligence per design doc §4.
//
// degrade.go decides what happens when intel is nil: ≤120 words → read
// verbatim with Status=degraded; else refuse with RefuseNoIntelligence.
func levelProse(rb rawBlock, target plan.Level, _ *compiledLex) levelResult {
	words := len(strings.Fields(rb.text))
	return levelResult{
		provenance:        plan.Provenance{LevelAsked: target},
		needsIntelligence: true,
		facts: []string{
			fmt.Sprintf("class: prose, words: %d, lines: %d", words, countLines(rb.text)),
		},
	}
}

// ---------------------------------------------------------------------
// Oversized split
// ---------------------------------------------------------------------

// maybeSplit — return sub-rawBlocks if rb is over the per-class
// threshold AND a clean structural seam exists. Otherwise return nil.
//
// Diagrams are intentionally not split — they have no clean seam.
// Prose splits on sentence boundaries. Code splits on top-level decls.
// Config splits on top-level keys. Tables split on row boundaries with
// the header repeated.
func maybeSplit(rb rawBlock, cls plan.Class) []rawBlock {
	switch cls {
	case plan.ClassProse:
		if !proseOverThreshold(rb.text) {
			return nil
		}
		return splitProse(rb)
	case plan.ClassCode:
		if !structuredOverThreshold(rb.text) {
			return nil
		}
		return splitCode(rb)
	case plan.ClassConfig:
		if !structuredOverThreshold(rb.text) {
			return nil
		}
		return splitConfig(rb)
	case plan.ClassTable:
		if !structuredOverThreshold(rb.text) {
			return nil
		}
		return splitTable(rb)
	}
	return nil
}

func proseOverThreshold(s string) bool {
	return countLines(s) > proseMaxLines || len(s) > proseMaxChars
}

func structuredOverThreshold(s string) bool {
	return countLines(s) > structuredMaxLines || len(s) > structuredMaxChars
}

// splitProse — split on sentence boundaries (`. ! ?` followed by a
// space or newline). If no boundary exists, return nil (un-splittable —
// the orchestrator will hand the whole thing to leveling and let
// degrade.go refuse it if it's also too large to read verbatim).
func splitProse(rb rawBlock) []rawBlock {
	var chunks [][2]int // (start, end) byte offsets in rb.text
	start := 0
	for i := 0; i < len(rb.text); i++ {
		c := rb.text[i]
		if c == '.' || c == '!' || c == '?' {
			next := i + 1
			if next >= len(rb.text) || rb.text[next] == ' ' || rb.text[next] == '\n' {
				// Cut here if chunk is at or above target.
				if len(rb.text[start:next]) >= proseMaxChars/2 ||
					linesIn(rb.text[start:next]) >= proseMaxLines/2 {
					chunks = append(chunks, [2]int{start, next})
					start = next
					// Skip the trailing space/newline.
					for start < len(rb.text) && (rb.text[start] == ' ' || rb.text[start] == '\n') {
						start++
					}
				}
			}
		}
	}
	if start < len(rb.text) {
		chunks = append(chunks, [2]int{start, len(rb.text)})
	}
	if len(chunks) < 2 {
		return nil
	}
	return chunksToBlocks(chunks, rb)
}

// codeL2MaxWords — the hard word ceiling for a code-class L2 adapter
// reply. MANDATORY: this MUST track the "at most 30 words" literal in the
// CodeUserL2 prompt template (internal/intelligencetmpl/tmpl.go, the
// CodeUserL2 const). This comment is the only link between the prompt's
// advertised cap and the value enforced here — if the prompt literal
// changes, change this constant in lockstep.
const codeL2MaxWords = 30

// firstSentenceWithinCap enforces the code-L2 framing on an adapter reply:
// one sentence, at most maxWords words. It is a STANDALONE first-sentence
// scan — deliberately NOT routed through splitProse, which applies a
// proseMaxChars/2 size floor and returns nil for short multi-sentence
// input (that floor would defeat the trim, leaving the whole reply intact).
//
// Behavior:
//   - Scans left-to-right for the first qualifying sentence terminator
//     (`.`, `!`, or `?` immediately followed by whitespace or end-of-string)
//     and cuts there. Requiring whitespace/EOF after the terminator keeps
//     "v1.5.0" intact; the accepted tradeoff is that "e.g. foo" cuts early
//     at "e.g." — over-trimming is honest (still the adapter's own words),
//     and lower-harm than over-voicing.
//   - A terminator-less reply is treated as ONE candidate sentence (LLMs
//     routinely omit trailing punctuation); it is not the splitProse
//     "unsplittable → nil" path.
//   - The (trimmed) candidate is word-counted via strings.Fields. The
//     comparison is strict greater-than (len(fields) > maxWords), so a
//     reply of exactly maxWords words passes.
//
// Returns the trimmed candidate sentence and ok=true when it is voiceable
// under the cap. ok=false means "not voiceable under the cap → the caller
// MUST refuse"; it covers both the over-cap case AND empty/whitespace-only
// input (nothing voiceable). The capped string is empty when ok=false.
// Never truncates mid-sentence (that would be fabrication).
func firstSentenceWithinCap(text string, maxWords int) (capped string, ok bool) {
	// First-qualifying-terminator scan. No size floor.
	end := len(text)
	for i := 0; i < len(text); i++ {
		c := text[i]
		if c == '.' || c == '!' || c == '?' {
			next := i + 1
			if next >= len(text) || text[next] == ' ' || text[next] == '\n' || text[next] == '\t' || text[next] == '\r' {
				end = next
				break
			}
		}
	}
	candidate := strings.TrimSpace(text[:end])
	if candidate == "" {
		// Empty / whitespace-only (or a terminator with nothing before it):
		// nothing voiceable → caller refuses.
		return "", false
	}
	if len(strings.Fields(candidate)) > maxWords {
		return "", false
	}
	return candidate, true
}

// splitCode — split at lines beginning with a top-level decl keyword.
func splitCode(rb rawBlock) []rawBlock {
	return splitOnLineMatch(rb, func(line string) bool {
		if len(line) == 0 || line[0] == ' ' || line[0] == '\t' {
			return false
		}
		for _, p := range []string{"func ", "type ", "class ", "def ", "function ", "interface ", "struct "} {
			if strings.HasPrefix(line, p) {
				return true
			}
		}
		return false
	})
}

// splitConfig — split at top-level YAML keys.
func splitConfig(rb rawBlock) []rawBlock {
	return splitOnLineMatch(rb, func(line string) bool {
		return isYAMLKeyLine(line)
	})
}

// splitTable — split at row boundaries, repeating the header in each chunk.
func splitTable(rb rawBlock) []rawBlock {
	lines := strings.Split(rb.text, "\n")
	if len(lines) < 4 {
		return nil
	}
	header, sep := lines[0], lines[1]
	body := lines[2:]
	rowsPerChunk := max(structuredMaxLines-2, 4)
	var blocks []rawBlock
	lineNo := rb.startLine + 2
	for i := 0; i < len(body); i += rowsPerChunk {
		end := min(i+rowsPerChunk, len(body))
		chunk := strings.Join(append([]string{header, sep}, body[i:end]...), "\n")
		blocks = append(blocks, rawBlock{
			text:      chunk,
			startLine: rb.startLine,
			endLine:   lineNo + (end - i) - 1,
			hint:      hintTable,
		})
		lineNo += end - i
	}
	if len(blocks) < 2 {
		return nil
	}
	return blocks
}

// splitOnLineMatch — generic line-based splitter. seam(line) returns
// true at each clean seam; chunks are the text between seams.
func splitOnLineMatch(rb rawBlock, seam func(string) bool) []rawBlock {
	lines := strings.Split(rb.text, "\n")
	var starts []int
	for i, ln := range lines {
		if seam(ln) {
			starts = append(starts, i)
		}
	}
	if len(starts) < 2 {
		return nil
	}
	var blocks []rawBlock
	for i := 0; i < len(starts); i++ {
		from := starts[i]
		to := len(lines)
		if i+1 < len(starts) {
			to = starts[i+1]
		}
		chunk := strings.Join(lines[from:to], "\n")
		blocks = append(blocks, rawBlock{
			text:      chunk,
			startLine: rb.startLine + from,
			endLine:   rb.startLine + to - 1,
			hint:      rb.hint,
			fenceInfo: rb.fenceInfo,
		})
	}
	return blocks
}

func chunksToBlocks(chunks [][2]int, rb rawBlock) []rawBlock {
	var blocks []rawBlock
	lineCursor := rb.startLine
	for _, ch := range chunks {
		text := rb.text[ch[0]:ch[1]]
		nLines := countLines(text)
		if nLines == 0 {
			nLines = 1
		}
		blocks = append(blocks, rawBlock{
			text:      text,
			startLine: lineCursor,
			endLine:   lineCursor + nLines - 1,
			hint:      rb.hint,
		})
		lineCursor += nLines
	}
	return blocks
}

// ---------------------------------------------------------------------
// Small helpers
// ---------------------------------------------------------------------

func countLines(s string) int {
	if s == "" {
		return 0
	}
	n := 1
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' && i != len(s)-1 {
			n++
		}
	}
	return n
}

func linesIn(s string) int { return countLines(s) }

// stripFenceMarkers — drop the leading and trailing ``` lines from a
// fenced code block, leaving just the body. Idempotent on plaintext.
func stripFenceMarkers(s string) string {
	lines := strings.Split(s, "\n")
	if len(lines) == 0 {
		return s
	}
	if strings.HasPrefix(strings.TrimSpace(lines[0]), "```") {
		lines = lines[1:]
	}
	if len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) == "```" {
		lines = lines[:len(lines)-1]
	}
	return strings.Join(lines, "\n")
}
