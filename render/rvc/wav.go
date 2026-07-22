package rvc

import (
	"encoding/binary"
	"fmt"
	"os"

	"github.com/vd09-projects/intelligent-tts-narration-library/plan"
)

// Canonical RIFF/WAVE layout constants. The worker's _atomic_write_wav uses
// Python's `wave` module, which emits exactly this 44-byte header (no LIST/fact
// chunks), so the hardcoded RIFF-size (4:8) and data-size (40:44) offsets hold.
const (
	wavHeaderBytes      = 44
	s16leBytesPerSample = 2
	dataChunkIDOffset   = 36 // "data" tag sits at [36:40] in a canonical 44-byte header.
)

// frameAlignWAV snaps a per-block WAV's PCM payload to a whole-millisecond byte
// boundary and returns the rewritten container (RIFF + data-size fields fixed).
//
// This is the 40 kHz reimplementation of sherpa's frameAlignWAV (parameterized by
// format): the RVC worker writes raw 40 kHz sample counts with no whole-ms
// guarantee, and the persistent sink DERIVES byte ranges from whole-ms durations
// when it patches a single block — so an unaligned payload would make a later
// #146 patch refuse the (uncorrupted) container on sub-ms rounding drift. At
// 40 kHz mono s16le, bytesPerMs = 80 (40 samples/ms), a clean whole-ms boundary.
//
// Canonical-header guard (review refinement): a WAV that is too short or whose
// 'data' chunk is not at the canonical offset fails LOUD (returns an error)
// rather than silently corrupting the size fields — so a future non-canonical
// worker WAV writer (e.g. one that adds a LIST/fact chunk) is caught, not
// mis-rewritten.
func frameAlignWAV(raw []byte, format plan.AudioFormat) ([]byte, error) {
	if len(raw) < wavHeaderBytes {
		return nil, fmt.Errorf("rvc: wav shorter than %d-byte header (got %d bytes)", wavHeaderBytes, len(raw))
	}
	if string(raw[dataChunkIDOffset:dataChunkIDOffset+4]) != "data" {
		return nil, fmt.Errorf("rvc: non-canonical WAV header — no 'data' chunk at offset %d; refusing to rewrite RIFF/data-size fields", dataChunkIDOffset)
	}

	bytesPerMs := int(format.SampleRate) * format.Channels * s16leBytesPerSample / 1000
	if bytesPerMs <= 0 {
		return nil, fmt.Errorf("rvc: zero bytes-per-ms from format %+v", format)
	}

	dataLen := len(raw) - wavHeaderBytes
	durMs := (dataLen + bytesPerMs/2) / bytesPerMs // round to nearest ms.
	target := durMs * bytesPerMs
	if target == dataLen {
		return raw, nil // already whole-ms — unchanged.
	}

	out := make([]byte, wavHeaderBytes+target)
	copy(out, raw[:wavHeaderBytes])
	if target < dataLen {
		copy(out[wavHeaderBytes:], raw[wavHeaderBytes:wavHeaderBytes+target])
	} else {
		copy(out[wavHeaderBytes:], raw[wavHeaderBytes:]) // tail stays zero (silence pad).
	}
	// Fix RIFF size (offset 4 = 36 + dataSize) and data-chunk size (offset 40).
	binary.LittleEndian.PutUint32(out[4:8], uint32(36+target))
	binary.LittleEndian.PutUint32(out[40:44], uint32(target))
	return out, nil
}

// wavDataDurationMs returns the whole-ms duration implied by an already-aligned
// WAV byte buffer. Because frameAlignWAV guarantees a whole-ms data chunk, this
// division is exact — the ms↔byte round-trip stays lossless.
func wavDataDurationMs(aligned []byte, format plan.AudioFormat) int {
	bytesPerMs := int(format.SampleRate) * format.Channels * s16leBytesPerSample / 1000
	if bytesPerMs <= 0 {
		return 0
	}
	return (len(aligned) - wavHeaderBytes) / bytesPerMs
}

// alignInto reads the raw worker output at rawPath, frame-aligns it to whole-ms at
// format, writes it atomically (temp + rename) to finalPath, and returns the
// aligned duration in ms. It reads the worker's *intermediate* output only —
// never a final file handed to the sink — so the "renderer never reads its own
// output back" invariant is preserved.
func alignInto(rawPath, finalPath string, format plan.AudioFormat) (int, error) {
	raw, err := os.ReadFile(rawPath)
	if err != nil {
		return 0, fmt.Errorf("rvc: read worker output %q: %w", rawPath, err)
	}
	aligned, err := frameAlignWAV(raw, format)
	if err != nil {
		return 0, err
	}
	tmp := finalPath + ".tmp"
	if err := os.WriteFile(tmp, aligned, 0o644); err != nil {
		return 0, fmt.Errorf("rvc: write temp %q: %w", tmp, err)
	}
	if err := os.Rename(tmp, finalPath); err != nil {
		_ = os.Remove(tmp)
		return 0, fmt.Errorf("rvc: rename %q to %q: %w", tmp, finalPath, err)
	}
	return wavDataDurationMs(aligned, format), nil
}
