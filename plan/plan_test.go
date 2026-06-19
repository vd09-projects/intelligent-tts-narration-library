package plan

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"
)

// ----------------------------------------------------------------------------
// Golden round-trip — design doc §2.7 verbatim fixtures.
//
// Decision (2) — tradeoff: testdata fixtures are committed verbatim from the
// design doc §2.7. Schema-doc drift is caught by these tests, not by separate
// validation. Status: accepted.
// ----------------------------------------------------------------------------

func TestRoundTrip_VoicedConfigBlock(t *testing.T) {
	t.Parallel()
	got := roundTripBlock(t, "example_voiced_config.json")

	// Spot-check load-bearing fields beyond DeepEqual: a regression on
	// these specific values is what the design doc §2.7 example exists to
	// pin down.
	if got.ID != "b004" || got.Class != ClassConfig || got.Status != StatusVoiced {
		t.Fatalf("voiced block fields drifted: %+v", got)
	}
	if len(got.Segments) != 1 || !strings.Contains(got.Segments[0].Text, "Replicas set to three") {
		t.Fatalf("voiced segment text drifted: %+v", got.Segments)
	}
}

func TestRoundTrip_RefusedImageBlock(t *testing.T) {
	t.Parallel()
	got := roundTripBlock(t, "example_refused_image.json")

	if got.Status != StatusRefused {
		t.Fatalf("refused block status drifted: got %q want %q", got.Status, StatusRefused)
	}
	if got.Refusal == nil {
		t.Fatal("refused block missing Refusal payload")
	}
	if got.Refusal.Reason != RefuseBareImage {
		t.Fatalf("refusal reason drifted: got %q want %q", got.Refusal.Reason, RefuseBareImage)
	}
	if !got.Refusal.Spoken {
		t.Fatal("refusal.spoken expected true by default (honesty rule)")
	}
}

func TestRoundTrip_FullPlan(t *testing.T) {
	t.Parallel()
	raw := readFixture(t, "example_full_plan.json")

	var p NarrationPlan
	if err := json.Unmarshal(raw, &p); err != nil {
		t.Fatalf("decode example_full_plan.json: %v", err)
	}
	first, err := json.Marshal(p)
	if err != nil {
		t.Fatalf("re-encode plan: %v", err)
	}
	var p2 NarrationPlan
	if err := json.Unmarshal(first, &p2); err != nil {
		t.Fatalf("decode re-encoded plan: %v", err)
	}
	if !reflect.DeepEqual(p, p2) {
		t.Fatalf("full-plan round-trip mismatch:\nfirst=%#v\nsecond=%#v", p, p2)
	}
	if p.SchemaVersion != SchemaVersion {
		t.Fatalf("fixture schema_version drift: got %q want %q", p.SchemaVersion, SchemaVersion)
	}
	if len(p.Blocks) != 2 {
		t.Fatalf("full plan expected 2 blocks, got %d", len(p.Blocks))
	}
}

// roundTripBlock decodes a single-Block fixture, re-encodes, re-decodes, and
// asserts DeepEqual on the two decoded values. Returns the first decoded
// Block for further field-level assertions by the caller.
func roundTripBlock(t *testing.T, fixture string) Block {
	t.Helper()
	raw := readFixture(t, fixture)

	var b Block
	if err := json.Unmarshal(raw, &b); err != nil {
		t.Fatalf("decode %s: %v", fixture, err)
	}
	first, err := json.Marshal(b)
	if err != nil {
		t.Fatalf("re-encode %s: %v", fixture, err)
	}
	var b2 Block
	if err := json.Unmarshal(first, &b2); err != nil {
		t.Fatalf("decode re-encoded %s: %v", fixture, err)
	}
	if !reflect.DeepEqual(b, b2) {
		t.Fatalf("%s round-trip mismatch:\nfirst=%#v\nsecond=%#v", fixture, b, b2)
	}
	return b
}

func readFixture(t *testing.T, name string) []byte {
	t.Helper()
	path := filepath.Join("testdata", name)
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read fixture %s: %v", path, err)
	}
	return raw
}

