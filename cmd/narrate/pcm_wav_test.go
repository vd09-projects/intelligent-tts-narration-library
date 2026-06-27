// Table-driven tests for the pure WAV header-strip reader (spike #100,
// suggestion 1). Build-tag-free: runs in the default `make test`, no `oto` tag
// required, because pcmReader has no audio-engine dependency.
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

func TestNewPCMReader(t *testing.T) {
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
			r, err := newPCMReader(bytes.NewReader(tc.input))
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
			got, rerr := io.ReadAll(r)
			if rerr != nil {
				t.Fatalf("ReadAll: %v", rerr)
			}
			if !bytes.Equal(got, tc.want) {
				t.Fatalf("PCM bytes = %v, want %v", got, tc.want)
			}
			// For the canonical case, assert the bounded reader stopped exactly
			// at the declared length and did not bleed trailing bytes.
			if !tc.overRead && len(got) != len(tc.want) {
				t.Fatalf("read %d bytes, want exactly %d", len(got), len(tc.want))
			}
		})
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
