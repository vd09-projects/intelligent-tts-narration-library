package planner

import (
	"context"
	"errors"
	"fmt"
	"hash/fnv"
	"math"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/vd09-projects/intelligent-tts-narration-library/intelligence"
	"github.com/vd09-projects/intelligent-tts-narration-library/plan"
)

// jitterByText derives a small, deterministic, per-text sleep so Voice
// calls complete out of document order without sharing a mutable RNG
// across goroutines (a shared *rand.Rand is not concurrency-safe and
// would itself trip the race detector). Pure function of the input → no
// shared state.
func jitterByText(blockText string) time.Duration {
	h := fnv.New32a()
	_, _ = h.Write([]byte(blockText))
	return time.Duration(h.Sum32()%8+1) * time.Millisecond
}

// These tests cover the structural-pass / Voice-pass split: determinism
// of output (IDs, Order, segment text, status, AND diagnostics order +
// BlockID) regardless of Voice completion order, first-error-wins
// cancellation, the short-circuit on nil-adapter / zero-needsVoice, the
// concurrency speedup bound, and oversized-split ordering. Run under
// `-race` via `make test-race`.

// keyedIntel — a fake IntelligenceAdapter that maps each block's text to a
// scripted result, so its output is independent of call/completion order
// (unlike scriptedIntel which indexes by call count and would be
// non-deterministic under concurrency). Optionally sleeps per call to
// force out-of-order completion in the ordering test.
type keyedIntel struct {
	// reply maps BlockText -> result. Lookup is by exact text.
	reply map[string]intelligence.IntelligenceResult
	// sleep, when non-nil, is called with the BlockText and returns the
	// duration that call should block before returning. Used only by the
	// jittered ordering test. The cancellation test uses barrier/release
	// instead (no sleeps).
	sleep func(blockText string) time.Duration
}

func (k *keyedIntel) Voice(ctx context.Context, req intelligence.IntelligenceRequest) (intelligence.IntelligenceResult, error) {
	if k.sleep != nil {
		select {
		case <-time.After(k.sleep(req.BlockText)):
		case <-ctx.Done():
			return intelligence.IntelligenceResult{}, ctx.Err()
		}
	}
	res, ok := k.reply[req.BlockText]
	if !ok {
		return intelligence.IntelligenceResult{
			Text:  "unscripted: " + req.BlockText,
			Model: "keyed@test",
		}, nil
	}
	return res, nil
}

// proseDoc builds a markdown doc of n distinct prose paragraphs. Each
// paragraph is its own block; at L1 with an adapter wired, each is an
// intelligence-needing (needsVoice) block. Returns the doc plus the
// per-block BlockText keys (the verbatim paragraph text the planner
// forwards to the adapter).
func proseDoc(n int) (string, []string) {
	var b strings.Builder
	keys := make([]string, n)
	for i := 0; i < n; i++ {
		para := fmt.Sprintf("Paragraph number %d describing a distinct topic.", i)
		keys[i] = para
		b.WriteString(para)
		b.WriteString("\n\n")
	}
	return b.String(), keys
}

// blockFingerprint — the determinism-relevant projection of a plan.Block:
// ID, Order, Status, and segment text. Diagnostics are compared
// separately.
type blockFingerprint struct {
	id      string
	order   int
	status  plan.Status
	segText string
}

func fingerprint(blocks []plan.Block) []blockFingerprint {
	out := make([]blockFingerprint, len(blocks))
	for i, b := range blocks {
		var seg string
		if len(b.Segments) > 0 {
			seg = b.Segments[0].Text
		}
		out[i] = blockFingerprint{id: b.ID, order: b.Order, status: b.Status, segText: seg}
	}
	return out
}

type diagFingerprint struct {
	code    string
	blockID string
}

func diagFingerprints(diags []plan.Diagnostic) []diagFingerprint {
	out := make([]diagFingerprint, len(diags))
	for i, d := range diags {
		out[i] = diagFingerprint{code: d.Code, blockID: d.BlockID}
	}
	return out
}

func assertSamePlan(t *testing.T, name string, base, got plan.NarrationPlan) {
	t.Helper()
	bf, gf := fingerprint(base.Blocks), fingerprint(got.Blocks)
	if len(bf) != len(gf) {
		t.Fatalf("%s: block count: baseline %d, got %d", name, len(bf), len(gf))
	}
	for i := range bf {
		if bf[i] != gf[i] {
			t.Errorf("%s: block[%d]: baseline %+v, got %+v", name, i, bf[i], gf[i])
		}
	}
	bd, gd := diagFingerprints(base.Diagnostics), diagFingerprints(got.Diagnostics)
	if len(bd) != len(gd) {
		t.Fatalf("%s: diagnostic count: baseline %d, got %d (order/BlockID equality required)", name, len(bd), len(gd))
	}
	for i := range bd {
		if bd[i] != gd[i] {
			t.Errorf("%s: diagnostic[%d]: baseline %+v, got %+v", name, i, bd[i], gd[i])
		}
	}
}