// ----------------------------------------------------------------------------
// ULID generation.
// ----------------------------------------------------------------------------

func TestNewPlanID_Properties(t *testing.T) {
	t.Parallel()
	const n = 100
	seen := make(map[string]struct{}, n)
	validChar := regexp.MustCompile(`^[0-9A-HJKMNP-TV-Z]{26}$`)

	for i := range n {
		id := NewPlanID()
		if len(id) != 26 {
			t.Fatalf("ULID %q length = %d, want 26", id, len(id))
		}
		if !validChar.MatchString(id) {
			t.Fatalf("ULID %q has invalid character class (must be Crockford base32)", id)
		}
		if _, dup := seen[id]; dup {
			t.Fatalf("ULID collision at iteration %d: %q already seen", i, id)
		}
		seen[id] = struct{}{}
	}
}

// ----------------------------------------------------------------------------
// Enum IsValid — table-driven.
// ----------------------------------------------------------------------------

func TestLevel_IsValid(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		l    Level
		want bool
	}{
		{"L1", L1, true},
		{"L2", L2, true},
		{"L3", L3, true},
		{"zero", 0, false},
		{"too_high", 4, false},
		{"negative", -1, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := tt.l.IsValid(); got != tt.want {
				t.Errorf("Level(%d).IsValid() = %v, want %v", tt.l, got, tt.want)
			}
		})
	}
}

func TestClass_IsValid(t *testing.T) {
	t.Parallel()
	valid := []Class{
		ClassProse, ClassCode, ClassConfig, ClassTable,
		ClassDiagramAsText, ClassExample, ClassHeading, ClassList, ClassUnknown,
	}
	for _, c := range valid {
		t.Run(string(c), func(t *testing.T) {
			t.Parallel()
			if !c.IsValid() {
				t.Errorf("Class %q expected valid", c)
			}
		})
	}
	for _, c := range []Class{"", "bogus", "Prose", "CODE"} {
		t.Run("invalid_"+string(c), func(t *testing.T) {
			t.Parallel()
			if c.IsValid() {
				t.Errorf("Class %q expected invalid", c)
			}
		})
	}
}

func TestStatus_IsValid(t *testing.T) {
	t.Parallel()
	for _, s := range []Status{StatusVoiced, StatusDegraded, StatusRefused} {
		if !s.IsValid() {
			t.Errorf("Status %q expected valid", s)
		}
	}
	for _, s := range []Status{"", "passed", "VOICED"} {
		if s.IsValid() {
			t.Errorf("Status %q expected invalid", s)
		}
	}
}

func TestRefusalReason_IsValid(t *testing.T) {
	t.Parallel()
	for _, r := range []RefusalReason{
		RefuseBareImage, RefuseTooLarge, RefuseTooRaw,
		RefuseNoIntelligence, RefuseUnsupported,
	} {
		if !r.IsValid() {
			t.Errorf("RefusalReason %q expected valid", r)
		}
	}
	for _, r := range []RefusalReason{"", "bare_image", "BARE_IMAGE_NO_DESCRIPTION"} {
		if r.IsValid() {
			t.Errorf("RefusalReason %q expected invalid", r)
		}
	}
}

func TestSourceKind_IsValid(t *testing.T) {
	t.Parallel()
	for _, k := range []SourceKind{
		SourceKindFile, SourceKindMCPText, SourceKindOCRScreenshot,
		SourceKindRawText, SourceKindLineRange, SourceKindCharSpan,
		SourceKindPixelRegion,
	} {
		if !k.IsValid() {
			t.Errorf("SourceKind %q expected valid", k)
		}
	}
	for _, k := range []SourceKind{"", "filesystem", "FILE"} {
		if k.IsValid() {
			t.Errorf("SourceKind %q expected invalid", k)
		}
	}
}

