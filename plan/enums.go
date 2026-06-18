// Package plan — enum types for the narration plan schema.
//
// Every enum type carries an IsValid() bool used by consumers (planner,
// adapters, sinks) to validate plan data without reaching into this
// package's internals. Unknown values round-trip through JSON (additive-
// compatible: consumers ignore unknown fields and unknown values).
package plan

// Level — per-block requested fidelity. L1 = gist, L2 = summary, L3 = detail.
// Modeled as int so JSON encodes 1/2/3 (matches design doc §2.1 examples).
type Level int

const (
	// L1 gist (1).
	L1 Level = 1
	// L2 summary (2).
	L2 Level = 2
	// L3 detail (3).
	L3 Level = 3
)

// IsValid reports whether l is one of L1, L2, L3.
func (l Level) IsValid() bool {
	return l == L1 || l == L2 || l == L3
}

// Class — block classification produced by the planner segmenter.
// Drives per-class leveling rules (CLAUDE.md domain rules).
type Class string

const (
	// ClassProse — running prose, narrative text.
	ClassProse Class = "prose"
	// ClassCode — source code block.
	ClassCode Class = "code"
	// ClassConfig — config (YAML/TOML/INI/JSON).
	ClassConfig Class = "config"
	// ClassTable — tabular data.
	ClassTable Class = "table"
	// ClassDiagramAsText — Mermaid, ASCII diagram, chart-as-YAML, etc.
	ClassDiagramAsText Class = "diagram_as_text"
	// ClassExample — worked example, sample I/O.
	ClassExample Class = "example"
	// ClassHeading — section heading.
	ClassHeading Class = "heading"
	// ClassList — bullet or numbered list.
	ClassList Class = "list"
	// ClassUnknown — unclassified (e.g. bare image, opaque region).
	ClassUnknown Class = "unknown"
)

// IsValid reports whether c is a recognized Class value.
func (c Class) IsValid() bool {
	switch c {
	case ClassProse, ClassCode, ClassConfig, ClassTable,
		ClassDiagramAsText, ClassExample, ClassHeading,
		ClassList, ClassUnknown:
		return true
	}
	return false
}

// Status — outcome of planning a block.
//
//   - voiced   — level fully met.
//   - degraded — voiced at lower fidelity than requested (e.g. prose read
//     verbatim instead of gisted because no intelligence adapter).
//   - refused  — not voiced; refusal carries reason + source map.
type Status string

const (
	// StatusVoiced — block voiced at the requested level.
	StatusVoiced Status = "voiced"
	// StatusDegraded — voiced at lower fidelity than requested.
	StatusDegraded Status = "degraded"
	// StatusRefused — not voiced; honesty-rule refusal.
	StatusRefused Status = "refused"
)

// IsValid reports whether s is a recognized Status value.
func (s Status) IsValid() bool {
	switch s {
	case StatusVoiced, StatusDegraded, StatusRefused:
		return true
	}
	return false
}

// RefusalReason — why a block was refused (honesty rule, design doc §2.5).
// Stored as string for JSON portability; consumers must tolerate unknown
// reasons (additive-compatible).
type RefusalReason string

const (
	// RefuseBareImage — image without a description.
	RefuseBareImage RefusalReason = "bare_image_no_description"
	// RefuseTooLarge — cannot faithfully fit the requested level.
	RefuseTooLarge RefusalReason = "too_large_for_level"
	// RefuseTooRaw — unparseable / noise.
	RefuseTooRaw RefusalReason = "too_raw_to_voice"
	// RefuseNoIntelligence — prose gist needs an adapter that wasn't supplied.
	RefuseNoIntelligence RefusalReason = "no_intelligence_available"
	// RefuseUnsupported — e.g. unknown diagram dialect.
	RefuseUnsupported RefusalReason = "unsupported_content"
)

// IsValid reports whether r is one of the five known refusal reasons.
func (r RefusalReason) IsValid() bool {
	switch r {
	case RefuseBareImage, RefuseTooLarge, RefuseTooRaw,
		RefuseNoIntelligence, RefuseUnsupported:
		return true
	}
	return false
}

// SourceKind — discriminator for both SourceRef.Kind (doc-level provenance)
// and SourceMap.Kind (block-level provenance). Same set of values keeps the
// schema "one shape for files, screenshots, and raw text" (design doc §2.4).
type SourceKind string

const (
	// SourceKindFile — file on disk; URI is a path.
	SourceKindFile SourceKind = "file"
	// SourceKindMCPText — text supplied through an MCP tool call.
	SourceKindMCPText SourceKind = "mcp_text"
	// SourceKindOCRScreenshot — screenshot run through OCR.
	SourceKindOCRScreenshot SourceKind = "ocr_screenshot"
	// SourceKindRawText — inline text payload.
	SourceKindRawText SourceKind = "raw_text"
	// SourceKindLineRange — block-level: line-number span into a text source.
	SourceKindLineRange SourceKind = "line_range"
	// SourceKindCharSpan — block-level: character span (raw text / MCP).
	SourceKindCharSpan SourceKind = "char_span"
	// SourceKindPixelRegion — block-level: pixel rect on a screenshot.
	SourceKindPixelRegion SourceKind = "pixel_region"
)

// IsValid reports whether k is a recognized SourceKind value.
func (k SourceKind) IsValid() bool {
	switch k {
	case SourceKindFile, SourceKindMCPText, SourceKindOCRScreenshot,
		SourceKindRawText, SourceKindLineRange, SourceKindCharSpan,
		SourceKindPixelRegion:
		return true
	}
	return false
}

// SegmentKind — what kind of thing the renderer should emit for a segment.
type SegmentKind string

const (
	// SegmentKindSpeech — spoken text.
	SegmentKindSpeech SegmentKind = "speech"
	// SegmentKindPause — silent gap of PauseMs milliseconds.
	SegmentKindPause SegmentKind = "pause"
	// SegmentKindEarcon — short branded audio cue (e.g. section change).
	SegmentKindEarcon SegmentKind = "earcon"
)

// IsValid reports whether k is a recognized SegmentKind value.
func (k SegmentKind) IsValid() bool {
	switch k {
	case SegmentKindSpeech, SegmentKindPause, SegmentKindEarcon:
		return true
	}
	return false
}
