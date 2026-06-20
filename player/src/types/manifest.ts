// TypeScript mirror of `sink/persistent` Manifest (sink/persistent/manifest.go).
// Same JSON-on-wire shape; field names snake_case. Block-level timing only —
// no per-word/per-segment fields (CLAUDE.md: "Sync is block-level only").

import type { AudioFormat as _AudioFormat } from './audioFormat.ts'
import type { Class, Level, SourceKind, Status } from './plan.ts'

export interface ManifestBlock {
  id: string
  class: Class
  level: Level
  status: Status
  start_ms: number
  end_ms: number
  audio_ref?: string
}

export interface Manifest {
  schema_version: number
  plan_schema_version: string
  source_kind: SourceKind
  source_uri: string
  content_hash: string
  audio_format: _AudioFormat
  voice: string
  blocks: ManifestBlock[]
  stale: boolean
  stale_reason?: string
}
