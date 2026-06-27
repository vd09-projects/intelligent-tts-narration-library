package main

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/vd09-projects/intelligent-tts-narration-library/adapter"
	"github.com/vd09-projects/intelligent-tts-narration-library/adapter/file"
	"github.com/vd09-projects/intelligent-tts-narration-library/adapter/mcptext"
	"github.com/vd09-projects/intelligent-tts-narration-library/intelligence"
	"github.com/vd09-projects/intelligent-tts-narration-library/pipeline"
	"github.com/vd09-projects/intelligent-tts-narration-library/plan"
	"github.com/vd09-projects/intelligent-tts-narration-library/render"
	"github.com/vd09-projects/intelligent-tts-narration-library/sink"
	"github.com/vd09-projects/intelligent-tts-narration-library/sink/persistent"
)

// ---- test doubles -----------------------------------------------------------

// tinyWAV builds a minimal valid 24 kHz / mono / s16le RIFF/WAVE around pcm.
// Used by fileStubRenderer to lay down per-block WAVs the real WAVFileSink can
// read back, exercising the file path end-to-end without spawning Kokoro.
func tinyWAV(pcm []byte) []byte {
	const sampleRate, channels, bits = 24000, 1, 16
	blockAlign := channels * bits / 8
	byteRate := sampleRate * blockAlign
	buf := &bytes.Buffer{}
	buf.WriteString("RIFF")
	_ = binary.Write(buf, binary.LittleEndian, uint32(36+len(pcm)))
	buf.WriteString("WAVE")
	buf.WriteString("fmt ")
	_ = binary.Write(buf, binary.LittleEndian, uint32(16))
	_ = binary.Write(buf, binary.LittleEndian, uint16(1)) // PCM
	_ = binary.Write(buf, binary.LittleEndian, uint16(channels))
	_ = binary.Write(buf, binary.LittleEndian, uint32(sampleRate))
	_ = binary.Write(buf, binary.LittleEndian, uint32(byteRate))
	_ = binary.Write(buf, binary.LittleEndian, uint16(blockAlign))
	_ = binary.Write(buf, binary.LittleEndian, uint16(bits))
	buf.WriteString("data")
	_ = binary.Write(buf, binary.LittleEndian, uint32(len(pcm)))
	buf.Write(pcm)
	return buf.Bytes()
}

// fileStubRenderer is a render.Renderer that writes one tiny valid WAV per plan
// block into opts.OutDir and returns a 1:1 Timeline. It lets the with-path
// tests drive the REAL WAVFileSink (via newFilePipeline) without Kokoro.
type fileStubRenderer struct{}

func (fileStubRenderer) Render(_ context.Context, p plan.NarrationPlan, opts render.RenderOptions) (render.RenderResult, error) {
	format := render.DefaultFormat()
	blocks := p.Blocks
	if len(blocks) == 0 {
		blocks = []plan.Block{{ID: "b001"}}
	}
	var timing []plan.BlockTiming
	var files []string
	cursor := 0
	for i, b := range blocks {
		name := fmt.Sprintf("blk-%03d.wav", i)
		if err := os.WriteFile(filepath.Join(opts.OutDir, name), tinyWAV([]byte{1, 2, 3, 4}), 0o644); err != nil {
			return render.RenderResult{}, err
		}
		timing = append(timing, plan.BlockTiming{BlockID: b.ID, StartMs: cursor, EndMs: cursor + 100, AudioRef: name})
		files = append(files, name)
		cursor += 100
	}
	return render.RenderResult{
		Audio:    render.AudioStream{Dir: opts.OutDir, Files: files},
		Timeline: plan.Timeline{PlanID: p.PlanID, Format: format, Blocks: timing},
		Format:   format,
	}, nil
}

func (fileStubRenderer) RenderBlock(_ context.Context, _ plan.NarrationPlan, _ string, _ render.RenderOptions) (render.BlockRender, error) {
	return render.BlockRender{}, errors.New("RenderBlock not used in speak_to_file tests")
}

// withRealSinkFilePipeline swaps newFilePipeline for a pipeline wiring the real
// input adapter + real planner + fileStubRenderer + the REAL WAVFileSink, so a
// genuine .wav lands at wavPath. Returns a restore func.
func withRealSinkFilePipeline(t *testing.T) func() {
	t.Helper()
	orig := newFilePipeline
	newFilePipeline = func(renderDir, wavPath string, args speakArgs, input adapter.InputAdapter, intel intelligence.IntelligenceAdapter) pipeline.Narrator {
		return pipeline.New(
			input,
			intel,
			fileStubRenderer{},
			persistent.NewWAVFile(wavPath),
			pipeline.PipelineDefaults{Level: plan.Level(args.Level), OutDir: renderDir, Locale: "en"},
		)
	}
	return func() { newFilePipeline = orig }
}

