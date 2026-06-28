package persistent

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/vd09-projects/intelligent-tts-narration-library/plan"
	"github.com/vd09-projects/intelligent-tts-narration-library/render"
	"github.com/vd09-projects/intelligent-tts-narration-library/sink"
)

func TestWAVFileSink_ImplementsOutputSink(t *testing.T) {
	t.Parallel()
	var _ sink.OutputSink = (*WAVFileSink)(nil)
	var _ sink.OutputSink = NewWAVFile("/tmp/out.wav")
}

// TestPersistentConsume_ByteParity pins the step-1 refactor: after buildSegments
// was extracted, the 3-file persistent Sink still writes audio.wav byte-for-byte
// equal to a direct writeCombinedWAV over the same per-block segments, and still
// emits its plan.json + manifest.json with the unchanged receipt. This is the
// parity proof that extracting buildSegments did not move any bytes.
func TestPersistentConsume_ByteParity(t *testing.T) {
	t.Parallel()
	p, res := twoBlockPlan(t)
	outDir := t.TempDir()

	rec, err := New(outDir, WithVoice("af_bella")).Consume(context.Background(), p, res)
	if err != nil {
		t.Fatalf("Consume: %v", err)
	}

	// Receipt unchanged: two voiced blocks, planned duration 1000.
	if rec.BlocksPlayed != 2 || rec.TotalDurationMs != 1000 {
		t.Errorf("receipt drift: got {BlocksPlayed:%d TotalDurationMs:%d} want {2 1000}",
			rec.BlocksPlayed, rec.TotalDurationMs)
	}

	// Independently recompute the expected audio.wav via the shared core.
	segs, blocks, dur, err := buildSegments(context.Background(), res, defaultExpectedFormat())
	if err != nil {
		t.Fatalf("buildSegments: %v", err)
	}
	if blocks != 2 || dur != 1000 {
		t.Errorf("buildSegments counts: got (%d,%d) want (2,1000)", blocks, dur)
	}
	var want bytes.Buffer
	if err := writeCombinedWAV(&want, defaultExpectedFormat(), segs); err != nil {
		t.Fatalf("writeCombinedWAV: %v", err)
	}

	got := mustRead(t, filepath.Join(outDir, audioFilename))
	if !bytes.Equal(got, want.Bytes()) {
		t.Errorf("audio.wav not byte-identical to direct wav-concat: got %d bytes, want %d bytes",
			len(got), want.Len())
	}

	// The 3-file sink still writes its sidecars (counter-metric: WAVFileSink is
	// the no-sidecar variant; the persistent Sink is unchanged).
	for _, name := range []string{planFilename, manifestFilename} {
		if _, err := os.Stat(filepath.Join(outDir, name)); err != nil {
			t.Errorf("persistent sink must still write %s: %v", name, err)
		}
	}
}

