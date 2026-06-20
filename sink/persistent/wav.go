// Package persistent is the phase-one persistent-output OutputSink.
//
// It implements sink.OutputSink by concatenating per-block WAVs from the
// renderer into a single audio.wav, writing plan.json verbatim from the
// input NarrationPlan, and emitting a manifest.json receipt (content
// hashes, voice id, format, per-block index).
//
// Invariants (CLAUDE.md):
//   - Imports plan/ + render/ + sink/ + stdlib only. No planner/, no
//     adapter/, no intelligence/, no pipeline/.
//   - The plan is the contract — plan.json is written verbatim from the
//     input plan.NarrationPlan. The sink never edits, sanitizes, or
//     re-derives plan fields.
//   - Refusal is data, not error. Refused blocks already carry audio of
//     Refusal.Message and are treated identical to voiced blocks.
//   - Empty AudioRef blocks (all-pause / no-speech) are skipped on read
//     (no file IO) but their planned duration still contributes to
//     SinkReceipt.TotalDurationMs and to inter-block silence accounting.
//   - SinkReceipt.TotalDurationMs is summed from Timeline.Blocks planned
//     duration, NOT wall-clock. Mirrors the ephemeral-sink invariant.
//   - Context cancellation propagates: Consume returns ctx.Err() with a
//     partial SinkReceipt and never leaves a partial audio.wav on disk.
//   - All three output files use atomic tmp+rename writes (Decision v1.4.0
//     in planner-task.md). Honesty rule extends to bytes on disk.
//   - Stale detection (CheckStale) is read-side and reports data; it never
//     auto-regenerates (CLAUDE.md domain rule).
package persistent

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/vd09-projects/intelligent-tts-narration-library/plan"
	"github.com/vd09-projects/intelligent-tts-narration-library/render"
)

// FOURCC tags used in the RIFF / WAVE container.
//
// Spec reference: Microsoft WAVE format (RIFF), informally documented
// across many sources. The minimum chunks we care about are "RIFF" (the
// container), "WAVE" (the form), "fmt " (audio format), and "data" (the
// PCM payload). Other chunks (LIST, INFO, JUNK, bext) may appear; we skip
// them on read.
var (
	fourccRIFF = [4]byte{'R', 'I', 'F', 'F'}
	fourccWAVE = [4]byte{'W', 'A', 'V', 'E'}
	fourccFMT  = [4]byte{'f', 'm', 't', ' '}
	fourccDATA = [4]byte{'d', 'a', 't', 'a'}
)

// PCM audio format code (the "fmt " chunk's wFormatTag field). 0x0001 is
// uncompressed linear PCM, which is what Kokoro emits and the only codec
// this sink supports (Decision v1.0.0).
const wFormatTagPCM uint16 = 0x0001

// riffHeader carries the parsed "fmt " chunk fields the sink validates
// against the expected plan.AudioFormat.
type riffHeader struct {
	FormatTag     uint16 // 0x0001 for PCM
	Channels      uint16
	SampleRate    uint32
	ByteRate      uint32 // sampleRate * channels * bitDepth / 8
	BlockAlign    uint16 // channels * bitDepth / 8
	BitsPerSample uint16
}

// wavSegment is one block's contribution to the combined output:
// leadingSilenceMs milliseconds of zero-padded silence followed by pcm
// bytes. PCM may be nil/empty for skipped (empty-AudioRef) blocks; the
// leading silence still renders per Decision v1.5.0.
type wavSegment struct {
	pcm              []byte
	leadingSilenceMs int
}

// formatMismatchError is returned by readWAV when the parsed "fmt " chunk
// disagrees with the expected plan.AudioFormat. The error names the
// divergent field so an upstream renderer regression is observable.
type formatMismatchError struct {
	Path     string
	Field    string
	Expected any
	Got      any
}

func (e *formatMismatchError) Error() string {
	return fmt.Sprintf("wav format mismatch at %s: %s expected %v got %v",
		e.Path, e.Field, e.Expected, e.Got)
}

