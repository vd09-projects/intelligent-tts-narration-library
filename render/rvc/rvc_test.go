package rvc

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/vd09-projects/intelligent-tts-narration-library/plan"
	"github.com/vd09-projects/intelligent-tts-narration-library/render"
)

// ---- fake inner renderer (B3): hermetic, no sherpa.Engine, no models ----

// fakeInner is a render.Renderer stub that (a) records the RenderOptions.Voice it
// is handed (so tests assert the mapped Kokoro SOURCE, invisible to the RVC
// worker) and (b) writes synthetic 24 kHz mono s16le silence WAVs into opts.OutDir
// to drive the repaint loop. It mirrors sherpa's empty-block handling: a pause-
// only / no-speech block yields an empty AudioRef and no file.
type fakeInner struct {
	voiceLog []string
}

func innerSpoken(blk plan.Block) string {
	if blk.Status == plan.StatusRefused {
		if blk.Refusal != nil {
			return strings.TrimSpace(blk.Refusal.Message)
		}
		return ""
	}
	var parts []string
	for _, s := range blk.Segments {
		if s.Kind == plan.SegmentKindSpeech && s.Text != "" {
			parts = append(parts, s.Text)
		}
	}
	return strings.Join(parts, " ")
}

func (f *fakeInner) writeBlock(outDir string, blk plan.Block) (relName string, durMs int, err error) {
	if innerSpoken(blk) == "" {
		return "", 0, nil
	}
	rel := blk.ID + ".wav"
	if err := os.WriteFile(filepath.Join(outDir, rel), silentWAV(24000, 100), 0o644); err != nil {
		return "", 0, err
	}
	return rel, 100, nil
}

func (f *fakeInner) Render(_ context.Context, p plan.NarrationPlan, opts render.RenderOptions) (render.RenderResult, error) {
	f.voiceLog = append(f.voiceLog, opts.Voice)
	timing := make([]plan.BlockTiming, 0, len(p.Blocks))
	files := []string{}
	cursor := 0
	for _, blk := range p.Blocks {
		rel, dur, err := f.writeBlock(opts.OutDir, blk)
		if err != nil {
			return render.RenderResult{}, err
		}
		if rel != "" {
			files = append(files, rel)
		}
		timing = append(timing, plan.BlockTiming{BlockID: blk.ID, StartMs: cursor, EndMs: cursor + dur, AudioRef: rel})
		cursor += dur
	}
	return render.RenderResult{
		Audio:    render.AudioStream{Dir: opts.OutDir, Files: files},
		Timeline: plan.Timeline{PlanID: p.PlanID, Format: render.DefaultFormat(), Blocks: timing},
		Format:   render.DefaultFormat(),
	}, nil
}

func (f *fakeInner) RenderBlock(_ context.Context, p plan.NarrationPlan, blockID string, opts render.RenderOptions) (render.BlockRender, error) {
	f.voiceLog = append(f.voiceLog, opts.Voice)
	var blk plan.Block
	found := false
	for i := range p.Blocks {
		if p.Blocks[i].ID == blockID {
			blk = p.Blocks[i]
			found = true
			break
		}
	}
	if !found {
		return render.BlockRender{}, fmt.Errorf("fakeInner: block %q not in plan", blockID)
	}
	rel, dur, err := f.writeBlock(opts.OutDir, blk)
	if err != nil {
		return render.BlockRender{}, err
	}
	files := []string{}
	if rel != "" {
		files = []string{rel}
	}
	return render.BlockRender{
		BlockID: blockID,
		Audio:   render.AudioStream{Dir: opts.OutDir, Files: files},
		Timing:  plan.BlockTiming{BlockID: blockID, StartMs: 0, EndMs: dur, AudioRef: rel},
		Format:  render.DefaultFormat(),
	}, nil
}

// ---- test helpers ----