// TestWAVFileSink_WritesSingleWAV_NoSidecars is the core AC3/AC5 proof: the
// wav-file sink writes ONE valid 24 kHz/mono/s16le RIFF/WAVE at the configured
// path, with a non-empty data chunk, byte-identical to the shared wav-concat —
// and writes NO plan.json / manifest.json.
func TestWAVFileSink_WritesSingleWAV_NoSidecars(t *testing.T) {
	t.Parallel()
	p, res := twoBlockPlan(t)
	outDir := t.TempDir()
	wavPath := filepath.Join(outDir, "speech.wav")

	rec, err := NewWAVFile(wavPath).Consume(context.Background(), p, res)
	if err != nil {
		t.Fatalf("Consume: %v", err)
	}
	if rec.BlocksPlayed != 2 || rec.TotalDurationMs != 1000 {
		t.Errorf("receipt: got {BlocksPlayed:%d TotalDurationMs:%d} want {2 1000}",
			rec.BlocksPlayed, rec.TotalDurationMs)
	}

	raw := mustRead(t, wavPath)
	hdr, pcm, err := parseWAV(wavPath, raw, render.DefaultFormat())
	if err != nil {
		t.Fatalf("output is not a valid WAV: %v", err)
	}
	if hdr.SampleRate != 24000 || hdr.Channels != 1 || hdr.BitsPerSample != 16 {
		t.Errorf("format drift: got %dHz/%dch/%dbit want 24000/1/16",
			hdr.SampleRate, hdr.Channels, hdr.BitsPerSample)
	}
	if len(pcm) == 0 {
		t.Error("data chunk is empty; want non-empty concatenated PCM")
	}

	// No sidecars anywhere in the target directory.
	for _, name := range []string{planFilename, manifestFilename} {
		if _, err := os.Stat(filepath.Join(outDir, name)); !os.IsNotExist(err) {
			t.Errorf("WAVFileSink must NOT write %s (stat err: %v)", name, err)
		}
	}
	// Exactly one file in the dir (the wav). Anything else = stray sidecar/tmp.
	entries, err := os.ReadDir(outDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != "speech.wav" {
		var names []string
		for _, e := range entries {
			names = append(names, e.Name())
		}
		t.Errorf("target dir should contain only speech.wav, got %v", names)
	}
}

// TestWAVFileSink_Edges pins the boundary behaviors the builder settled:
//   - empty input (zero blocks) → a valid empty-but-well-formed RIFF/WAVE with a
//     zero-length data chunk (NOT an error). This is the chosen all-refused/empty
//     resolution.
//   - empty-AudioRef block → contributes leading silence only, still valid.
//   - parent directory auto-created (MkdirAll) so a not-yet-existing dir works.
//   - empty Path → ErrNoWAVPath.
func TestWAVFileSink_Edges(t *testing.T) {
	t.Parallel()

	t.Run("empty input writes valid empty wav", func(t *testing.T) {
		t.Parallel()
		wavPath := filepath.Join(t.TempDir(), "empty.wav")
		res := render.RenderResult{
			Audio:    render.AudioStream{Dir: t.TempDir()},
			Timeline: plan.Timeline{Format: render.DefaultFormat()},
			Format:   render.DefaultFormat(),
		}
		rec, err := NewWAVFile(wavPath).Consume(context.Background(), plan.NarrationPlan{}, res)
		if err != nil {
			t.Fatalf("empty input must not error: %v", err)
		}
		if rec.BlocksPlayed != 0 || rec.TotalDurationMs != 0 {
			t.Errorf("empty receipt: got {%d %d} want {0 0}", rec.BlocksPlayed, rec.TotalDurationMs)
		}
		_, pcm, err := parseWAV(wavPath, mustRead(t, wavPath), render.DefaultFormat())
		if err != nil {
			t.Fatalf("empty wav must still be a valid RIFF/WAVE: %v", err)
		}
		if len(pcm) != 0 {
			t.Errorf("empty input data chunk: got %d bytes want 0", len(pcm))
		}
	})

	t.Run("empty AudioRef block emits silence only", func(t *testing.T) {
		t.Parallel()
		wavPath := filepath.Join(t.TempDir(), "silence.wav")
		res := render.RenderResult{
			Audio: render.AudioStream{Dir: t.TempDir()},
			Timeline: plan.Timeline{
				Format: render.DefaultFormat(),
				Blocks: []plan.BlockTiming{
					{BlockID: "b001", StartMs: 0, EndMs: 100, AudioRef: ""},
				},
			},
			Format: render.DefaultFormat(),
		}
		rec, err := NewWAVFile(wavPath).Consume(context.Background(), plan.NarrationPlan{}, res)
		if err != nil {
			t.Fatalf("Consume: %v", err)
		}
		// Empty AudioRef does not increment BlocksPlayed but counts duration.
		if rec.BlocksPlayed != 0 || rec.TotalDurationMs != 100 {
			t.Errorf("silence receipt: got {%d %d} want {0 100}", rec.BlocksPlayed, rec.TotalDurationMs)
		}
		if _, _, err := parseWAV(wavPath, mustRead(t, wavPath), render.DefaultFormat()); err != nil {
			t.Fatalf("silence wav invalid: %v", err)
		}
	})

	t.Run("auto-creates missing parent dir", func(t *testing.T) {
		t.Parallel()
		p, res := twoBlockPlan(t)
		wavPath := filepath.Join(t.TempDir(), "nested", "deeper", "out.wav")
		if _, err := NewWAVFile(wavPath).Consume(context.Background(), p, res); err != nil {
			t.Fatalf("Consume into missing parent: %v", err)
		}
		if _, err := os.Stat(wavPath); err != nil {
			t.Errorf("wav not written into auto-created parent: %v", err)
		}
	})

	t.Run("empty path returns ErrNoWAVPath", func(t *testing.T) {
		t.Parallel()
		_, res := twoBlockPlan(t)
		_, err := (&WAVFileSink{}).Consume(context.Background(), plan.NarrationPlan{}, res)
		if !errors.Is(err, ErrNoWAVPath) {
			t.Fatalf("want ErrNoWAVPath, got %v", err)
		}
	})
}
