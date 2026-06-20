package persistent

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
)

// errWriteCancelled stands in for "ctx cancelled or the writer errored after
// N bytes mid-write" — the failure mode issue #29 makes concrete. Using a
// distinct sentinel lets the assertions confirm the error propagates verbatim
// (wrapped) rather than being swallowed by the tmp+rename ceremony.
var errWriteCancelled = errors.New("persistent_test: write cancelled mid-stream")

// failAfterNTempFile wraps a real temp file but makes its Write fail once a
// cumulative N bytes have been written. The real *os.File underneath gives the
// failure path a genuine on-disk tmp to clean up (os.Remove) and a real
// directory entry for the rename that must never happen.
type failAfterNTempFile struct {
	real    *os.File
	limit   int // fail the Write that pushes cumulative bytes past this
	written int
}

func (f *failAfterNTempFile) Write(p []byte) (int, error) {
	if f.written >= f.limit {
		return 0, errWriteCancelled
	}
	// Write up to the limit so partial bytes really land in the tmp file —
	// the whole point is to prove a partially-written tmp never gets renamed
	// over the canonical path.
	room := f.limit - f.written
	if room > len(p) {
		room = len(p)
	}
	n, err := f.real.Write(p[:room])
	f.written += n
	if err != nil {
		return n, err
	}
	if room < len(p) {
		return n, errWriteCancelled
	}
	return n, nil
}

func (f *failAfterNTempFile) Chmod(mode os.FileMode) error { return f.real.Chmod(mode) }
func (f *failAfterNTempFile) Close() error                 { return f.real.Close() }
func (f *failAfterNTempFile) Name() string                 { return f.real.Name() }

// withFailAfterNWrite swaps the package-level openTempFile seam so the next
// temp file opened fails its Write after limit bytes. Restores the production
// seam on cleanup. Tests using it must not run in parallel — the seam is
// package-level, same documented constraint as sink/ephemeral's play seam.
func withFailAfterNWrite(t *testing.T, limit int) {
	t.Helper()
	orig := openTempFile
	openTempFile = func(dir, pattern string) (tempFile, error) {
		real, err := os.CreateTemp(dir, pattern)
		if err != nil {
			return nil, err
		}
		return &failAfterNTempFile{real: real, limit: limit}, nil
	}
	t.Cleanup(func() { openTempFile = orig })
}

// TestConsume_WriterErrorMidWrite_NoPartialAudio is the issue #29 fidelity
// case: a writer that errors mid-writeCombinedWAV must leave NO canonical
// audio.wav on disk. The atomic tmp+rename guarantee was already structural;
// this exercises it through a concrete injected failure.
func TestConsume_WriterErrorMidWrite_NoPartialAudio(t *testing.T) {
	p, res := twoBlockPlan(t)
	outDir := t.TempDir()
	s := New(outDir, WithVoice("af_bella"))

	// Fail after 20 bytes — past the 44-byte WAV header start, so the tmp
	// file is genuinely partially written when the error fires.
	withFailAfterNWrite(t, 20)

	_, err := s.Consume(context.Background(), p, res)
	if err == nil {
		t.Fatal("expected Consume to fail when the tmp writer errors mid-write")
	}
	if !errors.Is(err, errWriteCancelled) {
		t.Fatalf("expected wrapped errWriteCancelled, got: %v", err)
	}

	// The canonical audio.wav must NOT exist — a partial tmp was written but
	// never renamed into place.
	if _, statErr := os.Stat(filepath.Join(outDir, audioFilename)); !os.IsNotExist(statErr) {
		t.Fatalf("expected no canonical %s after mid-write failure, stat err = %v",
			audioFilename, statErr)
	}

	// And no orphan tmp files (.persistent-audio-*.wav) should linger — the
	// failure path removes the tmp best-effort.
	assertNoTmpLeftovers(t, outDir, ".persistent-audio-")
}

// TestConsume_WriterError_PreservesExistingAudio proves the atomic guarantee
// directly: a successful write, then a failing rewrite must leave the FIRST
// audio.wav byte-for-byte intact (no truncation, no partial overwrite).
func TestConsume_WriterError_PreservesExistingAudio(t *testing.T) {
	p, res := twoBlockPlan(t)
	outDir := t.TempDir()
	s := New(outDir, WithVoice("af_bella"))

	if _, err := s.Consume(context.Background(), p, res); err != nil {
		t.Fatalf("first Consume (should succeed): %v", err)
	}
	original, err := os.ReadFile(filepath.Join(outDir, audioFilename))
	if err != nil {
		t.Fatalf("read original audio.wav: %v", err)
	}

	// Second write fails mid-stream.
	withFailAfterNWrite(t, 20)
	if _, err := s.Consume(context.Background(), p, res); !errors.Is(err, errWriteCancelled) {
		t.Fatalf("expected mid-write failure on second Consume, got: %v", err)
	}

	after, err := os.ReadFile(filepath.Join(outDir, audioFilename))
	if err != nil {
		t.Fatalf("audio.wav vanished after failed rewrite: %v", err)
	}
	if string(after) != string(original) {
		t.Fatalf("audio.wav changed after a failed rewrite: original %d bytes, now %d bytes",
			len(original), len(after))
	}
}

// TestAtomicWriteFile_WriterError_NoPartialFile extends the same guarantee to
// the JSON outputs (plan.json / manifest.json route through atomicWriteFile):
// a writer that errors leaves no canonical file behind.
func TestAtomicWriteFile_WriterError_NoPartialFile(t *testing.T) {
	outDir := t.TempDir()
	target := filepath.Join(outDir, planFilename)
	payload := []byte(`{"schema_version":1,"blocks":[]}` + "\n")

	withFailAfterNWrite(t, 8) // fail partway through the payload

	err := atomicWriteFile(target, payload, 0o644)
	if !errors.Is(err, errWriteCancelled) {
		t.Fatalf("expected wrapped errWriteCancelled from atomicWriteFile, got: %v", err)
	}
	if _, statErr := os.Stat(target); !os.IsNotExist(statErr) {
		t.Fatalf("expected no canonical %s after mid-write failure, stat err = %v",
			planFilename, statErr)
	}
	assertNoTmpLeftovers(t, outDir, ".persistent-")
}

// assertNoTmpLeftovers fails if any temp file matching prefix lingers in dir —
// confirms the best-effort os.Remove on the failure path actually ran.
func assertNoTmpLeftovers(t *testing.T, dir, prefix string) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read outDir: %v", err)
	}
	for _, e := range entries {
		if name := e.Name(); len(name) >= len(prefix) && name[:len(prefix)] == prefix {
			t.Errorf("orphan tmp file left behind: %s", name)
		}
	}
}

// Compile-time guard: the seam's interface stays a superset of what *os.File
// offers, so the production assignment in tmpfile.go keeps type-checking.
var _ tempFile = (*failAfterNTempFile)(nil)
var _ io.Writer = (*failAfterNTempFile)(nil)
