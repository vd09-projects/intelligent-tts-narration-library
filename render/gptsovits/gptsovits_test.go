package gptsovits

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/vd09-projects/intelligent-tts-narration-library/plan"
	"github.com/vd09-projects/intelligent-tts-narration-library/render"
)

const fakeWorker = "./testdata/fake-gptsovits.sh"

// ── helpers ──────────────────────────────────────────────────────────────────

// newModelsRoot lays down a temp packaged-artifacts tree the engine reads to build
// the wire ref/prompt tokens. Only ref_transcript.txt is needed — the engine does
// not stat ref_audio.wav (the worker owns read-failed), and the fake worker ignores
// both, so no .wav fixture is committed (no golden audio in git).
func newModelsRoot(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	vdir := filepath.Join(root, "cool-jahns-gso")
	if err := os.MkdirAll(vdir, 0o755); err != nil {
		t.Fatalf("mkdir voice dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(vdir, "ref_transcript.txt"), []byte("this is the reference transcript.\n"), 0o644); err != nil {
		t.Fatalf("write transcript: %v", err)
	}
	return root
}

// newEngine builds an Engine wired to the fake worker + a temp models root, with
// short timeouts so timeout/cancel tests are fast. Callers override via extra cfg.
func newEngine(t *testing.T, models string) *Engine {
	t.Helper()
	return New(EngineConfig{WrapperPath: fakeWorker, ModelsRoot: models})
}

func speechN(texts ...string) plan.NarrationPlan {
	blocks := make([]plan.Block, 0, len(texts))
	for i, tx := range texts {
		b := plan.Block{
			ID:     blockID(i),
			Order:  i,
			Status: plan.StatusVoiced,
			Segments: []plan.Segment{
				{ID: "s1", Kind: plan.SegmentKindSpeech, Text: tx},
			},
		}
		blocks = append(blocks, b)
	}
	return plan.NarrationPlan{PlanID: "test-plan", Blocks: blocks}
}

func blockID(i int) string {
	return "b" + string(rune('0'+i))
}

func durOf(bt plan.BlockTiming) int { return bt.EndMs - bt.StartMs }

// ── AC4: happy multi-block ───────────────────────────────────────────────────

func TestRender_HappyMultiBlock(t *testing.T) {
	models := newModelsRoot(t)
	e := newEngine(t, models)
	out := t.TempDir()

	p := speechN("first block", "second block", "third block")
	res, err := e.Render(context.Background(), p, render.RenderOptions{OutDir: out})
	if err != nil {
		t.Fatalf("Render: %v", err)
	}

	// Three BlockTiming rows, monotonic non-overlapping cursor, each 200 ms.
	if len(res.Timeline.Blocks) != 3 {
		t.Fatalf("Timeline.Blocks = %d, want 3", len(res.Timeline.Blocks))
	}
	wantStart := 0
	for i, bt := range res.Timeline.Blocks {
		if bt.StartMs != wantStart {
			t.Errorf("block %d StartMs = %d, want %d (monotonic)", i, bt.StartMs, wantStart)
		}
		if durOf(bt) != 200 {
			t.Errorf("block %d duration = %d ms, want 200", i, durOf(bt))
		}
		if bt.AudioRef != blockID(i)+".wav" {
			t.Errorf("block %d AudioRef = %q, want %q", i, bt.AudioRef, blockID(i)+".wav")
		}
		wantStart = bt.EndMs
	}

	// Three-way format single-origin (engine leg): RenderResult.Format ==
	// Timeline.Format == OutputFormat() == the 32 kHz spec. The roster leg
	// (VoiceInfo.Format == OutputFormat()) is asserted in pipeline/voices_test.go.
	want := plan.AudioFormat{SampleRate: 32000, Channels: 1, Encoding: "pcm_s16le"}
	if res.Format != want || res.Format != OutputFormat() || res.Timeline.Format != OutputFormat() {
		t.Errorf("format single-origin broken: Format=%+v Timeline.Format=%+v OutputFormat=%+v want=%+v",
			res.Format, res.Timeline.Format, OutputFormat(), want)
	}

	// Warm-load-once: exactly one LOAD gso across the 3-block batch.
	if n := strings.Count(e.lastStderr, "LOAD gso"); n != 1 {
		t.Errorf("LOAD gso count = %d, want 1 (one warm worker per document)\nstderr: %s", n, e.lastStderr)
	}

	// Files list holds the three non-empty blocks; each WAV is on disk, 32 kHz whole-ms.
	if len(res.Audio.Files) != 3 {
		t.Errorf("Audio.Files = %v, want 3 entries", res.Audio.Files)
	}
	for i := range res.Timeline.Blocks {
		assertWholeMs32k(t, filepath.Join(out, blockID(i)+".wav"))
	}
}

// ── AC4: empty-text block skipped ────────────────────────────────────────────

func TestRender_EmptyTextBlockSkipped(t *testing.T) {
	models := newModelsRoot(t)
	e := newEngine(t, models)
	out := t.TempDir()

	p := plan.NarrationPlan{PlanID: "p", Blocks: []plan.Block{
		{ID: "b0", Order: 0, Status: plan.StatusVoiced, Segments: []plan.Segment{{ID: "s1", Kind: plan.SegmentKindSpeech, Text: "spoken"}}},
		{ID: "b1", Order: 1, Status: plan.StatusVoiced, Segments: []plan.Segment{{ID: "s1", Kind: plan.SegmentKindPause, PauseMs: 100}}},
		{ID: "b2", Order: 2, Status: plan.StatusVoiced, Segments: []plan.Segment{{ID: "s1", Kind: plan.SegmentKindSpeech, Text: "again"}}},
	}}
	res, err := e.Render(context.Background(), p, render.RenderOptions{OutDir: out})
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if len(res.Timeline.Blocks) != 3 {
		t.Fatalf("Timeline.Blocks = %d, want 3 (empty block still present)", len(res.Timeline.Blocks))
	}
	empty := res.Timeline.Blocks[1]
	if empty.BlockID != "b1" || durOf(empty) != 0 || empty.AudioRef != "" {
		t.Errorf("empty block timing = %+v, want zero-dur empty AudioRef", empty)
	}
	// The empty block is NOT in Files and produced no WAV.
	for _, f := range res.Audio.Files {
		if f == "b1.wav" {
			t.Errorf("empty block WAV leaked into Files: %v", res.Audio.Files)
		}
	}
	if _, err := os.Stat(filepath.Join(out, "b1.wav")); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("empty block WAV should not exist, stat err = %v", err)
	}
	// b2 starts exactly where b0 ended (empty block adds no time).
	if res.Timeline.Blocks[2].StartMs != res.Timeline.Blocks[0].EndMs {
		t.Errorf("b2 StartMs = %d, want %d", res.Timeline.Blocks[2].StartMs, res.Timeline.Blocks[0].EndMs)
	}
}

