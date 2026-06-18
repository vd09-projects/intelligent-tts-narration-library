package pipeline_test

import (
	"context"
	"errors"
	"testing"

	"github.com/vd09-projects/intelligent-tts-narration-library/adapter"
	"github.com/vd09-projects/intelligent-tts-narration-library/pipeline"
	"github.com/vd09-projects/intelligent-tts-narration-library/plan"
	"github.com/vd09-projects/intelligent-tts-narration-library/render"
	"github.com/vd09-projects/intelligent-tts-narration-library/sink"
)

// fakeAdapter is a stub InputAdapter that records calls and returns a
// pre-baked RawDocument (or err).
type fakeAdapter struct {
	calls int
	doc   adapter.RawDocument
	err   error
	gotCtx context.Context //nolint:containedctx // test recorder
	gotRef plan.SourceRef
}

func (f *fakeAdapter) Read(ctx context.Context, ref plan.SourceRef) (adapter.RawDocument, error) {
	f.calls++
	f.gotCtx = ctx
	f.gotRef = ref
	return f.doc, f.err
}

// fakeRenderer captures the plan it was handed and returns a canned result.
type fakeRenderer struct {
	calls    int
	gotPlan  plan.NarrationPlan
	gotOpts  render.RenderOptions
	res      render.RenderResult
	err      error
}

func (f *fakeRenderer) Render(_ context.Context, p plan.NarrationPlan, opts render.RenderOptions) (render.RenderResult, error) {
	f.calls++
	f.gotPlan = p
	f.gotOpts = opts
	return f.res, f.err
}

func (f *fakeRenderer) RenderBlock(_ context.Context, _ plan.NarrationPlan, _ string, _ render.RenderOptions) (render.BlockRender, error) {
	return render.BlockRender{}, errors.New("fakeRenderer.RenderBlock not used in pipeline tests")
}

// fakeSink captures the plan + render result and returns a canned receipt.
type fakeSink struct {
	calls   int
	gotPlan plan.NarrationPlan
	gotRes  render.RenderResult
	receipt sink.SinkReceipt
	err     error
}

func (f *fakeSink) Consume(_ context.Context, p plan.NarrationPlan, res render.RenderResult) (sink.SinkReceipt, error) {
	f.calls++
	f.gotPlan = p
	f.gotRes = res
	return f.receipt, f.err
}

// helpers -------------------------------------------------------------------

func helloDoc() adapter.RawDocument {
	return adapter.RawDocument{
		Source: plan.SourceRef{
			Kind:        plan.SourceKindFile,
			URI:         "/tmp/hello.md",
			ContentHash: "fake",
			Adapter:     "file@test",
		},
		Bytes: []byte("Hello world.\n"),
		OffsetMap: []adapter.OffsetSpan{{
			StartByte: 0, EndByte: 13,
			Origin: plan.SourceMap{Kind: plan.SourceKindLineRange, StartLine: 1, EndLine: 1},
		}},
	}
}

func ephemeralReceipt() sink.SinkReceipt {
	return sink.SinkReceipt{TotalDurationMs: 1234, BlocksPlayed: 1}
}

// tests ---------------------------------------------------------------------

