package sherpa

import (
	"fmt"
	"os"

	"github.com/vd09-projects/intelligent-tts-narration-library/plan"
)

// wavHeaderBytes — bytes-per-sample wav files we accept: standard RIFF
// header is 44 bytes. The Python runner emits exactly that layout; we
// trust it (planner-task.md, "header-trust fragility" risk).
const wavHeaderBytes = 44

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