func TestSegmentKind_IsValid(t *testing.T) {
	t.Parallel()
	for _, k := range []SegmentKind{SegmentKindSpeech, SegmentKindPause, SegmentKindEarcon} {
		if !k.IsValid() {
			t.Errorf("SegmentKind %q expected valid", k)
		}
	}
	for _, k := range []SegmentKind{"", "silence", "SPEECH"} {
		if k.IsValid() {
			t.Errorf("SegmentKind %q expected invalid", k)
		}
	}
}

// ----------------------------------------------------------------------------
// SchemaVersion / IsCompatible — table-driven.
// ----------------------------------------------------------------------------

func TestSchemaVersion_Constant(t *testing.T) {
	t.Parallel()
	if SchemaVersion != "1.0" {
		t.Fatalf("SchemaVersion drift: got %q want %q", SchemaVersion, "1.0")
	}
}

func TestIsCompatible(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		other string
		want  bool
	}{
		{"same_exact", "1.0", true},
		{"same_major_higher_minor", "1.5", true},
		{"same_major_zero_minor", "1.0", true},
		{"same_major_many_minor_digits", "1.99", true},
		{"different_major_zero", "0.9", false},
		{"different_major_two", "2.0", false},
		{"empty", "", false},
		{"no_dot", "1", false},
		{"trailing_dot", "1.", false},
		{"leading_dot", ".1", false},
		{"non_numeric_major", "v1.0", false},
		{"non_numeric_minor", "1.x", false},
		{"whitespace", " 1.0", false},
		{"leading_zero_major", "01.0", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := IsCompatible(tt.other); got != tt.want {
				t.Errorf("IsCompatible(%q) = %v, want %v", tt.other, got, tt.want)
			}
		})
	}
}

// ----------------------------------------------------------------------------
// Invariant guards — reflection scans.
//
// These tests fail loudly if a future commit smuggles timing into Block, word-
// level fields into BlockTiming, or non-snake-case json tags anywhere in the
// public schema. They encode CLAUDE.md invariants as runnable checks.
// ----------------------------------------------------------------------------

// forbiddenBlockSubstrings — fragments that would imply audio/sync data on
// the engine-neutral Block type. Block carries spoken text + provenance only;
// audio data lives in Timeline (timeline.go).
var forbiddenBlockSubstrings = []string{
	"audio", "ms", "offset", "duration", "sample", "rate",
}

func TestInvariant_BlockHasNoAudioFields(t *testing.T) {
	t.Parallel()
	rt := reflect.TypeOf(Block{})
	for i := 0; i < rt.NumField(); i++ {
		name := strings.ToLower(rt.Field(i).Name)
		for _, frag := range forbiddenBlockSubstrings {
			if strings.Contains(name, frag) {
				t.Errorf("Block.%s contains forbidden substring %q — audio/timing belongs in Timeline, not Block",
					rt.Field(i).Name, frag)
			}
		}
	}
}

// forbiddenBlockTimingSubstrings — fragments that would imply sub-block
// granularity. Sync is block-level only; word/segment/char/rune timings
// contradict gist mode (spoken text ≠ source text).
var forbiddenBlockTimingSubstrings = []string{
	"word", "segment", "char", "rune",
}

func TestInvariant_BlockTimingHasNoSubBlockFields(t *testing.T) {
	t.Parallel()
	rt := reflect.TypeOf(BlockTiming{})
	for i := 0; i < rt.NumField(); i++ {
		name := strings.ToLower(rt.Field(i).Name)
		for _, frag := range forbiddenBlockTimingSubstrings {
			if strings.Contains(name, frag) {
				t.Errorf("BlockTiming.%s contains forbidden substring %q — block-level sync only",
					rt.Field(i).Name, frag)
			}
		}
	}
}

// snakeCaseTagRe — allowed character class inside a json: tag.
// Matches "snake_case_name", "snake_case_name,omitempty", or "-".
// (commas separate the field name from options like "omitempty"; both are
// lowercase ASCII.)
var snakeCaseTagRe = regexp.MustCompile(`^[a-z0-9_,]+$`)

