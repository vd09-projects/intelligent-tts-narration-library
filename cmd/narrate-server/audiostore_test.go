package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// mintWAV reserves an id, writes payload to {id}/audio.wav, and commits — the
// production /narrate sequence (now a 3-file render_id DIR, #125), condensed for
// store/serve tests.
func mintWAV(t *testing.T, store *renderStore, payload []byte) string {
	t.Helper()
	id, dir, err := store.reserve()
	if err != nil {
		t.Fatalf("reserve: %v", err)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir render dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "audio.wav"), payload, 0o600); err != nil {
		t.Fatalf("write wav: %v", err)
	}
	store.commit(id)
	return id
}

// serveAudioReq drives one request through store.serveAudio with file as the
// {file} path value.
func serveAudioReq(method, file string, store *renderStore) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, "/audio/"+file, nil)
	r.SetPathValue("file", file)
	w := httptest.NewRecorder()
	store.serveAudio(w, r)
	return w
}

// ---------------------------------------------------------------------------
// Serve + containment.
// ---------------------------------------------------------------------------

func TestAudio_ServeAndContainment(t *testing.T) {
	store := newRenderStore(t.TempDir())
	payload := []byte("RIFF....WAVEfmt ....the-audio-bytes")
	id := mintWAV(t, store, payload)

	t.Run("serve_exact_bytes", func(t *testing.T) {
		w := serveAudioReq(http.MethodGet, id+".wav", store)
		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", w.Code)
		}
		if ct := w.Header().Get("Content-Type"); ct != "audio/wav" {
			t.Fatalf("Content-Type = %q, want audio/wav", ct)
		}
		if cc := w.Header().Get("Cache-Control"); cc != "no-store" {
			t.Fatalf("Cache-Control = %q, want no-store", cc)
		}
		if w.Body.String() != string(payload) {
			t.Fatalf("body = %q, want %q", w.Body.String(), payload)
		}
	})

	t.Run("bad_id_missing_field", func(t *testing.T) {
		for _, file := range []string{"nothex.wav", "ABCDEF.wav", id, "short.wav", id + ".wav.bak"} {
			w := serveAudioReq(http.MethodGet, file, store)
			if w.Code != http.StatusBadRequest {
				t.Fatalf("file %q: status = %d, want 400", file, w.Code)
			}
			if got := decodeErr(t, w).Reason; got != reasonMissingField {
				t.Fatalf("file %q: reason = %q, want %q", file, got, reasonMissingField)
			}
		}
	})

	t.Run("unknown_id_source_not_found", func(t *testing.T) {
		w := serveAudioReq(http.MethodGet, strings.Repeat("0", 32)+".wav", store)
		if w.Code != http.StatusNotFound {
			t.Fatalf("status = %d, want 404", w.Code)
		}
		if got := decodeErr(t, w).Reason; got != reasonSourceNotFound {
			t.Fatalf("reason = %q, want %q", got, reasonSourceNotFound)
		}
	})

	t.Run("non_get_method_not_allowed", func(t *testing.T) {
		w := serveAudioReq(http.MethodPost, id+".wav", store)
		if w.Code != http.StatusMethodNotAllowed {
			t.Fatalf("status = %d, want 405", w.Code)
		}
		if got := decodeErr(t, w).Reason; got != reasonMethodNotAllowed {
			t.Fatalf("reason = %q, want %q", got, reasonMethodNotAllowed)
		}
	})

	t.Run("symlink_escape_rejected", func(t *testing.T) {
		// A registered id whose on-disk render dir is a symlink escaping tempRoot
		// must be rejected by resolveWithin (EvalSymlinks + filepath.Rel) when it
		// resolves {id}/audio.wav, not served.
		outsideDir := t.TempDir()
		if err := os.WriteFile(filepath.Join(outsideDir, "audio.wav"), []byte("SECRET"), 0o600); err != nil {
			t.Fatalf("write outside: %v", err)
		}
		evilID := strings.Repeat("a", 32)
		link := filepath.Join(store.tempRoot, evilID)
		if err := os.Symlink(outsideDir, link); err != nil {
			t.Fatalf("symlink: %v", err)
		}
		store.commit(evilID) // register it so the map check passes and resolveWithin is exercised
		w := serveAudioReq(http.MethodGet, evilID+".wav", store)
		if w.Code != http.StatusNotFound {
			t.Fatalf("symlink-escape served (status %d) — containment breached", w.Code)
		}
		if w.Body.String() == "SECRET" {
			t.Fatal("symlink-escape leaked the outside file bytes")
		}
	})
}