// silentWAV builds a canonical 44-byte-header mono s16le WAV of ms milliseconds.
func silentWAV(sampleRate, ms int) []byte {
	dataBytes := sampleRate * ms / 1000 * 2
	buf := make([]byte, 44+dataBytes)
	copy(buf[0:4], "RIFF")
	binary.LittleEndian.PutUint32(buf[4:8], uint32(36+dataBytes))
	copy(buf[8:12], "WAVE")
	copy(buf[12:16], "fmt ")
	binary.LittleEndian.PutUint32(buf[16:20], 16)
	binary.LittleEndian.PutUint16(buf[20:22], 1)
	binary.LittleEndian.PutUint16(buf[22:24], 1)
	binary.LittleEndian.PutUint32(buf[24:28], uint32(sampleRate))
	binary.LittleEndian.PutUint32(buf[28:32], uint32(sampleRate*2))
	binary.LittleEndian.PutUint16(buf[32:34], 2)
	binary.LittleEndian.PutUint16(buf[34:36], 16)
	copy(buf[36:40], "data")
	binary.LittleEndian.PutUint32(buf[40:44], uint32(dataBytes))
	return buf
}

func fakeWorkerPath(t *testing.T) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("fake-rvc-worker.sh is bash; skipping on Windows")
	}
	abs, err := filepath.Abs("testdata/fake-rvc-worker.sh")
	if err != nil {
		t.Fatalf("abs path: %v", err)
	}
	if _, err := os.Stat(abs); err != nil {
		t.Fatalf("fake worker missing at %s: %v", abs, err)
	}
	return abs
}

func newRVC(t *testing.T, inner render.Renderer, cfg Config) *Renderer {
	t.Helper()
	if cfg.WrapperPath == "" {
		cfg.WrapperPath = fakeWorkerPath(t)
	}
	r, err := New(inner, cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return r
}

func speechBlock(id, text string) plan.Block {
	return plan.Block{
		ID: id, Class: plan.ClassProse, Level: plan.L2, Status: plan.StatusVoiced,
		Segments:   []plan.Segment{{ID: "seg", Kind: plan.SegmentKindSpeech, Text: text}},
		Provenance: plan.Provenance{VoicedBy: "verbatim", Deterministic: true, LevelAsked: plan.L2},
	}
}

func pauseBlock(id string) plan.Block {
	return plan.Block{
		ID: id, Class: plan.ClassProse, Level: plan.L2, Status: plan.StatusVoiced,
		Segments: []plan.Segment{{ID: "seg", Kind: plan.SegmentKindPause, PauseMs: 200}},
	}
}

func refusedBlock(id, msg string) plan.Block {
	return plan.Block{
		ID: id, Class: plan.ClassUnknown, Level: plan.L2, Status: plan.StatusRefused,
		Refusal: &plan.Refusal{Reason: plan.RefuseBareImage, Message: msg, Spoken: true},
	}
}

func planOf(blocks ...plan.Block) plan.NarrationPlan {
	for i := range blocks {
		blocks[i].Order = i
	}
	return plan.NarrationPlan{
		SchemaVersion: "1.0.0",
		PlanID:        "01HRVCTEST0000000000000000",
		CreatedAt:     "2026-07-22T12:00:00Z",
		Defaults:      plan.PlanDefaults{Level: plan.L2, Locale: "en"},
		Blocks:        blocks,
	}
}

func mustDataLen(t *testing.T, path string) int {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat %s: %v", path, err)
	}
	return int(info.Size()) - wavHeaderBytes
}

// ---- happy path ----

