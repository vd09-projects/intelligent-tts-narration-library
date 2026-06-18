package plan

import (
	"encoding/json"
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
