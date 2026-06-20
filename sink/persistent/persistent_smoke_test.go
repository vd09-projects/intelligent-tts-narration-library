//go:build manual

// Real-Kokoro smoke test for the persistent sink. Build-tag gated so
// `go test ./...` stays green without a Kokoro install. Run with:
//
//	go test -tags=manual ./sink/persistent/...
//
// The test wires file adapter + planner (no intelligence) + render/sherpa
// + sink/persistent against docs/samples/sample.md, then asserts the
// expected output triple (audio.wav + plan.json + manifest.json) exists
// and parses. Audio quality is verified by ear during the manual run —
// no automated audio comparison (no goldens, no PSNR).
//
// Mirrors sink/ephemeral/ephemeral_smoke_test.go in shape.
package persistent

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/vd09-projects/intelligent-tts-narration-library/adapter/file"
	"github.com/vd09-projects/intelligent-tts-narration-library/pipeline"
	"github.com/vd09-projects/intelligent-tts-narration-library/plan"
	"github.com/vd09-projects/intelligent-tts-narration-library/render"
	"github.com/vd09-projects/intelligent-tts-narration-library/render/sherpa"
)

func TestPersistent_Consume_RealKokoro_Smoke(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skipf("phase one is macOS-only; runtime=%s", runtime.GOOS)
	}
	repoRoot, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("abs repo root: %v", err)
	}
	sample := filepath.Join(repoRoot, "docs", "samples", "sample.md")
	if _, err := os.Stat(sample); err != nil {
		t.Skipf("missing %s; nothing to narrate", sample)
	}

	rendererTmp := t.TempDir()
	persistOut := t.TempDir()

	pl := pipeline.New(
		file.New(),
		nil, // no intelligence — deterministic + degraded path
		sherpa.New(sherpa.EngineConfig{}),
		New(persistOut, WithVoice("af_bella")),
		pipeline.PipelineDefaults{
			Level:  plan.L1,
			OutDir: rendererTmp,
			Locale: "en",
		},
	)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	res, err := pl.Narrate(ctx, plan.SourceRef{Kind: plan.SourceKindFile, URI: sample}, pipeline.NarrateRequest{
		Voice: "af_bella",
	})
	if err != nil {
		t.Fatalf("Narrate: %v", err)
	}

	// All three output files exist + non-empty.
	for _, name := range []string{audioFilename, planFilename, manifestFilename} {
		path := filepath.Join(persistOut, name)
		info, err := os.Stat(path)
		if err != nil {
			t.Errorf("expected %s; stat err=%v", path, err)
			continue
		}
		if info.Size() == 0 {
			t.Errorf("%s is empty", path)
		}
	}

	// audio.wav round-trips via readWAV (validates the RIFF header the
	// sink wrote against render.DefaultFormat()).
	if _, _, err := readWAV(filepath.Join(persistOut, audioFilename), render.DefaultFormat()); err != nil {
		t.Errorf("audio.wav does not parse via readWAV: %v", err)
	}

	// plan.json parses to a NarrationPlan.
	planRaw, err := os.ReadFile(filepath.Join(persistOut, planFilename))
	if err != nil {
		t.Fatal(err)
	}
	var p plan.NarrationPlan
	if err := json.Unmarshal(planRaw, &p); err != nil {
		t.Errorf("plan.json does not parse: %v", err)
	}

	// manifest.json parses + carries the expected fields.
	m, err := readManifest(filepath.Join(persistOut, manifestFilename))
	if err != nil {
		t.Fatalf("manifest.json: %v", err)
	}
	if m.ContentHash == "" {
		t.Error("manifest ContentHash should be non-empty after a real render")
	}
	if m.Voice != "af_bella" {
		t.Errorf("manifest Voice: got %q want %q", m.Voice, "af_bella")
	}
	if len(m.Blocks) == 0 {
		t.Error("manifest Blocks empty (expected at least one block from sample.md)")
	}
	if len(m.Blocks) != len(p.Blocks) {
		t.Errorf("manifest Blocks count (%d) != plan Blocks count (%d)",
			len(m.Blocks), len(p.Blocks))
	}

	// Receipt sanity.
	if res.TotalDurationMs <= 0 {
		t.Errorf("TotalDurationMs: got %d, want > 0", res.TotalDurationMs)
	}
}
