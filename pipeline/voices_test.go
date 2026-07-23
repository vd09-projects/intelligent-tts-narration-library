package pipeline_test

// voices_test.go — #156 Phase 1: the unified named-voice roster + resolver.
// Covers ResolveVoice (per-slug {Engine, RenderVoiceID, ManifestVoice, Format,
// RequiresWorker} + the meaningful-empty RenderVoiceID for RVC + the default /
// unknown arms), the roster<->rvc consistency guard, IsVoice, VoiceSlugs,
// VoiceHelp, and SlugForGender (the centralized --gender alias, byte-identical
// to the retired genderToVoice maps).

import (
	"errors"
	"sort"
	"strings"
	"testing"

	"github.com/vd09-projects/intelligent-tts-narration-library/pipeline"
	"github.com/vd09-projects/intelligent-tts-narration-library/render"
	"github.com/vd09-projects/intelligent-tts-narration-library/render/rvc"
)

func TestResolveVoice(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name           string
		voice          string
		wantSlug       string
		wantEngine     pipeline.VoiceEngine
		wantRenderID   string
		wantManifest   string
		wantRequires   bool
		wantFormat24kb bool // true => render.DefaultFormat() (24 kHz), false => rvc.OutputFormat() (40 kHz)
	}{
		{name: "empty defaults to af-bella", voice: "", wantSlug: "af-bella", wantEngine: pipeline.EngineKokoro, wantRenderID: "af_bella", wantManifest: "af_bella", wantRequires: false, wantFormat24kb: true},
		{name: "af-bella", voice: "af-bella", wantSlug: "af-bella", wantEngine: pipeline.EngineKokoro, wantRenderID: "af_bella", wantManifest: "af_bella", wantRequires: false, wantFormat24kb: true},
		{name: "am-michael", voice: "am-michael", wantSlug: "am-michael", wantEngine: pipeline.EngineKokoro, wantRenderID: "am_michael", wantManifest: "am_michael", wantRequires: false, wantFormat24kb: true},
		{name: "cool-jahns", voice: "cool-jahns", wantSlug: "cool-jahns", wantEngine: pipeline.EngineRVC, wantRenderID: "", wantManifest: "cool-jahns", wantRequires: true, wantFormat24kb: false},
		{name: "confident-neal", voice: "confident-neal", wantSlug: "confident-neal", wantEngine: pipeline.EngineRVC, wantRenderID: "", wantManifest: "confident-neal", wantRequires: true, wantFormat24kb: false},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			info, err := pipeline.ResolveVoice(tc.voice)
			if err != nil {
				t.Fatalf("ResolveVoice(%q) err = %v, want nil", tc.voice, err)
			}
			if info.Slug != tc.wantSlug {
				t.Errorf("Slug = %q, want %q", info.Slug, tc.wantSlug)
			}
			if info.Engine != tc.wantEngine {
				t.Errorf("Engine = %v, want %v", info.Engine, tc.wantEngine)
			}
			if info.RenderVoiceID != tc.wantRenderID {
				t.Errorf("RenderVoiceID = %q, want %q", info.RenderVoiceID, tc.wantRenderID)
			}
			if info.ManifestVoice != tc.wantManifest {
				t.Errorf("ManifestVoice = %q, want %q", info.ManifestVoice, tc.wantManifest)
			}
			if info.RequiresWorker != tc.wantRequires {
				t.Errorf("RequiresWorker = %v, want %v", info.RequiresWorker, tc.wantRequires)
			}
			wantFormat := render.DefaultFormat()
			if !tc.wantFormat24kb {
				wantFormat = rvc.OutputFormat()
			}
			if info.Format != wantFormat {
				t.Errorf("Format = %+v, want %+v", info.Format, wantFormat)
			}
		})
	}
}

// The meaningful-empty contract (S5): an RVC voice's RenderVoiceID is "" because
// the decorator overrides the Kokoro source; a Kokoro voice's is the engine id.
func TestResolveVoice_RVCRenderVoiceIDIsMeaningfulEmpty(t *testing.T) {
	t.Parallel()
	for _, slug := range pipeline.VoiceSlugs() {
		info, err := pipeline.ResolveVoice(slug)
		if err != nil {
			t.Fatalf("ResolveVoice(%q): %v", slug, err)
		}
		if info.Engine == pipeline.EngineRVC && info.RenderVoiceID != "" {
			t.Errorf("%s (RVC) RenderVoiceID = %q, want empty (decorator overrides the source)", slug, info.RenderVoiceID)
		}
		if info.Engine == pipeline.EngineKokoro && info.RenderVoiceID == "" {
			t.Errorf("%s (Kokoro) RenderVoiceID is empty, want an engine id", slug)
		}
	}
}

