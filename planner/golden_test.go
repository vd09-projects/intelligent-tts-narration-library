package planner

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vd09-projects/intelligent-tts-narration-library/intelligence"
	"github.com/vd09-projects/intelligent-tts-narration-library/plan"
)

// updateGolden — `go test ./planner -update` rewrites .want.json files
// from current planner output. Use when intentionally changing voicing
// or schema; never run blindly.
var updateGolden = flag.Bool("update", false, "rewrite golden plan.json fixtures")

// goldenCase — one fixture pair. Input is `name.{md|txt}`, expected
// output is `name.want.json`. Reads happen here (test code), keeping
// planner/ source I/O-free.
type goldenCase struct {
	name      string
	inputFile string
	level     plan.Level
	intel     intelligence.IntelligenceAdapter
}

// goldenCases — the ≥10 fixtures the AC requires.
//
// Naming convention: `{class}_{level}_{variant}` so a maintainer can
// scan the directory and infer coverage at a glance.
func goldenCases() []goldenCase {
	scripted := func(replies ...string) *scriptedIntel {
		rs := make([]intelligence.IntelligenceResult, len(replies))
		for i, r := range replies {
			rs[i] = intelligence.IntelligenceResult{Text: r, Model: "scripted@test"}
		}
		return &scriptedIntel{replies: rs}
	}
	return []goldenCase{
		{name: "prose_l1_short", inputFile: "prose_l1_short.md", level: plan.L1, intel: scripted("This is the L1 gist of the short paragraph.")},
		{name: "prose_l2_short", inputFile: "prose_l1_short.md", level: plan.L2, intel: scripted("This is the L2 summary expanding on the short paragraph.")},
		{name: "prose_l3_short", inputFile: "prose_l1_short.md", level: plan.L3, intel: scripted("This is the full L3 read-through of the short paragraph, every word.")},
		{name: "prose_degraded_verbatim", inputFile: "prose_l1_short.md", level: plan.L1, intel: nil},
		{name: "prose_refused_too_large", inputFile: "prose_refused_too_large.md", level: plan.L1, intel: nil},
		{name: "code_l1", inputFile: "code_l1.md", level: plan.L1, intel: nil},
		{name: "code_l2_intel", inputFile: "code_l2_small.md", level: plan.L2, intel: scripted("Computes the factorial of n recursively.")},
		{name: "code_l2_degraded", inputFile: "code_l2_small.md", level: plan.L2, intel: nil},
		{name: "code_l2_oversized_gate", inputFile: "code_l2_oversized.md", level: plan.L2, intel: scripted("SHOULD NOT APPEAR")},
		{name: "config_l1", inputFile: "config_l1.md", level: plan.L1, intel: nil},
		{name: "table_l1", inputFile: "table_l1.md", level: plan.L1, intel: nil},
		{name: "diagram_l1", inputFile: "diagram_l1.md", level: plan.L1, intel: nil},
		{name: "list_l1", inputFile: "list_l1.md", level: plan.L1, intel: nil},
		{name: "refused_bare_image", inputFile: "refused_bare_image.md", level: plan.L1, intel: nil},
		{name: "mixed_doc", inputFile: "mixed_doc.md", level: plan.L1, intel: nil},
	}
}

func TestGoldenFixtures(t *testing.T) {
	seams := deterministicSeams(t)
	for _, tc := range goldenCases() {
		t.Run(tc.name, func(t *testing.T) {
			inPath := filepath.Join("testdata", tc.inputFile)
			data, err := os.ReadFile(inPath)
			if err != nil {
				t.Fatalf("read input %s: %v", inPath, err)
			}
			doc := stubDoc(string(data), tc.inputFile)
			// Reset scripted intel call counter per case so each
			// test's scripted replies start at index 0 regardless of
			// the order tests run.
			if si, ok := tc.intel.(*scriptedIntel); ok && si != nil {
				si.calls = 0
				si.reqs = nil
			}
			got, err := Plan(context.Background(), doc, Request{Level: tc.level}, tc.intel, seams...)
			if err != nil {
				t.Fatalf("Plan: %v", err)
			}
			gotBytes, err := json.MarshalIndent(got, "", "  ")
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}
			gotBytes = append(gotBytes, '\n')

			wantPath := filepath.Join("testdata", tc.name+".want.json")
			if *updateGolden {
				if err := os.WriteFile(wantPath, gotBytes, 0o644); err != nil {
					t.Fatalf("write golden %s: %v", wantPath, err)
				}
				return
			}
			wantBytes, err := os.ReadFile(wantPath)
			if err != nil {
				t.Fatalf("read golden %s: %v (run -update to regenerate)", wantPath, err)
			}
			if string(gotBytes) != string(wantBytes) {
				t.Errorf("golden mismatch for %s\n--- want ---\n%s\n--- got ---\n%s",
					tc.name, summarizeForDiff(string(wantBytes)), summarizeForDiff(string(gotBytes)))
			}
			// Honesty-rule loop assertion — every refused block in
			// every fixture must satisfy Spoken=true + populated
			// SourceMap.
			for _, blk := range got.Blocks {
				if !honestyRuleSatisfied(blk) {
					t.Errorf("fixture %s block %s violated honesty rule: %+v", tc.name, blk.ID, blk)
				}
			}
		})
	}
}

// TestGolden_CodeL1ByteIdentical — explicit, dedicated regression guard
// (issue #48, Suggestion 3). The L2 enrichment must NOT perturb L1 code
// output. This reads the frozen code_l1.want.json and asserts the freshly
// produced plan is byte-identical, independent of the table-driven loop
// above so a maintainer sees the L1-stability contract named directly.
func TestGolden_CodeL1ByteIdentical(t *testing.T) {
	seams := deterministicSeams(t)
	data, err := os.ReadFile(filepath.Join("testdata", "code_l1.md"))
	if err != nil {
		t.Fatalf("read code_l1.md: %v", err)
	}
	got, err := Plan(context.Background(), stubDoc(string(data), "code_l1.md"), Request{Level: plan.L1}, nil, seams...)
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	gotBytes, err := json.MarshalIndent(got, "", "  ")
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	gotBytes = append(gotBytes, '\n')
	wantBytes, err := os.ReadFile(filepath.Join("testdata", "code_l1.want.json"))
	if err != nil {
		t.Fatalf("read code_l1.want.json: %v", err)
	}
	if string(gotBytes) != string(wantBytes) {
		t.Errorf("code_l1 golden drifted — L1 code behavior must stay byte-identical:\n--- want ---\n%s\n--- got ---\n%s",
			string(wantBytes), string(gotBytes))
	}
}

// summarizeForDiff — print up to first 30 lines and last 5 lines so the
// diff is readable even on a large plan.json.
func summarizeForDiff(s string) string {
	lines := strings.Split(s, "\n")
	if len(lines) <= 40 {
		return s
	}
	out := strings.Join(lines[:30], "\n")
	out += fmt.Sprintf("\n... (%d lines elided) ...\n", len(lines)-35)
	out += strings.Join(lines[len(lines)-5:], "\n")
	return out
}