// Is lets errors.Is(err, ErrFormatMismatch) match any *formatMismatchError, so
// the composition root can classify format-mismatch refusals (exit 2) without
// importing the unexported type (issue #28).
func (e *formatMismatchError) Is(target error) bool {
	return target == ErrFormatMismatch
}

// ErrInvalidWAV is the sentinel returned when a file does not parse as a
// valid RIFF/WAVE container (missing magic, truncated, etc.). Distinct
// from formatMismatchError so callers can tell "the file is broken" from
// "the file is a different valid format than we accept".
var ErrInvalidWAV = errors.New("invalid WAV container")

// readWAV reads a WAV file at path, validates its format chunk equals
// expected (per Decision v1.0.0 — hard-coded to render.DefaultFormat()),
// and returns the parsed header plus the raw PCM bytes from the "data"
// chunk.
//
// Unknown chunks (LIST, INFO, JUNK, bext, ...) are skipped — the parser
// scans by FOURCC rather than assuming fixed offsets. This is defensive
// against future engines that might add metadata chunks (Decision v1.0.0
// risk discussion in planner-task.md).
func readWAV(path string, expected plan.AudioFormat) (riffHeader, []byte, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return riffHeader{}, nil, fmt.Errorf("read wav %s: %w", path, err)
	}
	return parseWAV(path, raw, expected)
}

// parseWAV is the pure-bytes form of readWAV — pulled out so tests can
// feed synthetic byte slices without touching the filesystem.
func parseWAV(path string, raw []byte, expected plan.AudioFormat) (riffHeader, []byte, error) {
	if len(raw) < 12 {
		return riffHeader{}, nil, fmt.Errorf("%w: %s: file too short for RIFF header (%d bytes)",
			ErrInvalidWAV, path, len(raw))
	}
	if [4]byte(raw[0:4]) != fourccRIFF {
		return riffHeader{}, nil, fmt.Errorf("%w: %s: missing RIFF magic", ErrInvalidWAV, path)
	}
	if [4]byte(raw[8:12]) != fourccWAVE {
		return riffHeader{}, nil, fmt.Errorf("%w: %s: missing WAVE form", ErrInvalidWAV, path)
	}
	// RIFF chunk size at raw[4:8] is mostly informational; some encoders
	// underreport it. We trust per-subchunk sizes instead.

	var (
		header  riffHeader
		pcm     []byte
		gotFMT  bool
		gotDATA bool
	)

	// Walk subchunks starting at offset 12. Each subchunk is: 4-byte
	// FOURCC + 4-byte little-endian size + size bytes of payload. RIFF
	// pads odd-sized payloads to even with a single trailing byte.
	for off := 12; off+8 <= len(raw); {
		tag := [4]byte(raw[off : off+4])
		size := binary.LittleEndian.Uint32(raw[off+4 : off+8])
		payloadStart := off + 8
		payloadEnd := payloadStart + int(size)
		if payloadEnd > len(raw) {
			return riffHeader{}, nil, fmt.Errorf("%w: %s: subchunk %q claims %d bytes, file has %d remaining",
				ErrInvalidWAV, path, string(tag[:]), size, len(raw)-payloadStart)
		}
		switch tag {
		case fourccFMT:
			if size < 16 {
				return riffHeader{}, nil, fmt.Errorf("%w: %s: fmt chunk too small (%d bytes)",
					ErrInvalidWAV, path, size)
			}
			header.FormatTag = binary.LittleEndian.Uint16(raw[payloadStart : payloadStart+2])
			header.Channels = binary.LittleEndian.Uint16(raw[payloadStart+2 : payloadStart+4])
			header.SampleRate = binary.LittleEndian.Uint32(raw[payloadStart+4 : payloadStart+8])
			header.ByteRate = binary.LittleEndian.Uint32(raw[payloadStart+8 : payloadStart+12])
			header.BlockAlign = binary.LittleEndian.Uint16(raw[payloadStart+12 : payloadStart+14])
			header.BitsPerSample = binary.LittleEndian.Uint16(raw[payloadStart+14 : payloadStart+16])
			gotFMT = true
		case fourccDATA:
			pcm = raw[payloadStart:payloadEnd]
			gotDATA = true
		}
		// Advance past payload + pad byte if size is odd.
		off = payloadEnd
		if size%2 == 1 && off < len(raw) {
			off++
		}
		if gotFMT && gotDATA {
			break
		}
	}

	if !gotFMT {
		return riffHeader{}, nil, fmt.Errorf("%w: %s: missing fmt chunk", ErrInvalidWAV, path)
	}
	if !gotDATA {
		return riffHeader{}, nil, fmt.Errorf("%w: %s: missing data chunk", ErrInvalidWAV, path)
	}

	// Validate against expected format. Each mismatch reports the specific
	// divergent field so an upstream renderer regression is observable
	// quickly (planner-task.md risk discussion).
	if header.FormatTag != wFormatTagPCM {
		return header, nil, &formatMismatchError{Path: path, Field: "format_tag",
			Expected: wFormatTagPCM, Got: header.FormatTag}
	}
	if int(header.SampleRate) != expected.SampleRate {
		return header, nil, &formatMismatchError{Path: path, Field: "sample_rate",
			Expected: expected.SampleRate, Got: header.SampleRate}
	}
	if int(header.Channels) != expected.Channels {
		return header, nil, &formatMismatchError{Path: path, Field: "channels",
			Expected: expected.Channels, Got: header.Channels}
	}
	expectedBitDepth, err := bitDepthFromEncoding(expected.Encoding)
	if err != nil {
		return header, nil, fmt.Errorf("%s: %w", path, err)
	}
	if int(header.BitsPerSample) != expectedBitDepth {
		return header, nil, &formatMismatchError{Path: path, Field: "bits_per_sample",
			Expected: expectedBitDepth, Got: header.BitsPerSample}
	}

	return header, pcm, nil
}

