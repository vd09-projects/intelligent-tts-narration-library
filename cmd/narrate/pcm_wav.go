// WAV/RIFF header-strip → in-memory raw-PCM bytes for the listen path
// (issue #101, productionized from the #100 spike). Pure byte logic with NO
// audio-engine dependency, so it is unit-testable without a real device.
//
// Why it exists: the renderer emits one `audio.wav` per block (24 kHz mono
// int16, Kokoro native). The in-process oto player wants RAW PCM frames, not a
// RIFF container — so we walk the chunk list to the `data` chunk and return the
// declared-length sample bytes, with no resampling at our layer (CLAUDE.md:
// 24 kHz native).
//
// Returning a []byte (rather than an io.Reader over the live *os.File) is the
// load-bearing #101 change. The caller wraps it in a *bytes.Reader, which is
// natively an io.ReadSeeker + io.ReaderAt: it has no file descriptor, so the
// oto player's finalizer-driven teardown can never read a closed fd, and the
// seekable source is the substrate the future block-seek model (#77) needs
// — offset-zero is the first PCM sample. Wiring actual seek keybindings is out
// of scope here; this only makes the source seekable.
package main

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
)

// errNotRIFFWave flags a malformed RIFF/WAV stream. Distinct sentinel so callers
// (and tests) can tell a parse failure from an I/O failure on the underlying
// file.
var errNotRIFFWave = errors.New("pcm: not a RIFF/WAVE stream")

// stripWAVToPCM consumes the RIFF header of r, walks the chunk list to the
// `data` chunk (skipping `fmt `, `LIST`, `fact`, and any other ancillary
// chunks), reads the declared-length sample bytes fully into memory, and
// returns them as raw PCM. The whole block buffer is in hand on return, so the
// caller can construct a player only after the full PCM is loaded (no player is
// ever built over a partially-read source).
//
// It deliberately does NOT assume the canonical 44-byte header — a real Kokoro
// WAV, or any tool-rewritten one, can carry a `LIST`/`fact` chunk before
// `data`. The walk is the honest way to find the sample bytes.
//
// Bounding to the declared length means a truncated file (declared size larger
// than the bytes actually present) yields the bytes that ARE present rather than
// over-reading into whatever follows; the underlying reader returning EOF early
// simply ends the data short.
//
// The returned []byte wrapped in bytes.NewReader is an io.ReadSeeker +
// io.ReaderAt with offset-zero == first PCM sample — the seek substrate the #77
// block-seek model builds on.
func stripWAVToPCM(r io.Reader) ([]byte, error) {
	// RIFF master header: "RIFF" <uint32 chunkSize> "WAVE" (12 bytes).
	var hdr [12]byte
	if _, err := io.ReadFull(r, hdr[:]); err != nil {
		return nil, fmt.Errorf("pcm: read RIFF header: %w", err)
	}
	if string(hdr[0:4]) != "RIFF" || string(hdr[8:12]) != "WAVE" {
		return nil, errNotRIFFWave
	}

	// Walk chunks until `data`. Each chunk is an 8-byte header (4-byte ASCII id
	// + uint32 little-endian size) followed by `size` bytes of body. Chunk
	// bodies are word-aligned: an odd `size` carries a trailing pad byte that is
	// not counted in `size`, so we skip it before reading the next header.
	var chunkHdr [8]byte
	for {
		if _, err := io.ReadFull(r, chunkHdr[:]); err != nil {
			return nil, fmt.Errorf("pcm: walk to data chunk: %w", err)
		}
		id := string(chunkHdr[0:4])
		size := binary.LittleEndian.Uint32(chunkHdr[4:8])

		if id == "data" {
			// Position is now the first PCM sample byte. Read the declared data
			// length fully into memory, bounded so we never over-read past the
			// samples. A truncated file yields the present bytes (short read).
			pcm, err := io.ReadAll(io.LimitReader(r, int64(size)))
			if err != nil {
				return nil, fmt.Errorf("pcm: read data chunk: %w", err)
			}
			return pcm, nil
		}

		// Skip this chunk's body plus any word-alignment pad byte.
		skip := int64(size)
		if size%2 == 1 {
			skip++
		}
		if _, err := io.CopyN(io.Discard, r, skip); err != nil {
			return nil, fmt.Errorf("pcm: skip %q chunk: %w", id, err)
		}
	}
}
