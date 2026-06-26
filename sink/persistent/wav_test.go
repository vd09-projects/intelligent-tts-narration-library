package persistent

import (
	"bytes"
	"encoding/binary"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/vd09-projects/intelligent-tts-narration-library/plan"
	"github.com/vd09-projects/intelligent-tts-narration-library/render"
)

// synthWAV returns a synthetic in-memory WAV blob with the given format
// + PCM payload. Useful for tests that need a "valid" WAV without
// shelling to a real encoder.
//
// Generating fixtures in-test rather than checking in golden WAVs is
// Decision v1.7.0: a golden WAV would byte-couple to the writer under
// test, defeating the round-trip property.
func synthWAV(t *testing.T, sampleRate, channels, bitsPerSample int, pcm []byte) []byte {
	t.Helper()
	if bitsPerSample%8 != 0 {
		t.Fatalf("synthWAV: bitsPerSample must be a multiple of 8 (got %d)", bitsPerSample)
	}
	blockAlign := uint16(channels * bitsPerSample / 8)
	byteRate := uint32(sampleRate * channels * bitsPerSample / 8)
	dataSize := uint32(len(pcm))
	riffSize := 36 + dataSize

	buf := make([]byte, 0, 44+len(pcm))
	buf = append(buf, 'R', 'I', 'F', 'F')
	buf = binary.LittleEndian.AppendUint32(buf, riffSize)
	buf = append(buf, 'W', 'A', 'V', 'E')
	buf = append(buf, 'f', 'm', 't', ' ')
	buf = binary.LittleEndian.AppendUint32(buf, 16)
	buf = binary.LittleEndian.AppendUint16(buf, wFormatTagPCM)
	buf = binary.LittleEndian.AppendUint16(buf, uint16(channels))
	buf = binary.LittleEndian.AppendUint32(buf, uint32(sampleRate))
	buf = binary.LittleEndian.AppendUint32(buf, byteRate)
	buf = binary.LittleEndian.AppendUint16(buf, blockAlign)
	buf = binary.LittleEndian.AppendUint16(buf, uint16(bitsPerSample))
	buf = append(buf, 'd', 'a', 't', 'a')
	buf = binary.LittleEndian.AppendUint32(buf, dataSize)
	buf = append(buf, pcm...)
	return buf
}

// synthWAVNonPCM is synthWAV but with a caller-chosen format-tag. Used by
// the codec-mismatch test case.
func synthWAVNonPCM(sampleRate, channels, bitsPerSample int, formatTag uint16, pcm []byte) []byte {
	blockAlign := uint16(channels * bitsPerSample / 8)
	byteRate := uint32(sampleRate * channels * bitsPerSample / 8)
	dataSize := uint32(len(pcm))
	riffSize := 36 + dataSize
	buf := make([]byte, 0, 44+len(pcm))
	buf = append(buf, 'R', 'I', 'F', 'F')
	buf = binary.LittleEndian.AppendUint32(buf, riffSize)
	buf = append(buf, 'W', 'A', 'V', 'E')
	buf = append(buf, 'f', 'm', 't', ' ')
	buf = binary.LittleEndian.AppendUint32(buf, 16)
	buf = binary.LittleEndian.AppendUint16(buf, formatTag)
	buf = binary.LittleEndian.AppendUint16(buf, uint16(channels))
	buf = binary.LittleEndian.AppendUint32(buf, uint32(sampleRate))
	buf = binary.LittleEndian.AppendUint32(buf, byteRate)
	buf = binary.LittleEndian.AppendUint16(buf, blockAlign)
	buf = binary.LittleEndian.AppendUint16(buf, uint16(bitsPerSample))
	buf = append(buf, 'd', 'a', 't', 'a')
	buf = binary.LittleEndian.AppendUint32(buf, dataSize)
	buf = append(buf, pcm...)
	return buf
}

func TestParseWAV_FormatValidation_TableDriven(t *testing.T) {
	// 8 bytes of zero PCM — content irrelevant; just need a data chunk
	// the parser can find.
	samplePCM := []byte{0, 0, 0, 0, 0, 0, 0, 0}
	expected := render.DefaultFormat()

	cases := []struct {
		name        string
		sampleRate  int
		channels    int
		bitsPerSamp int
		formatTag   uint16
		wantOK      bool
		wantField   string // when wantOK=false
	}{
		{"happy-default-format", 24000, 1, 16, wFormatTagPCM, true, ""},
		{"wrong-sample-rate", 16000, 1, 16, wFormatTagPCM, false, "sample_rate"},
		{"wrong-channels", 24000, 2, 16, wFormatTagPCM, false, "channels"},
		{"wrong-bit-depth", 24000, 1, 24, wFormatTagPCM, false, "bits_per_sample"},
		// μ-law (0x0007) is one of the canonical non-PCM codecs.
		{"wrong-codec-mu-law", 24000, 1, 16, 0x0007, false, "format_tag"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var raw []byte
			if tc.formatTag == wFormatTagPCM {
				raw = synthWAV(t, tc.sampleRate, tc.channels, tc.bitsPerSamp, samplePCM)
			} else {
				raw = synthWAVNonPCM(tc.sampleRate, tc.channels, tc.bitsPerSamp, tc.formatTag, samplePCM)
			}
			_, pcm, err := parseWAV("synth.wav", raw, expected)
			if tc.wantOK {
				if err != nil {
					t.Fatalf("expected ok, got err: %v", err)
				}
				if !bytes.Equal(pcm, samplePCM) {
					t.Fatalf("pcm round-trip mismatch: got %v want %v", pcm, samplePCM)
				}
				return
			}
			if err == nil {
				t.Fatalf("expected error mentioning %s, got nil", tc.wantField)
			}
			var fme *formatMismatchError
			if !errors.As(err, &fme) {
				t.Fatalf("expected formatMismatchError, got %T: %v", err, err)
			}
			if fme.Field != tc.wantField {
				t.Fatalf("wrong field in error: got %q want %q", fme.Field, tc.wantField)
			}
		})
	}
}

