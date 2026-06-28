// types.ts — the narrate-server (#109) wire contract, pinned in ONE place (D4).
//
// All server-shape knowledge lives here. Components and hooks import these view
// models; they never re-derive a shape from a URL or a path. Two load-bearing
// rules from the plan:
//
//   1. `audio_url` is OPAQUE (D4). It is fed verbatim to <audio src>. The client
//      never parses out a render_id and never rebuilds `/audio/{id}.wav`. A
//      change to the server's audio path format therefore cannot break Earshot.
//
//   2. `message_ref` is the SINGLE mock<->live divergence point (D4). The shape
//      below is ASSUMED from docs/earshot-design.md §5 and is NOT yet confirmed
//      against a running #109. When #109 confirms the real shape this is a
//      one-line change here and nowhere else. See MessageRef below.
//
// These types mirror the Go JSON shapes verbatim (snake_case tags from plan/ and
// cmd/narrate-server). They are additive-compatible: unknown fields are ignored,
// unknown enum values round-trip (CLAUDE.md schema-versioning rule), so the
// string-union enums below are deliberately open via the `| (string & {})`
// escape hatch where a server may add a value we do not yet render.

// ---------------------------------------------------------------------------
// Enums (mirror plan/enums.go). Open unions: an unknown value must not be a type
// error, it renders as a neutral fallback (additive-compatible).
// ---------------------------------------------------------------------------

/** Per-block requested fidelity. 1 = gist, 2 = summary, 3 = detail. */
export type Level = 1 | 2 | 3;

/** Outcome of planning a block (plan/enums.go Status). */
export type Status = "voiced" | "degraded" | "refused";

/** Block classification (plan/enums.go Class). Open union. */
export type BlockClass =
  | "prose"
  | "code"
  | "config"
  | "table"
  | "diagram_as_text"
  | "example"
  | "heading"
  | "list"
  | "unknown"
  | (string & {});

/** Why a block was refused (plan/enums.go RefusalReason). Open union. */
export type RefusalReason =
  | "bare_image_no_description"
  | "too_large_for_level"
  | "too_raw_to_voice"
  | "no_intelligence_available"
  | "unsupported_content"
  | (string & {});

/** How spoken text was produced (plan/enums.go VoicedBy). Open union. */
export type VoicedBy = "planner" | "intelligence" | "verbatim" | (string & {});

/** Segment kind (plan/enums.go SegmentKind). Open union. */
export type SegmentKind = "speech" | "pause" | "earcon" | (string & {});

/** Coarse structural class on a pre-chunked session block (messages.go). */
export type SessionBlockClass = "prose" | "code" | (string & {});

// ---------------------------------------------------------------------------
// plan/ shapes (the engine-neutral plan schema the server passes through).
// ---------------------------------------------------------------------------

/** Block-level provenance (plan.SourceMap). */
export interface SourceMap {
  kind: string;
  start_line?: number;
  end_line?: number;
  start_char?: number;
  end_char?: number;
  raw_excerpt?: string;
}

/** A spoken (or pause/earcon) segment of a block (plan.Segment). */
export interface Segment {
  id: string;
  kind: SegmentKind;
  /** Final spoken words — already voiced by the planner (Segment.Text). */
  text?: string;
  pause_ms?: number;
}

/** Honesty-rule refusal carried as data, never as an error (plan.Refusal). */
export interface Refusal {
  reason: RefusalReason;
  message: string;
  spoken: boolean;
  source_map: SourceMap;
}

/** How a block's spoken text was produced (plan.Provenance). */
export interface Provenance {
  voiced_by: VoicedBy;
  deterministic: boolean;
  model?: string;
  level_asked?: Level;
}

/** One planned block (plan.Block). */
export interface Block {
  id: string;
  order: number;
  class: BlockClass;
  level: Level;
  status: Status;
  source_map: SourceMap;
  segments?: Segment[];
  sub_blocks?: Block[];
  /** Present iff status === "refused". */
  refusal?: Refusal;
  provenance: Provenance;
}