// allPublicTypes — every exported struct type in this package whose json
// tags participate in the wire schema. Add new types here when introduced.
func allPublicTypes() []reflect.Type {
	return []reflect.Type{
		reflect.TypeOf(NarrationPlan{}),
		reflect.TypeOf(SourceRef{}),
		reflect.TypeOf(PlanDefaults{}),
		reflect.TypeOf(Block{}),
		reflect.TypeOf(Provenance{}),
		reflect.TypeOf(Segment{}),
		reflect.TypeOf(VoicingDirective{}),
		reflect.TypeOf(SourceMap{}),
		reflect.TypeOf(Rect{}),
		reflect.TypeOf(Refusal{}),
		reflect.TypeOf(Diagnostic{}),
		reflect.TypeOf(Timeline{}),
		reflect.TypeOf(BlockTiming{}),
		reflect.TypeOf(AudioFormat{}),
	}
}

func TestInvariant_AllJSONTagsAreSnakeCase(t *testing.T) {
	t.Parallel()
	for _, rt := range allPublicTypes() {
		t.Run(rt.Name(), func(t *testing.T) {
			t.Parallel()
			for i := 0; i < rt.NumField(); i++ {
				f := rt.Field(i)
				tag := f.Tag.Get("json")
				if tag == "" || tag == "-" {
					continue
				}
				if !snakeCaseTagRe.MatchString(tag) {
					t.Errorf("%s.%s json tag %q has non-snake-case characters", rt.Name(), f.Name, tag)
				}
			}
		})
	}
}

// ----------------------------------------------------------------------------
// Refusal invariant — Block.Refusal present iff Block.Status == StatusRefused.
//
// The schema doc (plan.go Block.Refusal comment) says: "present iff Block.Status
// == StatusRefused". Round-trip fixtures happen to obey it, but a future
// construction path could violate it silently — these tests catch that.
// ----------------------------------------------------------------------------

// checkRefusalInvariant reports an error if Block.Refusal presence does not
// match Block.Status == StatusRefused. Walks SubBlocks recursively so a
// violation in a nested block is also caught. Returns nil when the invariant
// holds for b and every descendant.
func checkRefusalInvariant(b Block) error {
	hasRefusal := b.Refusal != nil
	wantRefusal := b.Status == StatusRefused
	if hasRefusal != wantRefusal {
		return fmt.Errorf("block %q: status=%q refusal_present=%t, want refusal_present=%t",
			b.ID, b.Status, hasRefusal, wantRefusal)
	}
	for i, sb := range b.SubBlocks {
		if err := checkRefusalInvariant(sb); err != nil {
			return fmt.Errorf("block %q sub_block[%d]: %w", b.ID, i, err)
		}
	}
	return nil
}

func TestInvariant_RefusalPresenceMatchesStatus(t *testing.T) {
	t.Parallel()

	// Single-Block fixtures: decode as Block, check directly.
	singleBlockFixtures := []string{
		"example_voiced_config.json",
		"example_refused_image.json",
	}
	for _, fixture := range singleBlockFixtures {
		t.Run(fixture, func(t *testing.T) {
			t.Parallel()
			raw := readFixture(t, fixture)
			var b Block
			if err := json.Unmarshal(raw, &b); err != nil {
				t.Fatalf("decode %s: %v", fixture, err)
			}
			if err := checkRefusalInvariant(b); err != nil {
				t.Errorf("fixture %s violates refusal invariant: %v", fixture, err)
			}
		})
	}

	// Full-plan fixture: decode as NarrationPlan, check every block.
	t.Run("example_full_plan.json", func(t *testing.T) {
		t.Parallel()
		raw := readFixture(t, "example_full_plan.json")
		var p NarrationPlan
		if err := json.Unmarshal(raw, &p); err != nil {
			t.Fatalf("decode example_full_plan.json: %v", err)
		}
		for _, b := range p.Blocks {
			if err := checkRefusalInvariant(b); err != nil {
				t.Errorf("fixture example_full_plan.json violates refusal invariant: %v", err)
			}
		}
	})
}

