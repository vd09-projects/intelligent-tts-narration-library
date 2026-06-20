package persistent

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vd09-projects/intelligent-tts-narration-library/plan"
	"github.com/vd09-projects/intelligent-tts-narration-library/render"
	"github.com/vd09-projects/intelligent-tts-narration-library/sink"
)

// twoBlockPlan returns a NarrationPlan + RenderResult fixture pair used
// across the happy-path tests. Block IDs / hashes are stable so byte-
// identical idempotency tests don't depend on test ordering.
//
// Pre-writes the per-block WAVs into res.Audio.Dir so Consume's readWAV
// path actually finds them.
func twoBlockPlan(t *testing.T) (plan.NarrationPlan, render.RenderResult) {
	t.Helper()
	audioDir := t.TempDir()

	pcmA := []byte{0x10, 0x11, 0x12, 0x13, 0x14, 0x15, 0x16, 0x17}
	pcmB := []byte{0x20, 0x21, 0x22, 0x23, 0x24, 0x25, 0x26, 0x27}
	writeFixture := func(name string, pcm []byte) string {
		path := filepath.Join(audioDir, name)
		if err := os.WriteFile(path, synthWAV(t, 24000, 1, 16, pcm), 0o644); err != nil {
			t.Fatalf("write fixture %s: %v", name, err)
		}
		return name
	}
	refA := writeFixture("b001.wav", pcmA)
	refB := writeFixture("b002.wav", pcmB)

	p := plan.NarrationPlan{
		SchemaVersion: plan.SchemaVersion,
		PlanID:        "01HTEST0000000000000000000",
		CreatedAt:     "2026-06-20T00:00:00Z",
		Source: plan.SourceRef{
			Kind:        plan.SourceKindFile,
			URI:         "/tmp/sample.md",
			ContentHash: "sha256-fixture-aaa",
			Adapter:     "file",
		},
		Defaults: plan.PlanDefaults{Level: plan.L1, Locale: "en"},
		Blocks: []plan.Block{
			{ID: "b001", Order: 0, Class: plan.ClassHeading, Level: plan.L1, Status: plan.StatusVoiced,
				SourceMap: plan.SourceMap{Kind: plan.SourceKindLineRange, StartLine: 1, EndLine: 1}},
			{ID: "b002", Order: 1, Class: plan.ClassProse, Level: plan.L1, Status: plan.StatusVoiced,
				SourceMap: plan.SourceMap{Kind: plan.SourceKindLineRange, StartLine: 3, EndLine: 3}},
		},
	}

	res := render.RenderResult{
		Audio: render.AudioStream{
			Dir:   audioDir,
			Files: []string{refA, refB},
		},
		Timeline: plan.Timeline{
			PlanID: p.PlanID,
			Format: render.DefaultFormat(),
			Blocks: []plan.BlockTiming{
				{BlockID: "b001", StartMs: 0, EndMs: 500, AudioRef: refA},
				{BlockID: "b002", StartMs: 600, EndMs: 1100, AudioRef: refB},
			},
		},
		Format: render.DefaultFormat(),
	}
	return p, res
}

func TestSink_ImplementsOutputSink(t *testing.T) {
	t.Parallel()
	// Compile-time assertion (also in persistent.go); this guards against
	// a refactor that drops the interface assertion at the package level.
	var _ sink.OutputSink = (*Sink)(nil)
	var _ sink.OutputSink = New("/tmp/out")
}

