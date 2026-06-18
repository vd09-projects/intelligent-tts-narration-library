//go:build manual

// Real-afplay smoke test. Build-tag gated so `go test ./...` stays green
// without audio hardware. Run with:
//
//	go test -tags=manual ./sink/ephemeral/...
//
// The test plays sink/ephemeral/testdata/5s.wav once through the real
// afplay binary on PATH. If the testdata file is absent the test is
// skipped — not failed — so the manual-tag run remains green on a fresh
// checkout. Generate the fixture once via:
//
//	./scripts/kokoro --voice af_bella --text "five second test clip" > sink/ephemeral/testdata/5s.wav
package ephemeral

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/vd09-projects/intelligent-tts-narration-library/plan"
	"github.com/vd09-projects/intelligent-tts-narration-library/render"
)

func TestSink_Consume_RealAfplay_Smoke(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skipf("afplay is macOS-only; runtime=%s", runtime.GOOS)
	}
	fixturePath, err := filepath.Abs(filepath.Join("testdata", "5s.wav"))
	if err != nil {
		t.Fatalf("abs path: %v", err)
	}
	if _, err := os.Stat(fixturePath); err != nil {
		t.Skip("missing testdata/5s.wav; generate via scripts/kokoro and commit")
	}

	dir := filepath.Dir(fixturePath)
	res := render.RenderResult{
		Audio: render.AudioStream{
			Dir:   dir,
			Files: []string{"5s.wav"},
		},
		Timeline: plan.Timeline{
			PlanID: "01HSMOKE0000000000000000000",
			Format: render.DefaultFormat(),
			Blocks: []plan.BlockTiming{
				{BlockID: "smoke-001", StartMs: 0, EndMs: 5000, AudioRef: "5s.wav"},
			},
		},
		Format: render.DefaultFormat(),
	}

	// Generous wall-clock cap so a slow audio device doesn't false-fail.
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	s := New(WithPerBlockTimeout(10 * time.Second))
	rec, err := s.Consume(ctx, plan.NarrationPlan{}, res)
	if err != nil {
		t.Fatalf("Consume: %v", err)
	}
	if rec.BlocksPlayed != 1 {
		t.Errorf("BlocksPlayed: got %d, want 1", rec.BlocksPlayed)
	}
	if rec.TotalDurationMs != 5000 {
		t.Errorf("TotalDurationMs: got %d, want 5000 (planned, not wall-clock)", rec.TotalDurationMs)
	}
}