func TestRender_HappyPath_MultiBlock(t *testing.T) {
	inner := &fakeInner{}
	out := t.TempDir()
	argsLog := filepath.Join(out, "args.log")
	t.Setenv("FAKE_RVC_ARGS_LOG", argsLog)

	r := newRVC(t, inner, Config{Voice: "cool-jahns"})
	p := planOf(speechBlock("blk-001", "hello"), speechBlock("blk-002", "world"))

	res, err := r.Render(context.Background(), p, render.RenderOptions{OutDir: out})
	if err != nil {
		t.Fatalf("Render: %v", err)
	}

	// Format is 40 kHz everywhere — never a hardcoded 24000.
	for _, f := range []plan.AudioFormat{res.Format, res.Timeline.Format} {
		if f.SampleRate != 40000 || f.Channels != 1 || f.Encoding != "pcm_s16le" {
			t.Errorf("Format = %+v, want 40000/1/pcm_s16le", f)
		}
	}

	// Two blocks, monotonic, 200 ms each (8000 samples at 40 kHz).
	if len(res.Timeline.Blocks) != 2 {
		t.Fatalf("want 2 BlockTimings, got %d", len(res.Timeline.Blocks))
	}
	cursor := 0
	for i, bt := range res.Timeline.Blocks {
		if bt.StartMs != cursor {
			t.Errorf("block %d StartMs = %d, want %d", i, bt.StartMs, cursor)
		}
		if bt.EndMs-bt.StartMs != 200 {
			t.Errorf("block %d duration = %d, want 200", i, bt.EndMs-bt.StartMs)
		}
		cursor = bt.EndMs
		// ms↔byte-exact: on-disk data length matches the ms duration (80 B/ms).
		wav := filepath.Join(out, bt.AudioRef)
		if got := mustDataLen(t, wav); got != (bt.EndMs-bt.StartMs)*80 {
			t.Errorf("block %d on-disk data len = %d, want %d", i, got, (bt.EndMs-bt.StartMs)*80)
		}
	}

	// Manifest points at the 40 kHz finals in <order> <block_id> <relpath> format.
	if res.Audio.ManifestPath == "" {
		t.Fatal("ManifestPath empty")
	}
	mb, err := os.ReadFile(res.Audio.ManifestPath)
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}
	if !strings.Contains(string(mb), "0 blk-001 blk-001.wav") || !strings.Contains(string(mb), "1 blk-002 blk-002.wav") {
		t.Errorf("manifest format wrong:\n%s", mb)
	}

	// Warm-load-once: the three real-format LOAD lines, each exactly once, one PID.
	assertWarmLoadOnce(t, r.lastStderr, "cool-jahns")
	if r.lastPID <= 0 {
		t.Errorf("lastPID = %d, want a real worker PID", r.lastPID)
	}

	// Intermediate dir removed (only the RVC finals + manifest remain in OutDir).
	entries, _ := os.ReadDir(out)
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "rvc-inter-") {
			t.Errorf("intermediate dir leaked into OutDir: %s", e.Name())
		}
	}
}

func assertWarmLoadOnce(t *testing.T, stderr, voice string) {
	t.Helper()
	for _, prefix := range []string{"LOAD shared", "LOAD net_g", "LOAD index"} {
		if n := strings.Count(stderr, prefix); n != 1 {
			t.Errorf("want exactly 1 %q line, got %d\nstderr:\n%s", prefix, n, stderr)
		}
	}
	// Real-worker LOAD-line fidelity (refinement 1): kind + slug, not bare kind.
	if !strings.Contains(stderr, "LOAD shared _shared") {
		t.Errorf("missing real-format `LOAD shared _shared`:\n%s", stderr)
	}
	if !strings.Contains(stderr, "LOAD net_g "+voice) {
		t.Errorf("missing `LOAD net_g %s`:\n%s", voice, stderr)
	}
}

// ---- voice map (B3): both source-side and target-side asserted ----