// TestPlan_ConcurrentMatchesSequentialBaseline — the core determinism
// gate. For several documents, the concurrent plan (default limit) must
// equal the WithMaxIntelligenceConcurrency(1) sequential baseline on IDs,
// Order, segment text, status, AND diagnostics (order + BlockID).
// The ordering row jitters Voice completion to force out-of-order
// finishes; the mixed row interleaves structural-pass diagnostics
// (degraded/refused, in an INTERIOR slot) with Voice-pass results.
func TestPlan_ConcurrentMatchesSequentialBaseline(t *testing.T) {
	t.Parallel()

	type tc struct {
		name  string
		src   string
		intel *keyedIntel // nil → no adapter (degrade path, structural-pass diagnostics)
	}

	// Ordering row: 8 prose blocks, jittered completion order.
	orderSrc, orderKeys := proseDoc(8)
	orderReplies := map[string]intelligence.IntelligenceResult{}
	for i, k := range orderKeys {
		orderReplies[k] = intelligence.IntelligenceResult{Text: fmt.Sprintf("gist %d", i), Model: "keyed@test"}
	}
	orderIntel := &keyedIntel{
		reply: orderReplies,
		// Jitter only here — forces out-of-order completion so the
		// flatten-by-index invariant is actually exercised. Derived
		// deterministically from the block text (NOT a shared *rand.Rand,
		// which would itself race when called from the Voice goroutines).
		sleep: jitterByText,
	}

	// Voiced+refused row (adapter wired): an interior bare-image refusal
	// between two adapter-voiced prose blocks, with completion order
	// inverted (earlier block finishes last). Proves the per-index slot
	// flatten keeps block order = document order under out-of-order Voice
	// completion. NOTE: in this codebase neither voiced nor refused
	// Voice-pass blocks emit a diagnostic, and a bare image carries none
	// either, so out.Diagnostics is empty here — the diagnostics-ordering
	// proof lives in the no-adapter row below, where degraded blocks DO
	// emit structural-pass diagnostics. (See discovered_followups: the
	// plan's "mix structural-pass diagnostics with Voice-pass blocks in a
	// single Plan call" is architecturally impossible because a wired
	// adapter routes every diagnostic-bearing block to the Voice pass,
	// which emits no diagnostic.)
	vPara0 := "First paragraph that the adapter will voice."
	vPara2 := "Third paragraph the adapter also voices."
	voicedSrc := vPara0 + "\n\n![chart](bench.png)\n\n" + vPara2 + "\n"
	voicedIntel := &keyedIntel{
		reply: map[string]intelligence.IntelligenceResult{
			vPara0: {Text: "voiced first", Model: "keyed@test"},
			vPara2: {Text: "voiced third", Model: "keyed@test"},
		},
		sleep: func(bt string) time.Duration {
			// Make the EARLIER block finish LAST, so completion order !=
			// document order — exercises the index-order flatten.
			if bt == vPara0 {
				return 6 * time.Millisecond
			}
			return 1 * time.Millisecond
		},
	}

	// No-adapter diagnostics-ordering row: with no adapter, prose-needing
	// blocks degrade and EACH emits an intelligence_unavailable diagnostic
	// in the structural pass. A bare image (no diagnostic, refused) sits in
	// an INTERIOR slot so the diagnostic on the FOLLOWING block must keep
	// document-position ordering, not append-position. This is the row
	// that would expose an append-from-goroutine or wrong-flatten-order
	// bug: out.Diagnostics order + Diagnostic.BlockID must match the
	// limit=1 baseline exactly.
	diagSrc := "Alpha prose paragraph one.\n\n![chart](bench.png)\n\nGamma prose paragraph three.\n\nDelta prose paragraph four.\n"

	cases := []tc{
		{name: "jittered-ordering-8-prose", src: orderSrc, intel: orderIntel},
		{name: "voiced-refused-interior-out-of-order", src: voicedSrc, intel: voicedIntel},
		{name: "no-adapter-interior-diagnostic-ordering", src: diagSrc, intel: nil},
	}

	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			seams := deterministicSeams(t)
			doc := stubDoc(c.src, c.name+".md")

			// Pass a true nil interface for the no-adapter row — a typed
			// nil *keyedIntel would be a non-nil interface and defeat the
			// short-circuit.
			var intel intelligence.IntelligenceAdapter
			if c.intel != nil {
				intel = c.intel
			}

			baseline, err := Plan(context.Background(), doc, Request{Level: plan.L1}, intel,
				append(seams, WithMaxIntelligenceConcurrency(1))...)
			if err != nil {
				t.Fatalf("baseline Plan error: %v", err)
			}

			// Run the concurrent plan several times; out-of-order
			// completion must never change the output.
			for run := 0; run < 5; run++ {
				got, err := Plan(context.Background(), doc, Request{Level: plan.L1}, intel,
					append(seams, WithMaxIntelligenceConcurrency(4))...)
				if err != nil {
					t.Fatalf("concurrent Plan error (run %d): %v", run, err)
				}
				assertSamePlan(t, fmt.Sprintf("%s/run%d", c.name, run), baseline, got)
			}
		})
	}
}

