package persistent

// wavfile.go — the single-wav OutputSink (issue #105).
//
// WAVFileSink writes EXACTLY ONE combined .wav (the same per-block wav-concat
// math the 3-file persistent Sink uses) at a caller-given path, and writes NO
// plan.json / manifest.json sidecars. It is the durable single-blob writer the
// MCP speak_to_file tool wires when the caller supplies an output_path.
//
// DEBT (package-name honesty): a wav-only, NON-persisting sink now lives in
// package `persistent`, whose name overstates its contents. Accepted for
// minimal churn this cycle. Trigger-on-objection follow-up: hoist the shared
// wav-concat core (writeCombinedWAV / wavSegment / silenceBytes / readWAV /
// buildSegments) into an internal/wavconcat package imported by both this sink
// and the persistent Sink.

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/vd09-projects/intelligent-tts-narration-library/plan"
	"github.com/vd09-projects/intelligent-tts-narration-library/render"
	"github.com/vd09-projects/intelligent-tts-narration-library/sink"
)

// ErrNoWAVPath signals that Consume was called on a WAVFileSink with an empty
// Path. NewWAVFile takes the path positionally so the omission is normally a
// caller bug caught up front; this sentinel is the final guard mirroring
// ErrNoOutDir on the persistent Sink.
var ErrNoWAVPath = errors.New("sink/persistent: WAVFileSink Path is empty")

// WAVFileSink is an OutputSink that concatenates the renderer's per-block WAVs
// into a single audio blob and writes it — and only it — at Path. No plan.json,
// no manifest.json (issue #105 AC5). The combined-wav bytes are byte-identical
// to what the 3-file persistent Sink would write for the same RenderResult,
// because both share buildSegments + writeAudio.
//
// Field semantics:
//   - Path — destination .wav file. Its parent directory is created with 0o755
//     if missing. An existing file at Path is overwritten in place (atomic
//     tmp+rename adjacent to Path, so the rename is intra-directory and cannot
//     hit cross-device EXDEV).
//   - ExpectedFormat — the AudioFormat readWAV validates per-block WAVs against.
//     Defaults to render.DefaultFormat() (24 kHz mono s16le).
type WAVFileSink struct {
	Path           string
	ExpectedFormat plan.AudioFormat
}

// Compile-time assertion: WAVFileSink implements sink.OutputSink.
var _ sink.OutputSink = (*WAVFileSink)(nil)

// NewWAVFile constructs a WAVFileSink writing to path. path is positional
// because it is mandatory — making it an option would let callers silently
// omit it (same rationale as persistent.New's outDir, Decision v1.3.0).
//
// It reuses the existing persistent Option set (WithExpectedFormat, WithVoice)
// by applying them to a throwaway *Sink and copying the only field this sink
// cares about (ExpectedFormat). WithVoice is therefore a harmless no-op here —
// a wav-only sink writes no manifest to carry a voice id into.
func NewWAVFile(path string, opts ...Option) *WAVFileSink {
	cfg := &Sink{ExpectedFormat: defaultExpectedFormat()}
	for _, opt := range opts {
		opt(cfg)
	}
	return &WAVFileSink{
		Path:           path,
		ExpectedFormat: cfg.ExpectedFormat,
	}
}

// Consume concatenates res.Timeline.Blocks into one combined .wav at w.Path.
//
// Flow:
//  1. Validate Path + MkdirAll its parent dir.
//  2. buildSegments — the shared per-block wav-concat walk (also used by the
//     3-file persistent Sink). Returns segments + receipt counts; on a
//     per-block read error or ctx cancel it returns the partial counts which
//     still populate a partial SinkReceipt.
//  3. writeAudio — atomic tmp+rename of the single combined wav. NO sidecars.
//
// Refusal handling: refused blocks already carry rendered audio of their
// Refusal.Message and are treated identically to voiced blocks (honesty rule —
// refusal is data, never an error). An all-refused plan still produces a valid
// RIFF/WAVE (the refusal audio is concatenated); an empty plan (no blocks)
// produces a valid empty-but-well-formed RIFF/WAVE with a zero-length data
// chunk — writeCombinedWAV always emits a self-consistent header. The sink
// never returns an "empty input" error.
//
// The plan argument is unused — this sink writes no plan.json / manifest.json,
// so it has nothing to project from the NarrationPlan.
func (w *WAVFileSink) Consume(ctx context.Context, _ plan.NarrationPlan, res render.RenderResult) (sink.SinkReceipt, error) {
	var receipt sink.SinkReceipt

	if w.Path == "" {
		return receipt, ErrNoWAVPath
	}

	parent := filepathDir(w.Path)
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return receipt, fmt.Errorf("sink/persistent: mkdir %s: %w", parent, err)
	}

	// expectedFormat — honor the configured format, else the documented
	// render.DefaultFormat() default. The zero-value path (no NewWAVFile)
	// lands here with an empty AudioFormat; treat that as "use default".
	expectedFormat := w.ExpectedFormat
	if expectedFormat == (plan.AudioFormat{}) {
		expectedFormat = defaultExpectedFormat()
	}

	segments, blocksWritten, totalDurationMs, err := buildSegments(ctx, res, expectedFormat)
	receipt.BlocksPlayed = blocksWritten
	receipt.TotalDurationMs = totalDurationMs
	if err != nil {
		return receipt, err
	}

	// writeAudio creates its atomic tmp file in filepathDir(w.Path) — adjacent
	// to the target — so the os.Rename is intra-directory and cannot fail with
	// cross-device EXDEV even when the caller's path is on a non-default mount.
	if err := writeAudio(w.Path, expectedFormat, segments); err != nil {
		return receipt, fmt.Errorf("sink/persistent: write %s: %w", w.Path, err)
	}

	return receipt, nil
}