// ── AC4: refused block is voiced; empty Refusal.Message → ErrMalformedPlan ─────

func TestRender_RefusedBlockVoiced(t *testing.T) {
	models := newModelsRoot(t)
	e := newEngine(t, models)
	out := t.TempDir()

	p := plan.NarrationPlan{PlanID: "p", Blocks: []plan.Block{
		{ID: "b0", Order: 0, Status: plan.StatusRefused, Refusal: &plan.Refusal{Message: "image not described"}},
	}}
	res, err := e.Render(context.Background(), p, render.RenderOptions{OutDir: out})
	if err != nil {
		t.Fatalf("Render refused block: %v", err)
	}
	if len(res.Timeline.Blocks) != 1 || durOf(res.Timeline.Blocks[0]) != 200 {
		t.Errorf("refused block should be voiced (200 ms), got %+v", res.Timeline.Blocks)
	}
}

func TestRender_RefusedEmptyMessageMalformed(t *testing.T) {
	models := newModelsRoot(t)
	e := newEngine(t, models)
	p := plan.NarrationPlan{PlanID: "p", Blocks: []plan.Block{
		{ID: "b0", Order: 0, Status: plan.StatusRefused, Refusal: &plan.Refusal{Message: "   "}},
	}}
	_, err := e.Render(context.Background(), p, render.RenderOptions{OutDir: t.TempDir()})
	if !errors.Is(err, ErrMalformedPlan) {
		t.Fatalf("err = %v, want ErrMalformedPlan", err)
	}
}

