// Table-driven tests for the pure WAV header-strip (issue #101, productionized
// from the #100 spike). Build-tag-free: runs in the default `make test` with no
// build tag, because stripWAVToPCM has no audio-engine dependency.
package main

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"testing"
)

// wavBuilder assembles a minimal RIFF/WAVE byte stream for tests. It is
// deliberately hand-rolled (not a library) so the on-wire bytes — including the
// exact chunk order and the canonical 44-byte header layout — are explicit and
// auditable in the test.
type chunk struct {
	id   string
	body []byte
}

func buildWAV(chunks ...chunk) []byte {
	var inner bytes.Buffer
	for _, c := range chunks {
		inner.WriteString(c.id)
		var sz [4]byte
		binary.LittleEndian.PutUint32(sz[:], uint32(len(c.body)))
		inner.Write(sz[:])
		inner.Write(c.body)
		// Word-alignment pad byte for odd-length bodies.
		if len(c.body)%2 == 1 {
			inner.WriteByte(0)
		}
	}
	var out bytes.Buffer
	out.WriteString("RIFF")
	var sz [4]byte
	binary.LittleEndian.PutUint32(sz[:], uint32(4+inner.Len())) // "WAVE" + chunks
	out.Write(sz[:])
	out.WriteString("WAVE")
	out.Write(inner.Bytes())
	return out.Bytes()
}

// canonicalFmt is the 16-byte PCM `fmt ` body: PCM/mono/24kHz/int16. Its size
// makes the classic header land at exactly 44 bytes before `data`.
func canonicalFmt() []byte {
	b := make([]byte, 16)
	binary.LittleEndian.PutUint16(b[0:2], 1)     // PCM
	binary.LittleEndian.PutUint16(b[2:4], 1)     // mono
	binary.LittleEndian.PutUint32(b[4:8], 24000) // sample rate
	binary.LittleEndian.PutUint32(b[8:12], 48000)
	binary.LittleEndian.PutUint16(b[12:14], 2)  // block align
	binary.LittleEndian.PutUint16(b[14:16], 16) // bits per sample
	return b
}

