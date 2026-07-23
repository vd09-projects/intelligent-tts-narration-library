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

// #156 D-C — startup --gender deprecation notice, asserted against the PINNED
// constants + the narrate-server: prefix (S7 disagreement note):
//   - --gender alone       → NoticeGenderDeprecated
//   - --gender AND --voice  → NoticeGenderIgnored
//   - --voice alone / no flags → silent
func TestParseFlags_GenderDeprecationNotice(t *testing.T) {
	cases := []struct {
		name string
		argv []string
		want string // "" => no notice
	}{
		{"gender alone", []string{"--gender", "male"}, pipeline.NoticeGenderDeprecated},
		{"gender + voice", []string{"--voice", "cool-jahns", "--gender", "male"}, pipeline.NoticeGenderIgnored},
		{"voice alone", []string{"--voice", "cool-jahns"}, ""},
		{"neither", nil, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			if _, err := parseFlags(tc.argv, &buf); err != nil {
				t.Fatalf("parseFlags(%v): %v", tc.argv, err)
			}
			out := buf.String()
			if tc.want == "" {
				if strings.Contains(out, "notice:") {
					t.Errorf("argv %v: want no notice, got %q", tc.argv, out)
				}
				return
			}
			if !strings.Contains(out, "narrate-server: notice: "+tc.want) {
				t.Errorf("argv %v: errOut = %q, want prefix+%q", tc.argv, out, tc.want)
			}
		})
	}
}

// captureNarrateWrite installs /narrate seams: a stub narrator (no Kokoro) + a
// writeRenderDir that records the (voice, format) it would persist. Returns
// pointers a test asserts after driving runNarrate.
func captureNarrateWrite(t *testing.T, gotVoice *string, gotFormat *plan.AudioFormat) {
	t.Helper()
	narrate := func(_ string, _ plan.Level, _ string, capturer *capturingSink) pipeline.Narrator {
		return narratorFunc(func(_ context.Context, _ plan.SourceRef, _ pipeline.NarrateRequest) (pipeline.NarrateResult, error) {
			capturer.plan = plan.NarrationPlan{Blocks: []plan.Block{{ID: "b0"}}}
			capturer.result = render.RenderResult{Timeline: plan.Timeline{Blocks: []plan.BlockTiming{{BlockID: "b0"}}}}
			capturer.captured = true
			return pipeline.NarrateResult{}, nil
		})
	}
	write := func(_ context.Context, dir, voice string, format plan.AudioFormat, _ plan.NarrationPlan, _ render.RenderResult) error {
		*gotVoice = voice
		*gotFormat = format
		return os.WriteFile(filepath.Join(dir, "audio.wav"), []byte("x"), 0o600)
	}
	installNarrateSeams(t, narrate, write)
}

// S4 named test — launch --voice=am-michael stamps manifest.voice=am_michael
// (underscore, not hyphen) at 24 kHz. The explicit launch voice is authoritative:
// it overrides the per-request gender.
func TestNarrate_LaunchVoice_KokoroStampsUnderscore24k(t *testing.T) {
	var gotVoice string
	var gotFormat plan.AudioFormat
	captureNarrateWrite(t, &gotVoice, &gotFormat)

	args := defaultArgs()
	args.Voice = "am-michael"
	// requestVoice = af_bella (request gender female) — must be OVERRIDDEN by the
	// explicit launch --voice.
	_, status, errResp := runNarrate(context.Background(), args, newRenderStore(t.TempDir()), "x", plan.L1, "af_bella")
	if errResp != nil {
		t.Fatalf("runNarrate: status=%d %+v", status, errResp)
	}
	if gotVoice != "am_michael" {
		t.Errorf("manifest voice = %q, want am_michael (underscore engine id, not the hyphen slug)", gotVoice)
	}
	if gotFormat != render.DefaultFormat() {
		t.Errorf("format = %+v, want 24 kHz default", gotFormat)
	}
}

// Launch --voice=cool-jahns (RVC) stamps the slug at 40 kHz.
func TestNarrate_LaunchVoice_RVCStampsSlug40k(t *testing.T) {
	var gotVoice string
	var gotFormat plan.AudioFormat
	captureNarrateWrite(t, &gotVoice, &gotFormat)

	args := defaultArgs()
	args.Voice = "cool-jahns"
	_, status, errResp := runNarrate(context.Background(), args, newRenderStore(t.TempDir()), "x", plan.L1, "af_bella")
	if errResp != nil {
		t.Fatalf("runNarrate: status=%d %+v", status, errResp)
	}
	if gotVoice != "cool-jahns" {
		t.Errorf("manifest voice = %q, want cool-jahns (RVC slug)", gotVoice)
	}
	if gotFormat != rvc.OutputFormat() {
		t.Errorf("format = %+v, want 40 kHz", gotFormat)
	}
}

