package main

// voice_test.go — #146 Phase 6: the --voice RVC launch flag on cmd/narrate-server
// (Decision D5). Covers parseFlags membership + the D4' gender/voice notice, the
// BuildRenderer routing through both factory seams (escalate + /narrate), and the
// D6 manifest.voice provenance + 40 kHz WithExpectedFormat via a real
// writeRenderDir round-trip (plus the F5 24 kHz-preserved counter-metric).

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vd09-projects/intelligent-tts-narration-library/pipeline"
	"github.com/vd09-projects/intelligent-tts-narration-library/plan"
	"github.com/vd09-projects/intelligent-tts-narration-library/render"
	"github.com/vd09-projects/intelligent-tts-narration-library/render/rvc"
	"github.com/vd09-projects/intelligent-tts-narration-library/render/sherpa"
	"github.com/vd09-projects/intelligent-tts-narration-library/sink/persistent"
)

func TestParseFlags_Voice(t *testing.T) {
	var buf bytes.Buffer

	a, err := parseFlags([]string{"--voice", "cool-jahns"}, &buf)
	if err != nil {
		t.Fatalf("valid --voice rejected: %v", err)
	}
	if a.Voice != "cool-jahns" {
		t.Errorf("parsed Voice = %q, want cool-jahns", a.Voice)
	}

	if _, err := parseFlags([]string{"--voice", "bogus"}, &buf); err == nil {
		t.Error("parseFlags accepted --voice bogus, want startup error")
	}

	def, err := parseFlags(nil, &buf)
	if err != nil {
		t.Fatalf("no flags: %v", err)
	}
	if def.Voice != "" {
		t.Errorf("default Voice = %q, want empty", def.Voice)
	}
}

func TestParseFlags_VoiceGenderNotice(t *testing.T) {
	const notice = "notice: --gender ignored"

	var buf bytes.Buffer
	if _, err := parseFlags([]string{"--voice", "cool-jahns", "--gender", "male"}, &buf); err != nil {
		t.Fatalf("parseFlags: %v", err)
	}
	if !strings.Contains(buf.String(), notice) {
		t.Errorf("explicit --gender + --voice: errOut = %q, want notice", buf.String())
	}

	buf.Reset()
	if _, err := parseFlags([]string{"--voice", "cool-jahns"}, &buf); err != nil {
		t.Fatalf("parseFlags: %v", err)
	}
	if strings.Contains(buf.String(), notice) {
		t.Errorf("--voice with default gender should NOT emit the notice, errOut = %q", buf.String())
	}
}

func TestServerNewPipeline_VoiceRoutesRenderer(t *testing.T) {
	capturer := &capturingSink{}
	rvcPipe := newPipeline(t.TempDir(), serverArgs{Voice: "cool-jahns"}, capturer).(*pipeline.Pipeline)
	if _, ok := rvcPipe.Renderer.(*rvc.Renderer); !ok {
		t.Errorf("escalate renderer with voice = %T, want *rvc.Renderer", rvcPipe.Renderer)
	}
	plainPipe := newPipeline(t.TempDir(), serverArgs{Voice: ""}, capturer).(*pipeline.Pipeline)
	if _, ok := plainPipe.Renderer.(*sherpa.Engine); !ok {
		t.Errorf("escalate renderer without voice = %T, want *sherpa.Engine", plainPipe.Renderer)
	}
}

func TestServerNewNarratePipeline_VoiceRoutesRenderer(t *testing.T) {
	capturer := &capturingSink{}
	rvcPipe := newNarratePipeline("confident-neal", plan.L1, t.TempDir(), capturer).(*pipeline.Pipeline)
	if _, ok := rvcPipe.Renderer.(*rvc.Renderer); !ok {
		t.Errorf("/narrate renderer with voice = %T, want *rvc.Renderer", rvcPipe.Renderer)
	}
	plainPipe := newNarratePipeline("", plan.L1, t.TempDir(), capturer).(*pipeline.Pipeline)
	if _, ok := plainPipe.Renderer.(*sherpa.Engine); !ok {
		t.Errorf("/narrate renderer without voice = %T, want *sherpa.Engine", plainPipe.Renderer)
	}
}

// D6 manifest-provenance lock: writeRenderDir under an RVC voice records the slug
// and validates the container at 40 kHz.
func TestWriteRenderDir_RVCVoice_RecordsSlugAnd40k(t *testing.T) {
	outDir, err := writeOneBlockDir(t, "cool-jahns", 40000)
	if err != nil {
		t.Fatalf("writeRenderDir 40k: %v", err)
	}
	m, err := persistent.ReadManifest(outDir)
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}
	if m.Voice != "cool-jahns" {
		t.Errorf("manifest voice = %q, want cool-jahns (D6)", m.Voice)
	}
	if m.AudioFormat != rvc.OutputFormat() {
		t.Errorf("manifest AudioFormat = %+v, want 40 kHz %+v", m.AudioFormat, rvc.OutputFormat())
	}
}