func TestRender_VoiceMap(t *testing.T) {
	cases := []struct {
		target    string
		wantSrc   string
		wantRateP string // " <target> <index_rate> <pitch>"
	}{
		{"cool-jahns", "am_michael", " cool-jahns 0.75 0"},
		{"confident-neal", "af_bella", " confident-neal 0.5 0"},
	}
	for _, tc := range cases {
		t.Run(tc.target, func(t *testing.T) {
			inner := &fakeInner{}
			out := t.TempDir()
			argsLog := filepath.Join(out, "args.log")
			t.Setenv("FAKE_RVC_ARGS_LOG", argsLog)

			r := newRVC(t, inner, Config{Voice: tc.target})
			p := planOf(speechBlock("blk-001", "hi"))
			if _, err := r.Render(context.Background(), p, render.RenderOptions{OutDir: out}); err != nil {
				t.Fatalf("Render: %v", err)
			}

			// Source side: the inner renderer saw the mapped Kokoro source.
			if len(inner.voiceLog) != 1 || inner.voiceLog[0] != tc.wantSrc {
				t.Errorf("inner voice = %v, want [%s]", inner.voiceLog, tc.wantSrc)
			}
			// Target side: the worker request line carried target + index_rate + pitch 0.
			line, err := os.ReadFile(argsLog)
			if err != nil {
				t.Fatalf("read args log: %v", err)
			}
			if !strings.Contains(string(line), tc.wantRateP) {
				t.Errorf("request line missing %q; log:\n%s", tc.wantRateP, line)
			}
		})
	}
}

// ---- RenderBlock (cold, StartMs=0, 40 kHz) ----

func TestRenderBlock_Single(t *testing.T) {
	inner := &fakeInner{}
	out := t.TempDir()
	r := newRVC(t, inner, Config{Voice: "confident-neal"})
	p := planOf(speechBlock("blk-001", "one"), speechBlock("blk-002", "two"))

	br, err := r.RenderBlock(context.Background(), p, "blk-002", render.RenderOptions{OutDir: out})
	if err != nil {
		t.Fatalf("RenderBlock: %v", err)
	}
	if br.BlockID != "blk-002" {
		t.Errorf("BlockID = %q, want blk-002", br.BlockID)
	}
	if br.Timing.StartMs != 0 {
		t.Errorf("StartMs = %d, want 0 (caller patches)", br.Timing.StartMs)
	}
	if br.Timing.EndMs != 200 {
		t.Errorf("EndMs = %d, want 200", br.Timing.EndMs)
	}
	if br.Format.SampleRate != 40000 {
		t.Errorf("Format.SampleRate = %d, want 40000", br.Format.SampleRate)
	}
	if inner.voiceLog[0] != "af_bella" {
		t.Errorf("inner voice = %q, want af_bella", inner.voiceLog[0])
	}
	if _, err := os.Stat(filepath.Join(out, "blk-002.wav")); err != nil {
		t.Errorf("final wav missing: %v", err)
	}
	// Sibling block must NOT be rendered.
	if _, err := os.Stat(filepath.Join(out, "blk-001.wav")); !os.IsNotExist(err) {
		t.Errorf("RenderBlock rendered blk-001; stat err = %v", err)
	}
}

// ---- empty-text block skipped ----

func TestRender_EmptyTextBlockSkipped(t *testing.T) {
	inner := &fakeInner{}
	out := t.TempDir()
	argsLog := filepath.Join(out, "args.log")
	t.Setenv("FAKE_RVC_ARGS_LOG", argsLog)

	r := newRVC(t, inner, Config{Voice: "cool-jahns"})
	p := planOf(speechBlock("blk-001", "hi"), pauseBlock("blk-002"))

	res, err := r.Render(context.Background(), p, render.RenderOptions{OutDir: out})
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if res.Timeline.Blocks[1].AudioRef != "" {
		t.Errorf("pause block AudioRef = %q, want empty", res.Timeline.Blocks[1].AudioRef)
	}
	if res.Timeline.Blocks[1].EndMs != res.Timeline.Blocks[1].StartMs {
		t.Errorf("pause block must have zero duration")
	}
	if _, err := os.Stat(filepath.Join(out, "blk-002.wav")); !os.IsNotExist(err) {
		t.Errorf("pause block must not write a wav")
	}
	// The worker must have been asked exactly once (only the voiced block).
	log, _ := os.ReadFile(argsLog)
	if n := strings.Count(string(log), "\n"); n != 1 {
		t.Errorf("worker request count = %d, want 1 (pause block skipped)\nlog:\n%s", n, log)
	}
}

