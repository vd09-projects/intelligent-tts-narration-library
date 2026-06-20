package persistent

import "os"

// tempFile is the minimal surface the atomic-write paths (writeAudio in
// persistent.go, atomicWriteFile in manifest.go) need from a freshly-opened
// temp file: stream bytes in, fix permissions, name it for the rename, and
// close. *os.File satisfies it in production.
//
// Extracted as an interface (issue #29) so tests can inject a temp file whose
// Write fails partway through — exercising the "ctx cancelled / writer errors
// mid-write" branch and asserting tmp+rename leaves no partial canonical file
// on disk. The atomic-write guarantee was already structural; this makes the
// failure case a concrete unit test.
type tempFile interface {
	Write(p []byte) (int, error)
	Chmod(mode os.FileMode) error
	Close() error
	Name() string
}

// openTempFile is the package-level seam mirroring sink/ephemeral's
// `var play playFunc`. Production opens a real temp file via os.CreateTemp;
// tests swap this var to inject a temp file whose writer fails after N bytes.
// Package-level (not a Sink field) to keep the public Sink struct minimal and
// the zero-value path clean — same rationale as the ephemeral play seam.
//
// The returned tempFile must already exist on disk under a real path (so the
// best-effort os.Remove cleanup on the failure path has something to remove);
// the production os.CreateTemp impl satisfies this naturally.
var openTempFile = func(dir, pattern string) (tempFile, error) {
	return os.CreateTemp(dir, pattern)
}
