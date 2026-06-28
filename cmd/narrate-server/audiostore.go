package main

// audiostore.go — the render store, the GET /audio/{render_id}.wav serve path,
// and the TTL GC reaper (#109 Steps 5–7).
//
// # Lock discipline (NON-NEGOTIABLE — plan R1)
//
// The store's sync.RWMutex is held ONLY across map operations + os.Open. It is
// NEVER held across a network-bound http.ServeContent or across an os.Remove:
//
//   - /audio takes the read-lock only across {entry lookup, resolveWithin,
//     os.Open}, RELEASES it, then serves from the already-open *os.File. On
//     POSIX an open fd keeps the file's bytes readable after unlink, so the
//     reaper may delete a wav whose serve is still streaming without tearing it.
//   - the reaper takes the write-lock only to snapshot expired entries and drop
//     their map keys (O(1) per entry, no I/O), RELEASES it, then os.Removes the
//     snapshotted files lock-free.
//
// Therefore a slow in-flight serve can never starve a concurrent /narrate mint
// (Go's RWMutex blocks new readers once a writer waits — so a serve that held
// the read-lock across ServeContent would let a waiting reaper write-lock stall
// every new mint). The wall-clock liveness test is the proof obligation,
// separate from -race.

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

// renderIDPattern — a render_id is exactly 32 lowercase hex chars (16 bytes of
// crypto/rand). Allowlist-before-join: validated BEFORE the id is ever joined
// onto tempRoot, so a traversal/separator id never reaches the filesystem.
var renderIDPattern = regexp.MustCompile(`^[0-9a-f]{32}$`)

// defaultReaperInterval bounds how often the GC sweep runs. The effective
// interval is min(this, audioTTL) so a short --audio-ttl still gets swept
// promptly (and the test injecting a tiny TTL fires deterministically).
const defaultReaperInterval = time.Minute

// renderMeta is the per-id bookkeeping the store keeps. createdAt is stamped
// from the store clock (s.now) so a fault-injected clock makes the reaper fire
// deterministically in tests.
type renderMeta struct {
	createdAt time.Time
}

// renderStore owns the server's temp wav root, the id→meta map, and the
// RWMutex guarding it. It is NOT a package global (testability): serve()
// constructs one over an os.MkdirTemp root and threads it into the handlers.
type renderStore struct {
	tempRoot string
	mu       sync.RWMutex
	entries  map[string]renderMeta
	now      func() time.Time // injectable clock; defaults to time.Now
}

// newRenderStore constructs a store rooted at tempRoot (already created by the
// caller). The clock defaults to time.Now; tests overwrite store.now.
func newRenderStore(tempRoot string) *renderStore {
	return &renderStore{
		tempRoot: tempRoot,
		entries:  make(map[string]renderMeta),
		now:      time.Now,
	}
}

// reserve returns a fresh render_id and its absolute wav path WITHOUT yet
// registering a map entry. The wav is written FIRST (by the /narrate handler),
// then commit() registers the id — so a crash/fault between reserve and commit
// leaves at most an untracked file on disk (reaped by the orphan scan), never a
// map entry pointing at a missing file. Matches the Failure-Mode table's
// "crash between WAVFileSink write and store mint insert" row.
func (s *renderStore) reserve() (id, path string, err error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", "", fmt.Errorf("mint render_id: %w", err)
	}
	id = hex.EncodeToString(b[:])
	return id, filepath.Join(s.tempRoot, id+".wav"), nil
}

// commit registers a reserved id after its wav is on disk. createdAt is stamped
// now so the TTL clock starts at publish time.
func (s *renderStore) commit(id string) {
	s.mu.Lock()
	s.entries[id] = renderMeta{createdAt: s.now()}
	s.mu.Unlock()
}

// open resolves a render_id to a live *os.File, taking the read-lock ONLY
// across {map lookup, resolveWithin, os.Open} and RETURNING the open file with
// the lock already released (R1). The caller serves from the fd; the reaper may
// unlink underneath it safely (POSIX open-fd survival). Returns the closed-enum
// ErrorResponse + HTTP status on any miss.
func (s *renderStore) open(id string) (*os.File, int, *ErrorResponse) {
	s.mu.RLock()
	_, tracked := s.entries[id]
	if !tracked {
		s.mu.RUnlock()
		return nil, http.StatusNotFound,
			&ErrorResponse{Reason: reasonSourceNotFound, Message: "unknown or expired render id: " + id}
	}

	// Containment: resolve {tempRoot}/{id}.wav with the SAME EvalSymlinks +
	// filepath.Rel boundary check /artifact uses (generalised to resolveWithin).
	served, status, errResp := resolveWithin(s.tempRoot, id+".wav")
	if errResp != nil {
		s.mu.RUnlock()
		return nil, status, errResp
	}

	f, err := os.Open(served) //nolint:gosec // path is containment-checked by resolveWithin
	s.mu.RUnlock()            // RELEASE before serving — the serve never holds the store lock.
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			// Raced delete between the map check and open: still caller-correctable.
			return nil, http.StatusNotFound,
				&ErrorResponse{Reason: reasonSourceNotFound, Message: "render artifact not found: " + id}
		}
		return nil, http.StatusInternalServerError,
			&ErrorResponse{Reason: reasonInternal, Message: "open render artifact: " + err.Error()}
	}
	return f, http.StatusOK, nil
}

