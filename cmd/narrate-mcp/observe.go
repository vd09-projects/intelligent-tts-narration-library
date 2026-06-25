// observe.go — the concrete Channel-2 observer writer (issue #81, ADR #77
// D5 Channel 2). The composition-root half of the per-block emit seam: the
// import-clean BlockObserver type lives in sink/ephemeral; the JSONL wire
// format, the ephemeral scratch-file lifecycle, and the env-driven opt-in
// live HERE, where the composition root is allowed to know concrete edges.
//
// What this gives a user that the after-the-fact Channel-1 receipt cannot:
// a live, block-by-block playback view in a SECOND terminal. The synchronous
// speak handler blocks on afplay per block, so its inline receipt is only
// assembled after the LAST block finishes. This writer appends one JSONL
// line per block to an ephemeral scratch file BEFORE each blocking play();
// cmd/narrate-observe tails it and renders progress while audio is playing.
//
// Design constraints honored here:
//   - Never breaks the listen path: a scratch open OR write failure emits at
//     most ONE STDERR line then goes silent — it never errors the speak call
//     (observability must not break playback).
//   - No source/spoken text on the wire (CLAUDE.md "local-only means secrets
//     get read aloud"): the event carries only structural metadata. The
//     scratch file is 0600 (owner-only) belt-and-suspenders.
//   - Ephemeral + per-call: the scratch file is NOT under the renderer's
//     outDir and is never teed into a durable sink.
//   - Decoupled: enabling the observer leaves the speak RESPONSE byte-
//     identical (the writer only touches the side file).
//
// Decision (v1) — channel-mechanism: accepted. Append-only JSONL + tail -f
// is the DEFAULT Channel-2 mechanism for #81. It beat native MCP
// notifications/progress (Session.NotifyProgress) for THIS surface: JSONL
// surfaces to a USER in a 2nd terminal (the D5 requirement), needs zero deps,
// stays ephemeral under /tmp with no durable coupling, and is live (emitted
// before each blocking play()). NotifyProgress surfaces to the MCP CLIENT,
// not a human terminal, and is invisible without client progressToken
// cooperation. Revisit when a real MCP-client UI wants progress — then add
// NotifyProgress ADDITIVELY alongside JSONL (same BlockEvent shape), not as a
// replacement.
//
// Decision (v2) — observer-seam-placement: accepted. The per-block Level +
// Status the observer renders are read by the sink from the plan.NarrationPlan
// it ALREADY receives in Consume (historically discarded), correlated 1:1 by
// BlockID. The originally-planned alternative — a cmd-side closure enriching
// from pipeline.BlockSummary — is unbuildable here: BlockSummaries only exist
// AFTER Narrate returns, but the observer must be wired into the sink BEFORE
// Narrate runs. Its stated justification (BlockSummary flattens oversized-split
// sub-blocks; the sink's plan.Blocks does not) does not apply phase one: no
// producer populates Block.SubBlocks, and render emits exactly one BlockTiming
// per top-level Block in plan order (render.go), so the sink-side lookup is
// correct AND live. The sink import wall is unaffected (plan/ already imported);
// guarded by sink/ephemeral/deps_test.go.
package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/vd09-projects/intelligent-tts-narration-library/sink/ephemeral"
)

// Channel-2 wire contract. The schema discriminator lets a future
// narrate.observe.* event (e.g. a terminal "done" line) coexist additively;
// the v field bumps only on a removal / meaning change (additive fields stay
// within v1, and readers ignore unknown fields).
const (
	observeSchemaName    = "narrate.observe.block"
	observeSchemaVersion = 1

	// envObserveFile names an EXACT scratch path (writer honors it verbatim;
	// no temp file). envObserve is the truthy auto-temp toggle. Precedence:
	// NARRATE_OBSERVE_FILE > NARRATE_OBSERVE (truthy → auto-temp) > off.
	envObserveFile = "NARRATE_OBSERVE_FILE"
	envObserve     = "NARRATE_OBSERVE"

	// observeTempDir is the DELIBERATE /tmp literal (not os.MkdirTemp("")'s
	// $TMPDIR) the writer and cmd/narrate-observe's newest-file glob agree on.
	// $TMPDIR on macOS is a long per-user /var/folders path that breaks a
	// fixed-root glob and reintroduces the unix-socket sun_path length
	// pressure that mechanism was rejected over. Keep both sides on /tmp; do
	// NOT "normalize" this to MkdirTemp("") or observer discovery breaks.
	observeTempDir     = "/tmp"
	observeTempPattern = "narrate-observe-*.jsonl"
)

