// TypeScript mirror of `sink/persistent` Manifest (sink/persistent/manifest.go).
// Same JSON-on-wire shape; field names snake_case. Block-level timing only —
// no per-word/per-segment fields (CLAUDE.md: "Sync is block-level only").

import type { AudioFormat as _AudioFormat } from './audioFormat.ts'
import type { Block, Class, Level, Refusal, SourceKind, Status } from './plan.ts'

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

// BlockTiming mirrors plan.BlockTiming (plan/timeline.go) EXACTLY — the
// /escalate success body returns one of these as `timing`. Block-level only,
// snake_case on the wire (CLAUDE.md: "Sync is block-level only").
export interface BlockTiming {
  block_id: string
  start_ms: number
  end_ms: number
  audio_ref?: string
}

// EscalateReason is the closed, append-only server enum (cmd/narrate-server
// reason tokens) UNION the client-only 'unreachable' for fetch-reject/timeout/
// CORS. The client switch on these MUST carry a default branch so an unknown
// (newly-added server) token still renders a generic inline error rather than
// crashing (forward-compat).
export type EscalateReason =
  | 'invalid_level'
  | 'missing_field'
  | 'method_not_allowed'
  | 'source_not_found'
  | 'stale_patch'
  | 'content_hash_mismatch'
  | 'unknown_block'
  | 'container_mismatch'
  | 'format_mismatch'
  | 'cancelled'
  | 'readback_failed'
  | 'internal'
  | 'unreachable'
  // `(string & {})` keeps the named tokens in editor autocomplete while still
  // accepting any future server token (forward-compat). Trade-off: it widens the
  // type to `string`, so a `switch` over EscalateReason will NOT be flagged as
  // non-exhaustive if a named token is ever dropped — every consumer switch MUST
  // therefore carry an explicit `default` branch (see escalateClient mapping).
  | (string & {})

// EscalateSuccess / EscalateErrorBody mirror the two server response shapes
// (cmd/narrate-server escalateResponse + ErrorResponse). These are the raw
// wire types; escalateClient maps them into the discriminated EscalateResult.
export interface EscalateSuccess {
  block: Block
  timing?: BlockTiming
  audio_ref?: string
  refusal?: Refusal
}

export interface EscalateErrorBody {
  reason: EscalateReason
  message: string
}