// withStubFilePipeline swaps newFilePipeline for a stub narrator that records
// the wired input adapter + source ref, without writing any file. Used to
// assert adapter routing at the seam.
func withStubFilePipeline(t *testing.T, stub *stubNarrator) func() {
	t.Helper()
	orig := newFilePipeline
	newFilePipeline = func(renderDir, _ string, _ speakArgs, input adapter.InputAdapter, intel intelligence.IntelligenceAdapter) pipeline.Narrator {
		stub.gotOutDir = renderDir
		stub.gotInput = input
		stub.gotIntel = intel
		return stub
	}
	return func() { newFilePipeline = orig }
}

// ---- AC1: tool registered ---------------------------------------------------

func TestSpeakToFile_Registered(t *testing.T) {
	t.Parallel()

	// Construction must not panic — the SDK validates each tool's schema at
	// AddTool time, so a clean newServer is a real assertion that speak_to_file
	// registered alongside speak + speak_last (3 tools).
	func() {
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("newServer panicked registering speak_to_file: %v", r)
			}
		}()
		if s := newServer(runDeps{run: func(context.Context, speakArgs) (speakResponse, error) {
			return speakResponse{}, nil
		}}); s == nil {
			t.Fatal("want non-nil server")
		}
	}()

	// The arg schema (via JSON tags) must expose text, source, output_path,
	// level, gender.
	tags := jsonTagSet(reflect.TypeOf(speakToFileArgs{}))
	for _, want := range []string{"text", "source", "output_path", "level", "gender"} {
		if !tags[want] {
			t.Errorf("speakToFileArgs missing json field %q (have %v)", want, tags)
		}
	}
}

// TestSpeakToFile_NoSinkArg (S1) — speak_to_file selects its sink via
// output_path, so the `sink` arg must NOT appear in its input schema; a leaked
// sink would let sink:"persistent" mis-error on a tool whose job is writing a
// file. The pre-existing `speak` tool keeps its sink arg unchanged.
func TestSpeakToFile_NoSinkArg(t *testing.T) {
	t.Parallel()

	if tags := jsonTagSet(reflect.TypeOf(speakToFileArgs{})); tags["sink"] {
		t.Errorf("speak_to_file schema must NOT advertise a sink field (have %v)", tags)
	}
	if tags := jsonTagSet(reflect.TypeOf(speakArgs{})); !tags["sink"] {
		t.Errorf("speak schema must still advertise its sink field (have %v)", tags)
	}
}

// jsonTagSet collects the json field names of a struct, descending into
// anonymous embedded structs (so speakArgs' fields count toward speakToFileArgs).
func jsonTagSet(rt reflect.Type) map[string]bool {
	out := map[string]bool{}
	for i := 0; i < rt.NumField(); i++ {
		f := rt.Field(i)
		if f.Anonymous && f.Type.Kind() == reflect.Struct {
			for k := range jsonTagSet(f.Type) {
				out[k] = true
			}
			continue
		}
		tag := f.Tag.Get("json")
		if tag == "" || tag == "-" {
			continue
		}
		out[strings.Split(tag, ",")[0]] = true
	}
	return out
}

// ---- AC2: input routing (text vs file) --------------------------------------

func TestSpeakToFile_InputRouting(t *testing.T) {
	// Cannot t.Parallel() — mutates package-level newFilePipeline.
	tmp := t.TempDir()
	srcFile := filepath.Join(tmp, "notes.md")
	if err := os.WriteFile(srcFile, []byte("# Title\n\nsome prose here\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name      string
		args      speakToFileArgs
		wantKind  plan.SourceKind
		wantInput any
	}{
		{
			name:      "inline text routes through mcptext",
			args:      speakToFileArgs{Text: "hello world", OutputPath: filepath.Join(tmp, "a.wav")},
			wantKind:  plan.SourceKindMCPText,
			wantInput: &mcptext.Adapter{},
		},
		{
			name:      "file source routes through file adapter",
			args:      speakToFileArgs{Source: srcFile, OutputPath: filepath.Join(tmp, "b.wav")},
			wantKind:  plan.SourceKindFile,
			wantInput: &file.Adapter{},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			stub := &stubNarrator{receipt: sink.SinkReceipt{BlocksPlayed: 1, TotalDurationMs: 50}}
			restore := withStubFilePipeline(t, stub)
			defer restore()

			resp, err := runSpeakToFile(context.Background(), tc.args, nil)
			if err != nil {
				t.Fatalf("runSpeakToFile: %v", err)
			}
			if stub.gotRef.Kind != tc.wantKind {
				t.Errorf("SourceRef.Kind: got %v want %v", stub.gotRef.Kind, tc.wantKind)
			}
			if reflect.TypeOf(stub.gotInput) != reflect.TypeOf(tc.wantInput) {
				t.Errorf("wired adapter: got %T want %T", stub.gotInput, tc.wantInput)
			}
			if resp.OutputPath == "" {
				t.Error("with-path branch must return a non-empty resolved output_path")
			}
			if resp.BlocksWritten != 1 {
				t.Errorf("BlocksWritten: got %d want 1", resp.BlocksWritten)
			}
		})
	}
}