// F5 counter-metric: with a gender-derived (non-RVC) voice, writeRenderDir keeps
// the 24 kHz default — no WithExpectedFormat — and records the Kokoro id.
func TestWriteRenderDir_KokoroVoice_Preserves24k(t *testing.T) {
	outDir, err := writeOneBlockDir(t, "af_bella", 24000)
	if err != nil {
		t.Fatalf("writeRenderDir 24k: %v", err)
	}
	m, err := persistent.ReadManifest(outDir)
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}
	if m.Voice != "af_bella" {
		t.Errorf("manifest voice = %q, want af_bella", m.Voice)
	}
	if m.AudioFormat != render.DefaultFormat() {
		t.Errorf("manifest AudioFormat = %+v, want 24 kHz default %+v", m.AudioFormat, render.DefaultFormat())
	}
}

// Strict-format lock: a 40 kHz container under a NON-RVC voice gets no
// WithExpectedFormat, so the sink's default 24 kHz validation refuses it —
// proving WithExpectedFormat is applied only for the RVC slug.
func TestWriteRenderDir_NonRVCVoice_NoExpectedFormat(t *testing.T) {
	_, err := writeOneBlockDir(t, "af_bella", 40000)
	if !errors.Is(err, persistent.ErrFormatMismatch) {
		t.Fatalf("want ErrFormatMismatch (no WithExpectedFormat for a non-RVC voice), got %v", err)
	}
}

// writeOneBlockDir renders a one-block persistent dir through the real
// writeRenderDir seam at the given sample rate + voice. Returns the outDir and
// the writeRenderDir error (nil on success). The per-block WAV is synthesized
// in-test (no committed audio), frame-aligned to a whole 100 ms.
func writeOneBlockDir(t *testing.T, voice string, sampleRate int) (string, error) {
	t.Helper()
	outDir := t.TempDir()
	audioDir := t.TempDir()

	bytesPerMs := sampleRate / 1000 * 2 // mono s16le → 2 bytes/frame
	pcm := make([]byte, 100*bytesPerMs)
	for i := range pcm {
		pcm[i] = byte(i % 251)
	}
	if err := os.WriteFile(filepath.Join(audioDir, "b0.wav"), synthWAVBytes(sampleRate, pcm), 0o644); err != nil {
		t.Fatalf("write block wav: %v", err)
	}

	p := plan.NarrationPlan{
		SchemaVersion: plan.SchemaVersion,
		Source:        plan.SourceRef{Kind: plan.SourceKindFile, URI: "/tmp/s.md", ContentHash: "h", Adapter: "file"},
		Defaults:      plan.PlanDefaults{Level: plan.L1, Locale: "en"},
		Blocks: []plan.Block{
			{ID: "b0", Order: 0, Class: plan.ClassProse, Level: plan.L1, Status: plan.StatusVoiced},
		},
	}
	format := render.DefaultFormat()
	if sampleRate == 40000 {
		format = rvc.OutputFormat()
	}
	res := render.RenderResult{
		Audio:    render.AudioStream{Dir: audioDir, Files: []string{"b0.wav"}},
		Timeline: plan.Timeline{Format: format, Blocks: []plan.BlockTiming{{BlockID: "b0", StartMs: 0, EndMs: 100, AudioRef: "b0.wav"}}},
		Format:   format,
	}
	return outDir, writeRenderDir(context.Background(), outDir, voice, p, res)
}

// synthWAVBytes builds a minimal PCM s16le mono RIFF/WAVE container in-test.
func synthWAVBytes(sampleRate int, pcm []byte) []byte {
	const channels, bits = 1, 16
	blockAlign := uint16(channels * bits / 8)
	byteRate := uint32(sampleRate * channels * bits / 8)
	dataSize := uint32(len(pcm))

	buf := make([]byte, 0, 44+len(pcm))
	buf = append(buf, 'R', 'I', 'F', 'F')
	buf = binary.LittleEndian.AppendUint32(buf, 36+dataSize)
	buf = append(buf, 'W', 'A', 'V', 'E', 'f', 'm', 't', ' ')
	buf = binary.LittleEndian.AppendUint32(buf, 16)
	buf = binary.LittleEndian.AppendUint16(buf, 1) // PCM
	buf = binary.LittleEndian.AppendUint16(buf, channels)
	buf = binary.LittleEndian.AppendUint32(buf, uint32(sampleRate))
	buf = binary.LittleEndian.AppendUint32(buf, byteRate)
	buf = binary.LittleEndian.AppendUint16(buf, blockAlign)
	buf = binary.LittleEndian.AppendUint16(buf, bits)
	buf = append(buf, 'd', 'a', 't', 'a')
	buf = binary.LittleEndian.AppendUint32(buf, dataSize)
	buf = append(buf, pcm...)
	return buf
}