func TestConsume_FreshWrite(t *testing.T) {
	p, res := twoBlockPlan(t)
	outDir := t.TempDir()
	s := New(outDir, WithVoice("af_bella"))

	rec, err := s.Consume(context.Background(), p, res)
	if err != nil {
		t.Fatalf("Consume: %v", err)
	}

	// All three output files exist.
	for _, name := range []string{audioFilename, planFilename, manifestFilename} {
		if _, err := os.Stat(filepath.Join(outDir, name)); err != nil {
			t.Errorf("expected %s to exist: %v", name, err)
		}
	}
	// Receipt math: two voiced blocks; planned duration sums to 1000.
	if rec.BlocksPlayed != 2 {
		t.Errorf("BlocksPlayed: got %d want 2", rec.BlocksPlayed)
	}
	if rec.TotalDurationMs != 1000 {
		t.Errorf("TotalDurationMs: got %d want 1000 (planned, not wall-clock)", rec.TotalDurationMs)
	}

	// plan.json round-trips back to the input plan (verbatim — sink does
	// not sanitize or re-derive).
	planRaw, err := os.ReadFile(filepath.Join(outDir, planFilename))
	if err != nil {
		t.Fatal(err)
	}
	var gotPlan plan.NarrationPlan
	if err := json.Unmarshal(planRaw, &gotPlan); err != nil {
		t.Fatalf("plan.json unmarshal: %v", err)
	}
	if gotPlan.PlanID != p.PlanID {
		t.Errorf("plan.json PlanID drift: got %q want %q", gotPlan.PlanID, p.PlanID)
	}
	if gotPlan.Source.ContentHash != p.Source.ContentHash {
		t.Errorf("plan.json ContentHash drift")
	}

	// Manifest content checks.
	m, err := readManifest(filepath.Join(outDir, manifestFilename))
	if err != nil {
		t.Fatalf("readManifest: %v", err)
	}
	if m.ContentHash != p.Source.ContentHash {
		t.Errorf("manifest ContentHash: got %q want %q", m.ContentHash, p.Source.ContentHash)
	}
	if m.Voice != "af_bella" {
		t.Errorf("manifest Voice: got %q want %q", m.Voice, "af_bella")
	}
	if len(m.Blocks) != 2 {
		t.Fatalf("manifest Blocks: got %d want 2", len(m.Blocks))
	}
	if m.Blocks[0].Class != plan.ClassHeading {
		t.Errorf("manifest Block[0].Class: got %q want %q", m.Blocks[0].Class, plan.ClassHeading)
	}
	if m.Blocks[1].StartMs != 600 || m.Blocks[1].EndMs != 1100 {
		t.Errorf("manifest Block[1] timing: got [%d,%d] want [600,1100]", m.Blocks[1].StartMs, m.Blocks[1].EndMs)
	}
	if m.Stale {
		t.Error("fresh-write manifest should not be marked Stale=true")
	}
}

func TestConsume_IdempotentRewrite(t *testing.T) {
	// Decision v1.4.0 + B3 (review v1): identical inputs → byte-identical
	// audio.wav, plan.json, manifest.json.
	p, res := twoBlockPlan(t)
	outDir := t.TempDir()
	s := New(outDir, WithVoice("af_bella"))

	if _, err := s.Consume(context.Background(), p, res); err != nil {
		t.Fatal(err)
	}
	firstAudio := mustRead(t, filepath.Join(outDir, audioFilename))
	firstPlan := mustRead(t, filepath.Join(outDir, planFilename))
	firstManifest := mustRead(t, filepath.Join(outDir, manifestFilename))

	if _, err := s.Consume(context.Background(), p, res); err != nil {
		t.Fatal(err)
	}
	secondAudio := mustRead(t, filepath.Join(outDir, audioFilename))
	secondPlan := mustRead(t, filepath.Join(outDir, planFilename))
	secondManifest := mustRead(t, filepath.Join(outDir, manifestFilename))

	if !bytes.Equal(firstAudio, secondAudio) {
		t.Errorf("audio.wav not byte-equal across rewrites (lens %d / %d)", len(firstAudio), len(secondAudio))
	}
	if !bytes.Equal(firstPlan, secondPlan) {
		t.Errorf("plan.json not byte-equal across rewrites")
	}
	if !bytes.Equal(firstManifest, secondManifest) {
		t.Errorf("manifest.json not byte-equal across rewrites")
	}
}