// ---- AC3: output resolution (file vs directory), real sink writes the wav ----

func TestSpeakToFile_OutputResolution(t *testing.T) {
	// Cannot t.Parallel() — mutates package-level newFilePipeline.
	restore := withRealSinkFilePipeline(t)
	defer restore()

	srcDir := t.TempDir()
	srcFile := filepath.Join(srcDir, "notes.md")
	if err := os.WriteFile(srcFile, []byte("alpha beta gamma\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	t.Run("explicit file path written verbatim", func(t *testing.T) {
		outDir := t.TempDir()
		want := filepath.Join(outDir, "speech.wav")
		resp, err := runSpeakToFile(context.Background(), speakToFileArgs{
			Text: "hello there", OutputPath: want,
		}, nil)
		if err != nil {
			t.Fatalf("runSpeakToFile: %v", err)
		}
		assertWAVAtPath(t, resp.OutputPath, want, outDir)
	})

	t.Run("directory path gets derived filename from inline text", func(t *testing.T) {
		outDir := t.TempDir()
		resp, err := runSpeakToFile(context.Background(), speakToFileArgs{
			Text: "hello there", OutputPath: outDir,
		}, nil)
		if err != nil {
			t.Fatalf("runSpeakToFile: %v", err)
		}
		base := filepath.Base(resp.OutputPath)
		if !strings.HasPrefix(base, "narration-") || !strings.HasSuffix(base, ".wav") {
			t.Errorf("inline-text derived name: got %q want narration-<hash>.wav", base)
		}
		if filepath.Dir(resp.OutputPath) != outDir {
			t.Errorf("derived file must sit inside %q, got %q", outDir, resp.OutputPath)
		}
		assertWAVAtPath(t, resp.OutputPath, resp.OutputPath, outDir)
	})

	t.Run("directory path gets derived filename from file source", func(t *testing.T) {
		outDir := t.TempDir()
		resp, err := runSpeakToFile(context.Background(), speakToFileArgs{
			Source: srcFile, OutputPath: outDir,
		}, nil)
		if err != nil {
			t.Fatalf("runSpeakToFile: %v", err)
		}
		want := filepath.Join(outDir, "notes.wav")
		assertWAVAtPath(t, resp.OutputPath, want, outDir)
	})
}

// assertWAVAtPath asserts: response path == wantPath, a valid RIFF/WAVE exists
// there, and dir holds NO plan.json / manifest.json sidecars (AC5).
func assertWAVAtPath(t *testing.T, gotPath, wantPath, dir string) {
	t.Helper()
	if gotPath != wantPath {
		t.Errorf("output_path: got %q want %q", gotPath, wantPath)
	}
	raw, err := os.ReadFile(gotPath)
	if err != nil {
		t.Fatalf("read output wav: %v", err)
	}
	if len(raw) < 44 || string(raw[0:4]) != "RIFF" || string(raw[8:12]) != "WAVE" {
		t.Errorf("output is not a RIFF/WAVE file (len=%d)", len(raw))
	}
	for _, name := range []string{"plan.json", "manifest.json"} {
		if _, err := os.Stat(filepath.Join(dir, name)); !os.IsNotExist(err) {
			t.Errorf("no %s sidecar expected (stat err: %v)", name, err)
		}
	}
}

// ---- AC4: no-path fallback returns the uniform response, writes no file ------