func TestPipeline_HappyPath_NilIntelligence(t *testing.T) {
	t.Parallel()
	ad := &fakeAdapter{doc: helloDoc()}
	rd := &fakeRenderer{res: render.RenderResult{
		Timeline: plan.Timeline{Blocks: []plan.BlockTiming{{BlockID: "b001", EndMs: 1000, AudioRef: "b001.wav"}}},
	}}
	sk := &fakeSink{receipt: ephemeralReceipt()}

	p := pipeline.New(ad, nil, rd, sk, pipeline.PipelineDefaults{
		Level: plan.L1, OutDir: "/tmp/out", Locale: "en",
	})

	got, err := p.Narrate(context.Background(),
		plan.SourceRef{Kind: plan.SourceKindFile, URI: "/tmp/hello.md"},
		pipeline.NarrateRequest{Voice: "af_bella"},
	)
	if err != nil {
		t.Fatalf("Narrate unexpected error: %v", err)
	}
	if got != ephemeralReceipt() {
		t.Errorf("receipt: got %+v want %+v", got, ephemeralReceipt())
	}
	if ad.calls != 1 {
		t.Errorf("adapter calls: got %d want 1", ad.calls)
	}
	if rd.calls != 1 {
		t.Errorf("renderer calls: got %d want 1", rd.calls)
	}
	if sk.calls != 1 {
		t.Errorf("sink calls: got %d want 1", sk.calls)
	}
	if rd.gotOpts.OutDir != "/tmp/out" {
		t.Errorf("renderer OutDir: got %q want %q", rd.gotOpts.OutDir, "/tmp/out")
	}
	if rd.gotOpts.Voice != "af_bella" {
		t.Errorf("renderer Voice: got %q want %q", rd.gotOpts.Voice, "af_bella")
	}
	if rd.gotPlan.PlanID == "" {
		t.Errorf("renderer received empty PlanID")
	}
}

func TestPipeline_AdapterError_StopsPipeline(t *testing.T) {
	t.Parallel()
	boom := errors.New("boom: stat failed")
	ad := &fakeAdapter{err: boom}
	rd := &fakeRenderer{}
	sk := &fakeSink{}

	p := pipeline.New(ad, nil, rd, sk, pipeline.PipelineDefaults{Level: plan.L1, OutDir: "/tmp"})

	got, err := p.Narrate(context.Background(),
		plan.SourceRef{Kind: plan.SourceKindFile, URI: "/tmp/missing.md"},
		pipeline.NarrateRequest{},
	)
	if err == nil {
		t.Fatal("Narrate returned nil error on adapter failure")
	}
	if !errors.Is(err, boom) {
		t.Errorf("error wrap: got %v, want wraps %v", err, boom)
	}
	if got != (sink.SinkReceipt{}) {
		t.Errorf("receipt on error: got %+v, want zero", got)
	}
	if rd.calls != 0 {
		t.Errorf("renderer must not be called after adapter error; got %d calls", rd.calls)
	}
	if sk.calls != 0 {
		t.Errorf("sink must not be called after adapter error; got %d calls", sk.calls)
	}
}

func TestPipeline_RefusalIsNotError(t *testing.T) {
	t.Parallel()
	// The planner already turns bare images into Status=refused blocks; the
	// pipeline must forward that plan through the renderer (which speaks the
	// refusal message) and the sink (which plays it) WITHOUT returning a Go
	// error. Receipt comes back populated.
	ad := &fakeAdapter{doc: adapter.RawDocument{
		Source: plan.SourceRef{Kind: plan.SourceKindFile, URI: "/tmp/refused.md", ContentHash: "fake", Adapter: "file@test"},
		Bytes:  []byte("![chart](missing.png)\n"),
		OffsetMap: []adapter.OffsetSpan{{
			StartByte: 0, EndByte: 22,
			Origin: plan.SourceMap{Kind: plan.SourceKindLineRange, StartLine: 1, EndLine: 1},
		}},
	}}
	// Renderer returns a non-error result even when the plan contains a
	// refused block. Refusal-as-data: receipt comes through populated.
	rd := &fakeRenderer{res: render.RenderResult{
		Timeline: plan.Timeline{Blocks: []plan.BlockTiming{{BlockID: "b001", EndMs: 500, AudioRef: "b001.wav"}}},
	}}
	sk := &fakeSink{receipt: sink.SinkReceipt{TotalDurationMs: 500, BlocksPlayed: 1}}

	p := pipeline.New(ad, nil, rd, sk, pipeline.PipelineDefaults{Level: plan.L1, OutDir: "/tmp"})

	got, err := p.Narrate(context.Background(),
		plan.SourceRef{Kind: plan.SourceKindFile, URI: "/tmp/refused.md"},
		pipeline.NarrateRequest{},
	)
	if err != nil {
		t.Fatalf("Narrate returned error on refusal-as-data: %v", err)
	}
	if got.BlocksPlayed == 0 {
		t.Errorf("receipt: BlocksPlayed should be > 0 even when blocks are refused; got %+v", got)
	}
	// Sanity: the plan handed to the sink should contain at least one refused
	// block — that's the whole point of refusal-as-data.
	foundRefused := false
	for _, b := range sk.gotPlan.Blocks {
		if b.Status == plan.StatusRefused {
			foundRefused = true
			break
		}
	}
	if !foundRefused {
		t.Errorf("expected at least one Status=refused block in sink-handed plan; got blocks=%+v", sk.gotPlan.Blocks)
	}
}