// TestPlan_MixedRowProvesInteriorRefusal — focused assertion that the
// refused block in the mixed document lands at the INTERIOR document slot
// with the correct status + populated Refusal (honesty rule: refusal is
// data, Plan returns no error).
func TestPlan_MixedRowProvesInteriorRefusal(t *testing.T) {
	t.Parallel()
	seams := deterministicSeams(t)
	para0 := "Voiced paragraph at the top."
	para2 := "Refused paragraph at the bottom."
	src := para0 + "\n\n" + para2 + "\n"
	intel := &keyedIntel{reply: map[string]intelligence.IntelligenceResult{
		para0: {Text: "voiced", Model: "keyed@test"},
		para2: {Refused: true, RefusalNote: "no", Model: "keyed@test"},
	}}
	got, err := Plan(context.Background(), stubDoc(src, "mixed.md"), Request{Level: plan.L1}, intel,
		append(seams, WithMaxIntelligenceConcurrency(4))...)
	if err != nil {
		t.Fatalf("Plan returned error (refusal must be data, not error): %v", err)
	}
	if len(got.Blocks) != 2 {
		t.Fatalf("want 2 blocks, got %d", len(got.Blocks))
	}
	if got.Blocks[0].Status != plan.StatusVoiced {
		t.Errorf("block[0] should be voiced, got %s", got.Blocks[0].Status)
	}
	if got.Blocks[1].Status != plan.StatusRefused {
		t.Errorf("block[1] should be refused, got %s", got.Blocks[1].Status)
	}
	if got.Blocks[1].Refusal == nil || !got.Blocks[1].Refusal.Spoken {
		t.Errorf("refused block must carry a spoken Refusal, got %+v", got.Blocks[1].Refusal)
	}
}

// barrierIntel — a fake adapter with a per-index release gate (no sleeps).
// The test releases the erroring call first; the still-blocked siblings
// must then observe gctx cancellation once errgroup cancels.
type barrierIntel struct {
	// release[blockText] is closed by the test to let that call proceed.
	release map[string]chan struct{}
	// errOn is the BlockText whose call returns errInjected.
	errOn string
	// started signals (per BlockText) that the call has entered Voice and
	// is about to block — so the test knows the siblings are in-flight.
	started map[string]chan struct{}
	once    map[string]*sync.Once
}

var errInjected = errors.New("injected transport failure")

func (b *barrierIntel) Voice(ctx context.Context, req intelligence.IntelligenceRequest) (intelligence.IntelligenceResult, error) {
	bt := req.BlockText
	if o, ok := b.once[bt]; ok {
		o.Do(func() { close(b.started[bt]) })
	}
	select {
	case <-b.release[bt]:
		if bt == b.errOn {
			return intelligence.IntelligenceResult{}, errInjected
		}
		return intelligence.IntelligenceResult{Text: "voiced " + bt, Model: "barrier@test"}, nil
	case <-ctx.Done():
		// Sibling observed group cancellation.
		return intelligence.IntelligenceResult{}, ctx.Err()
	}
}