// ---------------------------------------------------------------------------
// GC reaper + open-fd survival + orphan scan.
// ---------------------------------------------------------------------------

func TestRenderStore_ReapAndOpenFDSurvival(t *testing.T) {
	store := newRenderStore(t.TempDir())
	clock := time.Unix(1_000_000, 0)
	store.now = func() time.Time { return clock }
	const ttl = time.Minute

	// One old (expired) + one fresh entry.
	oldID := mintWAV(t, store, []byte("old-wav-bytes"))
	clock = clock.Add(2 * time.Minute) // advance so oldID's createdAt is now > ttl ago
	freshID := mintWAV(t, store, []byte("fresh-wav-bytes"))

	// Open the OLD wav's fd BEFORE reaping (open-fd survival check).
	f, status, errResp := store.open(oldID)
	if errResp != nil {
		t.Fatalf("open oldID: status %d %+v", status, errResp)
	}
	defer func() { _ = f.Close() }()

	store.reap(ttl)

	// Expired entry gone from map + disk; fresh kept.
	if _, ok := store.entries[oldID]; ok {
		t.Fatal("expired entry still in map after reap")
	}
	if _, err := os.Stat(filepath.Join(store.tempRoot, oldID)); !os.IsNotExist(err) {
		t.Fatalf("expired render dir still on disk after reap (err=%v)", err)
	}
	if _, ok := store.entries[freshID]; !ok {
		t.Fatal("fresh entry was reaped — should be kept inside its TTL")
	}

	// Open-fd survival: the unlinked old wav's bytes still read from the held fd.
	buf := make([]byte, len("old-wav-bytes"))
	n, _ := f.ReadAt(buf, 0)
	if string(buf[:n]) != "old-wav-bytes" {
		t.Fatalf("held fd after unlink read %q, want old-wav-bytes", buf[:n])
	}
}

