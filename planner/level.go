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

// levelList — voice "List of N items: item 1. item 2. …" at all levels.
//
// Items are extracted by stripping markdown list markers ("-", "*", "+",
// or digit-period) from each line. Indented continuation lines fold into
// their parent item.
func levelList(rb rawBlock, target plan.Level, lex *compiledLex) levelResult {
	items := extractListItems(rb.text)
	var spoken strings.Builder
	fmt.Fprintf(&spoken, "List of %d items: ", len(items))
	for i, item := range items {
		spoken.WriteString(voice(item, lex))
		if i < len(items)-1 {
			spoken.WriteString(". ")
		}
	}
	spoken.WriteString(".")
	return levelResult{
		segments: []plan.Segment{
			{ID: "s1", Kind: plan.SegmentKindSpeech, Text: spoken.String()},
		},
		provenance:    plan.Provenance{VoicedBy: "planner", Deterministic: true, LevelAsked: target},
		deterministic: true,
		facts:         []string{fmt.Sprintf("class: list, items: %d", len(items))},
	}
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

// levelCode — L1 = "a {N}-line {lang} code block". L2 = same plus enumerated
// top-level declarations. L3 = marks needsIntelligence=true; degrade.go
// downshifts to L2 if no intelligence is plugged in.
func levelCode(rb rawBlock, target plan.Level, lex *compiledLex) levelResult {
	body := stripFenceMarkers(rb.text)
	nLines := countLines(body)
	lang := strings.TrimSpace(rb.fenceInfo)
	langPhrase := "code"
	if lang != "" {
		langPhrase = humanLang(lang) + " code"
	}
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
		decls := extractTopLevelDecls(body)
		var spoken strings.Builder
		spoken.WriteString(l1)
		if len(decls) > 0 {
			spoken.WriteString(" Declares ")
			spoken.WriteString(strings.Join(decls, ", "))
			spoken.WriteString(".")
		}
		facts = append(facts, fmt.Sprintf("decls: %d", len(decls)))
		return levelResult{
			segments:      []plan.Segment{{ID: "s1", Kind: plan.SegmentKindSpeech, Text: voice(spoken.String(), lex)}},
			provenance:    plan.Provenance{VoicedBy: "planner", Deterministic: true, LevelAsked: target},
			deterministic: true,
			facts:         facts,
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
	for _, ln := range strings.Split(body, "\n") {
		// Only top-level — no leading whitespace.
		if len(ln) == 0 || ln[0] == ' ' || ln[0] == '\t' {
			continue
		}
		for _, p := range prefixes {
			if strings.HasPrefix(ln, p) {
				name := strings.TrimPrefix(ln, p)
				name = strings.TrimSpace(name)
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
		for _, ln := range strings.Split(body, "\n") {
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
		for _, ln := range strings.Split(body, "\n") {
			trimmed := strings.TrimSpace(ln)
			if strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]") {
				keys = append(keys, strings.Trim(trimmed, "[]"))
			}
		}
	default: // YAML
		for _, ln := range strings.Split(body, "\n") {
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
	for _, ln := range strings.Split(body, "\n") {
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
	for _, ln := range strings.Split(text, "\n") {
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
	for _, ln := range strings.Split(body, "\n") {
		s := strings.TrimSpace(ln)
		if s == "" || strings.HasPrefix(s, "graph ") || strings.HasPrefix(s, "flowchart ") ||
			strings.HasPrefix(s, "sequenceDiagram") || strings.HasPrefix(s, "classDiagram") {
			continue
		}
		for _, a := range arrowReps {
			s = strings.ReplaceAll(s, a, " ")
		}
		for _, tok := range strings.Fields(s) {
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
	rowsPerChunk := structuredMaxLines - 2
	if rowsPerChunk < 4 {
		rowsPerChunk = 4
	}
	var blocks []rawBlock
	lineNo := rb.startLine + 2
	for i := 0; i < len(body); i += rowsPerChunk {
		end := i + rowsPerChunk
		if end > len(body) {
			end = len(body)
		}
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