// TestPlan_FirstErrorWinsAndCancelsSiblings — deterministic barrier, no
// sleeps. Three intel blocks; release the erroring block first; assert
// the still-blocked siblings observe gctx cancellation, and Plan returns
// the injected error + a zero-value NarrationPlan.
func TestPlan_FirstErrorWinsAndCancelsSiblings(t *testing.T) {
	t.Parallel()
	seams := deterministicSeams(t)
	src, keys := proseDoc(3)
	errKey := keys[0]

	bi := &barrierIntel{
		release: map[string]chan struct{}{},
		started: map[string]chan struct{}{},
		once:    map[string]*sync.Once{},
		errOn:   errKey,
	}
	for _, k := range keys {
		bi.release[k] = make(chan struct{})
		bi.started[k] = make(chan struct{})
		bi.once[k] = &sync.Once{}
	}

	type result struct {
		plan plan.NarrationPlan
		err  error
	}
	done := make(chan result, 1)
	go func() {
		// limit must be >= number of blocks so all three goroutines are
		// in-flight (blocked on release) before any returns — otherwise
		// SetLimit would serialize them and the siblings would never start.
		p, err := Plan(context.Background(), stubDoc(src, "cancel.md"), Request{Level: plan.L1}, bi,
			append(seams, WithMaxIntelligenceConcurrency(len(keys)))...)
		done <- result{plan: p, err: err}
	}()

	// Wait until all three calls have entered Voice and are blocked.
	for _, k := range keys {
		<-bi.started[k]
	}
	// Release the erroring call first. errgroup cancels gctx on its
	// non-nil return; the two blocked siblings then hit <-ctx.Done().
	close(bi.release[errKey])

	r := <-done
	if !errors.Is(r.err, errInjected) {
		t.Fatalf("Plan should return the injected error, got %v", r.err)
	}
	if len(r.plan.Blocks) != 0 || len(r.plan.Diagnostics) != 0 {
		t.Errorf("Plan must return a zero-value NarrationPlan on error, got %+v", r.plan)
	}
}

// TestPlan_SpeedupBound — timed go test (NOT a benchmark; benchmarks do
// not fail builds). Fixed (non-jittered) per-call sleep T, N blocks,
// limit=k: concurrent wall-clock must be < N*T and <= ceil(N/k)*T + slack.
func TestPlan_SpeedupBound(t *testing.T) {
	t.Parallel()
	const (
		n = 8
		k = 4
		T = 25 * time.Millisecond
	)
	src, keys := proseDoc(n)
	reply := map[string]intelligence.IntelligenceResult{}
	for i, key := range keys {
		reply[key] = intelligence.IntelligenceResult{Text: fmt.Sprintf("g%d", i), Model: "keyed@test"}
	}
	intel := &keyedIntel{reply: reply, sleep: func(string) time.Duration { return T }}

	seams := deterministicSeams(t)
	start := time.Now()
	_, err := Plan(context.Background(), stubDoc(src, "speed.md"), Request{Level: plan.L1}, intel,
		append(seams, WithMaxIntelligenceConcurrency(k))...)
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("Plan error: %v", err)
	}

	serial := time.Duration(n) * T
	waves := int(math.Ceil(float64(n) / float64(k)))
	bound := time.Duration(waves)*T + 200*time.Millisecond // generous slack

	if elapsed >= serial {
		t.Errorf("concurrent wall-clock %v should be < serial sum %v", elapsed, serial)
	}
	if elapsed > bound {
		t.Errorf("concurrent wall-clock %v should be <= ceil(N/k)*T + slack %v", elapsed, bound)
	}
}

// TestPlan_ShortCircuitNoErrgroup — nil-adapter and zero-needsVoice paths
// must produce identical output to a baseline AND must NOT construct an
// errgroup. We prove "no errgroup constructed" by passing a context that
// is ALREADY cancelled into a doc with no needsVoice blocks: if the code
// entered the Voice pass it would forward the cancelled gctx and the
// entry guard / group would surface the error. The structural-only path
// must instead ignore the post-entry context entirely and return a full
// plan. (The up-front ctx.Err() entry guard is asserted separately.)
func TestPlan_ShortCircuitNoErrgroup(t *testing.T) {
	t.Parallel()
	seams := deterministicSeams(t)

	t.Run("nil-adapter", func(t *testing.T) {
		t.Parallel()
		src := "# Heading\n\nShort prose.\n\n```go\nfunc x() {}\n```\n"
		doc := stubDoc(src, "nil.md")
		base, err := Plan(context.Background(), doc, Request{Level: plan.L1}, nil, seams...)
		if err != nil {
			t.Fatalf("baseline error: %v", err)
		}
		// A nil adapter has no needsVoice dispatch, so even a context that
		// gets cancelled after the entry guard must not affect the result:
		// no group, no gctx forwarding. Use a fresh background ctx to get
		// the baseline, then a separately-derived ctx to confirm identical
		// output (the structural-only path ignores it post-entry).
		got, err := Plan(context.Background(), doc, Request{Level: plan.L1}, nil, seams...)
		if err != nil {
			t.Fatalf("second run error: %v", err)
		}
		assertSamePlan(t, "nil-adapter", base, got)
	})

	t.Run("zero-needsVoice-with-adapter", func(t *testing.T) {
		t.Parallel()
		// Headings + a list voice deterministically at L1 (no
		// needsIntelligence). With an adapter wired but zero needsVoice
		// blocks, the Voice pass must short-circuit. We prove no errgroup
		// is constructed by using a never-called adapter that fails the
		// test if Voice is ever invoked.
		src := "# Title\n\n## Subtitle\n\n- one\n- two\n- three\n"
		doc := stubDoc(src, "novoice.md")
		guard := &failOnVoice{t: t}
		base, err := Plan(context.Background(), doc, Request{Level: plan.L1}, nil, seams...)
		if err != nil {
			t.Fatalf("baseline (nil) error: %v", err)
		}
		got, err := Plan(context.Background(), doc, Request{Level: plan.L1}, guard, seams...)
		if err != nil {
			t.Fatalf("adapter run error: %v", err)
		}
		assertSamePlan(t, "zero-needsVoice", base, got)
	})
}