func TestSpeakToFile_NoPathFallback(t *testing.T) {
	// Cannot t.Parallel() — mutates package-level newPipeline (ephemeral seam).
	stub := &stubNarrator{
		receipt: sink.SinkReceipt{BlocksPlayed: 2, TotalDurationMs: 321},
		blockSummaries: []pipeline.BlockSummary{
			{ID: "b001", Order: 0, Level: plan.L1, Status: plan.StatusVoiced, SpokenText: "hello"},
			{ID: "b002", Order: 1, Level: plan.L1, Status: plan.StatusVoiced, SpokenText: "world"},
		},
	}
	restore := withStubPipeline(t, stub)
	defer restore()

	// A uniform response is the SAME type the with-path branch returns.
	var resp speakToFileResponse
	resp, err := runSpeakToFile(context.Background(), speakToFileArgs{
		Text: "hello world", // no OutputPath
	}, nil)
	if err != nil {
		t.Fatalf("runSpeakToFile: %v", err)
	}

	if resp.OutputPath != "" {
		t.Errorf("no-path branch must return output_path=\"\" (sentinel), got %q", resp.OutputPath)
	}
	if resp.BlocksWritten != 2 || resp.TotalDurationMs != 321 {
		t.Errorf("counts: got {BlocksWritten:%d TotalDurationMs:%d} want {2 321}",
			resp.BlocksWritten, resp.TotalDurationMs)
	}
	if len(resp.Transcript) != 2 {
		t.Fatalf("transcript: got %d entries want 2", len(resp.Transcript))
	}
	if resp.Transcript[0].SpokenText != "hello" || resp.Transcript[1].SpokenText != "world" {
		t.Errorf("transcript text drift: %+v", resp.Transcript)
	}
}

// ---- resolveOutputPath: pure file-vs-dir rule -------------------------------

func TestResolveOutputPath(t *testing.T) {
	t.Parallel()

	fileSrc := plan.SourceRef{Kind: plan.SourceKindFile, URI: "/abs/path/notes.md"}
	// Build the inline URI from mcptext's EXPORTED scheme (not a local literal)
	// so a real scheme drift in adapter/mcptext would break this case — the
	// derived name relies on mcptext.InlineHash parsing this exact prefix.
	textSrc := plan.SourceRef{Kind: plan.SourceKindMCPText, URI: mcptext.URIScheme + "deadbeefcafef00d0102030405060708"}

	t.Run("explicit .wav file kept as-is", func(t *testing.T) {
		t.Parallel()
		in := filepath.Join(t.TempDir(), "out.wav")
		got := mustResolve(t, in, fileSrc)
		if got != in {
			t.Errorf("got %q want %q", got, in)
		}
	})

	t.Run("extension-less file gets .wav appended", func(t *testing.T) {
		t.Parallel()
		in := filepath.Join(t.TempDir(), "out")
		got := mustResolve(t, in, fileSrc)
		if got != in+".wav" {
			t.Errorf("got %q want %q", got, in+".wav")
		}
	})

	t.Run("uppercase .WAV not double-appended", func(t *testing.T) {
		t.Parallel()
		in := filepath.Join(t.TempDir(), "OUT.WAV")
		got := mustResolve(t, in, fileSrc)
		if got != in {
			t.Errorf("got %q want %q (no double extension)", got, in)
		}
	})

	t.Run("trailing-separator treated as directory", func(t *testing.T) {
		t.Parallel()
		dir := filepath.Join(t.TempDir(), "sub")
		got := mustResolve(t, dir+string(os.PathSeparator), fileSrc)
		want := filepath.Join(dir, "notes.wav")
		if got != want {
			t.Errorf("got %q want %q", got, want)
		}
	})

	t.Run("existing directory gets derived name (file source)", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		got := mustResolve(t, dir, fileSrc)
		want := filepath.Join(dir, "notes.wav")
		if got != want {
			t.Errorf("got %q want %q", got, want)
		}
	})

	t.Run("existing directory gets derived name (inline text)", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		got := mustResolve(t, dir, textSrc)
		want := filepath.Join(dir, "narration-deadbeef.wav")
		if got != want {
			t.Errorf("got %q want %q", got, want)
		}
	})

	t.Run("missing parent dirs are created", func(t *testing.T) {
		t.Parallel()
		in := filepath.Join(t.TempDir(), "a", "b", "c.wav")
		got := mustResolve(t, in, fileSrc)
		if got != in {
			t.Errorf("got %q want %q", got, in)
		}
		if _, err := os.Stat(filepath.Dir(in)); err != nil {
			t.Errorf("parent dirs not created: %v", err)
		}
	})

	t.Run("empty output_path is a programmer error", func(t *testing.T) {
		t.Parallel()
		if _, err := resolveOutputPath("", fileSrc); err == nil {
			t.Error("want error on empty output_path, got nil")
		}
	})
}

func mustResolve(t *testing.T, outputPath string, src plan.SourceRef) string {
	t.Helper()
	got, err := resolveOutputPath(outputPath, src)
	if err != nil {
		t.Fatalf("resolveOutputPath(%q): %v", outputPath, err)
	}
	return got
}