func TestParseWAV_InvalidContainer(t *testing.T) {
	cases := []struct {
		name string
		raw  []byte
	}{
		{"too-short", []byte{0, 1, 2}},
		{"no-riff", append([]byte{'B', 'O', 'G', 'U', 0, 0, 0, 0, 'W', 'A', 'V', 'E'}, make([]byte, 32)...)},
		{"no-wave", append([]byte{'R', 'I', 'F', 'F', 0, 0, 0, 0, 'X', 'Y', 'Z', 'W'}, make([]byte, 32)...)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, _, err := parseWAV("bad.wav", tc.raw, render.DefaultFormat())
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if !errors.Is(err, ErrInvalidWAV) {
				t.Fatalf("expected ErrInvalidWAV, got %v", err)
			}
		})
	}
}

func TestParseWAV_SkipsUnknownChunks(t *testing.T) {
	// Build a WAV with a LIST chunk between fmt and data to confirm the
	// parser scans by FOURCC rather than assuming offsets. Future engines
	// might insert metadata chunks.
	pcm := []byte{1, 2, 3, 4, 5, 6, 7, 8}
	base := synthWAV(t, 24000, 1, 16, pcm)

	// Splice a 12-byte LIST chunk (8-byte header + 4-byte payload) in
	// after the fmt chunk (which ends at offset 36) and before the data
	// chunk (starts at offset 36).
	listChunk := []byte{'L', 'I', 'S', 'T', 4, 0, 0, 0, 'I', 'N', 'F', 'O'}
	mutated := append([]byte{}, base[:36]...)
	mutated = append(mutated, listChunk...)
	mutated = append(mutated, base[36:]...)

	_, gotPCM, err := parseWAV("with-list.wav", mutated, render.DefaultFormat())
	if err != nil {
		t.Fatalf("expected unknown chunks to be skipped, got err: %v", err)
	}
	if !bytes.Equal(gotPCM, pcm) {
		t.Fatalf("pcm mismatch after skipping LIST chunk: got %v want %v", gotPCM, pcm)
	}
}

func TestReadWAV_ReadsFromDisk(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "block.wav")
	pcm := []byte{0xAA, 0xBB, 0xCC, 0xDD, 0xEE, 0xFF, 0x11, 0x22}
	if err := os.WriteFile(path, synthWAV(t, 24000, 1, 16, pcm), 0o644); err != nil {
		t.Fatal(err)
	}
	_, got, err := readWAV(path, render.DefaultFormat())
	if err != nil {
		t.Fatalf("readWAV err: %v", err)
	}
	if !bytes.Equal(got, pcm) {
		t.Fatalf("pcm mismatch: got %v want %v", got, pcm)
	}
}

func TestReadWAV_MissingFile(t *testing.T) {
	_, _, err := readWAV("/nonexistent/path/here.wav", render.DefaultFormat())
	if err == nil {
		t.Fatal("expected error for missing file, got nil")
	}
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected os.ErrNotExist, got %v", err)
	}
}

func TestSilenceBytes(t *testing.T) {
	format := render.DefaultFormat() // 24 kHz, 1 ch, 16-bit
	cases := []struct {
		ms   int
		want int
	}{
		// 1000 ms at 24 kHz mono s16le = 24000 frames * 2 bytes = 48000 bytes
		{1000, 48000},
		// 500 ms = 24000 bytes
		{500, 24000},
		// 0 ms = 0
		{0, 0},
		// negative clamps to zero
		{-1, 0},
		{-1000, 0},
		// 10 ms = 480 bytes
		{10, 480},
	}
	for _, tc := range cases {
		got := silenceBytes(tc.ms, format)
		if got != tc.want {
			t.Errorf("silenceBytes(%d, default) = %d, want %d", tc.ms, got, tc.want)
		}
	}
}

func TestSilenceBytes_UnknownEncoding(t *testing.T) {
	// Degraded fallback returns 0 rather than panicking; callers should
	// validate format up-front but we don't want a crash in the silence
	// path.
	fmt := plan.AudioFormat{SampleRate: 24000, Channels: 1, Encoding: "pcm_f32le"}
	if got := silenceBytes(1000, fmt); got != 0 {
		t.Errorf("silenceBytes with unknown encoding: got %d, want 0 (degraded)", got)
	}
}

