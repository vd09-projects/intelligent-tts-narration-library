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
		{"", false},               // empty falls to the --gender alias / default
		{"af-bella", false},       // Kokoro roster slug (#156)
		{"am-michael", false},     // Kokoro roster slug (#156)
		{"cool-jahns", false},     // known RVC slug
		{"confident-neal", false}, // known RVC slug
		{"cool-jahns-gso", false}, // GSO peer roster slug (#162/#163) — accepted, resolves
		{"bogus", true},           // unknown slug → caller-error (ErrUnknownVoice, no silent fallback)
		{"af_bella", true},        // an underscore ENGINE id is not a roster slug
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

// #156 Phase 2b — listen re-keys off RequiresWorker: a 40 kHz RVC --voice is
// rejected with --listen; a 24 kHz Kokoro --voice is now accepted.
func TestFlagSet_Validate_VoiceWithListen(t *testing.T) {
	t.Parallel()
	cases := []struct {
		voice   string
		wantErr bool
	}{
		{"cool-jahns", true},     // RVC 40 kHz → rejected pending #154
		{"confident-neal", true}, // RVC 40 kHz → rejected
		{"cool-jahns-gso", true}, // GSO 32 kHz, requires worker → rejected (S2 generalized message)
		{"af-bella", false},      // Kokoro 24 kHz → allowed
		{"am-michael", false},    // Kokoro 24 kHz → allowed
		{"", false},              // no voice → allowed (gender alias)
	}
	for _, tc := range cases {
		a := flagSet{File: "/tmp/x.md", Level: 1, Sink: "ephemeral", Gender: "female", Voice: tc.voice, Listen: true}
		err := a.validate()
		if tc.wantErr && err == nil {
			t.Errorf("validate accepted --voice=%q with --listen, want error", tc.voice)
		}
		if !tc.wantErr && err != nil {
			t.Errorf("validate rejected --voice=%q with --listen: %v", tc.voice, err)
		}
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
// WithExpectedFormat applied) — byte-identical to pre-#156 behavior. The
// gender-derived id is resolved through the roster (SlugForGender → ResolveVoice).
func TestChooseSink_Persistent_NoVoice_PreservesDefault(t *testing.T) {
	t.Parallel()
	s := chooseSink(flagSet{Sink: "persistent", Out: "/tmp/out", Gender: "male", Voice: ""})
	ps, ok := s.(*persistent.Sink)
	if !ok {
		t.Fatalf("chooseSink returned %T, want *persistent.Sink", s)
	}
	slug, _ := pipeline.SlugForGender("male")
	want, _ := pipeline.ResolveVoice(slug)
	if ps.Voice != want.ManifestVoice {
		t.Errorf("persistent Voice = %q, want gender-derived %q", ps.Voice, want.ManifestVoice)
	}
	if want.ManifestVoice != "am_michael" {
		t.Errorf("male → %q, want am_michael (byte-identical engine id)", want.ManifestVoice)
	}
	if ps.ExpectedFormat != render.DefaultFormat() {
		t.Errorf("persistent ExpectedFormat = %+v, want 24 kHz default %+v (no WithExpectedFormat)", ps.ExpectedFormat, render.DefaultFormat())
	}
}

// #156 — a Kokoro --voice (am-michael) records the underscore engine id in
// manifest.voice and keeps the 24 kHz default (no WithExpectedFormat): the
// format re-key is off the roster, NOT "--voice != ”".
func TestChooseSink_Persistent_KokoroVoice_24k(t *testing.T) {
	t.Parallel()
	s := chooseSink(flagSet{Sink: "persistent", Out: "/tmp/out", Gender: "female", Voice: "am-michael"})
	ps, ok := s.(*persistent.Sink)
	if !ok {
		t.Fatalf("chooseSink returned %T, want *persistent.Sink", s)
	}
	if ps.Voice != "am_michael" {
		t.Errorf("persistent Voice = %q, want am_michael (Kokoro engine id, not the hyphen slug)", ps.Voice)
	}
	if ps.ExpectedFormat != render.DefaultFormat() {
		t.Errorf("Kokoro --voice ExpectedFormat = %+v, want 24 kHz default %+v (no WithExpectedFormat)", ps.ExpectedFormat, render.DefaultFormat())
	}
}

// S8 — alias-path == explicit-path byte-equality, per Kokoro voice: the
// --gender alias and the explicit --voice select the SAME manifest voice + format.
func TestChooseSink_AliasEqualsExplicit_PerKokoroVoice(t *testing.T) {
	t.Parallel()
	cases := []struct{ gender, voice string }{
		{"female", "af-bella"},
		{"male", "am-michael"},
	}
	for _, tc := range cases {
		alias := chooseSink(flagSet{Sink: "persistent", Out: "/tmp/out", Gender: tc.gender, Voice: ""}).(*persistent.Sink)
		explicit := chooseSink(flagSet{Sink: "persistent", Out: "/tmp/out", Gender: "female", Voice: tc.voice}).(*persistent.Sink)
		if alias.Voice != explicit.Voice {
			t.Errorf("--gender %s Voice=%q != --voice %s Voice=%q", tc.gender, alias.Voice, tc.voice, explicit.Voice)
		}
		if alias.ExpectedFormat != explicit.ExpectedFormat {
			t.Errorf("--gender %s format != --voice %s format", tc.gender, tc.voice)
		}
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

// #156 D-C — --gender deprecation notices, gated on cobra.Changed("gender") and
// asserted against the PINNED constants (S7):
//   - --gender alone       → NoticeGenderDeprecated
//   - --gender AND --voice  → NoticeGenderIgnored
//   - --voice alone / no flags → silent
func TestRoot_GenderDeprecationNotice(t *testing.T) {
	t.Parallel()
	run := func(args ...string) string {
		deps, _, stderr, _ := stubDeps(func(_ context.Context, _ flagSet, _, _ io.Writer) error { return nil })
		cmd := newRootCmd(*deps)
		cmd.SetArgs(append([]string{"--file=/tmp/x.md"}, args...))
		if err := cmd.Execute(); err != nil {
			t.Fatalf("Execute(%v): %v", args, err)
		}
		return stderr.String()
	}

	if out := run("--gender=male"); !strings.Contains(out, pipeline.NoticeGenderDeprecated) {
		t.Errorf("--gender alone: stderr = %q, want NoticeGenderDeprecated", out)
	}
	if out := run("--gender=male", "--voice=cool-jahns"); !strings.Contains(out, pipeline.NoticeGenderIgnored) {
		t.Errorf("--gender + --voice: stderr = %q, want NoticeGenderIgnored", out)
	}
	if out := run("--voice=cool-jahns"); strings.Contains(out, pipeline.NoticeGenderDeprecated) || strings.Contains(out, pipeline.NoticeGenderIgnored) {
		t.Errorf("--voice alone should be silent, stderr = %q", out)
	}
	if out := run(); strings.Contains(out, pipeline.NoticeGenderDeprecated) || strings.Contains(out, pipeline.NoticeGenderIgnored) {
		t.Errorf("no voice flags should be silent, stderr = %q", out)
	}
}
