// Package plan defines the narration plan — the load-bearing JSON contract
// every part of the system produces or consumes.
//
// Invariants (CLAUDE.md):
//   - Zero dependencies. This package imports nothing from this project, and
//     only the Go standard library externally. Enforced by deps_test.go.
//   - Times encode as RFC3339 strings, not time.Time. PlanID is ULID.
//   - JSON tags are snake_case. Unknown fields are ignored (additive-
//     compatible within a major schema_version).
//   - Plan stays engine-neutral and pre-render. Audio offsets live in
//     Timeline (timeline.go), keyed by block_id. No word-level sync.
//   - Refusal is data, not an error. Block.Status == StatusRefused carries
//     a Refusal pointer; the plan still renders end-to-end.
package plan

import (
	"crypto/rand"
	"encoding/binary"
	"time"
)

// NarrationPlan — top-level plan artifact (design doc §2.1).
type NarrationPlan struct {
	SchemaVersion string       `json:"schema_version"`
	PlanID        string       `json:"plan_id"`
	CreatedAt     string       `json:"created_at"` // RFC3339 string, not time.Time.
	Source        SourceRef    `json:"source"`
	Defaults      PlanDefaults `json:"defaults"`
	Blocks        []Block      `json:"blocks"`
	Diagnostics   []Diagnostic `json:"diagnostics,omitempty"`
}

// SourceRef — what was narrated, at the document level (design doc §2.1).
// The core never opens URI; it is an opaque handle the adapter understands.
type SourceRef struct {
	Kind        SourceKind `json:"kind"`
	URI         string     `json:"uri,omitempty"`
	ContentHash string     `json:"content_hash"`
	Adapter     string     `json:"adapter"`
}

// PlanDefaults — requested level, engine-neutral voice hint, locale.
type PlanDefaults struct {
	Level  Level  `json:"level"`
	Voice  string `json:"voice,omitempty"`
	Locale string `json:"locale"`
}

// Block — the unit of classification, leveling, sync, and re-render
// (design doc §2.2). Independently regenerable: stable id, own source map,
// own level, own provenance.
//
// Invariant: Block carries no audio offsets / durations / sample rates /
// per-segment ms. Timing belongs in BlockTiming (timeline.go), keyed by ID.
// Enforced by reflection in plan_test.go.
type Block struct {
	ID         string     `json:"id"`
	Order      int        `json:"order"`
	Class      Class      `json:"class"`
	Level      Level      `json:"level"`
	Status     Status     `json:"status"`
	SourceMap  SourceMap  `json:"source_map"`
	Segments   []Segment  `json:"segments,omitempty"`
	SubBlocks  []Block    `json:"sub_blocks,omitempty"`
	Refusal    *Refusal   `json:"refusal,omitempty"` // present iff Status == StatusRefused.
	Provenance Provenance `json:"provenance"`
}

// Provenance — how this block's spoken text was produced (design doc §2.2).
type Provenance struct {
	VoicedBy      string `json:"voiced_by"` // "planner" | "intelligence" | "verbatim"
	Deterministic bool   `json:"deterministic"`
	Model         string `json:"model,omitempty"`
	LevelAsked    Level  `json:"level_asked,omitempty"`
}

// Segment — what the renderer actually speaks (design doc §2.3).
// Text is the final literal words to say (e.g. "replicas set to three").
// Voicing is an optional phonetic hint the renderer may ignore.
type Segment struct {
	ID      string             `json:"id"`
	Kind    SegmentKind        `json:"kind"`
	Text    string             `json:"text,omitempty"`
	Voicing []VoicingDirective `json:"voicing,omitempty"`
	PauseMs int                `json:"pause_ms,omitempty"` // for Kind == SegmentKindPause
}

// VoicingDirective — optional phonetic hint over a rune span of Segment.Text
// (design doc §2.3). Renderer may ignore every directive and still speak
// correct words.
type VoicingDirective struct {
	Start    int    `json:"start"` // rune offset into Segment.Text
	End      int    `json:"end"`
	SayAs    string `json:"say_as,omitempty"`    // "characters" | "digits" | "verbatim"
	Phoneme  string `json:"phoneme,omitempty"`   // engine-neutral pronunciation hint
	Emphasis string `json:"emphasis,omitempty"`  // "none" | "moderate" | "strong"
}

// SourceMap — one shape for files, screenshots, and raw text (design doc §2.4).
// Only Kind and which sub-fields are set change between adapter types.
//
// Invariant: no word/segment/char/rune-level fields — block-level sync only.
// (Char-span fields are block-level on the raw text; not per-word timing.)
type SourceMap struct {
	Kind       SourceKind `json:"kind"`
	StartLine  int        `json:"start_line,omitempty"`
	EndLine    int        `json:"end_line,omitempty"`
	StartChar  int        `json:"start_char,omitempty"`
	EndChar    int        `json:"end_char,omitempty"`
	Region     *Rect      `json:"region,omitempty"`
	RawExcerpt string     `json:"raw_excerpt,omitempty"`
}

