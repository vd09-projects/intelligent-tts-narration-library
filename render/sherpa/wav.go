package sherpa

import (
	"encoding/binary"
	"fmt"
	"os"

	"github.com/vd09-projects/intelligent-tts-narration-library/plan"
)

// wavHeaderBytes — bytes-per-sample wav files we accept: standard RIFF
// header is 44 bytes. The Python runner emits exactly that layout; we
// trust it (planner-task.md, "header-trust fragility" risk).
const wavHeaderBytes = 44

// s16leBytesPerSample — phase-one PCM is signed 16-bit little-endian, so two
// bytes per sample. Shared by wavDurationMs and frameAlignWAV.
const s16leBytesPerSample = 2

// frameAlignWAV snaps a per-block WAV's PCM payload to a whole-millisecond
// byte boundary, returning the rewritten container (header sizes fixed).
//
// Why this is load-bearing: the persistent sink stores per-block duration in
// whole ms and DERIVES byte ranges from it (silenceBytes(EndMs-StartMs)) when
// it patches a single block. Raw Kokoro PCM is not a whole-ms multiple, so the
// actual byte length and the ms-derived length disagree by <1 ms of bytes.
// That disagreement is invisible until a later PatchBlock reconstructs byte
// ranges from the manifest and refuses the (uncorrupted) container as a
// container/manifest length mismatch — the per-block rounding errors only
// happen to cancel for some documents. Emitting whole-ms blocks here makes the
// ms<->byte round-trip exact everywhere downstream, so the check always holds.
//
// The target ms is round-to-nearest, identical to wavDurationMs, so EndMs is
// unchanged from the pre-alignment behavior — only the byte count is snapped.
// Padding appends inaudible trailing silence; a trim drops <0.5 ms of tail.
// A WAV whose payload is already whole-ms (the test fake's exact 200 ms) is
// returned unchanged.
func frameAlignWAV(raw []byte, fmtSpec plan.AudioFormat) []byte {
	if len(raw) < wavHeaderBytes {
		return raw // malformed — let wavDurationMs surface the error.
	}
	bytesPerSec := int(fmtSpec.SampleRate) * fmtSpec.Channels * s16leBytesPerSample
	bytesPerMs := bytesPerSec / 1000
	if bytesPerMs <= 0 {
		return raw
	}
	dataLen := len(raw) - wavHeaderBytes
	durMs := (dataLen + bytesPerMs/2) / bytesPerMs // round to nearest ms.
	target := durMs * bytesPerMs
	if target == dataLen {
		return raw
	}

	out := make([]byte, wavHeaderBytes+target)
	copy(out, raw[:wavHeaderBytes])
	if target < dataLen {
		copy(out[wavHeaderBytes:], raw[wavHeaderBytes:wavHeaderBytes+target])
	} else {
		copy(out[wavHeaderBytes:], raw[wavHeaderBytes:]) // tail stays zero (silence).
	}
	// Fix the RIFF size (offset 4 = 36 + dataSize) and data-chunk size
	// (offset 40 = dataSize) header fields to match the new payload.
	binary.LittleEndian.PutUint32(out[4:8], uint32(36+target))
	binary.LittleEndian.PutUint32(out[40:44], uint32(target))
	return out
}

// wavDurationMs returns the duration of a phase-one Kokoro WAV in
// milliseconds, derived from file size assuming the format described by
// fmt (sample rate, channel count, 16-bit PCM).
//
// Phase one: PCM s16le, mono, 24 000 Hz — 48 000 bytes/second.
// duration_ms = ((fileSize - 44) / bytesPerSecond) * 1000
//
// Returns (0, nil) for files of exactly header size (zero-sample WAV).
// Returns an error if the file is smaller than the header — that means
// the subprocess wrote partial output, which is the wrapper's job to
// prevent (it keeps stdout silent on failure per its contract).
func wavDurationMs(path string, fmtSpec plan.AudioFormat) (int, error) {
	info, err := os.Stat(path)
	if err != nil {
		return 0, fmt.Errorf("sherpa: stat wav %q: %w", path, err)
	}
	size := info.Size()
	if size < wavHeaderBytes {
		return 0, fmt.Errorf("sherpa: wav %q smaller than 44-byte header (got %d bytes)", path, size)
	}
	dataBytes := size - wavHeaderBytes
	bytesPerSample := 2 // s16le → 2 bytes
	bytesPerSec := int64(fmtSpec.SampleRate) * int64(fmtSpec.Channels) * int64(bytesPerSample)
	if bytesPerSec == 0 {
		return 0, fmt.Errorf("sherpa: zero bytes-per-second from format %+v", fmtSpec)
	}
	// Round to nearest ms.
	durMs := (dataBytes*1000 + bytesPerSec/2) / bytesPerSec
	return int(durMs), nil
}
