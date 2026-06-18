package sherpa

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/vd09-projects/intelligent-tts-narration-library/plan"
	"github.com/vd09-projects/intelligent-tts-narration-library/render"
)

// fakeBinary returns the absolute path to testdata/fake-kokoro.sh.
// Tests skip on Windows since the fake is a bash script (planner-task.md risk).
func fakeBinary(t *testing.T) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("fake-kokoro.sh is bash; skipping on Windows")
	}
	abs, err := filepath.Abs("testdata/fake-kokoro.sh")
	if err != nil {
		t.Fatalf("abs path: %v", err)
	}
	if _, err := os.Stat(abs); err != nil {
		t.Fatalf("fake binary missing at %s: %v", abs, err)
	}
	return abs
}

// loadGolden reads testdata/plan.json into a plan.NarrationPlan.
func loadGolden(t *testing.T) plan.NarrationPlan {
	t.Helper()
	raw, err := os.ReadFile("testdata/plan.json")
	if err != nil {
		t.Fatalf("read golden plan: %v", err)
	}
	var p plan.NarrationPlan
	if err := json.Unmarshal(raw, &p); err != nil {
		t.Fatalf("unmarshal golden plan: %v", err)
	}
	return p
}

// trivialPlan — single-block voiced prose plan, used for non-golden cases.
func trivialPlan() plan.NarrationPlan {
	return plan.NarrationPlan{
		SchemaVersion: "1.0.0",
		PlanID:        "01HTRIVIAL000000000000000000",
		CreatedAt:     "2026-06-18T12:00:00Z",
		Source: plan.SourceRef{
			Kind: plan.SourceKindFile, URI: "test", ContentHash: "sha256:fake", Adapter: "file",
		},
		Defaults: plan.PlanDefaults{Level: plan.L2, Locale: "en"},
		Blocks: []plan.Block{
			{
				ID: "blk-001", Order: 0, Class: plan.ClassProse, Level: plan.L2, Status: plan.StatusVoiced,
				SourceMap: plan.SourceMap{Kind: plan.SourceKindLineRange, StartLine: 1, EndLine: 1},
				Segments:  []plan.Segment{{ID: "seg-1", Kind: plan.SegmentKindSpeech, Text: "Hi."}},
				Provenance: plan.Provenance{VoicedBy: "verbatim", Deterministic: true, LevelAsked: plan.L2},
			},
		},
	}
}

func TestEngine_Render_VoicedProseOnly(t *testing.T) {
	bin := fakeBinary(t)
	engine := New(EngineConfig{BinaryPath: bin})
	out := t.TempDir()
	logPath := filepath.Join(out, "args.log")
	t.Setenv("FAKE_KOKORO_ARGS_LOG", logPath)

	res, err := engine.Render(context.Background(), trivialPlan(), render.RenderOptions{OutDir: out})
	if err != nil {
		t.Fatalf("Render: %v", err)
	}

	if len(res.Timeline.Blocks) != 1 {
		t.Fatalf("want 1 BlockTiming, got %d", len(res.Timeline.Blocks))
	}
	bt := res.Timeline.Blocks[0]
	if bt.BlockID != "blk-001" {
		t.Errorf("BlockID = %q, want blk-001", bt.BlockID)
	}
	if bt.EndMs <= bt.StartMs {
		t.Errorf("EndMs (%d) must exceed StartMs (%d)", bt.EndMs, bt.StartMs)
	}
	if bt.EndMs != 200 {
		t.Errorf("EndMs = %d, want 200 (fake emits 200 ms)", bt.EndMs)
	}
	if got := res.Format; got.SampleRate != 24000 || got.Channels != 1 || got.Encoding != "pcm_s16le" {
		t.Errorf("Format = %+v, want 24000/1/pcm_s16le", got)
	}
	wavPath := filepath.Join(out, "blk-001.wav")
	if info, err := os.Stat(wavPath); err != nil {
		t.Errorf("wav not written: %v", err)
	} else if info.Size() <= wavHeaderBytes {
		t.Errorf("wav too small: %d bytes", info.Size())
	}

	// Default voice = af_bella.
	logBytes, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read args log: %v", err)
	}
	if !strings.Contains(string(logBytes), "af_bella|") {
		t.Errorf("default voice not af_bella; log: %s", logBytes)
	}
}

