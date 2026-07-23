package main

// voice_test.go — #146 Phase 5: the RVC `voice` arg on the speak / speak_last /
// speak_to_file MCP tools. Covers validate() (caller-error on an unknown voice),
// the BuildRenderer routing through both factory seams, the 40 kHz
// WithExpectedFormat on the WAVFile path, the voice threading through the
// speak_last + speak_to_file arg conversions, and the F5 counter-metric (an
// omitted voice preserves the plain-Kokoro path byte-for-byte).

import (
	"context"
	"strings"
	"testing"

	"github.com/vd09-projects/intelligent-tts-narration-library/adapter"
	"github.com/vd09-projects/intelligent-tts-narration-library/adapter/file"
	"github.com/vd09-projects/intelligent-tts-narration-library/intelligence"
	"github.com/vd09-projects/intelligent-tts-narration-library/pipeline"
	"github.com/vd09-projects/intelligent-tts-narration-library/render"
	"github.com/vd09-projects/intelligent-tts-narration-library/render/rvc"
	"github.com/vd09-projects/intelligent-tts-narration-library/render/sherpa"
	"github.com/vd09-projects/intelligent-tts-narration-library/sink/ephemeral"
	"github.com/vd09-projects/intelligent-tts-narration-library/sink/persistent"
)

func TestSpeakArgs_Validate_Voice(t *testing.T) {
	t.Parallel()
	cases := []struct {
		voice   string
		wantErr bool
	}{
		{"", false},
		{"cool-jahns", false},
		{"confident-neal", false},
		{"bogus", true},
		{"af_bella", true}, // Kokoro source is not an RVC target slug
	}
	for _, tc := range cases {
		a := speakArgs{Source: "doc.md", Level: 1, Sink: "ephemeral", Gender: "female", Intelligence: "none", Voice: tc.voice}
		err := a.validate()
		if tc.wantErr {
			if err == nil {
				t.Errorf("validate accepted voice=%q, want error", tc.voice)
			} else if !strings.Contains(err.Error(), "caller-error: invalid_argument:") {
				t.Errorf("voice=%q error %q must carry the MCP caller-error prefix", tc.voice, err)
			}
		} else if err != nil {
			t.Errorf("validate rejected voice=%q: %v", tc.voice, err)
		}
	}
}

// Real newPipeline seam: a valid voice wires the rvc decorator; empty wires
// plain Kokoro (F5). sherpa.New/rvc.New spawn no subprocess at construction.
func TestNewPipeline_VoiceRoutesRenderer(t *testing.T) {
	t.Parallel()

	rvcPipe := newPipeline(t.TempDir(), speakArgs{Level: 1, Voice: "cool-jahns"}, file.New(), nil, nil)
	if p, ok := rvcPipe.(*pipeline.Pipeline); !ok {
		t.Fatalf("newPipeline returned %T, want *pipeline.Pipeline", rvcPipe)
	} else if _, ok := p.Renderer.(*rvc.Renderer); !ok {
		t.Errorf("with voice, renderer = %T, want *rvc.Renderer", p.Renderer)
	}

	plainPipe := newPipeline(t.TempDir(), speakArgs{Level: 1, Voice: ""}, file.New(), nil, nil)
	if p, ok := plainPipe.(*pipeline.Pipeline); !ok {
		t.Fatalf("newPipeline returned %T, want *pipeline.Pipeline", plainPipe)
	} else if _, ok := p.Renderer.(*sherpa.Engine); !ok {
		t.Errorf("without voice, renderer = %T, want *sherpa.Engine (no silent RVC)", p.Renderer)
	}
}