// ── AC4 / D5: per-block ERR HARD-fails the whole Render ───────────────────────

func TestRender_PerBlockErrHardStops(t *testing.T) {
	t.Setenv("FAKE_GSO_ERR", "infer-failed")
	models := newModelsRoot(t)
	e := newEngine(t, models)

	p := speechN("one", "two", "three")
	res, err := e.Render(context.Background(), p, render.RenderOptions{OutDir: t.TempDir()})
	if !errors.Is(err, ErrWorkerError) {
		t.Fatalf("err = %v, want ErrWorkerError (D5 hard stop)", err)
	}
	// No partial timeline — the whole Render fails, not per-block skip/degrade.
	if len(res.Timeline.Blocks) != 0 || res.Format != (plan.AudioFormat{}) {
		t.Errorf("expected zero RenderResult on D5 hard stop, got %+v", res)
	}
}

// ── AC4: worker lifecycle error taxonomy ─────────────────────────────────────

func TestRender_ErrorTaxonomy(t *testing.T) {
	cases := []struct {
		name    string
		env     map[string]string
		wrapper string
		want    error
	}{
		{name: "worker missing", wrapper: "./testdata/does-not-exist.sh", want: ErrWorkerMissing},
		{name: "exit 2 → missing", env: map[string]string{"FAKE_GSO_EXIT2": "1"}, want: ErrWorkerMissing},
		{name: "exit 78 → startup", env: map[string]string{"FAKE_GSO_FATAL_STARTUP": "1"}, want: ErrWorkerStartup},
		{name: "exit 70 → runtime", env: map[string]string{"FAKE_GSO_FATAL_RUNTIME": "1"}, want: ErrWorkerRuntime},
		{name: "unknown ERR category → protocol", env: map[string]string{"FAKE_GSO_ERR": "not-a-category"}, want: ErrWorkerProtocol},
		{name: "garbage line → protocol", env: map[string]string{"FAKE_GSO_GARBAGE": "1"}, want: ErrWorkerProtocol},
		{name: "EOF before reply → protocol", env: map[string]string{"FAKE_GSO_EOF": "1"}, want: ErrWorkerProtocol},
		{name: "OK empty path → protocol", env: map[string]string{"FAKE_GSO_EMPTY_OK": "1"}, want: ErrWorkerProtocol},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			for k, v := range tc.env {
				t.Setenv(k, v)
			}
			models := newModelsRoot(t)
			wrapper := fakeWorker
			if tc.wrapper != "" {
				wrapper = tc.wrapper
			}
			e := New(EngineConfig{WrapperPath: wrapper, ModelsRoot: models})
			_, err := e.Render(context.Background(), speechN("hi"), render.RenderOptions{OutDir: t.TempDir()})
			if !errors.Is(err, tc.want) {
				t.Fatalf("err = %v, want %v", err, tc.want)
			}
		})
	}
}

// ── AC4: per-block timeout → ErrTimeout, caller-cancel → ErrCanceled (ctx-first) ─

func TestRender_Timeout(t *testing.T) {
	t.Setenv("FAKE_GSO_SLEEP_MS", "1500")
	models := newModelsRoot(t)
	e := New(EngineConfig{WrapperPath: fakeWorker, ModelsRoot: models,
		FirstBlockTimeout: 60 * time.Millisecond, PerBlockTimeout: 60 * time.Millisecond})
	_, err := e.Render(context.Background(), speechN("slow"), render.RenderOptions{OutDir: t.TempDir()})
	if !errors.Is(err, ErrTimeout) {
		t.Fatalf("err = %v, want ErrTimeout", err)
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("ErrTimeout should wrap context.DeadlineExceeded, got %v", err)
	}
}