func TestRenderStore_OrphanScan(t *testing.T) {
	store := newRenderStore(t.TempDir())
	clock := time.Unix(2_000_000, 0)
	store.now = func() time.Time { return clock }
	const ttl = time.Minute

	// An untracked render_id DIR (e.g. crash between sink write and commit) with
	// an OLD mtime (#125 — orphans are now dirs, reaped by RemoveAll).
	orphan := filepath.Join(store.tempRoot, strings.Repeat("b", 32))
	if err := os.MkdirAll(orphan, 0o755); err != nil {
		t.Fatalf("mkdir orphan: %v", err)
	}
	if err := os.WriteFile(filepath.Join(orphan, "audio.wav"), []byte("orphan"), 0o600); err != nil {
		t.Fatalf("write orphan: %v", err)
	}
	oldMTime := clock.Add(-2 * time.Minute)
	// Chtimes the dir itself (the orphan scan keys on the DIR mtime).
	if err := os.Chtimes(orphan, oldMTime, oldMTime); err != nil {
		t.Fatalf("chtimes orphan: %v", err)
	}
	// A recent untracked render_id dir must be KEPT (inside the TTL window) — the
	// crash-window mtime heuristic protects a just-created-not-yet-committed dir.
	recent := filepath.Join(store.tempRoot, strings.Repeat("c", 32))
	if err := os.MkdirAll(recent, 0o755); err != nil {
		t.Fatalf("mkdir recent: %v", err)
	}
	if err := os.WriteFile(filepath.Join(recent, "audio.wav"), []byte("recent"), 0o600); err != nil {
		t.Fatalf("write recent: %v", err)
	}
	// A non-render_id directory (wrong name shape) must NEVER be reaped.
	stranger := filepath.Join(store.tempRoot, "not-a-render-id")
	if err := os.MkdirAll(stranger, 0o755); err != nil {
		t.Fatalf("mkdir stranger: %v", err)
	}
	if err := os.Chtimes(stranger, oldMTime, oldMTime); err != nil {
		t.Fatalf("chtimes stranger: %v", err)
	}

	store.reap(ttl)

	if _, err := os.Stat(orphan); !os.IsNotExist(err) {
		t.Fatalf("old orphan dir not reaped (err=%v)", err)
	}
	if _, err := os.Stat(recent); err != nil {
		t.Fatalf("recent untracked dir was reaped — should be kept: %v", err)
	}
	if _, err := os.Stat(stranger); err != nil {
		t.Fatalf("non-render_id dir was reaped — orphan scan must ignore it: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Liveness (separate from -race): a slow in-flight /audio serve must NOT block a
// concurrent mint. -race proves no data race but cannot prove this
// non-starvation property — it needs a wall-clock bound.
// ---------------------------------------------------------------------------

func TestRenderStore_Liveness_SlowServeDoesNotBlockMint(t *testing.T) {
	store := newRenderStore(t.TempDir())
	id := mintWAV(t, store, []byte("streaming-body-bytes"))

	// open() releases the store lock before returning the fd (R1); serving from
	// the fd holds no store lock. Simulate a slow serve with a blocking writer.
	f, _, errResp := store.open(id)
	if errResp != nil {
		t.Fatalf("open: %+v", errResp)
	}
	defer func() { _ = f.Close() }()

	bw := &blockingWriter{wrote: make(chan struct{}), release: make(chan struct{})}
	served := make(chan struct{})
	go func() {
		r := httptest.NewRequest(http.MethodGet, "/audio/"+id+".wav", nil)
		info, _ := f.Stat()
		http.ServeContent(bw, r, id+".wav", info.ModTime(), f)
		close(served)
	}()

	<-bw.wrote // the serve is now mid-stream, holding only the open fd

	// A concurrent mint must complete well within a tight deadline.
	done := make(chan struct{})
	go func() {
		id, dir, _ := store.reserve()
		_ = os.MkdirAll(dir, 0o755)
		_ = os.WriteFile(filepath.Join(dir, "audio.wav"), []byte("x"), 0o600)
		store.commit(id)
		// also exercise the reaper write-lock path concurrently
		store.reap(time.Hour)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		close(bw.release)
		<-served
		t.Fatal("mint/reap blocked behind a slow in-flight serve — lock held across ServeContent")
	}

	close(bw.release)
	<-served
}

// blockingWriter is an http.ResponseWriter whose first Write signals (wrote) and
// then blocks until release is closed — so the serve is deterministically held
// mid-stream while the test measures a concurrent mint.
type blockingWriter struct {
	header  http.Header
	wrote   chan struct{}
	release chan struct{}
	once    sync.Once
}

func (b *blockingWriter) Header() http.Header {
	if b.header == nil {
		b.header = make(http.Header)
	}
	return b.header
}
func (b *blockingWriter) WriteHeader(int) {}
func (b *blockingWriter) Write(p []byte) (int, error) {
	b.once.Do(func() { close(b.wrote) })
	<-b.release
	return len(p), nil
}

// ---------------------------------------------------------------------------
// Race discipline — concurrent mint / serve / reap against the store. Run under
// `make test-race`.
// ---------------------------------------------------------------------------

func TestRenderStore_ConcurrentMintServeReap(t *testing.T) {
	store := newRenderStore(t.TempDir())
	// Seed a few so serves have live targets immediately.
	seed := make([]string, 0, 8)
	for range 8 {
		seed = append(seed, mintWAV(t, store, []byte("seed")))
	}

	var wg sync.WaitGroup
	stop := make(chan struct{})

	// Minters.
	for range 4 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
					id, dir, _ := store.reserve()
					_ = os.MkdirAll(dir, 0o755)
					_ = os.WriteFile(filepath.Join(dir, "audio.wav"), []byte("c"), 0o600)
					store.commit(id)
				}
			}
		}()
	}
	// Servers.
	for range 4 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
					serveAudioReq(http.MethodGet, seed[0]+".wav", store)
				}
			}
		}()
	}
	// Reaper.
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-stop:
				return
			default:
				store.reap(time.Millisecond)
			}
		}
	}()

	time.Sleep(100 * time.Millisecond)
	close(stop)
	wg.Wait()
}
