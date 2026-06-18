package pipeline_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/vd09-projects/intelligent-tts-narration-library/adapter"
	adapterfile "github.com/vd09-projects/intelligent-tts-narration-library/adapter/file"
	"github.com/vd09-projects/intelligent-tts-narration-library/pipeline"
	"github.com/vd09-projects/intelligent-tts-narration-library/plan"
	"github.com/vd09-projects/intelligent-tts-narration-library/planner"
	"github.com/vd09-projects/intelligent-tts-narration-library/render"
	"github.com/vd09-projects/intelligent-tts-narration-library/sink"
)

// Decision (v6) — convention: accepted. Two benchmarks:
//
//   - BenchmarkNarratePlanner — planner alone, gated at <100 ms / op.
//   - BenchmarkNarrateEndToEnd — pipeline composed with stub renderer +
//     stub sink (zero-cost fakes), so the number reflects planner +
//     wiring cost without the Kokoro subprocess. Keeping them separate
//     prevents Kokoro latency from masking a planner regression.
//
// Both benchmarks read docs/samples/sample.md once outside the timer; the
// hot loop is allocation-free wrt the file read.

func sampleMarkdown(tb testing.TB) adapter.RawDocument {
	tb.Helper()
	root := benchRepoRoot(tb)
	a := adapterfile.New()
	doc, err := a.Read(context.Background(), plan.SourceRef{
		Kind: plan.SourceKindFile,
		URI:  filepath.Join(root, "docs", "samples", "sample.md"),
	})
	if err != nil {
		tb.Fatalf("read sample.md: %v", err)
	}
	return doc
}

func benchRepoRoot(tb testing.TB) string {
	tb.Helper()
	wd, err := os.Getwd()
	if err != nil {
		tb.Fatalf("getwd: %v", err)
	}
	dir := wd
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			tb.Fatalf("could not locate go.mod walking up from %s", wd)
		}
		dir = parent
	}
}

func BenchmarkNarratePlanner(b *testing.B) {
	doc := sampleMarkdown(b)
	req := planner.Request{Level: plan.L1}

	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		if _, err := planner.Plan(context.Background(), doc, req, nil); err != nil {
			b.Fatalf("planner.Plan: %v", err)
		}
	}
}

// noopRenderer + noopSink for the end-to-end bench. They deliberately do
// no real work; the bench reports planner + adapter + wiring cost, not
// subprocess time. The real perf gate is BenchmarkNarratePlanner.

type benchRenderer struct{}

func (benchRenderer) Render(_ context.Context, p plan.NarrationPlan, _ render.RenderOptions) (render.RenderResult, error) {
	timings := make([]plan.BlockTiming, 0, len(p.Blocks))
	for _, blk := range p.Blocks {
		timings = append(timings, plan.BlockTiming{BlockID: blk.ID})
	}
	return render.RenderResult{Timeline: plan.Timeline{Blocks: timings}}, nil
}

func (benchRenderer) RenderBlock(_ context.Context, _ plan.NarrationPlan, _ string, _ render.RenderOptions) (render.BlockRender, error) {
	return render.BlockRender{}, errors.New("benchRenderer.RenderBlock unused")
}

type benchSink struct{}

func (benchSink) Consume(_ context.Context, _ plan.NarrationPlan, res render.RenderResult) (sink.SinkReceipt, error) {
	return sink.SinkReceipt{BlocksPlayed: len(res.Timeline.Blocks)}, nil
}

// benchPipelineAdapter wraps a pre-read RawDocument so the bench's adapter
// hop is constant-time and equal across iterations.
type benchPipelineAdapter struct{ doc adapter.RawDocument }

func (a benchPipelineAdapter) Read(_ context.Context, _ plan.SourceRef) (adapter.RawDocument, error) {
	return a.doc, nil
}

func BenchmarkNarrateEndToEnd(b *testing.B) {
	doc := sampleMarkdown(b)
	p := pipeline.New(
		benchPipelineAdapter{doc: doc},
		nil,
		benchRenderer{},
		benchSink{},
		pipeline.PipelineDefaults{Level: plan.L1, OutDir: b.TempDir(), Locale: "en"},
	)

	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		if _, err := p.Narrate(context.Background(), doc.Source, pipeline.NarrateRequest{}); err != nil {
			b.Fatalf("Narrate: %v", err)
		}
	}
}