// serveAudio is the GET /audio/{render_id}.wav handler. The route is registered
// as "/audio/{file}" (Go 1.22 wildcards must be a whole path segment, so the
// ".wav" suffix is parsed off here rather than in the pattern).
func (s *renderStore) serveAudio(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, reasonMethodNotAllowed, "GET only")
		return
	}

	file := r.PathValue("file")
	// Allowlist-before-join: the leaf must be {32-hex}.wav. A traversal /
	// separator / wrong-suffix leaf is a malformed id (missing_field), rejected
	// before any filesystem touch.
	id, ok := strings.CutSuffix(file, ".wav")
	if !ok || !renderIDPattern.MatchString(id) {
		writeError(w, http.StatusBadRequest, reasonMissingField,
			fmt.Sprintf("render id must be 32 hex chars + .wav (got %q)", file))
		return
	}

	f, status, errResp := s.open(id)
	if errResp != nil {
		writeError(w, status, errResp.Reason, errResp.Message)
		return
	}
	defer func() { _ = f.Close() }()

	info, err := f.Stat()
	if err != nil {
		writeError(w, http.StatusInternalServerError, reasonInternal, "stat render artifact: "+err.Error())
		return
	}

	w.Header().Set("Content-Type", "audio/wav")
	w.Header().Set("Cache-Control", "no-store")
	// ServeContent (not FileServer) — no listing, just these bytes; honors Range
	// and sets Content-Length. The store lock is NOT held here (R1).
	http.ServeContent(w, r, file, info.ModTime(), f)
}

// reap is one GC sweep: drop+delete expired tracked entries, then orphan-scan
// tempRoot for untracked *.wav past the TTL (R-NB2 — covers a crash between
// WAVFileSink-write and commit). All os.Remove I/O happens with the store lock
// NOT held (R1).
func (s *renderStore) reap(ttl time.Duration) {
	cutoff := s.now().Add(-ttl)

	// Snapshot expired entries + drop their keys UNDER the write-lock (no I/O).
	s.mu.Lock()
	var expired []string
	for id, meta := range s.entries {
		if meta.createdAt.Before(cutoff) {
			expired = append(expired, id)
			delete(s.entries, id)
		}
	}
	// Snapshot the still-tracked id set for the orphan scan's exclusion test.
	tracked := make(map[string]struct{}, len(s.entries))
	for id := range s.entries {
		tracked[id] = struct{}{}
	}
	s.mu.Unlock()

	// Delete expired files lock-free. The open fd of any in-flight serve keeps
	// that serve's bytes readable after this unlink (POSIX).
	for _, id := range expired {
		_ = os.Remove(filepath.Join(s.tempRoot, id+".wav"))
	}

	// Orphan scan (R-NB2): remove any *.wav in tempRoot with no map entry whose
	// mtime is older than the cutoff. A just-written-not-yet-committed wav has
	// mtime ≈ now (NOT before cutoff), so it is never reaped inside its TTL.
	dirents, err := os.ReadDir(s.tempRoot)
	if err != nil {
		return
	}
	for _, de := range dirents {
		name := de.Name()
		oid, ok := strings.CutSuffix(name, ".wav")
		if !ok {
			continue
		}
		if _, isTracked := tracked[oid]; isTracked {
			continue
		}
		fi, ferr := de.Info()
		if ferr != nil || fi.IsDir() {
			continue
		}
		if fi.ModTime().Before(cutoff) {
			_ = os.Remove(filepath.Join(s.tempRoot, name))
		}
	}
}

// runReaper runs reap on a ticker until ctx is cancelled. interval is clamped to
// at most ttl so a tiny --audio-ttl is still swept promptly. RemoveAll(tempRoot)
// on shutdown (in serve) is the back-stop.
func (s *renderStore) runReaper(ctx context.Context, interval, ttl time.Duration) {
	if ttl > 0 && interval > ttl {
		interval = ttl
	}
	if interval <= 0 {
		interval = defaultReaperInterval
	}
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			s.reap(ttl)
		}
	}
}
