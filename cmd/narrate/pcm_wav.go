// WAV/RIFF header-strip → bounded raw-PCM io.Reader for the listen path
// (spike #100). Pure byte logic with NO audio-engine dependency, so it compiles
// in the default build and is unit-testable without the `oto` build tag.
//
// Why it exists: the renderer emits one `audio.wav` per block (24 kHz mono
// int16, Kokoro native). The oto in-process player wants a reader of RAW PCM
// frames, not a RIFF container — so we walk the chunk list to the `data` chunk
// and hand oto an io.Reader positioned at the first sample byte, bounded to the
// declared data length. No resampling at our layer (CLAUDE.md: 24 kHz native).
//
// This reader is the one component promoted to #101 (the production player), so
// it is unit-tested rather than treated as throwaway scaffolding.
package main

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
)

// pcmReadError flags a malformed RIFF/WAV stream. Distinct sentinel so callers
// (and tests) can tell a parse failure from an I/O failure on the underlying
// file.
var errNotRIFFWave = errors.New("pcm: not a RIFF/WAVE stream")

// newPCMReader consumes the RIFF header of r, walks the chunk list to the
// `data` chunk (skipping `fmt `, `LIST`, `fact`, and any other ancillary
// chunks), and returns an io.Reader positioned at the first PCM sample byte and
// bounded to the `data` chunk's declared length.
//
// It deliberately does NOT assume the canonical 44-byte header — a real Kokoro
// WAV, or any tool-rewritten one, can carry a `LIST`/`fact` chunk before
// `data`. The walk is the honest way to find the sample bytes (mirrors the
// walk-to-`data` rule already used by the playback-observability work).
//
// Bounding to the declared length means a truncated file (declared size larger
// than the bytes actually present) yields a short read + EOF rather than
// over-reading into whatever follows; the underlying reader returning EOF early
// is surfaced to the caller (oto), which stops cleanly.
//
// TODO(#101-followup): RIFF-parse the dataChunkStart byte offset from an
// io.ReaderAt and return it alongside the reader, so the production player can
// io.Seek(offset+n, …) for mid-block and jump-to-block seeking. The spike only
// needs a forward-only io.Reader, so offset-resume / io.Seeker support is
// intentionally out of scope here — what/where/why: the seek math belongs to
// the productionized single-path player, not this throwaway build-tagged spike.
func newPCMReader(r io.Reader) (io.Reader, error) {
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
			// Position is now the first PCM sample byte. Bound the reader to the
			// declared data length so we never over-read past the samples.
			return io.LimitReader(r, int64(size)), nil
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