// ---- refused block IS repainted ----

func TestRender_RefusedBlockRepainted(t *testing.T) {
	inner := &fakeInner{}
	out := t.TempDir()
	r := newRVC(t, inner, Config{Voice: "cool-jahns"})
	p := planOf(refusedBlock("blk-img", "image, not described"))

	res, err := r.Render(context.Background(), p, render.RenderOptions{OutDir: out})
	if err != nil {
		t.Fatalf("refused block must render, got: %v", err)
	}
	if res.Timeline.Blocks[0].AudioRef != "blk-img.wav" {
		t.Errorf("AudioRef = %q, want blk-img.wav", res.Timeline.Blocks[0].AudioRef)
	}
	if _, err := os.Stat(filepath.Join(out, "blk-img.wav")); err != nil {
		t.Errorf("refused block wav not written: %v", err)
	}
}

// ---- path with a space (POSIX-quote round-trip) ----

func TestRender_PathWithSpace(t *testing.T) {
	inner := &fakeInner{}
	out := t.TempDir()
	r := newRVC(t, inner, Config{Voice: "cool-jahns"})
	p := planOf(speechBlock("blk 001", "spaced"))

	res, err := r.Render(context.Background(), p, render.RenderOptions{OutDir: out})
	if err != nil {
		t.Fatalf("Render with spaced path: %v", err)
	}
	if res.Timeline.Blocks[0].AudioRef != "blk 001.wav" {
		t.Errorf("AudioRef = %q, want 'blk 001.wav'", res.Timeline.Blocks[0].AudioRef)
	}
	if _, err := os.Stat(filepath.Join(out, "blk 001.wav")); err != nil {
		t.Errorf("spaced-path wav not written: %v", err)
	}
}

// ---- frame-align: non-ms-aligned worker output trimmed to whole-ms ----

func TestRender_NonMsAlignedOutputTrimmed(t *testing.T) {
	inner := &fakeInner{}
	out := t.TempDir()
	t.Setenv("FAKE_RVC_NONALIGN", "1") // worker writes 8017 samples (not whole-ms).

	r := newRVC(t, inner, Config{Voice: "cool-jahns"})
	p := planOf(speechBlock("blk-001", "hi"))

	res, err := r.Render(context.Background(), p, render.RenderOptions{OutDir: out})
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	dataLen := mustDataLen(t, filepath.Join(out, "blk-001.wav"))
	if dataLen%80 != 0 {
		t.Errorf("on-disk data len = %d, not a whole-ms (80 B/ms) multiple", dataLen)
	}
	bt := res.Timeline.Blocks[0]
	if got := (bt.EndMs - bt.StartMs) * 80; got != dataLen {
		t.Errorf("timeline ms (%d → %d bytes) != on-disk data len %d", bt.EndMs-bt.StartMs, got, dataLen)
	}
}

// ---- unknown target slug at construction ----

func TestNew_UnknownVoice(t *testing.T) {
	_, err := New(&fakeInner{}, Config{Voice: "nope", WrapperPath: fakeWorkerPath(t)})
	if !errors.Is(err, ErrUnsupportedVoice) {
		t.Fatalf("want ErrUnsupportedVoice, got %v", err)
	}
}

// ---- failure modes ----

func TestRender_WorkerMissing_BadPath(t *testing.T) {
	r := newRVC(t, &fakeInner{}, Config{Voice: "cool-jahns", WrapperPath: "/nonexistent/rvc-nope"})
	_, err := r.Render(context.Background(), planOf(speechBlock("b", "x")), render.RenderOptions{OutDir: t.TempDir()})
	if !errors.Is(err, ErrWorkerMissing) {
		t.Fatalf("want ErrWorkerMissing, got %v", err)
	}
	if !strings.Contains(err.Error(), "rvc-worker-venv") {
		t.Errorf("missing fix hint `make rvc-worker-venv`: %v", err)
	}
}