func TestRender_Canceled(t *testing.T) {
	t.Setenv("FAKE_GSO_SLEEP_MS", "1500")
	models := newModelsRoot(t)
	e := newEngine(t, models)
	ctx, cancel := context.WithCancel(context.Background())
	time.AfterFunc(60*time.Millisecond, cancel)
	_, err := e.Render(ctx, speechN("slow"), render.RenderOptions{OutDir: t.TempDir()})
	if !errors.Is(err, ErrCanceled) {
		t.Fatalf("err = %v, want ErrCanceled (ctx-first classification)", err)
	}
}

// ── AC4: RenderBlock touches only the target block ───────────────────────────

func TestRenderBlock_SingleBlock(t *testing.T) {
	models := newModelsRoot(t)
	e := newEngine(t, models)
	out := t.TempDir()
	p := speechN("first", "second", "third")

	br, err := e.RenderBlock(context.Background(), p, "b1", render.RenderOptions{OutDir: out})
	if err != nil {
		t.Fatalf("RenderBlock: %v", err)
	}
	if br.BlockID != "b1" || br.Timing.StartMs != 0 || durOf(br.Timing) != 200 || br.Timing.AudioRef != "b1.wav" {
		t.Errorf("BlockRender = %+v, want b1 StartMs:0 200ms AudioRef:b1.wav (caller patches StartMs)", br)
	}
	if br.Format != OutputFormat() {
		t.Errorf("BlockRender.Format = %+v, want %+v", br.Format, OutputFormat())
	}
	// Only the target block's WAV exists — siblings are untouched.
	for _, sib := range []string{"b0.wav", "b2.wav"} {
		if _, err := os.Stat(filepath.Join(out, sib)); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("sibling %s should not exist after RenderBlock, stat err = %v", sib, err)
		}
	}
}

func TestRenderBlock_UnknownIDMalformed(t *testing.T) {
	models := newModelsRoot(t)
	e := newEngine(t, models)
	_, err := e.RenderBlock(context.Background(), speechN("only"), "nope", render.RenderOptions{OutDir: t.TempDir()})
	if !errors.Is(err, ErrMalformedPlan) {
		t.Fatalf("err = %v, want ErrMalformedPlan", err)
	}
}

// ── review suggestion 1: env isolation — distinct GSO_OUT_DIR + inherited env ──

func TestRender_EnvIsolation(t *testing.T) {
	t.Setenv("GSO_INHERIT_PROBE", "sentinel-inherit-xyz")
	models := newModelsRoot(t)
	e := newEngine(t, models)
	p := speechN("hi")

	if _, err := e.Render(context.Background(), p, render.RenderOptions{OutDir: t.TempDir()}); err != nil {
		t.Fatalf("Render 1: %v", err)
	}
	dir1 := e.lastOutDir
	stderr1 := e.lastStderr

	if _, err := e.Render(context.Background(), p, render.RenderOptions{OutDir: t.TempDir()}); err != nil {
		t.Fatalf("Render 2: %v", err)
	}
	dir2 := e.lastOutDir

	if dir1 == "" || dir1 == dir2 {
		t.Errorf("GSO_OUT_DIR not isolated per Render: dir1=%q dir2=%q (want distinct)", dir1, dir2)
	}
	// The child saw a parent-inherited env var → cmd.Env = append(os.Environ(), …),
	// NOT a single-var Env that would have wiped it (and PATH/venv with it).
	if !strings.Contains(stderr1, "sentinel-inherit-xyz") {
		t.Errorf("child did not inherit GSO_INHERIT_PROBE — env was wiped, not appended.\nstderr: %s", stderr1)
	}
}

// ── D4: content-addressed buffer discipline (identical requests) ─────────────