// bitDepthFromEncoding maps the plan.AudioFormat.Encoding string ("wav" /
// "pcm_s16le") to a bit depth. The phase-one encoding strings in
// render.DefaultFormat() are "wav" or "pcm_s16le" — both imply 16-bit
// signed little-endian PCM. A future format would need its own entry.
//
// Returns an error rather than a default so a Spec drift (e.g.
// render.DefaultFormat() switching to "pcm_f32le") fails loudly instead
// of silently mis-validating.
func bitDepthFromEncoding(encoding string) (int, error) {
	switch encoding {
	case "wav", "pcm_s16le":
		return 16, nil
	default:
		return 0, fmt.Errorf("persistent: unknown encoding %q (expected wav or pcm_s16le)", encoding)
	}
}

// silenceBytes returns the count of zero bytes that represent durationMs
// milliseconds of silence at the given format. Negative durations clamp
// to zero (planner-task.md silence-gap math: max(0, gap_ms)).
//
// Math: bytesPerFrame = channels * bitDepth / 8.
// frames = durationMs * sampleRate / 1000.
// bytes = frames * bytesPerFrame.
//
// Calculation uses int64 internally to avoid overflow on plausible inputs
// (24 kHz * 16-bit * 1 channel * 10-minute silence = ~28.8 MB, safely
// within int64). Result is int because PCM slice sizes are int-indexed.
func silenceBytes(durationMs int, format plan.AudioFormat) int {
	if durationMs <= 0 {
		return 0
	}
	bitDepth, err := bitDepthFromEncoding(format.Encoding)
	if err != nil {
		// Caller should validate format up front; reaching here means a
		// programmer error. Return 0 as a degraded but safe fallback —
		// no silence rather than a panic.
		return 0
	}
	bytesPerFrame := int64(format.Channels) * int64(bitDepth) / 8
	frames := int64(durationMs) * int64(format.SampleRate) / 1000
	return int(frames * bytesPerFrame)
}

