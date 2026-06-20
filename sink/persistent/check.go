package persistent

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/vd09-projects/intelligent-tts-narration-library/plan"
)

// CheckStale compares the manifest.json under outDir against the supplied
// plan and reports whether the persisted output is still aligned with the
// plan source.
//
// Semantics (Decision v1.6.0 — CheckStale is NOT part of the OutputSink
// interface; staleness is a read-side query):
//
//   - No manifest.json present → (false, "", nil). Absence is not staleness;
//     callers distinguish "never written" from "written but stale" via
//     their own existence check if needed.
//   - manifest.ContentHash == "" → (true, "manifest missing content_hash …",
//     nil). Treated as stale per planner-task.md S3 — a missing hash means
//     either an incomplete write or an older schema, both of which are
//     unsafe to trust.
//   - manifest.ContentHash != plan.Source.ContentHash → (true,
//     "content_hash mismatch: manifest=<a> plan=<b>", nil).
//   - manifest.SourceURI != plan.Source.URI OR manifest.SourceKind !=
//     plan.Source.Kind → (true, "source path mismatch: manifest=<a> plan=<b>",
//     nil). Source moved (e.g. file renamed); stale.
//   - All match → (false, "", nil).
//   - manifest.json present but corrupt JSON → (false, "", err). The error
//     is the load-bearing signal; we deliberately do NOT report stale=true
//     because we cannot prove staleness when we can't read the file.
//
// CheckStale does NOT regenerate. The honesty rule (CLAUDE.md "stale
// detection is data; do not auto-regenerate") is load-bearing — callers
// decide what to do with the verdict.
func CheckStale(outDir string, p plan.NarrationPlan) (bool, string, error) {
	manifestPath := filepath.Join(outDir, manifestFilename)
	m, err := readManifest(manifestPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			// "No prior manifest" is a defined state, not staleness. The
			// caller is in their first run for this outDir.
			return false, "", nil
		}
		// Corrupt JSON or other I/O error — we cannot tell whether the
		// output is stale, so we propagate the error and leave stale=false
		// so the caller does not act on a verdict we can't prove.
		return false, "", err
	}

	// Empty content_hash in the manifest is treated as stale per S3
	// (planner-task.md). It could mean (a) the writer crashed before
	// populating the field, or (b) the manifest was written by a schema
	// version that didn't carry the hash. Either way, trusting the bytes
	// on disk without a hash is unsafe.
	if m.ContentHash == "" {
		return true, "manifest missing content_hash (incomplete or older schema)", nil
	}

	if m.ContentHash != p.Source.ContentHash {
		return true, fmt.Sprintf("content_hash mismatch: manifest=%s plan=%s",
			m.ContentHash, p.Source.ContentHash), nil
	}

	if m.SourceURI != p.Source.URI || m.SourceKind != p.Source.Kind {
		// Combined message because source-path drift comes in two flavors
		// (URI or Kind) and the caller's response is the same either way.
		return true, fmt.Sprintf("source path mismatch: manifest=%s/%s plan=%s/%s",
			m.SourceKind, m.SourceURI, p.Source.Kind, p.Source.URI), nil
	}

	return false, "", nil
}
