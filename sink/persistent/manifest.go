package persistent

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/vd09-projects/intelligent-tts-narration-library/plan"
)

// ManifestSchemaVersion is the persistent-sink manifest schema version.
//
// Decision (v1.1.0) — schema: experimental. Schema starts at 1 and is
// additive-compatible under the same major version (mirrors plan
// SchemaVersion convention from CLAUDE.md "Schema versioning"). Field
// additions are backward-compatible; field renames / removals require a
// major bump. The const is the single source of truth — Consume writes
// it, tests pin it.
const ManifestSchemaVersion = 1

// Manifest is the build-time receipt the persistent sink emits next to
// audio.wav and plan.json. It carries the data CheckStale needs to
// answer "does this output dir still match its source?" without
// re-rendering — content hash, source URI, voice id, and the per-block
// index (block id ↔ audio_ref ↔ planned start/end ms).
//
// Invariants:
//   - All fields are scalars, structs, or slices — no maps. Field order
//     in encoding/json output follows struct declaration order, so the
//     output is byte-deterministic for unchanged inputs (Decision v1.2.0:
//     no timestamps).
//   - The Voice field carries the voice id populated by the composition
//     root via persistent.WithVoice (Decision v1.8.0). For a plain Kokoro
//     render this is the engine-specific source voice id (e.g. "af_bella");
//     for an RVC character render it is the RVC character slug the listener
//     asked to hear (e.g. "cool-jahns" / "confident-neal") — the honest
//     provenance, not the hidden Kokoro source (Decision D6, issue #146).
//     Empty Voice is a valid manifest — the sink does not invent a voice id.
//   - Stale + StaleReason are present-but-false on a fresh write. CheckStale
//     reads these fields when comparing; Consume never writes Stale=true
//     (auto-regeneration is forbidden by the honesty rule).
type Manifest struct {
	SchemaVersion     int              `json:"schema_version"`
	PlanSchemaVersion string           `json:"plan_schema_version"`
	SourceKind        plan.SourceKind  `json:"source_kind"`
	SourceURI         string           `json:"source_uri"`
	ContentHash       string           `json:"content_hash"`
	AudioFormat       plan.AudioFormat `json:"audio_format"`
	Voice             string           `json:"voice"`
	Blocks            []ManifestBlock  `json:"blocks"`
	Stale             bool             `json:"stale"`
	StaleReason       string           `json:"stale_reason,omitempty"`
}

// ManifestBlock is the per-block index row inside Manifest.Blocks.
//
// Invariant: block-level only. No per-segment, per-word, or per-rune
// fields — CLAUDE.md "Sync is block-level only".
type ManifestBlock struct {
	ID       string      `json:"id"`
	Class    plan.Class  `json:"class"`
	Level    plan.Level  `json:"level"`
	Status   plan.Status `json:"status"`
	StartMs  int         `json:"start_ms"`
	EndMs    int         `json:"end_ms"`
	AudioRef string      `json:"audio_ref,omitempty"`
}

// writeManifest marshals m to JSON (indent two spaces) and writes the
// bytes to path atomically via a sibling tmp file + rename.
//
// Atomicity rationale (Decision v1.4.0): a ctx cancel mid-write must not
// leave a partial manifest.json that CheckStale would mis-parse. The
// honesty rule extends to bytes on disk — partial state is worse than
// absent state.
func writeManifest(path string, m Manifest) error {
	raw, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return fmt.Errorf("persistent: marshal manifest: %w", err)
	}
	// MarshalIndent omits a trailing newline; append one so the file
	// follows the convention `cat manifest.json` produces a clean shell
	// prompt afterwards. Idempotency: the trailing newline is byte-stable.
	raw = append(raw, '\n')
	return atomicWriteFile(path, raw, 0o644)
}

// ReadManifest loads the Manifest from the manifest.json inside outDir.
//
// Additive exported reader (issue #49 / R1): the cmd/narrate-server HTTP edge
// resolves a persistent-sink directory's source (SourceURI / SourceKind /
// ContentHash) before calling Narrate, and re-reads the manifest after a patch
// to shape its response — both without re-implementing the read or reaching
// into the unexported readManifest. No behavior change to existing callers.
//
// Error semantics match readManifest: an absent file is wrapped so callers can
// errors.Is(err, os.ErrNotExist); a corrupt JSON payload is a hard error.
func ReadManifest(outDir string) (Manifest, error) {
	return readManifest(filepath.Join(outDir, manifestFilename))
}

// readManifest loads m from path. An absent file is reported as
// os.ErrNotExist (wrapped), so callers can use errors.Is(err, os.ErrNotExist)
// to distinguish "no prior manifest" from "manifest is broken".
//
// A corrupt JSON payload is a hard error returned to the caller — NOT a
// "stale" verdict. CheckStale follows this convention: corruption means
// "we cannot tell", not "definitely stale".
func readManifest(path string) (Manifest, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		// Wrap to preserve os.ErrNotExist semantics for callers using
		// errors.Is. fmt.Errorf with %w handles this correctly.
		return Manifest{}, fmt.Errorf("persistent: read manifest %s: %w", path, err)
	}
	var m Manifest
	if err := json.Unmarshal(raw, &m); err != nil {
		return Manifest{}, fmt.Errorf("persistent: parse manifest %s: %w", path, err)
	}
	return m, nil
}

// atomicWriteFile writes data to path via a sibling tmp file + rename,
// so a partial write (cancel, crash, disk full) leaves the existing file
// untouched rather than corrupting it.
//
// The tmp file lives in the same directory as the target so the rename
// is atomic on the same filesystem. Cross-filesystem renames silently
// fall back to copy-on-rename on some OSes, which would defeat the
// atomicity goal — same-dir tmp prevents that.
//
// On any failure during write/close/rename, the tmp file is removed
// best-effort and the original path stays unchanged.
func atomicWriteFile(path string, data []byte, perm os.FileMode) (retErr error) {
	dir := filepathDir(path)
	tmp, err := openTempFile(dir, ".persistent-*.tmp")
	if err != nil {
		return fmt.Errorf("persistent: create tmp in %s: %w", dir, err)
	}
	tmpPath := tmp.Name()
	defer func() {
		if retErr != nil {
			// Best-effort cleanup on failure; the failed write itself is
			// the load-bearing error, so we don't shadow it.
			_ = os.Remove(tmpPath)
		}
	}()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("persistent: write tmp %s: %w", tmpPath, err)
	}
	if err := tmp.Chmod(perm); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("persistent: chmod tmp %s: %w", tmpPath, err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("persistent: close tmp %s: %w", tmpPath, err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("persistent: rename %s -> %s: %w", tmpPath, path, err)
	}
	return nil
}

// filepathDir returns the directory portion of path. Pulled out as a
// tiny helper to keep the atomic-write code path readable and to allow
// a future swap to path/filepath.Dir if portability becomes an issue
// (it works fine today on darwin + linux).
func filepathDir(path string) string {
	for i := len(path) - 1; i >= 0; i-- {
		if path[i] == '/' || path[i] == '\\' {
			if i == 0 {
				return path[:1]
			}
			return path[:i]
		}
	}
	return "."
}