func TestRender_DupRequestBufferDiscipline(t *testing.T) {
	t.Setenv("FAKE_GSO_DUP_REQUEST", "1")
	models := newModelsRoot(t)
	e := newEngine(t, models)
	out := t.TempDir()

	// Two byte-identical requests → the worker mints the SAME content-addressed
	// path and overwrites it (shorter clip on the 2nd). The engine must have
	// buffered (frame-aligned into b0.wav) block 0 BEFORE issuing block 1's request,
	// so b0 keeps its 200 ms and b1 gets the 150 ms overwrite — no clobber.
	p := speechN("same words", "same words")
	res, err := e.Render(context.Background(), p, render.RenderOptions{OutDir: out})
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if durOf(res.Timeline.Blocks[0]) != 200 {
		t.Errorf("block 0 duration = %d ms, want 200 (buffered before overwrite)", durOf(res.Timeline.Blocks[0]))
	}
	if durOf(res.Timeline.Blocks[1]) != 150 {
		t.Errorf("block 1 duration = %d ms, want 150 (the overwrite)", durOf(res.Timeline.Blocks[1]))
	}
	assertWholeMs32k(t, filepath.Join(out, "b0.wav"))
	assertWholeMs32k(t, filepath.Join(out, "b1.wav"))
}

// ── frame-alignment of non-whole-ms worker output ────────────────────────────

func TestRender_NonAlignedFrameAligned(t *testing.T) {
	t.Setenv("FAKE_GSO_NONALIGN", "1")
	models := newModelsRoot(t)
	e := newEngine(t, models)
	out := t.TempDir()

	res, err := e.Render(context.Background(), speechN("odd length"), render.RenderOptions{OutDir: out})
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	// 6417 samples (12834 data bytes) rounds to 201 ms (12864 bytes, whole-ms).
	if durOf(res.Timeline.Blocks[0]) != 201 {
		t.Errorf("duration = %d ms, want 201 (nearest-ms of 6417 samples)", durOf(res.Timeline.Blocks[0]))
	}
	assertWholeMs32k(t, filepath.Join(out, "b0.wav"))
}

// ── path/space handling round-trip through the production wire ───────────────

func TestRender_SpacePathsRoundTrip(t *testing.T) {
	log := filepath.Join(t.TempDir(), "args.log")
	t.Setenv("FAKE_GSO_ARGS_LOG", log)

	// Models root with a space in the voice-dir chain → the ref_audio_path wire
	// token contains a space and must survive shlex round-trip.
	root := filepath.Join(t.TempDir(), "my models")
	vdir := filepath.Join(root, "cool-jahns-gso")
	if err := os.MkdirAll(vdir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(vdir, "ref_transcript.txt"), []byte("prompt with spaces"), 0o644); err != nil {
		t.Fatal(err)
	}
	e := New(EngineConfig{WrapperPath: fakeWorker, ModelsRoot: root})

	if _, err := e.Render(context.Background(), speechN("hello world spoken"), render.RenderOptions{OutDir: t.TempDir()}); err != nil {
		t.Fatalf("Render with spaces: %v", err)
	}

	raw, err := os.ReadFile(log)
	if err != nil {
		t.Fatalf("read args log: %v", err)
	}
	line := strings.TrimRight(string(raw), "\n")
	got := pyShlexSplit(t, line)
	wantRef, _ := filepath.Abs(filepath.Join(vdir, "ref_audio.wav"))
	want := []string{"hello world spoken", wantRef, "prompt with spaces", "cut4", "cool-jahns-gso"}
	if strings.Join(got, "\x00") != strings.Join(want, "\x00") {
		t.Errorf("wire round-trip split = %#v, want %#v\nline: %s", got, want, line)
	}
}

// ── review suggestion 2 + 3: shlex golden through the PRODUCTION buildRequest ──

