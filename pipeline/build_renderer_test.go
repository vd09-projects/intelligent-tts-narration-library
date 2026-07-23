package pipeline_test

import (
	"errors"
	"testing"

	"github.com/vd09-projects/intelligent-tts-narration-library/pipeline"
	"github.com/vd09-projects/intelligent-tts-narration-library/plan"
	"github.com/vd09-projects/intelligent-tts-narration-library/render"
	"github.com/vd09-projects/intelligent-tts-narration-library/render/rvc"
	"github.com/vd09-projects/intelligent-tts-narration-library/render/sherpa"
)

// TestBuildRenderer covers the three arms of the shared engine factory (D1):
// empty voice → plain Kokoro at 24 kHz; a valid RVC slug → the rvc decorator at
// 40 kHz; an unknown slug → ErrUnsupportedVoice with a nil renderer (honesty
// rule — never a silent Kokoro fallback).
func TestBuildRenderer(t *testing.T) {
	t.Run("empty voice → plain Kokoro at 24kHz", func(t *testing.T) {
		r, format, err := pipeline.BuildRenderer("")
		if err != nil {
			t.Fatalf("BuildRenderer(\"\") err = %v, want nil", err)
		}
		if _, ok := r.(*sherpa.Engine); !ok {
			t.Errorf("BuildRenderer(\"\") renderer type = %T, want *sherpa.Engine", r)
		}
		if format != render.DefaultFormat() {
			t.Errorf("BuildRenderer(\"\") format = %+v, want %+v", format, render.DefaultFormat())
		}
	})

	t.Run("valid RVC slug → decorator at 40kHz", func(t *testing.T) {
		for _, slug := range rvc.SupportedVoices() {
			r, format, err := pipeline.BuildRenderer(slug)
			if err != nil {
				t.Fatalf("BuildRenderer(%q) err = %v, want nil", slug, err)
			}
			if _, ok := r.(*rvc.Renderer); !ok {
				t.Errorf("BuildRenderer(%q) renderer type = %T, want *rvc.Renderer", slug, r)
			}
			if format != rvc.OutputFormat() {
				t.Errorf("BuildRenderer(%q) format = %+v, want %+v (40 kHz)", slug, format, rvc.OutputFormat())
			}
		}
	})

	t.Run("unknown slug → ErrUnsupportedVoice, nil renderer", func(t *testing.T) {
		r, format, err := pipeline.BuildRenderer("nope-not-a-voice")
		if !errors.Is(err, rvc.ErrUnsupportedVoice) {
			t.Fatalf("BuildRenderer(unknown) err = %v, want rvc.ErrUnsupportedVoice", err)
		}
		if r != nil {
			t.Errorf("BuildRenderer(unknown) renderer = %v, want nil (no silent Kokoro fallback)", r)
		}
		if format != (plan.AudioFormat{}) {
			t.Errorf("BuildRenderer(unknown) format = %+v, want zero", format)
		}
	})
}
