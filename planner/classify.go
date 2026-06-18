package planner

import (
	"strings"

	"github.com/vd09-projects/intelligent-tts-narration-library/plan"
)

// classify — assign a plan.Class to a rawBlock using a deterministic
// priority chain. Each rule below decides or passes; the first matching
// rule wins.
//
// Decision: convention — sniff order is heading → list → image → table
// → fenced code (with info-string subtypes) → plaintext pipe-table →
// plaintext config sniff → plaintext ascii-diagram → prose default.
// Why: structural hints from the segmenter are stronger evidence than
// content sniffing, so AST-derived rules run first. Plaintext sniffs run
// only when no AST hint exists (segmenter fell back to plaintext or the
// hint is ambiguous). ClassExample is reserved for intelligence-driven
// reclassification in a later phase; phase-one classifier never emits it.
//
// The chain is documented inline rule-by-rule so a future maintainer can
// reorder with intent. Every rule has a matching test case in classify_test.go.
func classify(rb rawBlock) plan.Class {
	// 1. Heading hint from AST → heading.
	if rb.hint == hintHeading {
		return plan.ClassHeading
	}
	// 2. List hint from AST → list.
	if rb.hint == hintList {
		return plan.ClassList
	}
	// 3. Image-only paragraph → unknown (refused downstream by honesty rule).
	if rb.hint == hintImage {
		return plan.ClassUnknown
	}
	// 4. Table hint from AST → table.
	if rb.hint == hintTable {
		return plan.ClassTable
	}
	// 5. Fenced code → inspect info string for sub-classification.
	if rb.hint == hintCode {
		return classifyCodeFence(rb.fenceInfo)
	}
	// 6. Plaintext paragraph: try table-pipe sniff before prose.
	if rb.hint == hintProse || rb.hint == hintBlockHTML || rb.hint == hintUnknown {
		if looksLikePipeTable(rb.text) {
			return plan.ClassTable
		}
		// 7. Plaintext config sniff (YAML/JSON/TOML/INI shapes).
		if looksLikeConfig(rb.text) {
			return plan.ClassConfig
		}
		// 8. ASCII / box-drawing diagram sniff.
		if looksLikeASCIIDiagram(rb.text) {
			return plan.ClassDiagramAsText
		}
	}
	// 9. Default — prose.
	return plan.ClassProse
}

// classifyCodeFence — map a fence info string to a Class.
//
// Diagrams: mermaid, dot, plantuml.
// Config dialects: yaml, yml, toml, ini, json, properties, env.
// Everything else (including empty fence info) is plain code.
func classifyCodeFence(info string) plan.Class {
	switch strings.ToLower(strings.TrimSpace(info)) {
	case "mermaid", "dot", "plantuml":
		return plan.ClassDiagramAsText
	case "yaml", "yml", "toml", "ini", "json", "properties", "env":
		return plan.ClassConfig
	default:
		return plan.ClassCode
	}
}

// looksLikePipeTable — plaintext sniff for a markdown-style pipe table.
// Requires ≥2 lines that begin or end with '|' AND a separator line
// matching the `|---|` or `| --- |` shape.
func looksLikePipeTable(text string) bool {
	lines := strings.Split(strings.TrimRight(text, "\n"), "\n")
	if len(lines) < 2 {
		return false
	}
	pipeLines := 0
	separator := false
	for _, ln := range lines {
		trimmed := strings.TrimSpace(ln)
		if trimmed == "" {
			continue
		}
		if strings.HasPrefix(trimmed, "|") || strings.HasSuffix(trimmed, "|") {
			pipeLines++
		}
		// Separator: pipes around groups of dashes (with optional colons + spaces).
		if isPipeSeparatorLine(trimmed) {
			separator = true
		}
	}
	return pipeLines >= 2 && separator
}

