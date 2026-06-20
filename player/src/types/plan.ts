// TypeScript mirror of the Go `plan` package schema (plan/plan.go +
// plan/enums.go). Field names follow the JSON-on-wire snake_case, NOT the Go
// PascalCase. Unknown fields/values round-trip — additive-compatible within
// a major schema_version (CLAUDE.md: "Schema versioning").

export type Level = 1 | 2 | 3

export type Class =
  | 'prose'
  | 'code'
  | 'config'
  | 'table'
  | 'diagram_as_text'
  | 'example'
  | 'heading'
  | 'list'
  | 'unknown'
  | (string & {})

export type Status = 'voiced' | 'degraded' | 'refused' | (string & {})

export type RefusalReason =
  | 'bare_image_no_description'
  | 'too_large_for_level'
  | 'too_raw_to_voice'
  | 'no_intelligence_available'
  | 'unsupported_content'
  | (string & {})

export type SourceKind =
  | 'file'
  | 'mcp_text'
  | 'ocr_screenshot'
  | 'raw_text'
  | 'line_range'
  | 'char_span'
  | 'pixel_region'
  | (string & {})

export type SegmentKind = 'speech' | 'pause' | 'earcon' | (string & {})

export type Severity = 'info' | 'warning' | (string & {})

export type SayAs = 'characters' | 'digits' | 'verbatim' | (string & {})

export type Emphasis = 'none' | 'moderate' | 'strong' | (string & {})

export interface Rect {
  x: number
  y: number
  w: number
  h: number
}

export interface SourceMap {
  kind: SourceKind
  start_line?: number
  end_line?: number
  start_char?: number
  end_char?: number
  region?: Rect
  raw_excerpt?: string
}

export interface Refusal {
  reason: RefusalReason
  message: string
  spoken: boolean
  source_map: SourceMap
}

export interface VoicingDirective {
  start: number
  end: number
  say_as?: SayAs
  phoneme?: string
  emphasis?: Emphasis
}

export interface Segment {
  id: string
  kind: SegmentKind
  text?: string
  voicing?: VoicingDirective[]
  pause_ms?: number
}

export interface Provenance {
  voiced_by: string
  deterministic: boolean
  model?: string
  level_asked?: Level
}

export interface Block {
  id: string
  order: number
  class: Class
  level: Level
  status: Status
  source_map: SourceMap
  segments?: Segment[]
  sub_blocks?: Block[]
  refusal?: Refusal
  provenance: Provenance
}

export interface SourceRef {
  kind: SourceKind
  uri?: string
  content_hash: string
  adapter: string
}

export interface PlanDefaults {
  level: Level
  voice?: string
  locale: string
}

export interface Diagnostic {
  code: string
  severity: Severity
  message: string
  block_id?: string
}

export interface NarrationPlan {
  schema_version: string
  plan_id: string
  created_at: string
  source: SourceRef
  defaults: PlanDefaults
  blocks: Block[]
  diagnostics?: Diagnostic[]
}
