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
	calls  int
	doc    adapter.RawDocument
	err    error
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
//
// RenderBlock is recorded too (blockCalls / gotBlockID / gotBlockOpts) so the
// single-block re-render tests can assert it was called exactly once with the
// right id. blockRes is the canned BlockRender it returns; blockErr is the
// canned error.
type fakeRenderer struct {
	calls   int
	gotPlan plan.NarrationPlan
	gotOpts render.RenderOptions
	res     render.RenderResult
	err     error

	blockCalls   int
	gotBlockID   string
	gotBlockOpts render.RenderOptions
	blockRes     render.BlockRender
	blockErr     error
}

func (f *fakeRenderer) Render(_ context.Context, p plan.NarrationPlan, opts render.RenderOptions) (render.RenderResult, error) {
	f.calls++
	f.gotPlan = p
	f.gotOpts = opts
	return f.res, f.err
}

func (f *fakeRenderer) RenderBlock(_ context.Context, _ plan.NarrationPlan, blockID string, opts render.RenderOptions) (render.BlockRender, error) {
	f.blockCalls++
	f.gotBlockID = blockID
	f.gotBlockOpts = opts
	return f.blockRes, f.blockErr
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

// multiBlockDoc yields a markdown document the planner segments into more
// than one block — a heading followed by a fenced code block. Used by the
// single-block re-render tests so they can target a specific block id like
// "b002".
func multiBlockDoc() adapter.RawDocument {
	body := "# Hello\n\n" + "```go\npackage main\n```\n"
	return adapter.RawDocument{
		Source: plan.SourceRef{
			Kind:        plan.SourceKindFile,
			URI:         "/tmp/multi.md",
			ContentHash: "doc-hash-actual",
			Adapter:     "file@test",
		},
		Bytes: []byte(body),
		OffsetMap: []adapter.OffsetSpan{{
			StartByte: 0, EndByte: len(body),
			Origin: plan.SourceMap{Kind: plan.SourceKindLineRange, StartLine: 1, EndLine: 4},
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
	if got.SinkReceipt != ephemeralReceipt() {
		t.Errorf("receipt: got %+v want %+v", got.SinkReceipt, ephemeralReceipt())
	}
	if len(got.BlockSummaries) == 0 {
		t.Errorf("BlockSummaries empty on whole-doc path; want one entry per plan block")
	}
	if got.BlockHashMismatch != nil {
		t.Errorf("BlockHashMismatch should be nil when ExpectedContentHash unset; got %+v", got.BlockHashMismatch)
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
	if got.SinkReceipt != (sink.SinkReceipt{}) {
		t.Errorf("receipt on error: got %+v, want zero", got.SinkReceipt)
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

// --- single-block re-render path (issue #14) -------------------------------

// TestPipeline_WholeDoc_BlockSummariesPopulated covers (e) + (f) from the
// locked plan: BlockSummaries length matches the plan's block count and each
// entry has populated id/class/level/status/line range.
func TestPipeline_WholeDoc_BlockSummariesPopulated(t *testing.T) {
	t.Parallel()
	ad := &fakeAdapter{doc: multiBlockDoc()}
	rd := &fakeRenderer{res: render.RenderResult{
		Timeline: plan.Timeline{Blocks: []plan.BlockTiming{{BlockID: "b001", EndMs: 100, AudioRef: "b001.wav"}}},
	}}
	sk := &fakeSink{receipt: ephemeralReceipt()}

	p := pipeline.New(ad, nil, rd, sk, pipeline.PipelineDefaults{Level: plan.L1, OutDir: "/tmp/out"})

	got, err := p.Narrate(context.Background(),
		plan.SourceRef{Kind: plan.SourceKindFile, URI: "/tmp/multi.md"},
		pipeline.NarrateRequest{}, // empty BlockID → whole-doc
	)
	if err != nil {
		t.Fatalf("Narrate unexpected error: %v", err)
	}
	if len(got.BlockSummaries) < 2 {
		t.Fatalf("BlockSummaries: want >=2 (heading + code), got %d (%+v)", len(got.BlockSummaries), got.BlockSummaries)
	}
	for i, s := range got.BlockSummaries {
		if s.ID == "" {
			t.Errorf("BlockSummaries[%d].ID empty: %+v", i, s)
		}
		if !s.Level.IsValid() {
			t.Errorf("BlockSummaries[%d].Level not valid: %+v", i, s)
		}
		if !s.Status.IsValid() {
			t.Errorf("BlockSummaries[%d].Status not valid: %+v", i, s)
		}
		if s.StartLine == 0 || s.EndLine == 0 {
			t.Errorf("BlockSummaries[%d].StartLine/EndLine zero (line range should be populated for line-range source maps): %+v", i, s)
		}
	}
	if got.BlockHashMismatch != nil {
		t.Errorf("BlockHashMismatch should be nil on whole-doc path; got %+v", got.BlockHashMismatch)
	}
	if rd.calls != 1 {
		t.Errorf("Render calls: got %d want 1 (whole-doc path)", rd.calls)
	}
	if rd.blockCalls != 0 {
		t.Errorf("RenderBlock must not be called on whole-doc path; got %d", rd.blockCalls)
	}
}

// TestPipeline_SingleBlock_UnknownID covers (a): non-empty BlockID with no
// matching block in the plan returns ErrBlockNotFound, error mentions the id.
func TestPipeline_SingleBlock_UnknownID(t *testing.T) {
	t.Parallel()
	ad := &fakeAdapter{doc: multiBlockDoc()}
	rd := &fakeRenderer{}
	sk := &fakeSink{}

	p := pipeline.New(ad, nil, rd, sk, pipeline.PipelineDefaults{Level: plan.L1, OutDir: "/tmp/out"})

	_, err := p.Narrate(context.Background(),
		plan.SourceRef{Kind: plan.SourceKindFile, URI: "/tmp/multi.md"},
		pipeline.NarrateRequest{BlockID: "b-does-not-exist"},
	)
	if err == nil {
		t.Fatal("Narrate: want ErrBlockNotFound, got nil")
	}
	if !errors.Is(err, pipeline.ErrBlockNotFound) {
		t.Errorf("error wrap: got %v, want wraps ErrBlockNotFound", err)
	}
	if !contains(err.Error(), "b-does-not-exist") {
		t.Errorf("error message %q should mention requested id %q", err.Error(), "b-does-not-exist")
	}
	if rd.calls != 0 {
		t.Errorf("Render must not be called on unknown-id path; got %d", rd.calls)
	}
	if rd.blockCalls != 0 {
		t.Errorf("RenderBlock must not be called on unknown-id path; got %d", rd.blockCalls)
	}
	if sk.calls != 0 {
		t.Errorf("Sink.Consume must not be called on unknown-id path; got %d", sk.calls)
	}
}

// TestPipeline_SingleBlock_ValidID covers (b): valid BlockID + LevelOverrides
// triggers RenderBlock with the right id, and Sink.Consume sees a one-block
// sub-plan whose single block has that id.
func TestPipeline_SingleBlock_ValidID(t *testing.T) {
	t.Parallel()
	ad := &fakeAdapter{doc: multiBlockDoc()}
	rd := &fakeRenderer{
		blockRes: render.BlockRender{
			BlockID: "b002",
			Timing:  plan.BlockTiming{BlockID: "b002", EndMs: 250, AudioRef: "b002.wav"},
		},
	}
	sk := &fakeSink{receipt: sink.SinkReceipt{TotalDurationMs: 250, BlocksPlayed: 1}}

	p := pipeline.New(ad, nil, rd, sk, pipeline.PipelineDefaults{Level: plan.L1, OutDir: "/tmp/out"})

	got, err := p.Narrate(context.Background(),
		plan.SourceRef{Kind: plan.SourceKindFile, URI: "/tmp/multi.md"},
		pipeline.NarrateRequest{
			BlockID:        "b002",
			LevelOverrides: map[string]plan.Level{"b002": plan.L3},
		},
	)
	if err != nil {
		t.Fatalf("Narrate unexpected error: %v", err)
	}
	if rd.calls != 0 {
		t.Errorf("Render (whole-doc) must not be called on single-block path; got %d", rd.calls)
	}
	if rd.blockCalls != 1 {
		t.Errorf("RenderBlock calls: got %d want 1", rd.blockCalls)
	}
	if rd.gotBlockID != "b002" {
		t.Errorf("RenderBlock got id %q want %q", rd.gotBlockID, "b002")
	}
	if rd.gotBlockOpts.OutDir != "/tmp/out" {
		t.Errorf("RenderBlock OutDir: got %q want %q", rd.gotBlockOpts.OutDir, "/tmp/out")
	}
	if sk.calls != 1 {
		t.Errorf("Sink.Consume calls: got %d want 1", sk.calls)
	}
	if len(sk.gotPlan.Blocks) != 1 {
		t.Errorf("Sink saw %d blocks, want exactly 1 (single-block sub-plan)", len(sk.gotPlan.Blocks))
	}
	if len(sk.gotPlan.Blocks) > 0 && sk.gotPlan.Blocks[0].ID != "b002" {
		t.Errorf("Sub-plan block id: got %q want %q", sk.gotPlan.Blocks[0].ID, "b002")
	}
	if len(got.BlockSummaries) < 2 {
		t.Errorf("BlockSummaries should list ALL plan blocks (for caller roster), got %d", len(got.BlockSummaries))
	}
	if got.BlockHashMismatch != nil {
		t.Errorf("BlockHashMismatch should be nil when ExpectedContentHash unset; got %+v", got.BlockHashMismatch)
	}
}

// TestPipeline_SingleBlock_HashMismatch covers (c): ExpectedContentHash that
// disagrees with the planner-produced Source.ContentHash surfaces a non-nil
// BlockHashMismatch on the result envelope, no error, re-render still ran.
func TestPipeline_SingleBlock_HashMismatch(t *testing.T) {
	t.Parallel()
	doc := multiBlockDoc() // Source.ContentHash = "doc-hash-actual"
	ad := &fakeAdapter{doc: doc}
	rd := &fakeRenderer{
		blockRes: render.BlockRender{
			BlockID: "b001",
			Timing:  plan.BlockTiming{BlockID: "b001", EndMs: 100, AudioRef: "b001.wav"},
		},
	}
	sk := &fakeSink{receipt: sink.SinkReceipt{TotalDurationMs: 100, BlocksPlayed: 1}}

	p := pipeline.New(ad, nil, rd, sk, pipeline.PipelineDefaults{Level: plan.L1, OutDir: "/tmp/out"})

	got, err := p.Narrate(context.Background(),
		plan.SourceRef{Kind: plan.SourceKindFile, URI: "/tmp/multi.md"},
		pipeline.NarrateRequest{
			BlockID:             "b001",
			ExpectedContentHash: "deadbeef-stale",
		},
	)
	if err != nil {
		t.Fatalf("Narrate unexpected error on hash mismatch (should be warning, not error): %v", err)
	}
	if got.BlockHashMismatch == nil {
		t.Fatal("BlockHashMismatch should be non-nil on mismatch; got nil")
	}
	if got.BlockHashMismatch.BlockID != "b001" {
		t.Errorf("BlockHashMismatch.BlockID: got %q want %q", got.BlockHashMismatch.BlockID, "b001")
	}
	if got.BlockHashMismatch.Expected != "deadbeef-stale" {
		t.Errorf("BlockHashMismatch.Expected: got %q want %q", got.BlockHashMismatch.Expected, "deadbeef-stale")
	}
	if got.BlockHashMismatch.Got != "doc-hash-actual" {
		t.Errorf("BlockHashMismatch.Got: got %q want %q (actual doc hash)", got.BlockHashMismatch.Got, "doc-hash-actual")
	}
	if rd.blockCalls != 1 {
		t.Errorf("RenderBlock should still be called on hash mismatch (warn, not block): got %d", rd.blockCalls)
	}
	if sk.calls != 1 {
		t.Errorf("Sink.Consume should still be called on hash mismatch: got %d", sk.calls)
	}
}

// TestPipeline_SingleBlock_HashMatch covers (d): ExpectedContentHash equal to
// the planner-produced Source.ContentHash → BlockHashMismatch is nil.
func TestPipeline_SingleBlock_HashMatch(t *testing.T) {
	t.Parallel()
	doc := multiBlockDoc() // Source.ContentHash = "doc-hash-actual"
	ad := &fakeAdapter{doc: doc}
	rd := &fakeRenderer{
		blockRes: render.BlockRender{
			BlockID: "b001",
			Timing:  plan.BlockTiming{BlockID: "b001", EndMs: 100, AudioRef: "b001.wav"},
		},
	}
	sk := &fakeSink{receipt: sink.SinkReceipt{TotalDurationMs: 100, BlocksPlayed: 1}}

	p := pipeline.New(ad, nil, rd, sk, pipeline.PipelineDefaults{Level: plan.L1, OutDir: "/tmp/out"})

	got, err := p.Narrate(context.Background(),
		plan.SourceRef{Kind: plan.SourceKindFile, URI: "/tmp/multi.md"},
		pipeline.NarrateRequest{
			BlockID:             "b001",
			ExpectedContentHash: "doc-hash-actual",
		},
	)
	if err != nil {
		t.Fatalf("Narrate unexpected error: %v", err)
	}
	if got.BlockHashMismatch != nil {
		t.Errorf("BlockHashMismatch should be nil when expected hash matches; got %+v", got.BlockHashMismatch)
	}
}

// contains is a tiny stdlib-free substring check so tests stay self-contained.
func contains(haystack, needle string) bool {
	if needle == "" {
		return true
	}
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}