func TestEngine_Render_VoiceOverride(t *testing.T) {
	bin := fakeBinary(t)
	out := t.TempDir()
	logPath := filepath.Join(out, "args.log")
	t.Setenv("FAKE_KOKORO_ARGS_LOG", logPath)

	engine := New(EngineConfig{BinaryPath: bin})
	_, err := engine.Render(context.Background(), trivialPlan(),
		render.RenderOptions{OutDir: out, Voice: "am_michael"})
	if err != nil {
		t.Fatalf("Render: %v", err)
	}

	logBytes, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read args log: %v", err)
	}
	if !strings.Contains(string(logBytes), "am_michael|") {
		t.Errorf("voice not am_michael; log: %s", logBytes)
	}
}

func TestEngine_Render_RefusedBareImage(t *testing.T) {
	bin := fakeBinary(t)
	engine := New(EngineConfig{BinaryPath: bin})
	out := t.TempDir()

	p := trivialPlan()
	p.Blocks = []plan.Block{
		{
			ID: "blk-img", Order: 0, Class: plan.ClassUnknown, Level: plan.L2, Status: plan.StatusRefused,
			SourceMap: plan.SourceMap{Kind: plan.SourceKindLineRange, StartLine: 7, EndLine: 7},
			Refusal: &plan.Refusal{
				Reason: plan.RefuseBareImage, Message: "image, not described", Spoken: true,
				SourceMap: plan.SourceMap{Kind: plan.SourceKindLineRange, StartLine: 7, EndLine: 7},
			},
			Provenance: plan.Provenance{VoicedBy: "planner", Deterministic: true, LevelAsked: plan.L2},
		},
	}
	res, err := engine.Render(context.Background(), p, render.RenderOptions{OutDir: out})
	if err != nil {
		t.Fatalf("refused block must render, got error: %v", err)
	}
	if len(res.Timeline.Blocks) != 1 {
		t.Fatalf("want 1 BlockTiming for refused block, got %d", len(res.Timeline.Blocks))
	}
	if res.Timeline.Blocks[0].AudioRef != "blk-img.wav" {
		t.Errorf("AudioRef = %q, want blk-img.wav", res.Timeline.Blocks[0].AudioRef)
	}
	if _, err := os.Stat(filepath.Join(out, "blk-img.wav")); err != nil {
		t.Errorf("refused block wav not written: %v", err)
	}
}

func TestEngine_Render_RefusedEmptyMessage(t *testing.T) {
	bin := fakeBinary(t)
	engine := New(EngineConfig{BinaryPath: bin})
	out := t.TempDir()

	p := trivialPlan()
	p.Blocks[0].Status = plan.StatusRefused
	p.Blocks[0].Refusal = &plan.Refusal{
		Reason: plan.RefuseBareImage, Message: "   ", Spoken: true,
	}
	_, err := engine.Render(context.Background(), p, render.RenderOptions{OutDir: out})
	if !errors.Is(err, ErrMalformedPlan) {
		t.Fatalf("want ErrMalformedPlan, got %v", err)
	}
}

func TestEngine_Render_ThreeBlockGolden(t *testing.T) {
	bin := fakeBinary(t)
	engine := New(EngineConfig{BinaryPath: bin})
	out := t.TempDir()

	res, err := engine.Render(context.Background(), loadGolden(t),
		render.RenderOptions{OutDir: out})
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if len(res.Timeline.Blocks) != 3 {
		t.Fatalf("want 3 BlockTimings, got %d", len(res.Timeline.Blocks))
	}
	wantOrder := []string{"blk-001", "blk-002", "blk-003"}
	for i, bt := range res.Timeline.Blocks {
		if bt.BlockID != wantOrder[i] {
			t.Errorf("Timeline[%d].BlockID = %q, want %q", i, bt.BlockID, wantOrder[i])
		}
		if bt.EndMs < bt.StartMs {
			t.Errorf("Timeline[%d]: EndMs (%d) < StartMs (%d)", i, bt.EndMs, bt.StartMs)
		}
		if i > 0 && bt.StartMs < res.Timeline.Blocks[i-1].EndMs {
			t.Errorf("Timeline[%d].StartMs (%d) < prev EndMs (%d) — not monotonic",
				i, bt.StartMs, res.Timeline.Blocks[i-1].EndMs)
		}
		if _, err := os.Stat(filepath.Join(out, bt.BlockID+".wav")); err != nil {
			t.Errorf("wav missing for %s: %v", bt.BlockID, err)
		}
	}
	if res.Audio.ManifestPath == "" {
		t.Error("ManifestPath empty")
	} else {
		mb, err := os.ReadFile(res.Audio.ManifestPath)
		if err != nil {
			t.Errorf("read manifest: %v", err)
		} else if !strings.Contains(string(mb), "blk-001") || !strings.Contains(string(mb), "blk-003") {
			t.Errorf("manifest missing expected ids: %s", mb)
		}
	}
}