func TestRender_WrapperExit2_VenvMissing(t *testing.T) {
	inner := &fakeInner{}
	t.Setenv("FAKE_RVC_EXIT2", "1")
	r := newRVC(t, inner, Config{Voice: "cool-jahns"})
	_, err := r.Render(context.Background(), planOf(speechBlock("b", "x")), render.RenderOptions{OutDir: t.TempDir()})
	if !errors.Is(err, ErrWorkerMissing) {
		t.Fatalf("want ErrWorkerMissing (wrapper exit 2), got %v", err)
	}
}

func TestRender_StartupFatal78(t *testing.T) {
	inner := &fakeInner{}
	t.Setenv("FAKE_RVC_FATAL_STARTUP", "1")
	r := newRVC(t, inner, Config{Voice: "cool-jahns"})
	_, err := r.Render(context.Background(), planOf(speechBlock("b", "x")), render.RenderOptions{OutDir: t.TempDir()})
	if !errors.Is(err, ErrWorkerStartup) {
		t.Fatalf("want ErrWorkerStartup, got %v", err)
	}
	if !strings.Contains(err.Error(), "rvc-export") {
		t.Errorf("missing fix hint `make rvc-export`: %v", err)
	}
}

func TestRender_RuntimeFatal70(t *testing.T) {
	inner := &fakeInner{}
	t.Setenv("FAKE_RVC_FATAL_RUNTIME", "1")
	r := newRVC(t, inner, Config{Voice: "cool-jahns"})
	_, err := r.Render(context.Background(), planOf(speechBlock("b", "x")), render.RenderOptions{OutDir: t.TempDir()})
	if !errors.Is(err, ErrWorkerRuntime) {
		t.Fatalf("want ErrWorkerRuntime, got %v", err)
	}
}

func TestRender_PerBlockERR_InferFailed(t *testing.T) {
	inner := &fakeInner{}
	t.Setenv("FAKE_RVC_ERR", "infer-failed")
	r := newRVC(t, inner, Config{Voice: "cool-jahns"})
	_, err := r.Render(context.Background(), planOf(speechBlock("b", "x")), render.RenderOptions{OutDir: t.TempDir()})
	if !errors.Is(err, ErrConvertFailed) {
		t.Fatalf("want ErrConvertFailed, got %v", err)
	}
	if !strings.Contains(err.Error(), "infer-failed") {
		t.Errorf("err should carry category: %v", err)
	}
}

func TestRender_UnknownErrCategory_Protocol(t *testing.T) {
	inner := &fakeInner{}
	t.Setenv("FAKE_RVC_ERR", "totally-unknown")
	r := newRVC(t, inner, Config{Voice: "cool-jahns"})
	_, err := r.Render(context.Background(), planOf(speechBlock("b", "x")), render.RenderOptions{OutDir: t.TempDir()})
	if !errors.Is(err, ErrWorkerProtocol) {
		t.Fatalf("want ErrWorkerProtocol for unknown ERR category, got %v", err)
	}
}

func TestRender_OKPathMismatch_Protocol(t *testing.T) {
	inner := &fakeInner{}
	t.Setenv("FAKE_RVC_BAD_OK", "1")
	r := newRVC(t, inner, Config{Voice: "cool-jahns"})
	_, err := r.Render(context.Background(), planOf(speechBlock("b", "x")), render.RenderOptions{OutDir: t.TempDir()})
	if !errors.Is(err, ErrWorkerProtocol) {
		t.Fatalf("want ErrWorkerProtocol for OK path mismatch, got %v", err)
	}
}