// Byte-identical back-compat: with NO launch --voice, the per-request gender
// drives manifest.voice + 24 kHz (launch --gender is inert for /narrate).
func TestNarrate_NoLaunchVoice_RequestGenderDrives(t *testing.T) {
	cases := []struct{ requestVoice, want string }{
		{"af_bella", "af_bella"},
		{"am_michael", "am_michael"},
	}
	for _, tc := range cases {
		t.Run(tc.want, func(t *testing.T) {
			var gotVoice string
			var gotFormat plan.AudioFormat
			captureNarrateWrite(t, &gotVoice, &gotFormat)

			args := defaultArgs() // no launch --voice; launch --gender inert here
			_, status, errResp := runNarrate(context.Background(), args, newRenderStore(t.TempDir()), "x", plan.L1, tc.requestVoice)
			if errResp != nil {
				t.Fatalf("runNarrate: status=%d %+v", status, errResp)
			}
			if gotVoice != tc.want {
				t.Errorf("manifest voice = %q, want %q (request gender drives)", gotVoice, tc.want)
			}
			if gotFormat != render.DefaultFormat() {
				t.Errorf("format = %+v, want 24 kHz default", gotFormat)
			}
		})
	}
}

// S4 named test — the --gender launch path byte-equals the --voice launch path
// for the same Kokoro voice (this launch resolution drives the renderer + the
// /escalate render/manifest/format).
func TestServer_LaunchGenderAliasEqualsVoice(t *testing.T) {
	cases := []struct{ gender, voice string }{
		{"female", "af-bella"},
		{"male", "am-michael"},
	}
	for _, tc := range cases {
		alias, err := pipeline.ResolveVoice(serverArgs{Gender: tc.gender}.effectiveLaunchSlug())
		if err != nil {
			t.Fatalf("resolve alias: %v", err)
		}
		explicit, err := pipeline.ResolveVoice(serverArgs{Voice: tc.voice}.effectiveLaunchSlug())
		if err != nil {
			t.Fatalf("resolve explicit: %v", err)
		}
		if alias != explicit {
			t.Errorf("--gender %s (%+v) != --voice %s (%+v)", tc.gender, alias, tc.voice, explicit)
		}
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

// D-D manifest-provenance lock: writeRenderDir under an RVC voice records the slug
// and validates the container at 40 kHz (format threaded per D-G).
func TestWriteRenderDir_RVCVoice_RecordsSlugAnd40k(t *testing.T) {
	outDir, err := writeOneBlockDir(t, "cool-jahns", 40000, rvc.OutputFormat())
	if err != nil {
		t.Fatalf("writeRenderDir 40k: %v", err)
	}
	m, err := persistent.ReadManifest(outDir)
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}
	if m.Voice != "cool-jahns" {
		t.Errorf("manifest voice = %q, want cool-jahns (D-D)", m.Voice)
	}
	if m.AudioFormat != rvc.OutputFormat() {
		t.Errorf("manifest AudioFormat = %+v, want 40 kHz %+v", m.AudioFormat, rvc.OutputFormat())
	}
}

// F5 counter-metric: with a Kokoro (24 kHz) format threaded, writeRenderDir keeps
// the 24 kHz default — no WithExpectedFormat — and records the Kokoro id.
func TestWriteRenderDir_KokoroVoice_Preserves24k(t *testing.T) {
	outDir, err := writeOneBlockDir(t, "af_bella", 24000, render.DefaultFormat())
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

// D-G decoupling lock: the sink's expected format is the THREADED format, NOT
// derived from the voice string. A 40 kHz container with a 24 kHz threaded format
// (no WithExpectedFormat) is refused — proving the manifest voice no longer acts
// as the format oracle (rvc.IsSupportedVoice was dropped).
func TestWriteRenderDir_ThreadedFormat_IsTheOracle(t *testing.T) {
	_, err := writeOneBlockDir(t, "af_bella", 40000, render.DefaultFormat())
	if !errors.Is(err, persistent.ErrFormatMismatch) {
		t.Fatalf("want ErrFormatMismatch (24 kHz threaded format rejects a 40 kHz container), got %v", err)
	}
}

// writeOneBlockDir renders a one-block persistent dir through the real
// writeRenderDir seam: a per-block WAV at wavSampleRate (synthesized in-test,
// frame-aligned to a whole 100 ms) written with expectFormat threaded to
// writeRenderDir. Returns the outDir and the writeRenderDir error (nil on
// success). Decoupling wavSampleRate from expectFormat lets a test exercise the
// D-G format-mismatch path.
func writeOneBlockDir(t *testing.T, voice string, wavSampleRate int, expectFormat plan.AudioFormat) (string, error) {
	t.Helper()
	outDir := t.TempDir()
	audioDir := t.TempDir()

	bytesPerMs := wavSampleRate / 1000 * 2 // mono s16le → 2 bytes/frame
	pcm := make([]byte, 100*bytesPerMs)
	for i := range pcm {
		pcm[i] = byte(i % 251)
	}
	if err := os.WriteFile(filepath.Join(audioDir, "b0.wav"), synthWAVBytes(wavSampleRate, pcm), 0o644); err != nil {
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
	wavFormat := render.DefaultFormat()
	if wavSampleRate == 40000 {
		wavFormat = rvc.OutputFormat()
	}
	res := render.RenderResult{
		Audio:    render.AudioStream{Dir: audioDir, Files: []string{"b0.wav"}},
		Timeline: plan.Timeline{Format: wavFormat, Blocks: []plan.BlockTiming{{BlockID: "b0", StartMs: 0, EndMs: 100, AudioRef: "b0.wav"}}},
		Format:   wavFormat,
	}
	return outDir, writeRenderDir(context.Background(), outDir, voice, expectFormat, p, res)
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