func TestRefusalInvariant_SyntheticCases(t *testing.T) {
	t.Parallel()

	// Minimal refusal stub — only Status/Refusal participate in the invariant,
	// so other fields stay zero.
	stubRefusal := &Refusal{
		Reason:  RefuseBareImage,
		Message: "stub",
		Spoken:  true,
	}

	tests := []struct {
		name    string
		block   Block
		wantErr bool
	}{
		{
			name:    "refused_with_refusal",
			block:   Block{ID: "b1", Status: StatusRefused, Refusal: stubRefusal},
			wantErr: false,
		},
		{
			name:    "refused_without_refusal",
			block:   Block{ID: "b2", Status: StatusRefused, Refusal: nil},
			wantErr: true,
		},
		{
			name:    "voiced_with_refusal",
			block:   Block{ID: "b3", Status: StatusVoiced, Refusal: stubRefusal},
			wantErr: true,
		},
		{
			name:    "voiced_without_refusal",
			block:   Block{ID: "b4", Status: StatusVoiced, Refusal: nil},
			wantErr: false,
		},
		{
			name:    "degraded_without_refusal",
			block:   Block{ID: "b5", Status: StatusDegraded, Refusal: nil},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := checkRefusalInvariant(tt.block)
			if (err != nil) != tt.wantErr {
				t.Errorf("checkRefusalInvariant(%+v) err = %v, wantErr %v",
					tt.block, err, tt.wantErr)
			}
		})
	}
}

func TestRefusalInvariant_RecursesIntoSubBlocks(t *testing.T) {
	t.Parallel()

	stubRefusal := &Refusal{Reason: RefuseBareImage, Message: "stub", Spoken: true}

	// Parent is valid (voiced + no refusal). Sub-block violates (voiced with
	// refusal). The checker must surface the sub-block error.
	parent := Block{
		ID:     "parent",
		Status: StatusVoiced,
		SubBlocks: []Block{
			{ID: "child", Status: StatusVoiced, Refusal: stubRefusal},
		},
	}
	err := checkRefusalInvariant(parent)
	if err == nil {
		t.Fatal("expected error from sub-block violation, got nil")
	}
	if !strings.Contains(err.Error(), "sub_block") {
		t.Errorf("error should mention sub_block path, got: %v", err)
	}
	if !strings.Contains(err.Error(), `"child"`) {
		t.Errorf("error should mention offending sub-block id 'child', got: %v", err)
	}
}

// ----------------------------------------------------------------------------
// Forward compatibility — unknown enum values round-trip without crashing.
//
// CLAUDE.md says the schema is "additive-compatible within a major
// schema_version: consumers ignore unknown fields." Unknown enum *values*
// (e.g. v1.5 adds StatusPartiallyVoiced) are the more common forward-compat
// case. A v1.0 reader must (a) decode without error, (b) report IsValid()
// false, (c) re-encode preserving the unknown value verbatim so the document
// survives a read-write cycle.
// ----------------------------------------------------------------------------

// fwdCompatProbe — covers one enum type at one struct site. raw is a minimal
// JSON document; decode parses raw into a fresh target; isValid runs IsValid()
// against the decoded field; expectLiteral is the unknown string that must
// survive in re-encoded JSON.
type fwdCompatProbe struct {
	name          string
	raw           string
	decode        func(raw []byte) (any, bool, error) // returns (target, isValid, err)
	expectLiteral string
}