func TestStripWAVToPCM(t *testing.T) {
	pcm8 := []byte{1, 2, 3, 4, 5, 6, 7, 8}

	tests := []struct {
		name     string
		input    []byte
		want     []byte // expected PCM bytes read out
		wantErr  bool
		errIs    error
		overRead bool // declared data size exceeds bytes actually present
	}{
		{
			name:  "canonical 44-byte header",
			input: buildWAV(chunk{"fmt ", canonicalFmt()}, chunk{"data", pcm8}),
			want:  pcm8,
		},
		{
			name: "extra LIST and fact chunks before data",
			input: buildWAV(
				chunk{"fmt ", canonicalFmt()},
				chunk{"LIST", []byte("INFOISFT\x00\x00\x00\x00")},
				chunk{"fact", []byte{0x04, 0, 0, 0}},
				chunk{"data", pcm8},
			),
			want: pcm8,
		},
		{
			name: "odd-length ancillary chunk before data (pad byte skipped)",
			input: buildWAV(
				chunk{"fmt ", canonicalFmt()},
				chunk{"LIST", []byte("INFOX")}, // 5 bytes → 1 pad byte
				chunk{"data", pcm8},
			),
			want: pcm8,
		},
		{
			name: "truncated data chunk: declared longer than present",
			// Hand-build so the `data` size says 8 but only 3 bytes follow.
			input:    truncatedDataWAV(canonicalFmt(), []byte{9, 9, 9}, 8),
			want:     []byte{9, 9, 9},
			overRead: true,
		},
		{
			name:    "not a RIFF stream",
			input:   []byte("NOPExxxxWAVE"),
			wantErr: true,
			errIs:   errNotRIFFWave,
		},
		{
			name:    "RIFF but not WAVE",
			input:   append([]byte("RIFF"), append(le32(100), []byte("AVIxfmt ")...)...),
			wantErr: true,
			errIs:   errNotRIFFWave,
		},
		{
			name:    "no data chunk (walk runs off the end)",
			input:   buildWAV(chunk{"fmt ", canonicalFmt()}),
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := stripWAVToPCM(bytes.NewReader(tc.input))
			if tc.wantErr {
				if err == nil {
					t.Fatalf("want error, got nil")
				}
				if tc.errIs != nil && !errors.Is(err, tc.errIs) {
					t.Fatalf("err = %v, want errors.Is %v", err, tc.errIs)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !bytes.Equal(got, tc.want) {
				t.Fatalf("PCM bytes = %v, want %v", got, tc.want)
			}
			// For the canonical case, assert the strip stopped exactly at the
			// declared length and did not bleed trailing bytes.
			if !tc.overRead && len(got) != len(tc.want) {
				t.Fatalf("read %d bytes, want exactly %d", len(got), len(tc.want))
			}
		})
	}
}

// TestStripWAVToPCM_Seekable pins the #101 seek substrate (issue #77 model): the
// stripped PCM, wrapped in a *bytes.Reader, is natively an io.ReadSeeker +
// io.ReaderAt where offset-zero is the first PCM sample. A Seek(n, SeekStart)
// then a 1-byte read returns the PCM byte at offset n — the round-trip the
// future block-seek model relies on. Wiring actual seek keybindings is out of
// scope; this only proves the source is seekable.
func TestStripWAVToPCM_Seekable(t *testing.T) {
	pcm := []byte{10, 20, 30, 40, 50, 60, 70, 80}
	wav := buildWAV(chunk{"fmt ", canonicalFmt()}, chunk{"data", pcm})

	got, err := stripWAVToPCM(bytes.NewReader(wav))
	if err != nil {
		t.Fatalf("stripWAVToPCM: %v", err)
	}
	if !bytes.Equal(got, pcm) {
		t.Fatalf("stripped PCM = %v, want %v", got, pcm)
	}

	// The player source is bytes.NewReader(pcm) — assert it satisfies the seek
	// substrate interfaces the production player hands to oto.
	src := bytes.NewReader(got)
	var _ io.ReadSeeker = src
	var _ io.ReaderAt = src

	// offset-zero == first PCM sample.
	if got[0] != pcm[0] {
		t.Fatalf("offset-zero byte = %d, want first sample %d", got[0], pcm[0])
	}

	// Seek/read round-trip at every offset: Seek(n) then read one byte == pcm[n].
	for n := 0; n < len(pcm); n++ {
		off, serr := src.Seek(int64(n), io.SeekStart)
		if serr != nil {
			t.Fatalf("Seek(%d): %v", n, serr)
		}
		if off != int64(n) {
			t.Fatalf("Seek(%d) returned offset %d", n, off)
		}
		var one [1]byte
		if _, rerr := io.ReadFull(src, one[:]); rerr != nil {
			t.Fatalf("read after Seek(%d): %v", n, rerr)
		}
		if one[0] != pcm[n] {
			t.Fatalf("byte at offset %d = %d, want %d", n, one[0], pcm[n])
		}
	}
}

func le32(v uint32) []byte {
	b := make([]byte, 4)
	binary.LittleEndian.PutUint32(b, v)
	return b
}

// truncatedDataWAV hand-builds a WAV whose `data` chunk DECLARES declaredSize
// bytes but actually carries only len(body) bytes, to exercise the bounded
// reader against an over-declared (truncated) data chunk.
func truncatedDataWAV(fmtBody, body []byte, declaredSize uint32) []byte {
	var inner bytes.Buffer
	inner.WriteString("fmt ")
	inner.Write(le32(uint32(len(fmtBody))))
	inner.Write(fmtBody)
	inner.WriteString("data")
	inner.Write(le32(declaredSize)) // lies: says declaredSize, writes len(body)
	inner.Write(body)

	var out bytes.Buffer
	out.WriteString("RIFF")
	out.Write(le32(uint32(4 + inner.Len())))
	out.WriteString("WAVE")
	out.Write(inner.Bytes())
	return out.Bytes()
}