func TestEngine_Render_MissingBinary(t *testing.T) {
	engine := New(EngineConfig{BinaryPath: "/nonexistent/nope"})
	_, err := engine.Render(context.Background(), trivialPlan(),
		render.RenderOptions{OutDir: t.TempDir()})
	if !errors.Is(err, ErrMissingBinary) {
		t.Fatalf("want ErrMissingBinary, got %v", err)
	}
}

func TestEngine_Render_SubprocessNonzero(t *testing.T) {
	bin := fakeBinary(t)
	t.Setenv("FAKE_KOKORO_FAIL", "1")
	engine := New(EngineConfig{BinaryPath: bin})
	_, err := engine.Render(context.Background(), trivialPlan(),
		render.RenderOptions{OutDir: t.TempDir()})
	if !errors.Is(err, ErrSubprocess) {
		t.Fatalf("want ErrSubprocess, got %v", err)
	}
	if !strings.Contains(err.Error(), "forced fail") {
		t.Errorf("err should carry stderr; got: %v", err)
	}
}

func TestEngine_Render_UnsupportedVoice(t *testing.T) {
	bin := fakeBinary(t)
	engine := New(EngineConfig{BinaryPath: bin})
	_, err := engine.Render(context.Background(), trivialPlan(),
		render.RenderOptions{OutDir: t.TempDir(), Voice: "zz_xx"})
	if !errors.Is(err, ErrUnsupportedVoice) {
		t.Fatalf("want ErrUnsupportedVoice, got %v", err)
	}
}

func TestEngine_Render_Timeout(t *testing.T) {
	bin := fakeBinary(t)
	t.Setenv("FAKE_KOKORO_SLEEP_MS", "500")
	engine := New(EngineConfig{BinaryPath: bin})
	_, err := engine.Render(context.Background(), trivialPlan(),
		render.RenderOptions{OutDir: t.TempDir(), PerBlockTimeout: 50 * time.Millisecond})
	if !errors.Is(err, ErrTimeout) {
		t.Fatalf("want ErrTimeout, got %v", err)
	}
}

func TestEngine_RenderBlock_Single(t *testing.T) {
	bin := fakeBinary(t)
	engine := New(EngineConfig{BinaryPath: bin})
	out := t.TempDir()

	br, err := engine.RenderBlock(context.Background(), loadGolden(t), "blk-002",
		render.RenderOptions{OutDir: out})
	if err != nil {
		t.Fatalf("RenderBlock: %v", err)
	}
	if br.BlockID != "blk-002" {
		t.Errorf("BlockID = %q, want blk-002", br.BlockID)
	}
	if len(br.Audio.Files) != 1 || br.Audio.Files[0] != "blk-002.wav" {
		t.Errorf("Audio.Files = %v, want [blk-002.wav]", br.Audio.Files)
	}
	if br.Timing.StartMs != 0 {
		t.Errorf("RenderBlock Timing.StartMs = %d, want 0 (caller patches)", br.Timing.StartMs)
	}
	if br.Timing.EndMs != 200 {
		t.Errorf("Timing.EndMs = %d, want 200", br.Timing.EndMs)
	}
	// Other blocks NOT rendered.
	if _, err := os.Stat(filepath.Join(out, "blk-001.wav")); !os.IsNotExist(err) {
		t.Errorf("RenderBlock should not render blk-001, but file exists: %v", err)
	}
}

func TestEngine_RenderBlock_NotFound(t *testing.T) {
	bin := fakeBinary(t)
	engine := New(EngineConfig{BinaryPath: bin})
	_, err := engine.RenderBlock(context.Background(), loadGolden(t), "nope",
		render.RenderOptions{OutDir: t.TempDir()})
	if !errors.Is(err, ErrMalformedPlan) {
		t.Fatalf("want ErrMalformedPlan, got %v", err)
	}
}

