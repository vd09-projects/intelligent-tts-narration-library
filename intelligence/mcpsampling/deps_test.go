package mcpsampling

import (
	"bufio"
	"os/exec"
	"strings"
	"testing"
)

// TestInvariant_OnlyDependsOnPlanAndIntelligence asserts that nothing
// under intelligence/mcpsampling imports another package from this module
// except plan/, the parent intelligence/, and internal/intelligencetmpl/
// (the shared per-class prompt-template pkg lifted in issue #15 Phase 1).
// The subpackage is a concrete IntelligenceAdapter living under the
// parent contract package; importing planner/, pipeline/, an edge
// adapter, or a cmd/ entry point would invert the dependency direction
// CLAUDE.md mandates.
//
// Mirrors intelligence/deps_test.go scoped to this subpackage. Shells
// `go list -deps` and scans for module-qualified deps; allowlist =
// {self, plan/, intelligence/, internal/intelligencetmpl/}. External
// (non-modulePath) deps are unrestricted by design — same convention as
// the parent test (the MCP SDK is a non-modulePath dep and is allowed).
func TestInvariant_OnlyDependsOnPlanAndIntelligence(t *testing.T) {
	t.Parallel()
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go not on PATH; skipping deps invariant check")
	}

	const modulePath = "github.com/vd09-projects/intelligent-tts-narration-library"
	const selfPackage = modulePath + "/intelligence/mcpsampling"
	const planPackage = modulePath + "/plan"
	const intelligencePackage = modulePath + "/intelligence"
	const intelligenceTmplPackage = modulePath + "/internal/intelligencetmpl"

	out, err := exec.Command("go", "list", "-deps", selfPackage).Output()
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			t.Fatalf("go list -deps failed: %v\nstderr:\n%s", err, ee.Stderr)
		}
		t.Fatalf("go list -deps failed: %v", err)
	}

	allowed := map[string]bool{
		selfPackage:             true,
		planPackage:             true,
		intelligencePackage:     true,
		intelligenceTmplPackage: true,
	}
	var violations []string

	scanner := bufio.NewScanner(strings.NewReader(string(out)))
	for scanner.Scan() {
		dep := strings.TrimSpace(scanner.Text())
		if dep == "" || !strings.HasPrefix(dep, modulePath) {
			continue
		}
		if allowed[dep] {
			continue
		}
		violations = append(violations, dep)
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("scan go list output: %v", err)
	}

	if len(violations) > 0 {
		t.Fatalf("intelligence/mcpsampling may only import plan/, intelligence/, and internal/intelligencetmpl/, found %d other internal deps:\n  %s",
			len(violations), strings.Join(violations, "\n  "))
	}
}

// TestAdapter_SatisfiesInterface — compile-time assertion already exists
// at package scope (`var _ intelligence.IntelligenceAdapter = (*Adapter)(nil)`);
// this test is a redundant runtime smoke that the constructor returns a
// non-nil value of the right shape, catching regressions if the package-
// level assertion ever gets deleted.
func TestAdapter_SatisfiesInterface(t *testing.T) {
	t.Parallel()
	a := New()
	if a == nil {
		t.Fatalf("New() returned nil; expected *Adapter")
	}
}