// writeCombinedWAV emits a single RIFF/WAVE container with the given
// format header plus the per-segment payloads concatenated together,
// each prefixed with leadingSilenceMs of zero bytes.
//
// The header's "RIFF size" and "data size" fields are computed from the
// summed payload, so the output is always self-consistent regardless of
// segment count or silence padding.
//
// io.Writer is the abstraction because callers wrap an atomic tmp file
// (Decision v1.4.0). Buffered writers are caller's responsibility — we
// emit byte-deterministic output and do not buffer internally so tests
// can assert byte-equality without flush coordination.
func writeCombinedWAV(out io.Writer, format plan.AudioFormat, segments []wavSegment) error {
	bitDepth, err := bitDepthFromEncoding(format.Encoding)
	if err != nil {
		return err
	}
	channels := uint16(format.Channels)
	sampleRate := uint32(format.SampleRate)
	bitsPerSample := uint16(bitDepth)
	blockAlign := channels * bitsPerSample / 8
	byteRate := sampleRate * uint32(blockAlign)

	// Compute the total payload size first — we need it for the RIFF and
	// data-chunk size fields, which sit at the top of the file.
	var dataSize int64
	for _, seg := range segments {
		dataSize += int64(silenceBytes(seg.leadingSilenceMs, format))
		dataSize += int64(len(seg.pcm))
	}
	if dataSize > int64(^uint32(0)) {
		return fmt.Errorf("persistent: combined wav payload %d bytes exceeds RIFF max (4 GiB)",
			dataSize)
	}
	// RIFF size = total file size - 8 (the "RIFF" tag + size field themselves).
	// File layout: 12 bytes "RIFF<size>WAVE" + 8 bytes "fmt <size>" + 16 bytes fmt payload
	//            + 8 bytes "data<size>" + dataSize bytes PCM.
	// So RIFF size = 4 (WAVE) + 8 (fmt header) + 16 (fmt payload) + 8 (data header) + dataSize = 36 + dataSize.
	riffSize := uint32(36 + dataSize)

	hdr := make([]byte, 0, 44)
	hdr = append(hdr, 'R', 'I', 'F', 'F')
	hdr = binary.LittleEndian.AppendUint32(hdr, riffSize)
	hdr = append(hdr, 'W', 'A', 'V', 'E')
	hdr = append(hdr, 'f', 'm', 't', ' ')
	hdr = binary.LittleEndian.AppendUint32(hdr, 16) // fmt payload size
	hdr = binary.LittleEndian.AppendUint16(hdr, wFormatTagPCM)
	hdr = binary.LittleEndian.AppendUint16(hdr, channels)
	hdr = binary.LittleEndian.AppendUint32(hdr, sampleRate)
	hdr = binary.LittleEndian.AppendUint32(hdr, byteRate)
	hdr = binary.LittleEndian.AppendUint16(hdr, blockAlign)
	hdr = binary.LittleEndian.AppendUint16(hdr, bitsPerSample)
	hdr = append(hdr, 'd', 'a', 't', 'a')
	hdr = binary.LittleEndian.AppendUint32(hdr, uint32(dataSize))

	if _, err := out.Write(hdr); err != nil {
		return fmt.Errorf("persistent: write wav header: %w", err)
	}

	// Per-segment payload: leading silence then PCM. Silence emission is
	// streamed in moderate chunks rather than allocating the full silence
	// span up front — keeps a 10-minute leading-silence outlier (28.8 MB)
	// off the heap.
	const silenceChunk = 4096
	zeros := make([]byte, silenceChunk)
	for _, seg := range segments {
		remaining := silenceBytes(seg.leadingSilenceMs, format)
		for remaining > 0 {
			n := min(remaining, silenceChunk)
			if _, err := out.Write(zeros[:n]); err != nil {
				return fmt.Errorf("persistent: write silence: %w", err)
			}
			remaining -= n
		}
		if len(seg.pcm) > 0 {
			if _, err := out.Write(seg.pcm); err != nil {
				return fmt.Errorf("persistent: write pcm: %w", err)
			}
		}
	}
	return nil
}

// defaultExpectedFormat returns the format readWAV should validate
// against when callers do not override it. Today this is exactly
// render.DefaultFormat() (24 kHz mono s16le WAV) — see Decision v1.0.0.
func defaultExpectedFormat() plan.AudioFormat {
	return render.DefaultFormat()
}