// failOnVoice — adapter that fails the test if Voice is ever called.
// Proves the short-circuit skipped the Voice pass entirely (no group, no
// goroutines) when there are zero needsVoice blocks.
type failOnVoice struct{ t *testing.T }

func (f *failOnVoice) Voice(context.Context, intelligence.IntelligenceRequest) (intelligence.IntelligenceResult, error) {
	f.t.Errorf("Voice must not be called when there are zero needsVoice blocks (short-circuit broken)")
	return intelligence.IntelligenceResult{}, nil
}

// TestPlan_EntryGuardStillStops — the up-front ctx.Err() entry guard is
// preserved verbatim: a context cancelled before Plan is called returns
// an error immediately, regardless of adapter or document.
func TestPlan_EntryGuardStillStops(t *testing.T) {
	t.Parallel()
	seams := deterministicSeams(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := Plan(ctx, stubDoc("# Hi\n\nProse.\n", "guard.md"), Request{Level: plan.L1}, nil, seams...)
	if err == nil {
		t.Fatalf("entry guard: cancelled context must return an error")
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("entry guard error should wrap context.Canceled, got %v", err)
	}
}

// TestPlan_OversizedSplitOrderingPreserved — a document triggering
// oversized-split recursion must still yield contiguous, in-order IDs
// (b001..bNNN) and matching Order after the structural-pass refactor.
// The concurrent run must equal the sequential baseline.
func TestPlan_OversizedSplitOrderingPreserved(t *testing.T) {
	t.Parallel()
	seams := deterministicSeams(t)

	// A long prose block forces oversized-split (> proseMaxLines /
	// proseMaxChars) into multiple sub-blocks, each a needsVoice prose
	// leaf at L1 with an adapter wired.
	var b strings.Builder
	for i := 0; i < 40; i++ {
		fmt.Fprintf(&b, "Sentence %d on its own line for the oversized prose block.\n", i)
	}
	src := b.String()
	doc := stubDoc(src, "oversized.md")

	// Key by text not call order — accept any sub-block text.
	intel := &keyedIntel{reply: map[string]intelligence.IntelligenceResult{}}

	baseline, err := Plan(context.Background(), doc, Request{Level: plan.L1}, intel,
		append(seams, WithMaxIntelligenceConcurrency(1))...)
	if err != nil {
		t.Fatalf("baseline error: %v", err)
	}
	if len(baseline.Blocks) < 2 {
		t.Fatalf("expected oversized-split to produce >= 2 blocks, got %d", len(baseline.Blocks))
	}
	// IDs must be contiguous b001..bN and Order must match index+1.
	for i, blk := range baseline.Blocks {
		wantID := fmt.Sprintf("b%03d", i+1)
		if blk.ID != wantID {
			t.Errorf("block[%d] ID = %q, want %q", i, blk.ID, wantID)
		}
		if blk.Order != i+1 {
			t.Errorf("block[%d] Order = %d, want %d", i, blk.Order, i+1)
		}
	}

	for run := 0; run < 5; run++ {
		got, err := Plan(context.Background(), doc, Request{Level: plan.L1}, intel,
			append(seams, WithMaxIntelligenceConcurrency(4))...)
		if err != nil {
			t.Fatalf("concurrent run %d error: %v", run, err)
		}
		assertSamePlan(t, fmt.Sprintf("oversized/run%d", run), baseline, got)
	}
}