// isPipeSeparatorLine — does the line match `|---|` or `| :---: | --- |`?
func isPipeSeparatorLine(line string) bool {
	if !strings.Contains(line, "|") || !strings.Contains(line, "-") {
		return false
	}
	// Stripped of pipes, spaces, dashes, colons — must be empty.
	stripped := line
	for _, r := range "|- :\t" {
		stripped = strings.ReplaceAll(stripped, string(r), "")
	}
	return stripped == ""
}

// looksLikeConfig — cheap structural sniffs for YAML / JSON / TOML / INI.
// Any one of the strong signals fires.
func looksLikeConfig(text string) bool {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return false
	}
	// JSON object/array.
	if strings.HasPrefix(trimmed, "{") || strings.HasPrefix(trimmed, "[") {
		// Must contain a quoted-key colon pattern to count.
		if strings.Contains(trimmed, "\":") || strings.Contains(trimmed, "\" :") {
			return true
		}
	}
	// TOML/INI section header.
	if strings.HasPrefix(trimmed, "[") && strings.Contains(trimmed, "]") {
		first := strings.SplitN(trimmed, "\n", 2)[0]
		if strings.HasSuffix(strings.TrimSpace(first), "]") {
			return true
		}
	}
	// YAML-shaped: ≥2 lines matching `key: value` at column 0.
	yamlKeyLines := 0
	for _, ln := range strings.Split(trimmed, "\n") {
		if isYAMLKeyLine(ln) {
			yamlKeyLines++
			if yamlKeyLines >= 2 {
				return true
			}
		}
	}
	return false
}

// isYAMLKeyLine — `^[a-zA-Z_][a-zA-Z0-9_-]*\s*:(\s|$)` with no leading
// whitespace (top-level key). Hand-rolled to avoid a regexp import.
func isYAMLKeyLine(line string) bool {
	if line == "" || line[0] == ' ' || line[0] == '\t' || line[0] == '#' {
		return false
	}
	colonAt := -1
	for i := 0; i < len(line); i++ {
		c := line[i]
		if i == 0 && !isIdentStart(c) {
			return false
		}
		if i > 0 && !isIdentRune(c) && c != ':' && c != ' ' && c != '\t' {
			return false
		}
		if c == ':' {
			colonAt = i
			break
		}
	}
	if colonAt <= 0 {
		return false
	}
	// After the colon: must be end-of-line or whitespace.
	rest := line[colonAt+1:]
	if rest == "" || rest == "\r" {
		return true
	}
	return rest[0] == ' ' || rest[0] == '\t'
}

// isIdentStart — first character of a YAML-ish key.
func isIdentStart(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || c == '_'
}

// isIdentRune — subsequent character of a YAML-ish key.
func isIdentRune(c byte) bool {
	return isIdentStart(c) || (c >= '0' && c <= '9') || c == '-'
}

// looksLikeASCIIDiagram — sniff for box-drawing or arrow-heavy ASCII art.
// Requires ≥3 lines containing at least two distinct diagram characters
// from the set { │ ─ ┌ ┐ └ ┘ ├ ┤ ┬ ┴ ┼ → ← ↑ ↓ | + - * }.
func looksLikeASCIIDiagram(text string) bool {
	diagramRunes := "│─┌┐└┘├┤┬┴┼→←↑↓"
	asciiAlt := "|+-*"
	lines := strings.Split(text, "\n")
	hits := 0
	for _, ln := range lines {
		if hasTwoDistinctRunes(ln, diagramRunes) || hasTwoDistinctRunes(ln, asciiAlt) {
			hits++
		}
		if hits >= 2 {
			return true
		}
	}
	return false
}

// hasTwoDistinctRunes — true when line contains ≥2 distinct runes drawn
// from the candidates set.
func hasTwoDistinctRunes(line, candidates string) bool {
	seen := make(map[rune]bool, 2)
	for _, r := range line {
		if strings.ContainsRune(candidates, r) {
			seen[r] = true
			if len(seen) >= 2 {
				return true
			}
		}
	}
	return false
}
