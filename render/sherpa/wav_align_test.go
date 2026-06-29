package sherpa

import (
	"encoding/binary"
	"testing"

	"github.com/vd09-projects/intelligent-tts-narration-library/render"
)

// makeWAV builds a minimal 44-byte-header PCM WAV with dataLen payload bytes.
func makeWAV(dataLen int) []byte {
	raw := make([]byte, wavHeaderBytes+dataLen)
	copy(raw, []byte("RIFF"))
	binary.LittleEndian.PutUint32(raw[4:8], uint32(36+dataLen))
	copy(raw[8:12], []byte("WAVE"))
	copy(raw[12:16], []byte("fmt "))
	copy(raw[36:40], []byte("data"))
	binary.LittleEndian.PutUint32(raw[40:44], uint32(dataLen))
	// Fill payload with a non-zero pattern so a trim is observable.
	for i := wavHeaderBytes; i < len(raw); i++ {
		raw[i] = byte(0x40 + i%8)
	}
	return raw
}

func TestFrameAlignWAV(t *testing.T) {
	format := render.DefaultFormat() // 24 kHz mono s16le → 48 bytes/ms.
	const bytesPerMs = 48

	t.Run("whole-ms payload returned unchanged", func(t *testing.T) {
		raw := makeWAV(200 * bytesPerMs) // exactly 200 ms, like the test fake.
		got := frameAlignWAV(raw, format)
		if len(got) != len(raw) {
			t.Fatalf("len changed: got %d want %d", len(got), len(raw))
		}
	})

	t.Run("rounds up: pads to next whole ms", func(t *testing.T) {
		// 9173 ms * 48 = 440304; +16 bytes lands at remainder 16 (<24 → rounds
		// down to 9173). Use +40 so it rounds UP to 9174 and pads.
		raw := makeWAV(9173*bytesPerMs + 40)
		got := frameAlignWAV(raw, format)
		gotData := len(got) - wavHeaderBytes
		if gotData%bytesPerMs != 0 {
			t.Fatalf("aligned payload %d not a whole-ms multiple", gotData)
		}
		if gotData != 9174*bytesPerMs {
			t.Fatalf("payload = %d, want %d (rounded up to 9174 ms)", gotData, 9174*bytesPerMs)
		}
		assertHeaderSizes(t, got, gotData)
	})

	t.Run("rounds down: trims sub-ms tail", func(t *testing.T) {
		raw := makeWAV(9173*bytesPerMs + 16) // remainder 16 (<24) → rounds to 9173.
		got := frameAlignWAV(raw, format)
		gotData := len(got) - wavHeaderBytes
		if gotData != 9173*bytesPerMs {
			t.Fatalf("payload = %d, want %d (rounded down to 9173 ms)", gotData, 9173*bytesPerMs)
		}
		assertHeaderSizes(t, got, gotData)
	})

	t.Run("round-trips exactly through wavDurationMs", func(t *testing.T) {
		// The whole point: after alignment, the byte length equals the ms-derived
		// length, so wavDurationMs(file)*bytesPerMs == on-disk payload bytes — the
		// invariant the persistent sink's F1 check relies on.
		for _, dataLen := range []int{1, 47, 440320, 530432, 9173*bytesPerMs + 23} {
			raw := makeWAV(dataLen)
			got := frameAlignWAV(raw, format)
			gotData := len(got) - wavHeaderBytes
			ms := gotData / bytesPerMs
			if ms*bytesPerMs != gotData {
				t.Errorf("dataLen=%d: aligned payload %d not whole-ms", dataLen, gotData)
			}
		}
	})

	t.Run("malformed (sub-header) returned as-is", func(t *testing.T) {
		raw := []byte{1, 2, 3}
		if got := frameAlignWAV(raw, format); len(got) != 3 {
			t.Fatalf("short input mutated: len %d", len(got))
		}
	})
}

func assertHeaderSizes(t *testing.T, raw []byte, dataLen int) {
	t.Helper()
	if got := binary.LittleEndian.Uint32(raw[40:44]); got != uint32(dataLen) {
		t.Errorf("data-chunk size header = %d, want %d", got, dataLen)
	}
	if got := binary.LittleEndian.Uint32(raw[4:8]); got != uint32(36+dataLen) {
		t.Errorf("RIFF size header = %d, want %d", got, 36+dataLen)
	}
}
