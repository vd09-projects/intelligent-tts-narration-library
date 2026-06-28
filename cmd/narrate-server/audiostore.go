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

// audioLeaf is the served artifact inside a render_id dir. A /narrate render
// now writes a 3-file persistent-sink directory ({tempRoot}/{id}/{audio.wav,
// plan.json, manifest.json}, #125) so the dir is patchable by render_id via
// sink/persistent.PatchBlock — GET /audio/{id}.wav serves {id}/audio.wav.
const audioLeaf = "audio.wav"

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

// reserve returns a fresh render_id and its absolute output-DIR path WITHOUT yet
// registering a map entry. The 3-file persistent dir is written FIRST (by the
// /narrate handler), then commit() registers the id — so a crash/fault between
// reserve and commit leaves at most an untracked directory on disk (reaped by
// the orphan scan), never a map entry pointing at a missing artifact. Matches
// the Failure-Mode table's "crash between sink write and store mint insert" row.
func (s *renderStore) reserve() (id, dir string, err error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", "", fmt.Errorf("mint render_id: %w", err)
	}
	id = hex.EncodeToString(b[:])
	return id, filepath.Join(s.tempRoot, id), nil
}

// dirFor resolves a tracked render_id to its absolute, containment-checked
// output directory ({tempRoot}/{id}) for the POST /narrate/block escalate path.
// The render_id is resolved INTERNALLY here — it is NEVER a user-supplied path,
// so the #109 arbitrary-file-read vector stays closed. Returns the closed-enum
// ErrorResponse + HTTP status on a malformed id (missing_field), an unknown or
// expired id (source_not_found/404), or a containment failure (source_not_found).
func (s *renderStore) dirFor(id string) (string, int, *ErrorResponse) {
	// Allowlist-before-join: a traversal/separator id never reaches the fs.
	if !renderIDPattern.MatchString(id) {
		return "", http.StatusBadRequest,
			&ErrorResponse{Reason: reasonMissingField, Message: "render id must be 32 hex chars: " + id}
	}
	s.mu.RLock()
	_, tracked := s.entries[id]
	s.mu.RUnlock()
	if !tracked {
		return "", http.StatusNotFound,
			&ErrorResponse{Reason: reasonSourceNotFound, Message: "unknown or expired render id: " + id}
	}
	// Containment: resolve {tempRoot}/{id} with the same EvalSymlinks +
	// filepath.Rel boundary check the serve + /artifact paths use.
	resolved, status, errResp := resolveWithin(s.tempRoot, id)
	if errResp != nil {
		return "", status, errResp
	}
	return resolved, http.StatusOK, nil
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

	// Containment: resolve {tempRoot}/{id}/audio.wav with the SAME EvalSymlinks +
	// filepath.Rel boundary check /artifact uses (generalised to resolveWithin).
	// The served leaf moved under the render_id dir (#125 3-file layout); the
	// public URL contract (/audio/{id}.wav) is unchanged.
	served, status, errResp := resolveWithin(s.tempRoot, filepath.Join(id, audioLeaf))
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
// tempRoot for untracked render_id dirs past the TTL (R-NB2 — covers a crash
// between the sink write and commit). All os.RemoveAll I/O happens with the
// store lock NOT held (R1). Each render_id is now a DIRECTORY (#125 3-file
// layout), so cleanup is os.RemoveAll, not os.Remove.
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

	// Delete expired dirs lock-free. The open fd of any in-flight serve keeps
	// that serve's bytes readable after the RemoveAll unlinks audio.wav (POSIX
	// open-fd survival, #62) — read-during-write consistency is preserved.
	//
	// FOLLOWUP(reaper-vs-escalate-ttl) (tracked: #128): the reaper does NOT take the per-dir
	// dirLocks mutex, so it can RemoveAll a render_id dir while a concurrent POST
	// /narrate/block is mid-escalate on the same id (escalate holds only the
	// per-dir mutex, not the store lock). The window degrades gracefully — the
	// escalate's readManifest/patch then surfaces a clean source_not_found/
	// internal error, no corruption and no cross-dir blast — but an escalate
	// in-flight should arguably refresh the id's TTL or take the per-dir lock so
	// a long re-render is not reaped under it. Left as-is: narrow window, graceful
	// failure, TTL is minutes; tightening it belongs in a focused change.
	for _, id := range expired {
		_ = os.RemoveAll(filepath.Join(s.tempRoot, id))
	}

	// Orphan scan (R-NB2): remove any render_id DIR in tempRoot with no map
	// entry whose mtime is older than the cutoff. A just-created-not-yet-
	// committed dir has mtime ≈ now (NOT before cutoff) — the crash-window mtime
	// heuristic — so it is never reaped inside its TTL.
	dirents, err := os.ReadDir(s.tempRoot)
	if err != nil {
		return
	}
	for _, de := range dirents {
		name := de.Name()
		if !de.IsDir() || !renderIDPattern.MatchString(name) {
			continue
		}
		if _, isTracked := tracked[name]; isTracked {
			continue
		}
		fi, ferr := de.Info()
		if ferr != nil {
			continue
		}
		if fi.ModTime().Before(cutoff) {
			_ = os.RemoveAll(filepath.Join(s.tempRoot, name))
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