func TestRender_UnparseableResponse_Protocol(t *testing.T) {
	inner := &fakeInner{}
	t.Setenv("FAKE_RVC_GARBAGE", "1")
	r := newRVC(t, inner, Config{Voice: "cool-jahns"})
	_, err := r.Render(context.Background(), planOf(speechBlock("b", "x")), render.RenderOptions{OutDir: t.TempDir()})
	if !errors.Is(err, ErrWorkerProtocol) {
		t.Fatalf("want ErrWorkerProtocol for garbage response, got %v", err)
	}
}

func TestRender_ShortCount_CleanExit_Protocol(t *testing.T) {
	inner := &fakeInner{}
	t.Setenv("FAKE_RVC_EOF", "1")
	r := newRVC(t, inner, Config{Voice: "cool-jahns"})
	_, err := r.Render(context.Background(), planOf(speechBlock("b", "x")), render.RenderOptions{OutDir: t.TempDir()})
	if !errors.Is(err, ErrWorkerProtocol) {
		t.Fatalf("want ErrWorkerProtocol for clean-exit short count, got %v", err)
	}
}

func TestRender_PerBlockTimeout(t *testing.T) {
	inner := &fakeInner{}
	t.Setenv("FAKE_RVC_SLEEP_MS", "600")
	r := newRVC(t, inner, Config{
		Voice:             "cool-jahns",
		PerBlockTimeout:   40 * time.Millisecond,
		FirstBlockTimeout: 40 * time.Millisecond, // block 1 is the one that sleeps.
	})
	_, err := r.Render(context.Background(), planOf(speechBlock("b", "x")), render.RenderOptions{OutDir: t.TempDir()})
	if !errors.Is(err, ErrTimeout) {
		t.Fatalf("want ErrTimeout, got %v", err)
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("ErrTimeout should wrap context.DeadlineExceeded: %v", err)
	}
}

func TestRender_CallerCancel_NotRuntime(t *testing.T) {
	inner := &fakeInner{}
	t.Setenv("FAKE_RVC_SLEEP_MS", "600")
	// First-block timeout is generous so the cancel wins the race, proving the
	// ctx-first classification (cancel must NOT be mislabeled as a worker crash).
	r := newRVC(t, inner, Config{Voice: "cool-jahns", FirstBlockTimeout: 10 * time.Second})

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(40 * time.Millisecond)
		cancel()
	}()
	_, err := r.Render(ctx, planOf(speechBlock("b", "x")), render.RenderOptions{OutDir: t.TempDir()})
	if !errors.Is(err, ErrCanceled) {
		t.Fatalf("want ErrCanceled, got %v", err)
	}
	if errors.Is(err, ErrWorkerRuntime) {
		t.Errorf("cancel must NOT be classified as ErrWorkerRuntime: %v", err)
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("ErrCanceled should wrap context.Canceled: %v", err)
	}
}

// ---- unit: frame-aligner canonical-header guard ----

func TestFrameAlignWAV_NonCanonicalHeaderFailsLoud(t *testing.T) {
	raw := silentWAV(40000, 200)
	// Corrupt the 'data' chunk id so the guard must refuse rather than rewrite.
	copy(raw[36:40], "LIST")
	if _, err := frameAlignWAV(raw, rvcFormat()); err == nil {
		t.Fatal("want error on non-canonical WAV header, got nil")
	}
}

func TestFrameAlignWAV_TooShortFailsLoud(t *testing.T) {
	if _, err := frameAlignWAV([]byte("RIFF"), rvcFormat()); err == nil {
		t.Fatal("want error on sub-header WAV, got nil")
	}
}

// ---- unit: ring writer retains the last N bytes, thread-safe ----

func TestRingWriter_RetainsTail(t *testing.T) {
	rw := newRingWriter(8)
	_, _ = rw.Write([]byte("abcdefghij")) // 10 bytes into an 8-byte ring.
	if got := rw.String(); got != "cdefghij" {
		t.Errorf("ring tail = %q, want cdefghij", got)
	}
}
