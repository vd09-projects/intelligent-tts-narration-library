package main

// voice_test.go — #146 Phase 4: the --voice RVC flag on cmd/narrate. Covers the
// validate() enum + listen mutual-exclusion (D4), the D4' gender/voice stderr
// notice, the D6 manifest.voice slug + 40 kHz WithExpectedFormat on the
// persistent + patch paths, and the F5 negative guard (voice="" preserves the
// 24 kHz plain-Kokoro path).

import (
	"context"
	"io"
	"strings"
	"testing"

	"github.com/vd09-projects/intelligent-tts-narration-library/pipeline"
	"github.com/vd09-projects/intelligent-tts-narration-library/render"
	"github.com/vd09-projects/intelligent-tts-narration-library/render/rvc"
	"github.com/vd09-projects/intelligent-tts-narration-library/render/sherpa"
	"github.com/vd09-projects/intelligent-tts-narration-library/sink/ephemeral"
	"github.com/vd09-projects/intelligent-tts-narration-library/sink/persistent"
)

func TestFlagSet_Validate_Voice(t *testing.T) {
	t.Parallel()
	cases := []struct {
		voice   string
		wantErr bool
	}{
		{"", false},               // empty preserves plain Kokoro
		{"cool-jahns", false},     // known RVC slug
		{"confident-neal", false}, // known RVC slug
		{"bogus", true},           // unknown slug → caller-error
		{"af_bella", true},        // a Kokoro SOURCE voice is not an RVC target
	}
	for _, tc := range cases {
		a := flagSet{File: "/tmp/x.md", Level: 1, Sink: "ephemeral", Gender: "female", Voice: tc.voice}
		err := a.validate()
		if tc.wantErr && err == nil {
			t.Errorf("validate accepted --voice=%q, want error", tc.voice)
		}
		if !tc.wantErr && err != nil {
			t.Errorf("validate rejected --voice=%q: %v", tc.voice, err)
		}
	}
}

func TestFlagSet_Validate_VoiceWithListen_Rejected(t *testing.T) {
	t.Parallel()
	a := flagSet{File: "/tmp/x.md", Level: 1, Sink: "ephemeral", Gender: "female", Voice: "cool-jahns", Listen: true}
	if err := a.validate(); err == nil {
		t.Error("validate accepted --voice with --listen (D4: must be rejected pending dynamic oto rate)")
	}
}

// D6 + WithExpectedFormat: a persistent sink built under --voice records the RVC
// slug in manifest.voice (via WithVoice) and validates the container at 40 kHz.
func TestChooseSink_Persistent_Voice(t *testing.T) {
	t.Parallel()
	s := chooseSink(flagSet{Sink: "persistent", Out: "/tmp/out", Gender: "female", Voice: "cool-jahns"})
	ps, ok := s.(*persistent.Sink)
	if !ok {
		t.Fatalf("chooseSink returned %T, want *persistent.Sink", s)
	}
	if ps.Voice != "cool-jahns" {
		t.Errorf("persistent Voice = %q, want %q (D6: RVC slug, not gender-derived)", ps.Voice, "cool-jahns")
	}
	if ps.ExpectedFormat != rvc.OutputFormat() {
		t.Errorf("persistent ExpectedFormat = %+v, want %+v (40 kHz)", ps.ExpectedFormat, rvc.OutputFormat())
	}
}

// F5 negative guard: with no --voice, the persistent sink keeps the gender-
// derived Kokoro voice AND the default 24 kHz expected format (no
// WithExpectedFormat applied) — byte-identical to pre-#146 behavior.
func TestChooseSink_Persistent_NoVoice_PreservesDefault(t *testing.T) {
	t.Parallel()
	s := chooseSink(flagSet{Sink: "persistent", Out: "/tmp/out", Gender: "male", Voice: ""})
	ps, ok := s.(*persistent.Sink)
	if !ok {
		t.Fatalf("chooseSink returned %T, want *persistent.Sink", s)
	}
	if ps.Voice != genderToVoice["male"] {
		t.Errorf("persistent Voice = %q, want gender-derived %q", ps.Voice, genderToVoice["male"])
	}
	if ps.ExpectedFormat != render.DefaultFormat() {
		t.Errorf("persistent ExpectedFormat = %+v, want 24 kHz default %+v (no WithExpectedFormat)", ps.ExpectedFormat, render.DefaultFormat())
	}
}

// Proves BuildRenderer routing through the factory seam: a valid --voice wires
// the rvc decorator; empty --voice wires plain Kokoro (F5). sherpa.New/rvc.New
// spawn no subprocess at construction, so this is hermetic.
func TestNewPipelineWithSink_VoiceRoutesRenderer(t *testing.T) {
	t.Parallel()

	rvcPipe := newPipelineWithSink(t.TempDir(), flagSet{Level: 1, Voice: "confident-neal"}, ephemeral.New())
	if p, ok := rvcPipe.(*pipeline.Pipeline); !ok {
		t.Fatalf("newPipelineWithSink returned %T, want *pipeline.Pipeline", rvcPipe)
	} else if _, ok := p.Renderer.(*rvc.Renderer); !ok {
		t.Errorf("with --voice, renderer = %T, want *rvc.Renderer", p.Renderer)
	}

	plainPipe := newPipelineWithSink(t.TempDir(), flagSet{Level: 1, Voice: ""}, ephemeral.New())
	if p, ok := plainPipe.(*pipeline.Pipeline); !ok {
		t.Fatalf("newPipelineWithSink returned %T, want *pipeline.Pipeline", plainPipe)
	} else if _, ok := p.Renderer.(*sherpa.Engine); !ok {
		t.Errorf("without --voice, renderer = %T, want *sherpa.Engine (no silent RVC)", p.Renderer)
	}
}

func TestRoot_VoiceFlag_Parsed(t *testing.T) {
	t.Parallel()
	var got flagSet
	deps, _, _, _ := stubDeps(func(_ context.Context, a flagSet, _, _ io.Writer) error {
		got = a
		return nil
	})
	cmd := newRootCmd(*deps)
	cmd.SetArgs([]string{"--file=/tmp/x.md", "--voice=cool-jahns"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if got.Voice != "cool-jahns" {
		t.Errorf("parsed --voice = %q, want cool-jahns", got.Voice)
	}
}

// D4' — an explicit --gender paired with a valid --voice emits a non-fatal
// stderr notice; --voice alone (default gender) or --gender alone stays silent.
func TestRoot_GenderVoiceNotice(t *testing.T) {
	t.Parallel()
	const notice = "notice: --gender ignored when --voice is set"

	run := func(args ...string) string {
		deps, _, stderr, _ := stubDeps(func(_ context.Context, _ flagSet, _, _ io.Writer) error { return nil })
		cmd := newRootCmd(*deps)
		cmd.SetArgs(append([]string{"--file=/tmp/x.md"}, args...))
		if err := cmd.Execute(); err != nil {
			t.Fatalf("Execute(%v): %v", args, err)
		}
		return stderr.String()
	}

	if out := run("--gender=male", "--voice=cool-jahns"); !strings.Contains(out, notice) {
		t.Errorf("explicit --gender + --voice: stderr = %q, want notice", out)
	}
	if out := run("--voice=cool-jahns"); strings.Contains(out, notice) {
		t.Errorf("--voice with default gender should NOT emit the notice, stderr = %q", out)
	}
	if out := run("--gender=male"); strings.Contains(out, notice) {
		t.Errorf("--gender alone should NOT emit the notice, stderr = %q", out)
	}
}