func TestPipeline_RendererError_PropagatesAsError(t *testing.T) {
	t.Parallel()
	ad := &fakeAdapter{doc: helloDoc()}
	rd := &fakeRenderer{err: errors.New("sherpa: timeout")}
	sk := &fakeSink{}

	p := pipeline.New(ad, nil, rd, sk, pipeline.PipelineDefaults{Level: plan.L1, OutDir: "/tmp"})

	_, err := p.Narrate(context.Background(),
		plan.SourceRef{Kind: plan.SourceKindFile, URI: "/tmp/x.md"},
		pipeline.NarrateRequest{},
	)
	if err == nil {
		t.Fatal("Narrate returned nil error on renderer failure")
	}
	if sk.calls != 0 {
		t.Errorf("sink must not run when renderer errored; got %d calls", sk.calls)
	}
}

func TestPipeline_SinkError_ReturnsPartialReceipt(t *testing.T) {
	t.Parallel()
	ad := &fakeAdapter{doc: helloDoc()}
	rd := &fakeRenderer{res: render.RenderResult{
		Timeline: plan.Timeline{Blocks: []plan.BlockTiming{{BlockID: "b001", EndMs: 500, AudioRef: "b001.wav"}}},
	}}
	sk := &fakeSink{
		receipt: sink.SinkReceipt{TotalDurationMs: 250, BlocksPlayed: 0},
		err:     errors.New("ephemeral: afplay died"),
	}

	p := pipeline.New(ad, nil, rd, sk, pipeline.PipelineDefaults{Level: plan.L1, OutDir: "/tmp"})

	got, err := p.Narrate(context.Background(),
		plan.SourceRef{Kind: plan.SourceKindFile, URI: "/tmp/x.md"},
		pipeline.NarrateRequest{},
	)
	if err == nil {
		t.Fatal("Narrate returned nil error on sink failure")
	}
	if got.TotalDurationMs != 250 {
		t.Errorf("partial receipt: got %+v want TotalDurationMs=250", got)
	}
}

func TestPipeline_New_PanicsOnNilEdges(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		a    adapter.InputAdapter
		r    render.Renderer
		s    sink.OutputSink
	}{
		{"nil adapter", nil, &fakeRenderer{}, &fakeSink{}},
		{"nil renderer", &fakeAdapter{}, nil, &fakeSink{}},
		{"nil sink", &fakeAdapter{}, &fakeRenderer{}, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			defer func() {
				if r := recover(); r == nil {
					t.Errorf("expected panic, got none")
				}
			}()
			_ = pipeline.New(tc.a, nil, tc.r, tc.s, pipeline.PipelineDefaults{Level: plan.L1, OutDir: "/tmp"})
		})
	}
}

func TestPipeline_New_FillsLocaleAndLevelDefaults(t *testing.T) {
	t.Parallel()
	p := pipeline.New(&fakeAdapter{}, nil, &fakeRenderer{}, &fakeSink{}, pipeline.PipelineDefaults{OutDir: "/tmp"})
	if p.Defaults.Locale != "en" {
		t.Errorf("Locale default: got %q want %q", p.Defaults.Locale, "en")
	}
	if p.Defaults.Level != plan.L1 {
		t.Errorf("Level default: got %v want %v", p.Defaults.Level, plan.L1)
	}
}