func TestConsume_RefusedBlock(t *testing.T) {
	// Refused blocks already carry audio of Refusal.Message and should be
	// treated identical to voiced blocks at the sink layer.
	p, res := twoBlockPlan(t)
	p.Blocks[1].Status = plan.StatusRefused
	p.Blocks[1].Refusal = &plan.Refusal{
		Reason:  plan.RefuseNoIntelligence,
		Message: "this prose block requires an intelligence adapter that wasn't supplied",
		Spoken:  true,
		SourceMap: plan.SourceMap{
			Kind: plan.SourceKindLineRange, StartLine: 3, EndLine: 3,
		},
	}
	outDir := t.TempDir()
	s := New(outDir, WithVoice("af_bella"))

	rec, err := s.Consume(context.Background(), p, res)
	if err != nil {
		t.Fatalf("Consume: %v", err)
	}
	// Refused-with-audio counts toward BlocksPlayed (same as voiced).
	if rec.BlocksPlayed != 2 {
		t.Errorf("BlocksPlayed: got %d want 2 (refused with audio counts)", rec.BlocksPlayed)
	}
	m, err := readManifest(filepath.Join(outDir, manifestFilename))
	if err != nil {
		t.Fatal(err)
	}
	if m.Blocks[1].Status != plan.StatusRefused {
		t.Errorf("manifest Block[1].Status: got %q want %q", m.Blocks[1].Status, plan.StatusRefused)
	}
}

func TestConsume_EmptyAudioRef(t *testing.T) {
	// Empty AudioRef = all-pause / no-speech block. No file IO; planned
	// duration in TotalDurationMs; BlocksPlayed does NOT increment.
	p, res := twoBlockPlan(t)
	// Replace block 0 timing with an empty-AudioRef row.
	res.Timeline.Blocks[0] = plan.BlockTiming{BlockID: "b001", StartMs: 0, EndMs: 200, AudioRef: ""}

	outDir := t.TempDir()
	s := New(outDir, WithVoice("af_bella"))
	rec, err := s.Consume(context.Background(), p, res)
	if err != nil {
		t.Fatalf("Consume: %v", err)
	}
	if rec.BlocksPlayed != 1 {
		t.Errorf("BlocksPlayed: got %d want 1 (empty-AudioRef does not count)", rec.BlocksPlayed)
	}
	// planned: 200 (empty) + 500 (b002 EndMs-StartMs=1100-600) = 700
	if rec.TotalDurationMs != 700 {
		t.Errorf("TotalDurationMs: got %d want 700", rec.TotalDurationMs)
	}
}

func TestConsume_MissingBlockAudio(t *testing.T) {
	// Non-empty AudioRef pointing to a missing file → wrapped error names
	// block id + path; partial receipt; no output files (partial-write
	// guard).
	p, res := twoBlockPlan(t)
	// Bury an unreachable ref into the second block; first block stays valid
	// so we can observe partial receipt state (BlocksPlayed=1).
	res.Timeline.Blocks[1].AudioRef = "missing.wav"

	outDir := t.TempDir()
	s := New(outDir, WithVoice("af_bella"))
	rec, err := s.Consume(context.Background(), p, res)
	if err == nil {
		t.Fatal("expected error for missing block audio, got nil")
	}
	if !errors.Is(err, ErrMissingBlockAudio) {
		t.Errorf("expected ErrMissingBlockAudio, got %v", err)
	}
	if !strings.Contains(err.Error(), "b002") {
		t.Errorf("error should mention block id b002, got %q", err.Error())
	}
	if !strings.Contains(err.Error(), "missing.wav") {
		t.Errorf("error should mention missing path, got %q", err.Error())
	}
	// Partial receipt: first block voiced before the failure.
	if rec.BlocksPlayed != 1 {
		t.Errorf("partial BlocksPlayed: got %d want 1", rec.BlocksPlayed)
	}
	// No output files left on disk.
	for _, name := range []string{audioFilename, planFilename, manifestFilename} {
		if _, err := os.Stat(filepath.Join(outDir, name)); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("expected %s NOT to exist after failure; stat err=%v", name, err)
		}
	}
}

func TestConsume_CtxCancelBeforeBlock(t *testing.T) {
	// Pre-cancelled ctx — Consume returns ctx.Err() immediately with empty
	// receipt; no output files.
	p, res := twoBlockPlan(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	outDir := t.TempDir()
	s := New(outDir, WithVoice("af_bella"))
	rec, err := s.Consume(ctx, p, res)
	if !errors.Is(err, context.Canceled) {
		t.Errorf("expected context.Canceled, got %v", err)
	}
	if rec.BlocksPlayed != 0 {
		t.Errorf("BlocksPlayed on pre-cancel: got %d want 0", rec.BlocksPlayed)
	}
	for _, name := range []string{audioFilename, planFilename, manifestFilename} {
		if _, err := os.Stat(filepath.Join(outDir, name)); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("expected %s NOT to exist after ctx cancel; stat err=%v", name, err)
		}
	}
}