func TestResolveVoice_Unknown(t *testing.T) {
	t.Parallel()
	info, err := pipeline.ResolveVoice("nope-not-a-voice")
	if !errors.Is(err, pipeline.ErrUnknownVoice) {
		t.Fatalf("err = %v, want pipeline.ErrUnknownVoice", err)
	}
	if info != (pipeline.VoiceInfo{}) {
		t.Errorf("VoiceInfo = %+v, want zero on error", info)
	}
	// The error lists the tagged roster so a caller sees the valid choices.
	if !strings.Contains(err.Error(), "af-bella") || !strings.Contains(err.Error(), "cool-jahns") {
		t.Errorf("ErrUnknownVoice message = %q, want it to list the roster", err.Error())
	}
}

// RVC-roster consistency (D-A drift guard): the set of RVC roster slugs must
// equal rvc.SupportedVoices(), so adding an RVC voice needs exactly one
// rvcVoices entry + one roster entry, and any drift trips this test.
func TestRoster_RVCConsistency(t *testing.T) {
	t.Parallel()
	var rosterRVC []string
	for _, slug := range pipeline.VoiceSlugs() {
		info, err := pipeline.ResolveVoice(slug)
		if err != nil {
			t.Fatalf("ResolveVoice(%q): %v", slug, err)
		}
		if info.Engine == pipeline.EngineRVC {
			rosterRVC = append(rosterRVC, slug)
		}
	}
	rvcSlugs := rvc.SupportedVoices()
	sort.Strings(rosterRVC)
	sort.Strings(rvcSlugs)
	if strings.Join(rosterRVC, ",") != strings.Join(rvcSlugs, ",") {
		t.Errorf("roster RVC slugs %v != rvc.SupportedVoices() %v", rosterRVC, rvcSlugs)
	}
}

func TestIsVoice(t *testing.T) {
	t.Parallel()
	for _, slug := range pipeline.VoiceSlugs() {
		if !pipeline.IsVoice(slug) {
			t.Errorf("IsVoice(%q) = false, want true", slug)
		}
	}
	for _, bad := range []string{"", "bogus", "af_bella", "am_michael"} {
		if pipeline.IsVoice(bad) {
			t.Errorf("IsVoice(%q) = true, want false (engine ids + empty are not roster slugs)", bad)
		}
	}
}

func TestVoiceSlugs_StableOrder(t *testing.T) {
	t.Parallel()
	got := pipeline.VoiceSlugs()
	want := []string{"af-bella", "am-michael", "cool-jahns", "confident-neal"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("VoiceSlugs() = %v, want %v (stable roster order)", got, want)
	}
}

func TestVoiceHelp_TagsEachEntry(t *testing.T) {
	t.Parallel()
	help := pipeline.VoiceHelp()
	for _, want := range []string{
		"af-bella (Kokoro · 24kHz · fast)",
		"am-michael (Kokoro · 24kHz · fast)",
		"cool-jahns (RVC · 40kHz · needs worker)",
		"confident-neal (RVC · 40kHz · needs worker)",
	} {
		if !strings.Contains(help, want) {
			t.Errorf("VoiceHelp() = %q, missing %q", help, want)
		}
	}
}

func TestSlugForGender(t *testing.T) {
	t.Parallel()
	cases := map[string]string{"female": "af-bella", "male": "am-michael"}
	for gender, wantSlug := range cases {
		slug, ok := pipeline.SlugForGender(gender)
		if !ok {
			t.Fatalf("SlugForGender(%q) ok = false, want true", gender)
		}
		if slug != wantSlug {
			t.Errorf("SlugForGender(%q) = %q, want %q", gender, slug, wantSlug)
		}
		// Byte-identical lock: the alias slug resolves to the pre-#156
		// genderToVoice engine id (female->af_bella, male->am_michael).
		info, err := pipeline.ResolveVoice(slug)
		if err != nil {
			t.Fatalf("ResolveVoice(%q): %v", slug, err)
		}
		wantID := map[string]string{"female": "af_bella", "male": "am_michael"}[gender]
		if info.RenderVoiceID != wantID || info.ManifestVoice != wantID {
			t.Errorf("gender %q -> %q -> RenderVoiceID %q / ManifestVoice %q, want both %q",
				gender, slug, info.RenderVoiceID, info.ManifestVoice, wantID)
		}
	}
	if _, ok := pipeline.SlugForGender("robot"); ok {
		t.Error("SlugForGender(\"robot\") ok = true, want false")
	}
}