// Rect — pixel rectangle for OCR / screenshot source maps.
type Rect struct {
	X int `json:"x"`
	Y int `json:"y"`
	W int `json:"w"`
	H int `json:"h"`
}

// Refusal — honesty rule as first-class data, not an error (design doc §2.5).
// Present on a Block iff Block.Status == StatusRefused. Spoken default true:
// the listener hears a short notice and gets block-level sync to the source.
type Refusal struct {
	Reason    RefusalReason `json:"reason"`
	Message   string        `json:"message"`
	Spoken    bool          `json:"spoken"`
	SourceMap SourceMap     `json:"source_map"`
}

// Diagnostic — plan-level note from the planner (e.g. "intelligence
// adapter unavailable; prose blocks under threshold read verbatim").
// Not surfaced as audio; consumers may render it in UI.
type Diagnostic struct {
	Code     string `json:"code"`
	Severity string `json:"severity"` // "info" | "warning"
	Message  string `json:"message"`
	BlockID  string `json:"block_id,omitempty"`
}

// ----------------------------------------------------------------------------
// ULID generator (design doc §2.1 — PlanID is ULID, 26 chars Crockford base32)
//
// **Decision (1) — schema: accepted.** Caller supplies PlanID; package exposes
// a single helper NewPlanID() using stdlib only. Why: the plan/ package must
// stay zero-dependency (invariant). A third-party ULID library would violate
// that, and the ULID spec is small enough to implement inline (6 bytes
// timestamp + 10 bytes randomness, Crockford base32). Monotonicity inside the
// same millisecond is deliberately not implemented; callers that need it can
// layer on a dependency. rand.Read failure panics rather than returning a
// sentinel — silent fabrication of a weak ID would violate the package's own
// honesty rule.
// ----------------------------------------------------------------------------

// crockfordAlphabet — ULID-canonical Crockford base32 alphabet, 32 chars.
// Excludes I, L, O, U to avoid ambiguity.
const crockfordAlphabet = "0123456789ABCDEFGHJKMNPQRSTVWXYZ"

// NewPlanID returns a new ULID as a 26-character Crockford-base32 string.
//
// Format (per ULID spec):
//   - 48 bits big-endian milliseconds since Unix epoch (10 chars base32)
//   - 80 bits cryptographically random entropy (16 chars base32)
//
// Stdlib only: time, crypto/rand, encoding/binary.
//
// Not monotonic: two IDs generated in the same millisecond have independent
// 80-bit random tails. Collision probability within the same ms is ~2^-40 per
// pair; acceptable for narration plans (one per document, not per request).
// If monotonicity is later needed, swap in github.com/oklog/ulid only in
// callers — plan/ stays zero-dep.
//
// crypto/rand failure: panics. Returning a sentinel ("") would silently
// fabricate a weak ID, violating the package's honesty-rule ethos. On
// darwin/linux rand.Read does not fail in practice; if it does, the system
// is unhealthy enough that aborting is the honest move.
func NewPlanID() string {
	var raw [16]byte

	// Timestamp: 48-bit ms big-endian into raw[0:6]. We write the full uint64
	// big-endian into a scratch buffer, then copy the low-order 6 bytes —
	// avoids the trap of writing past the 6-byte boundary into entropy land.
	var ts [8]byte
	ms := uint64(time.Now().UnixMilli()) // fits in 48 bits until year ~10889.
	binary.BigEndian.PutUint64(ts[:], ms)
	copy(raw[0:6], ts[2:8])

	// Entropy: 80 bits cryptographic randomness into raw[6:16].
	if _, err := rand.Read(raw[6:]); err != nil {
		panic("plan: crypto/rand failure generating PlanID: " + err.Error())
	}
	return encodeCrockford(raw)
}

// encodeCrockford converts 16 raw bytes (128 bits) into a 26-char ULID string.
// ULID layout: 128 bits → 26 base32 chars (130 bits encoded; top 2 bits zero).
func encodeCrockford(raw [16]byte) string {
	out := make([]byte, 26)

	// Pull a uint128 as two uint64s, high then low.
	hi := binary.BigEndian.Uint64(raw[0:8])
	lo := binary.BigEndian.Uint64(raw[8:16])

	// Emit characters from least significant 5-bit group up; reverse at end.
	// We process all 26 chars: 26 * 5 = 130 bits.
	for i := 25; i >= 0; i-- {
		out[i] = crockfordAlphabet[lo&0x1f]
		// Shift the 128-bit (hi:lo) right by 5 bits.
		lo = (lo >> 5) | (hi << 59)
		hi >>= 5
	}
	return string(out)
}