func TestConsume_TotalDurationMatchesTimeline(t *testing.T) {
	cases := []struct {
		name           string
		blocks         []plan.BlockTiming
		wantDurationMs int64
		// blocksPlayed expected (counts only non-empty AudioRef rows
		// that get a successfully-read fixture WAV)
		wantBlocksPlayed int
	}{
		{
			name: "no-gap",
			blocks: []plan.BlockTiming{
				{BlockID: "b001", StartMs: 0, EndMs: 500, AudioRef: "b001.wav"},
				{BlockID: "b002", StartMs: 500, EndMs: 1000, AudioRef: "b002.wav"},
			},
			wantDurationMs:   1000,
			wantBlocksPlayed: 2,
		},
		{
			name: "100ms-gap",
			blocks: []plan.BlockTiming{
				{BlockID: "b001", StartMs: 0, EndMs: 500, AudioRef: "b001.wav"},
				{BlockID: "b002", StartMs: 600, EndMs: 1100, AudioRef: "b002.wav"},
			},
			wantDurationMs:   1000,
			wantBlocksPlayed: 2,
		},
		{
			name: "leading-50ms",
			blocks: []plan.BlockTiming{
				{BlockID: "b001", StartMs: 50, EndMs: 550, AudioRef: "b001.wav"},
				{BlockID: "b002", StartMs: 600, EndMs: 1100, AudioRef: "b002.wav"},
			},
			wantDurationMs:   1000,
			wantBlocksPlayed: 2,
		},
		{
			name: "all-pause-middle",
			blocks: []plan.BlockTiming{
				{BlockID: "b001", StartMs: 0, EndMs: 500, AudioRef: "b001.wav"},
				{BlockID: "b002", StartMs: 500, EndMs: 700, AudioRef: ""},
				{BlockID: "b003", StartMs: 700, EndMs: 1200, AudioRef: "b002.wav"}, // reuse b002.wav fixture
			},
			wantDurationMs:   1200,
			wantBlocksPlayed: 2, // b002 is empty-AudioRef
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p, res := twoBlockPlan(t)
			// Patch in the timing rows; ensure plan.Blocks contains the new
			// IDs so buildManifest can find class/level/status.
			res.Timeline.Blocks = tc.blocks
			// Append an extra plan.Block for the all-pause-middle case so
			// the manifest join doesn't drop b003. The plan rows have
			// status=voiced but the actual AudioRef logic is what drives
			// BlocksPlayed.
			if tc.name == "all-pause-middle" {
				p.Blocks = append(p.Blocks, plan.Block{
					ID: "b003", Order: 2, Class: plan.ClassProse, Level: plan.L1, Status: plan.StatusVoiced,
				})
			}
			outDir := t.TempDir()
			s := New(outDir, WithVoice("test-voice"))
			rec, err := s.Consume(context.Background(), p, res)
			if err != nil {
				t.Fatalf("Consume: %v", err)
			}
			if rec.TotalDurationMs != tc.wantDurationMs {
				t.Errorf("TotalDurationMs: got %d want %d", rec.TotalDurationMs, tc.wantDurationMs)
			}
			if rec.BlocksPlayed != tc.wantBlocksPlayed {
				t.Errorf("BlocksPlayed: got %d want %d", rec.BlocksPlayed, tc.wantBlocksPlayed)
			}
		})
	}
}