func TestBuildRequest_ShlexGolden(t *testing.T) {
	if textSplitMethod != "cut4" {
		t.Fatalf("textSplitMethod = %q, want cut4 (worker rejects any other method as bad-args)", textSplitMethod)
	}
	raw, err := os.ReadFile("../../scripts/testdata/gso-shlex-golden.json")
	if err != nil {
		t.Fatalf("read golden: %v", err)
	}
	var golden struct {
		Cases []struct {
			Tokens []string `json:"tokens"`
			Wire   string   `json:"wire"`
		} `json:"cases"`
	}
	if err := json.Unmarshal(raw, &golden); err != nil {
		t.Fatalf("unmarshal golden: %v", err)
	}
	if len(golden.Cases) == 0 {
		t.Fatal("golden has no cases")
	}
	for i, c := range golden.Cases {
		if len(c.Tokens) != 5 {
			t.Fatalf("case %d: %d tokens, want 5", i, len(c.Tokens))
		}
		if c.Tokens[3] != "cut4" {
			t.Fatalf("case %d: text_split_method token = %q, want cut4", i, c.Tokens[3])
		}
		// Drive the PRODUCTION buildRequest (not just posixQuote round-trip): it
		// assembles [text ref prompt cut4 voice] and injects cut4 itself.
		line, err := buildRequest(c.Tokens[0], c.Tokens[1], c.Tokens[2], c.Tokens[4])
		if err != nil {
			t.Fatalf("case %d: buildRequest: %v", i, err)
		}
		got := pyShlexSplit(t, line)
		if strings.Join(got, "\x00") != strings.Join(c.Tokens, "\x00") {
			t.Errorf("case %d: shlex.split(buildRequest(...)) = %#v, want %#v\nline: %s", i, got, c.Tokens, line)
		}
		// The fixture's own canonical wire must also round-trip (fixture sanity).
		if fromWire := pyShlexSplit(t, c.Wire); strings.Join(fromWire, "\x00") != strings.Join(c.Tokens, "\x00") {
			t.Errorf("case %d: shlex.split(golden.wire) = %#v, want %#v", i, fromWire, c.Tokens)
		}
	}
}

func TestBuildRequest_RejectsNewline(t *testing.T) {
	if _, err := buildRequest("has\nnewline", "/ref.wav", "prompt", "cool-jahns-gso"); !errors.Is(err, ErrWorkerProtocol) {
		t.Fatalf("err = %v, want ErrWorkerProtocol (newline/CR rejected up front, bad-args parity)", err)
	}
}

// ── voice resolution ─────────────────────────────────────────────────────────

func TestResolveVoice(t *testing.T) {
	models := newModelsRoot(t)
	// default when unset
	v, err := resolveVoice(models, plan.NarrationPlan{}, render.RenderOptions{})
	if err != nil || v.slug != "cool-jahns-gso" {
		t.Fatalf("default resolve = (%+v, %v), want cool-jahns-gso", v, err)
	}
	if v.promptText != "this is the reference transcript." {
		t.Errorf("promptText = %q, want the packaged transcript (trimmed)", v.promptText)
	}
	if !filepath.IsAbs(v.refAudioPath) || !strings.HasSuffix(v.refAudioPath, "cool-jahns-gso/ref_audio.wav") {
		t.Errorf("refAudioPath = %q, want abs path ending cool-jahns-gso/ref_audio.wav", v.refAudioPath)
	}
	// unknown explicit voice → ErrUnsupportedVoice
	if _, err := resolveVoice(models, plan.NarrationPlan{}, render.RenderOptions{Voice: "bogus"}); !errors.Is(err, ErrUnsupportedVoice) {
		t.Errorf("unknown voice err = %v, want ErrUnsupportedVoice", err)
	}
	// missing packaged transcript → ErrUnsupportedVoice (config fault, pre-spawn)
	if _, err := resolveVoice(t.TempDir(), plan.NarrationPlan{}, render.RenderOptions{}); !errors.Is(err, ErrUnsupportedVoice) {
		t.Errorf("missing transcript err = %v, want ErrUnsupportedVoice", err)
	}
}