// observeNow is the timestamp seam — the wall clock the event's ts field
// reads. A package var so tests can freeze it and assert exact JSONL bytes.
var observeNow = func() time.Time { return time.Now().UTC() }

// blockEvent is one JSONL line: one block, plan order, newline-terminated.
// Block-granularity only (no per-word fields — sync invariant). NO source or
// spoken-text field BY DESIGN (secret-leak avoidance — see file header).
type blockEvent struct {
	Schema            string `json:"schema"`
	V                 int    `json:"v"`
	BlockID           string `json:"block_id"`
	Order             int    `json:"order"`
	Total             int    `json:"total"`
	Level             int    `json:"level"`
	Status            string `json:"status"`
	PlannedDurationMs int64  `json:"planned_duration_ms"`
	Playing           bool   `json:"playing"`
	TS                string `json:"ts"`
}

// resolveObservePath applies the writer-side opt-in precedence and reports
// the scratch target. autoTemp == true means "create a temp file under
// observeTempDir"; a non-empty path means "honor this exact path". Both
// false/empty means observation is OFF.
//
// Precedence (S13): NARRATE_OBSERVE_FILE (explicit path) > NARRATE_OBSERVE
// (truthy → auto-temp) > off. The -f flag is the OBSERVER's (reader) concern,
// not the writer's.
func resolveObservePath() (path string, autoTemp bool) {
	if p := os.Getenv(envObserveFile); p != "" {
		return p, false
	}
	if isTruthy(os.Getenv(envObserve)) {
		return "", true
	}
	return "", false
}

// isTruthy reports whether an env value enables a toggle. On/true/yes/1
// (case-insensitive) → true; everything else (incl. "", "0", "false") → off.
func isTruthy(v string) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

// newBlockObserver resolves the opt-in, opens the scratch file, and returns
// the observer the ephemeral sink calls per block plus a close func to defer.
// When observation is OFF — or the scratch file cannot be opened — it returns
// (nil, no-op close): a nil observer is the sink's documented off state, so
// the speak path stays byte-identical and never errors.
//
// On a successful open it announces the path on STDERR so the user can point
// `narrate-observe -f <path>` at it (or rely on the newest-file glob).
func newBlockObserver(stderr io.Writer) (ephemeral.BlockObserver, func()) {
	noop := func() {}

	path, autoTemp := resolveObservePath()
	var (
		f   *os.File
		err error
	)
	switch {
	case autoTemp:
		f, err = os.CreateTemp(observeTempDir, observeTempPattern) // CreateTemp opens 0600.
	case path != "":
		f, err = os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	default:
		return nil, noop // off
	}
	if err != nil {
		// Open failure (e.g. a bad NARRATE_OBSERVE_FILE path): one STDERR
		// line, then disabled. NEVER propagated to the speak call.
		_, _ = fmt.Fprintf(stderr, "narrate-observe: disabled (%v)\n", err)
		return nil, noop
	}

	_, _ = fmt.Fprintf(stderr, "narrate-observe: writing %s\n", f.Name())

	// once bounds write-failure noise to a single STDERR line for the whole
	// call (no per-block log spam, no perf hit on the playback path).
	var once sync.Once
	obs := func(p ephemeral.BlockProgress) {
		line, mErr := marshalBlockEvent(p)
		if mErr != nil {
			// Marshaling our own well-typed struct should never fail; if it
			// somehow does, skip this line silently — never break playback.
			return
		}
		if _, wErr := f.Write(line); wErr != nil {
			once.Do(func() {
				_, _ = fmt.Fprintf(stderr, "narrate-observe: disabled (%v)\n", wErr)
			})
		}
	}
	return obs, func() { _ = f.Close() }
}

// marshalBlockEvent renders one BlockProgress into a single newline-
// terminated JSONL line. The newline is appended to the marshaled bytes so
// the caller issues ONE f.Write per line — a tail reader never observes a
// line without its terminator (partial-line atomicity).
func marshalBlockEvent(p ephemeral.BlockProgress) ([]byte, error) {
	ev := blockEvent{
		Schema:            observeSchemaName,
		V:                 observeSchemaVersion,
		BlockID:           p.Timing.BlockID,
		Order:             p.Order,
		Total:             p.Total,
		Level:             int(p.Level),
		Status:            string(p.Status),
		PlannedDurationMs: int64(p.Timing.EndMs - p.Timing.StartMs),
		Playing:           p.Playing,
		TS:                observeNow().Format(time.RFC3339Nano),
	}
	b, err := json.Marshal(ev)
	if err != nil {
		return nil, err
	}
	return append(b, '\n'), nil
}