func TestConsume_FormatMismatch(t *testing.T) {
	// A per-block WAV at wrong sample rate → wrapped error with block id
	// and the divergent field. Output files NOT written.
	p, res := twoBlockPlan(t)
	// Overwrite b002 with a bogus-rate WAV.
	bogusPath := filepath.Join(res.Audio.Dir, "b002.wav")
	if err := os.WriteFile(bogusPath, synthWAV(t, 16000, 1, 16, []byte{1, 2, 3, 4}), 0o644); err != nil {
		t.Fatal(err)
	}
	outDir := t.TempDir()
	s := New(outDir, WithVoice("af_bella"))
	rec, err := s.Consume(context.Background(), p, res)
	if err == nil {
		t.Fatal("expected format-mismatch error, got nil")
	}
	if !strings.Contains(err.Error(), "b002") {
		t.Errorf("error should mention block id b002, got %q", err.Error())
	}
	if !strings.Contains(err.Error(), "sample_rate") {
		t.Errorf("error should mention sample_rate, got %q", err.Error())
	}
	if rec.BlocksPlayed != 1 {
		t.Errorf("partial receipt BlocksPlayed: got %d want 1", rec.BlocksPlayed)
	}
	for _, name := range []string{audioFilename, planFilename, manifestFilename} {
		if _, statErr := os.Stat(filepath.Join(outDir, name)); !errors.Is(statErr, os.ErrNotExist) {
			t.Errorf("expected %s NOT to exist after failure; stat err=%v", name, statErr)
		}
	}
}

func TestConsume_VoiceFromOption(t *testing.T) {
	// WithVoice populates Manifest.Voice; absent → empty string.
	p, res := twoBlockPlan(t)

	t.Run("with-voice", func(t *testing.T) {
		outDir := t.TempDir()
		s := New(outDir, WithVoice("af_bella"))
		if _, err := s.Consume(context.Background(), p, res); err != nil {
			t.Fatal(err)
		}
		m, err := readManifest(filepath.Join(outDir, manifestFilename))
		if err != nil {
			t.Fatal(err)
		}
		if m.Voice != "af_bella" {
			t.Errorf("Voice: got %q want %q", m.Voice, "af_bella")
		}
	})

	t.Run("no-voice-option", func(t *testing.T) {
		outDir := t.TempDir()
		s := New(outDir) // no WithVoice
		if _, err := s.Consume(context.Background(), p, res); err != nil {
			t.Fatal(err)
		}
		m, err := readManifest(filepath.Join(outDir, manifestFilename))
		if err != nil {
			t.Fatal(err)
		}
		if m.Voice != "" {
			t.Errorf("Voice without option: got %q want \"\" (sink does not invent)", m.Voice)
		}
	})
}

func TestConsume_NoOutDir(t *testing.T) {
	// Zero-value Sink — empty OutDir — returns the documented sentinel.
	p, res := twoBlockPlan(t)
	s := &Sink{}
	_, err := s.Consume(context.Background(), p, res)
	if !errors.Is(err, ErrNoOutDir) {
		t.Errorf("expected ErrNoOutDir, got %v", err)
	}
}

func TestConsume_LeadingSilenceBeforeFirstBlock(t *testing.T) {
	// Decision v1.5.0: leading silence before block 0 is emitted if
	// StartMs > 0. Verify the audio.wav data size accounts for it.
	p, res := twoBlockPlan(t)
	// Shift both blocks so the timeline starts at 200ms.
	res.Timeline.Blocks[0].StartMs = 200
	res.Timeline.Blocks[0].EndMs = 700
	res.Timeline.Blocks[1].StartMs = 800
	res.Timeline.Blocks[1].EndMs = 1300

	outDir := t.TempDir()
	s := New(outDir, WithVoice("af_bella"))
	if _, err := s.Consume(context.Background(), p, res); err != nil {
		t.Fatal(err)
	}

	// Read back audio.wav and confirm the data chunk includes the 200ms
	// of leading silence (200ms * 24000 * 2 bytes / 1000 = 9600 bytes)
	// plus the inter-block 100ms gap (4800 bytes) plus 2 × 8 bytes of PCM.
	raw, err := os.ReadFile(filepath.Join(outDir, audioFilename))
	if err != nil {
		t.Fatal(err)
	}
	_, pcm, err := parseWAV("audio.wav", raw, render.DefaultFormat())
	if err != nil {
		t.Fatalf("parseWAV: %v", err)
	}
	wantSize := 9600 + 8 + 4800 + 8
	if len(pcm) != wantSize {
		t.Errorf("combined PCM size: got %d want %d (leading silence + segments + gap)", len(pcm), wantSize)
	}
}

// mustRead reads the file at path or fails the test. Pulled out so the
// idempotency test stays readable.
func mustRead(t *testing.T, path string) []byte {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return raw
}