// Real newFilePipeline seam: a voice wires the rvc decorator AND a 40 kHz
// WAVFile ExpectedFormat; empty wires plain Kokoro AND the 24 kHz default (no
// WithExpectedFormat) — F5.
func TestNewFilePipeline_VoiceRoutesRendererAndFormat(t *testing.T) {
	t.Parallel()

	rvcPipe := newFilePipeline(t.TempDir(), "/tmp/out.wav", speakArgs{Level: 1, Voice: "confident-neal"}, file.New(), nil)
	p, ok := rvcPipe.(*pipeline.Pipeline)
	if !ok {
		t.Fatalf("newFilePipeline returned %T, want *pipeline.Pipeline", rvcPipe)
	}
	if _, ok := p.Renderer.(*rvc.Renderer); !ok {
		t.Errorf("with voice, renderer = %T, want *rvc.Renderer", p.Renderer)
	}
	if ws, ok := p.Sink.(*persistent.WAVFileSink); !ok {
		t.Fatalf("sink = %T, want *persistent.WAVFileSink", p.Sink)
	} else if ws.ExpectedFormat != rvc.OutputFormat() {
		t.Errorf("WAVFile ExpectedFormat = %+v, want 40 kHz %+v", ws.ExpectedFormat, rvc.OutputFormat())
	}

	plainPipe := newFilePipeline(t.TempDir(), "/tmp/out.wav", speakArgs{Level: 1, Voice: ""}, file.New(), nil)
	pp := plainPipe.(*pipeline.Pipeline)
	if _, ok := pp.Renderer.(*sherpa.Engine); !ok {
		t.Errorf("without voice, renderer = %T, want *sherpa.Engine", pp.Renderer)
	}
	if ws, ok := pp.Sink.(*persistent.WAVFileSink); !ok {
		t.Fatalf("sink = %T, want *persistent.WAVFileSink", pp.Sink)
	} else if ws.ExpectedFormat != render.DefaultFormat() {
		t.Errorf("WAVFile ExpectedFormat = %+v, want 24 kHz default %+v (no WithExpectedFormat)", ws.ExpectedFormat, render.DefaultFormat())
	}
}

func TestToSpeakArgs_ThreadsVoice(t *testing.T) {
	t.Parallel()
	got := speakToFileArgs{Voice: "cool-jahns"}.toSpeakArgs()
	if got.Voice != "cool-jahns" {
		t.Errorf("toSpeakArgs Voice = %q, want cool-jahns", got.Voice)
	}
}

// captureVoiceAtSeam swaps newPipeline for a stub that records the speakArgs.Voice
// reaching the factory (the exact call site that invokes BuildRenderer) and
// returns a benign narrator. Returns a pointer to the captured voice + a restore.
func captureVoiceAtSeam(gotVoice *string) func() {
	orig := newPipeline
	newPipeline = func(_ string, args speakArgs, _ adapter.InputAdapter, _ intelligence.IntelligenceAdapter, _ ephemeral.BlockObserver) pipeline.Narrator {
		*gotVoice = args.Voice
		return &stubNarrator{}
	}
	return func() { newPipeline = orig }
}

// speak: the tool arg reaches the newPipeline seam (→ BuildRenderer).
func TestRunSpeak_ThreadsVoiceToPipeline(t *testing.T) {
	var gotVoice string
	restore := captureVoiceAtSeam(&gotVoice)
	defer restore()

	if _, err := runSpeakWithCache(context.Background(),
		speakArgs{Text: "hello world", Level: 1, Sink: "ephemeral", Gender: "female", Intelligence: "none", Voice: "cool-jahns"},
		nil); err != nil {
		t.Fatalf("runSpeakWithCache: %v", err)
	}
	if gotVoice != "cool-jahns" {
		t.Errorf("voice reaching newPipeline = %q, want cool-jahns", gotVoice)
	}
}

// speak_last: the resolved text is narrated and the voice threads from
// speakLastArgs through to the newPipeline seam.
func TestRunSpeakLast_ThreadsVoiceToPipeline(t *testing.T) {
	dir := t.TempDir()
	path := writeLines(t, dir, "t.jsonl",
		[]string{`{"type":"assistant","message":{"content":[{"type":"text","text":"a prior assistant reply"}]}}`})

	var gotVoice string
	restore := captureVoiceAtSeam(&gotVoice)
	defer restore()

	if _, err := runSpeakLast(context.Background(),
		speakLastArgs{Level: 1, Gender: "female", Intelligence: "none", Voice: "confident-neal", TranscriptPath: path},
		nil); err != nil {
		t.Fatalf("runSpeakLast: %v", err)
	}
	if gotVoice != "confident-neal" {
		t.Errorf("voice reaching newPipeline via speak_last = %q, want confident-neal", gotVoice)
	}
}