func TestSupportedVoices(t *testing.T) {
	if got := SupportedVoices(); len(got) != 1 || got[0] != "cool-jahns-gso" {
		t.Errorf("SupportedVoices() = %v, want [cool-jahns-gso]", got)
	}
	if !IsSupportedVoice("cool-jahns-gso") || IsSupportedVoice("af-bella") {
		t.Error("IsSupportedVoice membership wrong")
	}
}

// ── frame-align unit coverage ────────────────────────────────────────────────

func TestFrameAlignWAV(t *testing.T) {
	f := OutputFormat()
	// already whole-ms (6400 samples = 200 ms) → unchanged.
	whole := makeWAV(6400)
	got, err := frameAlignWAV(whole, f)
	if err != nil {
		t.Fatalf("frameAlignWAV whole: %v", err)
	}
	if len(got) != len(whole) || wavDataDurationMs(got, f) != 200 {
		t.Errorf("whole-ms input changed: len %d→%d dur %d", len(whole), len(got), wavDataDurationMs(got, f))
	}
	// non-whole-ms (6417 samples) → snapped to 201 ms, data a whole-ms multiple.
	odd := makeWAV(6417)
	got, err = frameAlignWAV(odd, f)
	if err != nil {
		t.Fatalf("frameAlignWAV odd: %v", err)
	}
	if (len(got)-wavHeaderBytes)%64 != 0 || wavDataDurationMs(got, f) != 201 {
		t.Errorf("odd input not frame-aligned: dataLen %d dur %d", len(got)-wavHeaderBytes, wavDataDurationMs(got, f))
	}
	// non-canonical header (no 'data' at offset 36) → fails LOUD.
	bad := makeWAV(64)
	copy(bad[dataChunkIDOffset:dataChunkIDOffset+4], []byte("LIST"))
	if _, err := frameAlignWAV(bad, f); err == nil {
		t.Error("non-canonical header should fail loud, got nil")
	}
}

// ── shared assert / fixture helpers ──────────────────────────────────────────

func assertWholeMs32k(t *testing.T, path string) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat %q: %v", path, err)
	}
	data := info.Size() - wavHeaderBytes
	if data < 0 || data%64 != 0 {
		t.Errorf("%q data bytes = %d, not a whole-ms multiple at 32 kHz (bytesPerMs=64)", path, data)
	}
}

// makeWAV builds an in-memory canonical 32 kHz mono s16le WAV with n silent samples.
func makeWAV(n int) []byte {
	dataBytes := n * 2
	out := make([]byte, wavHeaderBytes+dataBytes)
	copy(out[0:4], "RIFF")
	putU32(out[4:8], uint32(36+dataBytes))
	copy(out[8:12], "WAVE")
	copy(out[12:16], "fmt ")
	putU32(out[16:20], 16)
	putU16(out[20:22], 1)
	putU16(out[22:24], 1)
	putU32(out[24:28], 32000)
	putU32(out[28:32], 64000)
	putU16(out[32:34], 2)
	putU16(out[34:36], 16)
	copy(out[36:40], "data")
	putU32(out[40:44], uint32(dataBytes))
	return out
}

func putU16(b []byte, v uint16) { b[0] = byte(v); b[1] = byte(v >> 8) }
func putU32(b []byte, v uint32) {
	b[0] = byte(v)
	b[1] = byte(v >> 8)
	b[2] = byte(v >> 16)
	b[3] = byte(v >> 24)
}

// pyShlexSplit splits a wire line with Python's shlex (the worker's own splitter) so
// the round-trip is validated against the real thing, not a Go re-implementation.
// Python3 is already a hard dependency of the test suite (the fake worker uses it).
func pyShlexSplit(t *testing.T, line string) []string {
	t.Helper()
	cmd := exec.Command("python3", "-c",
		`import shlex,sys; sys.stdout.write("\x00".join(shlex.split(sys.stdin.buffer.read().decode("utf-8"))))`)
	cmd.Stdin = strings.NewReader(line)
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("pyShlexSplit(%q): %v", line, err)
	}
	if len(out) == 0 {
		return []string{}
	}
	return strings.Split(string(out), "\x00")
}