func TestEngine_Render_PlanDefaultsVoiceHint(t *testing.T) {
	bin := fakeBinary(t)
	out := t.TempDir()
	logPath := filepath.Join(out, "args.log")
	t.Setenv("FAKE_KOKORO_ARGS_LOG", logPath)

	engine := New(EngineConfig{BinaryPath: bin})
	p := trivialPlan()
	p.Defaults.Voice = "am_michael" // engine voice id used as hint.
	if _, err := engine.Render(context.Background(), p, render.RenderOptions{OutDir: out}); err != nil {
		t.Fatalf("Render: %v", err)
	}
	logBytes, _ := os.ReadFile(logPath)
	if !strings.Contains(string(logBytes), "am_michael|") {
		t.Errorf("PlanDefaults voice not honored; log: %s", logBytes)
	}
}

func TestEngine_Render_UnknownPlanHintFallsBack(t *testing.T) {
	bin := fakeBinary(t)
	out := t.TempDir()
	logPath := filepath.Join(out, "args.log")
	t.Setenv("FAKE_KOKORO_ARGS_LOG", logPath)

	engine := New(EngineConfig{BinaryPath: bin})
	p := trivialPlan()
	p.Defaults.Voice = "completely_unknown_voice"
	if _, err := engine.Render(context.Background(), p, render.RenderOptions{OutDir: out}); err != nil {
		t.Fatalf("Render: %v", err)
	}
	logBytes, _ := os.ReadFile(logPath)
	if !strings.Contains(string(logBytes), "af_bella|") {
		t.Errorf("unknown hint should fall back to af_bella; log: %s", logBytes)
	}
}

func TestEngine_Render_OutDirRequired(t *testing.T) {
	engine := New(EngineConfig{BinaryPath: fakeBinary(t)})
	_, err := engine.Render(context.Background(), trivialPlan(), render.RenderOptions{})
	if err == nil || !strings.Contains(err.Error(), "OutDir") {
		t.Fatalf("want OutDir-required error, got %v", err)
	}
}

func TestEngine_Render_EmptyTextBlock_NoAudioRef(t *testing.T) {
	// Regression: review-findings B2. An all-pause / no-speech-segment block
	// must emit a BlockTiming with empty AudioRef, no WAV file, and not
	// appear in Audio.Files.
	bin := fakeBinary(t)
	engine := New(EngineConfig{BinaryPath: bin})
	out := t.TempDir()

	p := trivialPlan()
	p.Blocks[0].Segments = []plan.Segment{
		{ID: "seg-1", Kind: plan.SegmentKindPause, PauseMs: 250},
	}
	res, err := engine.Render(context.Background(), p, render.RenderOptions{OutDir: out})
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if len(res.Timeline.Blocks) != 1 {
		t.Fatalf("want 1 BlockTiming, got %d", len(res.Timeline.Blocks))
	}
	if got := res.Timeline.Blocks[0].AudioRef; got != "" {
		t.Errorf("empty-text block AudioRef = %q, want empty", got)
	}
	if len(res.Audio.Files) != 0 {
		t.Errorf("empty-text block must not appear in Audio.Files, got %v", res.Audio.Files)
	}
	if _, err := os.Stat(filepath.Join(out, "blk-001.wav")); !os.IsNotExist(err) {
		t.Errorf("empty-text block must not write wav: stat err = %v", err)
	}
}

func TestEngine_RenderBlock_EmptyText_NoAudioRef(t *testing.T) {
	// Regression: review-findings B3 — RenderBlock mirror of B2.
	bin := fakeBinary(t)
	engine := New(EngineConfig{BinaryPath: bin})
	out := t.TempDir()

	p := trivialPlan()
	p.Blocks[0].Segments = []plan.Segment{
		{ID: "seg-1", Kind: plan.SegmentKindPause, PauseMs: 100},
	}
	br, err := engine.RenderBlock(context.Background(), p, "blk-001",
		render.RenderOptions{OutDir: out})
	if err != nil {
		t.Fatalf("RenderBlock: %v", err)
	}
	if br.Timing.AudioRef != "" {
		t.Errorf("RenderBlock AudioRef = %q for empty-text block, want empty", br.Timing.AudioRef)
	}
	if len(br.Audio.Files) != 0 {
		t.Errorf("RenderBlock Audio.Files = %v, want empty", br.Audio.Files)
	}
}

func TestDefaultFormat_PhaseOne(t *testing.T) {
	f := render.DefaultFormat()
	if f.SampleRate != 24000 || f.Channels != 1 || f.Encoding != "pcm_s16le" {
		t.Errorf("DefaultFormat = %+v, want 24000/1/pcm_s16le", f)
	}
}