func TestForwardCompat_UnknownEnumValuesRoundTrip(t *testing.T) {
	t.Parallel()

	probes := []fwdCompatProbe{
		{
			name:          "block_status",
			raw:           `{"id":"b1","status":"future_status_added_in_v1_5"}`,
			expectLiteral: "future_status_added_in_v1_5",
			decode: func(raw []byte) (any, bool, error) {
				var b Block
				err := json.Unmarshal(raw, &b)
				return b, b.Status.IsValid(), err
			},
		},
		{
			name:          "block_class",
			raw:           `{"id":"b1","class":"future_class_v1_5"}`,
			expectLiteral: "future_class_v1_5",
			decode: func(raw []byte) (any, bool, error) {
				var b Block
				err := json.Unmarshal(raw, &b)
				return b, b.Class.IsValid(), err
			},
		},
		{
			name:          "sourcemap_kind",
			raw:           `{"kind":"future_source_kind_v1_5"}`,
			expectLiteral: "future_source_kind_v1_5",
			decode: func(raw []byte) (any, bool, error) {
				var sm SourceMap
				err := json.Unmarshal(raw, &sm)
				return sm, sm.Kind.IsValid(), err
			},
		},
		{
			name:          "sourceref_kind",
			raw:           `{"kind":"future_source_kind_v1_5","content_hash":"x","adapter":"y"}`,
			expectLiteral: "future_source_kind_v1_5",
			decode: func(raw []byte) (any, bool, error) {
				var sr SourceRef
				err := json.Unmarshal(raw, &sr)
				return sr, sr.Kind.IsValid(), err
			},
		},
		{
			name:          "segment_kind",
			raw:           `{"id":"s1","kind":"future_segment_kind_v1_5"}`,
			expectLiteral: "future_segment_kind_v1_5",
			decode: func(raw []byte) (any, bool, error) {
				var s Segment
				err := json.Unmarshal(raw, &s)
				return s, s.Kind.IsValid(), err
			},
		},
		{
			name:          "refusal_reason",
			raw:           `{"reason":"future_refusal_reason_v1_5","message":"x","spoken":true}`,
			expectLiteral: "future_refusal_reason_v1_5",
			decode: func(raw []byte) (any, bool, error) {
				var r Refusal
				err := json.Unmarshal(raw, &r)
				return r, r.Reason.IsValid(), err
			},
		},
	}

	for _, p := range probes {
		t.Run(p.name, func(t *testing.T) {
			t.Parallel()

			target, isValid, err := p.decode([]byte(p.raw))
			if err != nil {
				t.Fatalf("decode unknown value should not error, got: %v", err)
			}
			if isValid {
				t.Errorf("IsValid() expected false for unknown value, got true")
			}

			out, err := json.Marshal(target)
			if err != nil {
				t.Fatalf("re-encode: %v", err)
			}
			if !strings.Contains(string(out), p.expectLiteral) {
				t.Errorf("re-encoded JSON should preserve unknown value %q verbatim, got: %s",
					p.expectLiteral, string(out))
			}
		})
	}
}

// ----------------------------------------------------------------------------
// Omitempty smoke — zero-value plan + zero-value block emit minimal JSON.
//
// Catches a future regression where someone drops `,omitempty` on
// NarrationPlan.Diagnostics, PlanDefaults.Voice, Block.SubBlocks, etc. The
// schema's additive-compatibility guarantee depends on optional fields not
// appearing in encoded output unless populated.
// ----------------------------------------------------------------------------

func TestOmitempty_ZeroValuePlanProducesMinimalJSON(t *testing.T) {
	t.Parallel()

	// Zero-value NarrationPlan: diagnostics slice empty, defaults.voice
	// unset — neither should appear in encoded JSON.
	planJSON, err := json.Marshal(NarrationPlan{})
	if err != nil {
		t.Fatalf("marshal NarrationPlan{}: %v", err)
	}
	planStr := string(planJSON)
	forbiddenPlanKeys := []string{`"diagnostics"`, `"voice"`}
	for _, key := range forbiddenPlanKeys {
		if strings.Contains(planStr, key) {
			t.Errorf("zero-value NarrationPlan JSON should not contain %s, got: %s",
				key, planStr)
		}
	}

	// Zero-value Block: segments / sub_blocks / refusal all omitempty —
	// none should appear in encoded JSON.
	blockJSON, err := json.Marshal(Block{})
	if err != nil {
		t.Fatalf("marshal Block{}: %v", err)
	}
	blockStr := string(blockJSON)
	forbiddenBlockKeys := []string{`"segments"`, `"sub_blocks"`, `"refusal"`}
	for _, key := range forbiddenBlockKeys {
		if strings.Contains(blockStr, key) {
			t.Errorf("zero-value Block JSON should not contain %s, got: %s",
				key, blockStr)
		}
	}
}