/** Audio format descriptor (plan.AudioFormat). */
export interface AudioFormat {
  sample_rate: number;
  channels: number;
  encoding: string;
}

/** Block-level audio sync entry (plan.BlockTiming). No word-level timing. */
export interface BlockTiming {
  block_id: string;
  start_ms: number;
  end_ms: number;
  audio_ref?: string;
}

/** Block-level audio sync (plan.Timeline), keyed by block_id. */
export interface Timeline {
  plan_id: string;
  format: AudioFormat;
  blocks: BlockTiming[];
}

// ---------------------------------------------------------------------------
// narrate-server (#109) request/response shapes.
// ---------------------------------------------------------------------------

/** GET /sessions/{id}/messages — one display block (messages.go sessionBlock). */
export interface SessionBlock {
  text: string;
  class: SessionBlockClass;
}

/** GET /sessions/{id}/messages — one message (messages.go sessionMessage). */
export interface SessionMessage {
  id: string;
  role: string;
  turn: number;
  blocks: SessionBlock[];
}

/** GET /sessions/{id}/messages 200 body (messages.go messagesResponse). */
export interface MessagesResponse {
  session_id: string;
  messages: SessionMessage[];
}

/**
 * D4 DIVERGENCE POINT — the assumed `message_ref` shape for POST /narrate.
 *
 * docs/earshot-design.md §5 documents `POST /narrate {text | message_ref, ...}`,
 * but the LIVE #109 narrate.go currently accepts inline `text` ONLY (R2: no
 * server-side source/ref path yet). So this client narrates by `text` today
 * (assembled from the selected message's blocks). `MessageRef` is reserved for
 * when #109 grows the by-reference path; confirmed at the Phase 6 live /verify.
 *
 * THIS INTERFACE IS THE ONLY THING THAT CHANGES when the real shape lands.
 */
export interface MessageRef {
  session_id: string;
  message_id: string;
}

/** Voice gender selector (MCP `gender` arg). Defaults to female server-side. */
export type Gender = "female" | "male";

/** POST /narrate request body (narrate.go narrateRequest). */
export interface NarrateRequest {
  text: string;
  level: Level;
  gender?: Gender;
}

/**
 * POST /narrate and POST /narrate/file 200 body (narrate.go narrateResponse).
 * `audio_url` is OPAQUE (D4): used verbatim as the <audio> source.
 *
 * `render_id` (#113) is the bare render id the server ALSO encodes inside
 * audio_url. The client uses THIS to drive POST /narrate/block — it never
 * parses render_id back out of the opaque audio_url (D4 preserved).
 */
export interface NarrateResponse {
  audio_url: string;
  render_id: string;
  blocks: Block[];
  timeline: Timeline;
}

/**
 * POST /narrate/block request body (narrate.go narrateBlockRequest, #110/#113).
 * render_id keys the server-internal render dir; block_id + level select the one
 * block to re-render at a new fidelity.
 */
export interface NarrateBlockRequest {
  render_id: string;
  block_id: string;
  level: Level;
}

/**
 * POST /narrate/block 200 body (narrate.go narrateBlockResponse, #110/#113).
 *
 * `block` carries the re-rendered block (status reflects voiced/degraded, or
 * refused on an escalation-returned refusal — refusal is DATA, not an error).
 * `timing` + `refusal` follow the same nullability as /escalate. `audio_url`
 * is the SAME opaque stable URL (the combined wav is rewritten in place).
 *
 * `timeline` (#113) is the FULL post-patch document timeline — the patch shifts
 * every downstream sibling's offset, so the client adopts this WHOLESALE
 * (server-authoritative; no client-side offset recompute).
 */
export interface NarrateBlockResponse {
  block: Block;
  timing?: BlockTiming;
  refusal?: Refusal;
  audio_url: string;
  timeline: Timeline;
}

/** Normalized error body (cmd/narrate-server ErrorResponse {reason, message}). */
export interface ServerError {
  reason: string;
  message: string;
}