func TestWriteCombinedWAV_RoundTrip(t *testing.T) {
	// Two voiced segments, one with leading silence.
	pcmA := []byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08}
	pcmB := []byte{0x09, 0x0A, 0x0B, 0x0C, 0x0D, 0x0E, 0x0F, 0x10}
	segments := []wavSegment{
		{pcm: pcmA, leadingSilenceMs: 0},
		{pcm: pcmB, leadingSilenceMs: 100}, // 100 ms gap = 4800 bytes of zeros
	}
	var buf bytes.Buffer
	if err := writeCombinedWAV(&buf, render.DefaultFormat(), segments); err != nil {
		t.Fatalf("writeCombinedWAV err: %v", err)
	}
	// Round-trip through parseWAV.
	_, pcm, err := parseWAV("combined.wav", buf.Bytes(), render.DefaultFormat())
	if err != nil {
		t.Fatalf("parseWAV of combined buffer: %v", err)
	}
	wantSize := len(pcmA) + 4800 + len(pcmB)
	if len(pcm) != wantSize {
		t.Errorf("combined PCM size: got %d want %d", len(pcm), wantSize)
	}
	// Layout check: pcmA, then 4800 zeros, then pcmB.
	if !bytes.Equal(pcm[:len(pcmA)], pcmA) {
		t.Errorf("first segment mismatch: got %v want %v", pcm[:len(pcmA)], pcmA)
	}
	silence := pcm[len(pcmA) : len(pcmA)+4800]
	for i, b := range silence {
		if b != 0 {
			t.Errorf("silence byte %d non-zero (=%d)", i, b)
			break
		}
	}
	if !bytes.Equal(pcm[len(pcmA)+4800:], pcmB) {
		t.Errorf("second segment mismatch")
	}
}

func TestWriteCombinedWAV_DataSizeHeaderCorrect(t *testing.T) {
	// Verify the data-chunk size header equals the actual data byte
	// count — load-bearing for downstream WAV consumers (afplay, players).
	pcm := []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}
	segments := []wavSegment{{pcm: pcm, leadingSilenceMs: 50}} // 50 ms = 2400 zeros
	var buf bytes.Buffer
	if err := writeCombinedWAV(&buf, render.DefaultFormat(), segments); err != nil {
		t.Fatal(err)
	}
	raw := buf.Bytes()
	// data chunk header at offset 36; size at offset 40 (little-endian uint32).
	if [4]byte(raw[36:40]) != fourccDATA {
		t.Fatalf("expected 'data' at offset 36, got %q", string(raw[36:40]))
	}
	dataSize := binary.LittleEndian.Uint32(raw[40:44])
	wantDataSize := uint32(2400 + len(pcm))
	if dataSize != wantDataSize {
		t.Errorf("data chunk size header: got %d want %d", dataSize, wantDataSize)
	}
	// RIFF size at offset 4 = 36 + dataSize.
	riffSize := binary.LittleEndian.Uint32(raw[4:8])
	wantRIFFSize := uint32(36) + wantDataSize
	if riffSize != wantRIFFSize {
		t.Errorf("RIFF size header: got %d want %d", riffSize, wantRIFFSize)
	}
}

func TestWriteCombinedWAV_EmptyPCMSegment(t *testing.T) {
	// A pause-only segment (empty PCM, non-zero leading silence) should
	// not panic and the combined output should contain exactly the silence
	// bytes.
	segments := []wavSegment{
		{pcm: nil, leadingSilenceMs: 250}, // 250 ms = 12000 bytes
	}
	var buf bytes.Buffer
	if err := writeCombinedWAV(&buf, render.DefaultFormat(), segments); err != nil {
		t.Fatalf("writeCombinedWAV with empty PCM: %v", err)
	}
	_, pcm, err := parseWAV("empty.wav", buf.Bytes(), render.DefaultFormat())
	if err != nil {
		t.Fatalf("parseWAV: %v", err)
	}
	if len(pcm) != 12000 {
		t.Errorf("expected 12000 silence bytes, got %d", len(pcm))
	}
	for i, b := range pcm {
		if b != 0 {
			t.Errorf("non-zero silence at byte %d", i)
			break
		}
	}
}

func TestWriteCombinedWAV_Deterministic(t *testing.T) {
	// Two runs with identical input → byte-identical output. Load-bearing
	// for the idempotent rewrite AC.
	pcm := []byte{0x10, 0x20, 0x30, 0x40, 0x50, 0x60}
	segments := []wavSegment{
		{pcm: pcm, leadingSilenceMs: 75},
		{pcm: pcm, leadingSilenceMs: 100},
	}
	var a, b bytes.Buffer
	if err := writeCombinedWAV(&a, render.DefaultFormat(), segments); err != nil {
		t.Fatal(err)
	}
	if err := writeCombinedWAV(&b, render.DefaultFormat(), segments); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(a.Bytes(), b.Bytes()) {
		t.Errorf("non-deterministic output: lengths %d vs %d", a.Len(), b.Len())
	}
}
